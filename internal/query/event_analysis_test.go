package query

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite): %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("gdb.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := gdb.AutoMigrate(&model.TrackEvent{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return gdb
}

func TestTopEventsFromTrackEvents_EmptyRange_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)

	start := time.Now().UTC().Add(-1 * time.Hour)
	end := time.Now().UTC()

	var out []TopEventRow
	if err := topEventsFromTrackEvents(context.Background(), db, 1, start, end, 20, "", &out); err != nil {
		t.Fatalf("topEventsFromTrackEvents: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result, got %d rows", len(out))
	}
}
