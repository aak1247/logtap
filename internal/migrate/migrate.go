package migrate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aak1247/logtap/internal/detector"
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
		&model.UserFirstSeen{},
		&model.ProjectCounter{},
		&model.LogDailyStat{},
		&model.CleanupPolicy{},
		&model.EventDefinition{},
		&model.PropertyDefinition{},
		&model.AnalysisView{},
		&model.PluginPackageSetting{},

		// Alerting (optional feature; safe to have tables even if unused).
		&model.AlertContact{},
		&model.AlertContactGroup{},
		&model.AlertContactGroupMember{},
		&model.AlertWecomBot{},
		&model.AlertWebhookEndpoint{},
		&model.AlertRule{},
		&model.AlertState{},
		&model.AlertDelivery{},
		&model.MonitorDefinition{},
		&model.MonitorRun{},
	); err != nil {
		return err
	}

	// Detector result persistence (handled by detector package, but also migrate here for consistency).
	if err := gdb.AutoMigrate(&detector.DetectorResult{}); err != nil {
		return fmt.Errorf("auto migrate detector_results: %w", err)
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

	if strings.EqualFold(db.Dialector.Name(), "postgres") {
		if err := ensureAlertStateUniqueIndex(gdb); err != nil {
			return err
		}
	}

	if err := backfillUserFirstSeenIfEmpty(gdb); err != nil {
		return err
	}
	if err := backfillMetricsRollupsIfEmpty(gdb); err != nil {
		return err
	}

	if strings.EqualFold(db.Dialector.Name(), "postgres") {
		if err := ensureTimescaleCompatiblePrimaryKeys(gdb); err != nil {
			return err
		}
		if err := ensureTimescaleCompatibleUniqueIndex(gdb, "logs", "idx_logs_dedupe", []string{"project_id", "ingest_id", "timestamp"}); err != nil {
			return err
		}
		if err := ensureTimescaleCompatibleUniqueIndex(gdb, "track_events", "idx_track_events_dedupe", []string{"project_id", "ingest_id", "timestamp"}); err != nil {
			return err
		}
		if err := ensureTimescaleHypertables(gdb, opts.RequireTimescale, timescaleInstalled); err != nil {
			return err
		}
	}

	return nil
}

func ensureAlertStateUniqueIndex(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			WITH keep AS (
				SELECT
					rule_id,
					key_hash,
					min(id) AS keep_id,
					max(occurrences) AS occurrences,
					max(backoff_exp) AS backoff_exp,
					max(last_seen_at) AS last_seen_at,
					max(last_sent_at) AS last_sent_at,
					max(next_allowed_at) AS next_allowed_at,
					min(created_at) AS created_at,
					max(updated_at) AS updated_at
				FROM alert_states
				GROUP BY rule_id, key_hash
				HAVING count(*) > 1
			)
			UPDATE alert_states AS s
			SET
				occurrences = keep.occurrences,
				backoff_exp = keep.backoff_exp,
				last_seen_at = keep.last_seen_at,
				last_sent_at = keep.last_sent_at,
				next_allowed_at = keep.next_allowed_at,
				created_at = keep.created_at,
				updated_at = keep.updated_at
			FROM keep
			WHERE s.id = keep.keep_id
		`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			WITH keep AS (
				SELECT rule_id, key_hash, min(id) AS keep_id
				FROM alert_states
				GROUP BY rule_id, key_hash
				HAVING count(*) > 1
			)
			DELETE FROM alert_states AS s
			USING keep
			WHERE s.rule_id = keep.rule_id
				AND s.key_hash = keep.key_hash
				AND s.id <> keep.keep_id
		`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DROP INDEX IF EXISTS idx_alert_states_rule_key`).Error; err != nil {
			return err
		}
		return tx.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_states_rule_key
			ON alert_states (rule_id, key_hash)
		`).Error
	})
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
	if err := db.Exec(`SELECT create_hypertable('events', 'timestamp', if_not_exists => TRUE, migrate_data => TRUE)`).Error; err != nil {
		if require {
			return fmt.Errorf("create_hypertable events: %w", err)
		}
	}
	if err := db.Exec(`SELECT create_hypertable('logs', 'timestamp', if_not_exists => TRUE, migrate_data => TRUE)`).Error; err != nil {
		if require {
			return fmt.Errorf("create_hypertable logs: %w", err)
		}
	}
	if err := db.Exec(`SELECT create_hypertable('track_events', 'timestamp', if_not_exists => TRUE, migrate_data => TRUE)`).Error; err != nil {
		if require {
			return fmt.Errorf("create_hypertable track_events: %w", err)
		}
	}
	return nil
}

func ensureTimescaleCompatiblePrimaryKeys(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	definitions := []struct {
		table      string
		constraint string
		columns    []string
	}{
		{table: "events", constraint: "events_pkey", columns: []string{"id", "timestamp"}},
		{table: "logs", constraint: "logs_pkey", columns: []string{"id", "timestamp"}},
		{table: "track_events", constraint: "track_events_pkey", columns: []string{"id", "timestamp"}},
	}
	for _, def := range definitions {
		matched, err := primaryKeyMatches(db, def.table, def.constraint, def.columns)
		if err != nil {
			return err
		}
		if matched {
			continue
		}
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s`, def.table, def.constraint)).Error; err != nil {
			return err
		}
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s PRIMARY KEY (%s)`, def.table, def.constraint, strings.Join(def.columns, ", "))).Error; err != nil {
			return err
		}
	}
	return nil
}

func primaryKeyMatches(db *gorm.DB, table string, constraint string, columns []string) (bool, error) {
	var got string
	err := db.Raw(`
		SELECT COALESCE(array_to_string(array_agg(a.attname ORDER BY k.ord), ','), '')
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord) ON TRUE
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
		WHERE c.contype = 'p' AND t.relname = ? AND c.conname = ?
	`, table, constraint).Scan(&got).Error
	if err != nil {
		return false, err
	}
	return got == strings.Join(columns, ","), nil
}

func ensureTimescaleCompatibleUniqueIndex(db *gorm.DB, table string, index string, columns []string) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if strings.TrimSpace(table) == "" || strings.TrimSpace(index) == "" || len(columns) == 0 {
		return errors.New("invalid unique index definition")
	}
	matched, err := uniqueIndexMatches(db, table, index, columns)
	if err != nil {
		return err
	}
	if matched {
		return nil
	}
	if err := db.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, index)).Error; err != nil {
		return err
	}
	return db.Exec(fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s)`, index, table, strings.Join(columns, ", "))).Error
}

func uniqueIndexMatches(db *gorm.DB, table string, index string, columns []string) (bool, error) {
	var got string
	err := db.Raw(`
		SELECT COALESCE(array_to_string(array_agg(a.attname ORDER BY k.ord), ','), '')
		FROM pg_class i
		JOIN pg_index ix ON ix.indexrelid = i.oid
		JOIN pg_class t ON t.oid = ix.indrelid
		JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord) ON k.attnum > 0
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
		WHERE i.relname = ? AND t.relname = ? AND ix.indisunique
	`, index, table).Scan(&got).Error
	if err != nil {
		return false, err
	}
	return got == strings.Join(columns, ","), nil
}

func backfillUserFirstSeenIfEmpty(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if !db.Migrator().HasTable("user_first_seen") {
		return nil
	}

	var existing int
	if err := db.Raw("SELECT 1 FROM user_first_seen LIMIT 1").Scan(&existing).Error; err != nil {
		return fmt.Errorf("check user_first_seen: %w", err)
	}
	if existing == 1 {
		return nil
	}

	parts := []string{}
	if db.Migrator().HasTable("logs") {
		parts = append(parts, "SELECT project_id, distinct_id, timestamp FROM logs WHERE distinct_id IS NOT NULL AND distinct_id <> ''")
	}
	if db.Migrator().HasTable("track_events") {
		parts = append(parts, "SELECT project_id, distinct_id, timestamp FROM track_events WHERE distinct_id IS NOT NULL AND distinct_id <> ''")
	}
	if db.Migrator().HasTable("events") {
		parts = append(parts, "SELECT project_id, distinct_id, timestamp FROM events WHERE distinct_id IS NOT NULL AND distinct_id <> ''")
	}
	if len(parts) == 0 {
		return nil
	}

	sql := `
		INSERT INTO user_first_seen (project_id, distinct_id, first_seen, updated_at)
		SELECT project_id, distinct_id, MIN(timestamp), NOW()
		FROM (` + strings.Join(parts, " UNION ALL ") + `) src
		GROUP BY project_id, distinct_id
		ON CONFLICT (project_id, distinct_id) DO UPDATE
		SET first_seen = LEAST(user_first_seen.first_seen, EXCLUDED.first_seen),
		    updated_at = NOW()
	`
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("backfill user_first_seen: %w", err)
	}
	return nil
}

func backfillMetricsRollupsIfEmpty(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if !strings.EqualFold(db.Dialector.Name(), "postgres") {
		return nil
	}
	if !db.Migrator().HasTable("project_counters") || !db.Migrator().HasTable("log_daily_stats") {
		return nil
	}

	var existing int
	if err := db.Raw("SELECT 1 FROM project_counters LIMIT 1").Scan(&existing).Error; err != nil {
		return fmt.Errorf("check project_counters: %w", err)
	}
	if existing == 1 {
		return nil
	}

	if db.Migrator().HasTable("logs") {
		if err := db.Exec(`
			INSERT INTO project_counters (project_id, metric, value, updated_at)
			SELECT project_id, 'logs_total', COUNT(*), NOW()
			FROM logs
			GROUP BY project_id
			ON CONFLICT (project_id, metric) DO UPDATE
			SET value = EXCLUDED.value,
			    updated_at = NOW()
		`).Error; err != nil {
			return fmt.Errorf("backfill log counters: %w", err)
		}
		if err := db.Exec(`
			INSERT INTO log_daily_stats (project_id, day, kind, level, count, updated_at)
			SELECT project_id,
			       to_char((timestamp AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS day,
			       'log' AS kind,
			       COALESCE(NULLIF(lower(trim(level)), ''), 'unknown') AS level,
			       COUNT(*) AS count,
			       NOW()
			FROM logs
			GROUP BY project_id, day, level
			ON CONFLICT (project_id, day, kind, level) DO UPDATE
			SET count = EXCLUDED.count,
			    updated_at = NOW()
		`).Error; err != nil {
			return fmt.Errorf("backfill log daily stats: %w", err)
		}
	}

	if db.Migrator().HasTable("track_events") {
		if err := db.Exec(`
			INSERT INTO project_counters (project_id, metric, value, updated_at)
			SELECT project_id, 'events_total', COUNT(*), NOW()
			FROM track_events
			GROUP BY project_id
			ON CONFLICT (project_id, metric) DO UPDATE
			SET value = EXCLUDED.value,
			    updated_at = NOW()
		`).Error; err != nil {
			return fmt.Errorf("backfill event counters: %w", err)
		}
		if err := db.Exec(`
			INSERT INTO log_daily_stats (project_id, day, kind, level, count, updated_at)
			SELECT project_id,
			       to_char((timestamp AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS day,
			       'event' AS kind,
			       'event' AS level,
			       COUNT(*) AS count,
			       NOW()
			FROM track_events
			GROUP BY project_id, day
			ON CONFLICT (project_id, day, kind, level) DO UPDATE
			SET count = EXCLUDED.count,
			    updated_at = NOW()
		`).Error; err != nil {
			return fmt.Errorf("backfill event daily stats: %w", err)
		}
	}
	return nil
}
