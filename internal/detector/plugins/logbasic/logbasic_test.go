package logbasic

import (
	"context"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

func TestPlugin_Execute(t *testing.T) {
	t.Parallel()

	p := New()
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	out, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 7,
		Now:       now,
		Payload: map[string]any{
			"source":   "logs",
			"severity": "error",
			"status":   "firing",
			"message":  "db timeout",
			"labels": map[string]any{
				"env": "prod",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(out))
	}
	if out[0].ProjectID != 7 {
		t.Fatalf("unexpected project id: %d", out[0].ProjectID)
	}
	if out[0].SourceType != "log_basic" {
		t.Fatalf("unexpected source type: %q", out[0].SourceType)
	}
	if out[0].Labels["env"] != "prod" {
		t.Fatalf("expected label env=prod")
	}
}

func TestPlugin_ValidateConfig(t *testing.T) {
	t.Parallel()

	p := New()
	if err := p.ValidateConfig(nil); err != nil {
		t.Fatalf("ValidateConfig nil: %v", err)
	}
	if err := p.ValidateConfig([]byte(`{"enabled":true}`)); err != nil {
		t.Fatalf("ValidateConfig object: %v", err)
	}
	if err := p.ValidateConfig([]byte(`[`)); err == nil {
		t.Fatalf("expected invalid json error")
	}
}
