package migration

import (
	"context"
	"errors"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

func exportProjectConfig(ctx context.Context, db *gorm.DB, projectID int, includeNotificationSecrets bool, out *ProjectConfig) error {
	var cleanup model.CleanupPolicy
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).First(&cleanup).Error; err == nil {
		out.CleanupPolicy = &CleanupPolicyRecord{
			Enabled:                  cleanup.Enabled,
			LogsRetentionDays:        cleanup.LogsRetentionDays,
			EventsRetentionDays:      cleanup.EventsRetentionDays,
			TrackEventsRetentionDays: cleanup.TrackEventsRetentionDays,
			ScheduleHourUTC:          cleanup.ScheduleHourUTC,
			ScheduleMinuteUTC:        cleanup.ScheduleMinuteUTC,
			LastRunAt:                cleanup.LastRunAt,
			NextRunAt:                cleanup.NextRunAt,
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	var eventDefs []model.EventDefinition
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&eventDefs).Error; err != nil {
		return err
	}
	for _, r := range eventDefs {
		out.EventDefinitions = append(out.EventDefinitions, EventDefinitionRecord{
			ID: r.ID, Name: r.Name, DisplayName: r.DisplayName, Category: r.Category, Description: r.Description,
			Status: r.Status, Owner: r.Owner, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}

	var propDefs []model.PropertyDefinition
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&propDefs).Error; err != nil {
		return err
	}
	for _, r := range propDefs {
		out.PropertyDefinitions = append(out.PropertyDefinitions, PropertyDefinitionRecord{
			ID: r.ID, Key: r.Key, DisplayName: r.DisplayName, Type: r.Type, Description: r.Description, Status: r.Status,
			EnumValues: rawOrNil(r.EnumValues), ExampleValues: rawOrNil(r.ExampleValues), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}

	var views []model.AnalysisView
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&views).Error; err != nil {
		return err
	}
	for _, r := range views {
		out.AnalysisViews = append(out.AnalysisViews, AnalysisViewRecord{
			ID: r.ID, Name: r.Name, Description: r.Description, AnalysisType: r.AnalysisType, Query: rawOrObject(r.Query),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}

	var pluginSettings []model.PluginPackageSetting
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&pluginSettings).Error; err != nil {
		return err
	}
	for _, r := range pluginSettings {
		out.PluginPackageSettings = append(out.PluginPackageSettings, PluginPackageSettingRecord{
			ID: r.ID, PackageID: r.PackageID, Enabled: r.Enabled, Config: rawOrObject(r.Config), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}

	if err := exportAlertConfig(ctx, db, projectID, includeNotificationSecrets, out); err != nil {
		return err
	}
	return exportMonitorConfig(ctx, db, projectID, out)
}
