package store

import (
	"context"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/google/uuid"
)

func TestTrackEventDailyRowsFromTrackEvents_Aggregates(t *testing.T) {
	t.Parallel()

	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	events := []model.TrackEvent{
		{ProjectID: 1, Timestamp: ts, Name: "signup", DistinctID: "u1"},
		{ProjectID: 1, Timestamp: ts.Add(1 * time.Minute), Name: "signup", DistinctID: "u1"},
		{ProjectID: 1, Timestamp: ts, Name: "signup", DistinctID: "u2"},
		{ProjectID: 1, Timestamp: ts, Name: "checkout", DistinctID: "u1"},
	}
	rows := TrackEventDailyRowsFromTrackEvents(events)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d (%v)", len(rows), rows)
	}

	m := map[string]int64{}
	for _, r := range rows {
		m[r.Day+"|"+r.Name+"|"+r.DistinctID] = r.Events
	}
	day := ts.Format("2006-01-02")
	if m[day+"|signup|u1"] != 2 {
		t.Fatalf("expected signup/u1=2, got %d", m[day+"|signup|u1"])
	}
	if m[day+"|signup|u2"] != 1 {
		t.Fatalf("expected signup/u2=1, got %d", m[day+"|signup|u2"])
	}
	if m[day+"|checkout|u1"] != 1 {
		t.Fatalf("expected checkout/u1=1, got %d", m[day+"|checkout|u1"])
	}
}

func TestUpsertTrackEventDailyBatch_Increments(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	ts := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	rows1 := []model.TrackEventDaily{
		{ProjectID: 1, Day: ts.Format("2006-01-02"), Name: "signup", DistinctID: "u1", Events: 2},
	}
	rows2 := []model.TrackEventDaily{
		{ProjectID: 1, Day: ts.Format("2006-01-02"), Name: "signup", DistinctID: "u1", Events: 3},
	}

	if err := UpsertTrackEventDailyBatch(ctx, db, rows1); err != nil {
		t.Fatalf("UpsertTrackEventDailyBatch rows1: %v", err)
	}
	if err := UpsertTrackEventDailyBatch(ctx, db, rows2); err != nil {
		t.Fatalf("UpsertTrackEventDailyBatch rows2: %v", err)
	}

	var got model.TrackEventDaily
	if err := db.WithContext(ctx).
		Where("project_id = ? AND day = ? AND name = ? AND distinct_id = ?", 1, ts.Format("2006-01-02"), "signup", "u1").
		First(&got).Error; err != nil {
		t.Fatalf("query rollup: %v", err)
	}
	if got.Events != 5 {
		t.Fatalf("expected events=5, got %d", got.Events)
	}
}

func TestPruneAndRebuildTrackEventDailyForRetention_PrunedAndRebuilt(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	// seed rollup with an old day and cutoff day
	if err := UpsertTrackEventDailyBatch(ctx, db, []model.TrackEventDaily{
		{ProjectID: 1, Day: "2025-01-01", Name: "signup", DistinctID: "u1", Events: 9},
		{ProjectID: 1, Day: "2025-01-02", Name: "signup", DistinctID: "u1", Events: 9},
	}); err != nil {
		t.Fatalf("seed rollup: %v", err)
	}

	// seed track_events for cutoff day; only these should remain after rebuild.
	if err := db.WithContext(ctx).Create(&model.TrackEvent{
		ProjectID:  1,
		Timestamp:  time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
		Name:       "signup",
		DistinctID: "u1",
	}).Error; err != nil {
		t.Fatalf("insert track event: %v", err)
	}

	before := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)
	if err := PruneAndRebuildTrackEventDailyForRetention(ctx, db, 1, before); err != nil {
		t.Fatalf("PruneAndRebuildTrackEventDailyForRetention: %v", err)
	}

	var nOld int64
	if err := db.WithContext(ctx).Table("track_event_daily").
		Where("project_id = ? AND day = ?", 1, "2025-01-01").
		Count(&nOld).Error; err != nil {
		t.Fatalf("count old day: %v", err)
	}
	if nOld != 0 {
		t.Fatalf("expected old day pruned, got %d", nOld)
	}

	var got model.TrackEventDaily
	if err := db.WithContext(ctx).
		Where("project_id = ? AND day = ? AND name = ? AND distinct_id = ?", 1, "2025-01-02", "signup", "u1").
		First(&got).Error; err != nil {
		t.Fatalf("query cutoff day: %v", err)
	}
	if got.Events != 1 {
		t.Fatalf("expected rebuilt events=1, got %d", got.Events)
	}
}

func TestInsertLogsAndTrackEventsBatch_IdempotentRollup(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	ingestID := uuid.New()
	ts := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	logRow := model.Log{
		ProjectID:  1,
		Timestamp:  ts,
		IngestID:   &ingestID,
		Level:      "event",
		DistinctID: "u1",
		Message:    "signup",
	}

	if err := InsertLogsAndTrackEventsBatch(ctx, db, []model.Log{logRow}); err != nil {
		t.Fatalf("insert batch1: %v", err)
	}
	if err := InsertLogsAndTrackEventsBatch(ctx, db, []model.Log{logRow}); err != nil {
		t.Fatalf("insert batch2: %v", err)
	}

	var trackEvents int64
	if err := db.WithContext(ctx).Model(&model.TrackEvent{}).Where("project_id = ?", 1).Count(&trackEvents).Error; err != nil {
		t.Fatalf("count track_events: %v", err)
	}
	if trackEvents != 1 {
		t.Fatalf("expected track_events=1, got %d", trackEvents)
	}

	var daily model.TrackEventDaily
	if err := db.WithContext(ctx).
		Where("project_id = ? AND day = ? AND name = ? AND distinct_id = ?", 1, "2025-03-01", "signup", "u1").
		First(&daily).Error; err != nil {
		t.Fatalf("query rollup: %v", err)
	}
	if daily.Events != 1 {
		t.Fatalf("expected rollup events=1, got %d", daily.Events)
	}
}
