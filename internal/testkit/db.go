package testkit

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/aak1247/logtap/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenTestDB returns an in-memory sqlite database with schema migrated.
// It's meant as a "replaceable DB" for integration tests (no Postgres required).
func OpenTestDB(t testing.TB) *gorm.DB {
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
