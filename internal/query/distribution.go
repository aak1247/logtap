package query

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
)

// GET /api/:projectId/analytics/dist?dim=os|browser|country|region|city|asn_org&metric=events|users&start=RFC3339&end=RFC3339&limit=10
func DistributionHandler(recorder *metrics.RedisRecorder) gin.HandlerFunc {
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

		dim := strings.ToLower(strings.TrimSpace(c.Query("dim")))
		switch dim {
		case "os", "browser", "country", "region", "city", "asn_org":
		default:
			respondErr(c, http.StatusBadRequest, "invalid dim")
			return
		}

		limit := 10
		if s := strings.TrimSpace(c.Query("limit")); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				limit = n
			}
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
		start = clampDistSpan(start, end, "")
		metric := parseDistMetric(c.Query("metric"))

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		items, err := recorder.DistributionMetric(ctx, projectID, dim, start, end, limit, metric)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"dim":        dim,
			"metric":     metric,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
			"items":      items,
		})
	}
}

// GET /api/:projectId/analytics/dist/series?dim=country|region|city&metric=events|users&bucket=day|week|month|year&start=RFC3339&end=RFC3339&limit=10
func DistributionSeriesHandler(recorder *metrics.RedisRecorder) gin.HandlerFunc {
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

		dim := strings.ToLower(strings.TrimSpace(c.Query("dim")))
		switch dim {
		case "os", "browser", "country", "region", "city", "asn_org":
		default:
			respondErr(c, http.StatusBadRequest, "invalid dim")
			return
		}

		bucket := strings.ToLower(strings.TrimSpace(c.Query("bucket")))
		switch bucket {
		case "week", "month", "year":
		default:
			bucket = "day"
		}

		limit := 10
		if s := strings.TrimSpace(c.Query("limit")); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				limit = n
			}
		}

		now := time.Now().UTC()
		start, okStart := parseTime(c.Query("start"))
		end, okEnd := parseTime(c.Query("end"))
		if !okEnd {
			end = now
		}
		if !okStart {
			start = defaultDistributionStart(end, bucket)
		}
		start = clampDistSpan(start, end, bucket)
		metric := parseDistMetric(c.Query("metric"))

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		series, err := recorder.DistributionSeries(ctx, projectID, dim, start, end, bucket, limit, metric)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"dim":        dim,
			"metric":     metric,
			"bucket":     bucket,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
			"series":     series,
		})
	}
}

func parseDistMetric(metric string) string {
	if strings.EqualFold(strings.TrimSpace(metric), "users") {
		return "users"
	}
	return "events"
}

func defaultDistributionStart(end time.Time, bucket string) time.Time {
	switch bucket {
	case "year":
		return end.AddDate(-4, 0, 0)
	case "month":
		return end.AddDate(0, -11, 0)
	case "week":
		return end.AddDate(0, 0, -7*11)
	default:
		return end.AddDate(0, 0, -29)
	}
}

// clampDistSpan bounds the requested range so one dashboard request cannot
// make the metrics layer iterate an unbounded number of days of Redis keys.
func clampDistSpan(start, end time.Time, bucket string) time.Time {
	var maxSpan time.Duration
	switch bucket {
	case "week":
		maxSpan = 3 * 365 * 24 * time.Hour
	case "month":
		maxSpan = 2 * 365 * 24 * time.Hour
	case "year":
		maxSpan = 10 * 365 * 24 * time.Hour
	default:
		maxSpan = 180 * 24 * time.Hour
	}
	if minStart := end.Add(-maxSpan); start.Before(minStart) {
		return minStart
	}
	return start
}
