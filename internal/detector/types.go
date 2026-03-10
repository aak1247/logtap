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
