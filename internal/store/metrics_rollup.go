package store

import (
	"context"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	CounterLogsTotal   = "logs_total"
	CounterEventsTotal = "events_total"
)

type MetricsToday struct {
	Logs   int64
	Events int64
	Errors int64
	Users  int64
}

type MetricsTotal struct {
	Logs   int64
	Events int64
	Users  int64
}

func UpsertLogMetricsFromLogs(ctx context.Context, db *gorm.DB, rows []model.Log) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	counterDeltas, dailyRows := logMetricRows(rows)
	if err := incrementProjectCounters(ctx, db, counterDeltas); err != nil {
		return err
	}
	return upsertLogDailyStats(ctx, db, dailyRows)
}

func UpsertEventMetricsFromEvents(ctx context.Context, db *gorm.DB, rows []model.Event) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	counterDeltas, dailyRows := eventMetricRows(rows)
	if err := incrementProjectCounters(ctx, db, counterDeltas); err != nil {
		return err
	}
	return upsertLogDailyStats(ctx, db, dailyRows)
}

func UpsertEventMetricsFromTrackEvents(ctx context.Context, db *gorm.DB, rows []model.TrackEvent) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	counterDeltas, dailyRows := trackEventMetricRows(rows)
	if err := incrementProjectCounters(ctx, db, counterDeltas); err != nil {
		return err
	}
	return upsertLogDailyStats(ctx, db, dailyRows)
}

func GetDBMetricsToday(ctx context.Context, db *gorm.DB, projectID int, now time.Time) (MetricsToday, bool, error) {
	var out MetricsToday
	if db == nil || projectID <= 0 || !db.Migrator().HasTable(model.LogDailyStat{}.TableName()) {
		return out, false, nil
	}
	day := now.UTC().Format("2006-01-02")
	type row struct {
		Kind  string `gorm:"column:kind"`
		Level string `gorm:"column:level"`
		Count int64  `gorm:"column:count"`
	}
	var rows []row
	if err := db.WithContext(ctx).Table(model.LogDailyStat{}.TableName()).
		Select("kind, level, count").
		Where("project_id = ? AND day = ?", projectID, day).
		Scan(&rows).Error; err != nil {
		return out, true, err
	}
	for _, row := range rows {
		switch row.Kind {
		case "log":
			out.Logs += row.Count
		case "event":
			out.Events += row.Count
		}
		if row.Level == "error" || row.Level == "fatal" {
			out.Errors += row.Count
		}
	}
	if db.Migrator().HasTable(model.UserFirstSeen{}.TableName()) {
		start := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
		end := start.Add(24 * time.Hour)
		if err := db.WithContext(ctx).Table(model.UserFirstSeen{}.TableName()).
			Where("project_id = ? AND first_seen >= ? AND first_seen < ?", projectID, start, end).
			Count(&out.Users).Error; err != nil {
			return out, true, err
		}
	}
	return out, true, nil
}

func GetDBMetricsTotal(ctx context.Context, db *gorm.DB, projectID int) (MetricsTotal, bool, error) {
	var out MetricsTotal
	if db == nil || projectID <= 0 || !db.Migrator().HasTable(model.ProjectCounter{}.TableName()) {
		return out, false, nil
	}
	type row struct {
		Metric string `gorm:"column:metric"`
		Value  int64  `gorm:"column:value"`
	}
	var rows []row
	if err := db.WithContext(ctx).Table(model.ProjectCounter{}.TableName()).
		Select("metric, value").
		Where("project_id = ?", projectID).
		Scan(&rows).Error; err != nil {
		return out, true, err
	}
	for _, row := range rows {
		switch row.Metric {
		case CounterLogsTotal:
			out.Logs = row.Value
		case CounterEventsTotal:
			out.Events = row.Value
		}
	}
	if db.Migrator().HasTable(model.UserFirstSeen{}.TableName()) {
		if err := db.WithContext(ctx).Table(model.UserFirstSeen{}.TableName()).
			Where("project_id = ?", projectID).
			Count(&out.Users).Error; err != nil {
			return out, true, err
		}
	}
	return out, true, nil
}

func logMetricRows(rows []model.Log) (map[projectMetric]int64, []model.LogDailyStat) {
	counterDeltas := map[projectMetric]int64{}
	daily := map[dailyMetric]int64{}
	for _, row := range rows {
		if row.ProjectID <= 0 || row.Timestamp.IsZero() {
			continue
		}
		counterDeltas[projectMetric{projectID: row.ProjectID, metric: CounterLogsTotal}]++
		level := normalizeMetricLevel(row.Level)
		daily[dailyMetric{projectID: row.ProjectID, day: row.Timestamp.UTC().Format("2006-01-02"), kind: "log", level: level}]++
	}
	return counterDeltas, dailyMetricValues(daily)
}

func eventMetricRows(rows []model.Event) (map[projectMetric]int64, []model.LogDailyStat) {
	counterDeltas := map[projectMetric]int64{}
	daily := map[dailyMetric]int64{}
	for _, row := range rows {
		if row.ProjectID <= 0 || row.Timestamp.IsZero() {
			continue
		}
		counterDeltas[projectMetric{projectID: row.ProjectID, metric: CounterEventsTotal}]++
		level := normalizeMetricLevel(row.Level)
		daily[dailyMetric{projectID: row.ProjectID, day: row.Timestamp.UTC().Format("2006-01-02"), kind: "event", level: level}]++
	}
	return counterDeltas, dailyMetricValues(daily)
}

func trackEventMetricRows(rows []model.TrackEvent) (map[projectMetric]int64, []model.LogDailyStat) {
	counterDeltas := map[projectMetric]int64{}
	daily := map[dailyMetric]int64{}
	for _, row := range rows {
		if row.ProjectID <= 0 || row.Timestamp.IsZero() {
			continue
		}
		counterDeltas[projectMetric{projectID: row.ProjectID, metric: CounterEventsTotal}]++
		daily[dailyMetric{projectID: row.ProjectID, day: row.Timestamp.UTC().Format("2006-01-02"), kind: "event", level: "event"}]++
	}
	return counterDeltas, dailyMetricValues(daily)
}

type projectMetric struct {
	projectID int
	metric    string
}

type dailyMetric struct {
	projectID int
	day       string
	kind      string
	level     string
}

func incrementProjectCounters(ctx context.Context, db *gorm.DB, deltas map[projectMetric]int64) error {
	if db == nil || len(deltas) == 0 {
		return nil
	}
	now := time.Now().UTC()
	rows := make([]model.ProjectCounter, 0, len(deltas))
	for key, delta := range deltas {
		if key.projectID <= 0 || strings.TrimSpace(key.metric) == "" || delta <= 0 {
			continue
		}
		rows = append(rows, model.ProjectCounter{ProjectID: key.projectID, Metric: key.metric, Value: delta, UpdatedAt: now})
	}
	if len(rows) == 0 {
		return nil
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "project_id"}, {Name: "metric"}},
		DoUpdates: clause.Assignments(map[string]any{
			"value":      gorm.Expr("project_counters.value + EXCLUDED.value"),
			"updated_at": now,
		}),
	}).CreateInBatches(&rows, 200).Error
}

func upsertLogDailyStats(ctx context.Context, db *gorm.DB, rows []model.LogDailyStat) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for i := range rows {
		rows[i].UpdatedAt = now
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "project_id"}, {Name: "day"}, {Name: "kind"}, {Name: "level"}},
		DoUpdates: clause.Assignments(map[string]any{
			"count":      gorm.Expr("log_daily_stats.count + EXCLUDED.count"),
			"updated_at": now,
		}),
	}).CreateInBatches(&rows, 200).Error
}

func dailyMetricValues(daily map[dailyMetric]int64) []model.LogDailyStat {
	if len(daily) == 0 {
		return nil
	}
	out := make([]model.LogDailyStat, 0, len(daily))
	for key, count := range daily {
		if key.projectID <= 0 || count <= 0 {
			continue
		}
		out = append(out, model.LogDailyStat{ProjectID: key.projectID, Day: key.day, Kind: key.kind, Level: key.level, Count: count})
	}
	return out
}

func normalizeMetricLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		return "unknown"
	}
	return level
}
