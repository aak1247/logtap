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
	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/detector/plugins/logbasic"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/monitor"
	"github.com/aak1247/logtap/internal/testkit"
)

func TestIntegration_MonitorRunAndRunsAPI(t *testing.T) {
	t.Parallel()

	srv := testkit.NewServer(t)
	client := srv.HTTP.Client()
	baseURL := srv.HTTP.URL
	boot := testkit.Bootstrap(t, client, baseURL)
	authz := map[string]string{"Authorization": "Bearer " + boot.Token}

	status, body := testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/monitors", baseURL, boot.ProjectID),
		map[string]any{
			"name":         "api-health-monitor",
			"detectorType": "log_basic",
			"config":       map[string]any{},
			"intervalSec":  30,
			"timeoutMs":    2000,
			"enabled":      true,
		},
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("create monitor status=%d body=%s", status, string(body))
	}
	env := testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create monitor code=%d err=%s", env.Code, env.Err)
	}
	var created model.MonitorDefinition
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode monitor: %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("expected monitor id > 0, got %d", created.ID)
	}

	status, body = testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/monitors/%d/run", baseURL, boot.ProjectID, created.ID),
		nil,
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("run monitor status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("run monitor code=%d err=%s", env.Code, env.Err)
	}

	reg := detector.NewRegistry()
	if err := reg.RegisterStatic(logbasic.New()); err != nil {
		t.Fatalf("register log_basic: %v", err)
	}
	mw := monitor.NewWorker(srv.DB, reg)
	if n, err := mw.RunOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("monitor worker RunOnce n=%d err=%v", n, err)
	}

	status, body = testkit.DoJSON(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/%d/monitors/%d/runs?limit=10", baseURL, boot.ProjectID, created.ID),
		nil,
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("list monitor runs status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("list monitor runs code=%d err=%s", env.Code, env.Err)
	}
	var runsData struct {
		Items []model.MonitorRun `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &runsData); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runsData.Items) != 1 {
		t.Fatalf("expected 1 monitor run, got %d", len(runsData.Items))
	}
	if runsData.Items[0].MonitorID != created.ID {
		t.Fatalf("expected monitor_id=%d, got %d", created.ID, runsData.Items[0].MonitorID)
	}
	if runsData.Items[0].Status != "success" {
		t.Fatalf("expected run status success, got %q", runsData.Items[0].Status)
	}
	if runsData.Items[0].SignalCount != 1 {
		t.Fatalf("expected signal_count=1, got %d", runsData.Items[0].SignalCount)
	}
}

func TestE2E_MonitorSignalToAlertWebhookDelivery(t *testing.T) {
	t.Parallel()

	srv := testkit.NewServer(t)
	client := srv.HTTP.Client()
	baseURL := srv.HTTP.URL
	boot := testkit.Bootstrap(t, client, baseURL)
	authz := map[string]string{"Authorization": "Bearer " + boot.Token}

	webhookPayloadCh := make(chan map[string]any, 1)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		select {
		case webhookPayloadCh <- payload:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(hook.Close)

	status, body := testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/alerts/webhook-endpoints", baseURL, boot.ProjectID),
		map[string]any{
			"name": "monitor-hook",
			"url":  hook.URL,
		},
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("create webhook endpoint status=%d body=%s", status, string(body))
	}
	env := testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create webhook endpoint code=%d err=%s", env.Code, env.Err)
	}
	var endpoint model.AlertWebhookEndpoint
	if err := json.Unmarshal(env.Data, &endpoint); err != nil {
		t.Fatalf("decode endpoint: %v", err)
	}

	monitorName := "monitor-e2e-heartbeat"
	status, body = testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/alerts/rules", baseURL, boot.ProjectID),
		map[string]any{
			"name":    "monitor-heartbeat-rule",
			"enabled": true,
			"source":  "logs",
			"match": map[string]any{
				"levels":          []string{"info"},
				"messageKeywords": []string{monitorName},
			},
			"repeat": map[string]any{
				"windowSec":      60,
				"threshold":      1,
				"baseBackoffSec": 60,
				"maxBackoffSec":  60,
			},
			"targets": map[string]any{
				"webhookEndpointIds": []int{endpoint.ID},
			},
		},
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("create alert rule status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create alert rule code=%d err=%s", env.Code, env.Err)
	}
	var rule model.AlertRule
	if err := json.Unmarshal(env.Data, &rule); err != nil {
		t.Fatalf("decode alert rule: %v", err)
	}

	status, body = testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/monitors", baseURL, boot.ProjectID),
		map[string]any{
			"name":         monitorName,
			"detectorType": "log_basic",
			"config":       map[string]any{},
			"intervalSec":  60,
			"timeoutMs":    2000,
			"enabled":      true,
		},
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("create monitor status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create monitor code=%d err=%s", env.Code, env.Err)
	}
	var monitorRow model.MonitorDefinition
	if err := json.Unmarshal(env.Data, &monitorRow); err != nil {
		t.Fatalf("decode monitor row: %v", err)
	}

	status, body = testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/monitors/%d/run", baseURL, boot.ProjectID, monitorRow.ID),
		nil,
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("run monitor status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("run monitor code=%d err=%s", env.Code, env.Err)
	}

	reg := detector.NewRegistry()
	if err := reg.RegisterStatic(logbasic.New()); err != nil {
		t.Fatalf("register log_basic: %v", err)
	}
	mw := monitor.NewWorker(srv.DB, reg)
	if n, err := mw.RunOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("monitor worker RunOnce n=%d err=%v", n, err)
	}

	var pending []model.AlertDelivery
	if err := srv.DB.Where("project_id = ? AND status = ?", boot.ProjectID, "pending").Find(&pending).Error; err != nil {
		t.Fatalf("query pending deliveries: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending delivery, got %d", len(pending))
	}
	if pending[0].RuleID != rule.ID {
		t.Fatalf("expected rule_id=%d, got %d", rule.ID, pending[0].RuleID)
	}

	aw := alert.NewWorker(srv.DB, srv.Config)
	aw.HTTPClient = hook.Client()
	aw.Now = func() time.Time { return time.Now().UTC() }
	if n, err := aw.ProcessOnce(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("alert worker ProcessOnce n=%d err=%v", n, err)
	}

	var sent model.AlertDelivery
	if err := srv.DB.First(&sent, pending[0].ID).Error; err != nil {
		t.Fatalf("load sent delivery: %v", err)
	}
	if sent.Status != "sent" {
		t.Fatalf("expected delivery status sent, got %q (last_error=%q)", sent.Status, sent.LastError)
	}

	select {
	case payload := <-webhookPayloadCh:
		if payload["projectId"] != float64(boot.ProjectID) {
			t.Fatalf("unexpected webhook projectId payload: %v", payload)
		}
		if payload["ruleId"] != float64(rule.ID) {
			t.Fatalf("unexpected webhook ruleId payload: %v", payload)
		}
		if payload["deliveryId"] != float64(sent.ID) {
			t.Fatalf("unexpected webhook deliveryId payload: %v", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for webhook callback")
	}
}
