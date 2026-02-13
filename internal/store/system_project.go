package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

type SystemIngestConfig struct {
	ProjectID  int    `json:"project_id"`
	ProjectKey string `json:"project_key"`
}

func EnsureSystemIngestConfig(ctx context.Context, db *gorm.DB, ownerUserID int64) (SystemIngestConfig, error) {
	if db == nil {
		return SystemIngestConfig{}, gorm.ErrInvalidDB
	}
	if ownerUserID <= 0 {
		return SystemIngestConfig{}, errors.New("ownerUserID required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Prefer existing system project (global).
	var p model.Project
	err := db.WithContext(ctx).
		Where("is_system = ?", true).
		Order("id ASC").
		First(&p).Error
	if err != nil {
		// Create a system project under current user.
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return SystemIngestConfig{}, err
		}
		name := "System"
		p = model.Project{
			OwnerUserID: ownerUserID,
			Name:        name,
			IsDefault:   false,
			IsSystem:    true,
			CreatedAt:   time.Now().UTC(),
		}
		if err := db.WithContext(ctx).Create(&p).Error; err != nil {
			return SystemIngestConfig{}, err
		}
	}

	// Reuse an active key if it exists.
	var k model.ProjectKey
	err = db.WithContext(ctx).
		Where("project_id = ? AND revoked_at IS NULL", p.ID).
		Order("id ASC").
		First(&k).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return SystemIngestConfig{}, err
		}
		row, err := CreateProjectKey(ctx, db, p.ID, "system")
		if err != nil {
			return SystemIngestConfig{}, err
		}
		return SystemIngestConfig{ProjectID: p.ID, ProjectKey: row.Key}, nil
	}

	key := strings.TrimSpace(k.Key)
	if key == "" {
		row, err := CreateProjectKey(ctx, db, p.ID, "system")
		if err != nil {
			return SystemIngestConfig{}, err
		}
		key = row.Key
	}

	return SystemIngestConfig{ProjectID: p.ID, ProjectKey: key}, nil
}
