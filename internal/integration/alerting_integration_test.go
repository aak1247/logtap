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

func TestIntegration_Alerting_ContactsAndGroups_NoRuleIDPath(t *testing.T) {
	t.Parallel()

	srv := testkit.NewServer(t)
	client := srv.HTTP.Client()
	baseURL := srv.HTTP.URL
	boot := testkit.Bootstrap(t, client, baseURL)
	authz := map[string]string{"Authorization": "Bearer " + boot.Token}

	status, body := testkit.DoJSON(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/%d/alerts/contacts", baseURL, boot.ProjectID),
		nil,
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("list contacts status=%d body=%s", status, string(body))
	}
	env := testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("list contacts code=%d err=%s", env.Code, env.Err)
	}
	var contactsData struct {
		Items []model.AlertContact `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &contactsData); err != nil {
		t.Fatalf("decode contacts: %v", err)
	}
	if len(contactsData.Items) != 0 {
		t.Fatalf("expected 0 contacts initially, got %d", len(contactsData.Items))
	}

	status, body = testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/alerts/contacts", baseURL, boot.ProjectID),
		map[string]any{
			"type":  "email",
			"name":  "oncall",
			"value": "oncall@example.com",
		},
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("create contact status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create contact code=%d err=%s", env.Code, env.Err)
	}
	var contact model.AlertContact
	if err := json.Unmarshal(env.Data, &contact); err != nil {
		t.Fatalf("decode contact: %v", err)
	}
	if contact.ID <= 0 || contact.Type != "email" {
		t.Fatalf("unexpected contact: %+v", contact)
	}

	status, body = testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/alerts/contact-groups", baseURL, boot.ProjectID),
		map[string]any{
			"type":             "email",
			"name":             "OnCall Team",
			"memberContactIds": []int{contact.ID},
		},
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("create contact group status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create contact group code=%d err=%s", env.Code, env.Err)
	}
	var groupData struct {
		Group            model.AlertContactGroup `json:"group"`
		MemberContactIDs []int                   `json:"memberContactIds"`
	}
	if err := json.Unmarshal(env.Data, &groupData); err != nil {
		t.Fatalf("decode contact group: %v", err)
	}
	if groupData.Group.ID <= 0 {
		t.Fatalf("expected group id > 0, got %+v", groupData.Group)
	}
	if len(groupData.MemberContactIDs) != 1 || groupData.MemberContactIDs[0] != contact.ID {
		t.Fatalf("unexpected group members: %+v", groupData.MemberContactIDs)
	}
}

func TestIntegration_Alerting_RulesTest(t *testing.T) {
	t.Parallel()

	srv := testkit.NewServer(t)
	client := srv.HTTP.Client()
	baseURL := srv.HTTP.URL
	boot := testkit.Bootstrap(t, client, baseURL)

	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(hook.Close)

	authz := map[string]string{"Authorization": "Bearer " + boot.Token}

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

	status, body = testkit.DoJSON(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/%d/alerts/rules/test", baseURL, boot.ProjectID),
		map[string]any{
			"source":  "logs",
			"level":   "error",
			"message": "boom!",
			"fields":  map[string]any{"env": "prod"},
		},
		authz,
	)
	if status != http.StatusOK {
		t.Fatalf("rules test status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("rules test code=%d err=%s", env.Code, env.Err)
	}
	var data struct {
		Items []struct {
			RuleID      int `json:"ruleId"`
			Matched     bool
			WillEnqueue bool `json:"willEnqueue"`
			Deliveries  []struct {
				ChannelType string `json:"channelType"`
				Target      string `json:"target"`
			} `json:"deliveries"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode rules test: %v", err)
	}
	if len(data.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(data.Items))
	}
	if !data.Items[0].Matched || !data.Items[0].WillEnqueue {
		t.Fatalf("expected matched+willEnqueue, got %+v", data.Items[0])
	}
	if len(data.Items[0].Deliveries) != 1 || data.Items[0].Deliveries[0].ChannelType != "webhook" || data.Items[0].Deliveries[0].Target != hook.URL {
		t.Fatalf("unexpected deliveries: %+v", data.Items[0].Deliveries)
	}
	if data.Items[0].RuleID <= 0 {
		t.Fatalf("expected ruleId > 0, got %d", data.Items[0].RuleID)
	}
}
