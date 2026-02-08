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

func DeleteLogsBeforeBatched(ctx context.Context, db *gorm.DB, projectID int, before time.Time, batchSize int) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if projectID <= 0 {
		return 0, gorm.ErrInvalidData
	}
	if batchSize <= 0 {
		batchSize = 5000
	}

	before = before.UTC()
	// Use a subquery to limit deletion size and keep transactions short.
	// Works on Postgres/TimescaleDB and SQLite.
	res := db.WithContext(ctx).Exec(`
		WITH doomed AS (
			SELECT id FROM logs
			WHERE project_id = ? AND timestamp < ?
			ORDER BY timestamp ASC
			LIMIT ?
		)
		DELETE FROM logs WHERE id IN (SELECT id FROM doomed)
	`, projectID, before, batchSize)
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

func DeleteEventsBeforeBatched(ctx context.Context, db *gorm.DB, projectID int, before time.Time, batchSize int) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if projectID <= 0 {
		return 0, gorm.ErrInvalidData
	}
	if batchSize <= 0 {
		batchSize = 5000
	}

	before = before.UTC()
	res := db.WithContext(ctx).Exec(`
		WITH doomed AS (
			SELECT id FROM events
			WHERE project_id = ? AND timestamp < ?
			ORDER BY timestamp ASC
			LIMIT ?
		)
		DELETE FROM events WHERE id IN (SELECT id FROM doomed)
	`, projectID, before, batchSize)
	return res.RowsAffected, res.Error
}

func DeleteTrackEventsBeforeBatched(ctx context.Context, db *gorm.DB, projectID int, before time.Time, batchSize int) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if projectID <= 0 {
		return 0, gorm.ErrInvalidData
	}
	if batchSize <= 0 {
		batchSize = 5000
	}

	before = before.UTC()
	res := db.WithContext(ctx).Exec(`
		WITH doomed AS (
			SELECT id FROM track_events
			WHERE project_id = ? AND timestamp < ?
			ORDER BY timestamp ASC
			LIMIT ?
		)
		DELETE FROM track_events WHERE id IN (SELECT id FROM doomed)
	`, projectID, before, batchSize)
	return res.RowsAffected, res.Error
}

func DeleteTrackEventsBefore(ctx context.Context, db *gorm.DB, projectID int, before time.Time) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	res := db.WithContext(ctx).
		Where("project_id = ? AND timestamp < ?", projectID, before.UTC()).
		Delete(&model.TrackEvent{})
	return res.RowsAffected, res.Error
}
