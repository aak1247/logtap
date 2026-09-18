package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aak1247/logtap/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func InsertLogsBatch(ctx context.Context, db *gorm.DB, rows []model.Log) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	newRows, err := filterExistingLogIngestIDs(ctx, db, rows)
	if err != nil {
		return err
	}
	if len(newRows) == 0 {
		return nil
	}
	// Use a conservative batch size; caller can already be batching.
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&newRows, 200).Error; err != nil {
		return err
	}
	if err := UpsertUserFirstSeenFromLogs(ctx, db, newRows); err != nil {
		return err
	}
	return UpsertLogMetricsFromLogs(ctx, db, newRows)
}

// InsertEventsBatch inserts rows idempotently and returns the rows that were
// new (passed the existence filter), so callers can trigger per-row side
// effects such as alert evaluation without a separate existence query.
func InsertEventsBatch(ctx context.Context, db *gorm.DB, rows []model.Event) ([]model.Event, error) {
	if db == nil || len(rows) == 0 {
		return nil, nil
	}
	var kept []model.Event
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockEventIDs(ctx, tx, rows); err != nil {
			return err
		}
		newRows, err := filterExistingEventIDs(ctx, tx, rows)
		if err != nil {
			return err
		}
		if len(newRows) == 0 {
			return nil
		}
		if err := tx.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(&newRows, 1000).Error; err != nil {
			return err
		}
		if err := UpsertUserFirstSeenFromEvents(ctx, tx, newRows); err != nil {
			return err
		}
		if err := UpsertEventMetricsFromEvents(ctx, tx, newRows); err != nil {
			return err
		}
		kept = newRows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return kept, nil
}

func lockEventIDs(ctx context.Context, db *gorm.DB, rows []model.Event) error {
	if db == nil || !strings.EqualFold(db.Dialector.Name(), "postgres") || len(rows) == 0 {
		return nil
	}
	seen := map[uuid.UUID]bool{}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.ID == uuid.Nil || seen[row.ID] {
			continue
		}
		seen[row.ID] = true
		keys = append(keys, row.ID.String())
	}
	return lockAdvisoryKeys(ctx, db, keys, "event id")
}

// lockAdvisoryKeys takes all transaction-scoped advisory locks in a single
// round trip. Keys are sorted so concurrent batches acquire overlapping locks
// in a consistent order, avoiding lock-order deadlocks.
func lockAdvisoryKeys(ctx context.Context, db *gorm.DB, keys []string, what string) error {
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	placeholders := make([]string, len(keys))
	args := make([]any, len(keys))
	for i, k := range keys {
		placeholders[i] = "(?)"
		args[i] = k
	}
	stmt := `SELECT pg_advisory_xact_lock(hashtextextended(k, 0)) FROM (VALUES ` +
		strings.Join(placeholders, ",") + `) AS v(k)`
	if err := db.WithContext(ctx).Exec(stmt, args...).Error; err != nil {
		return fmt.Errorf("lock %s: %w", what, err)
	}
	return nil
}

func filterExistingEventIDs(ctx context.Context, db *gorm.DB, rows []model.Event) ([]model.Event, error) {
	if db == nil || len(rows) == 0 {
		return rows, nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	seen := map[uuid.UUID]bool{}
	for _, row := range rows {
		if row.ID == uuid.Nil || seen[row.ID] {
			continue
		}
		seen[row.ID] = true
		ids = append(ids, row.ID)
	}
	if len(ids) == 0 {
		return rows, nil
	}

	type foundRow struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	var found []foundRow
	if err := db.WithContext(ctx).
		Model(&model.Event{}).
		Select("id").
		Where("id IN ?", ids).
		Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("check existing events: %w", err)
	}
	existing := map[uuid.UUID]bool{}
	for _, row := range found {
		existing[row.ID] = true
	}

	seenInBatch := map[uuid.UUID]bool{}
	filtered := make([]model.Event, 0, len(rows))
	for _, row := range rows {
		if row.ID != uuid.Nil {
			if existing[row.ID] || seenInBatch[row.ID] {
				continue
			}
			seenInBatch[row.ID] = true
		}
		filtered = append(filtered, row)
	}
	return filtered, nil
}
