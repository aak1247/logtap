package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/testkit"
)

func TestIntegration_Alerting_Webhook_Delivery(t *testing.T) {
	t.Parallel()

	srv := testkit.NewServer(t)
	client := srv.HTTP.Client()
	baseURL := srv.HTTP.URL
	boot := testkit.Bootstrap(t, client, baseURL)

	var got map[string]any
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(hook.Close)

	authz := map[string]string{"Authorization": "Bearer " + boot.Token}

	// Create webhook endpoint
	status, body := testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/alerts/webhook-endpoints", baseURL, boot.ProjectID),
		map[string]any{"name": "ops", "url": hook.URL},
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("create webhook endpoint status=%d body=%s", status, string(body))
	}
	env := testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create webhook endpoint code=%d err=%s", env.Code, env.Err)
	}
	var ep model.AlertWebhookEndpoint
	if err := json.Unmarshal(env.Data, &ep); err != nil {
		t.Fatalf("decode endpoint: %v", err)
	}

	// Create rule targeting webhook endpoint
	match := alert.RuleMatch{Levels: []string{"error"}, MessageKeywords: []string{"boom"}}
	dedupeByMessage := true
	repeat := alert.RuleRepeat{WindowSec: 60, Threshold: 1, BaseBackoffSec: 60, MaxBackoffSec: 60, DedupeByMessage: &dedupeByMessage}
	targets := alert.RuleTargets{WebhookEndpointIDs: []int{ep.ID}}
	status, body = testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/alerts/rules", baseURL, boot.ProjectID),
		map[string]any{
			"name":    "BoomRule",
			"enabled": true,
			"source":  "logs",
			"match":   match,
			"repeat":  repeat,
			"targets": targets,
		},
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("create rule status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create rule code=%d err=%s", env.Code, env.Err)
	}

	// Ingest an error log that triggers the rule
	ingestHeaders := map[string]string{"X-Project-Key": boot.ProjectKey}
	status, body = testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/logs/", baseURL, boot.ProjectID),
		map[string]any{"level": "error", "message": "boom!"},
		ingestHeaders,
	)
	if status != http.StatusAccepted {
		t.Fatalf("ingest status=%d body=%s", status, string(body))
	}

	var pending []model.AlertDelivery
	if err := srv.DB.Where("project_id = ? AND status = ?", boot.ProjectID, "pending").Find(&pending).Error; err != nil {
		t.Fatalf("query deliveries: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending delivery, got %d", len(pending))
	}

	worker := alert.NewWorker(srv.DB, srv.Config)
	worker.HTTPClient = hook.Client()
	worker.Now = func() time.Time { return time.Now().UTC() }

	if n, err := worker.ProcessOnce(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("ProcessOnce n=%d err=%v", n, err)
	}

	var cur model.AlertDelivery
	if err := srv.DB.First(&cur, pending[0].ID).Error; err != nil {
		t.Fatalf("load delivery: %v", err)
	}
	if cur.Status != "sent" {
		t.Fatalf("expected sent, got %q (last_error=%q)", cur.Status, cur.LastError)
	}
	if got["projectId"] != float64(boot.ProjectID) {
		t.Fatalf("unexpected webhook payload: %v", got)
	}
}
