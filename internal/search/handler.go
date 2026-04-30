package search

import (
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
		start, _ := parseTimeQS(c.Query("start"))
		end, _ := parseTimeQS(c.Query("end"))
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

		result, err := engine.Search(c.Request.Context(), q, projectID, TimeRange{
			Start: start,
			End:   end,
		}, page, pageSize)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
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
