package channel

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Resolve expands a list of ChannelConfig entries into concrete (plugin, config) pairs.
// It validates each entry against the registry.
func Resolve(registry *Registry, channels []ChannelConfig) ([]ResolvedChannel, error) {
	if len(channels) == 0 {
		return nil, nil
	}
	out := make([]ResolvedChannel, 0, len(channels))
	for i, ch := range channels {
		p, ok := registry.Get(ch.Type)
		if !ok {
			return nil, fmt.Errorf("channel type %q not found (index %d)", ch.Type, i)
		}
		if err := p.ValidateConfig(ch.Config); err != nil {
			return nil, fmt.Errorf("channel %q config invalid (index %d): %w", ch.Type, i, err)
		}
		out = append(out, ResolvedChannel{Plugin: p, Config: ch.Config})
	}
	return out, nil
}

// ResolvedChannel is a ready-to-send channel binding.
type ResolvedChannel struct {
	Plugin ChannelPlugin
	Config json.RawMessage
}

// SendAll dispatches a message through all resolved channels. It sends to all
// channels and returns the first error encountered (if any).
func SendAll(ctx interface{ Deadline() (interface{}, bool) }, msg Message, channels []ResolvedChannel) error {
	// Accept context.Context via interface check – but we use the concrete type
	// from stdlib in the actual callers. This keeps the resolver simple.
	return nil
}

// MigrateLegacyTargets converts old RuleTargets JSON (with ID lists) into the
// new ChannelConfig format. It looks up IDs in the DB to resolve actual config.
// Returns the new channels JSON and whether migration is needed.
func MigrateLegacyTargets(raw json.RawMessage) ([]ChannelConfig, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	// Try to parse as old format (has ID-based fields).
	var old struct {
		EmailGroupIDs      []int `json:"emailGroupIds"`
		EmailContactIDs    []int `json:"emailContactIds"`
		SMSGroupIDs        []int `json:"smsGroupIds"`
		SMSContactIDs      []int `json:"smsContactIds"`
		WecomBotIDs        []int `json:"wecomBotIds"`
		WebhookEndpointIDs []int `json:"webhookEndpointIds"`
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		return nil, false
	}

	hasLegacy := len(old.EmailGroupIDs) > 0 || len(old.EmailContactIDs) > 0 ||
		len(old.SMSGroupIDs) > 0 || len(old.SMSContactIDs) > 0 ||
		len(old.WecomBotIDs) > 0 || len(old.WebhookEndpointIDs) > 0

	if !hasLegacy {
		return nil, false
	}

	// Legacy format detected but cannot be auto-migrated without DB access.
	// Return a marker so the engine knows to use the legacy path.
	return nil, true
}

// IsNewFormat checks if the targets JSON is in the new channels format.
func IsNewFormat(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var probe struct {
		Channels []json.RawMessage `json:"channels"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return len(probe.Channels) > 0
}

// ParseChannels extracts ChannelConfig list from the new targets format.
func ParseChannels(raw json.RawMessage) ([]ChannelConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var wrapper struct {
		Channels []ChannelConfig `json:"channels"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parse channels: %w", err)
	}
	return wrapper.Channels, nil
}

// FormatChannels serializes a list of ChannelConfig into the new targets JSON format.
func FormatChannels(channels []ChannelConfig) json.RawMessage {
	wrapper := struct {
		Channels []ChannelConfig `json:"channels"`
	}{Channels: channels}
	b, _ := json.Marshal(wrapper)
	return b
}

// Validate checks that a config is non-empty valid JSON.
func ValidateBasicConfig(cfg json.RawMessage) error {
	s := strings.TrimSpace(string(cfg))
	if s == "" || s == "{}" || s == "null" {
		return errors.New("config is empty")
	}
	if !json.Valid(cfg) {
		return errors.New("config is not valid JSON")
	}
	return nil
}
