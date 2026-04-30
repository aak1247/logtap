package detector

import (
	"context"
	"encoding/json"
	"time"
)

type Signal struct {
	ProjectID  int
	Source     string
	SourceType string
	Severity   string
	Status     string
	Title      string
	Message    string
	Labels     map[string]string
	Fields     map[string]any
	OccurredAt time.Time
}

type ExecuteRequest struct {
	ProjectID int
	Config    json.RawMessage
	Payload   map[string]any
	Now       time.Time
}

type DetectorPlugin interface {
	Type() string
	ConfigSchema() json.RawMessage
	ValidateConfig(cfg json.RawMessage) error
	Execute(ctx context.Context, req ExecuteRequest) ([]Signal, error)
}

// LifecyclePlugin is an optional interface that plugins can implement to receive
// activation/deactivation callbacks when the monitor worker starts or stops.
type LifecyclePlugin interface {
	DetectorPlugin
	OnActivate(ctx context.Context, config json.RawMessage) error
	OnDeactivate(ctx context.Context) error
}

// HealthCheckPlugin is an optional interface for plugins that support health checks.
type HealthCheckPlugin interface {
	DetectorPlugin
	HealthCheck(ctx context.Context) error
}

// ResultStorePlugin is an optional interface for plugins that can persist and
// query structured results beyond simple signals.
type ResultStorePlugin interface {
	DetectorPlugin
	StoreResults(ctx context.Context, projectID int, results []TypedResult) error
	QueryResults(ctx context.Context, projectID int, query ResultQuery) ([]TypedResult, error)
}

// AggregatablePlugin is an optional interface for plugins that support time-series
// aggregation queries over historical results.
type AggregatablePlugin interface {
	DetectorPlugin
	Aggregate(ctx context.Context, projectID int, timeRange TimeRange, interval AggregateInterval) ([]MetricPoint, error)
}

// --- New types for enhanced detector framework ---

type TypedResult struct {
	DetectorType string
	ProjectID    int
	MonitorID    int
	Timestamp    time.Time
	Data         json.RawMessage
	Tags         map[string]string
}

type ResultQuery struct {
	ProjectID int
	MonitorID int
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

type MetricPoint struct {
	Timestamp time.Time
	Value     float64
	Labels    map[string]string
}

type TimeRange struct {
	Start time.Time
	End   time.Time
}

type AggregateInterval string

const (
	IntervalMinute AggregateInterval = "1m"
	IntervalHour   AggregateInterval = "1h"
	IntervalDay    AggregateInterval = "1d"
)

type RegistrationMode string

const (
	ModeStatic RegistrationMode = "static"
	ModePlugin RegistrationMode = "plugin"
)

type Descriptor struct {
	Type string
	Mode RegistrationMode
	Path string
}
