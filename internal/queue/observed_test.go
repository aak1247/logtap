package queue

import (
	"errors"
	"testing"

	"github.com/aak1247/logtap/internal/obs"
)

type stubPublisher struct {
	err error
}

func (p stubPublisher) Publish(_ string, _ []byte) error { return p.err }

func TestObservePublisher_Publish(t *testing.T) {
	t.Parallel()

	stats := obs.New()
	p := ObservePublisher(stubPublisher{}, stats)

	if err := p.Publish("logs", []byte("x")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	snap := stats.Snapshot()
	if snap.NSQ.PublishTotal != 1 || snap.NSQ.PublishErrors != 0 || snap.NSQ.PublishBytes != 1 {
		t.Fatalf("unexpected snapshot: %+v", snap.NSQ)
	}
}

func TestObservePublisher_PublishError(t *testing.T) {
	t.Parallel()

	stats := obs.New()
	p := ObservePublisher(stubPublisher{err: errors.New("boom")}, stats)

	if err := p.Publish("logs", []byte("x")); err == nil {
		t.Fatalf("expected error")
	}
	snap := stats.Snapshot()
	if snap.NSQ.PublishTotal != 1 || snap.NSQ.PublishErrors != 1 {
		t.Fatalf("unexpected snapshot: %+v", snap.NSQ)
	}
}
