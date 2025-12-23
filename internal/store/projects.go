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
