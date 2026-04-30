package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrServiceNotConfigured = errors.New("channel service not configured")
	ErrChannelNotFound      = errors.New("channel type not found")
)

// Service provides high-level channel operations.
type Service struct {
	Registry *Registry
}

func NewService(registry *Registry) *Service {
	return &Service{Registry: registry}
}

func (s *Service) ListDescriptors() ([]Descriptor, error) {
	if s == nil || s.Registry == nil {
		return nil, ErrServiceNotConfigured
	}
	return s.Registry.List(), nil
}

func (s *Service) GetSchema(channelType string) (json.RawMessage, error) {
	p, err := s.getPlugin(channelType)
	if err != nil {
		return nil, err
	}
	schema := p.ConfigSchema()
	if len(strings.TrimSpace(string(schema))) == 0 {
		return json.RawMessage(`{"type":"object","additionalProperties":true}`), nil
	}
	var anyJSON any
	if err := json.Unmarshal(schema, &anyJSON); err != nil {
		return nil, fmt.Errorf("invalid channel schema: %w", err)
	}
	return schema, nil
}

func (s *Service) Validate(channelType string, cfg json.RawMessage) error {
	p, err := s.getPlugin(channelType)
	if err != nil {
		return err
	}
	if err := p.ValidateConfig(cfg); err != nil {
		return fmt.Errorf("validate channel config: %w", err)
	}
	return nil
}

func (s *Service) Send(ctx context.Context, channelType string, msg Message, cfg json.RawMessage) error {
	p, err := s.getPlugin(channelType)
	if err != nil {
		return err
	}
	return p.Send(ctx, msg, cfg)
}

func (s *Service) getPlugin(channelType string) (ChannelPlugin, error) {
	if s == nil || s.Registry == nil {
		return nil, ErrServiceNotConfigured
	}
	typ := strings.ToLower(strings.TrimSpace(channelType))
	if typ == "" {
		return nil, fmt.Errorf("%w: empty type", ErrChannelNotFound)
	}
	p, ok := s.Registry.Get(typ)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrChannelNotFound, typ)
	}
	return p, nil
}
