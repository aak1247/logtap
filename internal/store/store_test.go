package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/model"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestDB(t *testing.T) *gorm.DB {
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

	if err := gdb.AutoMigrate(&model.User{}, &model.Project{}, &model.ProjectKey{}, &model.Event{}, &model.Log{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return gdb
}

func TestUsersCRUD(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	n, err := CountUsers(ctx, db)
	if err != nil || n != 0 {
		t.Fatalf("CountUsers: n=%d err=%v", n, err)
	}

	uid, err := CreateUser(ctx, db, "  A@B.COM  ", "hash")
	if err != nil || uid <= 0 {
		t.Fatalf("CreateUser: uid=%d err=%v", uid, err)
	}

	n, err = CountUsers(ctx, db)
	if err != nil || n != 1 {
		t.Fatalf("CountUsers: n=%d err=%v", n, err)
	}

	u, ok, err := GetUserByEmail(ctx, db, "a@b.com")
	if err != nil || !ok || u.ID != uid || u.Email != "a@b.com" {
		t.Fatalf("GetUserByEmail: u=%+v ok=%v err=%v", u, ok, err)
	}
	u2, ok, err := GetUserByID(ctx, db, uid)
	if err != nil || !ok || u2.ID != uid {
		t.Fatalf("GetUserByID: u=%+v ok=%v err=%v", u2, ok, err)
	}

	if _, ok, err := GetUserByEmail(ctx, db, "missing@x"); err != nil || ok {
		t.Fatalf("expected missing user to be (ok=false, err=nil), got ok=%v err=%v", ok, err)
	}
}

func TestProjectsAndKeys(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	uid, err := CreateUser(ctx, db, "a@b.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	p, err := CreateProject(ctx, db, uid, "  My Project  ")
	if err != nil || p.ID <= 0 || p.OwnerUserID != uid {
		t.Fatalf("CreateProject: p=%+v err=%v", p, err)
	}
	list, err := ListProjectsByOwner(ctx, db, uid)
	if err != nil || len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("ListProjectsByOwner: items=%v err=%v", list, err)
	}

	k, err := CreateProjectKey(ctx, db, p.ID, "  default  ")
	if err != nil || k.ID <= 0 || k.ProjectID != p.ID || k.Key == "" {
		t.Fatalf("CreateProjectKey: k=%+v err=%v", k, err)
	}
	ok, err := ValidateProjectKey(ctx, db, p.ID, k.Key)
	if err != nil || !ok {
		t.Fatalf("ValidateProjectKey: ok=%v err=%v", ok, err)
	}
	items, err := ListProjectKeys(ctx, db, p.ID)
	if err != nil || len(items) != 1 || items[0].ID != k.ID {
		t.Fatalf("ListProjectKeys: items=%v err=%v", items, err)
	}

	revoked, err := RevokeProjectKey(ctx, db, p.ID, k.ID, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || !revoked {
		t.Fatalf("RevokeProjectKey: revoked=%v err=%v", revoked, err)
	}
	ok, err = ValidateProjectKey(ctx, db, p.ID, k.Key)
	if err != nil || ok {
		t.Fatalf("expected revoked key to be invalid, ok=%v err=%v", ok, err)
	}
}

func TestInsertLog_DistinctIDSelection(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Prefer user.id
	lp := ingest.CustomLogPayload{
		Level:     "info",
		Message:   "hello",
		DeviceID:  "d1",
		Timestamp: &ts,
		User:      map[string]any{"id": "u1"},
		Fields:    map[string]any{"device_id": "d2"},
	}
	if err := InsertLog(ctx, db, "1", lp); err != nil {
		t.Fatalf("InsertLog: %v", err)
	}
	var row model.Log
	if err := db.WithContext(ctx).First(&row).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if row.DistinctID != "u1" || row.DeviceID != "d1" {
		t.Fatalf("unexpected distinct/device: %q/%q", row.DistinctID, row.DeviceID)
	}

	// Missing timestamp is an error.
	lp2 := ingest.CustomLogPayload{Message: "x"}
	if err := InsertLog(ctx, db, "1", lp2); err == nil {
		t.Fatalf("expected error for missing timestamp")
	}
}

func TestInsertEvent_StableEventID_AndTitle(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)

	ev := map[string]any{
		"event_id":    "not-a-uuid",
		"timestamp":   ts,
		"level":       "error",
		"message":     "",
		"exception":   map[string]any{"values": []any{map[string]any{"type": "Boom", "value": "bad"}}},
		"environment": "prod",
		"user":        map[string]any{"id": "u1"},
	}
	if err := InsertEvent(ctx, db, "1", ev); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	wantID := uuid.NewSHA1(uuid.Nil, []byte("not-a-uuid"))
	var row model.Event
	if err := db.WithContext(ctx).Where("id = ?", wantID.String()).First(&row).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if row.ID != wantID {
		t.Fatalf("expected id %v, got %v", wantID, row.ID)
	}
	if row.Title != "Boom: bad" {
		t.Fatalf("expected title from exception, got %q", row.Title)
	}
	if string(row.Data) == "" || string(row.Data) == "null" {
		t.Fatalf("expected data json")
	}
}

func TestParseSentryTimestamp(t *testing.T) {
	t.Parallel()

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	if got := parseSentryTimestamp(base.Format(time.RFC3339Nano)); !got.Equal(base) {
		t.Fatalf("expected %v, got %v", base, got)
	}
	if got := parseSentryTimestamp(float64(base.Unix()) + 0.5); got.Unix() != base.Unix() || got.Nanosecond() == 0 {
		t.Fatalf("expected fractional timestamp, got %v", got)
	}
	if got := parseSentryTimestamp(int64(base.Unix())); got.Unix() != base.Unix() {
		t.Fatalf("expected unix %d, got %d", base.Unix(), got.Unix())
	}
	if got := parseSentryTimestamp(json.Number(fmt.Sprintf("%d.25", base.Unix()))); got.Unix() != base.Unix() {
		t.Fatalf("expected unix %d, got %d", base.Unix(), got.Unix())
	}
}

