package store

import (
	"context"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

func DeleteLogsBefore(ctx context.Context, db *gorm.DB, projectID int, before time.Time) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	res := db.WithContext(ctx).
		Where("project_id = ? AND timestamp < ?", projectID, before.UTC()).
		Delete(&model.Log{})
	return res.RowsAffected, res.Error
}

func DeleteEventsBefore(ctx context.Context, db *gorm.DB, projectID int, before time.Time) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	res := db.WithContext(ctx).
		Where("project_id = ? AND timestamp < ?", projectID, before.UTC()).
		Delete(&model.Event{})
	return res.RowsAffected, res.Error
}
