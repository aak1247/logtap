package query

import (
	"context"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/model"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
)

func TestRetentionRows_FallsBackToDBWhenRedisMissingHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openActiveSeriesTestDB(t)
	day1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)
	if err := db.Create(&[]model.Log{
		{ProjectID: 1, Timestamp: day1, DistinctID: "u1", Level: "info", Message: "a", Fields: datatypes.JSON([]byte("{}"))},
		{ProjectID: 1, Timestamp: day1.Add(time.Hour), DistinctID: "u2", Level: "info", Message: "b", Fields: datatypes.JSON([]byte("{}"))},
		{ProjectID: 1, Timestamp: day2, DistinctID: "u1", Level: "info", Message: "c", Fields: datatypes.JSON([]byte("{}"))},
	}).Error; err != nil {
		t.Fatalf("insert logs: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	rec := metrics.NewRedisRecorder(rdb)

	rows, err := retentionRows(ctx, rec, db, 1, day1, day1, []int{1})
	if err != nil {
		t.Fatalf("retentionRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %+v", rows)
	}
	if rows[0].CohortSize != 2 {
		t.Fatalf("expected cohort size 2, got %+v", rows[0])
	}
	if len(rows[0].Points) != 1 || rows[0].Points[0].Active != 1 || rows[0].Points[0].Rate != 0.5 {
		t.Fatalf("unexpected retention point: %+v", rows[0].Points)
	}
}
