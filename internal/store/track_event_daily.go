package store

import (
	"context"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TrackEventDailyRowsFromTrackEvents(events []model.TrackEvent) []model.TrackEventDaily {
	if len(events) == 0 {
		return nil
	}
	type key struct {
		projectID  int
		day        string
		name       string
		distinctID string
	}
	agg := make(map[key]int64, len(events))
	for _, e := range events {
		if e.ProjectID <= 0 || e.Timestamp.IsZero() || e.Name == "" || e.DistinctID == "" {
			continue
		}
		day := e.Timestamp.UTC().Format("2006-01-02")
		k := key{
			projectID:  e.ProjectID,
			day:        day,
			name:       e.Name,
			distinctID: e.DistinctID,
		}
		agg[k]++
	}
	if len(agg) == 0 {
		return nil
	}
	out := make([]model.TrackEventDaily, 0, len(agg))
	for k, n := range agg {
		out = append(out, model.TrackEventDaily{
			ProjectID:  k.projectID,
			Day:        k.day,
			Name:       k.name,
			DistinctID: k.distinctID,
			Events:     n,
		})
	}
	return out
}

func UpsertTrackEventDailyBatch(ctx context.Context, db *gorm.DB, rows []model.TrackEventDaily) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "project_id"},
				{Name: "day"},
				{Name: "name"},
				{Name: "distinct_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"events":     gorm.Expr("track_event_daily.events + EXCLUDED.events"),
				"updated_at": now,
			}),
		}).
		CreateInBatches(&rows, 200).Error
}

func PruneAndRebuildTrackEventDailyForRetention(ctx context.Context, db *gorm.DB, projectID int, before time.Time) error {
	if db == nil || projectID <= 0 {
		return gorm.ErrInvalidData
	}
	if !db.Migrator().HasTable(model.TrackEventDaily{}.TableName()) {
		return nil
	}

	before = before.UTC()
	cutoffDay := before.Format("2006-01-02")
	dayStart := time.Date(before.Year(), before.Month(), before.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)
	now := time.Now().UTC()

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Prune fully expired days.
		if err := tx.Exec(
			`DELETE FROM track_event_daily WHERE project_id = ? AND day < ?`,
			projectID, cutoffDay,
		).Error; err != nil {
			return err
		}

		// Rebuild the cutoff day (retention cutoff is time-of-day, so this day can be partial).
		if err := tx.Exec(
			`DELETE FROM track_event_daily WHERE project_id = ? AND day = ?`,
			projectID, cutoffDay,
		).Error; err != nil {
			return err
		}

		// Insert fresh aggregates from remaining track_events.
		return tx.Exec(`
			INSERT INTO track_event_daily (project_id, day, name, distinct_id, events, created_at, updated_at)
			SELECT
				project_id,
				?,
				name,
				distinct_id,
				COUNT(*) AS events,
				?,
				?
			FROM track_events
			WHERE project_id = ? AND timestamp >= ? AND timestamp < ?
			GROUP BY project_id, name, distinct_id
		`, cutoffDay, now, now, projectID, dayStart, dayEnd).Error
	})
}
