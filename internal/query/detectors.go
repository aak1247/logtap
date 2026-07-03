package query

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/aak1247/logtap/internal/detector"
	"github.com/gin-gonic/gin"
)

func ListDetectorsHandler(svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		items, err := svc.ListDescriptors()
		if err != nil {
			if errors.Is(err, detector.ErrServiceNotConfigured) {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func ListDetectorPackagesHandler(svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		items, err := svc.ListPackages()
		if err != nil {
			if errors.Is(err, detector.ErrServiceNotConfigured) {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func ListDetectorViewsHandler(svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		items, err := svc.ListViews(c.Param("detectorType"))
		if err != nil {
			if errors.Is(err, detector.ErrServiceNotConfigured) {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func ListPluginViewsHandler(svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		items, err := svc.ListViews("")
		if err != nil {
			if errors.Is(err, detector.ErrServiceNotConfigured) {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func GetDetectorSchemaHandler(svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		schema, err := svc.GetSchema(c.Param("detectorType"))
		if err != nil {
			switch {
			case errors.Is(err, detector.ErrServiceNotConfigured):
				respondErr(c, http.StatusServiceUnavailable, err.Error())
			case errors.Is(err, detector.ErrDetectorNotFound):
				respondErr(c, http.StatusNotFound, err.Error())
			default:
				respondErr(c, http.StatusServiceUnavailable, err.Error())
			}
			return
		}
		respondOK(c, gin.H{
			"detectorType": c.Param("detectorType"),
			"schema":       schema,
		})
	}
}

func DetectorHealthHandler(svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		if err := svc.HealthCheck(c.Request.Context(), c.Param("detectorType")); err != nil {
			switch {
			case errors.Is(err, detector.ErrDetectorNotFound):
				respondErr(c, http.StatusNotFound, err.Error())
			default:
				respondErr(c, http.StatusServiceUnavailable, err.Error())
			}
			return
		}
		respondOK(c, gin.H{"status": "healthy"})
	}
}

func DetectorAggregateHandler(svc *detector.Service, store *detector.ResultStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}

		projectID, err := strconv.Atoi(c.Query("project_id"))
		if err != nil || projectID <= 0 {
			respondErr(c, http.StatusBadRequest, "project_id is required")
			return
		}

		startStr := c.Query("start")
		endStr := c.Query("end")
		if startStr == "" || endStr == "" {
			respondErr(c, http.StatusBadRequest, "start and end are required")
			return
		}
		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			respondErr(c, http.StatusBadRequest, "invalid start time, use RFC3339")
			return
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			respondErr(c, http.StatusBadRequest, "invalid end time, use RFC3339")
			return
		}

		interval := detector.AggregateInterval(c.Query("interval"))
		if interval == "" {
			interval = detector.IntervalHour
		}
		monitorID := 0
		if raw := c.Query("monitor_id"); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v <= 0 {
				respondErr(c, http.StatusBadRequest, "invalid monitor_id")
				return
			}
			monitorID = v
		}

		tr := detector.TimeRange{Start: start, End: end}

		// First try the plugin's own Aggregate method
		var aggregateErr error
		if monitorID == 0 {
			var points []detector.MetricPoint
			points, aggregateErr = svc.Aggregate(c.Request.Context(), c.Param("detectorType"), projectID, tr, interval)
			if aggregateErr == nil {
				respondOK(c, gin.H{"points": points})
				return
			}
		}

		// Fallback to generic store-based aggregation
		if store != nil {
			elapsed, e1 := store.AggregateAvgFloat(c.Request.Context(), c.Param("detectorType"), projectID, monitorID, "elapsed_ms", tr, interval)
			success, e2 := store.AggregateSuccessRate(c.Request.Context(), c.Param("detectorType"), projectID, monitorID, tr, interval)
			if e1 == nil && e2 == nil {
				respondOK(c, gin.H{
					"elapsed_ms":   elapsed,
					"success_rate": success,
				})
				return
			}
		}

		errMsg := "detector aggregate not available"
		if aggregateErr != nil {
			errMsg = aggregateErr.Error()
		}
		respondErr(c, http.StatusServiceUnavailable, errMsg)
	}
}
