package store

import (
	"context"

	"github.com/aak1247/logtap/internal/model"
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
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(&rows, 200).Error
}
