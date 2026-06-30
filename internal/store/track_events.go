package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TrackEventRowsFromLogs(logs []model.Log) []model.TrackEvent {
	if len(logs) == 0 {
		return nil
	}
	out := make([]model.TrackEvent, 0, len(logs))
	for _, row := range logs {
		if strings.TrimSpace(row.Level) != "event" {
			continue
		}
		name := strings.TrimSpace(row.Message)
		if name == "" {
			continue
		}
		distinctID := strings.TrimSpace(row.DistinctID)
		if distinctID == "" {
			continue
		}
		out = append(out, model.TrackEvent{
			ProjectID:  row.ProjectID,
			Timestamp:  row.Timestamp,
			IngestID:   row.IngestID,
			Name:       name,
			DistinctID: distinctID,
			DeviceID:   strings.TrimSpace(row.DeviceID),
		})
	}
	return out
}

func InsertTrackEventsBatch(ctx context.Context, db *gorm.DB, rows []model.TrackEvent) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&rows, 200).Error
}

func InsertTrackEventsAndRollupBatch(ctx context.Context, db *gorm.DB, rows []model.TrackEvent) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	if strings.EqualFold(db.Dialector.Name(), "postgres") {
		return insertTrackEventsAndRollupPostgres(ctx, db, rows)
	}
	return insertTrackEventsAndRollupBestEffort(ctx, db, rows)
}

func InsertLogsAndTrackEventsBatch(ctx context.Context, db *gorm.DB, logs []model.Log) error {
	if db == nil || len(logs) == 0 {
		return nil
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockLogIngestIDs(ctx, tx, logs); err != nil {
			return err
		}
		newLogs, err := filterExistingLogIngestIDs(ctx, tx, logs)
		if err != nil {
			return err
		}
		if len(newLogs) == 0 {
			return nil
		}
		events := TrackEventRowsFromLogs(newLogs)
		if err := tx.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(&newLogs, 200).Error; err != nil {
			return err
		}
		if err := UpsertUserFirstSeenFromLogs(ctx, tx, newLogs); err != nil {
			return err
		}
		if err := UpsertLogMetricsFromLogs(ctx, tx, newLogs); err != nil {
			return err
		}
		if len(events) > 0 {
			if err := InsertTrackEventsAndRollupBatch(ctx, tx, events); err != nil {
				return err
			}
		}
		return nil
	})
}

func lockLogIngestIDs(ctx context.Context, db *gorm.DB, logs []model.Log) error {
	if db == nil || !strings.EqualFold(db.Dialector.Name(), "postgres") || len(logs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	keys := make([]string, 0, len(logs))
	for _, row := range logs {
		if row.ProjectID <= 0 || row.IngestID == nil || *row.IngestID == uuid.Nil {
			continue
		}
		key := fmt.Sprintf("%d:%s", row.ProjectID, row.IngestID.String())
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	for _, key := range keys {
		if err := db.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, key).Error; err != nil {
			return fmt.Errorf("lock log ingest id: %w", err)
		}
	}
	return nil
}

func filterExistingLogIngestIDs(ctx context.Context, db *gorm.DB, logs []model.Log) ([]model.Log, error) {
	if db == nil || len(logs) == 0 {
		return logs, nil
	}
	byProject := map[int][]uuid.UUID{}
	for _, row := range logs {
		if row.ProjectID <= 0 || row.IngestID == nil || *row.IngestID == uuid.Nil {
			continue
		}
		byProject[row.ProjectID] = append(byProject[row.ProjectID], *row.IngestID)
	}
	if len(byProject) == 0 {
		return logs, nil
	}

	existingByProject := map[int]map[uuid.UUID]bool{}
	for projectID, ingestIDs := range byProject {
		type foundRow struct {
			IngestID uuid.UUID `gorm:"column:ingest_id"`
		}
		var found []foundRow
		if err := db.WithContext(ctx).
			Model(&model.Log{}).
			Select("ingest_id").
			Where("project_id = ? AND ingest_id IN ?", projectID, ingestIDs).
			Scan(&found).Error; err != nil {
			return nil, fmt.Errorf("check existing logs: %w", err)
		}
		if len(found) == 0 {
			continue
		}
		existingByProject[projectID] = map[uuid.UUID]bool{}
		for _, row := range found {
			existingByProject[projectID][row.IngestID] = true
		}
	}

	filtered := make([]model.Log, 0, len(logs))
	seenInBatch := map[int]map[uuid.UUID]bool{}
	for _, row := range logs {
		if row.ProjectID > 0 && row.IngestID != nil && *row.IngestID != uuid.Nil {
			if existingByProject[row.ProjectID][*row.IngestID] {
				continue
			}
			if seenInBatch[row.ProjectID] == nil {
				seenInBatch[row.ProjectID] = map[uuid.UUID]bool{}
			}
			if seenInBatch[row.ProjectID][*row.IngestID] {
				continue
			}
			seenInBatch[row.ProjectID][*row.IngestID] = true
		}
		filtered = append(filtered, row)
	}
	return filtered, nil
}

type trackEventJSONRow struct {
	ProjectID  int        `json:"project_id"`
	Timestamp  time.Time  `json:"timestamp"`
	IngestID   *uuid.UUID `json:"ingest_id"`
	Name       string     `json:"name"`
	DistinctID string     `json:"distinct_id"`
	DeviceID   string     `json:"device_id"`
}

func insertTrackEventsAndRollupPostgres(ctx context.Context, db *gorm.DB, rows []model.TrackEvent) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	in := make([]trackEventJSONRow, 0, len(rows))
	for _, r := range rows {
		if r.ProjectID <= 0 || r.Timestamp.IsZero() || strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.DistinctID) == "" {
			continue
		}
		in = append(in, trackEventJSONRow{
			ProjectID:  r.ProjectID,
			Timestamp:  r.Timestamp.UTC(),
			IngestID:   r.IngestID,
			Name:       strings.TrimSpace(r.Name),
			DistinctID: strings.TrimSpace(r.DistinctID),
			DeviceID:   strings.TrimSpace(r.DeviceID),
		})
	}
	if len(in) == 0 {
		return nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	// Insert track_events with idempotency, roll up only inserted rows, and maintain first-seen users.
	// This keeps rollups exact under retries/concurrency (at-least-once delivery).
	return db.WithContext(ctx).Exec(`
		WITH input AS (
			SELECT *
			FROM jsonb_to_recordset(?::jsonb)
			AS t(
				project_id int,
				timestamp timestamptz,
				ingest_id uuid,
				name text,
				distinct_id text,
				device_id text
			)
		),
		ins AS (
			INSERT INTO track_events (project_id, timestamp, ingest_id, name, distinct_id, device_id)
			SELECT project_id, timestamp, ingest_id, name, distinct_id, device_id
			FROM input
			ON CONFLICT (project_id, ingest_id, timestamp) DO NOTHING
			RETURNING project_id, timestamp, name, distinct_id
		),
		user_seen AS (
			INSERT INTO user_first_seen (project_id, distinct_id, first_seen, updated_at)
			SELECT project_id, distinct_id, MIN(timestamp), NOW()
			FROM ins
			WHERE distinct_id IS NOT NULL AND distinct_id <> ''
			GROUP BY project_id, distinct_id
			ON CONFLICT (project_id, distinct_id) DO UPDATE
			SET first_seen = LEAST(user_first_seen.first_seen, EXCLUDED.first_seen),
			    updated_at = NOW()
			RETURNING 1
		),
		agg AS (
			SELECT
				project_id,
				to_char((timestamp AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS day,
				name,
				distinct_id,
				COUNT(*)::bigint AS events
			FROM ins
			GROUP BY project_id, day, name, distinct_id
		)
		INSERT INTO track_event_daily (project_id, day, name, distinct_id, events, created_at, updated_at)
		SELECT project_id, day, name, distinct_id, events, NOW(), NOW()
		FROM agg
		ON CONFLICT (project_id, day, name, distinct_id) DO UPDATE
		SET events = track_event_daily.events + EXCLUDED.events,
		    updated_at = NOW()
	`, string(b)).Error
}

func insertTrackEventsAndRollupBestEffort(ctx context.Context, db *gorm.DB, rows []model.TrackEvent) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	// Best-effort idempotency for non-Postgres dialects: filter existing ingest_ids first.
	// This is exact under retries when no concurrent writers race on the same ingest_id set.
	byProject := map[int][]model.TrackEvent{}
	for _, r := range rows {
		if r.ProjectID <= 0 || r.Timestamp.IsZero() || strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.DistinctID) == "" {
			continue
		}
		byProject[r.ProjectID] = append(byProject[r.ProjectID], r)
	}
	for projectID, evs := range byProject {
		var ingestIDs []uuid.UUID
		for _, e := range evs {
			if e.IngestID != nil {
				ingestIDs = append(ingestIDs, *e.IngestID)
			}
		}
		existing := map[uuid.UUID]bool{}
		if len(ingestIDs) > 0 {
			type row struct {
				IngestID uuid.UUID `gorm:"column:ingest_id"`
			}
			var found []row
			if err := db.WithContext(ctx).
				Table("track_events").
				Select("ingest_id").
				Where("project_id = ? AND ingest_id IN ?", projectID, ingestIDs).
				Scan(&found).Error; err != nil {
				return fmt.Errorf("check existing track_events: %w", err)
			}
			for _, f := range found {
				existing[f.IngestID] = true
			}
		}

		newEvents := make([]model.TrackEvent, 0, len(evs))
		for _, e := range evs {
			if e.IngestID != nil && existing[*e.IngestID] {
				continue
			}
			newEvents = append(newEvents, e)
		}
		if len(newEvents) == 0 {
			continue
		}
		if err := InsertTrackEventsBatch(ctx, db, newEvents); err != nil {
			return err
		}
		daily := TrackEventDailyRowsFromTrackEvents(newEvents)
		if len(daily) > 0 {
			if err := UpsertTrackEventDailyBatch(ctx, db, daily); err != nil {
				return err
			}
		}
		if err := UpsertUserFirstSeenFromTrackEvents(ctx, db, newEvents); err != nil {
			return err
		}
		if err := UpsertEventMetricsFromTrackEvents(ctx, db, newEvents); err != nil {
			return err
		}
	}
	return nil
}
