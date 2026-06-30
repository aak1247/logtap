package store

import (
	"context"
	"fmt"
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
	if db == nil || projectID <= 0 {
		return out, false, nil
	}
	day := now.UTC().Format("2006-01-02")
	if db.Migrator().HasTable(model.LogDailyStat{}.TableName()) {
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
	}

	raw, rawOK, err := GetDBMetricsTodayRaw(ctx, db, projectID, now)
	if err != nil || !rawOK {
		return out, rawOK || db.Migrator().HasTable(model.LogDailyStat{}.TableName()), err
	}
	if raw.Logs > out.Logs {
		out.Logs = raw.Logs
	}
	if raw.Events > out.Events {
		out.Events = raw.Events
	}
	if raw.Errors > out.Errors {
		out.Errors = raw.Errors
	}
	out.Users = raw.Users
	return out, true, nil
}

func GetDBMetricsTodayRaw(ctx context.Context, db *gorm.DB, projectID int, now time.Time) (MetricsToday, bool, error) {
	var out MetricsToday
	if db == nil || projectID <= 0 {
		return out, false, nil
	}
	start := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	var activeSources []string
	if db.Migrator().HasTable(model.Log{}.TableName()) {
		if err := db.WithContext(ctx).Table(model.Log{}.TableName()).
			Where("project_id = ? AND timestamp >= ? AND timestamp < ?", projectID, start, end).
			Count(&out.Logs).Error; err != nil {
			return out, true, err
		}
		var logEvents int64
		if err := db.WithContext(ctx).Table(model.Log{}.TableName()).
			Where("project_id = ? AND timestamp >= ? AND timestamp < ? AND level = ?", projectID, start, end, "event").
			Count(&logEvents).Error; err != nil {
			return out, true, err
		}
		out.Events = logEvents
		if err := db.WithContext(ctx).Table(model.Log{}.TableName()).
			Where("project_id = ? AND timestamp >= ? AND timestamp < ? AND level IN ?", projectID, start, end, []string{"error", "fatal"}).
			Count(&out.Errors).Error; err != nil {
			return out, true, err
		}
		activeSources = append(activeSources, model.Log{}.TableName())
	}
	if db.Migrator().HasTable(model.Event{}.TableName()) {
		var events int64
		if err := db.WithContext(ctx).Table(model.Event{}.TableName()).
			Where("project_id = ? AND timestamp >= ? AND timestamp < ?", projectID, start, end).
			Count(&events).Error; err != nil {
			return out, true, err
		}
		out.Events += events
		var errorsCount int64
		if err := db.WithContext(ctx).Table(model.Event{}.TableName()).
			Where("project_id = ? AND timestamp >= ? AND timestamp < ? AND level IN ?", projectID, start, end, []string{"error", "fatal"}).
			Count(&errorsCount).Error; err != nil {
			return out, true, err
		}
		out.Errors += errorsCount
		activeSources = append(activeSources, model.Event{}.TableName())
	}
	if db.Migrator().HasTable(model.TrackEvent{}.TableName()) {
		var events int64
		if err := db.WithContext(ctx).Table(model.TrackEvent{}.TableName()).
			Where("project_id = ? AND timestamp >= ? AND timestamp < ?", projectID, start, end).
			Count(&events).Error; err != nil {
			return out, true, err
		}
		if events > out.Events {
			out.Events = events
		}
	}
	users, err := countDistinctUsersAcrossSources(ctx, db, projectID, activeSources, start, end)
	if err != nil {
		return out, true, err
	}
	out.Users = users
	return out, true, nil
}

func GetDBMetricsTotal(ctx context.Context, db *gorm.DB, projectID int) (MetricsTotal, bool, error) {
	var out MetricsTotal
	if db == nil || projectID <= 0 {
		return out, false, nil
	}
	if db.Migrator().HasTable(model.ProjectCounter{}.TableName()) {
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
	}
	if db.Migrator().HasTable(model.UserFirstSeen{}.TableName()) {
		if err := db.WithContext(ctx).Table(model.UserFirstSeen{}.TableName()).
			Where("project_id = ?", projectID).
			Count(&out.Users).Error; err != nil {
			return out, true, err
		}
	}
	raw, rawOK, err := GetDBMetricsTotalRaw(ctx, db, projectID)
	if err != nil || !rawOK {
		return out, rawOK || db.Migrator().HasTable(model.ProjectCounter{}.TableName()), err
	}
	if raw.Logs > out.Logs {
		out.Logs = raw.Logs
	}
	if raw.Events > out.Events {
		out.Events = raw.Events
	}
	if raw.Users > out.Users {
		out.Users = raw.Users
	}
	return out, true, nil
}

func GetDBMetricsTotalRaw(ctx context.Context, db *gorm.DB, projectID int) (MetricsTotal, bool, error) {
	var out MetricsTotal
	if db == nil || projectID <= 0 {
		return out, false, nil
	}
	var activeSources []string
	if db.Migrator().HasTable(model.Log{}.TableName()) {
		if err := db.WithContext(ctx).Table(model.Log{}.TableName()).Where("project_id = ?", projectID).Count(&out.Logs).Error; err != nil {
			return out, true, err
		}
		var logEvents int64
		if err := db.WithContext(ctx).Table(model.Log{}.TableName()).Where("project_id = ? AND level = ?", projectID, "event").Count(&logEvents).Error; err != nil {
			return out, true, err
		}
		out.Events = logEvents
		activeSources = append(activeSources, model.Log{}.TableName())
	}
	if db.Migrator().HasTable(model.Event{}.TableName()) {
		var events int64
		if err := db.WithContext(ctx).Table(model.Event{}.TableName()).Where("project_id = ?", projectID).Count(&events).Error; err != nil {
			return out, true, err
		}
		out.Events += events
		activeSources = append(activeSources, model.Event{}.TableName())
	}
	if db.Migrator().HasTable(model.TrackEvent{}.TableName()) {
		var events int64
		if err := db.WithContext(ctx).Table(model.TrackEvent{}.TableName()).Where("project_id = ?", projectID).Count(&events).Error; err != nil {
			return out, true, err
		}
		if events > out.Events {
			out.Events = events
		}
	}
	users, err := countDistinctUsersAcrossSources(ctx, db, projectID, activeSources, time.Time{}, time.Time{})
	if err != nil {
		return out, true, err
	}
	out.Users = users
	return out, true, nil
}

func countDistinctUsersAcrossSources(ctx context.Context, db *gorm.DB, projectID int, sources []string, start, end time.Time) (int64, error) {
	if db == nil || projectID <= 0 || len(sources) == 0 {
		return 0, nil
	}
	var b strings.Builder
	args := make([]any, 0, len(sources)*3)
	b.WriteString("WITH active_users AS (")
	for i, source := range sources {
		if i > 0 {
			b.WriteString(" UNION ")
		}
		b.WriteString("SELECT distinct_id FROM ")
		b.WriteString(source)
		b.WriteString(" WHERE project_id = ? AND distinct_id IS NOT NULL AND distinct_id <> ''")
		args = append(args, projectID)
		if !start.IsZero() {
			b.WriteString(" AND timestamp >= ?")
			args = append(args, start)
		}
		if !end.IsZero() {
			b.WriteString(" AND timestamp < ?")
			args = append(args, end)
		}
	}
	b.WriteString(") SELECT COUNT(*) FROM active_users")
	var users int64
	if err := db.WithContext(ctx).Raw(b.String(), args...).Scan(&users).Error; err != nil {
		return 0, fmt.Errorf("count distinct active users: %w", err)
	}
	return users, nil
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
