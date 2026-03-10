package monitor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/testkit"
	"gorm.io/datatypes"
)

type stubDetector struct {
	typ     string
	signals []detector.Signal
	err     error
}

func (s stubDetector) Type() string { return s.typ }

func (s stubDetector) ConfigSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }

func (s stubDetector) ValidateConfig(_ json.RawMessage) error { return nil }

func (s stubDetector) Execute(_ context.Context, _ detector.ExecuteRequest) ([]detector.Signal, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]detector.Signal, 0, len(s.signals))
	out = append(out, s.signals...)
	return out, nil
}

func TestWorker_RunOnce_SendsAlertFromSignal(t *testing.T) {
	t.Parallel()

	db := testkit.OpenTestDB(t)
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	ep := model.AlertWebhookEndpoint{ProjectID: 1, Name: "hook", URL: "http://example.invalid/hook"}
	if err := db.Create(&ep).Error; err != nil {
		t.Fatalf("create webhook endpoint: %v", err)
	}
	match, _ := json.Marshal(alert.RuleMatch{
		Levels: []string{"error"},
		FieldsAll: []alert.FieldMatch{
			{Path: "source_type", Op: alert.OpEquals, Value: "http_check"},
		},
	})
	repeat, _ := json.Marshal(alert.RuleRepeat{WindowSec: 60, Threshold: 1, BaseBackoffSec: 60, MaxBackoffSec: 60})
	targets, _ := json.Marshal(alert.RuleTargets{WebhookEndpointIDs: []int{ep.ID}})
	rule := model.AlertRule{
		ProjectID: 1,
		Name:      "HTTP monitor",
		Enabled:   true,
		Source:    string(alert.SourceLogs),
		Match:     datatypes.JSON(match),
		Repeat:    datatypes.JSON(repeat),
		Targets:   datatypes.JSON(targets),
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}
	monitorRow := model.MonitorDefinition{
		ProjectID:    1,
		Name:         "healthz",
		DetectorType: "http_check",
		Config:       datatypes.JSON([]byte(`{"url":"http://127.0.0.1/healthz"}`)),
		IntervalSec:  60,
		TimeoutMS:    3000,
		Enabled:      true,
		NextRunAt:    now.Add(-time.Second),
	}
	if err := db.Create(&monitorRow).Error; err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	reg := detector.NewRegistry()
	if err := reg.RegisterStatic(stubDetector{
		typ: "http_check",
		signals: []detector.Signal{
			{
				Source:     "logs",
				SourceType: "http_check",
				Severity:   "error",
				Status:     "firing",
				Message:    "healthz timeout",
				Fields: map[string]any{
					"endpoint": "/healthz",
				},
			},
		},
	}); err != nil {
		t.Fatalf("register detector: %v", err)
	}

	w := NewWorker(db, reg)
	w.Now = func() time.Time { return now }
	w.BatchSize = 10
	w.LeaseDuration = 30 * time.Second
	processed, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected processed=1, got %d", processed)
	}

	var runs []model.MonitorRun
	if err := db.Order("id ASC").Find(&runs).Error; err != nil {
		t.Fatalf("find runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Status != "success" {
		t.Fatalf("expected success status, got %q", runs[0].Status)
	}
	if runs[0].SignalCount != 1 {
		t.Fatalf("expected signal_count=1, got %d", runs[0].SignalCount)
	}

	var deliveryCount int64
	if err := db.Model(&model.AlertDelivery{}).Count(&deliveryCount).Error; err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if deliveryCount != 1 {
		t.Fatalf("expected 1 delivery, got %d", deliveryCount)
	}

	var updated model.MonitorDefinition
	if err := db.Where("id = ?", monitorRow.ID).First(&updated).Error; err != nil {
		t.Fatalf("load updated monitor: %v", err)
	}
	if updated.LeaseUntil != nil {
		t.Fatalf("expected lease to be cleared")
	}
	if !updated.NextRunAt.Equal(now.Add(60 * time.Second)) {
		t.Fatalf("expected next_run_at=%s, got %s", now.Add(60*time.Second), updated.NextRunAt)
	}
}

func TestWorker_RunOnce_MissingDetectorRecordsFailedRun(t *testing.T) {
	t.Parallel()

	db := testkit.OpenTestDB(t)
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	row := model.MonitorDefinition{
		ProjectID:    2,
		Name:         "unknown",
		DetectorType: "missing_detector",
		Config:       datatypes.JSON([]byte(`{}`)),
		IntervalSec:  30,
		TimeoutMS:    1000,
		Enabled:      true,
		NextRunAt:    now.Add(-time.Second),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	reg := detector.NewRegistry()
	w := NewWorker(db, reg)
	w.Now = func() time.Time { return now }
	processed, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected processed=1, got %d", processed)
	}

	var run model.MonitorRun
	if err := db.Where("monitor_id = ?", row.ID).First(&run).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != "failed" {
		t.Fatalf("expected failed status, got %q", run.Status)
	}

	var deliveryCount int64
	if err := db.Model(&model.AlertDelivery{}).Count(&deliveryCount).Error; err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if deliveryCount != 0 {
		t.Fatalf("expected 0 deliveries, got %d", deliveryCount)
	}
}
