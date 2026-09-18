package queue

import (
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

var ErrPublishQueueFull = errors.New("async publish queue full")

type asyncBatchMessage struct {
	topic  string
	bodies [][]byte
}

// AsyncBufferedPublisher decouples HTTP ingest from NSQ network round-trips.
// Ingest handlers enqueue batches into a channel with bounded queuing time.
type AsyncBufferedPublisher struct {
	underlying BatchPublisher
	ch         chan asyncBatchMessage
	stopCh     chan struct{}
	done       sync.WaitGroup

	dropped atomic.Int64
}

const (
	defaultAsyncBatchQueueSize = 10000
	defaultAsyncWorkers        = 16
	enqueueTimeout             = 1 * time.Second
)

func NewAsyncBufferedPublisher(underlying BatchPublisher) *AsyncBufferedPublisher {
	p := &AsyncBufferedPublisher{
		underlying: underlying,
		ch:         make(chan asyncBatchMessage, defaultAsyncBatchQueueSize),
		stopCh:     make(chan struct{}),
	}
	for i := 0; i < defaultAsyncWorkers; i++ {
		p.done.Add(1)
		go p.worker()
	}
	return p
}

func (p *AsyncBufferedPublisher) Publish(topic string, body []byte) error {
	return p.MultiPublish(topic, [][]byte{body})
}

func (p *AsyncBufferedPublisher) MultiPublish(topic string, bodies [][]byte) error {
	if len(bodies) == 0 {
		return nil
	}
	msg := asyncBatchMessage{topic: topic, bodies: bodies}
	
	timer := time.NewTimer(enqueueTimeout)
	defer timer.Stop()

	select {
	case p.ch <- msg:
		return nil
	case <-timer.C:
		n := p.dropped.Add(1)
		if n == 1 || n%1000 == 0 {
			log.Printf("async publisher enqueue timeout (buffer full), dropped=%d", n)
		}
		return ErrPublishQueueFull
	case <-p.stopCh:
		return errors.New("async publisher stopped")
	}
}

func (p *AsyncBufferedPublisher) Stop() {
	close(p.stopCh)
	p.done.Wait()
}

func (p *AsyncBufferedPublisher) worker() {
	defer p.done.Done()

	for {
		select {
		case <-p.stopCh:
			// Drain remaining messages
			for {
				select {
				case msg := <-p.ch:
					_ = p.underlying.MultiPublish(msg.topic, msg.bodies)
				default:
					return
				}
			}
		case msg := <-p.ch:
			// Direct chunked publish to NSQ
			if err := p.underlying.MultiPublish(msg.topic, msg.bodies); err != nil {
				log.Printf("async publisher flush %s (len=%d) error: %v", msg.topic, len(msg.bodies), err)
			}
		}
	}
}
