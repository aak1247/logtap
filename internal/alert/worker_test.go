package alert

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/model"
)

func TestWorker_Webhook_Sent(t *testing.T) {
	t.Parallel()

	db := openAlertTestDB(t)

	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := model.AlertDelivery{
		ProjectID:     1,
		RuleID:        1,
		ChannelType:   "webhook",
		Target:        srv.URL,
		Title:         "t",
		Content:       "c",
		Status:        "pending",
		Attempts:      0,
		NextAttemptAt: now.Add(-time.Second),
	}
	if err := db.Create(&d).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	w := NewWorker(db, config.Config{})
	w.HTTPClient = srv.Client()
	w.Now = func() time.Time { return now }

	if n, err := w.ProcessOnce(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("ProcessOnce n=%d err=%v", n, err)
	}

	var cur model.AlertDelivery
	if err := db.First(&cur, d.ID).Error; err != nil {
		t.Fatalf("load delivery: %v", err)
	}
	if cur.Status != "sent" {
		t.Fatalf("expected sent, got %q (last_error=%q)", cur.Status, cur.LastError)
	}
	if got["title"] != "t" {
		t.Fatalf("unexpected webhook payload: %v", got)
	}
}

func TestWorker_Email_MissingConfig_Failed(t *testing.T) {
	t.Parallel()

	db := openAlertTestDB(t)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := model.AlertDelivery{
		ProjectID:     1,
		RuleID:        1,
		ChannelType:   "email",
		Target:        "a@b.com",
		Title:         "t",
		Content:       "c",
		Status:        "pending",
		Attempts:      0,
		NextAttemptAt: now.Add(-time.Second),
	}
	if err := db.Create(&d).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	w := NewWorker(db, config.Config{})
	w.Now = func() time.Time { return now }

	if n, err := w.ProcessOnce(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("ProcessOnce n=%d err=%v", n, err)
	}

	var cur model.AlertDelivery
	if err := db.First(&cur, d.ID).Error; err != nil {
		t.Fatalf("load delivery: %v", err)
	}
	if cur.Status != "failed" {
		t.Fatalf("expected failed, got %q", cur.Status)
	}
}
