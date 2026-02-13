package selflog

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/queue"
	"gorm.io/gorm"
)

type Writer struct {
	db        *gorm.DB
	publisher queue.Publisher
	component string

	ch chan string

	mu         sync.Mutex
	cachedPID  int
	lastLookup time.Time
}

func NewWriter(db *gorm.DB, publisher queue.Publisher, component string) *Writer {
	w := &Writer{
		db:        db,
		publisher: publisher,
		component: strings.TrimSpace(component),
		ch:        make(chan string, 256),
	}
	go w.run()
	return w
}

func (w *Writer) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	s := strings.TrimSpace(string(p))
	if s == "" {
		return len(p), nil
	}
	select {
	case w.ch <- s:
	default:
	}
	return len(p), nil
}

func (w *Writer) run() {
	for line := range w.ch {
		pid := w.defaultProjectID()
		if pid <= 0 || w.publisher == nil {
			continue
		}

		now := time.Now().UTC()
		fields := map[string]any{
			"source":    "self",
			"component": w.component,
		}
		payloadBytes, _ := json.Marshal(ingest.CustomLogPayload{
			Level:     "info",
			Message:   line,
			Fields:    fields,
			Timestamp: &now,
		})
		msgBytes, _ := json.Marshal(ingest.NSQMessage{
			Type:      "log",
			ProjectID: strconv.Itoa(pid),
			Received:  now,
			Payload:   payloadBytes,
		})
		_ = w.publisher.Publish("logs", msgBytes)
	}
}

func (w *Writer) defaultProjectID() int {
	if w == nil || w.db == nil {
		return 0
	}
	const ttl = 30 * time.Second

	w.mu.Lock()
	if w.cachedPID > 0 && time.Since(w.lastLookup) < ttl {
		pid := w.cachedPID
		w.mu.Unlock()
		return pid
	}
	w.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var p model.Project
	err := w.db.WithContext(ctx).
		Where("is_system = ?", true).
		Order("id ASC").
		First(&p).Error
	if err != nil {
		// Fallback to default project.
		err2 := w.db.WithContext(ctx).
			Where("is_default = ?", true).
			Order("id ASC").
			First(&p).Error
		if err2 != nil {
			// Backward-compatible: if there's exactly one project, treat it as default.
			var n int64
			if err := w.db.WithContext(ctx).Model(&model.Project{}).Count(&n).Error; err != nil || n != 1 {
				w.mu.Lock()
				w.cachedPID = 0
				w.lastLookup = time.Now()
				w.mu.Unlock()
				return 0
			}
			if err := w.db.WithContext(ctx).
				Order("id ASC").
				First(&p).Error; err != nil {
				w.mu.Lock()
				w.cachedPID = 0
				w.lastLookup = time.Now()
				w.mu.Unlock()
				return 0
			}
			_ = w.db.WithContext(ctx).Model(&model.Project{}).Where("id = ?", p.ID).Update("is_default", true).Error
		}
	}

	w.mu.Lock()
	w.cachedPID = p.ID
	w.lastLookup = time.Now()
	w.mu.Unlock()
	return p.ID
}
