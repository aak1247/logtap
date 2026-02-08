package queue

import "github.com/aak1247/logtap/internal/obs"

type observedPublisher struct {
	inner Publisher
	stats *obs.Stats
}

func ObservePublisher(p Publisher, stats *obs.Stats) Publisher {
	if p == nil || stats == nil {
		return p
	}
	if _, ok := p.(*observedPublisher); ok {
		return p
	}
	return &observedPublisher{inner: p, stats: stats}
}

func (p *observedPublisher) Publish(topic string, body []byte) error {
	err := p.inner.Publish(topic, body)
	p.stats.ObserveNSQPublish(len(body), err)
	return err
}

func (p *observedPublisher) MultiPublish(topic string, bodies [][]byte) error {
	bp, ok := p.inner.(BatchPublisher)
	if !ok {
		// Fallback: publish one-by-one.
		for _, b := range bodies {
			if err := p.Publish(topic, b); err != nil {
				return err
			}
		}
		return nil
	}
	err := bp.MultiPublish(topic, bodies)
	total := 0
	for _, b := range bodies {
		total += len(b)
	}
	p.stats.ObserveNSQPublish(total, err)
	return err
}
