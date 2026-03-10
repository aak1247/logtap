package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openAlertTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite): %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("gdb.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := gdb.AutoMigrate(
		&model.AlertContact{},
		&model.AlertContactGroup{},
		&model.AlertContactGroupMember{},
		&model.AlertWecomBot{},
		&model.AlertWebhookEndpoint{},
		&model.AlertRule{},
		&model.AlertState{},
		&model.AlertDelivery{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return gdb
}

func TestEngine_ThresholdAndBackoff(t *testing.T) {
	t.Parallel()

	db := openAlertTestDB(t)

	ep := model.AlertWebhookEndpoint{ProjectID: 1, Name: "hook", URL: "http://example.invalid/hook"}
	if err := db.Create(&ep).Error; err != nil {
		t.Fatalf("create webhook endpoint: %v", err)
	}

	match, _ := json.Marshal(RuleMatch{Levels: []string{"error"}, MessageKeywords: []string{"boom"}})
	repeat, _ := json.Marshal(RuleRepeat{WindowSec: 60, Threshold: 2, BaseBackoffSec: 60, MaxBackoffSec: 60})
	targets, _ := json.Marshal(RuleTargets{WebhookEndpointIDs: []int{ep.ID}})

	rule := model.AlertRule{
		ProjectID: 1,
		Name:      "Boom",
		Enabled:   true,
		Source:    string(SourceLogs),
		Match:     datatypes.JSON(match),
		Repeat:    datatypes.JSON(repeat),
		Targets:   datatypes.JSON(targets),
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(db)
	e.Now = func() time.Time { return now }

	in := Input{ProjectID: 1, Source: SourceLogs, Timestamp: now, Level: "error", Message: "boom!", Fields: map[string]any{}}
	if err := e.Evaluate(context.Background(), in); err != nil {
		t.Fatalf("Evaluate #1: %v", err)
	}
	var deliveries []model.AlertDelivery
	if err := db.Find(&deliveries).Error; err != nil {
		t.Fatalf("find deliveries: %v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("expected 0 deliveries, got %d", len(deliveries))
	}

	if err := e.Evaluate(context.Background(), in); err != nil {
		t.Fatalf("Evaluate #2: %v", err)
	}
	deliveries = nil
	if err := db.Find(&deliveries).Error; err != nil {
		t.Fatalf("find deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	if err := e.Evaluate(context.Background(), in); err != nil {
		t.Fatalf("Evaluate #3: %v", err)
	}
	var count int64
	if err := db.Model(&model.AlertDelivery{}).Count(&count).Error; err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected still 1 delivery due to backoff, got %d", count)
	}
}

func TestEngine_WindowExpiryResetsBackoff(t *testing.T) {
	t.Parallel()

	db := openAlertTestDB(t)

	ep := model.AlertWebhookEndpoint{ProjectID: 1, Name: "hook", URL: "http://example.invalid/hook"}
	if err := db.Create(&ep).Error; err != nil {
		t.Fatalf("create webhook endpoint: %v", err)
	}

	match, _ := json.Marshal(RuleMatch{Levels: []string{"error"}, MessageKeywords: []string{"boom"}})
	dedupeByMessage := true
	repeat, _ := json.Marshal(RuleRepeat{
		WindowSec:       60,
		Threshold:       1,
		BaseBackoffSec:  3600,
		MaxBackoffSec:   3600,
		DedupeByMessage: &dedupeByMessage,
	})
	targets, _ := json.Marshal(RuleTargets{WebhookEndpointIDs: []int{ep.ID}})

	rule := model.AlertRule{
		ProjectID: 1,
		Name:      "Boom",
		Enabled:   true,
		Source:    string(SourceLogs),
		Match:     datatypes.JSON(match),
		Repeat:    datatypes.JSON(repeat),
		Targets:   datatypes.JSON(targets),
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(db)
	e.Now = func() time.Time { return now }

	in := Input{ProjectID: 1, Source: SourceLogs, Timestamp: now, Level: "error", Message: "boom!", Fields: map[string]any{}}
	if err := e.Evaluate(context.Background(), in); err != nil {
		t.Fatalf("Evaluate #1: %v", err)
	}

	now = now.Add(61 * time.Second) // window expired but backoff would still be active unless reset.
	in.Timestamp = now
	if err := e.Evaluate(context.Background(), in); err != nil {
		t.Fatalf("Evaluate #2: %v", err)
	}

	var count int64
	if err := db.Model(&model.AlertDelivery{}).Count(&count).Error; err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 deliveries after window expiry, got %d", count)
	}
}

func TestEngine_EvaluateSignal_RoutesThroughExistingTargets(t *testing.T) {
	t.Parallel()

	db := openAlertTestDB(t)

	ep := model.AlertWebhookEndpoint{ProjectID: 1, Name: "hook", URL: "http://example.invalid/hook"}
	if err := db.Create(&ep).Error; err != nil {
		t.Fatalf("create webhook endpoint: %v", err)
	}

	match, _ := json.Marshal(RuleMatch{
		Levels: []string{"error"},
		FieldsAll: []FieldMatch{
			{Path: "source_type", Op: OpEquals, Value: "http_check"},
			{Path: "labels.env", Op: OpEquals, Value: "prod"},
		},
	})
	repeat, _ := json.Marshal(RuleRepeat{WindowSec: 60, Threshold: 1, BaseBackoffSec: 60, MaxBackoffSec: 60})
	targets, _ := json.Marshal(RuleTargets{WebhookEndpointIDs: []int{ep.ID}})

	rule := model.AlertRule{
		ProjectID: 1,
		Name:      "SyntheticCritical",
		Enabled:   true,
		Source:    string(SourceBoth),
		Match:     datatypes.JSON(match),
		Repeat:    datatypes.JSON(repeat),
		Targets:   datatypes.JSON(targets),
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}

	e := NewEngine(db)
	sig := detector.Signal{
		ProjectID:  1,
		Source:     "logs",
		SourceType: "http_check",
		Severity:   "error",
		Status:     "firing",
		Message:    "probe timeout",
		Labels: map[string]string{
			"env": "prod",
		},
		Fields: map[string]any{
			"latency_ms": 1200,
		},
		OccurredAt: time.Now().UTC(),
	}
	if err := e.EvaluateSignal(context.Background(), sig); err != nil {
		t.Fatalf("EvaluateSignal: %v", err)
	}

	var count int64
	if err := db.Model(&model.AlertDelivery{}).Count(&count).Error; err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 delivery, got %d", count)
	}
}
