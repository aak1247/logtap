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

func TestMetricsUseDBBeforeEmptyRedis(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openActiveSeriesTestDB(t)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := db.Create(&[]model.Log{
		{ProjectID: 1, Timestamp: now, DistinctID: "u1", Level: "error", Message: "boom", Fields: datatypes.JSON([]byte("{}"))},
		{ProjectID: 1, Timestamp: now.Add(time.Minute), DistinctID: "u2", Level: "event", Message: "signup", Fields: datatypes.JSON([]byte("{}"))},
	}).Error; err != nil {
		t.Fatalf("insert logs: %v", err)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	rec := metrics.NewRedisRecorder(rdb)

	logs, events, errorsCount, users, ok, err := metricsToday(ctx, rec, db, 1, now)
	if err != nil || !ok {
		t.Fatalf("metricsToday ok=%v err=%v", ok, err)
	}
	if logs != 2 || events != 1 || errorsCount != 1 || users != 2 {
		t.Fatalf("unexpected today metrics: logs=%d events=%d errors=%d users=%d", logs, events, errorsCount, users)
	}

	totalLogs, totalEvents, totalUsers, ok, err := metricsTotal(ctx, rec, db, 1)
	if err != nil || !ok {
		t.Fatalf("metricsTotal ok=%v err=%v", ok, err)
	}
	if totalLogs != 2 || totalEvents != 1 || totalUsers != 2 {
		t.Fatalf("unexpected total metrics: logs=%d events=%d users=%d", totalLogs, totalEvents, totalUsers)
	}
}
