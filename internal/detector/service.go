package detector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrServiceNotConfigured = errors.New("detector service not configured")
	ErrDetectorNotFound     = errors.New("detector type not found")
)

type Service struct {
	Registry *Registry
	Store    *ResultStore
	Now      func() time.Time
}

func NewService(registry *Registry, store *ResultStore) *Service {
	return &Service{
		Registry: registry,
		Store:    store,
		Now:      time.Now,
	}
}

func (s *Service) ListDescriptors() ([]Descriptor, error) {
	if s == nil || s.Registry == nil {
		return nil, ErrServiceNotConfigured
	}
	return s.Registry.List(), nil
}

func (s *Service) GetSchema(detectorType string) (json.RawMessage, error) {
	p, err := s.getPlugin(detectorType)
	if err != nil {
		return nil, err
	}
	schema := p.ConfigSchema()
	if len(strings.TrimSpace(string(schema))) == 0 {
		return json.RawMessage(`{"type":"object","additionalProperties":true}`), nil
	}
	// Ensure schema is valid JSON.
	var anyJSON any
	if err := json.Unmarshal(schema, &anyJSON); err != nil {
		return nil, fmt.Errorf("invalid detector schema: %w", err)
	}
	return schema, nil
}

func (s *Service) Validate(detectorType string, cfg json.RawMessage) error {
	p, err := s.getPlugin(detectorType)
	if err != nil {
		return err
	}
	if err := p.ValidateConfig(cfg); err != nil {
		return fmt.Errorf("validate detector config: %w", err)
	}
	return nil
}

func (s *Service) TestExecute(ctx context.Context, detectorType string, req ExecuteRequest) ([]Signal, time.Duration, error) {
	p, err := s.getPlugin(detectorType)
	if err != nil {
		return nil, 0, err
	}
	start := s.nowUTC()
	signals, err := p.Execute(ctx, req)
	elapsed := s.nowUTC().Sub(start)
	return signals, elapsed, err
}

func (s *Service) HealthCheck(ctx context.Context, detectorType string) error {
	p, err := s.getPlugin(detectorType)
	if err != nil {
		return err
	}
	hc, ok := p.(HealthCheckPlugin)
	if !ok {
		return fmt.Errorf("detector %s does not implement HealthCheckPlugin", detectorType)
	}
	return hc.HealthCheck(ctx)
}

func (s *Service) Aggregate(ctx context.Context, detectorType string, projectID int, tr TimeRange, interval AggregateInterval) ([]MetricPoint, error) {
	p, err := s.getPlugin(detectorType)
	if err != nil {
		return nil, err
	}
	agg, ok := p.(AggregatablePlugin)
	if !ok {
		return nil, fmt.Errorf("detector %s does not implement AggregatablePlugin", detectorType)
	}
	return agg.Aggregate(ctx, projectID, tr, interval)
}

func (s *Service) getPlugin(detectorType string) (DetectorPlugin, error) {
	if s == nil || s.Registry == nil {
		return nil, ErrServiceNotConfigured
	}
	typ := strings.ToLower(strings.TrimSpace(detectorType))
	if typ == "" {
		return nil, fmt.Errorf("%w: empty type", ErrDetectorNotFound)
	}
	p, ok := s.Registry.Get(typ)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDetectorNotFound, typ)
	}
	return p, nil
}

func (s *Service) nowUTC() time.Time {
	if s == nil || s.Now == nil {
		return time.Now().UTC()
	}
	t := s.Now()
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
