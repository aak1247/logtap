package store

import (
	"context"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSetDefaultProject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Project{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	p1 := model.Project{OwnerUserID: 1, Name: "p1"}
	p2 := model.Project{OwnerUserID: 1, Name: "p2"}
	if err := db.WithContext(ctx).Create(&p1).Error; err != nil {
		t.Fatalf("create p1: %v", err)
	}
	if err := db.WithContext(ctx).Create(&p2).Error; err != nil {
		t.Fatalf("create p2: %v", err)
	}

	if err := SetDefaultProject(ctx, db, 1, p2.ID); err != nil {
		t.Fatalf("set default: %v", err)
	}

	var got1, got2 model.Project
	if err := db.WithContext(ctx).First(&got1, p1.ID).Error; err != nil {
		t.Fatalf("load p1: %v", err)
	}
	if err := db.WithContext(ctx).First(&got2, p2.ID).Error; err != nil {
		t.Fatalf("load p2: %v", err)
	}
	if got1.IsDefault {
		t.Fatalf("expected p1 not default")
	}
	if !got2.IsDefault {
		t.Fatalf("expected p2 default")
	}
}
