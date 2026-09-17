package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/store"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApplyResult struct {
	ProjectID int              `json:"project_id"`
	Inserted  map[string]int64 `json:"inserted"`
	Skipped   map[string]int64 `json:"skipped"`
	Warnings  []string         `json:"warnings,omitempty"`
}

func ApplyProjectBundle(ctx context.Context, db *gorm.DB, targetProjectID int, pb ProjectBundle) (ApplyResult, error) {
	if db == nil {
		return ApplyResult{}, errors.New("database not configured")
	}
	if targetProjectID <= 0 {
		return ApplyResult{}, errors.New("target project id required")
	}
	res := ApplyResult{
		ProjectID: targetProjectID,
		Inserted:  map[string]int64{},
		Skipped:   map[string]int64{},
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&model.Project{}).Where("id = ?", targetProjectID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := applyProjectConfig(ctx, tx, targetProjectID, pb.Config, &res); err != nil {
			return err
		}
		if err := applyEvents(ctx, tx, targetProjectID, pb.Events, &res); err != nil {
			return err
		}
		return applyLogs(ctx, tx, pb.SourceProjectID, targetProjectID, pb.Logs, &res)
	})
	return res, err
}

func applyProjectConfig(ctx context.Context, tx *gorm.DB, projectID int, cfg ProjectConfig, res *ApplyResult) error {
	if cfg.CleanupPolicy != nil {
		row := model.CleanupPolicy{
			ProjectID:                projectID,
			Enabled:                  cfg.CleanupPolicy.Enabled,
			LogsRetentionDays:        cfg.CleanupPolicy.LogsRetentionDays,
			EventsRetentionDays:      cfg.CleanupPolicy.EventsRetentionDays,
			TrackEventsRetentionDays: cfg.CleanupPolicy.TrackEventsRetentionDays,
			ScheduleHourUTC:          cfg.CleanupPolicy.ScheduleHourUTC,
			ScheduleMinuteUTC:        cfg.CleanupPolicy.ScheduleMinuteUTC,
			LastRunAt:                cfg.CleanupPolicy.LastRunAt,
			NextRunAt:                cfg.CleanupPolicy.NextRunAt,
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "project_id"}},
			UpdateAll: true,
		}).Create(&row).Error; err != nil {
			return err
		}
		res.Inserted["cleanup_policies"] = 1
	}

	for _, r := range cfg.EventDefinitions {
		row := model.EventDefinition{
			ProjectID: projectID, Name: r.Name, DisplayName: r.DisplayName, Category: r.Category,
			Description: r.Description, Status: r.Status, Owner: r.Owner,
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "project_id"}, {Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"display_name", "category", "description", "status", "owner", "updated_at",
			}),
		}).Create(&row).Error; err != nil {
			return err
		}
		res.Inserted["event_definitions"]++
	}

	for _, r := range cfg.PropertyDefinitions {
		row := model.PropertyDefinition{
			ProjectID: projectID, Key: r.Key, DisplayName: r.DisplayName, Type: r.Type,
			Description: r.Description, Status: r.Status,
			EnumValues: datatypes.JSON(rawDefault(r.EnumValues, "null")), ExampleValues: datatypes.JSON(rawDefault(r.ExampleValues, "null")),
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "project_id"}, {Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"display_name", "type", "description", "status", "enum_values", "example_values", "updated_at",
			}),
		}).Create(&row).Error; err != nil {
			return err
		}
		res.Inserted["property_definitions"]++
	}

	for _, r := range cfg.AnalysisViews {
		row := model.AnalysisView{
			ProjectID: projectID, Name: r.Name, Description: r.Description, AnalysisType: r.AnalysisType,
			Query: datatypes.JSON(rawDefault(r.Query, "{}")),
		}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		res.Inserted["analysis_views"]++
	}

	for _, r := range cfg.PluginPackageSettings {
		row := model.PluginPackageSetting{
			ProjectID: projectID, PackageID: r.PackageID, Enabled: r.Enabled,
			Config: datatypes.JSON(rawDefault(r.Config, "{}")),
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "project_id"}, {Name: "package_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"enabled", "config", "updated_at"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		res.Inserted["plugin_package_settings"]++
	}

	if err := applyAlertConfig(ctx, tx, projectID, cfg, res); err != nil {
		return err
	}
	return applyMonitorConfig(ctx, tx, projectID, cfg, res)
}

func applyAlertConfig(ctx context.Context, tx *gorm.DB, projectID int, cfg ProjectConfig, res *ApplyResult) error {
	contactIDMap := map[int]int{}
	for _, r := range cfg.AlertContacts {
		row := model.AlertContact{ProjectID: projectID, Type: r.Type, Name: r.Name, Value: r.Value}
		if err := firstOrCreateAlertContact(ctx, tx, &row); err != nil {
			return err
		}
		contactIDMap[r.ID] = row.ID
		res.Inserted["alert_contacts"]++
	}

	groupIDMap := map[int]int{}
	for _, r := range cfg.AlertContactGroups {
		row := model.AlertContactGroup{ProjectID: projectID, Type: r.Type, Name: r.Name}
		if err := firstOrCreateAlertContactGroup(ctx, tx, &row); err != nil {
			return err
		}
		groupIDMap[r.ID] = row.ID
		res.Inserted["alert_contact_groups"]++
	}

	for _, r := range cfg.AlertContactGroupMembers {
		newGroupID := groupIDMap[r.GroupID]
		newContactID := contactIDMap[r.ContactID]
		if newGroupID <= 0 || newContactID <= 0 {
			res.Skipped["alert_contact_group_members"]++
			continue
		}
		row := model.AlertContactGroupMember{GroupID: newGroupID, ContactID: newContactID}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		res.Inserted["alert_contact_group_members"]++
	}

	wecomIDMap := map[int]int{}
	for _, r := range cfg.AlertWecomBots {
		if strings.TrimSpace(r.WebhookURL) == "" {
			res.Skipped["alert_wecom_bots"]++
			continue
		}
		row := model.AlertWecomBot{ProjectID: projectID, Name: r.Name, WebhookURL: r.WebhookURL}
		if err := firstOrCreateWecomBot(ctx, tx, &row); err != nil {
			return err
		}
		wecomIDMap[r.ID] = row.ID
		res.Inserted["alert_wecom_bots"]++
	}

	webhookIDMap := map[int]int{}
	for _, r := range cfg.AlertWebhookEndpoints {
		if strings.TrimSpace(r.URL) == "" {
			res.Skipped["alert_webhook_endpoints"]++
			continue
		}
		row := model.AlertWebhookEndpoint{ProjectID: projectID, Name: r.Name, URL: r.URL}
		if err := firstOrCreateWebhookEndpoint(ctx, tx, &row); err != nil {
			return err
		}
		webhookIDMap[r.ID] = row.ID
		res.Inserted["alert_webhook_endpoints"]++
	}

	for _, r := range cfg.AlertRules {
		targets, ok := rewriteRuleTargets(r.Targets, contactIDMap, groupIDMap, wecomIDMap, webhookIDMap)
		enabled := r.Enabled
		if !ok {
			enabled = false
			res.Warnings = append(res.Warnings, fmt.Sprintf("alert rule %q disabled because some targets could not be remapped", r.Name))
		}
		row := model.AlertRule{
			ProjectID: projectID, Name: r.Name, Enabled: enabled, Source: r.Source,
			Match: datatypes.JSON(rawDefault(r.Match, "{}")), Repeat: datatypes.JSON(rawDefault(r.Repeat, "{}")),
			Targets: datatypes.JSON(targets),
		}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		res.Inserted["alert_rules"]++
	}
	return nil
}

func applyMonitorConfig(ctx context.Context, tx *gorm.DB, projectID int, cfg ProjectConfig, res *ApplyResult) error {
	for _, r := range cfg.MonitorDefinitions {
		nextRunAt := r.NextRunAt
		if nextRunAt.IsZero() {
			nextRunAt = time.Now().UTC()
		}
		row := model.MonitorDefinition{
			ProjectID: projectID, Name: r.Name, DetectorType: r.DetectorType,
			Config: datatypes.JSON(rawDefault(r.Config, "{}")), IntervalSec: r.IntervalSec, TimeoutMS: r.TimeoutMS,
			Enabled: r.Enabled, NextRunAt: nextRunAt,
		}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		res.Inserted["monitor_definitions"]++
	}
	return nil
}

func applyLogs(ctx context.Context, tx *gorm.DB, sourceProjectID int, targetProjectID int, rows []LogRecord, res *ApplyResult) error {
	logs := make([]model.Log, 0, len(rows))
	for _, r := range rows {
		ingestID := r.IngestID
		if ingestID == nil || *ingestID == uuid.Nil {
			stable := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("migration:%d:%d:%s:%s", sourceProjectID, r.SourceID, r.Timestamp.UTC().Format(time.RFC3339Nano), r.Message)))
			ingestID = &stable
		}
		logs = append(logs, model.Log{
			ProjectID: targetProjectID, Timestamp: r.Timestamp.UTC(), IngestID: ingestID, Level: r.Level,
			DistinctID: r.DistinctID, DeviceID: r.DeviceID, TraceID: r.TraceID, SpanID: r.SpanID,
			Message: r.Message, Fields: datatypes.JSON(rawDefault(r.Fields, "{}")),
		})
	}
	if _, err := store.InsertLogsAndTrackEventsBatch(ctx, tx, logs); err != nil {
		return err
	}
	res.Inserted["logs"] += int64(len(logs))
	return nil
}

func applyEvents(ctx context.Context, tx *gorm.DB, targetProjectID int, rows []EventRecord, res *ApplyResult) error {
	events := make([]model.Event, 0, len(rows))
	for _, r := range rows {
		if r.ID == uuid.Nil {
			r.ID = uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("migration:event:%d:%s:%s", targetProjectID, r.Timestamp.UTC().Format(time.RFC3339Nano), r.Title)))
		}
		events = append(events, model.Event{
			ID: r.ID, ProjectID: targetProjectID, Timestamp: r.Timestamp.UTC(), Level: r.Level,
			DistinctID: r.DistinctID, DeviceID: r.DeviceID, OS: r.OS, Platform: r.Platform,
			ReleaseTag: r.ReleaseTag, Environment: r.Environment, UserID: r.UserID, Title: r.Title,
			Data: datatypes.JSON(rawDefault(r.Data, "{}")),
		})
	}
	if _, err := store.InsertEventsBatch(ctx, tx, events); err != nil {
		return err
	}
	res.Inserted["events"] += int64(len(events))
	return nil
}

func rawDefault(v json.RawMessage, fallback string) []byte {
	if len(v) == 0 {
		return []byte(fallback)
	}
	return []byte(v)
}

func firstOrCreateAlertContact(ctx context.Context, tx *gorm.DB, row *model.AlertContact) error {
	err := tx.WithContext(ctx).Where("project_id = ? AND type = ? AND value = ?", row.ProjectID, row.Type, row.Value).First(row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.WithContext(ctx).Create(row).Error
	}
	return err
}

func firstOrCreateAlertContactGroup(ctx context.Context, tx *gorm.DB, row *model.AlertContactGroup) error {
	err := tx.WithContext(ctx).Where("project_id = ? AND type = ? AND name = ?", row.ProjectID, row.Type, row.Name).First(row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.WithContext(ctx).Create(row).Error
	}
	return err
}

func firstOrCreateWecomBot(ctx context.Context, tx *gorm.DB, row *model.AlertWecomBot) error {
	err := tx.WithContext(ctx).Where("project_id = ? AND webhook_url = ?", row.ProjectID, row.WebhookURL).First(row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.WithContext(ctx).Create(row).Error
	}
	return err
}

func firstOrCreateWebhookEndpoint(ctx context.Context, tx *gorm.DB, row *model.AlertWebhookEndpoint) error {
	err := tx.WithContext(ctx).Where("project_id = ? AND url = ?", row.ProjectID, row.URL).First(row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.WithContext(ctx).Create(row).Error
	}
	return err
}

func rewriteRuleTargets(raw json.RawMessage, contactIDMap map[int]int, groupIDMap map[int]int, wecomIDMap map[int]int, webhookIDMap map[int]int) ([]byte, bool) {
	var targets alert.RuleTargets
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &targets); err != nil {
			return rawDefault(raw, "{}"), false
		}
	}
	ok := true
	targets.EmailContactIDs, ok = rewriteIDs(targets.EmailContactIDs, contactIDMap, ok)
	targets.SMSContactIDs, ok = rewriteIDs(targets.SMSContactIDs, contactIDMap, ok)
	targets.EmailGroupIDs, ok = rewriteIDs(targets.EmailGroupIDs, groupIDMap, ok)
	targets.SMSGroupIDs, ok = rewriteIDs(targets.SMSGroupIDs, groupIDMap, ok)
	targets.WecomBotIDs, ok = rewriteIDs(targets.WecomBotIDs, wecomIDMap, ok)
	targets.WebhookEndpointIDs, ok = rewriteIDs(targets.WebhookEndpointIDs, webhookIDMap, ok)
	b, err := json.Marshal(targets)
	if err != nil {
		return []byte(`{}`), false
	}
	return b, ok
}

func rewriteIDs(in []int, mapping map[int]int, ok bool) ([]int, bool) {
	if len(in) == 0 {
		return nil, ok
	}
	out := make([]int, 0, len(in))
	for _, id := range in {
		if id <= 0 {
			continue
		}
		newID := mapping[id]
		if newID <= 0 {
			ok = false
			continue
		}
		out = append(out, newID)
	}
	return out, ok
}
