package queue

// Publisher is a minimal interface for ingest handlers.
type Publisher interface {
	Publish(topic string, body []byte) error
}

// BatchPublisher is optional; when available, handlers can reduce nsqd round-trips.
type BatchPublisher interface {
	Publisher
	MultiPublish(topic string, bodies [][]byte) error
}
