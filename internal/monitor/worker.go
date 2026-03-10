package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Worker struct {
	DB            *gorm.DB
	Registry      *detector.Registry
	Engine        *alert.Engine
	TickInterval  time.Duration
	BatchSize     int
	LeaseDuration time.Duration
	NodeID        string
	Now           func() time.Time
}

func NewWorker(db *gorm.DB, registry *detector.Registry) *Worker {
	nodeID := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if nodeID == "" {
		nodeID = "local"
	}
	nowFn := time.Now
	return &Worker{
		DB:            db,
		Registry:      registry,
		Engine:        alert.NewEngine(db),
		TickInterval:  2 * time.Second,
		BatchSize:     20,
		LeaseDuration: 60 * time.Second,
		NodeID:        nodeID,
		Now:           nowFn,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.DB == nil {
		return nil
	}
	if w.TickInterval <= 0 {
		w.TickInterval = 2 * time.Second
	}
	ticker := time.NewTicker(w.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := w.RunOnce(ctx); err != nil {
				// Continue running even if one cycle fails.
				continue
			}
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	if w == nil || w.DB == nil {
		return 0, nil
	}
	now := w.nowUTC()
	items, err := w.claimDue(ctx, now)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range items {
		processed++
		if err := w.executeOne(ctx, item); err != nil {
			return processed, err
		}
	}
	return processed, nil
}

func (w *Worker) claimDue(ctx context.Context, now time.Time) ([]model.MonitorDefinition, error) {
	limit := w.BatchSize
	if limit <= 0 {
		limit = 20
	}
	leaseDur := w.LeaseDuration
	if leaseDur <= 0 {
		leaseDur = 60 * time.Second
	}
	leaseUntil := now.Add(leaseDur)

	var items []model.MonitorDefinition
	isPostgres := strings.EqualFold(strings.TrimSpace(w.DB.Dialector.Name()), "postgres")
	err := w.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.WithContext(ctx).
			Where("enabled = ? AND next_run_at <= ? AND (lease_until IS NULL OR lease_until <= ?)", true, now, now).
			Order("id ASC").
			Limit(limit)
		if isPostgres {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := q.Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		claimed := make([]model.MonitorDefinition, 0, len(items))
		for _, item := range items {
			res := tx.WithContext(ctx).
				Model(&model.MonitorDefinition{}).
				Where("id = ? AND (lease_until IS NULL OR lease_until <= ?)", item.ID, now).
				Updates(map[string]any{
					"lease_owner": w.NodeID,
					"lease_until": leaseUntil,
					"updated_at":  now,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				it := item
				it.LeaseOwner = w.NodeID
				it.LeaseUntil = &leaseUntil
				claimed = append(claimed, it)
			}
		}
		items = claimed
		return nil
	})
	return items, err
}

func (w *Worker) executeOne(ctx context.Context, item model.MonitorDefinition) error {
	startedAt := w.nowUTC()
	if w.Registry == nil {
		return w.finalizeRun(ctx, item, startedAt, monitorRunResult{
			Status:      "failed",
			SignalCount: 0,
			Error:       "detector registry not configured",
		})
	}
	intervalSec := normalizeInterval(item.IntervalSec)
	timeoutMS := normalizeTimeout(item.TimeoutMS)

	res := monitorRunResult{
		Status:      "success",
		SignalCount: 0,
		Error:       "",
		Details: map[string]any{
			"detectorType": item.DetectorType,
		},
	}

	p, ok := w.Registry.Get(item.DetectorType)
	if !ok {
		res.Status = "failed"
		res.Error = fmt.Sprintf("detector type not found: %s", item.DetectorType)
		return w.finalizeRun(ctx, item, startedAt, res)
	}

	cfgRaw := json.RawMessage(item.Config)
	if err := p.ValidateConfig(cfgRaw); err != nil {
		res.Status = "failed"
		res.Error = fmt.Sprintf("invalid detector config: %v", err)
		return w.finalizeRun(ctx, item, startedAt, res)
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	signals, err := p.Execute(execCtx, detector.ExecuteRequest{
		ProjectID: item.ProjectID,
		Config:    cfgRaw,
		Payload: map[string]any{
			"monitor_id":   item.ID,
			"monitor_name": item.Name,
		},
		Now: startedAt,
	})
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
		return w.finalizeRun(ctx, item, startedAt, res)
	}

	for _, sig := range signals {
		normalized := normalizeSignal(item, sig, startedAt)
		if w.Engine == nil {
			w.Engine = alert.NewEngine(w.DB)
		}
		if err := w.Engine.EvaluateSignal(execCtx, normalized); err != nil {
			res.Status = "failed"
			res.Error = err.Error()
			return w.finalizeRun(ctx, item, startedAt, res)
		}
		res.SignalCount++
	}
	res.Details["signals"] = res.SignalCount
	res.Details["intervalSec"] = intervalSec
	return w.finalizeRun(ctx, item, startedAt, res)
}

type monitorRunResult struct {
	Status      string
	SignalCount int
	Error       string
	Details     map[string]any
}

func (w *Worker) finalizeRun(ctx context.Context, item model.MonitorDefinition, startedAt time.Time, res monitorRunResult) error {
	finishedAt := w.nowUTC()
	nextRunAt := finishedAt.Add(time.Duration(normalizeInterval(item.IntervalSec)) * time.Second)
	if !item.Enabled {
		nextRunAt = item.NextRunAt
	}

	if res.Status == "" {
		res.Status = "success"
	}
	if res.Details == nil {
		res.Details = map[string]any{}
	}
	resultJSON, _ := json.Marshal(res.Details)

	run := model.MonitorRun{
		MonitorID:   item.ID,
		ProjectID:   item.ProjectID,
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
		Status:      res.Status,
		SignalCount: res.SignalCount,
		Error:       strings.TrimSpace(res.Error),
		Result:      datatypes.JSON(resultJSON),
	}

	return w.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Create(&run).Error; err != nil {
			return err
		}
		update := map[string]any{
			"lease_owner": "",
			"lease_until": nil,
			"updated_at":  finishedAt,
		}
		if item.Enabled {
			update["next_run_at"] = nextRunAt
		}
		return tx.WithContext(ctx).
			Model(&model.MonitorDefinition{}).
			Where("id = ?", item.ID).
			Updates(update).Error
	})
}

func normalizeSignal(item model.MonitorDefinition, sig detector.Signal, now time.Time) detector.Signal {
	out := sig
	if out.ProjectID <= 0 {
		out.ProjectID = item.ProjectID
	}
	if strings.TrimSpace(out.Source) == "" {
		out.Source = "logs"
	}
	if strings.TrimSpace(out.SourceType) == "" {
		out.SourceType = strings.TrimSpace(item.DetectorType)
	}
	if strings.TrimSpace(out.Severity) == "" {
		out.Severity = "info"
	}
	if strings.TrimSpace(out.Status) == "" {
		out.Status = "firing"
	}
	if strings.TrimSpace(out.Message) == "" {
		out.Message = strings.TrimSpace(item.Name)
	}
	if out.OccurredAt.IsZero() {
		out.OccurredAt = now
	}
	if out.Fields == nil {
		out.Fields = map[string]any{}
	}
	out.Fields["monitor_id"] = item.ID
	out.Fields["monitor_name"] = item.Name
	return out
}

func normalizeInterval(v int) int {
	if v <= 0 {
		return 60
	}
	if v > 86400 {
		return 86400
	}
	return v
}

func normalizeTimeout(v int) int {
	if v <= 0 {
		return 5000
	}
	if v > 120000 {
		return 120000
	}
	return v
}

func (w *Worker) nowUTC() time.Time {
	if w == nil || w.Now == nil {
		return time.Now().UTC()
	}
	t := w.Now()
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
