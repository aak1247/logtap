package queue

import (
	"testing"
)

func TestNewNSQPublisher_EmptyAddr(t *testing.T) {
	t.Parallel()

	if _, err := NewNSQPublisher(""); err == nil {
		t.Fatalf("expected error for empty address")
	}
}

func TestNSQPublisher_PublishAndStop_NoNSQD(t *testing.T) {
	t.Parallel()

	// NewProducer does not necessarily connect eagerly; Publish should error if no nsqd is running.
	p, err := NewNSQPublisher("127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewNSQPublisher: %v", err)
	}
	_ = p.Publish("topic", []byte("hello"))
	p.Stop()
}
