package query

import (
	"context"
	"net/http"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
)

func MetricsTodayHandler(recorder *metrics.RedisRecorder) gin.HandlerFunc {
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

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		events, errorsCount, users, ok, err := recorder.Today(ctx, projectID, time.Now().UTC())
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok {
			respondErr(c, http.StatusNotImplemented, "metrics not ready")
			return
		}
		respondOK(c, gin.H{
			"project_id": projectID,
			"date":       time.Now().UTC().Format("2006-01-02"),
			"events":     events,
			"errors":     errorsCount,
			"users":      users,
		})
	}
}
