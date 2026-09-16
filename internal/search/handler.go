package search

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
)

// SearchHandler returns a gin handler that uses SearchEngine for unified search.
func SearchHandler(engine *SearchEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		if engine == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "search not configured"})
			return
		}

		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		q := c.Query("q")
		start, okStart := parseTimeQS(c.Query("start"))
		end, okEnd := parseTimeQS(c.Query("end"))
		if !okEnd {
			end = time.Now().UTC()
		}
		if !okStart {
			start = end.AddDate(0, 0, -29)
		}
		if end.Before(start) {
			start, end = end, start
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))

		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 50
		}
		if pageSize > 500 {
			pageSize = 500
		}

		// Bound slow ILIKE scans so they cannot hold a pool connection until
		// the client disconnects.
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		result, err := engine.Search(ctx, q, projectID, TimeRange{
			Start: start,
			End:   end,
		}, page, pageSize)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"total":  result.Total,
				"items":  result.Hits,
				"hits":   result.Hits,
				"facets": flattenFacets(result.Facets),
			},
		})
	}
}

func flattenFacets(facets map[string]Facet) map[string][]FacetBucket {
	if len(facets) == 0 {
		return nil
	}
	out := make(map[string][]FacetBucket, len(facets))
	for field, facet := range facets {
		out[field] = facet.Buckets
	}
	return out
}

func parseTimeQS(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
