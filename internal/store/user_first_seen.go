package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func UpsertUserFirstSeenFromLogs(ctx context.Context, db *gorm.DB, rows []model.Log) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	items := userFirstSeenFromLogs(rows)
	return upsertUserFirstSeen(ctx, db, items)
}

func UpsertUserFirstSeenFromTrackEvents(ctx context.Context, db *gorm.DB, rows []model.TrackEvent) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	items := userFirstSeenFromTrackEvents(rows)
	return upsertUserFirstSeen(ctx, db, items)
}

func UpsertUserFirstSeenFromEvents(ctx context.Context, db *gorm.DB, rows []model.Event) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	items := userFirstSeenFromEvents(rows)
	return upsertUserFirstSeen(ctx, db, items)
}

func userFirstSeenFromLogs(rows []model.Log) []model.UserFirstSeen {
	byUser := map[string]model.UserFirstSeen{}
	for _, row := range rows {
		distinctID := strings.TrimSpace(row.DistinctID)
		if row.ProjectID <= 0 || distinctID == "" || row.Timestamp.IsZero() {
			continue
		}
		key := userFirstSeenKey(row.ProjectID, distinctID)
		item := model.UserFirstSeen{ProjectID: row.ProjectID, DistinctID: distinctID, FirstSeen: row.Timestamp.UTC()}
		if cur, ok := byUser[key]; !ok || item.FirstSeen.Before(cur.FirstSeen) {
			byUser[key] = item
		}
	}
	return userFirstSeenValues(byUser)
}

func userFirstSeenFromTrackEvents(rows []model.TrackEvent) []model.UserFirstSeen {
	byUser := map[string]model.UserFirstSeen{}
	for _, row := range rows {
		distinctID := strings.TrimSpace(row.DistinctID)
		if row.ProjectID <= 0 || distinctID == "" || row.Timestamp.IsZero() {
			continue
		}
		key := userFirstSeenKey(row.ProjectID, distinctID)
		item := model.UserFirstSeen{ProjectID: row.ProjectID, DistinctID: distinctID, FirstSeen: row.Timestamp.UTC()}
		if cur, ok := byUser[key]; !ok || item.FirstSeen.Before(cur.FirstSeen) {
			byUser[key] = item
		}
	}
	return userFirstSeenValues(byUser)
}

func userFirstSeenFromEvents(rows []model.Event) []model.UserFirstSeen {
	byUser := map[string]model.UserFirstSeen{}
	for _, row := range rows {
		distinctID := strings.TrimSpace(row.DistinctID)
		if row.ProjectID <= 0 || distinctID == "" || row.Timestamp.IsZero() {
			continue
		}
		key := userFirstSeenKey(row.ProjectID, distinctID)
		item := model.UserFirstSeen{ProjectID: row.ProjectID, DistinctID: distinctID, FirstSeen: row.Timestamp.UTC()}
		if cur, ok := byUser[key]; !ok || item.FirstSeen.Before(cur.FirstSeen) {
			byUser[key] = item
		}
	}
	return userFirstSeenValues(byUser)
}

func upsertUserFirstSeen(ctx context.Context, db *gorm.DB, rows []model.UserFirstSeen) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	if strings.EqualFold(db.Dialector.Name(), "postgres") {
		return db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "project_id"}, {Name: "distinct_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"first_seen": gorm.Expr("LEAST(user_first_seen.first_seen, EXCLUDED.first_seen)"),
				"updated_at": time.Now().UTC(),
			}),
		}).CreateInBatches(&rows, 200).Error
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "project_id"}, {Name: "distinct_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"first_seen": gorm.Expr("MIN(user_first_seen.first_seen, excluded.first_seen)"),
			"updated_at": time.Now().UTC(),
		}),
	}).CreateInBatches(&rows, 200).Error
}

func userFirstSeenKey(projectID int, distinctID string) string {
	return fmt.Sprintf("%d\x00%s", projectID, distinctID)
}

func userFirstSeenValues(byUser map[string]model.UserFirstSeen) []model.UserFirstSeen {
	if len(byUser) == 0 {
		return nil
	}
	out := make([]model.UserFirstSeen, 0, len(byUser))
	for _, item := range byUser {
		out = append(out, item)
	}
	return out
}
