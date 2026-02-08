package store

import (
	"context"
	"errors"
	"strings"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

type ProjectRow struct {
	ID          int    `json:"id"`
	OwnerUserID int64  `json:"owner_user_id"`
	Name        string `json:"name"`
}

func ListProjectsByOwner(ctx context.Context, db *gorm.DB, ownerUserID int64) ([]ProjectRow, error) {
	if db == nil || ownerUserID <= 0 {
		return nil, nil
	}
	var rows []model.Project
	if err := db.WithContext(ctx).
		Where("owner_user_id = ?", ownerUserID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ProjectRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProjectRow{ID: r.ID, OwnerUserID: r.OwnerUserID, Name: r.Name})
	}
	return out, nil
}

func CreateProject(ctx context.Context, db *gorm.DB, ownerUserID int64, name string) (ProjectRow, error) {
	if db == nil || ownerUserID <= 0 {
		return ProjectRow{}, nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Untitled"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	p := model.Project{OwnerUserID: ownerUserID, Name: name}
	if err := db.WithContext(ctx).Create(&p).Error; err != nil {
		return ProjectRow{}, err
	}
	return ProjectRow{ID: p.ID, OwnerUserID: p.OwnerUserID, Name: p.Name}, nil
}

func GetProjectByID(ctx context.Context, db *gorm.DB, id int) (ProjectRow, bool, error) {
	if db == nil || id <= 0 {
		return ProjectRow{}, false, nil
	}
	var p model.Project
	err := db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ProjectRow{}, false, nil
		}
		return ProjectRow{}, false, err
	}
	return ProjectRow{ID: p.ID, OwnerUserID: p.OwnerUserID, Name: p.Name}, true, nil
}

func DeleteProject(ctx context.Context, db *gorm.DB, projectID int) (bool, error) {
	if db == nil || projectID <= 0 {
		return false, nil
	}

	var deleted bool
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", projectID).Delete(&model.ProjectKey{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.Event{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&model.Log{}).Error; err != nil {
			return err
		}

		// Best-effort cleanup for optional per-project tables.
		for _, table := range []string{
			"alert_contacts",
			"alert_contact_groups",
			"alert_contact_group_members",
			"alert_wecom_bots",
			"alert_webhook_endpoints",
			"alert_rules",
			"alert_states",
			"alert_deliveries",
		} {
			if err := deleteByProjectIDIfTableExists(tx, table, projectID); err != nil {
				return err
			}
		}

		res := tx.Where("id = ?", projectID).Delete(&model.Project{})
		if res.Error != nil {
			return res.Error
		}
		deleted = res.RowsAffected > 0
		return nil
	})
	return deleted, err
}

func deleteByProjectIDIfTableExists(tx *gorm.DB, table string, projectID int) error {
	if tx == nil {
		return nil
	}
	if !tx.Migrator().HasTable(table) {
		return nil
	}
	return tx.Exec("DELETE FROM "+table+" WHERE project_id = ?", projectID).Error
}
