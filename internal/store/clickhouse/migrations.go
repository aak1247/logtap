package clickhouse

import (
	"context"
	"fmt"
	"log"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ddlStatement is one idempotent migration step. Optional statements (data
// skipping indices that depend on newer ClickHouse features) are logged and
// skipped on failure instead of aborting startup.
type ddlStatement struct {
	sql      string
	optional bool
}

// Migrate applies the ClickHouse schema (design §4) idempotently. logTTLDays
// controls the raw data retention (open-source single-table mode; enterprise
// tier tables are not created here). The open-source edition creates the
// plain MergeTree layout with a fixed DefaultTenantID column and no
// per-tenant quota objects.
func Migrate(ctx context.Context, conn driver.Conn, logTTLDays int) error {
	for i, stmt := range ddlStatements(logTTLDays) {
		if err := conn.Exec(ctx, stmt.sql); err != nil {
			if stmt.optional {
				log.Printf("clickhouse migrate: optional statement %d skipped: %v", i, err)
				continue
			}
			return fmt.Errorf("clickhouse migrate statement %d: %w", i, err)
		}
	}
	return nil
}

func ddlStatements(logTTLDays int) []ddlStatement {
	const defaultTenant = "00000000-0000-0000-0000-000000000001"
	stmts := []ddlStatement{
		{
			sql: fmt.Sprintf(`CREATE TABLE IF NOT EXISTS logtap.logs (
	tenant_id UUID DEFAULT '%s',
	project_id UInt32,
	timestamp DateTime64(3, 'UTC') CODEC(Delta(8), ZSTD(1)),
	ingest_id UUID,
	level LowCardinality(String) DEFAULT '',
	message String CODEC(ZSTD(1)),
	fields Map(LowCardinality(String), String) CODEC(ZSTD(1)),
	trace_id String DEFAULT '',
	span_id String DEFAULT '',
	distinct_id String DEFAULT '',
	device_id String DEFAULT ''
) ENGINE = MergeTree
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, project_id, toStartOfFiveMinutes(timestamp), timestamp)
TTL timestamp + INTERVAL %d DAY
SETTINGS ttl_only_drop_parts = 1, index_granularity = 8192, non_replicated_deduplication_window = 1000`, defaultTenant, logTTLDays),
		},
		{
			// Full-text index over message (jieba tokenizer, ClickHouse
			// 26.2+). Optional: older servers fall back to column scan.
			sql:      fmt.Sprintf("ALTER TABLE logtap.logs ADD INDEX IF NOT EXISTS idx_message_text message TYPE text(tokenizer = 'jieba') GRANULARITY 1"),
			optional: true,
		},
		{
			sql:      "ALTER TABLE logtap.logs ADD INDEX IF NOT EXISTS idx_trace trace_id TYPE bloom_filter(0.01) GRANULARITY 4",
			optional: true,
		},
		{
			sql: fmt.Sprintf(`CREATE TABLE IF NOT EXISTS logtap.events (
	tenant_id UUID DEFAULT '%s',
	project_id UInt32,
	timestamp DateTime64(3, 'UTC') CODEC(Delta(8), ZSTD(1)),
	event_id UUID,
	level LowCardinality(String) DEFAULT '',
	title String DEFAULT '' CODEC(ZSTD(1)),
	distinct_id String DEFAULT '',
	device_id String DEFAULT '',
	os LowCardinality(String) DEFAULT '',
	platform LowCardinality(String) DEFAULT '',
	release_tag LowCardinality(String) DEFAULT '',
	environment LowCardinality(String) DEFAULT '',
	user_id String DEFAULT '',
	data String CODEC(ZSTD(3))
) ENGINE = MergeTree
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, project_id, toStartOfFiveMinutes(timestamp), timestamp)
TTL timestamp + INTERVAL %d DAY
SETTINGS ttl_only_drop_parts = 1`, defaultTenant, logTTLDays),
		},
		{
			sql:      "ALTER TABLE logtap.events ADD INDEX IF NOT EXISTS idx_event_id event_id TYPE bloom_filter(0.005) GRANULARITY 4",
			optional: true,
		},
		{
			sql: fmt.Sprintf(`CREATE TABLE IF NOT EXISTS logtap.track_events (
	tenant_id UUID DEFAULT '%s',
	project_id UInt32,
	timestamp DateTime64(3, 'UTC') CODEC(Delta(8), ZSTD(1)),
	ingest_id UUID,
	name String CODEC(ZSTD(1)),
	distinct_id String DEFAULT '',
	device_id String DEFAULT ''
) ENGINE = MergeTree
PARTITION BY toDate(timestamp)
ORDER BY (tenant_id, project_id, name, timestamp)
TTL timestamp + INTERVAL %d DAY
SETTINGS ttl_only_drop_parts = 1, non_replicated_deduplication_window = 1000`, defaultTenant, logTTLDays),
		},
		{
			sql:      "ALTER TABLE logtap.track_events ADD INDEX IF NOT EXISTS idx_name name TYPE bloom_filter(0.01) GRANULARITY 4",
			optional: true,
		},
		{
			// Daily log statistics rollup feeding /metrics and /logs/trend
			// day buckets. Kept 18 months (~540 days), matching the PG
			// metrics month TTL.
			sql: `CREATE TABLE IF NOT EXISTS logtap.log_daily_stats (
	tenant_id UUID,
	project_id UInt32,
	day Date,
	level LowCardinality(String),
	cnt AggregateFunction(count, UInt64),
	uniq_users AggregateFunction(uniqExact, String)
) ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, project_id, day, level)
TTL day + INTERVAL 540 DAY`,
		},
		{
			sql: `CREATE MATERIALIZED VIEW IF NOT EXISTS logtap.logs_daily_mv
TO logtap.log_daily_stats
AS SELECT
	tenant_id, project_id,
	toDate(timestamp) AS day,
	level,
	countState() AS cnt,
	uniqExactState(distinct_id) AS uniq_users
FROM logtap.logs
GROUP BY tenant_id, project_id, day, level`,
		},
		{
			sql: `CREATE TABLE IF NOT EXISTS logtap.track_event_daily (
	tenant_id UUID,
	project_id UInt32,
	day Date,
	name String,
	cnt AggregateFunction(count, UInt64),
	uniq_users AggregateFunction(uniqExact, String)
) ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, project_id, day, name)
TTL day + INTERVAL 540 DAY`,
		},
		{
			sql: `CREATE MATERIALIZED VIEW IF NOT EXISTS logtap.track_events_daily_mv
TO logtap.track_event_daily
AS SELECT
	tenant_id, project_id,
	toDate(timestamp) AS day,
	name,
	countState() AS cnt,
	uniqExactState(distinct_id) AS uniq_users
FROM logtap.track_events
GROUP BY tenant_id, project_id, day, name`,
		},
	}
	return stmts
}
