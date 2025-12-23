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
	rdb *redis.Client
}

func NewRedisRecorder(rdb *redis.Client) *RedisRecorder {
	return &RedisRecorder{rdb: rdb}
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
	pipe.Incr(ctx, fmt.Sprintf("metrics:events:%d:%s", projectID, date))
	if level == "error" || level == "fatal" {
		pipe.Incr(ctx, fmt.Sprintf("metrics:errors:%d:%s", projectID, date))
	}
	if distinctID != "" {
		pipe.PFAdd(ctx, fmt.Sprintf("active:dau:%d:%s", projectID, date), distinctID)
		pipe.PFAdd(ctx, fmt.Sprintf("active:mau:%d:%s", projectID, month), distinctID)
		pipe.PFAdd(ctx, fmt.Sprintf("metrics:users:%d:%s", projectID, date), distinctID)
	}
	if deviceID != "" {
		pipe.PFAdd(ctx, fmt.Sprintf("active:devices:%d:%s", projectID, date), deviceID)
	}
	_, _ = pipe.Exec(ctx)
}

func (r *RedisRecorder) ObserveEventDist(ctx context.Context, projectID int, ts time.Time, dims map[string]string) {
	if r == nil || r.rdb == nil {
		return
	}
	date := ts.UTC().Format("2006-01-02")

	pipe := r.rdb.Pipeline()
	for dim, key := range dims {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		pipe.HIncrBy(ctx, fmt.Sprintf("dist:%s:%d:%s", dim, projectID, date), key, 1)
	}
	_, _ = pipe.Exec(ctx)
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
	pipe.Incr(ctx, fmt.Sprintf("metrics:logs:%d:%s", projectID, date))
	if distinctID != "" {
		pipe.PFAdd(ctx, fmt.Sprintf("active:dau:%d:%s", projectID, date), distinctID)
		pipe.PFAdd(ctx, fmt.Sprintf("active:mau:%d:%s", projectID, month), distinctID)
		pipe.PFAdd(ctx, fmt.Sprintf("metrics:users:%d:%s", projectID, date), distinctID)
	}
	if deviceID != "" {
		pipe.PFAdd(ctx, fmt.Sprintf("active:devices:%d:%s", projectID, date), deviceID)
	}
	_, _ = pipe.Exec(ctx)
}

func (r *RedisRecorder) Today(ctx context.Context, projectID int, now time.Time) (events int64, errors int64, users int64, ok bool, err error) {
	if r == nil || r.rdb == nil {
		return 0, 0, 0, false, nil
	}
	date := now.UTC().Format("2006-01-02")
	eventsKey := fmt.Sprintf("metrics:events:%d:%s", projectID, date)
	errorsKey := fmt.Sprintf("metrics:errors:%d:%s", projectID, date)
	usersKey := fmt.Sprintf("metrics:users:%d:%s", projectID, date)

	pipe := r.rdb.Pipeline()
	eventsCmd := pipe.Get(ctx, eventsKey)
	errorsCmd := pipe.Get(ctx, errorsKey)
	usersCmd := pipe.PFCount(ctx, usersKey)
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return 0, 0, 0, true, err
	}
	events, _ = eventsCmd.Int64()
	errors, _ = errorsCmd.Int64()
	users, _ = usersCmd.Result()
	return events, errors, users, true, nil
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
		u      map[int]*redis.IntCmd
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
			u:      map[int]*redis.IntCmd{},
		}
		for _, d := range offsets {
			t := cur.AddDate(0, 0, d).Format("2006-01-02")
			bKey := fmt.Sprintf("active:dau:%d:%s", projectID, t)
			rc.b[d] = pipe.PFCount(ctx, bKey)
			rc.u[d] = pipe.PFCount(ctx, aKey, bKey) // union cardinality
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
			u, _ := rc.u[d].Result()
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
