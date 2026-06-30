package query

import (
	"context"
	"net/http"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/project"
	"github.com/aak1247/logtap/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func MetricsTodayHandler(recorder *metrics.RedisRecorder, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if recorder == nil && db == nil {
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

		now := time.Now().UTC()
		logs, events, errorsCount, users, ok, err := metricsToday(ctx, recorder, db, projectID, now)
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
			"date":       now.Format("2006-01-02"),
			"logs":       logs,
			"events":     events,
			"errors":     errorsCount,
			"users":      users,
		})
	}
}

func MetricsTotalHandler(recorder *metrics.RedisRecorder, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if recorder == nil && db == nil {
			respondErr(c, http.StatusNotImplemented, "metrics not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		logs, events, users, ok, err := metricsTotal(ctx, recorder, db, projectID)
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
			"logs":       logs,
			"events":     events,
			"users":      users,
		})
	}
}

func metricsToday(ctx context.Context, recorder *metrics.RedisRecorder, db *gorm.DB, projectID int, now time.Time) (logs int64, events int64, errorsCount int64, users int64, ok bool, err error) {
	if db != nil {
		row, ok, err := store.GetDBMetricsToday(ctx, db, projectID, now)
		if err != nil || ok {
			return row.Logs, row.Events, row.Errors, row.Users, ok, err
		}
	}
	if recorder == nil {
		return 0, 0, 0, 0, false, nil
	}
	return recorder.Today(ctx, projectID, now)
}

func metricsTotal(ctx context.Context, recorder *metrics.RedisRecorder, db *gorm.DB, projectID int) (logs int64, events int64, users int64, ok bool, err error) {
	if db != nil {
		row, ok, err := store.GetDBMetricsTotal(ctx, db, projectID)
		if err != nil || ok {
			return row.Logs, row.Events, row.Users, ok, err
		}
	}
	if recorder == nil {
		return 0, 0, 0, false, nil
	}
	return recorder.Total(ctx, projectID)
}
