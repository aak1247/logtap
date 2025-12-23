package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

type ProjectKeyRow struct {
	ID        int        `json:"id"`
	ProjectID int        `json:"project_id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func ListProjectKeys(ctx context.Context, db *gorm.DB, projectID int) ([]ProjectKeyRow, error) {
	if db == nil || projectID <= 0 {
		return nil, nil
	}
	var rows []model.ProjectKey
	if err := db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ProjectKeyRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProjectKeyRow{
			ID:        r.ID,
			ProjectID: r.ProjectID,
			Name:      r.Name,
			Key:       r.Key,
			RevokedAt: r.RevokedAt,
			CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

func CreateProjectKey(ctx context.Context, db *gorm.DB, projectID int, name string) (ProjectKeyRow, error) {
	if db == nil || projectID <= 0 {
		return ProjectKeyRow{}, nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	if len(name) > 200 {
		name = name[:200]
	}

	key, err := newProjectKey()
	if err != nil {
		return ProjectKeyRow{}, err
	}

	k := model.ProjectKey{ProjectID: projectID, Name: name, Key: key}
	err = db.WithContext(ctx).Create(&k).Error
	if err != nil {
		// Retry once only if it looks like a key collision.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			key2, err2 := newProjectKey()
			if err2 != nil {
				return ProjectKeyRow{}, err
			}
			k2 := model.ProjectKey{ProjectID: projectID, Name: name, Key: key2}
			if err := db.WithContext(ctx).Create(&k2).Error; err != nil {
				return ProjectKeyRow{}, err
			}
			return ProjectKeyRow{
				ID:        k2.ID,
				ProjectID: k2.ProjectID,
				Name:      k2.Name,
				Key:       k2.Key,
				CreatedAt: k2.CreatedAt,
				RevokedAt: k2.RevokedAt,
			}, nil
		}
		return ProjectKeyRow{}, err
	}
	return ProjectKeyRow{
		ID:        k.ID,
		ProjectID: k.ProjectID,
		Name:      k.Name,
		Key:       k.Key,
		CreatedAt: k.CreatedAt,
		RevokedAt: k.RevokedAt,
	}, nil
}

func RevokeProjectKey(ctx context.Context, db *gorm.DB, projectID int, keyID int, now time.Time) (bool, error) {
	if db == nil || projectID <= 0 || keyID <= 0 {
		return false, nil
	}
	res := db.WithContext(ctx).Model(&model.ProjectKey{}).
		Where("id = ? AND project_id = ? AND revoked_at IS NULL", keyID, projectID).
		Update("revoked_at", now.UTC())
	return res.RowsAffected > 0, res.Error
}

func ValidateProjectKey(ctx context.Context, db *gorm.DB, projectID int, key string) (bool, error) {
	if db == nil || projectID <= 0 {
		return false, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false, nil
	}
	var n int64
	err := db.WithContext(ctx).Model(&model.ProjectKey{}).
		Where("project_id = ? AND key = ? AND revoked_at IS NULL", projectID, key).
		Count(&n).Error
	return n > 0, err
}

func newProjectKey() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return "pk_" + base64.RawURLEncoding.EncodeToString(b), nil
}
