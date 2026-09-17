package queue

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/nsqio/go-nsq"
)

// producerPoolSize bounds how many nsqd connections ingest publishes fan
// out across. go-nsq serializes each producer's commands through one
// connection with a synchronous round trip per publish, so a single
// producer caps throughput at ~1/RTT regardless of caller concurrency.
const producerPoolSize = 8

type NSQPublisher struct {
	producers []*nsq.Producer
	next      atomic.Uint64
}

func NewNSQPublisher(nsqdAddress string) (*NSQPublisher, error) {
	if nsqdAddress == "" {
		return nil, errors.New("nsqd address is empty")
	}
	newProducer := func() (*nsq.Producer, error) {
		cfg := nsq.NewConfig()
		cfg.DialTimeout = 2 * time.Second
		// go-nsq requires ReadTimeout > HeartbeatInterval (default heartbeat is 30s).
		cfg.ReadTimeout = 35 * time.Second
		cfg.WriteTimeout = 5 * time.Second
		return nsq.NewProducer(nsqdAddress, cfg)
	}
	producers := make([]*nsq.Producer, 0, producerPoolSize)
	for i := 0; i < producerPoolSize; i++ {
		producer, err := newProducer()
		if err != nil {
			for _, p := range producers {
				p.Stop()
			}
			return nil, err
		}
		producers = append(producers, producer)
	}
	return &NSQPublisher{producers: producers}, nil
}

func (p *NSQPublisher) pick() *nsq.Producer {
	return p.producers[p.next.Add(1)%uint64(len(p.producers))]
}

func (p *NSQPublisher) Publish(topic string, body []byte) error {
	return p.pick().Publish(topic, body)
}

func (p *NSQPublisher) MultiPublish(topic string, bodies [][]byte) error {
	return p.pick().MultiPublish(topic, bodies)
}

func (p *NSQPublisher) Stop() {
	for _, producer := range p.producers {
		producer.Stop()
	}
}
