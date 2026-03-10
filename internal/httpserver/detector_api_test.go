package httpserver_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/testkit"
)

func TestDetectorCatalogAndSchemaAPI(t *testing.T) {
	t.Parallel()

	s := testkit.NewServer(t)
	baseURL := s.HTTP.URL
	client := s.HTTP.Client()

	boot := testkit.Bootstrap(t, client, baseURL)
	headers := map[string]string{"Authorization": "Bearer " + boot.Token}

	status, body := testkit.DoJSON(t, client, http.MethodGet, baseURL+"/api/plugins/detectors", nil, headers)
	if status != http.StatusOK {
		t.Fatalf("list detectors status=%d body=%s", status, string(body))
	}
	env := testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("list detectors code=%d err=%s", env.Code, env.Err)
	}
	var data struct {
		Items []struct {
			Type string `json:"type"`
			Mode string `json:"mode"`
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal detectors: %v", err)
	}
	if len(data.Items) == 0 {
		t.Fatalf("expected non-empty detector list")
	}
	found := false
	for _, item := range data.Items {
		if item.Type == "log_basic" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected log_basic in detector list, got %+v", data.Items)
	}

	status, body = testkit.DoJSON(t, client, http.MethodGet, baseURL+"/api/plugins/detectors/log_basic/schema", nil, headers)
	if status != http.StatusOK {
		t.Fatalf("schema status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("schema code=%d err=%s", env.Code, env.Err)
	}
	var schemaData struct {
		DetectorType string          `json:"detectorType"`
		Schema       json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(env.Data, &schemaData); err != nil {
		t.Fatalf("unmarshal schema data: %v", err)
	}
	if schemaData.DetectorType != "log_basic" {
		t.Fatalf("unexpected detectorType: %q", schemaData.DetectorType)
	}
	if len(schemaData.Schema) == 0 {
		t.Fatalf("expected non-empty schema")
	}
}

func TestMonitorCreateUpdateAndTestValidation(t *testing.T) {
	t.Parallel()

	s := testkit.NewServer(t)
	baseURL := s.HTTP.URL
	client := s.HTTP.Client()

	boot := testkit.Bootstrap(t, client, baseURL)
	headers := map[string]string{"Authorization": "Bearer " + boot.Token}
	monitorBase := fmt.Sprintf("%s/api/%d/monitors", baseURL, boot.ProjectID)

	// Invalid detector type should fail at create time.
	status, body := testkit.DoJSON(t, client, http.MethodPost, monitorBase, map[string]any{
		"name":         "invalid",
		"detectorType": "missing_detector",
		"config":       map[string]any{},
	}, headers)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid detectorType, got %d body=%s", status, string(body))
	}

	// Valid monitor create.
	status, body = testkit.DoJSON(t, client, http.MethodPost, monitorBase, map[string]any{
		"name":         "log-basic-check",
		"detectorType": "log_basic",
		"config":       map[string]any{},
		"intervalSec":  30,
		"timeoutMs":    1000,
		"enabled":      true,
	}, headers)
	if status != http.StatusOK {
		t.Fatalf("create monitor status=%d body=%s", status, string(body))
	}
	env := testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create monitor code=%d err=%s", env.Code, env.Err)
	}
	var monitorRow struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &monitorRow); err != nil {
		t.Fatalf("unmarshal monitor: %v", err)
	}
	if monitorRow.ID <= 0 {
		t.Fatalf("expected monitor id > 0")
	}

	// Invalid config on update should fail (log_basic expects object).
	updateURL := fmt.Sprintf("%s/%d", monitorBase, monitorRow.ID)
	status, body = testkit.DoJSON(t, client, http.MethodPut, updateURL, map[string]any{
		"config": []any{"bad"},
	}, headers)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid config, got %d body=%s", status, string(body))
	}

	// test endpoint should run detector and not enqueue alerts.
	testURL := fmt.Sprintf("%s/%d/test", monitorBase, monitorRow.ID)
	status, body = testkit.DoJSON(t, client, http.MethodPost, testURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("monitor test status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("monitor test code=%d err=%s", env.Code, env.Err)
	}
	var testData struct {
		SignalCount int `json:"signalCount"`
	}
	if err := json.Unmarshal(env.Data, &testData); err != nil {
		t.Fatalf("unmarshal test data: %v", err)
	}
	if testData.SignalCount != 1 {
		t.Fatalf("expected signalCount=1, got %d", testData.SignalCount)
	}

	var deliveryCount int64
	if err := s.DB.Model(&model.AlertDelivery{}).Count(&deliveryCount).Error; err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if deliveryCount != 0 {
		t.Fatalf("expected no alert deliveries from test endpoint, got %d", deliveryCount)
	}
}
