package queue

import (
	"errors"
	"time"

	"github.com/nsqio/go-nsq"
)

type NSQPublisher struct {
	producer *nsq.Producer
}

func NewNSQPublisher(nsqdAddress string) (*NSQPublisher, error) {
	if nsqdAddress == "" {
		return nil, errors.New("nsqd address is empty")
	}
	cfg := nsq.NewConfig()
	cfg.DialTimeout = 2 * time.Second
	// go-nsq requires ReadTimeout > HeartbeatInterval (default heartbeat is 30s).
	cfg.ReadTimeout = 35 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	producer, err := nsq.NewProducer(nsqdAddress, cfg)
	if err != nil {
		return nil, err
	}
	return &NSQPublisher{producer: producer}, nil
}

func (p *NSQPublisher) Publish(topic string, body []byte) error {
	return p.producer.Publish(topic, body)
}

func (p *NSQPublisher) MultiPublish(topic string, bodies [][]byte) error {
	return p.producer.MultiPublish(topic, bodies)
}

func (p *NSQPublisher) Stop() {
	p.producer.Stop()
}
