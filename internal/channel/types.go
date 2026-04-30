package channel

import (
	"context"
	"encoding/json"
)

// Message is the unified notification payload sent through all channels.
type Message struct {
	ProjectID int               `json:"projectId"`
	RuleID    int               `json:"ruleId"`
	RuleName  string            `json:"ruleName"`
	Title     string            `json:"title"`
	Content   string            `json:"content"`
	Level     string            `json:"level"`
	Source    string            `json:"source"`
	Fields    map[string]any    `json:"fields,omitempty"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// ChannelPlugin is the interface that every notification channel must implement.
type ChannelPlugin interface {
	// Type returns the unique channel type identifier, e.g. "wecom_bot", "webhook", "email", "feishu".
	Type() string

	// ConfigSchema returns a JSON Schema describing the per-channel configuration.
	ConfigSchema() json.RawMessage

	// ValidateConfig checks that a channel config blob is valid.
	ValidateConfig(cfg json.RawMessage) error

	// Send dispatches a notification message via this channel using the provided config.
	Send(ctx context.Context, msg Message, cfg json.RawMessage) error
}

// ChannelConfig is a single channel entry stored in an AlertRule.
type ChannelConfig struct {
	Type   string          `json:"type"`
	Config json.RawMessage `json:"config"`
}

// RegistrationMode indicates how a plugin was registered.
type RegistrationMode string

const (
	ModeStatic RegistrationMode = "static"
	ModePlugin RegistrationMode = "plugin"
)

// Descriptor holds metadata about a registered channel plugin.
type Descriptor struct {
	Type string
	Mode RegistrationMode
	Path string
}
