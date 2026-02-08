package query

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func GetEventHandler(db *gorm.DB) gin.HandlerFunc {
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
		eventID := strings.TrimSpace(c.Param("eventId"))
		eid, err := uuid.Parse(eventID)
		if err != nil {
			respondErr(c, http.StatusBadRequest, "invalid eventId")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var e model.Event
		if err := db.WithContext(ctx).
			Select("data").
			Where("project_id = ? AND id = ?", projectID, eid).
			First(&e).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, json.RawMessage(e.Data))
	}
}

func RecentEventsHandler(db *gorm.DB) gin.HandlerFunc {
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
		limit := parseLimit(c.Query("limit"), 50, 500)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		type row struct {
			ID        string    `json:"id"`
			Timestamp time.Time `json:"timestamp"`
			Level     string    `json:"level,omitempty"`
			Title     string    `json:"title,omitempty"`
		}
		type dbRow struct {
			ID        uuid.UUID
			Timestamp time.Time
			Level     string
			Title     string
		}
		var rows []dbRow
		if err := db.WithContext(ctx).
			Model(&model.Event{}).
			Select("id, timestamp, level, title").
			Where("project_id = ?", projectID).
			Order("timestamp DESC").
			Limit(limit).
			Find(&rows).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		out := make([]row, 0, len(rows))
		for _, r := range rows {
			out = append(out, row{
				ID:        r.ID.String(),
				Timestamp: r.Timestamp,
				Level:     r.Level,
				Title:     r.Title,
			})
		}
		respondOK(c, out)
	}
}

func SearchLogsHandler(db *gorm.DB) gin.HandlerFunc {
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

		q := strings.TrimSpace(c.Query("q"))
		mode := strings.ToLower(strings.TrimSpace(c.Query("mode")))
		traceID := strings.TrimSpace(c.Query("trace_id"))
		level := strings.TrimSpace(c.Query("level"))
		start, _ := parseTime(c.Query("start"))
		end, _ := parseTime(c.Query("end"))
		limit := parseLimit(c.Query("limit"), 100, 500)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		qdb := db.WithContext(ctx).Model(&model.Log{}).Where("project_id = ?", projectID)
		if !start.IsZero() {
			qdb = qdb.Where("timestamp >= ?", start)
		}
		if !end.IsZero() {
			qdb = qdb.Where("timestamp <= ?", end)
		}
		if traceID != "" {
			qdb = qdb.Where("trace_id = ?", traceID)
		}
		if level != "" {
			qdb = qdb.Where("level = ?", level)
		}
		if q != "" {
			useFTS := (mode == "" || mode == "fts") && strings.EqualFold(db.Dialector.Name(), "postgres")
			if useFTS {
				qdb = qdb.Where(
					"to_tsvector('simple', coalesce(message,'') || ' ' || coalesce(fields::text,'')) @@ plainto_tsquery('simple', ?)",
					q,
				)
			} else {
				pat := "%" + q + "%"
				qdb = qdb.Where(db.Where("message ILIKE ?", pat).Or("fields::text ILIKE ?", pat))
			}
		}

		type row struct {
			ID        int64
			Timestamp time.Time
			Level     string
			TraceID   string
			SpanID    string
			Message   string
			Fields    datatypes.JSON
		}
		var rows []row
		if err := qdb.
			Select("id, timestamp, level, trace_id, span_id, message, fields").
			Order("timestamp DESC").
			Limit(limit).
			Find(&rows).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		out := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			entry := map[string]any{
				"id":        r.ID,
				"timestamp": r.Timestamp,
				"level":     r.Level,
				"trace_id":  r.TraceID,
				"span_id":   r.SpanID,
				"message":   r.Message,
			}
			if len(r.Fields) > 0 && string(r.Fields) != "null" && string(r.Fields) != "{}" {
				var fields map[string]any
				_ = json.Unmarshal(r.Fields, &fields)
				if len(fields) > 0 {
					entry["fields"] = fields
				}
			}
			out = append(out, entry)
		}
		respondOK(c, out)
	}
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

func parseLimit(s string, def, max int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
