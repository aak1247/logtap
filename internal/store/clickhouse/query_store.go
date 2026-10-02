package clickhouse

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	chgo "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"github.com/aak1247/logtap/internal/tenant"
)

// Query-side helpers over the ClickHouse schema. Every statement
// unconditionally filters tenant_id (design §7.1); open-source builds always
// pass the fixed DefaultTenantID from tenant.FromOrDefault.

// interactiveCtx bounds one query to a small thread budget so many short
// console queries can run concurrently on a few cores (design §6.4.4).
func interactiveCtx(ctx context.Context, maxThreads int) context.Context {
	if maxThreads <= 0 {
		maxThreads = 2
	}
	return chgo.Context(ctx, chgo.WithSettings(chgo.Settings{
		"max_threads": maxThreads,
	}))
}

// LogEntry is one log row shaped for the query handlers.
type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	IngestID  string            `json:"ingest_id"`
	Level     string            `json:"level"`
	TraceID   string            `json:"trace_id"`
	SpanID    string            `json:"span_id"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// SearchCursor is the keyset pagination marker: base64("ts|ingest_id").
type SearchCursor struct {
	Timestamp time.Time
	IngestID  string
}

func EncodeCursor(c SearchCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.Timestamp.UTC().Format(time.RFC3339Nano) + "|" + c.IngestID))
}

func DecodeCursor(s string) (SearchCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return SearchCursor{}, fmt.Errorf("invalid cursor")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return SearchCursor{}, fmt.Errorf("invalid cursor")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return SearchCursor{}, fmt.Errorf("invalid cursor")
	}
	return SearchCursor{Timestamp: ts.UTC(), IngestID: parts[1]}, nil
}

// KeywordExpr translates a keyword filter into a CH boolean expression on
// message: fts mode uses the jieba text index (hasToken/hasAllTokens),
// contains mode does an in-column case-insensitive search.
func KeywordExpr(q, mode string) (string, []any) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", nil
	}
	if mode == "contains" {
		return "positionCaseInsensitive(message, ?) > 0", []any{q}
	}
	tokens := strings.Fields(q)
	if len(tokens) == 1 {
		return "hasToken(message, ?)", []any{tokens[0]}
	}
	args := make([]any, 0, len(tokens))
	for _, t := range tokens {
		args = append(args, t)
	}
	return "hasAllTokens(message, ?)", args
}

// RawFilter is a pre-rendered boolean condition with bound args (built by
// the search adapter from whitelisted DSL fields).
type RawFilter struct {
	Expr string
	Args []any
}

// chTime formats a time for binding into DateTime64(3) comparisons. Binding
// a Go time.Time directly loses sub-second precision (the driver binds
// DateTime at second precision by default), which corrupts keyset pagination
// boundaries.
func chTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05.000") }

type LogsFilter struct {
	TenantID  tenant.ID
	ProjectID uint32
	Start     time.Time
	End       time.Time
	Level     string
	TraceID   string
	// Keyword + Mode express the /logs/search q parameter (fts|contains).
	Keyword string
	Mode    string
	// Keywords are AND-ed case-insensitive substring matches (unified search).
	Keywords []string
	// Filters are extra pre-built conditions from the search adapter.
	Filters []RawFilter
}

func (f LogsFilter) where() (string, []any) {
	ten := f.TenantID
	if ten == "" {
		ten = tenant.DefaultTenantID
	}
	cond := []string{"tenant_id = ?", "project_id = ?"}
	args := []any{ten, f.ProjectID}
	if !f.Start.IsZero() {
		cond = append(cond, "timestamp >= toDateTime64(?, 3, 'UTC')")
		args = append(args, chTime(f.Start))
	}
	if !f.End.IsZero() {
		cond = append(cond, "timestamp <= toDateTime64(?, 3, 'UTC')")
		args = append(args, chTime(f.End))
	}
	if f.Level != "" {
		cond = append(cond, "level = ?")
		args = append(args, f.Level)
	}
	if f.TraceID != "" {
		cond = append(cond, "trace_id = ?")
		args = append(args, f.TraceID)
	}
	if expr, kwArgs := KeywordExpr(f.Keyword, f.Mode); expr != "" {
		cond = append(cond, expr)
		args = append(args, kwArgs...)
	}
	for _, kw := range f.Keywords {
		if kw = strings.TrimSpace(kw); kw != "" {
			cond = append(cond, "positionCaseInsensitive(message, ?) > 0")
			args = append(args, kw)
		}
	}
	for _, rf := range f.Filters {
		cond = append(cond, rf.Expr)
		args = append(args, rf.Args...)
	}
	return strings.Join(cond, " AND "), args
}

// SearchLogs selects one page of logs (keyset pagination when cursor set).
func SearchLogs(ctx context.Context, conn driver.Conn, f LogsFilter, limit int, cursor *SearchCursor) ([]LogEntry, error) {
	where, args := f.where()
	sql := "SELECT timestamp, ingest_id, level, trace_id, span_id, message, fields FROM logtap.logs WHERE " + where
	if cursor != nil {
		sql += " AND (timestamp, ingest_id) < (toDateTime64(?, 3, 'UTC'), ?)"
		args = append(args, chTime(cursor.Timestamp), cursor.IngestID)
	}
	sql += " ORDER BY timestamp DESC, ingest_id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := conn.Query(interactiveCtx(ctx, 2), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LogEntry, 0, limit)
	for rows.Next() {
		var e LogEntry
		var ingestID uuid.UUID
		var fields map[string]string
		if err := rows.Scan(&e.Timestamp, &ingestID, &e.Level, &e.TraceID, &e.SpanID, &e.Message, &fields); err != nil {
			return nil, err
		}
		e.IngestID = ingestID.String()
		e.Fields = fields
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountLogs returns the exact matching row count (cheap on column store).
func CountLogs(ctx context.Context, conn driver.Conn, f LogsFilter) (int64, error) {
	where, args := f.where()
	sql := "SELECT count() FROM logtap.logs WHERE " + where
	var n uint64
	if err := conn.QueryRow(interactiveCtx(ctx, 2), sql, args...).Scan(&n); err != nil {
		return 0, err
	}
	return int64(n), nil
}

// LevelFacet returns level → count over the filtered set.
func LevelFacet(ctx context.Context, conn driver.Conn, f LogsFilter) (map[string]int64, error) {
	where, args := f.where()
	sql := "SELECT level, count() AS c FROM logtap.logs WHERE " + where + " GROUP BY level ORDER BY c DESC"
	rows, err := conn.Query(interactiveCtx(ctx, 2), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var level string
		var n uint64
		if err := rows.Scan(&level, &n); err != nil {
			return nil, err
		}
		out[level] = int64(n)
	}
	return out, rows.Err()
}

// TrendPoint is one (bucket, count) pair; Bucket carries the formatted label.
type TrendPoint struct {
	Bucket string
	Count  int64
}

// TrendTop is one top-message row.
type TrendTop struct {
	Message string
	Count   int64
}

func trendBucketExpr(bucket string) string {
	if bucket == "day" {
		return "formatDateTime(toStartOfDay(timestamp), '%Y-%m-%dT00:00:00Z')"
	}
	return "formatDateTime(toStartOfHour(timestamp), '%Y-%m-%dT%H:00:00Z')"
}

// LogTrend aggregates counts per hour/day bucket plus the top messages.
func LogTrend(ctx context.Context, conn driver.Conn, f LogsFilter, bucket string, top int) ([]TrendPoint, []TrendTop, error) {
	where, args := f.where()
	bucketExpr := trendBucketExpr(bucket)
	rows, err := conn.Query(interactiveCtx(ctx, 4),
		"SELECT "+bucketExpr+" AS bucket, count() AS c FROM logtap.logs WHERE "+where+" GROUP BY bucket ORDER BY bucket",
		args...)
	if err != nil {
		return nil, nil, err
	}
	points := make([]TrendPoint, 0, 32)
	for rows.Next() {
		var bucket string
		var n uint64
		p := TrendPoint{}
		if err := rows.Scan(&bucket, &n); err != nil {
			rows.Close()
			return nil, nil, err
		}
		p.Bucket, p.Count = bucket, int64(n)
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	topRows, err := conn.Query(interactiveCtx(ctx, 4),
		"SELECT substring(message, 1, 80) AS m, count() AS c FROM logtap.logs WHERE "+where+" GROUP BY m ORDER BY c DESC LIMIT ?",
		append(append([]any{}, args...), top)...)
	if err != nil {
		return points, nil, err
	}
	defer topRows.Close()
	tops := make([]TrendTop, 0, top)
	for topRows.Next() {
		var msg string
		var n uint64
		t := TrendTop{}
		if err := topRows.Scan(&msg, &n); err != nil {
			return points, nil, err
		}
		t.Message, t.Count = msg, int64(n)
		tops = append(tops, t)
	}
	return points, tops, topRows.Err()
}

// LevelStat is one per-level rollup row.
type LevelStat struct {
	Level string
	Count int64
	Users int64
}

// MetricsByLevel aggregates the log_daily_stats rollup (design §6.2):
// regular /metrics reads never touch the raw table.
func MetricsByLevel(ctx context.Context, conn driver.Conn, projectID uint32, fromDay, toDay *time.Time) ([]LevelStat, error) {
	sql := "SELECT level, countMerge(cnt) AS c, uniqExactMerge(uniq_users) AS u FROM logtap.log_daily_stats WHERE tenant_id = ? AND project_id = ?"
	args := []any{tenant.DefaultTenantID, projectID}
	if fromDay != nil {
		sql += " AND day >= ?"
		args = append(args, fromDay.UTC().Format("2006-01-02"))
	}
	if toDay != nil {
		sql += " AND day <= ?"
		args = append(args, toDay.UTC().Format("2006-01-02"))
	}
	sql += " GROUP BY level"
	rows, err := conn.Query(interactiveCtx(ctx, 2), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LevelStat, 0, 8)
	for rows.Next() {
		var level string
		var c, u uint64
		if err := rows.Scan(&level, &c, &u); err != nil {
			return nil, err
		}
		out = append(out, LevelStat{Level: level, Count: int64(c), Users: int64(u)})
	}
	return out, rows.Err()
}

// MetricsUsers returns the exact distinct distinct_id count over the rollup
// for the window. It is a separate single-group query because merging
// per-level states and summing would double count users seen on several
// levels.
func MetricsUsers(ctx context.Context, conn driver.Conn, projectID uint32, fromDay, toDay *time.Time) (int64, error) {
	sql := "SELECT uniqExactMerge(uniq_users) FROM logtap.log_daily_stats WHERE tenant_id = ? AND project_id = ?"
	args := []any{tenant.DefaultTenantID, projectID}
	if fromDay != nil {
		sql += " AND day >= ?"
		args = append(args, fromDay.UTC().Format("2006-01-02"))
	}
	if toDay != nil {
		sql += " AND day <= ?"
		args = append(args, toDay.UTC().Format("2006-01-02"))
	}
	var n uint64
	if err := conn.QueryRow(interactiveCtx(ctx, 2), sql, args...).Scan(&n); err != nil {
		return 0, err
	}
	return int64(n), nil
}

// MetricsExact pushes a ≤24h reconciliation window down to the raw table
// with dedup-aware counting (design §4.7.1 exact=1).
func MetricsExact(ctx context.Context, conn driver.Conn, projectID uint32, start, end time.Time) (logs int64, users int64, err error) {
	sql := "SELECT uniqExact(ingest_id) AS logs, uniqExact(distinct_id) AS users FROM logtap.logs WHERE tenant_id = ? AND project_id = ? AND timestamp >= toDateTime64(?, 3, 'UTC') AND timestamp < toDateTime64(?, 3, 'UTC')"
	var logsN, usersN uint64
	err = conn.QueryRow(interactiveCtx(ctx, 4), sql, tenant.DefaultTenantID, projectID, chTime(start), chTime(end)).Scan(&logsN, &usersN)
	logs, users = int64(logsN), int64(usersN)
	return
}

// EventEntry is one event row for /events/recent.
type EventEntry struct {
	ID        string
	Timestamp time.Time
	Level     string
	Title     string
}

func RecentEvents(ctx context.Context, conn driver.Conn, projectID uint32, limit int) ([]EventEntry, error) {
	rows, err := conn.Query(interactiveCtx(ctx, 2),
		"SELECT event_id, timestamp, level, title FROM logtap.events WHERE tenant_id = ? AND project_id = ? ORDER BY timestamp DESC LIMIT ?",
		tenant.DefaultTenantID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EventEntry, 0, limit)
	for rows.Next() {
		var e EventEntry
		var id uuid.UUID
		if err := rows.Scan(&id, &e.Timestamp, &e.Level, &e.Title); err != nil {
			return nil, err
		}
		e.ID = id.String()
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetEventData returns the raw Sentry payload JSON for one event.
func GetEventData(ctx context.Context, conn driver.Conn, projectID uint32, eventID uuid.UUID) (string, bool, error) {
	var data string
	err := conn.QueryRow(interactiveCtx(ctx, 2),
		"SELECT data FROM logtap.events WHERE tenant_id = ? AND project_id = ? AND event_id = ? LIMIT 1",
		tenant.DefaultTenantID, projectID, eventID).Scan(&data)
	if err != nil {
		if isNotFoundErr(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return data, true, nil
}

func isNotFoundErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no rows")
}

// TopEventStat is one name aggregated row for /analytics/events/top.
type TopEventStat struct {
	Name   string
	Events int64
	Users  int64
}

// TopEvents reads the track_event_daily rollup, falling back to the raw
// track_events table for windows the rollup does not cover.
func TopEvents(ctx context.Context, conn driver.Conn, projectID uint32, start, end time.Time, limit int, q string) ([]TopEventStat, error) {
	fromDay := start.UTC().Format("2006-01-02")
	toDay := end.UTC().Format("2006-01-02")
	cond := "tenant_id = ? AND project_id = ? AND day >= ? AND day <= ?"
	args := []any{tenant.DefaultTenantID, projectID, fromDay, toDay}
	if q != "" {
		cond += " AND positionCaseInsensitive(name, ?) > 0"
		args = append(args, q)
	}
	rows, err := conn.Query(interactiveCtx(ctx, 2),
		"SELECT name, countMerge(cnt) AS events, uniqExactMerge(uniq_users) AS users FROM logtap.track_event_daily WHERE "+cond+" GROUP BY name ORDER BY events DESC LIMIT ?",
		append(args, limit)...)
	if err != nil {
		return nil, err
	}
	out := make([]TopEventStat, 0, limit)
	for rows.Next() {
		var name string
		var events, users uint64
		if err := rows.Scan(&name, &events, &users); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, TopEventStat{Name: name, Events: int64(events), Users: int64(users)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(out) > 0 {
		return out, nil
	}

	// Raw fallback (rollup MV only covers data written after its creation).
	rawCond := "tenant_id = ? AND project_id = ? AND timestamp >= toDateTime64(?, 3, 'UTC') AND timestamp <= toDateTime64(?, 3, 'UTC')"
	rawArgs := []any{tenant.DefaultTenantID, projectID, chTime(start), chTime(end)}
	if q != "" {
		rawCond += " AND positionCaseInsensitive(name, ?) > 0"
		rawArgs = append(rawArgs, q)
	}
	rows, err = conn.Query(interactiveCtx(ctx, 4),
		"SELECT name, count() AS events, uniqExact(distinct_id) AS users FROM logtap.track_events WHERE "+rawCond+" GROUP BY name ORDER BY events DESC LIMIT ?",
		append(rawArgs, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var events, users uint64
		if err := rows.Scan(&name, &events, &users); err != nil {
			return nil, err
		}
		out = append(out, TopEventStat{Name: name, Events: int64(events), Users: int64(users)})
	}
	return out, rows.Err()
}

// CustomStat mirrors the PG customRow shape (bucket/event/prop/events/users).
type CustomStat struct {
	Bucket string
	Event  string
	Prop   string
	Events int64
	Users  int64
}

func customBucketExpr(granularity string) string {
	switch granularity {
	case "week":
		return "formatDateTime(toStartOfWeek(timestamp, 1), '%Y-%m-%d')"
	case "month":
		return "formatDateTime(toStartOfMonth(timestamp), '%Y-%m-%d')"
	default:
		return "formatDateTime(toStartOfDay(timestamp), '%Y-%m-%d')"
	}
}

// CustomAnalytics aggregates the custom analytics rows over the logs table:
// event dimension = message (level='event'), property dimension =
// fields[propertyKey]. groupBy flags select the dimensions.
func CustomAnalytics(ctx context.Context, conn driver.Conn, projectID uint32, granularity string, start, end time.Time, groupByEvent, groupByProp bool, propertyKey string, targetEvents []string, restrictToEvents bool) ([]CustomStat, error) {
	bucketExpr := customBucketExpr(granularity)
	selectParts := []string{bucketExpr + " AS bucket"}
	groupParts := []string{"bucket"}
	if groupByEvent {
		selectParts = append(selectParts, "message AS event_name")
		groupParts = append(groupParts, "event_name")
	}
	if groupByProp {
		selectParts = append(selectParts, "ifNull(fields[?], '') AS prop_value")
		groupParts = append(groupParts, "prop_value")
	}
	selectParts = append(selectParts, "count() AS events", "uniqExact(distinct_id) AS users")

	cond := []string{"tenant_id = ?", "project_id = ?", "timestamp >= toDateTime64(?, 3, 'UTC')", "timestamp < toDateTime64(?, 3, 'UTC')"}
	args := []any{tenant.DefaultTenantID, projectID, chTime(start), chTime(end)}
	if groupByEvent || restrictToEvents || len(targetEvents) > 0 {
		cond = append(cond, "level = 'event'")
	}
	if len(targetEvents) > 0 {
		placeholders := make([]string, len(targetEvents))
		for i, e := range targetEvents {
			placeholders[i] = "?"
			args = append(args, e)
		}
		cond = append(cond, "message IN ("+strings.Join(placeholders, ",")+")")
	}

	sql := "SELECT " + strings.Join(selectParts, ", ") + " FROM logtap.logs WHERE " + strings.Join(cond, " AND ") +
		" GROUP BY " + strings.Join(groupParts, ", ") + " ORDER BY bucket"
	if groupByProp {
		// The property-key placeholder sits in the SELECT clause, before the
		// WHERE placeholders, so its bind value goes first.
		args = append([]any{propertyKey}, args...)
	}
	rows, err := conn.Query(interactiveCtx(ctx, 4), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CustomStat, 0, 64)
	for rows.Next() {
		var s CustomStat
		dest := []any{&s.Bucket}
		if groupByEvent {
			dest = append(dest, &s.Event)
		}
		if groupByProp {
			dest = append(dest, &s.Prop)
		}
		var events, users uint64
		dest = append(dest, &events, &users)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		s.Events, s.Users = int64(events), int64(users)
		out = append(out, s)
	}
	return out, rows.Err()
}

// FunnelCounts computes per-step user counts with windowFunnel over
// track_events (source=track_events) or event logs (source=logs).
func FunnelCounts(ctx context.Context, conn driver.Conn, projectID uint32, steps []string, start, end time.Time, withinSec int64, source string) ([]int64, string, error) {
	table := "logtap.track_events"
	nameExpr := "name"
	extra := ""
	usedSource := "track_events"
	if source == "logs" {
		table = "logtap.logs"
		nameExpr = "message"
		extra = " AND level = 'event'"
		usedSource = "logs"
	}
	if withinSec <= 0 {
		withinSec = int64(end.Sub(start) / time.Second)
		if withinSec <= 0 {
			withinSec = 1
		}
	}
	conds := make([]string, len(steps))
	args := []any{}
	for i, s := range steps {
		conds[i] = nameExpr + " = ?"
		args = append(args, s)
	}
	sql := "SELECT level, count() AS c FROM (" +
		"SELECT windowFunnel(?)(toDateTime(timestamp), " + strings.Join(conds, ", ") + ") AS level " +
		"FROM " + table + " WHERE tenant_id = ? AND project_id = ? AND timestamp >= toDateTime64(?, 3, 'UTC') AND timestamp <= toDateTime64(?, 3, 'UTC') AND distinct_id != ''" + extra +
		" GROUP BY distinct_id) GROUP BY level"
	allArgs := append([]any{withinSec}, args...)
	allArgs = append(allArgs, tenant.DefaultTenantID, projectID, chTime(start), chTime(end))
	rows, err := conn.Query(interactiveCtx(ctx, 4), sql, allArgs...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	levelCounts := map[int64]int64{}
	for rows.Next() {
		var level uint8
		var c uint64
		if err := rows.Scan(&level, &c); err != nil {
			return nil, "", err
		}
		levelCounts[int64(level)] = int64(c)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	counts := make([]int64, len(steps))
	for i := range steps {
		var sum int64
		for l, c := range levelCounts {
			if l >= int64(i+1) {
				sum += c
			}
		}
		counts[i] = sum
	}
	return counts, usedSource, nil
}

// StorageTableEstimate is the per-table estimate for /storage/estimate.
type StorageTableEstimate struct {
	Count       int64
	AvgRowBytes int64
	EstBytes    int64
}

// StorageEstimate approximates a project's share of each table by scaling
// the table's compressed bytes (system.parts) with the project's row share.
func StorageEstimate(ctx context.Context, conn driver.Conn, projectID uint32) (logs, events StorageTableEstimate, err error) {
	for _, t := range []struct {
		name    string
		logsTab bool
		dst     *StorageTableEstimate
	}{
		{"logs", true, &logs},
		{"events", false, &events},
	} {
		var tableBytes, tableRows uint64
		if err := conn.QueryRow(interactiveCtx(ctx, 2),
			"SELECT ifNull(sum(data_compressed_bytes), 0), ifNull(sum(rows), 0) FROM system.parts WHERE active AND database = 'logtap' AND table = ?",
			t.name).Scan(&tableBytes, &tableRows); err != nil {
			return logs, events, err
		}
		var projectRows uint64
		q := "SELECT count() FROM logtap." + t.name + " WHERE tenant_id = ? AND project_id = ?"
		if err := conn.QueryRow(interactiveCtx(ctx, 2), q, tenant.DefaultTenantID, projectID).Scan(&projectRows); err != nil {
			return logs, events, err
		}
		avg := int64(0)
		if tableRows > 0 {
			avg = int64(tableBytes / tableRows)
		}
		t.dst.Count = int64(projectRows)
		t.dst.AvgRowBytes = avg
		t.dst.EstBytes = avg * int64(projectRows)
	}
	return logs, events, nil
}
