package detector

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestService_ListSchemaValidateAndExecute(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	if err := reg.RegisterStatic(stubPlugin{typ: "http_check"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := NewService(reg)

	items, err := svc.ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors: %v", err)
	}
	if len(items) != 1 || items[0].Type != "http_check" {
		t.Fatalf("unexpected descriptors: %+v", items)
	}

	schema, err := svc.GetSchema("http_check")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if string(schema) == "" {
		t.Fatalf("expected non-empty schema")
	}

	if err := svc.Validate("http_check", []byte(`{"k":"v"}`)); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	sig, elapsed, err := svc.TestExecute(context.Background(), "http_check", ExecuteRequest{})
	if err != nil {
		t.Fatalf("TestExecute: %v", err)
	}
	if len(sig) != 0 {
		t.Fatalf("expected empty signals, got %d", len(sig))
	}
	if elapsed < 0 {
		t.Fatalf("expected non-negative elapsed")
	}
}

func TestService_Errors(t *testing.T) {
	t.Parallel()

	svc := NewService(nil)
	if _, err := svc.ListDescriptors(); !errors.Is(err, ErrServiceNotConfigured) {
		t.Fatalf("expected ErrServiceNotConfigured, got %v", err)
	}
	reg := NewRegistry()
	svc = NewService(reg)

	if _, err := svc.GetSchema("missing"); !errors.Is(err, ErrDetectorNotFound) {
		t.Fatalf("expected ErrDetectorNotFound, got %v", err)
	}
	if err := svc.Validate("missing", nil); !errors.Is(err, ErrDetectorNotFound) {
		t.Fatalf("expected ErrDetectorNotFound, got %v", err)
	}
	if _, _, err := svc.TestExecute(context.Background(), "missing", ExecuteRequest{}); !errors.Is(err, ErrDetectorNotFound) {
		t.Fatalf("expected ErrDetectorNotFound, got %v", err)
	}
}

func TestService_TestExecuteElapsed(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	if err := reg.RegisterStatic(stubPlugin{typ: "http_check"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	now := time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)
	svc := NewService(reg)
	svc.Now = func() time.Time {
		now = now.Add(5 * time.Millisecond)
		return now
	}
	_, elapsed, err := svc.TestExecute(context.Background(), "http_check", ExecuteRequest{})
	if err != nil {
		t.Fatalf("TestExecute: %v", err)
	}
	if elapsed != 5*time.Millisecond {
		t.Fatalf("expected 5ms elapsed, got %v", elapsed)
	}
}
