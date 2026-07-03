package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/model"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNewRedisClient(t *testing.T) {
	t.Parallel()

	if _, err := NewRedisClient("", "", 0); err == nil {
		t.Fatalf("expected error for empty addr")
	}

	mr := miniredis.RunT(t)
	rdb, err := NewRedisClient(mr.Addr(), "", 0)
	if err != nil {
		t.Fatalf("NewRedisClient: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestRedisRecorder_Today_Active_Distribution_Retention(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	rec := NewRedisRecorder(rdb)
	ctx := context.Background()

	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	day1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)

	rec.ObserveEvent(ctx, 1, "error", "u1", "d1", "iOS", day2)
	rec.ObserveEventDist(ctx, 1, day2, "u1", map[string]string{"os": "iOS", "browser": "Chrome"})
	rec.ObserveEventDist(ctx, 1, day2, "u1", map[string]string{"os": "iOS", "browser": "Chrome"})
	rec.ObserveLog(ctx, 1, "info", "u2", "d2", day2)

	logs, events, errorsCount, users, ok, err := rec.Today(ctx, 1, now)
	if err != nil || !ok {
		t.Fatalf("Today: logs=%d events=%d errors=%d users=%d ok=%v err=%v", logs, events, errorsCount, users, ok, err)
	}
	if events != 1 || errorsCount != 1 {
		t.Fatalf("expected events=1 errors=1, got %d/%d", events, errorsCount)
	}
	if logs != 1 {
		t.Fatalf("expected logs=1, got %d", logs)
	}
	if users < 1 {
		t.Fatalf("expected users>=1, got %d", users)
	}

	// Active series (day bucket).
	series, err := rec.ActiveSeries(ctx, 1, day1, day2, "day")
	if err != nil {
		t.Fatalf("ActiveSeries(day): %v", err)
	}
	if len(series) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(series))
	}

	// Distribution aggregation and ordering.
	items, err := rec.Distribution(ctx, 1, "os", day1, day2, 10)
	if err != nil {
		t.Fatalf("Distribution: %v", err)
	}
	if len(items) != 1 || items[0].Key != "iOS" || items[0].Count != 2 {
		t.Fatalf("unexpected dist items: %+v", items)
	}
	rec.ObserveEventDist(ctx, 1, day1, "u1", map[string]string{"os": "iOS"})
	userItems, err := rec.DistributionMetric(ctx, 1, "os", day1, day2, 10, "users")
	if err != nil {
		t.Fatalf("DistributionMetric(users): %v", err)
	}
	if len(userItems) != 1 || userItems[0].Key != "iOS" || userItems[0].Count != 1 {
		t.Fatalf("unexpected user dist items: %+v", userItems)
	}
	buckets, err := rec.DistributionSeries(ctx, 1, "os", day1, day2, "day", 10, "users")
	if err != nil {
		t.Fatalf("DistributionSeries(users): %v", err)
	}
	if len(buckets) != 2 || len(buckets[1].Items) != 1 || buckets[1].Items[0].Count != 1 {
		t.Fatalf("unexpected user dist series: %+v", buckets)
	}

	// Retention for cohort day1 (u1 active on day1 and day2).
	rec.ObserveEvent(ctx, 1, "info", "u1", "", "", day1)
	rows, err := rec.Retention(ctx, 1, day1, day1, []int{1})
	if err != nil {
		t.Fatalf("Retention: %v", err)
	}
	if len(rows) != 1 || rows[0].CohortSize != 1 {
		t.Fatalf("unexpected retention rows: %+v", rows)
	}
	if len(rows[0].Points) != 1 || rows[0].Points[0].Day != 1 {
		t.Fatalf("unexpected retention points: %+v", rows[0].Points)
	}
	if rows[0].Points[0].Active != 1 || rows[0].Points[0].Rate != 1.0 {
		t.Fatalf("unexpected retention point: %+v", rows[0].Points[0])
	}
}

func TestRedisRecorder_WarmActiveUsersFromDB(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite): %v", err)
	}
	if err := gdb.AutoMigrate(&model.Log{}, &model.Event{}, &model.TrackEvent{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	now := time.Now().UTC()
	day1 := now.AddDate(0, 0, -2)
	day2 := now.AddDate(0, 0, -1)
	if err := gdb.Create(&[]model.Log{
		{ProjectID: 1, Timestamp: day1, DistinctID: "u1", Level: "info", Message: "a", Fields: datatypes.JSON([]byte("{}"))},
		{ProjectID: 1, Timestamp: day1.Add(time.Hour), DistinctID: "u1", Level: "info", Message: "b", Fields: datatypes.JSON([]byte("{}"))},
		{ProjectID: 1, Timestamp: day2, DistinctID: "u2", Level: "info", Message: "c", Fields: datatypes.JSON([]byte("{}"))},
	}).Error; err != nil {
		t.Fatalf("insert logs: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	rec := NewRedisRecorder(rdb)

	if err := rdb.Set(ctx, activeWarmupDayReadyKey(now), 30, time.Hour).Err(); err != nil {
		t.Fatalf("set ready day: %v", err)
	}
	if err := rdb.Set(ctx, activeWarmupMonthReadyKey(now), 6, time.Hour).Err(); err != nil {
		t.Fatalf("set ready month: %v", err)
	}
	if err := rec.WarmActiveUsersFromDB(ctx, gdb, ActiveWarmupOptions{Days: 3, Months: 2, BatchSize: 2, Now: now}); err != nil {
		t.Fatalf("WarmActiveUsersFromDB: %v", err)
	}

	daySeries, err := rec.ActiveSeries(ctx, 1, day1, day2, "day")
	if err != nil {
		t.Fatalf("ActiveSeries: %v", err)
	}
	if len(daySeries) != 2 || daySeries[0].Active != 1 || daySeries[1].Active != 1 {
		t.Fatalf("unexpected day series: %+v", daySeries)
	}
	monthSeries, err := rec.ActiveSeries(ctx, 1, day1, day2, "month")
	if err != nil {
		t.Fatalf("ActiveSeries(month): %v", err)
	}
	if len(monthSeries) != 1 || monthSeries[0].Active != 2 {
		t.Fatalf("unexpected month series: %+v", monthSeries)
	}
	if !rec.ActiveWarmupCovers(ctx, day1, day2, "day") {
		t.Fatalf("expected warmup to cover %s - %s", day1, day2)
	}
	if !rec.ActiveWarmupCovers(ctx, day1, day2, "month") {
		t.Fatalf("expected warmup to cover month range %s - %s", day1, day2)
	}
}
