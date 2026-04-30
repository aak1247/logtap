package sslcheck

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aak1247/logtap/internal/detector"
)

func TestPlugin_Type(t *testing.T) {
	p := New()
	if p.Type() != "ssl_check" {
		t.Errorf("Type() = %q, want %q", p.Type(), "ssl_check")
	}
}

func TestPlugin_ValidateConfig(t *testing.T) {
	p := New()
	if err := p.ValidateConfig(json.RawMessage(`{"host":"example.com"}`)); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}
	if err := p.ValidateConfig(json.RawMessage(`{}`)); err == nil {
		t.Error("empty config should fail")
	}
	if err := p.ValidateConfig(json.RawMessage(`{"host":"example.com","port":99999}`)); err == nil {
		t.Error("invalid port should fail")
	}
}

func TestPlugin_Execute(t *testing.T) {
	p := New()
	signals, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    json.RawMessage(`{"host":"example.com"}`),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	// Should resolve successfully for example.com
	if signals[0].Severity == "error" {
		t.Logf("Warning: ssl_check got error for example.com: %s (may be network issue)", signals[0].Message)
	}
}

func TestPlugin_HealthCheck(t *testing.T) {
	p := New()
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Logf("HealthCheck warning (network dependent): %v", err)
	}
}

func TestPlugin_Lifecycle(t *testing.T) {
	p := New()
	if err := p.OnActivate(context.Background(), nil); err != nil {
		t.Errorf("OnActivate failed: %v", err)
	}
	if err := p.OnDeactivate(context.Background()); err != nil {
		t.Errorf("OnDeactivate failed: %v", err)
	}
}
