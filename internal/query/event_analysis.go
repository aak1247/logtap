package query

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/project"
	"github.com/aak1247/logtap/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TopEventRow struct {
	Name   string `json:"name"`
	Events int64  `json:"events"`
	Users  int64  `json:"users"`
}

// GET /api/:projectId/analytics/events/top?start=RFC3339&end=RFC3339&limit=20&q=...
// Event name is derived from track events stored in logs: logs.level='event' and logs.message as event name.
func TopEventsHandler(db *gorm.DB) gin.HandlerFunc {
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

		now := time.Now().UTC()
		start, okStart := parseTime(c.Query("start"))
		end, okEnd := parseTime(c.Query("end"))
		if !okEnd {
			end = now
		}
		if !okStart {
			start = end.AddDate(0, 0, -6) // 7 days incl today
		}
		limit := parseLimit(c.Query("limit"), 20, 200)
		q := strings.TrimSpace(c.Query("q"))

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var out []TopEventRow
		if rows, err := topEventsFromTrackEventDailyOnly(ctx, db, projectID, start, end, limit, q); err == nil {
			out = rows
		} else if err := topEventsFromTrackEvents(ctx, db, projectID, start, end, limit, q, &out); err != nil {
			// Fallback to logs when track_events is missing or empty.
			if err := topEventsFromLogs(ctx, db, projectID, start, end, limit, q, &out); err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
			"items":      out,
		})
	}
}

func topEventsFromTrackEventDailyOnly(
	ctx context.Context,
	db *gorm.DB,
	projectID int,
	start, end time.Time,
	limit int,
	q string,
) ([]TopEventRow, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if projectID <= 0 {
		return nil, gorm.ErrInvalidData
	}
	if !db.Migrator().HasTable("track_event_daily") {
		return nil, fmt.Errorf("rollup table missing")
	}

	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	dayStart := start.Format("2006-01-02")
	dayEnd := end.Format("2006-01-02")

	var b strings.Builder
	var args []any
	args = append(args, projectID, dayStart, dayEnd)

	b.WriteString("WITH per_user AS (")
	b.WriteString("  SELECT name, distinct_id, SUM(events) AS events")
	b.WriteString("  FROM track_event_daily")
	b.WriteString("  WHERE project_id = ? AND day >= ? AND day <= ?")
	b.WriteString("  GROUP BY name, distinct_id")
	b.WriteString(")")
	b.WriteString(" SELECT name, SUM(events) AS events, COUNT(*) AS users")
	b.WriteString(" FROM per_user")
	if q != "" {
		b.WriteString(" WHERE " + nameLikeExpr(db))
		args = append(args, likeArg(db, q))
	}
	b.WriteString(" GROUP BY name")
	b.WriteString(" ORDER BY events DESC, name ASC")
	b.WriteString(" LIMIT ?")
	args = append(args, limit)

	var out []TopEventRow
	if err := db.WithContext(ctx).Raw(b.String(), args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func topEventsFromTrackEvents(ctx context.Context, db *gorm.DB, projectID int, start, end time.Time, limit int, q string, out *[]TopEventRow) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if projectID <= 0 {
		return gorm.ErrInvalidData
	}
	if !db.Migrator().HasTable("track_events") {
		return fmt.Errorf("track_events unavailable")
	}

	// Prefer the daily rollup table for multi-day ranges when available, but keep results exact
	// by only using rollups for full interior days and querying raw events for boundary slices.
	if rows, err := topEventsFromTrackEventsWithDailyRollup(ctx, db, projectID, start, end, limit, q); err == nil && len(rows) > 0 {
		*out = rows
		return nil
	}

	qdb := db.WithContext(ctx).Table("track_events").
		Select("name as name, COUNT(*) as events, COUNT(DISTINCT distinct_id) as users").
		Where("project_id = ? AND timestamp >= ? AND timestamp <= ?", projectID, start, end).
		Group("name").
		Order("events DESC, name ASC").
		Limit(limit)
	if q != "" {
		qdb = qdb.Where(nameLikeExpr(db), likeArg(db, q))
	}
	return qdb.Scan(out).Error
}

func topEventsFromTrackEventsWithDailyRollup(
	ctx context.Context,
	db *gorm.DB,
	projectID int,
	start time.Time,
	end time.Time,
	limit int,
	q string,
) ([]TopEventRow, error) {
	if db == nil || projectID <= 0 {
		return nil, gorm.ErrInvalidData
	}
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		return nil, gorm.ErrInvalidData
	}

	if !db.Migrator().HasTable("track_event_daily") {
		return nil, fmt.Errorf("rollup table missing")
	}

	startDay0 := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDay0 := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	if startDay0.Equal(endDay0) {
		return nil, fmt.Errorf("single day range")
	}

	leftEnd := startDay0.Add(24 * time.Hour)
	rightStart := endDay0
	interiorStart := leftEnd
	interiorEnd := endDay0.Add(-24 * time.Hour)
	if interiorStart.After(interiorEnd) {
		return nil, fmt.Errorf("no interior days")
	}
	dayStart := interiorStart.Format("2006-01-02")
	dayEnd := interiorEnd.Format("2006-01-02")

	var b strings.Builder
	var args []any
	args = append(args, projectID, start, leftEnd)
	args = append(args, projectID, rightStart, end)
	args = append(args, projectID, dayStart, dayEnd)

	b.WriteString("WITH per_user AS (")
	b.WriteString(" SELECT name, distinct_id, SUM(events) AS events")
	b.WriteString(" FROM (")
	b.WriteString("   SELECT name, distinct_id, COUNT(*) AS events")
	b.WriteString("   FROM track_events")
	b.WriteString("   WHERE project_id = ? AND timestamp >= ? AND timestamp < ?")
	b.WriteString("   GROUP BY name, distinct_id")
	b.WriteString("   UNION ALL")
	b.WriteString("   SELECT name, distinct_id, COUNT(*) AS events")
	b.WriteString("   FROM track_events")
	b.WriteString("   WHERE project_id = ? AND timestamp >= ? AND timestamp <= ?")
	b.WriteString("   GROUP BY name, distinct_id")
	b.WriteString("   UNION ALL")
	b.WriteString("   SELECT name, distinct_id, SUM(events) AS events")
	b.WriteString("   FROM track_event_daily")
	b.WriteString("   WHERE project_id = ? AND day >= ? AND day <= ?")
	b.WriteString("   GROUP BY name, distinct_id")
	b.WriteString(" ) x")
	b.WriteString(" GROUP BY name, distinct_id")
	b.WriteString(")")
	b.WriteString(" SELECT name, SUM(events) AS events, COUNT(*) AS users")
	b.WriteString(" FROM per_user")
	if q != "" {
		b.WriteString(" WHERE " + nameLikeExpr(db))
		args = append(args, likeArg(db, q))
	}
	b.WriteString(" GROUP BY name")
	b.WriteString(" ORDER BY events DESC, name ASC")
	b.WriteString(" LIMIT ?")
	args = append(args, limit)

	var out []TopEventRow
	if err := db.WithContext(ctx).Raw(b.String(), args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func topEventsFromLogs(ctx context.Context, db *gorm.DB, projectID int, start, end time.Time, limit int, q string, out *[]TopEventRow) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	qdb := db.WithContext(ctx).Table("logs").
		Select("message as name, COUNT(*) as events, COUNT(DISTINCT distinct_id) as users").
		Where("project_id = ? AND level = ? AND distinct_id IS NOT NULL AND distinct_id <> '' AND timestamp >= ? AND timestamp <= ?", projectID, "event", start, end).
		Group("message").
		Order("events DESC, message ASC").
		Limit(limit)
	if q != "" {
		col := "message"
		qdb = qdb.Where(nameLikeExpr(db, col), likeArg(db, q))
	}
	return qdb.Scan(out).Error
}

func nameLikeExpr(db *gorm.DB, col ...string) string {
	c := "name"
	if len(col) > 0 && strings.TrimSpace(col[0]) != "" {
		c = col[0]
	}
	if db != nil && strings.EqualFold(db.Dialector.Name(), "postgres") {
		return c + " ILIKE ?"
	}
	return "LOWER(" + c + ") LIKE ?"
}

func likeArg(db *gorm.DB, q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return "%"
	}
	if db != nil && strings.EqualFold(db.Dialector.Name(), "postgres") {
		return "%" + q + "%"
	}
	return "%" + strings.ToLower(q) + "%"
}

type FunnelStep struct {
	Name       string  `json:"name"`
	Users      int64   `json:"users"`
	Conversion float64 `json:"conversion"`
	Dropoff    int64   `json:"dropoff"`
}

// GET /api/:projectId/analytics/funnel?steps=a,b,c&start=RFC3339&end=RFC3339&within=24h
// Funnel is computed from track events stored in logs: logs.level='event', logs.message as event name and logs.distinct_id as user id.
func FunnelHandler(db *gorm.DB) gin.HandlerFunc {
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
			start = end.AddDate(0, 0, -6) // 7 days incl today
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
		withinRaw := strings.TrimSpace(c.Query("within"))
		if withinRaw != "" {
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

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		// Funnel requires detail tables (track_events/logs). If retention already purged the requested window,
		// results would be incomplete. Fail fast with actionable guidance.
		if pol, ok, err := store.GetCleanupPolicy(ctx, db, projectID); err == nil && ok && pol.Enabled {
			source := strings.ToLower(strings.TrimSpace(c.Query("source")))
			if source == "" || source == "auto" || source == "track_events" {
				if pol.TrackEventsRetentionDays > 0 {
					cutoff := now.Add(-time.Duration(pol.TrackEventsRetentionDays) * 24 * time.Hour)
					if start.Before(cutoff) {
						respondErr(
							c,
							http.StatusBadRequest,
							fmt.Sprintf(
								"funnel requires track_events; requested start=%s is older than retention cutoff=%s (track_events_retention_days=%d). Increase track_events_retention_days or reduce funnel time range.",
								start.UTC().Format(time.RFC3339),
								cutoff.UTC().Format(time.RFC3339),
								pol.TrackEventsRetentionDays,
							),
						)
						return
					}
				}
			}
			if source == "logs" && pol.LogsRetentionDays > 0 {
				cutoff := now.Add(-time.Duration(pol.LogsRetentionDays) * 24 * time.Hour)
				if start.Before(cutoff) {
					respondErr(
						c,
						http.StatusBadRequest,
						fmt.Sprintf(
							"funnel source=logs; requested start=%s is older than retention cutoff=%s (logs_retention_days=%d). Increase logs_retention_days or reduce funnel time range.",
							start.UTC().Format(time.RFC3339),
							cutoff.UTC().Format(time.RFC3339),
							pol.LogsRetentionDays,
						),
					)
					return
				}
			}
		}

		source := strings.ToLower(strings.TrimSpace(c.Query("source")))
		counts, usedSource, err := computeFunnelCounts(ctx, db, projectID, steps, start, end, withinSec, source)
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

type funnelCountsRow struct {
	Step1 int64 `gorm:"column:step1"`
	Step2 int64 `gorm:"column:step2"`
	Step3 int64 `gorm:"column:step3"`
	Step4 int64 `gorm:"column:step4"`
	Step5 int64 `gorm:"column:step5"`
	Step6 int64 `gorm:"column:step6"`
	Step7 int64 `gorm:"column:step7"`
	Step8 int64 `gorm:"column:step8"`
}

func computeFunnelCounts(ctx context.Context, db *gorm.DB, projectID int, steps []string, start, end time.Time, withinSec int64, source string) ([]int64, string, error) {
	switch source {
	case "", "auto":
		if ok := trackEventsAvailable(ctx, db, projectID, start, end, steps); ok {
			counts, err := computeFunnelCountsFromTableSQL(ctx, db, projectID, steps, start, end, withinSec, funnelTableSpec{
				Table:       "track_events",
				NameCol:     "name",
				RootFilter:  "distinct_id IS NOT NULL AND distinct_id <> ''",
				AliasFilter: "l.distinct_id IS NOT NULL AND l.distinct_id <> ''",
			})
			return counts, "track_events", err
		}
		counts, err := computeFunnelCountsFromTableSQL(ctx, db, projectID, steps, start, end, withinSec, funnelTableSpec{
			Table:       "logs",
			NameCol:     "message",
			RootFilter:  "level = 'event' AND distinct_id IS NOT NULL AND distinct_id <> ''",
			AliasFilter: "l.level = 'event' AND l.distinct_id IS NOT NULL AND l.distinct_id <> ''",
		})
		return counts, "logs", err
	case "track_events", "track":
		counts, err := computeFunnelCountsFromTableSQL(ctx, db, projectID, steps, start, end, withinSec, funnelTableSpec{
			Table:       "track_events",
			NameCol:     "name",
			RootFilter:  "distinct_id IS NOT NULL AND distinct_id <> ''",
			AliasFilter: "l.distinct_id IS NOT NULL AND l.distinct_id <> ''",
		})
		return counts, "track_events", err
	case "logs":
		counts, err := computeFunnelCountsFromTableSQL(ctx, db, projectID, steps, start, end, withinSec, funnelTableSpec{
			Table:       "logs",
			NameCol:     "message",
			RootFilter:  "level = 'event' AND distinct_id IS NOT NULL AND distinct_id <> ''",
			AliasFilter: "l.level = 'event' AND l.distinct_id IS NOT NULL AND l.distinct_id <> ''",
		})
		return counts, "logs", err
	default:
		return nil, "", fmt.Errorf("invalid source")
	}
}

func trackEventsAvailable(ctx context.Context, db *gorm.DB, projectID int, start, end time.Time, steps []string) bool {
	if db == nil || projectID <= 0 || len(steps) == 0 {
		return false
	}
	exists := 0
	err := db.WithContext(ctx).
		Table("track_events").
		Select("1").
		Where("project_id = ? AND timestamp >= ? AND timestamp <= ? AND name IN ?", projectID, start, end, steps).
		Limit(1).
		Scan(&exists).Error
	return err == nil && exists == 1
}

type funnelTableSpec struct {
	Table       string
	NameCol     string
	RootFilter  string
	AliasFilter string
}

func computeFunnelCountsFromTableSQL(ctx context.Context, db *gorm.DB, projectID int, steps []string, start, end time.Time, withinSec int64, spec funnelTableSpec) ([]int64, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if projectID <= 0 {
		return nil, gorm.ErrInvalidData
	}
	if len(steps) < 2 {
		return nil, gorm.ErrInvalidData
	}
	if len(steps) > 8 {
		steps = steps[:8]
	}

	// Use epoch-microseconds arithmetic so the query can run on both Postgres and SQLite.
	// - postgres: EXTRACT(EPOCH FROM ts) * 1e6
	// - sqlite: (julianday(ts) - 2440587.5) * 86400 * 1e6
	epochUS := "EXTRACT(EPOCH FROM timestamp) * 1000000"
	epochUSAlias := func(alias string) string {
		if strings.EqualFold(db.Dialector.Name(), "sqlite") {
			return fmt.Sprintf("((julianday(%s.timestamp) - 2440587.5) * 86400.0 * 1000000.0)", alias)
		}
		return fmt.Sprintf("(EXTRACT(EPOCH FROM %s.timestamp) * 1000000)", alias)
	}
	if strings.EqualFold(db.Dialector.Name(), "sqlite") {
		epochUS = "((julianday(timestamp) - 2440587.5) * 86400.0 * 1000000.0)"
	}

	withinUS := withinSec * 1_000_000

	var b strings.Builder
	var args []any

	// s1: cohort = users who did step1 in range; store t1_us.
	b.WriteString("WITH s1 AS (")
	b.WriteString(" SELECT distinct_id, CAST(MIN(" + epochUS + ") AS INTEGER) AS t1_us")
	b.WriteString(" FROM " + spec.Table)
	b.WriteString(" WHERE project_id = ? AND " + spec.RootFilter)
	b.WriteString(" AND timestamp >= ? AND timestamp <= ? AND " + spec.NameCol + " = ?")
	b.WriteString(" GROUP BY distinct_id")
	b.WriteString(")")
	args = append(args, projectID, start, end, steps[0])

	prevCTE := "s1"
	prevCol := "t1_us"
	for i := 2; i <= len(steps); i++ {
		cte := fmt.Sprintf("s%d", i)
		stepName := steps[i-1]
		curCol := fmt.Sprintf("t%d_us", i)

		b.WriteString(", ")
		b.WriteString(cte)
		b.WriteString(" AS (")
		b.WriteString(" SELECT ")
		b.WriteString(prevCTE + ".distinct_id, " + prevCTE + ".t1_us")
		for j := 2; j < i; j++ {
			b.WriteString(", " + prevCTE + fmt.Sprintf(".t%d_us", j))
		}
		b.WriteString(", (")
		b.WriteString(" SELECT CAST(MIN(" + epochUSAlias("l") + ") AS INTEGER)")
		b.WriteString(" FROM " + spec.Table + " l")
		b.WriteString(" WHERE l.project_id = ? AND " + spec.AliasFilter + " AND l.distinct_id = " + prevCTE + ".distinct_id")
		b.WriteString(" AND l.timestamp >= ? AND l.timestamp <= ? AND l." + spec.NameCol + " = ?")
		b.WriteString(" AND " + epochUSAlias("l") + " >= " + prevCTE + "." + prevCol)
		b.WriteString(" AND (? = 0 OR " + epochUSAlias("l") + " <= " + prevCTE + ".t1_us + ?)")
		b.WriteString(" ) AS " + curCol)
		b.WriteString(" FROM " + prevCTE)
		b.WriteString(")")

		args = append(args, projectID, start, end, stepName, withinUS, withinUS)
		prevCTE = cte
		prevCol = curCol
	}

	// Final counts. Each row is a step1 user; later steps are nullable.
	b.WriteString(" SELECT ")
	for i := 1; i <= len(steps); i++ {
		if i == 1 {
			b.WriteString("COUNT(*) AS step1")
			continue
		}
		col := fmt.Sprintf("t%d_us", i)
		b.WriteString(fmt.Sprintf(", SUM(CASE WHEN %s IS NOT NULL THEN 1 ELSE 0 END) AS step%d", col, i))
	}
	b.WriteString(" FROM " + prevCTE)

	var row funnelCountsRow
	if err := db.WithContext(ctx).Raw(b.String(), args...).Scan(&row).Error; err != nil {
		return nil, err
	}

	raw := []int64{row.Step1, row.Step2, row.Step3, row.Step4, row.Step5, row.Step6, row.Step7, row.Step8}
	out := make([]int64, len(steps))
	copy(out, raw[:len(steps)])
	return out, nil
}

func parseCSVSteps(raw string, maxN int) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) > 200 {
			p = p[:200]
		}
		key := strings.ToLower(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
		if len(out) >= maxN {
			break
		}
	}
	return out
}
