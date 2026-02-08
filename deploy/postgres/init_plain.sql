-- Deprecated: logtap uses code-driven migrations (gorm + internal/migrate) on startup.
-- Plain PostgreSQL schema (no TimescaleDB required); kept only for manual reference.

CREATE TABLE IF NOT EXISTS events (
    id UUID PRIMARY KEY,
    project_id INT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    level VARCHAR(20),
    platform VARCHAR(50),
    release_tag VARCHAR(100),
    environment VARCHAR(50),
    user_id VARCHAR(255),
    title TEXT,
    data JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_project_ts ON events (project_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_data ON events USING GIN (data);

CREATE TABLE IF NOT EXISTS logs (
    id BIGSERIAL PRIMARY KEY,
    project_id INT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    ingest_id UUID,
    level VARCHAR(20),
    trace_id VARCHAR(64),
    span_id VARCHAR(64),
    message TEXT NOT NULL,
    fields JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_logs_project_ts ON logs (project_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_logs_fields ON logs USING GIN (fields);
CREATE INDEX IF NOT EXISTS idx_logs_search_expr
  ON logs USING GIN (to_tsvector('simple', coalesce(message,'') || ' ' || coalesce(fields::text,'')));
CREATE UNIQUE INDEX IF NOT EXISTS idx_logs_dedupe ON logs (project_id, ingest_id);

-- Denormalized track events for analytics (funnel/top events).
CREATE TABLE IF NOT EXISTS track_events (
    id BIGSERIAL PRIMARY KEY,
    project_id INT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    ingest_id UUID,
    name TEXT NOT NULL,
    distinct_id VARCHAR(255) NOT NULL,
    device_id VARCHAR(255)
);

CREATE INDEX IF NOT EXISTS idx_track_events_project_ts ON track_events (project_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_track_events_project_name_ts ON track_events (project_id, name, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_track_events_project_user_ts ON track_events (project_id, distinct_id, timestamp DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_track_events_dedupe ON track_events (project_id, ingest_id);
