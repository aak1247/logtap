package query

import (
	"context"
	"net/http"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/aak1247/logtap/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetCleanupPolicyHandler(db *gorm.DB) gin.HandlerFunc {
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

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		row, ok, err := store.GetCleanupPolicy(ctx, db, projectID)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok {
			saved, err := store.UpsertCleanupPolicy(ctx, db, defaultCleanupPolicy(projectID))
			if err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			respondOK(c, saved)
			return
		}
		respondOK(c, row)
	}
}

func UpsertCleanupPolicyHandler(db *gorm.DB) gin.HandlerFunc {
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

		var req struct {
			Enabled                  *bool `json:"enabled"`
			LogsRetentionDays        *int  `json:"logs_retention_days"`
			EventsRetentionDays      *int  `json:"events_retention_days"`
			TrackEventsRetentionDays *int  `json:"track_events_retention_days"`
			ScheduleHourUTC          *int  `json:"schedule_hour_utc"`
			ScheduleMinuteUTC        *int  `json:"schedule_minute_utc"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		cur, ok, err := store.GetCleanupPolicy(ctx, db, projectID)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok {
			cur = defaultCleanupPolicy(projectID)
		}
		next := cur
		next.ProjectID = projectID
		if req.Enabled != nil {
			next.Enabled = *req.Enabled
		}
		if req.LogsRetentionDays != nil {
			next.LogsRetentionDays = *req.LogsRetentionDays
		}
		if req.EventsRetentionDays != nil {
			next.EventsRetentionDays = *req.EventsRetentionDays
		}
		if req.TrackEventsRetentionDays != nil {
			next.TrackEventsRetentionDays = *req.TrackEventsRetentionDays
		}
		if req.ScheduleHourUTC != nil {
			next.ScheduleHourUTC = *req.ScheduleHourUTC
		}
		if req.ScheduleMinuteUTC != nil {
			next.ScheduleMinuteUTC = *req.ScheduleMinuteUTC
		}

		if next.LogsRetentionDays < 0 || next.LogsRetentionDays > 3650 {
			respondErr(c, http.StatusBadRequest, "invalid logs_retention_days (expected 0..3650)")
			return
		}
		if next.EventsRetentionDays < 0 || next.EventsRetentionDays > 3650 {
			respondErr(c, http.StatusBadRequest, "invalid events_retention_days (expected 0..3650)")
			return
		}
		if next.TrackEventsRetentionDays < 0 || next.TrackEventsRetentionDays > 3650 {
			respondErr(c, http.StatusBadRequest, "invalid track_events_retention_days (expected 0..3650)")
			return
		}
		if next.ScheduleHourUTC < 0 || next.ScheduleHourUTC > 23 {
			respondErr(c, http.StatusBadRequest, "invalid schedule_hour_utc (expected 0..23)")
			return
		}
		if next.ScheduleMinuteUTC < 0 || next.ScheduleMinuteUTC > 59 {
			respondErr(c, http.StatusBadRequest, "invalid schedule_minute_utc (expected 0..59)")
			return
		}

		saved, err := store.UpsertCleanupPolicy(ctx, db, next)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, saved)
	}
}

func RunCleanupPolicyHandler(db *gorm.DB) gin.HandlerFunc {
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

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		policy, ok, err := store.GetCleanupPolicy(ctx, db, projectID)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}
		if !policy.Enabled {
			respondErr(c, http.StatusBadRequest, "cleanup policy is disabled")
			return
		}

		now := time.Now().UTC()
		var logsDeleted int64
		var eventsDeleted int64
		var trackEventsDeleted int64
		var logsBefore string
		var eventsBefore string
		var trackEventsBefore string

		if policy.LogsRetentionDays > 0 {
			before := now.Add(-time.Duration(policy.LogsRetentionDays) * 24 * time.Hour)
			logsBefore = before.Format(time.RFC3339)
			n, err := store.DeleteLogsBefore(ctx, db, projectID, before)
			if err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			logsDeleted = n
		}
		if policy.EventsRetentionDays > 0 {
			before := now.Add(-time.Duration(policy.EventsRetentionDays) * 24 * time.Hour)
			eventsBefore = before.Format(time.RFC3339)
			n, err := store.DeleteEventsBefore(ctx, db, projectID, before)
			if err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			eventsDeleted = n
		}
		if policy.TrackEventsRetentionDays > 0 {
			before := now.Add(-time.Duration(policy.TrackEventsRetentionDays) * 24 * time.Hour)
			trackEventsBefore = before.Format(time.RFC3339)
			n, err := store.DeleteTrackEventsBefore(ctx, db, projectID, before)
			if err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			trackEventsDeleted = n
		}

		_ = store.MarkCleanupPolicyRun(ctx, db, projectID, now, policy.ScheduleHourUTC, policy.ScheduleMinuteUTC)

		respondOK(c, gin.H{
			"project_id":           projectID,
			"logs_deleted":         logsDeleted,
			"events_deleted":       eventsDeleted,
			"track_events_deleted": trackEventsDeleted,
			"logs_before":          logsBefore,
			"events_before":        eventsBefore,
			"track_events_before":  trackEventsBefore,
			"ran_at":               now.Format(time.RFC3339),
		})
	}
}

func defaultCleanupPolicy(projectID int) model.CleanupPolicy {
	return model.CleanupPolicy{
		ProjectID:                projectID,
		Enabled:                  false,
		LogsRetentionDays:        30,
		EventsRetentionDays:      30,
		TrackEventsRetentionDays: 0,
		ScheduleHourUTC:          3,
		ScheduleMinuteUTC:        0,
	}
}
