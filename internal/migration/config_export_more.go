package migration

import (
	"context"
	"encoding/json"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

func rawOrObject(v []byte) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(v)
}

func rawOrNil(v []byte) json.RawMessage {
	if len(v) == 0 {
		return nil
	}
	return json.RawMessage(v)
}

func exportAlertConfig(ctx context.Context, db *gorm.DB, projectID int, includeNotificationSecrets bool, out *ProjectConfig) error {
	var contacts []model.AlertContact
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&contacts).Error; err != nil {
		return err
	}
	for _, r := range contacts {
		out.AlertContacts = append(out.AlertContacts, AlertContactRecord{
			ID: r.ID, Type: r.Type, Name: r.Name, Value: r.Value, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}

	var groups []model.AlertContactGroup
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&groups).Error; err != nil {
		return err
	}
	groupIDs := make([]int, 0, len(groups))
	for _, r := range groups {
		groupIDs = append(groupIDs, r.ID)
		out.AlertContactGroups = append(out.AlertContactGroups, AlertContactGroupRecord{
			ID: r.ID, Type: r.Type, Name: r.Name, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	if len(groupIDs) > 0 {
		var members []model.AlertContactGroupMember
		if err := db.WithContext(ctx).Where("group_id IN ?", groupIDs).Order("id ASC").Find(&members).Error; err != nil {
			return err
		}
		for _, r := range members {
			out.AlertContactGroupMembers = append(out.AlertContactGroupMembers, AlertContactGroupMemberRecord{
				ID: r.ID, GroupID: r.GroupID, ContactID: r.ContactID, CreatedAt: r.CreatedAt,
			})
		}
	}

	var bots []model.AlertWecomBot
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&bots).Error; err != nil {
		return err
	}
	for _, r := range bots {
		webhookURL := r.WebhookURL
		if !includeNotificationSecrets {
			webhookURL = ""
		}
		out.AlertWecomBots = append(out.AlertWecomBots, AlertWecomBotRecord{
			ID: r.ID, Name: r.Name, WebhookURL: webhookURL, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}

	var endpoints []model.AlertWebhookEndpoint
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&endpoints).Error; err != nil {
		return err
	}
	for _, r := range endpoints {
		url := r.URL
		if !includeNotificationSecrets {
			url = ""
		}
		out.AlertWebhookEndpoints = append(out.AlertWebhookEndpoints, AlertWebhookEndpointRecord{
			ID: r.ID, Name: r.Name, URL: url, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}

	var rules []model.AlertRule
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&rules).Error; err != nil {
		return err
	}
	for _, r := range rules {
		out.AlertRules = append(out.AlertRules, AlertRuleRecord{
			ID: r.ID, Name: r.Name, Enabled: r.Enabled, Source: r.Source,
			Match: rawOrObject(r.Match), Repeat: rawOrObject(r.Repeat), Targets: rawOrObject(r.Targets),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return nil
}

func exportMonitorConfig(ctx context.Context, db *gorm.DB, projectID int, out *ProjectConfig) error {
	var monitors []model.MonitorDefinition
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&monitors).Error; err != nil {
		return err
	}
	for _, r := range monitors {
		out.MonitorDefinitions = append(out.MonitorDefinitions, MonitorDefinitionRecord{
			ID: r.ID, Name: r.Name, DetectorType: r.DetectorType, Config: rawOrObject(r.Config),
			IntervalSec: r.IntervalSec, TimeoutMS: r.TimeoutMS, Enabled: r.Enabled, NextRunAt: r.NextRunAt,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return nil
}
