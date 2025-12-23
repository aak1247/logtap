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

// GET /api/:projectId/analytics/dist?dim=os|browser|country|region|city|asn_org&start=RFC3339&end=RFC3339&limit=10
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

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		items, err := recorder.Distribution(ctx, projectID, dim, start, end, limit)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"dim":        dim,
			"start":      start.UTC().Format(time.RFC3339),
			"end":        end.UTC().Format(time.RFC3339),
			"items":      items,
		})
	}
}
