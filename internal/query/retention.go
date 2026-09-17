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

	// Compute cohort sizes and per-offset retained counts entirely in the
	// database via a self-join over the (day, distinct_id) pairs. Pushing the
	// aggregation down avoids loading every active (day, user) pair into Go
	// memory; only the small aggregate matrix is transferred. The DISTINCT
	// wraps the whole union so a user active in several sources on the same
	// day counts once.
	var b strings.Builder
	var args []any
	b.WriteString("WITH active_events AS (SELECT DISTINCT day, distinct_id FROM (")
	for i, source := range sources {
		if i > 0 {
			b.WriteString(" UNION ALL ")
		}
		b.WriteString("SELECT ")
		b.WriteString(dayExpr)
		b.WriteString(" AS day, distinct_id FROM ")
		b.WriteString(source)
		b.WriteString(" WHERE project_id = ? AND distinct_id IS NOT NULL AND distinct_id <> '' AND timestamp >= ? AND timestamp < ?")
		args = append(args, projectID, startDay, queryEndExclusive)
	}
	b.WriteString(") u), pairs AS (")
	b.WriteString(" SELECT a.day AS cohort_day, b.day AS active_day")
	b.WriteString(" FROM active_events a JOIN active_events b ON b.distinct_id = a.distinct_id AND b.day >= a.day")
	b.WriteString(") SELECT cohort_day, active_day, COUNT(*) AS users FROM pairs GROUP BY cohort_day, active_day")

	type pairRow struct {
		CohortDay string `gorm:"column:cohort_day"`
		ActiveDay string `gorm:"column:active_day"`
		Users     int64  `gorm:"column:users"`
	}
	var pairs []pairRow
	if err := db.WithContext(ctx).Raw(b.String(), args...).Scan(&pairs).Error; err != nil {
		return nil, err
	}

	retained := map[[2]string]int64{}
	cohortSizes := map[string]int64{}
	for _, p := range pairs {
		cohort := normalizeRetentionDay(p.CohortDay)
		active := normalizeRetentionDay(p.ActiveDay)
		if cohort == "" || active == "" {
			continue
		}
		retained[[2]string{cohort, active}] += p.Users
		if cohort == active {
			cohortSizes[cohort] = retained[[2]string{cohort, active}]
		}
	}

	out := make([]metrics.RetentionRow, 0, int(endDay.Sub(startDay).Hours()/24)+1)
	for cur := startDay; !cur.After(endDay); cur = cur.AddDate(0, 0, 1) {
		cohort := cur.Format("2006-01-02")
		row := metrics.RetentionRow{
			Cohort:     cohort,
			CohortSize: cohortSizes[cohort],
			Points:     make([]metrics.RetentionPoint, 0, len(offsets)),
		}
		for _, d := range offsets {
			activeDay := cur.AddDate(0, 0, d).Format("2006-01-02")
			active := retained[[2]string{cohort, activeDay}]
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

// normalizeRetentionDay trims sqlite DATE() output ("2006-01-02 00:00:00")
// down to the plain date label.
func normalizeRetentionDay(day string) string {
	day = strings.TrimSpace(day)
	if len(day) > len("2006-01-02") {
		day = day[:len("2006-01-02")]
	}
	return day
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
