package query

import (
	"context"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/model"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openActiveSeriesTestDB(t testing.TB) *gorm.DB {
	t.Helper()

	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite): %v", err)
	}
	if err := gdb.AutoMigrate(&model.Log{}, &model.Event{}, &model.TrackEvent{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return gdb
}

func TestActiveSeries_MergesRedisWithDBBackfill(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openActiveSeriesTestDB(t)
	day1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)
	if err := db.Create(&[]model.Log{
		{ProjectID: 1, Timestamp: day1, DistinctID: "u1", Level: "info", Message: "a", Fields: datatypes.JSON([]byte("{}"))},
		{ProjectID: 1, Timestamp: day1.Add(time.Hour), DistinctID: "u2", Level: "info", Message: "b", Fields: datatypes.JSON([]byte("{}"))},
	}).Error; err != nil {
		t.Fatalf("insert logs: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	rec := metrics.NewRedisRecorder(rdb)
	rec.ObserveLog(ctx, 1, "info", "u3", "d3", day2)

	series, err := activeSeries(ctx, rec, db, 1, day1, day2, "day")
	if err != nil {
		t.Fatalf("activeSeries: %v", err)
	}
	if len(series) != 2 {
		t.Fatalf("expected 2 buckets, got %+v", series)
	}
	if series[0].Bucket != "2025-01-01" || series[0].Active != 2 {
		t.Fatalf("unexpected first bucket: %+v", series[0])
	}
	if series[1].Bucket != "2025-01-02" || series[1].Active != 1 {
		t.Fatalf("unexpected second bucket: %+v", series[1])
	}
}

func TestActiveSeries_SkipsDBBackfillWhenRedisWarmupCoversRange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openActiveSeriesTestDB(t)
	day := time.Now().UTC().AddDate(0, 0, -1)
	if err := db.Create(&model.Log{
		ProjectID:  1,
		Timestamp:  day,
		DistinctID: "u1",
		Level:      "info",
		Message:    "a",
		Fields:     datatypes.JSON([]byte("{}")),
	}).Error; err != nil {
		t.Fatalf("insert log: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	rec := metrics.NewRedisRecorder(rdb)
	if err := rdb.Set(ctx, "active:warmup:ready:day:"+time.Now().UTC().Format("2006-01-02"), 14, time.Hour).Err(); err != nil {
		t.Fatalf("set ready: %v", err)
	}

	series, err := activeSeries(ctx, rec, db, 1, day, day, "day")
	if err != nil {
		t.Fatalf("activeSeries: %v", err)
	}
	if len(series) != 1 || series[0].Active != 0 {
		t.Fatalf("expected redis-only zero bucket, got %+v", series)
	}
}

func TestActiveSeries_MergesMonthlyRedisWithDBBackfill(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openActiveSeriesTestDB(t)
	month1 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	month2 := time.Date(2025, 2, 15, 10, 0, 0, 0, time.UTC)
	if err := db.Create(&[]model.Log{
		{ProjectID: 1, Timestamp: month1, DistinctID: "u1", Level: "info", Message: "a", Fields: datatypes.JSON([]byte("{}"))},
		{ProjectID: 1, Timestamp: month1.Add(time.Hour), DistinctID: "u2", Level: "info", Message: "b", Fields: datatypes.JSON([]byte("{}"))},
	}).Error; err != nil {
		t.Fatalf("insert logs: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	rec := metrics.NewRedisRecorder(rdb)
	rec.ObserveLog(ctx, 1, "info", "u3", "d3", month2)

	series, err := activeSeries(ctx, rec, db, 1, month1, month2, "month")
	if err != nil {
		t.Fatalf("activeSeries month: %v", err)
	}
	if len(series) != 2 {
		t.Fatalf("expected 2 buckets, got %+v", series)
	}
	if series[0].Bucket != "2025-01" || series[0].Active != 2 {
		t.Fatalf("unexpected first month: %+v", series[0])
	}
	if series[1].Bucket != "2025-02" || series[1].Active != 1 {
		t.Fatalf("unexpected second month: %+v", series[1])
	}
}
