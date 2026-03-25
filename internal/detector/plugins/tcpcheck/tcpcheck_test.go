package tcpcheck

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

func TestPlugin_ValidateConfig_Basic(t *testing.T) {
	p := Plugin{}

	if err := p.ValidateConfig(nil); err == nil {
		t.Fatalf("expected error for empty config")
	}

	cfg := mustJSON(map[string]any{"host": "127.0.0.1", "port": 80})
	if err := p.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig valid: %v", err)
	}

	badPort := mustJSON(map[string]any{"host": "127.0.0.1", "port": 70000})
	if err := p.ValidateConfig(badPort); err == nil {
		t.Fatalf("expected error for invalid port")
	}
}

func TestPlugin_Execute_Success(t *testing.T) {
	p := Plugin{}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	cfg := mustJSON(map[string]any{
		"host":      host,
		"port":      atoi(portStr),
		"timeoutMs": 1000,
	})

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
	if s.SourceType != "tcp_check" {
		t.Fatalf("unexpected SourceType=%q", s.SourceType)
	}
	if s.Severity != "info" || s.Status != "resolved" {
		t.Fatalf("expected info/resolved, got severity=%q status=%q", s.Severity, s.Status)
	}
}

func TestPlugin_Execute_Failure(t *testing.T) {
	p := Plugin{}

	cfg := mustJSON(map[string]any{
		"host":      "127.0.0.1",
		"port":      65000,
		"timeoutMs": 200,
	})

	sigs, err := p.Execute(context.Background(), detector.ExecuteRequest{
		ProjectID: 1,
		Config:    cfg,
		Payload:   map[string]any{},
		Now:       time.Now().UTC(),
	})
	if err != nil {
		// We accept either an error or an error signal; just ensure it does not panic or hang.
		return
	}
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sigs))
	}
	s := sigs[0]
	if s.Severity != "error" || s.Status != "firing" {
		t.Fatalf("expected error/firing, got severity=%q status=%q", s.Severity, s.Status)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
