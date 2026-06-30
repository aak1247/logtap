package query

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GET /api/:projectId/analytics/active?bucket=day|month&start=RFC3339&end=RFC3339
func ActiveSeriesHandler(recorder *metrics.RedisRecorder, db *gorm.DB) gin.HandlerFunc {
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

		bucket := strings.ToLower(strings.TrimSpace(c.Query("bucket")))
		if bucket != "month" {
			bucket = "day"
		}

		now := time.Now().UTC()
		start, okStart := parseTime(c.Query("start"))
		end, okEnd := parseTime(c.Query("end"))
		if !okEnd {
			end = now
		}
		if !okStart {
			if bucket == "month" {
				start = end.AddDate(0, -5, 0) // 6 months incl current
			} else {
				start = end.AddDate(0, 0, -13) // 14 days incl today
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		series, err := activeSeries(ctx, recorder, db, projectID, start, end, bucket)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"bucket":     bucket,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
			"series":     series,
		})
	}
}

func activeSeries(ctx context.Context, recorder *metrics.RedisRecorder, db *gorm.DB, projectID int, start, end time.Time, bucket string) ([]metrics.BucketCount, error) {
	if bucket == "month" {
		var series []metrics.BucketCount
		var redisErr error
		if recorder != nil {
			series, redisErr = recorder.ActiveSeries(ctx, projectID, start, end, bucket)
		}
		if db == nil || (redisErr == nil && recorder.ActiveWarmupCovers(ctx, start, end, bucket)) {
			return series, redisErr
		}
		dbCounts, err := activeMonthCountsFromDB(ctx, db, projectID, start, end)
		if err != nil {
			if len(series) > 0 || redisErr == nil {
				return series, redisErr
			}
			return nil, err
		}
		if len(series) == 0 {
			series = emptyMonthSeries(start, end)
		}
		for i := range series {
			if n := dbCounts[series[i].Bucket]; n > series[i].Active {
				series[i].Active = n
			}
		}
		return series, nil
	}

	var series []metrics.BucketCount
	var redisErr error
	if recorder != nil {
		series, redisErr = recorder.ActiveSeries(ctx, projectID, start, end, bucket)
	}
	if db == nil || (redisErr == nil && recorder.ActiveWarmupCovers(ctx, start, end, bucket)) {
		return series, redisErr
	}

	dbCounts, err := activeDayCountsFromDB(ctx, db, projectID, start, end)
	if err != nil {
		if len(series) > 0 || redisErr == nil {
			return series, redisErr
		}
		return nil, err
	}
	if len(series) == 0 {
		series = emptyDaySeries(start, end)
	}
	for i := range series {
		if n := dbCounts[series[i].Bucket]; n > series[i].Active {
			series[i].Active = n
		}
	}
	return series, nil
}

type activeDayCountRow struct {
	Day    string `gorm:"column:day"`
	Active int64  `gorm:"column:active"`
}

func activeDayCountsFromDB(ctx context.Context, db *gorm.DB, projectID int, start, end time.Time) (map[string]int64, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endExclusive := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)

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
		b.WriteString("SELECT ")
		b.WriteString(dayExpr)
		b.WriteString(" AS day, distinct_id FROM ")
		b.WriteString(source)
		b.WriteString(" WHERE project_id = ? AND distinct_id IS NOT NULL AND distinct_id <> '' AND timestamp >= ? AND timestamp < ?")
		args = append(args, projectID, startDay, endExclusive)
	}
	b.WriteString(") SELECT day, COUNT(DISTINCT distinct_id) AS active FROM active_events GROUP BY day ORDER BY day")

	var rows []activeDayCountRow
	if err := db.WithContext(ctx).Raw(b.String(), args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		day := strings.TrimSpace(row.Day)
		if day == "" {
			continue
		}
		if len(day) > len("2006-01-02") {
			day = day[:len("2006-01-02")]
		}
		out[day] = row.Active
	}
	return out, nil
}

func activeMonthCountsFromDB(ctx context.Context, db *gorm.DB, projectID int, start, end time.Time) (map[string]int64, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	startMonth := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	endExclusive := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)

	sources := activeDBSources(db)
	if len(sources) == 0 {
		return nil, fmt.Errorf("logs/events unavailable")
	}

	monthExpr := "STRFTIME('%Y-%m', timestamp)"
	if strings.EqualFold(db.Dialector.Name(), "postgres") {
		monthExpr = "TO_CHAR(timestamp AT TIME ZONE 'UTC', 'YYYY-MM')"
	}

	var b strings.Builder
	var args []any
	b.WriteString("WITH active_events AS (")
	for i, source := range sources {
		if i > 0 {
			b.WriteString(" UNION ALL ")
		}
		b.WriteString("SELECT ")
		b.WriteString(monthExpr)
		b.WriteString(" AS month, distinct_id FROM ")
		b.WriteString(source)
		b.WriteString(" WHERE project_id = ? AND distinct_id IS NOT NULL AND distinct_id <> '' AND timestamp >= ? AND timestamp < ?")
		args = append(args, projectID, startMonth, endExclusive)
	}
	b.WriteString(") SELECT month AS day, COUNT(DISTINCT distinct_id) AS active FROM active_events GROUP BY month ORDER BY month")

	var rows []activeDayCountRow
	if err := db.WithContext(ctx).Raw(b.String(), args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		month := strings.TrimSpace(row.Day)
		if len(month) >= len("2006-01") {
			out[month[:len("2006-01")]] = row.Active
		}
	}
	return out, nil
}

func activeDBSources(db *gorm.DB) []string {
	if db == nil {
		return nil
	}
	var sources []string
	for _, table := range []string{"logs", "events", "track_events"} {
		if db.Migrator().HasTable(table) {
			sources = append(sources, table)
		}
	}
	return sources
}

func emptyDaySeries(start, end time.Time) []metrics.BucketCount {
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	cur := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]metrics.BucketCount, 0, int(last.Sub(cur).Hours()/24)+1)
	for !cur.After(last) {
		out = append(out, metrics.BucketCount{Bucket: cur.Format("2006-01-02")})
		cur = cur.AddDate(0, 0, 1)
	}
	return out
}

func emptyMonthSeries(start, end time.Time) []metrics.BucketCount {
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	cur := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	out := make([]metrics.BucketCount, 0, (last.Year()-cur.Year())*12+int(last.Month()-cur.Month())+1)
	for !cur.After(last) {
		out = append(out, metrics.BucketCount{Bucket: cur.Format("2006-01")})
		cur = cur.AddDate(0, 1, 0)
	}
	return out
}
