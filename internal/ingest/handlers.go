package ingest

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/queue"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type NSQMessage struct {
	Type      string          `json:"type"`
	ProjectID string          `json:"project_id"`
	Received  time.Time       `json:"received"`
	Payload   json.RawMessage `json:"payload"`
	Meta      *MessageMeta    `json:"meta,omitempty"`
}

type MessageMeta struct {
	ClientIP  string `json:"client_ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

type CustomLogPayload struct {
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	DeviceID  string            `json:"device_id,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
	SpanID    string            `json:"span_id,omitempty"`
	Fields    map[string]any    `json:"fields,omitempty"`
	Timestamp *time.Time        `json:"timestamp,omitempty"`
	Extra     map[string]any    `json:"extra,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	User      map[string]any    `json:"user,omitempty"`
	SDK       map[string]any    `json:"sdk,omitempty"`
	Contexts  map[string]any    `json:"contexts,omitempty"`
}

type TrackEventPayload struct {
	Name       string            `json:"name"`
	Properties map[string]any    `json:"properties,omitempty"`
	DeviceID   string            `json:"device_id,omitempty"`
	TraceID    string            `json:"trace_id,omitempty"`
	SpanID     string            `json:"span_id,omitempty"`
	Timestamp  *time.Time        `json:"timestamp,omitempty"`
	Extra      map[string]any    `json:"extra,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	User       map[string]any    `json:"user,omitempty"`
	SDK        map[string]any    `json:"sdk,omitempty"`
	Contexts   map[string]any    `json:"contexts,omitempty"`
}

func SentryStoreHandler(publisher queue.Publisher) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := readBody(c, 5<<20)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		var event map[string]any
		if err := json.Unmarshal(body, &event); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		eventID, _ := event["event_id"].(string)
		if eventID == "" {
			eventID = uuid.NewString()
			event["event_id"] = eventID
		}

		payload, _ := json.Marshal(NSQMessage{
			Type:      "event",
			ProjectID: c.Param("projectId"),
			Received:  time.Now().UTC(),
			Payload:   mustJSON(event),
			Meta: &MessageMeta{
				ClientIP:  c.ClientIP(),
				UserAgent: c.GetHeader("User-Agent"),
			},
		})
		if err := publisher.Publish("events", payload); err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}

		c.JSON(http.StatusOK, gin.H{"id": eventID})
	}
}

func SentryEnvelopeHandler(publisher queue.Publisher) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := readBody(c, 20<<20)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		env, err := ParseEnvelope(body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		eventID, _ := env.Header["event_id"].(string)
		if eventID == "" {
			eventID = uuid.NewString()
			env.Header["event_id"] = eventID
		}

		if event, ok := env.FirstEventJSON(); ok {
			if _, has := event["event_id"].(string); !has {
				event["event_id"] = eventID
			}
			payload, _ := json.Marshal(NSQMessage{
				Type:      "event",
				ProjectID: c.Param("projectId"),
				Received:  time.Now().UTC(),
				Payload:   mustJSON(event),
				Meta: &MessageMeta{
					ClientIP:  c.ClientIP(),
					UserAgent: c.GetHeader("User-Agent"),
				},
			})
			if err := publisher.Publish("events", payload); err != nil {
				c.Status(http.StatusServiceUnavailable)
				return
			}
		} else {
			// If we can't parse an event item, still enqueue the raw envelope for later processing.
			payload, _ := json.Marshal(NSQMessage{
				Type:      "envelope",
				ProjectID: c.Param("projectId"),
				Received:  time.Now().UTC(),
				Payload:   json.RawMessage(mustJSON(map[string]any{"event_id": eventID, "raw": string(body)})),
				Meta: &MessageMeta{
					ClientIP:  c.ClientIP(),
					UserAgent: c.GetHeader("User-Agent"),
				},
			})
			if err := publisher.Publish("events", payload); err != nil {
				c.Status(http.StatusServiceUnavailable)
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"id": eventID})
	}
}

func CustomLogHandler(publisher queue.Publisher) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := readBody(c, 5<<20)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		items, err := decodeOneOrMany[CustomLogPayload](body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()

		for _, logPayload := range items {
			if strings.TrimSpace(logPayload.Message) == "" {
				c.Status(http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(logPayload.Level) == "" {
				logPayload.Level = "info"
			}
			if logPayload.Timestamp == nil {
				ts := now
				logPayload.Timestamp = &ts
			}

			payload, _ := json.Marshal(NSQMessage{
				Type:      "log",
				ProjectID: c.Param("projectId"),
				Received:  now,
				Payload:   mustJSON(logPayload),
				Meta: &MessageMeta{
					ClientIP:  c.ClientIP(),
					UserAgent: c.GetHeader("User-Agent"),
				},
			})
			if err := publisher.Publish("logs", payload); err != nil {
				c.Status(http.StatusServiceUnavailable)
				return
			}
		}

		c.Status(http.StatusAccepted)
	}
}

func TrackEventHandler(publisher queue.Publisher) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := readBody(c, 5<<20)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		items, err := decodeOneOrMany[TrackEventPayload](body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()

		for _, ev := range items {
			name := strings.TrimSpace(ev.Name)
			if name == "" {
				c.Status(http.StatusBadRequest)
				return
			}
			if ev.Timestamp == nil {
				ts := now
				ev.Timestamp = &ts
			}
			logPayload := CustomLogPayload{
				Level:     "event",
				Message:   name,
				DeviceID:  ev.DeviceID,
				TraceID:   ev.TraceID,
				SpanID:    ev.SpanID,
				Fields:    ev.Properties,
				Timestamp: ev.Timestamp,
				Extra:     ev.Extra,
				Tags:      ev.Tags,
				User:      ev.User,
				SDK:       ev.SDK,
				Contexts:  ev.Contexts,
			}

			payload, _ := json.Marshal(NSQMessage{
				Type:      "log",
				ProjectID: c.Param("projectId"),
				Received:  now,
				Payload:   mustJSON(logPayload),
				Meta: &MessageMeta{
					ClientIP:  c.ClientIP(),
					UserAgent: c.GetHeader("User-Agent"),
				},
			})
			if err := publisher.Publish("logs", payload); err != nil {
				c.Status(http.StatusServiceUnavailable)
				return
			}
		}

		c.Status(http.StatusAccepted)
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func decodeOneOrMany[T any](body []byte) ([]T, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, errors.New("empty body")
	}
	if body[0] == byte('[') {
		var items []T
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, errors.New("empty array")
		}
		return items, nil
	}
	var item T
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, err
	}
	return []T{item}, nil
}

func readBody(c *gin.Context, limit int64) ([]byte, error) {
	defer c.Request.Body.Close()

	raw := io.LimitReader(c.Request.Body, limit)
	enc := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Encoding")))
	if strings.Contains(enc, "gzip") {
		zr, err := gzip.NewReader(raw)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(io.LimitReader(zr, limit))
	}
	return io.ReadAll(raw)
}
