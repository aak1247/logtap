package store

import (
	"context"
	"fmt"
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
	// Use a conservative batch size; caller can already be batching.
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&rows, 200).Error
}

func InsertEventsBatch(ctx context.Context, db *gorm.DB, rows []model.Event) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		return tx.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(&newRows, 200).Error
	})
}

func lockEventIDs(ctx context.Context, db *gorm.DB, rows []model.Event) error {
	if db == nil || !strings.EqualFold(db.Dialector.Name(), "postgres") || len(rows) == 0 {
		return nil
	}
	seen := map[uuid.UUID]bool{}
	for _, row := range rows {
		if row.ID == uuid.Nil || seen[row.ID] {
			continue
		}
		seen[row.ID] = true
		if err := db.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, row.ID.String()).Error; err != nil {
			return fmt.Errorf("lock event id: %w", err)
		}
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
