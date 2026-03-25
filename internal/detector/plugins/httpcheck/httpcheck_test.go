package httpcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

func TestPlugin_ValidateConfig_Basic(t *testing.T) {
	p := Plugin{}

	if err := p.ValidateConfig(nil); err == nil {
		t.Fatalf("expected error for empty config")
	}

	cfg := mustJSON(map[string]any{"url": "https://example.com"})
	if err := p.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig valid url: %v", err)
	}

	bad := mustJSON(map[string]any{"url": "ftp://example.com"})
	if err := p.ValidateConfig(bad); err == nil {
		t.Fatalf("expected error for non-http url")
	}
}

func TestPlugin_Execute_Success2xx(t *testing.T) {
	p := Plugin{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	t.Cleanup(ts.Close)

	cfg := mustJSON(map[string]any{
		"url":                 ts.URL,
		"expectStatus":        []int{200},
		"timeoutMs":           2000,
		"method":              "GET",
		"expectBodySubstring": "OK",
	})

	sigs, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    cfg,
		Payload: map[string]any{
			"monitor_id":   123,
			"monitor_name": "uptime-test",
		},
		Now: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sigs))
	}
	s := sigs[0]
	if s.SourceType != "http_check" {
		t.Fatalf("unexpected SourceType=%q", s.SourceType)
	}
	if s.Severity != "info" || s.Status != "resolved" {
		t.Fatalf("expected info/resolved, got severity=%q status=%q", s.Severity, s.Status)
	}
	if got := s.Fields["status_code"]; got != float64(200) && got != 200 {
		t.Fatalf("expected status_code 200, got %#v", got)
	}
}

func TestPlugin_Execute_Non2xxFailure(t *testing.T) {
	p := Plugin{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("maintenance"))
	}))
	t.Cleanup(ts.Close)

	cfg := mustJSON(map[string]any{"url": ts.URL})

	sigs, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    cfg,
		Payload:   map[string]any{},
		Now:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sigs))
	}
	s := sigs[0]
	if s.Severity != "error" || s.Status != "firing" {
		t.Fatalf("expected error/firing, got severity=%q status=%q", s.Severity, s.Status)
	}
}

// Note: timeout behavior depends on environment and scheduler; we don't
// assert a strict error here, only that Execute returns quickly enough
// when a very small timeoutMs is configured.
func TestPlugin_Execute_TimeoutConfig_DoesNotHang(t *testing.T) {
	p := Plugin{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	t.Cleanup(ts.Close)

	cfg := mustJSON(map[string]any{
		"url":       ts.URL,
		"timeoutMs": 10,
	})

	start := time.Now()
	_, _ = p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    cfg,
		Payload:   map[string]any{},
		Now:       time.Now().UTC(),
	})
	if time.Since(start) > time.Second {
		t.Fatalf("Execute took too long with small timeoutMs")
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}
