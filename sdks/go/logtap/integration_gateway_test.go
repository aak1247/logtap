package logtap

import (
	"context"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/testkit"
)

func TestGoSDK_Integration_Gateway_SQLite(t *testing.T) {
	t.Parallel()

	srv := testkit.NewServer(t)
	client := srv.HTTP.Client()
	baseURL := srv.HTTP.URL

	boot := testkit.Bootstrap(t, client, baseURL)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sdk, err := NewClient(ClientOptions{
		BaseURL:       baseURL,
		ProjectID:     int64(boot.ProjectID),
		ProjectKey:    boot.ProjectKey,
		Gzip:          true,
		FlushInterval: -1,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = sdk.Close(context.Background()) })

	sdk.Info("go-sdk-e2e", map[string]any{"k": "v"}, nil)
	sdk.Track("go-sdk-signup", map[string]any{"plan": "pro"}, nil)

	if err := sdk.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	logs := testkit.SearchLogs(t, client, baseURL, boot.Token, boot.ProjectID, testkit.SearchLogsParams{Limit: 50})
	hasInfo := false
	hasEvent := false
	for _, row := range logs {
		msg, _ := row["message"].(string)
		level, _ := row["level"].(string)
		if msg == "go-sdk-e2e" && level == "info" {
			hasInfo = true
		}
		if msg == "go-sdk-signup" && level == "event" {
			hasEvent = true
		}
	}
	if !hasInfo {
		t.Fatalf("expected info log not found (logs=%v)", logs)
	}
	if !hasEvent {
		t.Fatalf("expected event log not found (logs=%v)", logs)
	}
}
