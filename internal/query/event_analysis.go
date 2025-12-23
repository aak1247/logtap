package query

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
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
		qdb := db.WithContext(ctx).Table("logs").
			Select("message as name, COUNT(*) as events, COUNT(DISTINCT distinct_id) as users").
			Where("project_id = ? AND level = ? AND distinct_id IS NOT NULL AND distinct_id <> '' AND timestamp >= ? AND timestamp <= ?", projectID, "event", start, end).
			Group("message").
			Order("events DESC, message ASC").
			Limit(limit)
		if q != "" {
			qdb = qdb.Where("message ILIKE ?", "%"+q+"%")
		}
		if err := qdb.Scan(&out).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
			"items":      out,
		})
	}
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

		type logRow struct {
			DistinctID string
			Timestamp  time.Time
			Message    string
		}
		var rows []logRow
		if err := db.WithContext(ctx).
			Model(&model.Log{}).
			Select("distinct_id, timestamp, message").
			Where("project_id = ? AND level = ? AND distinct_id IS NOT NULL AND distinct_id <> '' AND timestamp >= ? AND timestamp <= ? AND message IN ?", projectID, "event", start, end, steps).
			Order("distinct_id ASC").
			Order("timestamp ASC").
			Find(&rows).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		window := time.Duration(withinSec) * time.Second
		counts := make([]int64, len(steps))

		curID := ""
		reached := 0
		var t0 time.Time
		var last time.Time
		active := true

		flush := func() {
			if curID == "" {
				return
			}
			for i := 0; i < reached; i++ {
				counts[i]++
			}
		}

		for _, r := range rows {
			if r.DistinctID != curID {
				flush()
				curID = r.DistinctID
				reached = 0
				t0 = time.Time{}
				last = time.Time{}
				active = true
			}
			if !active || reached >= len(steps) {
				continue
			}
			want := steps[reached]
			if r.Message != want {
				continue
			}
			if reached == 0 {
				t0 = r.Timestamp
				last = r.Timestamp
				reached = 1
				continue
			}
			if withinSec > 0 && !t0.IsZero() && r.Timestamp.After(t0.Add(window)) {
				active = false
				continue
			}
			if !last.IsZero() && r.Timestamp.Before(last) {
				continue
			}
			last = r.Timestamp
			reached++
		}
		flush()

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
			"steps":       out,
		})
	}
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
