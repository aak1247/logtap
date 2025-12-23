package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestFromEnv_RequiresPostgresWhenConsumersEnabled(t *testing.T) {
	t.Setenv("RUN_CONSUMERS", "true")
	t.Setenv("POSTGRES_URL", "")
	t.Setenv("NSQD_ADDRESS", "127.0.0.1:4150")

	if _, err := FromEnv(); err == nil {
		t.Fatalf("expected error when RUN_CONSUMERS=true and POSTGRES_URL missing")
	}
}

func TestFromEnv_DefaultsAndToggles(t *testing.T) {
	t.Setenv("RUN_CONSUMERS", "false")
	t.Setenv("POSTGRES_URL", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("NSQD_ADDRESS", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("ENABLE_METRICS", "true")
	t.Setenv("AUTH_SECRET", "")
	t.Setenv("AUTH_TOKEN_TTL", "not-a-duration")
	t.Setenv("MAINTENANCE_MODE", "true")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("expected default HTTPAddr=:8080, got %q", cfg.HTTPAddr)
	}
	if strings.TrimSpace(cfg.NSQDAddress) == "" {
		t.Fatalf("expected NSQDAddress to be set")
	}
	if cfg.RunConsumers {
		t.Fatalf("expected RunConsumers=false")
	}
	if cfg.EnableMetrics {
		t.Fatalf("expected metrics disabled when REDIS_ADDR is empty")
	}
	if cfg.AuthTokenTTL != 168*time.Hour {
		t.Fatalf("expected default AuthTokenTTL, got %v", cfg.AuthTokenTTL)
	}
	if !cfg.MaintenanceMode {
		t.Fatalf("expected MaintenanceMode=true")
	}
}

func TestFromEnv_AuthSecretValidation(t *testing.T) {
	t.Setenv("RUN_CONSUMERS", "false")
	t.Setenv("NSQD_ADDRESS", "127.0.0.1:4150")

	t.Setenv("AUTH_SECRET", "%%%")
	if _, err := FromEnv(); err == nil {
		t.Fatalf("expected error for invalid base64 AUTH_SECRET")
	}

	short := make([]byte, 31)
	t.Setenv("AUTH_SECRET", base64.RawStdEncoding.EncodeToString(short))
	if _, err := FromEnv(); err == nil {
		t.Fatalf("expected error for short AUTH_SECRET")
	}

	ok32 := make([]byte, 32)
	t.Setenv("AUTH_SECRET", base64.StdEncoding.EncodeToString(ok32)) // padded form
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if len(cfg.AuthSecret) != 32 {
		t.Fatalf("expected decoded secret len=32, got %d", len(cfg.AuthSecret))
	}
}

func TestHelpers_ParseAndRedact(t *testing.T) {
	if parseBoolDefault("not-bool", true) != true {
		t.Fatalf("expected parseBoolDefault fallback")
	}
	if parseIntDefault("not-int", 7) != 7 {
		t.Fatalf("expected parseIntDefault fallback")
	}
	if parseDurationDefault("-1s", 3*time.Second) != 3*time.Second {
		t.Fatalf("expected parseDurationDefault fallback for non-positive")
	}
	if parseDurationDefault("bad", 3*time.Second) != 3*time.Second {
		t.Fatalf("expected parseDurationDefault fallback for invalid")
	}

	if _, err := decodeBase64Any(" "); err == nil {
		t.Fatalf("expected decodeBase64Any to error for empty")
	}

	raw := []byte("hello")
	for _, s := range []string{
		base64.RawStdEncoding.EncodeToString(raw),
		base64.StdEncoding.EncodeToString(raw),
		base64.RawURLEncoding.EncodeToString(raw),
		base64.URLEncoding.EncodeToString(raw),
	} {
		got, err := decodeBase64Any(s)
		if err != nil || string(got) != "hello" {
			t.Fatalf("decodeBase64Any(%q)=%q err=%v", s, string(got), err)
		}
	}

	if got := redactPostgresURL(""); got != "<none>" {
		t.Fatalf("expected <none>, got %q", got)
	}
	if got := redactPostgresURL("http://bad url"); got != "<set>" {
		t.Fatalf("expected <set> for invalid url, got %q", got)
	}
	if got := redactPostgresURL("postgres://u:p@host:5432/db?sslmode=disable"); got != "u@host:5432/db" {
		t.Fatalf("unexpected redaction: %q", got)
	}
}

