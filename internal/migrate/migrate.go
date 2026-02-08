package migrate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

type Options struct {
	RequireTimescale bool
}

func AutoMigrate(ctx context.Context, db *gorm.DB, opts Options) error {
	gdb := db.WithContext(ctx)
	timescaleInstalled := false
	if strings.EqualFold(db.Dialector.Name(), "postgres") {
		installed, err := ensureTimescaleExtension(gdb, opts.RequireTimescale)
		if err != nil {
			return err
		}
		timescaleInstalled = installed
	}
	if err := gdb.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.ProjectKey{},
		&model.Event{},
		&model.Log{},
		&model.TrackEvent{},
		&model.TrackEventDaily{},
		&model.CleanupPolicy{},

		// Alerting (optional feature; safe to have tables even if unused).
		&model.AlertContact{},
		&model.AlertContactGroup{},
		&model.AlertContactGroupMember{},
		&model.AlertWecomBot{},
		&model.AlertWebhookEndpoint{},
		&model.AlertRule{},
		&model.AlertState{},
		&model.AlertDelivery{},
	); err != nil {
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

	// Idempotency for logs: dedupe retries by a stable ingest_id (e.g. NSQ message id).
	if err := gdb.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_logs_dedupe ON logs (project_id, ingest_id)`).Error; err != nil {
		return err
	}

	// Idempotency for track events: derived from logs, keyed by ingest_id.
	if err := gdb.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_track_events_dedupe ON track_events (project_id, ingest_id)`).Error; err != nil {
		return err
	}

	if err := gdb.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_project_keys_project_name ON project_keys (project_id, name)`).Error; err != nil {
		return err
	}

	if strings.EqualFold(db.Dialector.Name(), "postgres") {
		if err := ensureTimescaleHypertables(gdb, opts.RequireTimescale, timescaleInstalled); err != nil {
			return err
		}
	}

	return nil
}

func ensureTimescaleExtension(db *gorm.DB, require bool) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}

	// Best-effort: if TimescaleDB isn't installed/enabled, fall back to plain tables.
	var available int
	if err := db.Raw(`SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb' LIMIT 1`).Scan(&available).Error; err != nil {
		if require {
			return false, fmt.Errorf("timescaledb extension availability check failed: %w", err)
		}
		return false, nil
	}
	if available != 1 {
		if require {
			return false, errors.New("timescaledb extension not available on this postgres")
		}
		return false, nil
	}

	// If permission is missing, ignore and continue without hypertables.
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS timescaledb`).Error; err != nil {
		if require {
			return false, fmt.Errorf("enable timescaledb extension: %w", err)
		}
		return false, nil
	}

	var installed int
	if err := db.Raw(`SELECT 1 FROM pg_extension WHERE extname = 'timescaledb' LIMIT 1`).Scan(&installed).Error; err != nil {
		if require {
			return false, fmt.Errorf("timescaledb extension installed check failed: %w", err)
		}
		return false, nil
	}
	if installed != 1 {
		if require {
			return false, errors.New("timescaledb extension not installed")
		}
		return false, nil
	}

	return true, nil
}

func ensureTimescaleHypertables(db *gorm.DB, require bool, timescaleInstalled bool) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if !timescaleInstalled {
		if require {
			return errors.New("timescaledb required but extension is not installed")
		}
		return nil
	}

	// Make hypertables if possible (idempotent).
	if err := db.Exec(`SELECT create_hypertable('events', 'timestamp', if_not_exists => TRUE)`).Error; err != nil {
		if require {
			return fmt.Errorf("create_hypertable events: %w", err)
		}
	}
	if err := db.Exec(`SELECT create_hypertable('logs', 'timestamp', if_not_exists => TRUE)`).Error; err != nil {
		if require {
			return fmt.Errorf("create_hypertable logs: %w", err)
		}
	}
	if err := db.Exec(`SELECT create_hypertable('track_events', 'timestamp', if_not_exists => TRUE)`).Error; err != nil {
		if require {
			return fmt.Errorf("create_hypertable track_events: %w", err)
		}
	}
	return nil
}
