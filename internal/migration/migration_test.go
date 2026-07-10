package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestExportAndApplyProjectBundle(t *testing.T) {
	sourceDB := openMigrationTestDB(t, "source")
	targetDB := openMigrationTestDB(t, "target")
	ctx := context.Background()
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)

	source := model.Project{OwnerUserID: 11, Name: "local-prod"}
	if err := sourceDB.Create(&source).Error; err != nil {
		t.Fatalf("create source project: %v", err)
	}
	target := model.Project{OwnerUserID: 22, Name: "cloud-prod"}
	if err := targetDB.Create(&target).Error; err != nil {
		t.Fatalf("create target project: %v", err)
	}
	ingestID := uuid.New()
	if err := sourceDB.Create(&model.Log{
		ProjectID: source.ID,
		Timestamp: now,
		IngestID:  &ingestID,
		Level:     "info",
		Message:   "signup",
		Fields:    datatypes.JSON([]byte(`{"plan":"pro"}`)),
	}).Error; err != nil {
		t.Fatalf("create log: %v", err)
	}
	eventID := uuid.New()
	if err := sourceDB.Create(&model.Event{
		ID:        eventID,
		ProjectID: source.ID,
		Timestamp: now,
		Level:     "error",
		Title:     "panic",
		Data:      datatypes.JSON([]byte(`{"event_id":"` + eventID.String() + `"}`)),
	}).Error; err != nil {
		t.Fatalf("create event: %v", err)
	}
	if err := sourceDB.Create(&model.EventDefinition{ProjectID: source.ID, Name: "signup", Status: "active"}).Error; err != nil {
		t.Fatalf("create event definition: %v", err)
	}
	endpoint := model.AlertWebhookEndpoint{ProjectID: source.ID, Name: "ops", URL: "https://example.com/hook?token=secret"}
	if err := sourceDB.Create(&endpoint).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	targets, _ := json.Marshal(alert.RuleTargets{WebhookEndpointIDs: []int{endpoint.ID}})
	if err := sourceDB.Create(&model.AlertRule{
		ProjectID: source.ID,
		Name:      "errors",
		Enabled:   true,
		Source:    "logs",
		Match:     datatypes.JSON([]byte(`{}`)),
		Repeat:    datatypes.JSON([]byte(`{}`)),
		Targets:   datatypes.JSON(targets),
	}).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}

	bundle, err := ExportBundle(ctx, sourceDB, ExportOptions{
		OwnerUserID:                source.OwnerUserID,
		ProjectIDs:                 []int{source.ID},
		IncludeNotificationSecrets: true,
	})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if len(bundle.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(bundle.Projects))
	}
	if len(bundle.Projects[0].Logs) != 1 || len(bundle.Projects[0].Events) != 1 {
		t.Fatalf("unexpected exported counts: logs=%d events=%d", len(bundle.Projects[0].Logs), len(bundle.Projects[0].Events))
	}
	if got := bundle.Projects[0].Config.AlertWebhookEndpoints[0].URL; got != endpoint.URL {
		t.Fatalf("expected webhook secret url to export, got %q", got)
	}

	result, err := ApplyProjectBundle(ctx, targetDB, target.ID, bundle.Projects[0])
	if err != nil {
		t.Fatalf("ApplyProjectBundle: %v", err)
	}
	if result.ProjectID != target.ID {
		t.Fatalf("unexpected target project id: %d", result.ProjectID)
	}

	var logs int64
	if err := targetDB.Model(&model.Log{}).Where("project_id = ?", target.ID).Count(&logs).Error; err != nil {
		t.Fatalf("count logs: %v", err)
	}
	var events int64
	if err := targetDB.Model(&model.Event{}).Where("project_id = ?", target.ID).Count(&events).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if logs != 1 || events != 1 {
		t.Fatalf("expected imported logs/events, got logs=%d events=%d", logs, events)
	}
	var importedEndpoint model.AlertWebhookEndpoint
	if err := targetDB.Where("project_id = ?", target.ID).First(&importedEndpoint).Error; err != nil {
		t.Fatalf("find imported endpoint: %v", err)
	}
	if importedEndpoint.URL != endpoint.URL {
		t.Fatalf("expected webhook URL to migrate, got %q", importedEndpoint.URL)
	}
	var importedRule model.AlertRule
	if err := targetDB.Where("project_id = ?", target.ID).First(&importedRule).Error; err != nil {
		t.Fatalf("find imported rule: %v", err)
	}
	if !importedRule.Enabled {
		t.Fatalf("expected rule to remain enabled after target remap")
	}
	var importedTargets alert.RuleTargets
	if err := json.Unmarshal(importedRule.Targets, &importedTargets); err != nil {
		t.Fatalf("decode imported targets: %v", err)
	}
	if len(importedTargets.WebhookEndpointIDs) != 1 || importedTargets.WebhookEndpointIDs[0] != importedEndpoint.ID {
		t.Fatalf("targets were not remapped: %+v endpoint=%d", importedTargets, importedEndpoint.ID)
	}
}

func TestExportBundleWithoutOwnerExportsAllNonSystemProjects(t *testing.T) {
	db := openMigrationTestDB(t, "no-owner")
	ctx := context.Background()

	p1 := model.Project{OwnerUserID: 11, Name: "owner-a"}
	p2 := model.Project{OwnerUserID: 22, Name: "owner-b"}
	system := model.Project{OwnerUserID: 11, Name: "system", IsSystem: true}
	if err := db.Create(&[]model.Project{p1, p2, system}).Error; err != nil {
		t.Fatalf("create projects: %v", err)
	}

	bundle, err := ExportBundle(ctx, db, ExportOptions{IncludeNotificationSecrets: true})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if len(bundle.Projects) != 2 {
		t.Fatalf("expected 2 non-system projects, got %d", len(bundle.Projects))
	}
	got := map[string]bool{}
	for _, p := range bundle.Projects {
		got[p.Name] = true
		if p.Name == "system" {
			t.Fatalf("system project should not be exported")
		}
	}
	if !got["owner-a"] || !got["owner-b"] {
		t.Fatalf("unexpected projects exported: %#v", got)
	}
}

func openMigrationTestDB(t *testing.T, suffix string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.QueryEscape(t.Name()+"/"+suffix))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.ProjectKey{},
		&model.Event{},
		&model.Log{},
		&model.TrackEvent{},
		&model.TrackEventDaily{},
		&model.UserFirstSeen{},
		&model.ProjectCounter{},
		&model.LogDailyStat{},
		&model.CleanupPolicy{},
		&model.EventDefinition{},
		&model.PropertyDefinition{},
		&model.AnalysisView{},
		&model.PluginPackageSetting{},
		&model.AlertContact{},
		&model.AlertContactGroup{},
		&model.AlertContactGroupMember{},
		&model.AlertWecomBot{},
		&model.AlertWebhookEndpoint{},
		&model.AlertRule{},
		&model.AlertState{},
		&model.AlertDelivery{},
		&model.MonitorDefinition{},
		&model.MonitorRun{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}
