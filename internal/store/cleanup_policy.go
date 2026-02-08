package store

import (
	"context"
	"errors"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetCleanupPolicy(ctx context.Context, db *gorm.DB, projectID int) (model.CleanupPolicy, bool, error) {
	if db == nil || projectID <= 0 {
		return model.CleanupPolicy{}, false, gorm.ErrInvalidDB
	}
	var row model.CleanupPolicy
	err := db.WithContext(ctx).Where("project_id = ?", projectID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.CleanupPolicy{}, false, nil
		}
		return model.CleanupPolicy{}, false, err
	}
	return row, true, nil
}

func UpsertCleanupPolicy(ctx context.Context, db *gorm.DB, row model.CleanupPolicy) (model.CleanupPolicy, error) {
	if db == nil || row.ProjectID <= 0 {
		return model.CleanupPolicy{}, gorm.ErrInvalidDB
	}
	now := time.Now().UTC()
	row.UpdatedAt = now
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	if row.Enabled {
		next := ComputeNextRunAt(now, row.ScheduleHourUTC, row.ScheduleMinuteUTC)
		row.NextRunAt = &next
	} else {
		row.NextRunAt = nil
	}

	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}},
		UpdateAll: true,
	}).Create(&row).Error; err != nil {
		return model.CleanupPolicy{}, err
	}
	return row, nil
}

func ListCleanupPoliciesDue(ctx context.Context, db *gorm.DB, now time.Time, limit int) ([]model.CleanupPolicy, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	var rows []model.CleanupPolicy
	q := db.WithContext(ctx).
		Where("enabled = true").
		Where("next_run_at IS NOT NULL").
		Where("next_run_at <= ?", now.UTC()).
		Order("next_run_at ASC").
		Limit(limit)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func MarkCleanupPolicyRun(ctx context.Context, db *gorm.DB, projectID int, lastRunAt time.Time, hourUTC, minuteUTC int) error {
	if db == nil || projectID <= 0 {
		return gorm.ErrInvalidDB
	}
	lastRunAt = lastRunAt.UTC()
	next := ComputeNextRunAt(lastRunAt.Add(1*time.Minute), hourUTC, minuteUTC)
	return db.WithContext(ctx).
		Model(&model.CleanupPolicy{}).
		Where("project_id = ?", projectID).
		Updates(map[string]any{
			"last_run_at": lastRunAt,
			"next_run_at": next,
			"updated_at":  time.Now().UTC(),
		}).Error
}

func MarkCleanupPolicyInProgress(ctx context.Context, db *gorm.DB, projectID int, lastRunAt time.Time, nextRunAt time.Time) error {
	if db == nil || projectID <= 0 {
		return gorm.ErrInvalidDB
	}
	lastRunAt = lastRunAt.UTC()
	nextRunAt = nextRunAt.UTC()
	return db.WithContext(ctx).
		Model(&model.CleanupPolicy{}).
		Where("project_id = ?", projectID).
		Updates(map[string]any{
			"last_run_at": lastRunAt,
			"next_run_at": nextRunAt,
			"updated_at":  time.Now().UTC(),
		}).Error
}

func ComputeNextRunAt(now time.Time, hourUTC, minuteUTC int) time.Time {
	now = now.UTC()
	if hourUTC < 0 {
		hourUTC = 0
	}
	if hourUTC > 23 {
		hourUTC = 23
	}
	if minuteUTC < 0 {
		minuteUTC = 0
	}
	if minuteUTC > 59 {
		minuteUTC = 59
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), hourUTC, minuteUTC, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}
