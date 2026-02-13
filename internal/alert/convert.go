package alert

import (
	"encoding/json"

	"github.com/aak1247/logtap/internal/model"
)

func InputFromLog(row model.Log) Input {
	fields := map[string]any{}
	if len(row.Fields) > 0 {
		_ = json.Unmarshal(row.Fields, &fields)
	}
	if fields == nil {
		fields = map[string]any{}
	}
	if row.IngestID != nil {
		fields["ingest_id"] = row.IngestID.String()
	}
	if row.DistinctID != "" {
		fields["distinct_id"] = row.DistinctID
	}
	if row.DeviceID != "" {
		fields["device_id"] = row.DeviceID
	}
	if row.TraceID != "" {
		fields["trace_id"] = row.TraceID
	}
	if row.SpanID != "" {
		fields["span_id"] = row.SpanID
	}

	return Input{
		ProjectID: row.ProjectID,
		Source:    SourceLogs,
		Timestamp: row.Timestamp.UTC(),
		Level:     row.Level,
		Message:   row.Message,
		Fields:    fields,
	}
}

func InputFromEvent(row model.Event) Input {
	fields := map[string]any{}
	if len(row.Data) > 0 {
		_ = json.Unmarshal(row.Data, &fields)
	}
	if fields == nil {
		fields = map[string]any{}
	}

	fields["event_id"] = row.ID.String()
	if row.DistinctID != "" {
		fields["distinct_id"] = row.DistinctID
	}
	if row.DeviceID != "" {
		fields["device_id"] = row.DeviceID
	}
	if row.OS != "" {
		fields["os"] = row.OS
	}
	if row.Platform != "" {
		fields["platform"] = row.Platform
	}
	if row.ReleaseTag != "" {
		fields["release"] = row.ReleaseTag
	}
	if row.Environment != "" {
		fields["environment"] = row.Environment
	}
	if row.UserID != "" {
		fields["user_id"] = row.UserID
	}

	return Input{
		ProjectID: row.ProjectID,
		Source:    SourceEvents,
		Timestamp: row.Timestamp.UTC(),
		Level:     row.Level,
		Message:   row.Title,
		Fields:    fields,
	}
}
