package query

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/project"
	"github.com/aak1247/logtap/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DELETE /api/:projectId/logs/cleanup?before=RFC3339
func CleanupLogsHandler(db *gorm.DB) gin.HandlerFunc {
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
		beforeRaw := strings.TrimSpace(c.Query("before"))
		if beforeRaw == "" {
			respondErr(c, http.StatusBadRequest, "before required")
			return
		}
		before, ok := parseTime(beforeRaw)
		if !ok {
			respondErr(c, http.StatusBadRequest, "invalid before")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		n, err := store.DeleteLogsBefore(ctx, db, projectID, before)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"deleted": n})
	}
}

// DELETE /api/:projectId/events/cleanup?before=RFC3339
func CleanupEventsHandler(db *gorm.DB) gin.HandlerFunc {
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
		beforeRaw := strings.TrimSpace(c.Query("before"))
		if beforeRaw == "" {
			respondErr(c, http.StatusBadRequest, "before required")
			return
		}
		before, ok := parseTime(beforeRaw)
		if !ok {
			respondErr(c, http.StatusBadRequest, "invalid before")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		n, err := store.DeleteEventsBefore(ctx, db, projectID, before)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"deleted": n})
	}
}
