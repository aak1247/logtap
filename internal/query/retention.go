package query

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GET /api/:projectId/analytics/retention?start=RFC3339&end=RFC3339&days=1,7,30
func RetentionHandler(recorder *metrics.RedisRecorder, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if recorder == nil && db == nil {
			respondErr(c, http.StatusNotImplemented, "metrics not configured")
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
			start = end.AddDate(0, 0, -13) // 14 days incl today
		}

		days := parseCSVPositiveInts(c.Query("days"), []int{1, 7, 30}, 10, 365)
		sort.Ints(days)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		rows, err := retentionRows(ctx, recorder, db, projectID, start, end, days)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
			"days":       days,
			"rows":       rows,
		})
	}
}

func retentionRows(ctx context.Context, recorder *metrics.RedisRecorder, db *gorm.DB, projectID int, start, end time.Time, days []int) ([]metrics.RetentionRow, error) {
	maxOffset := 0
	for _, d := range days {
		if d > maxOffset {
			maxOffset = d
		}
	}
	coverageEnd := end.AddDate(0, 0, maxOffset)

	var rows []metrics.RetentionRow
	var redisErr error
	if recorder != nil {
		rows, redisErr = recorder.Retention(ctx, projectID, start, end, days)
	}
	if db == nil || (redisErr == nil && recorder.ActiveWarmupCovers(ctx, start, coverageEnd, "day")) {
		return rows, redisErr
	}

	dbRows, err := retentionRowsFromDB(ctx, db, projectID, start, end, days)
	if err != nil {
		if len(rows) > 0 || redisErr == nil {
			return rows, redisErr
		}
		return nil, err
	}
	return dbRows, nil
}

func retentionRowsFromDB(ctx context.Context, db *gorm.DB, projectID int, start, end time.Time, dayOffsets []int) ([]metrics.RetentionRow, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	offsets := normalizeRetentionOffsets(dayOffsets)
	maxOffset := 0
	for _, d := range offsets {
		if d > maxOffset {
			maxOffset = d
		}
	}

	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	queryEndExclusive := endDay.AddDate(0, 0, maxOffset+1)
	sources := activeDBSources(db)
	if len(sources) == 0 {
		return nil, fmt.Errorf("logs/events unavailable")
	}

	dayExpr := "DATE(timestamp)"
	if strings.EqualFold(db.Dialector.Name(), "postgres") {
		dayExpr = "TO_CHAR(timestamp AT TIME ZONE 'UTC', 'YYYY-MM-DD')"
	}
	var b strings.Builder
	var args []any
	b.WriteString("WITH active_events AS (")
	for i, source := range sources {
		if i > 0 {
			b.WriteString(" UNION ALL ")
		}
		b.WriteString("SELECT DISTINCT ")
		b.WriteString(dayExpr)
		b.WriteString(" AS day, distinct_id FROM ")
		b.WriteString(source)
		b.WriteString(" WHERE project_id = ? AND distinct_id IS NOT NULL AND distinct_id <> '' AND timestamp >= ? AND timestamp < ?")
		args = append(args, projectID, startDay, queryEndExclusive)
	}
	b.WriteString(") SELECT day, distinct_id FROM active_events ORDER BY day")

	type activeUserRow struct {
		Day        string `gorm:"column:day"`
		DistinctID string `gorm:"column:distinct_id"`
	}
	var activeRows []activeUserRow
	if err := db.WithContext(ctx).Raw(b.String(), args...).Scan(&activeRows).Error; err != nil {
		return nil, err
	}

	byDay := map[string]map[string]struct{}{}
	for _, row := range activeRows {
		day := strings.TrimSpace(row.Day)
		if len(day) > len("2006-01-02") {
			day = day[:len("2006-01-02")]
		}
		distinctID := strings.TrimSpace(row.DistinctID)
		if day == "" || distinctID == "" {
			continue
		}
		if byDay[day] == nil {
			byDay[day] = map[string]struct{}{}
		}
		byDay[day][distinctID] = struct{}{}
	}

	out := make([]metrics.RetentionRow, 0, int(endDay.Sub(startDay).Hours()/24)+1)
	for cur := startDay; !cur.After(endDay); cur = cur.AddDate(0, 0, 1) {
		cohort := cur.Format("2006-01-02")
		cohortUsers := byDay[cohort]
		row := metrics.RetentionRow{
			Cohort:     cohort,
			CohortSize: int64(len(cohortUsers)),
			Points:     make([]metrics.RetentionPoint, 0, len(offsets)),
		}
		for _, d := range offsets {
			targetUsers := byDay[cur.AddDate(0, 0, d).Format("2006-01-02")]
			active := retentionIntersectionSize(cohortUsers, targetUsers)
			rate := 0.0
			if row.CohortSize > 0 {
				rate = float64(active) / float64(row.CohortSize)
			}
			row.Points = append(row.Points, metrics.RetentionPoint{Day: d, Active: active, Rate: rate})
		}
		out = append(out, row)
	}
	return out, nil
}

func normalizeRetentionOffsets(dayOffsets []int) []int {
	seen := map[int]bool{}
	var offsets []int
	for _, d := range dayOffsets {
		if d <= 0 || d > 365 || seen[d] {
			continue
		}
		seen[d] = true
		offsets = append(offsets, d)
	}
	sort.Ints(offsets)
	if len(offsets) == 0 {
		return []int{1, 7, 30}
	}
	if len(offsets) > 10 {
		return offsets[:10]
	}
	return offsets
}

func retentionIntersectionSize(a, b map[string]struct{}) int64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	var n int64
	for id := range a {
		if _, ok := b[id]; ok {
			n++
		}
	}
	return n
}

func parseCSVPositiveInts(raw string, def []int, maxN int, maxValue int) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	parts := strings.Split(raw, ",")
	seen := map[int]bool{}
	var out []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > maxValue {
			continue
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
		if len(out) >= maxN {
			break
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
