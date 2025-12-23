package query

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
)

// GET /api/:projectId/analytics/active?bucket=day|month&start=RFC3339&end=RFC3339
func ActiveSeriesHandler(recorder *metrics.RedisRecorder) gin.HandlerFunc {
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

		series, err := recorder.ActiveSeries(ctx, projectID, start, end, bucket)
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
