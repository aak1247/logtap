package metrics

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisRecorder struct {
	rdb      *redis.Client
	dayTTL   time.Duration
	distTTL  time.Duration
	monthTTL time.Duration
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

func (r *RedisRecorder) ObserveEventDist(ctx context.Context, projectID int, ts time.Time, dims map[string]string) {
	if r == nil || r.rdb == nil {
		return
	}
	date := ts.UTC().Format("2006-01-02")

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

	acc := map[string]int64{}
	cur := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

	for !cur.After(last) {
		b := cur.Format("2006-01-02")
		hashKey := fmt.Sprintf("dist:%s:%d:%s", dim, projectID, b)
		m, err := r.rdb.HGetAll(ctx, hashKey).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		for k, v := range m {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				continue
			}
			acc[k] += n
		}
		cur = cur.AddDate(0, 0, 1)
	}

	items := make([]DistItem, 0, len(acc))
	for k, v := range acc {
		items = append(items, DistItem{Key: k, Count: v})
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
	return items, nil
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
