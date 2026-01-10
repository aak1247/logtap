-- Plain PostgreSQL schema (no TimescaleDB required).

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
    level VARCHAR(20),
    trace_id VARCHAR(64),
    span_id VARCHAR(64),
    message TEXT NOT NULL,
    fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    search_vector tsvector GENERATED ALWAYS AS (
      to_tsvector('simple', coalesce(message,'') || ' ' || coalesce(fields::text,''))
    ) STORED
);

CREATE INDEX IF NOT EXISTS idx_logs_project_ts ON logs (project_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_logs_fields ON logs USING GIN (fields);
CREATE INDEX IF NOT EXISTS idx_logs_search ON logs USING GIN (search_vector);

