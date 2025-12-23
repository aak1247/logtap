package queue

// Publisher is a minimal interface for ingest handlers.
type Publisher interface {
	Publish(topic string, body []byte) error
}
