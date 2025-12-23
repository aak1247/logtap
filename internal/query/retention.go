package query

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
)

// GET /api/:projectId/analytics/retention?start=RFC3339&end=RFC3339&days=1,7,30
func RetentionHandler(recorder *metrics.RedisRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if recorder == nil {
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

		rows, err := recorder.Retention(ctx, projectID, start, end, days)
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
