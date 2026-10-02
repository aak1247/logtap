package query

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CustomAnalyticsRequest is a minimal MVP request model for unified analytics.
// It intentionally supports a constrained set of options to keep SQL manageable.
//
// analysis_type: "event" | "property"
//   - "event": counts are grouped by event (logs.message) and optional property.
//   - "property": counts are grouped primarily by a single property.
//
// metric.type: "count_events" | "count_users"
//
// group_by: up to 2 elements from {"time", "event", "property:<key>"}.
//   - "time" uses the selected granularity (day/week/month).
//   - "event" groups by event name.
//   - "property:<key>" groups by a single JSON field from logs.fields.
//
// filter (MVP):
//   - events: optional whitelist of event names to include.
//   - properties: map of property key -> allowed values (IN filter).
type CustomAnalyticsRequest struct {
	AnalysisType string `json:"analysis_type"`
	TimeRange    struct {
		Start       string `json:"start"`
		End         string `json:"end"`
		Granularity string `json:"granularity"`
	} `json:"time_range"`
	Target struct {
		Events   []string `json:"events"`
		Property string   `json:"property"`
	} `json:"target"`
	Metric struct {
		Type string `json:"type"`
	} `json:"metric"`
	GroupBy []string `json:"group_by"`
	Filter  struct {
		Events     []string                       `json:"events"`
		Properties map[string]CustomPropertyFilter `json:"properties"`
	} `json:"filter"`
}

// CustomPropertyFilter is a minimal IN/equality filter for a property.
type CustomPropertyFilter struct {
	Values []string `json:"values"`
}

// CustomAnalyticsSeriesPoint represents a single datapoint in a time series.
type CustomAnalyticsSeriesPoint struct {
	Time  string `json:"time"`
	Value int64  `json:"value"`
}

// CustomAnalyticsSeries represents one logical series (line/bar/segment).
type CustomAnalyticsSeries struct {
	Name       string            `json:"name"`
	Dimensions map[string]string `json:"dimensions"`
	Points     []CustomAnalyticsSeriesPoint `json:"points"`
	Total      int64                        `json:"total"`
}

type customRow struct {
	Bucket string `gorm:"column:bucket"`
	Event  string `gorm:"column:event_name"`
	Prop   string `gorm:"column:prop_value"`
	Events int64  `gorm:"column:events"`
	Users  int64  `gorm:"column:users"`
}

// CustomAnalyticsHandler implements POST /api/:projectId/analytics/custom.
func CustomAnalyticsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
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

		// Parse time range with sensible defaults.
		now := time.Now().UTC()
		start, end, err := parseCustomTimeRange(req.TimeRange.Start, req.TimeRange.End, now)
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		if end.Before(start) {
			start, end = end, start
		}
		if end.Sub(start) > 180*24*time.Hour {
			respondErr(c, http.StatusBadRequest, "time range too large (max 180d)")
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

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		rows, err := runCustomAnalyticsQuery(ctx, db, projectID, analysisType, metricType, granularity, start, end, groupBy, propertyKey, &req)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		series := buildCustomSeries(rows, metricType, groupBy, propertyKey)
		respondOK(c, gin.H{
			"project_id":   projectID,
			"analysis_type": analysisType,
			"metric":       metricType,
			"granularity":  granularity,
			"start":        start.UTC().Format(time.RFC3339),
			"end":          end.UTC().Format(time.RFC3339),
			"group_by":     groupBy,
			"property_key": propertyKey,
			"series":       series,
		})
	}
}

func parseCustomTimeRange(startRaw, endRaw string, now time.Time) (time.Time, time.Time, error) {
	var start, end time.Time
	var okStart, okEnd bool
	if strings.TrimSpace(startRaw) != "" {
		if ts, err := time.Parse(time.RFC3339, startRaw); err == nil {
			start = ts.UTC()
			okStart = true
		} else {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start (expected RFC3339)")
		}
	}
	if strings.TrimSpace(endRaw) != "" {
		if ts, err := time.Parse(time.RFC3339, endRaw); err == nil {
			end = ts.UTC()
			okEnd = true
		} else {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end (expected RFC3339)")
		}
	}
	if !okEnd {
		end = now
	}
	if !okStart {
		start = end.AddDate(0, 0, -13) // default 14 days
	}
	return start, end, nil
}

// parseGroupBy validates and normalizes group_by tokens.
func parseGroupBy(raw []string) ([]string, string, error) {
	if len(raw) == 0 {
		return []string{"time"}, "", nil
	}
	if len(raw) > 2 {
		return nil, "", fmt.Errorf("too many group_by dimensions (max 2)")
	}
	seen := map[string]bool{}
	var out []string
	var propertyKey string
	for _, g := range raw {
		g = strings.TrimSpace(strings.ToLower(g))
		if g == "" {
			continue
		}
		if seen[g] {
			continue
		}
		if g == "time" || g == "event" {
			out = append(out, g)
			seen[g] = true
			continue
		}
		if strings.HasPrefix(g, "property:") {
			key := strings.TrimSpace(g[len("property:"):])
			if key == "" {
				return nil, "", fmt.Errorf("invalid group_by property (empty key)")
			}
			if propertyKey != "" && propertyKey != key {
				return nil, "", fmt.Errorf("only one property dimension is supported")
			}
			if !isSafePropertyKey(key) {
				return nil, "", fmt.Errorf("invalid property key")
			}
			propertyKey = key
			out = append(out, "property")
			seen["property"] = true
			continue
		}
		return nil, "", fmt.Errorf("invalid group_by token: %s", g)
	}
	if len(out) == 0 {
		out = []string{"time"}
	}
	return out, propertyKey, nil
}

func isSafePropertyKey(key string) bool {
	// Restrict to a conservative set to avoid SQL injection in JSON path.
	for _, r := range key {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '_' || r == '.' || r == '-' {
			continue
		}
		return false
	}
	return key != ""
}

func timeBucketExpr(db *gorm.DB, granularity string) string {
	// Note: we compute a string expression, evaluated by the DB directly.
	// For sqlite we use strftime; for postgres we use date_trunc/DATE.
	if db != nil && strings.EqualFold(db.Dialector.Name(), "sqlite") {
		switch granularity {
		case "week":
			return "strftime('%Y-%W', timestamp)"
		case "month":
			return "strftime('%Y-%m', timestamp)"
		default: // day
			return "strftime('%Y-%m-%d', timestamp)"
		}
	}
	// postgres / others
	switch granularity {
	case "week":
		return "to_char(date_trunc('week', timestamp), 'YYYY-MM-DD')"
	case "month":
		return "to_char(date_trunc('month', timestamp), 'YYYY-MM')"
	default: // day
		return "to_char(timestamp::date, 'YYYY-MM-DD')"
	}
}

func propertyExpr(db *gorm.DB, key string) string {
	if key == "" {
		return "''"
	}
	if db != nil && strings.EqualFold(db.Dialector.Name(), "sqlite") {
		// SQLite json_extract with a simple path.
		return fmt.Sprintf("json_extract(fields, '$.%s')", key)
	}
	// Postgres: fields->>'key'
	return fmt.Sprintf("fields->>'%s'", key)
}

func runCustomAnalyticsQuery(
	ctx context.Context,
	db *gorm.DB,
	projectID int,
	analysisType string,
	metricType string,
	granularity string,
	start, end time.Time,
	groupBy []string,
	propertyKey string,
	req *CustomAnalyticsRequest,
) ([]customRow, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if projectID <= 0 {
		return nil, gorm.ErrInvalidData
	}
	if len(groupBy) == 0 {
		groupBy = []string{"time"}
	}

	useTime := false
	useEvent := false
	useProp := false
	for _, g := range groupBy {
		switch g {
		case "time":
			useTime = true
		case "event":
			useEvent = true
		case "property":
			useProp = propertyKey != ""
		}
	}

	// Build SELECT clause.
	bucketExpr := "'all'"
	selectCols := []string{}
	groupCols := []string{}
	if useTime {
		bucketExpr = timeBucketExpr(db, granularity)
	}
	selectCols = append(selectCols, fmt.Sprintf("%s AS bucket", bucketExpr))
	groupCols = append(groupCols, "bucket")

	nameExpr := "message"
	if useEvent {
		selectCols = append(selectCols, fmt.Sprintf("%s AS event_name", nameExpr))
		groupCols = append(groupCols, "event_name")
	} else {
		selectCols = append(selectCols, "'' AS event_name")
	}

	propExpr := "''"
	if useProp {
		propExpr = propertyExpr(db, propertyKey)
	}
	selectCols = append(selectCols, fmt.Sprintf("%s AS prop_value", propExpr))
	if useProp {
		groupCols = append(groupCols, "prop_value")
	}

	metricExprEvents := "COUNT(*) AS events"
	metricExprUsers := "COUNT(DISTINCT distinct_id) AS users"
	selectCols = append(selectCols, metricExprEvents, metricExprUsers)

	// Base query from logs where level='event'.
	q := db.WithContext(ctx).
		Table("logs").
		Select(strings.Join(selectCols, ", ")).
		Where("project_id = ? AND level = 'event' AND timestamp >= ? AND timestamp <= ?", projectID, start, end)

	// Event filters (from target.events or filter.events).
	var eventsFilter []string
	for _, e := range req.Target.Events {
		if strings.TrimSpace(e) != "" {
			eventsFilter = append(eventsFilter, strings.TrimSpace(e))
		}
	}
	for _, e := range req.Filter.Events {
		if strings.TrimSpace(e) != "" {
			eventsFilter = append(eventsFilter, strings.TrimSpace(e))
		}
	}
	if len(eventsFilter) > 0 {
		q = q.Where("message IN ?", eventsFilter)
	}

	// Property filters (simple IN filter on fields->>'key').
	for key, f := range req.Filter.Properties {
		if len(f.Values) == 0 {
			continue
		}
		if !isSafePropertyKey(key) {
			return nil, errors.New("invalid property key in filter")
		}
		col := propertyExpr(db, key)
		q = q.Where(col+" IN ?", f.Values)
	}

	if len(groupCols) > 0 {
		q = q.Group(strings.Join(groupCols, ", "))
	}
	q = q.Order("bucket ASC, event_name ASC, prop_value ASC")

	var rows []customRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func buildCustomSeries(rows []customRow, metricType string, groupBy []string, propertyKey string) []CustomAnalyticsSeries {
	useTime := false
	useEvent := false
	useProp := false
	for _, g := range groupBy {
		switch g {
		case "time":
			useTime = true
		case "event":
			useEvent = true
		case "property":
			useProp = propertyKey != ""
		}
	}

	seriesMap := map[string]*CustomAnalyticsSeries{}
	for _, r := range rows {
		// Build dimension map and series key.
		dims := map[string]string{}
		keyParts := []string{}
		if useEvent && r.Event != "" {
			dims["event"] = r.Event
			keyParts = append(keyParts, "event="+r.Event)
		}
		if useProp && r.Prop != "" {
			dims["property"] = r.Prop
			keyParts = append(keyParts, "property="+r.Prop)
		}
		if len(keyParts) == 0 {
			keyParts = append(keyParts, "all")
		}
		seriesKey := strings.Join(keyParts, "|")

		val := r.Events
		if metricType == "count_users" {
			val = r.Users
		}

		// Initialize series if first time.
		s, ok := seriesMap[seriesKey]
		if !ok {
			name := seriesKey
			if useEvent && !useProp {
				name = r.Event
			} else if useProp && !useEvent {
				name = r.Prop
			}
			s = &CustomAnalyticsSeries{
				Name:       name,
				Dimensions: dims,
			}
			seriesMap[seriesKey] = s
		}
		if useTime {
			// Append time-series point.
			s.Points = append(s.Points, CustomAnalyticsSeriesPoint{Time: r.Bucket, Value: val})
		}
		// Aggregate total regardless of time.
		s.Total += val
	}

	out := make([]CustomAnalyticsSeries, 0, len(seriesMap))
	for _, s := range seriesMap {
		// Ensure deterministic order of points by time.
		if len(s.Points) > 1 {
			// Points are already ordered by bucket in SQL ORDER BY.
		}
		out = append(out, *s)
	}
	return out
}
