package logtap

import "time"

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
	LevelFatal Level = "fatal"
	LevelEvent Level = "event"
)

type User struct {
	ID       string         `json:"id,omitempty"`
	Email    string         `json:"email,omitempty"`
	Username string         `json:"username,omitempty"`
	Traits   map[string]any `json:"traits,omitempty"`
}

type LogPayload struct {
	Level     Level             `json:"level"`
	Message   string            `json:"message"`
	Timestamp *time.Time        `json:"timestamp,omitempty"`
	DeviceID  string            `json:"device_id,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
	SpanID    string            `json:"span_id,omitempty"`
	Fields    map[string]any    `json:"fields,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	User      map[string]any    `json:"user,omitempty"`
	Contexts  map[string]any    `json:"contexts,omitempty"`
	Extra     map[string]any    `json:"extra,omitempty"`
	SDK       map[string]any    `json:"sdk,omitempty"`
}

type TrackEventPayload struct {
	Name       string            `json:"name"`
	Timestamp  *time.Time        `json:"timestamp,omitempty"`
	DeviceID   string            `json:"device_id,omitempty"`
	TraceID    string            `json:"trace_id,omitempty"`
	SpanID     string            `json:"span_id,omitempty"`
	Properties map[string]any    `json:"properties,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	User       map[string]any    `json:"user,omitempty"`
	Contexts   map[string]any    `json:"contexts,omitempty"`
	Extra      map[string]any    `json:"extra,omitempty"`
	SDK        map[string]any    `json:"sdk,omitempty"`
}

type LogOptions struct {
	Timestamp time.Time
	TraceID   string
	SpanID    string
	Tags      map[string]string
	DeviceID  string
	User      *User
	Contexts  map[string]any
	Extra     map[string]any
}

type TrackOptions struct {
	Timestamp time.Time
	TraceID   string
	SpanID    string
	Tags      map[string]string
	DeviceID  string
	User      *User
	Contexts  map[string]any
	Extra     map[string]any
	Immediate bool
}
