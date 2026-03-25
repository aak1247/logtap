package metricthreshold

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

func TestPlugin_ValidateConfig_Basic(t *testing.T) {
	p := Plugin{}

	if err := p.ValidateConfig(nil); err == nil {
		t.Fatalf("expected error for empty config")
	}

	v := 10.0
	cfg := mustJSON(map[string]any{"field": "value", "op": ">", "value": v})
	if err := p.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig valid: %v", err)
	}

	between := mustJSON(map[string]any{"field": "value", "op": "between", "min": 1, "max": 0})
	if err := p.ValidateConfig(between); err == nil {
		t.Fatalf("expected error for min>max")
	}
}

func TestPlugin_Execute_NoViolation(t *testing.T) {
	p := Plugin{}

	cfg := mustJSON(map[string]any{"field": "value", "op": ">", "value": 10})

	sigs, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    cfg,
		Payload: map[string]any{
			"value": 5,
		},
		Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sigs))
	}
	s := sigs[0]
	if s.Severity != "info" || s.Status != "resolved" {
		t.Fatalf("expected info/resolved, got severity=%q status=%q", s.Severity, s.Status)
	}
}

func TestPlugin_Execute_Violation(t *testing.T) {
	p := Plugin{}

	cfg := mustJSON(map[string]any{"field": "value", "op": ">", "value": 3, "severityOnViolation": "warn"})

	sigs, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    cfg,
		Payload: map[string]any{
			"value": 5,
		},
		Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sigs))
	}
	s := sigs[0]
	if s.Severity != "warn" || s.Status != "firing" {
		t.Fatalf("expected warn/firing, got severity=%q status=%q", s.Severity, s.Status)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}
