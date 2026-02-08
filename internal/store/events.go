package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aak1247/logtap/internal/identity"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func InsertEvent(ctx context.Context, db *gorm.DB, projectID string, event map[string]any) error {
	row, err := EventRowFromMap(projectID, event)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func EventRowFromMap(projectID string, event map[string]any) (model.Event, error) {
	projectIDInt, err := project.ParseID(projectID)
	if err != nil {
		return model.Event{}, err
	}

	eventID, _ := event["event_id"].(string)
	if eventID == "" {
		eventID = uuid.NewString()
		event["event_id"] = eventID
	}
	if _, err := uuid.Parse(eventID); err != nil {
		// Keep ID stable but ensure it's storable as UUID.
		eventID = uuid.NewSHA1(uuid.Nil, []byte(eventID)).String()
		event["event_id"] = eventID
	}

	ts := parseSentryTimestamp(event["timestamp"])
	level, _ := event["level"].(string)
	distinctID, _ := identity.ExtractDistinctID(event)
	deviceID := identity.ExtractDeviceID(event)
	osName := identity.ExtractOS(event)
	platform, _ := event["platform"].(string)
	releaseTag, _ := event["release"].(string)
	environment, _ := event["environment"].(string)
	userID := extractUserID(event["user"])
	title := extractTitle(event)

	data, _ := json.Marshal(event)

	id, _ := uuid.Parse(eventID)
	row := model.Event{
		ID:          id,
		ProjectID:   projectIDInt,
		Timestamp:   ts,
		Level:       level,
		DistinctID:  distinctID,
		DeviceID:    deviceID,
		OS:          osName,
		Platform:    platform,
		ReleaseTag:  releaseTag,
		Environment: environment,
		UserID:      userID,
		Title:       title,
		Data:        datatypes.JSON(data),
	}
	return row, nil
}

func parseSentryTimestamp(v any) time.Time {
	now := time.Now().UTC()
	switch t := v.(type) {
	case string:
		if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return ts.UTC()
		}
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			return ts.UTC()
		}
	case float64:
		sec := int64(t)
		nsec := int64((t - float64(sec)) * 1e9)
		return time.Unix(sec, nsec).UTC()
	case int64:
		return time.Unix(t, 0).UTC()
	case json.Number:
		if f, err := t.Float64(); err == nil {
			sec := int64(f)
			nsec := int64((f - float64(sec)) * 1e9)
			return time.Unix(sec, nsec).UTC()
		}
	}
	return now
}

func extractUserID(v any) string {
	user, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if id, _ := user["id"].(string); id != "" {
		return id
	}
	if username, _ := user["username"].(string); username != "" {
		return username
	}
	if email, _ := user["email"].(string); email != "" {
		return email
	}
	return ""
}

func extractTitle(event map[string]any) string {
	if msg, _ := event["message"].(string); msg != "" {
		return msg
	}
	exc, ok := event["exception"].(map[string]any)
	if !ok {
		return ""
	}
	values, ok := exc["values"].([]any)
	if !ok || len(values) == 0 {
		return ""
	}
	first, ok := values[0].(map[string]any)
	if !ok {
		return ""
	}
	typ, _ := first["type"].(string)
	val, _ := first["value"].(string)
	switch {
	case typ != "" && val != "":
		return fmt.Sprintf("%s: %s", typ, val)
	case typ != "":
		return typ
	case val != "":
		return val
	default:
		return ""
	}
}
