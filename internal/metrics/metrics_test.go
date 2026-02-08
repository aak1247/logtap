package metrics

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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
	rec.ObserveEventDist(ctx, 1, day2, map[string]string{"os": "iOS", "browser": "Chrome"})
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
	if len(items) != 1 || items[0].Key != "iOS" || items[0].Count != 1 {
		t.Fatalf("unexpected dist items: %+v", items)
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
