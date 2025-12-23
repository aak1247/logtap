package migrate

import (
	"context"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

func AutoMigrate(ctx context.Context, db *gorm.DB) error {
	gdb := db.WithContext(ctx)
	if err := gdb.AutoMigrate(&model.User{}, &model.Project{}, &model.ProjectKey{}, &model.Event{}, &model.Log{}); err != nil {
		return err
	}

	// GIN indexes for JSONB.
	if err := gdb.Exec(`CREATE INDEX IF NOT EXISTS idx_events_data ON events USING GIN (data)`).Error; err != nil {
		return err
	}
	if err := gdb.Exec(`CREATE INDEX IF NOT EXISTS idx_logs_fields ON logs USING GIN (fields)`).Error; err != nil {
		return err
	}

	// Full text search index for /logs/search?q=... (expression index; no schema init needed).
	if err := gdb.Exec(`
		CREATE INDEX IF NOT EXISTS idx_logs_search_expr
		ON logs USING GIN (to_tsvector('simple', coalesce(message,'') || ' ' || coalesce(fields::text,'')))
	`).Error; err != nil {
		return err
	}

	if err := gdb.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_project_keys_project_name ON project_keys (project_id, name)`).Error; err != nil {
		return err
	}

	return nil
}
