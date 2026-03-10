package detector

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type stubPlugin struct {
	typ string
}

func (s stubPlugin) Type() string { return s.typ }

func (stubPlugin) ConfigSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }

func (stubPlugin) ValidateConfig(_ json.RawMessage) error { return nil }

func (stubPlugin) Execute(_ context.Context, _ ExecuteRequest) ([]Signal, error) { return nil, nil }

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	if err := r.RegisterStatic(stubPlugin{typ: "http_check"}); err != nil {
		t.Fatalf("RegisterStatic: %v", err)
	}
	if err := r.RegisterDynamic("/tmp/detector.so", stubPlugin{typ: "process_check"}); err != nil {
		t.Fatalf("RegisterDynamic: %v", err)
	}

	p, ok := r.Get("HTTP_CHECK")
	if !ok {
		t.Fatalf("expected plugin to exist")
	}
	if p.Type() != "http_check" {
		t.Fatalf("unexpected plugin type %q", p.Type())
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 descriptors, got %d", len(list))
	}
}

func TestRegistry_RejectsDuplicateType(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	if err := r.RegisterStatic(stubPlugin{typ: "http_check"}); err != nil {
		t.Fatalf("RegisterStatic: %v", err)
	}
	err := r.RegisterStatic(stubPlugin{typ: "http_check"})
	if !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("expected ErrAlreadyRegistered, got %v", err)
	}
}
