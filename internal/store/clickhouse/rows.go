package clickhouse

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/aak1247/logtap/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/aak1247/logtap/internal/model"
)

// LogRow is the ClickHouse encoding of model.Log (design §4.1).
type LogRow struct {
	TenantID   uuid.UUID
	ProjectID  uint32
	Timestamp  time.Time
	IngestID   uuid.UUID
	Level      string
	Message    string
	Fields     map[string]string
	TraceID    string
	SpanID     string
	DistinctID string
	DeviceID   string
}

// EventRow is the ClickHouse encoding of model.Event (design §4.2).
type EventRow struct {
	TenantID    uuid.UUID
	ProjectID   uint32
	Timestamp   time.Time
	EventID     uuid.UUID
	Level       string
	Title       string
	DistinctID  string
	DeviceID    string
	OS          string
	Platform    string
	ReleaseTag  string
	Environment string
	UserID      string
	Data        string
}

// TrackEventRow is the ClickHouse encoding of model.TrackEvent (design §4.3).
type TrackEventRow struct {
	TenantID   uuid.UUID
	ProjectID  uint32
	Timestamp  time.Time
	IngestID   uuid.UUID
	Name       string
	DistinctID string
	DeviceID   string
}

// LogRowsFromModel converts PG model rows into ClickHouse rows. Non-string
// field values are serialized as JSON strings because the ClickHouse fields
// column is Map(LowCardinality(String), String).
func LogRowsFromModel(logs []model.Log) []LogRow {
	rows := make([]LogRow, 0, len(logs))
	for _, l := range logs {
		ingestID := uuid.UUID{}
		if l.IngestID != nil {
			ingestID = *l.IngestID
		}
		rows = append(rows, LogRow{
			ProjectID:  clampProjectID(l.ProjectID),
			Timestamp:  l.Timestamp.UTC(),
			IngestID:   ingestID,
			Level:      l.Level,
			Message:    l.Message,
			Fields:     fieldsToMap(l.Fields),
			TraceID:    l.TraceID,
			SpanID:     l.SpanID,
			DistinctID: l.DistinctID,
			DeviceID:   l.DeviceID,
		})
	}
	return rows
}

// EventRowsFromModel converts PG model events into ClickHouse rows.
func EventRowsFromModel(events []model.Event) []EventRow {
	rows := make([]EventRow, 0, len(events))
	for _, e := range events {
		rows = append(rows, EventRow{
			ProjectID:   clampProjectID(e.ProjectID),
			Timestamp:   e.Timestamp.UTC(),
			EventID:     e.ID,
			Level:       e.Level,
			Title:       e.Title,
			DistinctID:  e.DistinctID,
			DeviceID:    e.DeviceID,
			OS:          e.OS,
			Platform:    e.Platform,
			ReleaseTag:  e.ReleaseTag,
			Environment: e.Environment,
			UserID:      e.UserID,
			Data:        string(e.Data),
		})
	}
	return rows
}

// TrackEventRowsFromModel converts PG model track events into ClickHouse rows.
func TrackEventRowsFromModel(events []model.TrackEvent) []TrackEventRow {
	rows := make([]TrackEventRow, 0, len(events))
	for _, e := range events {
		ingestID := uuid.UUID{}
		if e.IngestID != nil {
			ingestID = *e.IngestID
		}
		rows = append(rows, TrackEventRow{
			ProjectID:  clampProjectID(e.ProjectID),
			Timestamp:  e.Timestamp.UTC(),
			IngestID:   ingestID,
			Name:       e.Name,
			DistinctID: e.DistinctID,
			DeviceID:   e.DeviceID,
		})
	}
	return rows
}

// WithTenant stamps the tenant onto every row (open-source builds pass the
// fixed DefaultTenantID).
func (r *LogRow) WithTenant(id tenant.ID)        { r.TenantID = parseTenantUUID(id) }
func (r *EventRow) WithTenant(id tenant.ID)      { r.TenantID = parseTenantUUID(id) }
func (r *TrackEventRow) WithTenant(id tenant.ID) { r.TenantID = parseTenantUUID(id) }

func parseTenantUUID(id tenant.ID) uuid.UUID {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		parsed, _ = uuid.Parse(string(tenant.DefaultTenantID))
	}
	return parsed
}

func clampProjectID(id int) uint32 {
	if id < 0 {
		return 0
	}
	return uint32(id)
}

func fieldsToMap(raw datatypes.JSON) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	for k, v := range m {
		switch tv := v.(type) {
		case string:
			out[k] = tv
		case nil:
			out[k] = ""
		default:
			enc, err := json.Marshal(tv)
			if err != nil {
				continue
			}
			out[k] = string(enc)
		}
	}
	return out
}

const insertLogsSQL = "INSERT INTO logtap.logs (tenant_id, project_id, timestamp, ingest_id, level, message, fields, trace_id, span_id, distinct_id, device_id)"

// InsertLogs batch-inserts log rows in one ClickHouse batch.
func InsertLogs(ctx context.Context, conn driver.Conn, rows []LogRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := conn.PrepareBatch(ctx, insertLogsSQL)
	if err != nil {
		return err
	}
	for i := range rows {
		r := &rows[i]
		if err := batch.Append(r.TenantID, r.ProjectID, r.Timestamp, r.IngestID, r.Level, r.Message, r.Fields, r.TraceID, r.SpanID, r.DistinctID, r.DeviceID); err != nil {
			return err
		}
	}
	return batch.Send()
}

const insertEventsSQL = "INSERT INTO logtap.events (tenant_id, project_id, timestamp, event_id, level, title, distinct_id, device_id, os, platform, release_tag, environment, user_id, data)"

// InsertEvents batch-inserts event rows in one ClickHouse batch.
func InsertEvents(ctx context.Context, conn driver.Conn, rows []EventRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := conn.PrepareBatch(ctx, insertEventsSQL)
	if err != nil {
		return err
	}
	for i := range rows {
		r := &rows[i]
		if err := batch.Append(r.TenantID, r.ProjectID, r.Timestamp, r.EventID, r.Level, r.Title, r.DistinctID, r.DeviceID, r.OS, r.Platform, r.ReleaseTag, r.Environment, r.UserID, r.Data); err != nil {
			return err
		}
	}
	return batch.Send()
}

const insertTrackEventsSQL = "INSERT INTO logtap.track_events (tenant_id, project_id, timestamp, ingest_id, name, distinct_id, device_id)"

// InsertTrackEvents batch-inserts track event rows in one ClickHouse batch.
func InsertTrackEvents(ctx context.Context, conn driver.Conn, rows []TrackEventRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := conn.PrepareBatch(ctx, insertTrackEventsSQL)
	if err != nil {
		return err
	}
	for i := range rows {
		r := &rows[i]
		if err := batch.Append(r.TenantID, r.ProjectID, r.Timestamp, r.IngestID, r.Name, r.DistinctID, r.DeviceID); err != nil {
			return err
		}
	}
	return batch.Send()
}
