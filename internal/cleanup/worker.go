package cleanup

import (
	"context"
	"log"
	"time"

	"github.com/aak1247/logtap/internal/obs"
	"github.com/aak1247/logtap/internal/store"
	"gorm.io/gorm"
)

type Worker struct {
	DB              *gorm.DB
	Interval        time.Duration
	Limit           int
	DeleteBatchSize int
	MaxBatches      int
	BatchSleep      time.Duration
	Stats           *obs.Stats
}

func NewWorker(db *gorm.DB) *Worker {
	return &Worker{
		DB:              db,
		Interval:        10 * time.Minute,
		Limit:           50,
		DeleteBatchSize: 5000,
		MaxBatches:      50,
		BatchSleep:      0,
	}
}

func (w *Worker) Run(ctx context.Context) {
	if w == nil || w.DB == nil {
		return
	}
	_ = w.runOnce(ctx)

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.runOnce(ctx)
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) error {
	now := time.Now().UTC()
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	policies, err := store.ListCleanupPoliciesDue(runCtx, w.DB, now, w.Limit)
	if err != nil {
		log.Printf("cleanup: list due policies: %v", err)
		return err
	}
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		projectID := p.ProjectID
		projectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := w.runPolicy(projectCtx, projectID, p.LogsRetentionDays, p.EventsRetentionDays, p.TrackEventsRetentionDays, p.ScheduleHourUTC, p.ScheduleMinuteUTC)
		cancel()
		if err != nil {
			log.Printf("cleanup: project=%d: %v", projectID, err)
		}
	}
	return nil
}

func (w *Worker) runPolicy(ctx context.Context, projectID int, logsDays int, eventsDays int, trackEventsDays int, hourUTC int, minuteUTC int) error {
	now := time.Now().UTC()
	maxBatches := w.MaxBatches
	if maxBatches <= 0 {
		maxBatches = 1
	}
	batchSize := w.DeleteBatchSize
	if batchSize <= 0 {
		batchSize = 5000
	}

	incomplete := false
	if logsDays > 0 {
		before := now.Add(-time.Duration(logsDays) * 24 * time.Hour)
		var last int64
		for i := 0; i < maxBatches; i++ {
			n, err := store.DeleteLogsBeforeBatched(ctx, w.DB, projectID, before, batchSize)
			if err != nil {
				return err
			}
			if w.Stats != nil && n > 0 {
				w.Stats.ObserveCleanupDeleted(n, 0)
			}
			last = n
			if n == 0 {
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if w.BatchSleep > 0 {
				time.Sleep(w.BatchSleep)
			}
		}
		if last > 0 {
			incomplete = true
		}
	}
	if eventsDays > 0 {
		before := now.Add(-time.Duration(eventsDays) * 24 * time.Hour)
		var last int64
		for i := 0; i < maxBatches; i++ {
			n, err := store.DeleteEventsBeforeBatched(ctx, w.DB, projectID, before, batchSize)
			if err != nil {
				return err
			}
			if w.Stats != nil && n > 0 {
				w.Stats.ObserveCleanupDeleted(0, n)
			}
			last = n
			if n == 0 {
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if w.BatchSleep > 0 {
				time.Sleep(w.BatchSleep)
			}
		}
		if last > 0 {
			incomplete = true
		}
	}
	if trackEventsDays > 0 {
		before := now.Add(-time.Duration(trackEventsDays) * 24 * time.Hour)
		var last int64
		for i := 0; i < maxBatches; i++ {
			n, err := store.DeleteTrackEventsBeforeBatched(ctx, w.DB, projectID, before, batchSize)
			if err != nil {
				return err
			}
			last = n
			if n == 0 {
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if w.BatchSleep > 0 {
				time.Sleep(w.BatchSleep)
			}
		}
		if last > 0 {
			incomplete = true
		}
	}
	if incomplete {
		next := now.Add(w.Interval)
		if !next.After(now) {
			next = now.Add(10 * time.Minute)
		}
		return store.MarkCleanupPolicyInProgress(ctx, w.DB, projectID, now, next)
	}
	return store.MarkCleanupPolicyRun(ctx, w.DB, projectID, now, hourUTC, minuteUTC)
}
