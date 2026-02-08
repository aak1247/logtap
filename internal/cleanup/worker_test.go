package cleanup

import (
	"context"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/testkit"
)

func TestWorkerRunPolicy_DoesNotDeleteTrackEventsWithLogsRetention(t *testing.T) {
	db := testkit.OpenTestDB(t)
	if err := db.AutoMigrate(&model.CleanupPolicy{}); err != nil {
		t.Fatalf("AutoMigrate(CleanupPolicy): %v", err)
	}

	now := time.Now().UTC()
	old := now.Add(-40 * 24 * time.Hour)

	if err := db.Create(&model.Log{
		ProjectID:  1,
		Timestamp:  old,
		Level:      "event",
		DistinctID: "u1",
		Message:    "signup",
	}).Error; err != nil {
		t.Fatalf("insert log: %v", err)
	}
	if err := db.Create(&model.TrackEvent{
		ProjectID:  1,
		Timestamp:  old,
		Name:       "signup",
		DistinctID: "u1",
	}).Error; err != nil {
		t.Fatalf("insert track_event: %v", err)
	}

	w := NewWorker(db)
	w.MaxBatches = 5
	w.DeleteBatchSize = 1000

	if err := w.runPolicy(context.Background(), 1, 30, 0, 0, 3, 0); err != nil {
		t.Fatalf("runPolicy: %v", err)
	}

	var logs int64
	if err := db.Model(&model.Log{}).Where("project_id = ?", 1).Count(&logs).Error; err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if logs != 0 {
		t.Fatalf("expected logs deleted, got %d", logs)
	}

	var trackEvents int64
	if err := db.Model(&model.TrackEvent{}).Where("project_id = ?", 1).Count(&trackEvents).Error; err != nil {
		t.Fatalf("count track_events: %v", err)
	}
	if trackEvents != 1 {
		t.Fatalf("expected track_events kept, got %d", trackEvents)
	}
}
