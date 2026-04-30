package dnscheck

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aak1247/logtap/internal/detector"
)

func TestPlugin_Type(t *testing.T) {
	p := New()
	if p.Type() != "dns_check" {
		t.Errorf("Type() = %q, want %q", p.Type(), "dns_check")
	}
}

func TestPlugin_ValidateConfig(t *testing.T) {
	p := New()
	if err := p.ValidateConfig(json.RawMessage(`{"domain":"example.com"}`)); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}
	if err := p.ValidateConfig(json.RawMessage(`{}`)); err == nil {
		t.Error("empty config should fail")
	}
	if err := p.ValidateConfig(json.RawMessage(`{"domain":"example.com","recordType":"INVALID"}`)); err == nil {
		t.Error("invalid recordType should fail")
	}
}

func TestPlugin_Execute(t *testing.T) {
	p := New()
	signals, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    json.RawMessage(`{"domain":"localhost","recordType":"A"}`),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
}

func TestPlugin_HealthCheck(t *testing.T) {
	p := New()
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Errorf("HealthCheck failed: %v", err)
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
