package metrics

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type RedisRecorder struct {
	rdb      *redis.Client
	dayTTL   time.Duration
	distTTL  time.Duration
	monthTTL time.Duration
}

type RebuildProjectOptions struct {
	ProjectID  int
	Since      *time.Time
	Until      *time.Time
	BatchSize  int
	ResetRedis bool
}

type RebuildProjectResult struct {
	ProjectID int   `json:"project_id"`
	Logs      int64 `json:"logs"`
	Events    int64 `json:"events"`
	Enabled   bool  `json:"enabled"`
}

type RecorderOption func(*RedisRecorder)

func WithTTLs(dayTTL, distTTL, monthTTL time.Duration) RecorderOption {
	return func(r *RedisRecorder) {
		if dayTTL > 0 {
			r.dayTTL = dayTTL
		}
		if distTTL > 0 {
			r.distTTL = distTTL
		}
		if monthTTL > 0 {
			r.monthTTL = monthTTL
		}
	}
}

func NewRedisRecorder(rdb *redis.Client, opts ...RecorderOption) *RedisRecorder {
	r := &RedisRecorder{
		rdb:      rdb,
		dayTTL:   180 * 24 * time.Hour,
		distTTL:  90 * 24 * time.Hour,
		monthTTL: 18 * 31 * 24 * time.Hour,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

func (r *RedisRecorder) ObserveEvent(ctx context.Context, projectID int, level string, distinctID string, deviceID string, osName string, ts time.Time) {
	if r == nil || r.rdb == nil {
		return
	}
	date := ts.UTC().Format("2006-01-02")
	month := ts.UTC().Format("2006-01")
	distinctID = strings.TrimSpace(distinctID)
	deviceID = strings.TrimSpace(deviceID)
	osName = strings.TrimSpace(osName)

	pipe := r.rdb.Pipeline()
	expire := map[string]time.Duration{}
	eventsDayKey := fmt.Sprintf("metrics:events:%d:%s", projectID, date)
	pipe.Incr(ctx, eventsDayKey)
	expire[eventsDayKey] = r.dayTTL

	pipe.Incr(ctx, fmt.Sprintf("metrics:events:%d:total", projectID))
	if level == "error" || level == "fatal" {
		errorsDayKey := fmt.Sprintf("metrics:errors:%d:%s", projectID, date)
		pipe.Incr(ctx, errorsDayKey)
		expire[errorsDayKey] = r.dayTTL
		pipe.Incr(ctx, fmt.Sprintf("metrics:errors:%d:total", projectID))
	}
	if distinctID != "" {
		dauKey := fmt.Sprintf("active:dau:%d:%s", projectID, date)
		pipe.PFAdd(ctx, dauKey, distinctID)
		expire[dauKey] = r.dayTTL

		mauKey := fmt.Sprintf("active:mau:%d:%s", projectID, month)
		pipe.PFAdd(ctx, mauKey, distinctID)
		expire[mauKey] = r.monthTTL

		usersDayKey := fmt.Sprintf("metrics:users:%d:%s", projectID, date)
		pipe.PFAdd(ctx, usersDayKey, distinctID)
		expire[usersDayKey] = r.dayTTL

		pipe.PFAdd(ctx, fmt.Sprintf("metrics:users:%d:total", projectID), distinctID)
	}
	if deviceID != "" {
		devKey := fmt.Sprintf("active:devices:%d:%s", projectID, date)
		pipe.PFAdd(ctx, devKey, deviceID)
		expire[devKey] = r.dayTTL
	}
	_, _ = pipe.Exec(ctx)
	r.expireKeys(ctx, expire)
}

func (r *RedisRecorder) ObserveEventDist(ctx context.Context, projectID int, ts time.Time, distinctID string, dims map[string]string) {
	if r == nil || r.rdb == nil {
		return
	}
	date := ts.UTC().Format("2006-01-02")
	distinctID = strings.TrimSpace(distinctID)

	pipe := r.rdb.Pipeline()
	expire := map[string]time.Duration{}
	for dim, key := range dims {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		hashKey := fmt.Sprintf("dist:%s:%d:%s", dim, projectID, date)
		pipe.HIncrBy(ctx, hashKey, key, 1)
		expire[hashKey] = r.distTTL
		if distinctID != "" {
			userKey := fmt.Sprintf("dist_users:%s:%d:%s:%s", dim, projectID, date, key)
			pipe.PFAdd(ctx, userKey, distinctID)
			expire[userKey] = r.distTTL
		}
	}
	_, _ = pipe.Exec(ctx)
	r.expireKeys(ctx, expire)
}

func (r *RedisRecorder) ObserveLog(ctx context.Context, projectID int, level string, distinctID string, deviceID string, ts time.Time) {
	if r == nil || r.rdb == nil {
		return
	}
	date := ts.UTC().Format("2006-01-02")
	month := ts.UTC().Format("2006-01")
	distinctID = strings.TrimSpace(distinctID)
	deviceID = strings.TrimSpace(deviceID)

	pipe := r.rdb.Pipeline()
	expire := map[string]time.Duration{}
	logsDayKey := fmt.Sprintf("metrics:logs:%d:%s", projectID, date)
	pipe.Incr(ctx, logsDayKey)
	expire[logsDayKey] = r.dayTTL

	pipe.Incr(ctx, fmt.Sprintf("metrics:logs:%d:total", projectID))
	if distinctID != "" {
		dauKey := fmt.Sprintf("active:dau:%d:%s", projectID, date)
		pipe.PFAdd(ctx, dauKey, distinctID)
		expire[dauKey] = r.dayTTL

		mauKey := fmt.Sprintf("active:mau:%d:%s", projectID, month)
		pipe.PFAdd(ctx, mauKey, distinctID)
		expire[mauKey] = r.monthTTL

		usersDayKey := fmt.Sprintf("metrics:users:%d:%s", projectID, date)
		pipe.PFAdd(ctx, usersDayKey, distinctID)
		expire[usersDayKey] = r.dayTTL

		pipe.PFAdd(ctx, fmt.Sprintf("metrics:users:%d:total", projectID), distinctID)
	}
	if deviceID != "" {
		devKey := fmt.Sprintf("active:devices:%d:%s", projectID, date)
		pipe.PFAdd(ctx, devKey, deviceID)
		expire[devKey] = r.dayTTL
	}
	_, _ = pipe.Exec(ctx)
	r.expireKeys(ctx, expire)
}

func (r *RedisRecorder) RebuildProjectFromDB(ctx context.Context, db *gorm.DB, opts RebuildProjectOptions) (RebuildProjectResult, error) {
	res := RebuildProjectResult{ProjectID: opts.ProjectID, Enabled: r != nil && r.rdb != nil}
	if r == nil || r.rdb == nil || db == nil || opts.ProjectID <= 0 {
		return res, nil
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}
	if opts.ResetRedis {
		if err := r.DeleteProjectMetrics(ctx, opts.ProjectID); err != nil {
			return res, err
		}
	}
	if db.Migrator().HasTable(model.Log{}.TableName()) {
		q := db.WithContext(ctx).Where("project_id = ?", opts.ProjectID)
		q = applyRebuildRange(q, opts.Since, opts.Until)
		var rows []model.Log
		if err := q.Order("timestamp ASC, id ASC").FindInBatches(&rows, batchSize, func(tx *gorm.DB, _ int) error {
			for _, row := range rows {
				r.ObserveLog(ctx, row.ProjectID, row.Level, row.DistinctID, row.DeviceID, row.Timestamp)
				res.Logs++
			}
			return nil
		}).Error; err != nil {
			return res, err
		}
	}
	if db.Migrator().HasTable(model.Event{}.TableName()) {
		q := db.WithContext(ctx).Where("project_id = ?", opts.ProjectID)
		q = applyRebuildRange(q, opts.Since, opts.Until)
		var rows []model.Event
		if err := q.Order("timestamp ASC").FindInBatches(&rows, batchSize, func(tx *gorm.DB, _ int) error {
			for _, row := range rows {
				r.ObserveEvent(ctx, row.ProjectID, row.Level, row.DistinctID, row.DeviceID, row.OS, row.Timestamp)
				r.ObserveEventDist(ctx, row.ProjectID, row.Timestamp, row.DistinctID, map[string]string{"os": row.OS})
				res.Events++
			}
			return nil
		}).Error; err != nil {
			return res, err
		}
	}
	return res, nil
}

func (r *RedisRecorder) DeleteProjectMetrics(ctx context.Context, projectID int) error {
	if r == nil || r.rdb == nil || projectID <= 0 {
		return nil
	}
	patterns := []string{
		fmt.Sprintf("metrics:logs:%d:*", projectID),
		fmt.Sprintf("metrics:events:%d:*", projectID),
		fmt.Sprintf("metrics:errors:%d:*", projectID),
		fmt.Sprintf("metrics:users:%d:*", projectID),
		fmt.Sprintf("metrics:totals:%d:*", projectID),
		fmt.Sprintf("active:dau:%d:*", projectID),
		fmt.Sprintf("active:mau:%d:*", projectID),
		fmt.Sprintf("active:devices:%d:*", projectID),
		fmt.Sprintf("dist:*:%d:*", projectID),
		fmt.Sprintf("dist_users:*:%d:*", projectID),
	}
	for _, pattern := range patterns {
		if err := r.deleteByPattern(ctx, pattern); err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisRecorder) deleteByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, next, err := r.rdb.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := r.rdb.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func applyRebuildRange(q *gorm.DB, since *time.Time, until *time.Time) *gorm.DB {
	if since != nil {
		q = q.Where("timestamp >= ?", since.UTC())
	}
	if until != nil {
		q = q.Where("timestamp < ?", until.UTC())
	}
	return q
}

func (r *RedisRecorder) expireKeys(ctx context.Context, keys map[string]time.Duration) {
	if r == nil || r.rdb == nil || len(keys) == 0 {
		return
	}
	pipe := r.rdb.Pipeline()
	for k, ttl := range keys {
		if strings.TrimSpace(k) == "" || ttl <= 0 {
			continue
		}
		pipe.Expire(ctx, k, ttl)
	}
	_, _ = pipe.Exec(ctx)
}

func (r *RedisRecorder) Today(ctx context.Context, projectID int, now time.Time) (logs int64, events int64, errors int64, users int64, ok bool, err error) {
	if r == nil || r.rdb == nil {
		return 0, 0, 0, 0, false, nil
	}
	date := now.UTC().Format("2006-01-02")
	logsKey := fmt.Sprintf("metrics:logs:%d:%s", projectID, date)
	eventsKey := fmt.Sprintf("metrics:events:%d:%s", projectID, date)
	errorsKey := fmt.Sprintf("metrics:errors:%d:%s", projectID, date)
	usersKey := fmt.Sprintf("metrics:users:%d:%s", projectID, date)

	pipe := r.rdb.Pipeline()
	logsCmd := pipe.Get(ctx, logsKey)
	eventsCmd := pipe.Get(ctx, eventsKey)
	errorsCmd := pipe.Get(ctx, errorsKey)
	usersCmd := pipe.PFCount(ctx, usersKey)
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return 0, 0, 0, 0, true, err
	}
	logs, _ = logsCmd.Int64()
	events, _ = eventsCmd.Int64()
	errors, _ = errorsCmd.Int64()
	users, _ = usersCmd.Result()
	return logs, events, errors, users, true, nil
}

func (r *RedisRecorder) Total(ctx context.Context, projectID int) (logs int64, events int64, users int64, ok bool, err error) {
	if r == nil || r.rdb == nil {
		return 0, 0, 0, false, nil
	}
	if err := r.ensureTotals(ctx, projectID); err != nil {
		return 0, 0, 0, true, err
	}

	logsKey := fmt.Sprintf("metrics:logs:%d:total", projectID)
	eventsKey := fmt.Sprintf("metrics:events:%d:total", projectID)
	usersKey := fmt.Sprintf("metrics:users:%d:total", projectID)

	pipe := r.rdb.Pipeline()
	logsCmd := pipe.Get(ctx, logsKey)
	eventsCmd := pipe.Get(ctx, eventsKey)
	usersCmd := pipe.PFCount(ctx, usersKey)
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return 0, 0, 0, true, err
	}
	logs, _ = logsCmd.Int64()
	events, _ = eventsCmd.Int64()
	users, _ = usersCmd.Result()
	return logs, events, users, true, nil
}

func (r *RedisRecorder) ensureTotals(ctx context.Context, projectID int) error {
	if r == nil || r.rdb == nil {
		return nil
	}
	readyKey := fmt.Sprintf("metrics:totals:%d:ready", projectID)
	if ok, _ := r.rdb.Get(ctx, readyKey).Result(); ok == "1" {
		return nil
	}

	logsTotal, err := r.sumByKeyPattern(ctx, fmt.Sprintf("metrics:logs:%d:*", projectID), func(key string) bool {
		return strings.HasSuffix(key, ":total")
	})
	if err != nil {
		return err
	}
	eventsTotal, err := r.sumByKeyPattern(ctx, fmt.Sprintf("metrics:events:%d:*", projectID), func(key string) bool {
		return strings.HasSuffix(key, ":total")
	})
	if err != nil {
		return err
	}

	// Merge all daily HLLs into the total HLL, so upgrades can still show a sensible value.
	usersTotalKey := fmt.Sprintf("metrics:users:%d:total", projectID)
	if err := r.pfmergeByKeyPattern(ctx, usersTotalKey, fmt.Sprintf("metrics:users:%d:*", projectID), func(key string) bool {
		return strings.HasSuffix(key, ":total")
	}); err != nil {
		return err
	}

	pipe := r.rdb.Pipeline()
	pipe.Set(ctx, fmt.Sprintf("metrics:logs:%d:total", projectID), logsTotal, 0)
	pipe.Set(ctx, fmt.Sprintf("metrics:events:%d:total", projectID), eventsTotal, 0)
	pipe.Set(ctx, readyKey, "1", 0)
	_, err = pipe.Exec(ctx)
	return err
}

type ActiveWarmupOptions struct {
	Days      int
	Months    int
	BatchSize int
	Now       time.Time
}

type activeWarmupRow struct {
	ProjectID  int
	Day        string
	DistinctID string
}

func (r *RedisRecorder) WarmActiveUsersFromDB(ctx context.Context, db *gorm.DB, opts ActiveWarmupOptions) error {
	if r == nil || r.rdb == nil || db == nil {
		return nil
	}
	days := opts.Days
	if days <= 0 {
		days = 30
	}
	months := opts.Months
	if months <= 0 {
		months = 6
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}
	now := opts.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	readyDayKey := activeWarmupDayReadyKey(now)
	readyMonthKey := activeWarmupMonthReadyKey(now)

	sources := activeWarmupSources(db)
	if len(sources) == 0 {
		return nil
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -days+1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -months+1, 0)
	start := dayStart
	if monthStart.Before(start) {
		start = monthStart
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	query, args := activeWarmupSQL(db, sources, start, end)

	rows, err := db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	pipe := r.rdb.Pipeline()
	expire := map[string]time.Duration{}
	pending := 0
	for rows.Next() {
		var row activeWarmupRow
		if err := rows.Scan(&row.ProjectID, &row.Day, &row.DistinctID); err != nil {
			return err
		}
		day := normalizeActiveWarmupDay(row.Day)
		distinctID := strings.TrimSpace(row.DistinctID)
		if row.ProjectID <= 0 || day == "" || distinctID == "" {
			continue
		}
		mauKey := fmt.Sprintf("active:mau:%d:%s", row.ProjectID, day[:len("2006-01")])
		pipe.PFAdd(ctx, mauKey, distinctID)
		expire[mauKey] = r.monthTTL
		pending++
		if activeWarmupDayInRange(day, dayStart, end) {
			dauKey := fmt.Sprintf("active:dau:%d:%s", row.ProjectID, day)
			usersDayKey := fmt.Sprintf("metrics:users:%d:%s", row.ProjectID, day)
			pipe.PFAdd(ctx, dauKey, distinctID)
			pipe.PFAdd(ctx, usersDayKey, distinctID)
			expire[dauKey] = r.dayTTL
			expire[usersDayKey] = r.dayTTL
			pending += 2
		}
		if pending >= batchSize {
			if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
				return err
			}
			pipe = r.rdb.Pipeline()
			pending = 0
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if pending > 0 {
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return err
		}
	}
	r.expireKeys(ctx, expire)
	pipe = r.rdb.Pipeline()
	pipe.Set(ctx, readyDayKey, days, 25*time.Hour)
	pipe.Set(ctx, readyMonthKey, months, 25*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}

func (r *RedisRecorder) ActiveWarmupCovers(ctx context.Context, start, end time.Time, bucket string) bool {
	if r == nil || r.rdb == nil {
		return false
	}
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	today := time.Now().UTC()
	todayDay := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	if bucket == "month" {
		startMonth := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		endMonth := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
		todayMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		if endMonth.After(todayMonth) {
			return false
		}
		needed := (todayMonth.Year()-startMonth.Year())*12 + int(todayMonth.Month()-startMonth.Month()) + 1
		if needed <= 0 {
			return false
		}
		readyMonths, err := r.rdb.Get(ctx, activeWarmupMonthReadyKey(todayDay)).Int()
		return err == nil && readyMonths >= needed
	}

	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	if endDay.After(todayDay) {
		return false
	}
	needed := int(todayDay.Sub(startDay).Hours()/24) + 1
	if needed <= 0 {
		return false
	}
	readyDays, err := r.rdb.Get(ctx, activeWarmupDayReadyKey(todayDay)).Int()
	return err == nil && readyDays >= needed
}

func activeWarmupSources(db *gorm.DB) []string {
	if db == nil {
		return nil
	}
	var sources []string
	for _, table := range []string{"logs", "events", "track_events"} {
		if db.Migrator().HasTable(table) {
			sources = append(sources, table)
		}
	}
	return sources
}

func activeWarmupSQL(db *gorm.DB, sources []string, start, end time.Time) (string, []any) {
	dayExpr := "DATE(timestamp)"
	if db != nil && strings.EqualFold(db.Dialector.Name(), "postgres") {
		dayExpr = "TO_CHAR(timestamp AT TIME ZONE 'UTC', 'YYYY-MM-DD')"
	}

	var b strings.Builder
	args := make([]any, 0, len(sources)*2)
	b.WriteString("WITH active_events AS (")
	for i, source := range sources {
		if i > 0 {
			b.WriteString(" UNION ALL ")
		}
		b.WriteString("SELECT project_id, timestamp, distinct_id FROM ")
		b.WriteString(source)
		b.WriteString(" WHERE distinct_id IS NOT NULL AND distinct_id <> '' AND timestamp >= ? AND timestamp < ?")
		args = append(args, start, end)
	}
	b.WriteString(") SELECT DISTINCT project_id, ")
	b.WriteString(dayExpr)
	b.WriteString(" AS day, distinct_id FROM active_events ORDER BY project_id, day")
	return b.String(), args
}

func normalizeActiveWarmupDay(day string) string {
	day = strings.TrimSpace(day)
	if len(day) < len("2006-01-02") {
		return ""
	}
	return day[:len("2006-01-02")]
}

func activeWarmupDayInRange(day string, start, end time.Time) bool {
	t, err := time.ParseInLocation("2006-01-02", day, time.UTC)
	return err == nil && !t.Before(start) && t.Before(end)
}

func activeWarmupDayReadyKey(t time.Time) string {
	return fmt.Sprintf("active:warmup:ready:day:%s", t.UTC().Format("2006-01-02"))
}

func activeWarmupMonthReadyKey(t time.Time) string {
	return fmt.Sprintf("active:warmup:ready:month:%s", t.UTC().Format("2006-01-02"))
}

func (r *RedisRecorder) sumByKeyPattern(
	ctx context.Context,
	pattern string,
	skip func(key string) bool,
) (int64, error) {
	var (
		cursor uint64
		total  int64
	)
	for {
		keys, nextCursor, err := r.rdb.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return 0, err
		}
		if len(keys) > 0 {
			pipe := r.rdb.Pipeline()
			cmds := make([]*redis.StringCmd, 0, len(keys))
			for _, k := range keys {
				if skip != nil && skip(k) {
					continue
				}
				cmds = append(cmds, pipe.Get(ctx, k))
			}
			_, execErr := pipe.Exec(ctx)
			if execErr != nil && execErr != redis.Nil {
				return 0, execErr
			}
			for _, cmd := range cmds {
				n, _ := cmd.Int64()
				total += n
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return total, nil
}

func (r *RedisRecorder) pfmergeByKeyPattern(
	ctx context.Context,
	destKey string,
	pattern string,
	skip func(key string) bool,
) error {
	var cursor uint64
	for {
		keys, nextCursor, err := r.rdb.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return err
		}
		var src []string
		for _, k := range keys {
			if skip != nil && skip(k) {
				continue
			}
			src = append(src, k)
		}
		if len(src) > 0 {
			if err := r.rdb.PFMerge(ctx, destKey, src...).Err(); err != nil && err != redis.Nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

type BucketCount struct {
	Bucket string `json:"bucket"`
	Active int64  `json:"active"`
}

type DistItem struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type DistBucket struct {
	Bucket string     `json:"bucket"`
	Items  []DistItem `json:"items"`
}

type RetentionPoint struct {
	Day    int     `json:"day"`
	Active int64   `json:"active"`
	Rate   float64 `json:"rate"`
}

type RetentionRow struct {
	Cohort     string           `json:"cohort"`
	CohortSize int64            `json:"cohort_size"`
	Points     []RetentionPoint `json:"points"`
}

var pfUnionCountScript = `
redis.call('PFMERGE', KEYS[1], KEYS[2], KEYS[3])
local n = redis.call('PFCOUNT', KEYS[1])
redis.call('DEL', KEYS[1])
return n
`

func (r *RedisRecorder) Distribution(ctx context.Context, projectID int, dim string, start, end time.Time, limit int) ([]DistItem, error) {
	return r.DistributionMetric(ctx, projectID, dim, start, end, limit, "events")
}

func (r *RedisRecorder) DistributionMetric(ctx context.Context, projectID int, dim string, start, end time.Time, limit int, metric string) ([]DistItem, error) {
	if r == nil || r.rdb == nil {
		return nil, nil
	}
	dim = strings.TrimSpace(dim)
	if dim == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	metric = normalizeDistMetric(metric)

	acc := map[string]int64{}
	if metric == "users" {
		m, err := r.distributionUsersForWindow(ctx, projectID, dim, start, end)
		if err != nil {
			return nil, err
		}
		acc = m
	} else {
		cur := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
		for !cur.After(last) {
			day := cur.Format("2006-01-02")
			m, err := r.distributionEventsForDay(ctx, projectID, dim, day)
			if err != nil {
				return nil, err
			}
			for k, v := range m {
				acc[k] += v
			}
			cur = cur.AddDate(0, 0, 1)
		}
	}

	return topDistItems(acc, limit), nil
}

func (r *RedisRecorder) DistributionSeries(ctx context.Context, projectID int, dim string, start, end time.Time, bucket string, limit int, metric string) ([]DistBucket, error) {
	if r == nil || r.rdb == nil {
		return nil, nil
	}
	dim = strings.TrimSpace(dim)
	if dim == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	bucket = normalizeDistBucket(bucket)
	metric = normalizeDistMetric(metric)

	var out []DistBucket
	for _, window := range distWindows(start, end, bucket) {
		acc := map[string]int64{}
		if metric == "users" {
			m, err := r.distributionUsersForWindow(ctx, projectID, dim, window.start, window.end)
			if err != nil {
				return nil, err
			}
			acc = m
		} else {
			cur := window.start
			for !cur.After(window.end) {
				day := cur.Format("2006-01-02")
				m, err := r.distributionEventsForDay(ctx, projectID, dim, day)
				if err != nil {
					return nil, err
				}
				for k, v := range m {
					acc[k] += v
				}
				cur = cur.AddDate(0, 0, 1)
			}
		}
		out = append(out, DistBucket{
			Bucket: window.label,
			Items:  topDistItems(acc, limit),
		})
	}
	return out, nil
}

func topDistItems(acc map[string]int64, limit int) []DistItem {
	items := make([]DistItem, 0, len(acc))
	for k, v := range acc {
		if v > 0 {
			items = append(items, DistItem{Key: k, Count: v})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (r *RedisRecorder) distributionEventsForDay(ctx context.Context, projectID int, dim string, day string) (map[string]int64, error) {
	hashKey := fmt.Sprintf("dist:%s:%d:%s", dim, projectID, day)
	m, err := r.rdb.HGetAll(ctx, hashKey).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	out := make(map[string]int64, len(m))
	for k, v := range m {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		out[k] = n
	}
	return out, nil
}

func (r *RedisRecorder) distributionUsersForWindow(ctx context.Context, projectID int, dim string, start, end time.Time) (map[string]int64, error) {
	keysByValue := map[string][]string{}
	cur := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	for !cur.After(last) {
		day := cur.Format("2006-01-02")
		keys, err := r.distributionUserKeysForDay(ctx, projectID, dim, day)
		if err != nil {
			return nil, err
		}
		for value, dayKeys := range keys {
			keysByValue[value] = append(keysByValue[value], dayKeys...)
		}
		cur = cur.AddDate(0, 0, 1)
	}

	out := make(map[string]int64, len(keysByValue))
	for value, keys := range keysByValue {
		if len(keys) == 0 {
			continue
		}
		n, err := r.pfCountUnion(ctx, keys)
		if err != nil {
			return nil, err
		}
		out[value] = n
	}
	return out, nil
}

func (r *RedisRecorder) distributionUserKeysForDay(ctx context.Context, projectID int, dim string, day string) (map[string][]string, error) {
	pattern := fmt.Sprintf("dist_users:%s:%d:%s:*", dim, projectID, day)
	keys, err := r.rdb.Keys(ctx, pattern).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	out := make(map[string][]string, len(keys))
	prefix := fmt.Sprintf("dist_users:%s:%d:%s:", dim, projectID, day)
	for _, key := range keys {
		value := strings.TrimPrefix(key, prefix)
		if value == "" {
			continue
		}
		out[value] = append(out[value], key)
	}
	return out, nil
}

func (r *RedisRecorder) distributionUsersForDay(ctx context.Context, projectID int, dim string, day string) (map[string]int64, error) {
	keysByValue, err := r.distributionUserKeysForDay(ctx, projectID, dim, day)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(keysByValue))
	for value, keys := range keysByValue {
		if len(keys) == 0 {
			continue
		}
		n, err := r.pfCountUnion(ctx, keys)
		if err != nil {
			return nil, err
		}
		out[value] = n
	}
	return out, nil
}

func (r *RedisRecorder) pfCountUnion(ctx context.Context, keys []string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	if len(keys) == 1 {
		n, err := r.rdb.PFCount(ctx, keys[0]).Result()
		if err == redis.Nil {
			return 0, nil
		}
		return n, err
	}
	tmpKey := fmt.Sprintf("dist_users:tmp:%d", time.Now().UnixNano())
	if err := r.rdb.PFMerge(ctx, tmpKey, keys...).Err(); err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	n, err := r.rdb.PFCount(ctx, tmpKey).Result()
	_ = r.rdb.Del(ctx, tmpKey).Err()
	if err == redis.Nil {
		return 0, nil
	}
	return n, err
}

type distWindow struct {
	label string
	start time.Time
	end   time.Time
}

func distWindows(start, end time.Time, bucket string) []distWindow {
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	var out []distWindow
	switch bucket {
	case "year":
		cur := time.Date(startDay.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		for !cur.After(endDay) {
			winEnd := cur.AddDate(1, 0, -1)
			out = append(out, clippedDistWindow(cur.Format("2006"), cur, winEnd, startDay, endDay))
			cur = cur.AddDate(1, 0, 0)
		}
	case "month":
		cur := time.Date(startDay.Year(), startDay.Month(), 1, 0, 0, 0, 0, time.UTC)
		for !cur.After(endDay) {
			winEnd := cur.AddDate(0, 1, -1)
			out = append(out, clippedDistWindow(cur.Format("2006-01"), cur, winEnd, startDay, endDay))
			cur = cur.AddDate(0, 1, 0)
		}
	case "week":
		cur := startOfISOWeek(startDay)
		for !cur.After(endDay) {
			year, week := cur.ISOWeek()
			winEnd := cur.AddDate(0, 0, 6)
			out = append(out, clippedDistWindow(fmt.Sprintf("%04d-W%02d", year, week), cur, winEnd, startDay, endDay))
			cur = cur.AddDate(0, 0, 7)
		}
	default:
		for cur := startDay; !cur.After(endDay); cur = cur.AddDate(0, 0, 1) {
			out = append(out, distWindow{label: cur.Format("2006-01-02"), start: cur, end: cur})
		}
	}
	return out
}

func clippedDistWindow(label string, start, end, min, max time.Time) distWindow {
	if start.Before(min) {
		start = min
	}
	if end.After(max) {
		end = max
	}
	return distWindow{label: label, start: start, end: end}
}

func startOfISOWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return t.AddDate(0, 0, 1-weekday)
}

func normalizeDistBucket(bucket string) string {
	switch strings.ToLower(strings.TrimSpace(bucket)) {
	case "week", "month", "year":
		return strings.ToLower(strings.TrimSpace(bucket))
	default:
		return "day"
	}
}

func normalizeDistMetric(metric string) string {
	if strings.EqualFold(strings.TrimSpace(metric), "users") {
		return "users"
	}
	return "events"
}

func (r *RedisRecorder) Retention(ctx context.Context, projectID int, start, end time.Time, dayOffsets []int) ([]RetentionRow, error) {
	if r == nil || r.rdb == nil {
		return nil, nil
	}
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}

	seen := map[int]bool{}
	var offsets []int
	for _, d := range dayOffsets {
		if d <= 0 || d > 365 {
			continue
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		offsets = append(offsets, d)
	}
	sort.Ints(offsets)
	if len(offsets) == 0 {
		offsets = []int{1, 7, 30}
	}
	if len(offsets) > 10 {
		offsets = offsets[:10]
	}

	type rowCmds struct {
		cohort time.Time
		a      *redis.IntCmd
		b      map[int]*redis.IntCmd
		u      map[int]*redis.Cmd
	}
	var cmds []rowCmds

	cur := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

	pipe := r.rdb.Pipeline()
	for !cur.After(last) {
		cohortDate := cur.Format("2006-01-02")
		aKey := fmt.Sprintf("active:dau:%d:%s", projectID, cohortDate)
		rc := rowCmds{
			cohort: cur,
			a:      pipe.PFCount(ctx, aKey),
			b:      map[int]*redis.IntCmd{},
			u:      map[int]*redis.Cmd{},
		}
		for _, d := range offsets {
			t := cur.AddDate(0, 0, d).Format("2006-01-02")
			bKey := fmt.Sprintf("active:dau:%d:%s", projectID, t)
			rc.b[d] = pipe.PFCount(ctx, bKey)
			tmpKey := fmt.Sprintf("tmp:pfu:%d:%d:%d", projectID, cur.UnixNano(), d)
			rc.u[d] = pipe.Eval(ctx, pfUnionCountScript, []string{tmpKey, aKey, bKey})
		}
		cmds = append(cmds, rc)
		cur = cur.AddDate(0, 0, 1)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	out := make([]RetentionRow, 0, len(cmds))
	for _, rc := range cmds {
		a, _ := rc.a.Result()
		row := RetentionRow{
			Cohort:     rc.cohort.Format("2006-01-02"),
			CohortSize: a,
		}
		for _, d := range offsets {
			b, _ := rc.b[d].Result()
			u, _ := rc.u[d].Int64()
			inter := a + b - u
			if inter < 0 {
				inter = 0
			}
			rate := 0.0
			if a > 0 {
				rate = float64(inter) / float64(a)
			}
			row.Points = append(row.Points, RetentionPoint{Day: d, Active: inter, Rate: rate})
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *RedisRecorder) ActiveSeries(ctx context.Context, projectID int, start, end time.Time, bucket string) ([]BucketCount, error) {
	if r == nil || r.rdb == nil {
		return nil, nil
	}
	start = start.UTC()
	end = end.UTC()
	if end.Before(start) {
		start, end = end, start
	}

	switch bucket {
	case "month":
		return r.activeByMonth(ctx, projectID, start, end)
	default:
		return r.activeByDay(ctx, projectID, start, end)
	}
}

func (r *RedisRecorder) activeByDay(ctx context.Context, projectID int, start, end time.Time) ([]BucketCount, error) {
	var out []BucketCount
	cur := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

	for !cur.After(last) {
		b := cur.Format("2006-01-02")
		key := fmt.Sprintf("active:dau:%d:%s", projectID, b)
		n, err := r.rdb.PFCount(ctx, key).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		out = append(out, BucketCount{Bucket: b, Active: n})
		cur = cur.AddDate(0, 0, 1)
	}
	return out, nil
}

func (r *RedisRecorder) activeByMonth(ctx context.Context, projectID int, start, end time.Time) ([]BucketCount, error) {
	var out []BucketCount
	cur := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)

	for !cur.After(last) {
		b := cur.Format("2006-01")
		key := fmt.Sprintf("active:mau:%d:%s", projectID, b)
		n, err := r.rdb.PFCount(ctx, key).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		out = append(out, BucketCount{Bucket: b, Active: n})
		cur = cur.AddDate(0, 1, 0)
	}
	return out, nil
}
