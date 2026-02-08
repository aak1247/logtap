package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/aak1247/logtap/internal/identity"
	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func InsertLog(ctx context.Context, db *gorm.DB, projectID string, lp ingest.CustomLogPayload) error {
	row, err := LogRowFromPayload(projectID, lp)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Create(&row).Error
}

func LogRowFromPayload(projectID string, lp ingest.CustomLogPayload) (model.Log, error) {
	projectIDInt, err := project.ParseID(projectID)
	if err != nil {
		return model.Log{}, err
	}
	if lp.Timestamp == nil {
		return model.Log{}, errors.New("timestamp required")
	}

	fields, _ := json.Marshal(lp.Fields)
	deviceID := strings.TrimSpace(lp.DeviceID)
	distinctID := ""
	if id := identity.ExtractUserID(map[string]any{"user": lp.User}); id != "" {
		distinctID = id
	} else if deviceID != "" {
		distinctID = deviceID
	} else if v, ok := lp.Fields["device_id"]; ok {
		if s, ok := v.(string); ok {
			deviceID = strings.TrimSpace(s)
			if distinctID == "" {
				distinctID = deviceID
			}
		}
	}

	row := model.Log{
		ProjectID:  projectIDInt,
		Timestamp:  lp.Timestamp.UTC(),
		Level:      strings.TrimSpace(lp.Level),
		DistinctID: distinctID,
		DeviceID:   deviceID,
		TraceID:    strings.TrimSpace(lp.TraceID),
		SpanID:     strings.TrimSpace(lp.SpanID),
		Message:    lp.Message,
		Fields:     datatypes.JSON(fields),
	}
	return row, nil
}

func LogRowFromPayloadWithIngestID(projectID string, lp ingest.CustomLogPayload, ingestID uuid.UUID) (model.Log, error) {
	row, err := LogRowFromPayload(projectID, lp)
	if err != nil {
		return model.Log{}, err
	}
	row.IngestID = &ingestID
	return row, nil
}
