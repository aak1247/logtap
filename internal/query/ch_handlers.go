package query

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"strings"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/project"
	"github.com/aak1247/logtap/internal/store/clickhouse"
	"github.com/aak1247/logtap/internal/tenant"
)

// QueryRouter picks the storage backend per request. The unified-search
// adapter follows STORAGE_BACKEND directly; the data endpoints route through
// Route(): clickhouse mode always uses CH, dual mode applies the gray-release
// rules (QUERY_BACKEND_PROJECTS / QUERY_BACKEND_PERCENT), postgres never.
type QueryRouter struct {
	cfg  config.Config
	read chdriver.Conn // nil when the ClickHouse backend is disabled
}

func NewQueryRouter(cfg config.Config, read chdriver.Conn) *QueryRouter {
	return &QueryRouter{cfg: cfg, read: read}
}

func (r *QueryRouter) chEnabled() bool { return r != nil && r.read != nil }

func (r *QueryRouter) useCH(projectID string) bool {
	if !r.chEnabled() {
		return false
	}
	if r.cfg.StorageBackend == config.StorageBackendClickHouse {
		return true
	}
	if r.cfg.StorageBackend != config.StorageBackendDual {
		return false
	}
	id, err := project.ParseID(projectID)
	if err != nil {
		return false
	}
	for _, p := range r.cfg.QueryBackendProjects {
		if p == id {
			return true
		}
	}
	if r.cfg.QueryBackendPercent > 0 {
		h := fnv.New32a()
		_, _ = h.Write([]byte(projectID))
		return int(h.Sum32()%100) < r.cfg.QueryBackendPercent
	}
	return false
}

// Route wraps the PG and CH variants of one endpoint. exact=1 (the
// reconciliation mode) always goes to CH when available — it is a CH-only
// capability (design §4.7.1).
func (r *QueryRouter) Route(pg, ch gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if r.chEnabled() && (c.Query("exact") == "1" || r.useCH(c.Param("projectId"))) {
			ch(c)
			return
		}
		pg(c)
	}
}

func chTenant(c *gin.Context) tenant.ID { return tenant.FromOrDefault(c.Request.Context()) }

// CHSearchLogsHandler is the ClickHouse twin of SearchLogsHandler. The row
// "id" is the NSQ ingest_id (ClickHouse keeps no row id). When the request
// carries cursor=, keyset pagination is used and the continuation marker is
// returned in the X-Next-Cursor header.
func (r *QueryRouter) CHSearchLogsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		q := strings.TrimSpace(c.Query("q"))
		mode := strings.ToLower(strings.TrimSpace(c.Query("mode")))
		traceID := strings.TrimSpace(c.Query("trace_id"))
		level := strings.TrimSpace(c.Query("level"))
		start, okStart := parseTime(c.Query("start"))
		end, okEnd := parseTime(c.Query("end"))
		if !okEnd {
			end = time.Now().UTC()
		}
		if !okStart {
			start = end.AddDate(0, 0, -29)
		}
		if end.Before(start) {
			start, end = end, start
		}
		limit := parseLimit(c.Query("limit"), 100, 500)

		f := clickhouse.LogsFilter{
			TenantID:  chTenant(c),
			ProjectID: uint32(projectID),
			Start:     start,
			End:       end,
			Level:     level,
			TraceID:   traceID,
			Keyword:   q,
			Mode:      mode,
		}
		var cursor *clickhouse.SearchCursor
		if cur := strings.TrimSpace(c.Query("cursor")); cur != "" {
			c2, err := clickhouse.DecodeCursor(cur)
			if err != nil {
				respondErr(c, http.StatusBadRequest, err.Error())
				return
			}
			cursor = &c2
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		rows, err := clickhouse.SearchLogs(ctx, r.read, f, limit, cursor)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, e := range rows {
			entry := map[string]any{
				"id":        e.IngestID,
				"timestamp": e.Timestamp,
				"level":     e.Level,
				"trace_id":  e.TraceID,
				"span_id":   e.SpanID,
				"message":   e.Message,
			}
			if len(e.Fields) > 0 {
				fields := make(map[string]any, len(e.Fields))
				for k, v := range e.Fields {
					fields[k] = v
				}
				entry["fields"] = fields
			}
			out = append(out, entry)
		}
		if cursor != nil && len(rows) == limit {
			last := rows[len(rows)-1]
			c.Header("X-Next-Cursor", clickhouse.EncodeCursor(clickhouse.SearchCursor{Timestamp: last.Timestamp, IngestID: last.IngestID}))
		}
		respondOK(c, out)
	}
}

// CHLogTrendHandler is the ClickHouse twin of LogTrendHandler.
func (r *QueryRouter) CHLogTrendHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		level := strings.TrimSpace(c.Query("level"))
		if level == "" {
			level = "error"
		}
		bucket := strings.ToLower(strings.TrimSpace(c.Query("bucket")))
		if bucket != "day" {
			bucket = "hour"
		}
		top := parseLimit(c.Query("top"), 10, 50)

		now := time.Now().UTC()
		start, okStart := parseTime(c.Query("start"))
		end, okEnd := parseTime(c.Query("end"))
		if !okEnd {
			end = now
		}
		if !okStart {
			if bucket == "day" {
				start = end.AddDate(0, 0, -6)
			} else {
				start = end.Add(-24 * time.Hour)
			}
		}
		if end.Before(start) {
			start, end = end, start
		}
		if maxStart := end.Add(-30 * 24 * time.Hour); start.Before(maxStart) {
			start = maxStart
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		f := clickhouse.LogsFilter{TenantID: chTenant(c), ProjectID: uint32(projectID), Start: start, End: end, Level: level}
		points, tops, err := clickhouse.LogTrend(ctx, r.read, f, bucket, top)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		counts := make(map[string]int64, len(points))
		for _, p := range points {
			counts[p.Bucket] = p.Count
		}
		step := time.Hour
		if bucket == "day" {
			step = 24 * time.Hour
		}
		bucketStart := start.Truncate(step)
		var series []int64
		var labels []string
		for t := bucketStart; t.Before(end); t = t.Add(step) {
			label := bucketLabel(t, bucket)
			series = append(series, counts[label])
			labels = append(labels, label)
		}
		topItems := make([]map[string]any, 0, len(tops))
		for _, t := range tops {
			topItems = append(topItems, gin.H{"message": t.Message, "count": t.Count})
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"level":      level,
			"bucket":     bucket,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
			"labels":     labels,
			"points":     series,
			"top":        topItems,
		})
	}
}

func composeLevelStats(stats []clickhouse.LevelStat) (logs, events, errorsCount, users int64) {
	for _, s := range stats {
		logs += s.Count
		if s.Level == "event" {
			events = s.Count
		}
		if s.Level == "error" || s.Level == "fatal" {
			errorsCount += s.Count
		}
		users += s.Users
	}
	return
}

// CHMetricsTodayHandler reads the ClickHouse daily rollup (design §6.2):
// millisecond-level reads, no full-table fallback.
func (r *QueryRouter) CHMetricsTodayHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		now := time.Now().UTC()
		stats, err := clickhouse.MetricsByLevel(ctx, r.read, uint32(projectID), &now, &now)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		users, err := clickhouse.MetricsUsers(ctx, r.read, uint32(projectID), &now, &now)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		logs, events, errorsCount, _ := composeLevelStats(stats)
		respondOK(c, gin.H{
			"project_id": projectID,
			"date":       now.Format("2006-01-02"),
			"logs":       logs,
			"events":     events,
			"errors":     errorsCount,
			"users":      users,
		})
	}
}

// CHMetricsTotalHandler reads the all-time rollup. exact=1 switches to the
// dedup-aware raw-table reconciliation path, which requires a start/end
// window of at most 24 hours (design §4.7.1).
func (r *QueryRouter) CHMetricsTotalHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		if c.Query("exact") == "1" {
			start, okStart := parseTime(c.Query("start"))
			end, okEnd := parseTime(c.Query("end"))
			if !okStart || !okEnd {
				respondErr(c, http.StatusBadRequest, "exact=1 requires start and end (RFC3339)")
				return
			}
			if end.Before(start) {
				start, end = end, start
			}
			if end.Sub(start) > 24*time.Hour {
				respondErr(c, http.StatusBadRequest, "exact=1 window must be <= 24h")
				return
			}
			logs, users, err := clickhouse.MetricsExact(ctx, r.read, uint32(projectID), start, end)
			if err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			respondOK(c, gin.H{
				"project_id": projectID,
				"logs":       logs,
				"events":     int64(0),
				"users":      users,
				"exact":      true,
				"window":     gin.H{"start": start.UTC().Format(time.RFC3339), "end": end.UTC().Format(time.RFC3339)},
			})
			return
		}

		stats, err := clickhouse.MetricsByLevel(ctx, r.read, uint32(projectID), nil, nil)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		users, err := clickhouse.MetricsUsers(ctx, r.read, uint32(projectID), nil, nil)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		logs, events, _, _ := composeLevelStats(stats)
		respondOK(c, gin.H{
			"project_id": projectID,
			"logs":       logs,
			"events":     events,
			"users":      users,
		})
	}
}

// CHRecentEventsHandler is the ClickHouse twin of RecentEventsHandler.
func (r *QueryRouter) CHRecentEventsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		limit := parseLimit(c.Query("limit"), 50, 500)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		rows, err := clickhouse.RecentEvents(ctx, r.read, uint32(projectID), limit)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		type row struct {
			ID        string    `json:"id"`
			Timestamp time.Time `json:"timestamp"`
			Level     string    `json:"level,omitempty"`
			Title     string    `json:"title,omitempty"`
		}
		out := make([]row, 0, len(rows))
		for _, e := range rows {
			out = append(out, row{ID: e.ID, Timestamp: e.Timestamp, Level: e.Level, Title: e.Title})
		}
		respondOK(c, out)
	}
}

// CHGetEventHandler is the ClickHouse twin of GetEventHandler.
func (r *QueryRouter) CHGetEventHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		eid, err := uuid.Parse(strings.TrimSpace(c.Param("eventId")))
		if err != nil {
			respondErr(c, http.StatusBadRequest, "invalid eventId")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		data, ok, err := clickhouse.GetEventData(ctx, r.read, uint32(projectID), eid)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}
		respondOK(c, json.RawMessage(data))
	}
}

// CHTopEventsHandler is the ClickHouse twin of TopEventsHandler.
func (r *QueryRouter) CHTopEventsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		now := time.Now().UTC()
		start, okStart := parseTime(c.Query("start"))
		end, okEnd := parseTime(c.Query("end"))
		if !okEnd {
			end = now
		}
		if !okStart {
			start = end.AddDate(0, 0, -6)
		}
		if end.Before(start) {
			start, end = end, start
		}
		limit := parseLimit(c.Query("limit"), 20, 200)
		q := strings.TrimSpace(c.Query("q"))

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		stats, err := clickhouse.TopEvents(ctx, r.read, uint32(projectID), start, end, limit, q)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		items := make([]TopEventRow, 0, len(stats))
		for _, s := range stats {
			items = append(items, TopEventRow{Name: s.Name, Events: s.Events, Users: s.Users})
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
			"items":      items,
		})
	}
}

// CHFunnelHandler computes the funnel with windowFunnel. The PG retention
// pre-flight is skipped: ClickHouse retention is TTL-managed per table.
func (r *QueryRouter) CHFunnelHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		steps := parseCSVSteps(c.Query("steps"), 8)
		if len(steps) < 2 {
			respondErr(c, http.StatusBadRequest, "steps required (comma-separated), at least 2")
			return
		}
		now := time.Now().UTC()
		start, okStart := parseTime(c.Query("start"))
		end, okEnd := parseTime(c.Query("end"))
		if !okEnd {
			end = now
		}
		if !okStart {
			start = end.AddDate(0, 0, -6)
		}
		if end.Before(start) {
			respondErr(c, http.StatusBadRequest, "invalid time range")
			return
		}
		if end.Sub(start) > 31*24*time.Hour {
			respondErr(c, http.StatusBadRequest, "time range too large (max 31d)")
			return
		}
		var withinSec int64
		if withinRaw := strings.TrimSpace(c.Query("within")); withinRaw != "" {
			d, err := time.ParseDuration(withinRaw)
			if err != nil || d <= 0 {
				respondErr(c, http.StatusBadRequest, "invalid within duration")
				return
			}
			if d > 30*24*time.Hour {
				d = 30 * 24 * time.Hour
			}
			withinSec = int64(d / time.Second)
			if withinSec <= 0 {
				withinSec = 1
			}
		}
		source := strings.ToLower(strings.TrimSpace(c.Query("source")))
		if source != "" && source != "auto" && source != "track_events" && source != "track" && source != "logs" {
			respondErr(c, http.StatusBadRequest, "invalid source")
			return
		}
		if source == "auto" {
			source = ""
		}
		if source == "track" {
			source = "track_events"
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		counts, usedSource, err := clickhouse.FunnelCounts(ctx, r.read, uint32(projectID), steps, start, end, withinSec, source)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		var out []FunnelStep
		prev := int64(0)
		for i, name := range steps {
			cur := counts[i]
			conv := 0.0
			drop := int64(0)
			if i == 0 {
				conv = 1.0
			} else if prev > 0 {
				conv = float64(cur) / float64(prev)
				drop = prev - cur
			}
			out = append(out, FunnelStep{Name: name, Users: cur, Conversion: conv, Dropoff: drop})
			prev = cur
		}
		respondOK(c, gin.H{
			"project_id":  projectID,
			"start":       start.UTC().Format(time.RFC3339),
			"end":         end.UTC().Format(time.RFC3339),
			"within_secs": withinSec,
			"source":      usedSource,
			"steps":       out,
		})
	}
}

// CHCustomAnalyticsHandler is the ClickHouse twin of CustomAnalyticsHandler;
// the series assembly reuses the shared buildCustomSeries so the response
// shape is identical.
func (r *QueryRouter) CHCustomAnalyticsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		var req CustomAnalyticsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		analysisType := strings.ToLower(strings.TrimSpace(req.AnalysisType))
		if analysisType != "event" && analysisType != "property" {
			respondErr(c, http.StatusBadRequest, "invalid analysis_type (expected event|property)")
			return
		}
		metricType := strings.ToLower(strings.TrimSpace(req.Metric.Type))
		if metricType == "" {
			metricType = "count_events"
		}
		if metricType != "count_events" && metricType != "count_users" {
			respondErr(c, http.StatusBadRequest, "invalid metric.type (expected count_events|count_users)")
			return
		}
		now := time.Now().UTC()
		start, end, err := parseCustomTimeRange(req.TimeRange.Start, req.TimeRange.End, now)
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		if end.Before(start) {
			start, end = end, start
		}
		// Column store keeps 365d windows affordable (design §6.2).
		if end.Sub(start) > 365*24*time.Hour {
			respondErr(c, http.StatusBadRequest, "time range too large (max 365d)")
			return
		}
		granularity := strings.ToLower(strings.TrimSpace(req.TimeRange.Granularity))
		if granularity == "" {
			granularity = "day"
		}
		if granularity != "day" && granularity != "week" && granularity != "month" {
			respondErr(c, http.StatusBadRequest, "invalid granularity (expected day|week|month)")
			return
		}
		groupBy, propertyKey, err := parseGroupBy(req.GroupBy)
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		useEvent, useProp := false, false
		for _, g := range groupBy {
			switch g {
			case "event":
				useEvent = true
			case "property":
				useProp = propertyKey != ""
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		stats, err := clickhouse.CustomAnalytics(
			ctx, r.read, uint32(projectID), granularity, start, end,
			useEvent, useProp, propertyKey, req.Target.Events, len(req.Target.Events) > 0,
		)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		rows := make([]customRow, 0, len(stats))
		for _, s := range stats {
			rows = append(rows, customRow{Bucket: s.Bucket, Event: s.Event, Prop: s.Prop, Events: s.Events, Users: s.Users})
		}
		series := buildCustomSeries(rows, metricType, groupBy, propertyKey)
		respondOK(c, gin.H{
			"project_id":    projectID,
			"analysis_type": analysisType,
			"metric":        metricType,
			"granularity":   granularity,
			"start":         start.UTC().Format(time.RFC3339),
			"end":           end.UTC().Format(time.RFC3339),
			"group_by":      groupBy,
			"property_key":  propertyKey,
			"series":        series,
		})
	}
}

// CHStorageEstimateHandler is the ClickHouse twin of StorageEstimateHandler.
func (r *QueryRouter) CHStorageEstimateHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.chEnabled() {
			respondErr(c, http.StatusNotImplemented, "clickhouse backend not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		logsTab, eventsTab, err := clickhouse.StorageEstimate(ctx, r.read, uint32(projectID))
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		type estTable struct {
			Count       int64 `json:"count"`
			SampleSize  int   `json:"sample_size"`
			AvgRowBytes int64 `json:"avg_row_bytes"`
			EstBytes    int64 `json:"est_bytes"`
		}
		toTable := func(t clickhouse.StorageTableEstimate) estTable {
			sample := t.Count
			if sample > 1<<31-1 {
				sample = 1<<31 - 1
			}
			return estTable{Count: t.Count, SampleSize: int(sample), AvgRowBytes: t.AvgRowBytes, EstBytes: t.EstBytes}
		}
		l, e := toTable(logsTab), toTable(eventsTab)
		respondOK(c, gin.H{
			"project_id":   projectID,
			"logs":         l,
			"events":       e,
			"total_bytes":  l.EstBytes + e.EstBytes,
			"estimated_at": time.Now().UTC(),
		})
	}
}
