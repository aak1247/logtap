package consumer

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrBatcherClosed = errors.New("batcher closed")

type flushFunc[T any] func(ctx context.Context, items []T) error

type batchReq[T any] struct {
	item T
	done chan error
}

type Batcher[T any] struct {
	maxSize       int
	flushInterval time.Duration
	flushTimeout  time.Duration
	flush         flushFunc[T]

	in     chan batchReq[T]
	stopCh chan struct{}
	doneCh chan struct{}

	closeOnce sync.Once
}

func NewBatcher[T any](maxSize int, flushInterval, flushTimeout time.Duration, flush flushFunc[T]) *Batcher[T] {
	if maxSize <= 0 {
		maxSize = 200
	}
	if flushInterval <= 0 {
		flushInterval = 50 * time.Millisecond
	}
	if flushTimeout <= 0 {
		flushTimeout = 5 * time.Second
	}
	if flush == nil {
		panic("nil flush func")
	}

	b := &Batcher[T]{
		maxSize:       maxSize,
		flushInterval: flushInterval,
		flushTimeout:  flushTimeout,
		flush:         flush,
		in:            make(chan batchReq[T], maxSize*2),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	go b.loop()
	return b
}

func (b *Batcher[T]) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() { close(b.stopCh) })
	<-b.doneCh
}

func (b *Batcher[T]) Add(item T) error {
	if b == nil {
		return ErrBatcherClosed
	}
	done := make(chan error, 1)
	req := batchReq[T]{item: item, done: done}

	select {
	case <-b.stopCh:
		return ErrBatcherClosed
	case b.in <- req:
	}

	select {
	case err := <-done:
		return err
	case <-b.stopCh:
		return ErrBatcherClosed
	}
}

func (b *Batcher[T]) loop() {
	defer close(b.doneCh)

	var (
		batch       []batchReq[T]
		timer       = time.NewTimer(b.flushInterval)
		timerActive bool
	)
	if !timer.Stop() {
		<-timer.C
	}

	flush := func(items []batchReq[T]) {
		if len(items) == 0 {
			return
		}
		rows := make([]T, 0, len(items))
		for _, it := range items {
			rows = append(rows, it.item)
		}

		ctx, cancel := context.WithTimeout(context.Background(), b.flushTimeout)
		err := b.flush(ctx, rows)
		cancel()

		for _, it := range items {
			it.done <- err
			close(it.done)
		}
	}

	stopTimer := func() {
		if !timerActive {
			return
		}
		timerActive = false
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}

	startTimer := func() {
		if timerActive {
			return
		}
		timerActive = true
		timer.Reset(b.flushInterval)
	}

	for {
		var timerCh <-chan time.Time
		if timerActive {
			timerCh = timer.C
		}

		select {
		case req := <-b.in:
			if len(batch) == 0 {
				startTimer()
			}
			batch = append(batch, req)
			if len(batch) >= b.maxSize {
				stopTimer()
				flush(batch)
				batch = batch[:0]
			}
		case <-timerCh:
			stopTimer()
			flush(batch)
			batch = batch[:0]
		case <-b.stopCh:
			stopTimer()
			for {
				select {
				case req := <-b.in:
					batch = append(batch, req)
				default:
					flush(batch)
					return
				}
			}
		}
	}
}
