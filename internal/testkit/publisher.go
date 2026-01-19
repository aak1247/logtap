package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/store"
	"gorm.io/gorm"
)

// InlinePublisher bypasses NSQ in tests by writing directly to the DB.
type InlinePublisher struct {
	DB *gorm.DB
}

func (p *InlinePublisher) Publish(_ string, body []byte) error {
	if p.DB == nil {
		return errors.New("testkit: DB is nil")
	}

	var msg ingest.NSQMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	switch msg.Type {
	case "log":
		var lp ingest.CustomLogPayload
		if err := json.Unmarshal(msg.Payload, &lp); err != nil {
			return err
		}
		return store.InsertLog(ctx, p.DB, msg.ProjectID, lp)
	case "event", "envelope":
		var ev map[string]any
		if err := json.Unmarshal(msg.Payload, &ev); err != nil {
			return err
		}
		return store.InsertEvent(ctx, p.DB, msg.ProjectID, ev)
	default:
		return nil
	}
}
