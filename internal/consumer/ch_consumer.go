package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/enrich"
	"github.com/aak1247/logtap/internal/identity"
	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/obs"
	"github.com/aak1247/logtap/internal/store"
	"github.com/aak1247/logtap/internal/store/clickhouse"
	"github.com/aak1247/logtap/internal/tenant"
	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// *nsq.Message already provides the acknowledgement controls the CH batcher
// needs; with DisableAutoResponse the batcher owns the message lifecycle.
var _ clickhouse.MessageCtl = (*nsq.Message)(nil)

// chConsumerMetrics shares the batcher's logtap_consumer expvar map.
var chConsumerMetrics = clickhouse.Metrics()

// dedupGate is the consumer-side Redis dedup gate (design §4.7): keys are
// recorded only AFTER a durable ClickHouse flush, so a failed flush can never
// blacklist a message that still needs a retry.
type dedupGate struct {
	rdb *redis.Client
	ttl time.Duration
}

func newDedupGate(rdb *redis.Client) *dedupGate {
	if rdb == nil {
		return nil
	}
	return &dedupGate{rdb: rdb, ttl: 48 * time.Hour}
}

func (g *dedupGate) enabled() bool { return g != nil }

func (g *dedupGate) seen(ctx context.Context, key string) bool {
	if !g.enabled() || key == "" {
		return false
	}
	n, err := g.rdb.Exists(ctx, key).Result()
	if err != nil {
		// Fail open: a Redis outage must not stop ingestion.
		return false
	}
	return n > 0
}

// record stores dedup keys asynchronously after a successful flush.
func (g *dedupGate) record(keys []string) {
	if !g.enabled() || len(keys) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pipe := g.rdb.Pipeline()
	for _, k := range keys {
		if k != "" {
			pipe.Set(ctx, k, "1", g.ttl)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("clickhouse dedup record: %v", err)
	}
}

// CHConsumer wraps one NSQ consumer feeding a ClickHouse batcher.
type CHConsumer struct {
	consumer *nsq.Consumer
	stop     func()
}

// Stop stops pulling, waits for in-flight handlers, then drains the batcher
// (flush what fits, Requeue(0) the rest — design §5.2.2).
func (c *CHConsumer) Stop() {
	if c == nil {
		return
	}
	if c.consumer != nil {
		c.consumer.Stop()
		<-c.consumer.StopChan
	}
	if c.stop != nil {
		c.stop()
	}
}

// chEventItem carries the extra sidecar data the event flush callback needs.
type chEventItem struct {
	Event   model.Event
	Browser string
}

// NewCHLogConsumer consumes the logs topic into ClickHouse through an async
// batcher (channel: NSQ_LOG_CHANNEL_CH, default ch-log-consumer).
func NewCHLogConsumer(ctx context.Context, cfg config.Config, clients *clickhouse.Clients, gdb *gorm.DB, rdb *redis.Client, recorder *metrics.RedisRecorder, geoip *enrich.GeoIP, stats *obs.Stats) (*CHConsumer, error) {
	gate := newDedupGate(rdb)
	var evaluator *alert.AsyncEvaluator
	if cfg.CHEnableSidecars && gdb != nil {
		evaluator = alert.NewAsyncEvaluator(alert.NewEngine(gdb, nil))
	}

	batcher := clickhouse.NewBatcher[model.Log](
		clickhouse.BatcherOptions{
			BatchSize:     cfg.CHLogBatchSize,
			FlushInterval: cfg.CHLogFlushInterval,
			FlushTimeout:  cfg.CHFlushTimeout,
			DrainTimeout:  cfg.CHDrainTimeout,
			QueueSize:     cfg.CHQueueBufferSize,
			Shards:        cfg.CHWriteShards,
		},
		flushCHLogs(clients),
		func(items []clickhouse.Item[model.Log]) {
			gate.record(dedupKeysOf(items))
			if !cfg.CHEnableSidecars {
				return
			}
			sidecarCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			for i := range items {
				row := items[i].Row
				if recorder != nil {
					recorder.ObserveLog(sidecarCtx, row.ProjectID, row.Level, row.DistinctID, row.DeviceID, row.Timestamp)
					if geoip != nil && items[i].ClientIP != "" {
						if g, ok := geoip.Lookup(items[i].ClientIP); ok {
							recorder.ObserveEventDist(sidecarCtx, row.ProjectID, row.Timestamp, row.DistinctID, map[string]string{
								"country": g.Country,
								"region":  g.Region,
								"city":    g.City,
								"asn_org": g.ASNOrg,
							})
						}
					}
				}
				if evaluator != nil {
					evaluator.Submit(alert.InputFromLog(row))
				}
			}
		},
	)

	handler := nsq.HandlerFunc(func(m *nsq.Message) error {
		m.DisableAutoResponse()
		msgStart := time.Now()
		finish := func(err error) {
			m.Finish()
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), err)
			}
		}

		var msg ingest.NSQMessage
		if err := json.Unmarshal(m.Body, &msg); err != nil {
			finish(nil)
			return nil
		}
		if msg.Type != "log" {
			finish(nil)
			return nil
		}
		var lp ingest.CustomLogPayload
		if err := json.Unmarshal(msg.Payload, &lp); err != nil {
			finish(nil)
			return nil
		}
		if lp.Timestamp == nil {
			now := time.Now().UTC()
			lp.Timestamp = &now
		}
		if lp.Message == "" {
			finish(nil)
			return nil
		}
		var ingestID uuid.UUID
		copy(ingestID[:], m.ID[:])
		row, err := store.LogRowFromPayloadWithIngestID(msg.ProjectID, lp, ingestID)
		if err != nil {
			// Permanent payload error: requeueing would never succeed.
			log.Printf("ch consumer: dropping malformed log (project=%s): %v", msg.ProjectID, err)
			finish(err)
			return nil
		}

		tenantID := msg.TenantID
		if tenantID == "" {
			tenantID = string(tenant.DefaultTenantID)
		}
		dedupKey := ""
		if gate.enabled() {
			dedupKey = fmt.Sprintf("dedup:%s:%d:%s", tenantID, row.ProjectID, row.IngestID)
			gateCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			seen := gate.seen(gateCtx, dedupKey)
			cancel()
			if seen {
				chConsumerMetrics.Add("dedup_dropped_total", 1)
				finish(nil)
				return nil
			}
		}

		item := clickhouse.Item[model.Log]{
			Ctl:        m,
			Row:        row,
			TenantID:   tenantID,
			DedupKey:   dedupKey,
			ClientIP:   messageClientIP(msg),
			EnqueuedAt: time.Now(),
			Attempts:   int(m.Attempts),
		}
		if err := batcher.Enqueue(item); err != nil {
			m.Requeue(0)
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), err)
			}
			return nil
		}
		if stats != nil {
			stats.ObserveConsumerMessage(time.Since(msgStart), nil)
		}
		return nil
	})

	c, err := newCHNSQConsumer(ctx, cfg, "logs", cfg.NSQLogChannelCH, handler)
	if err != nil {
		batcher.Stop()
		if evaluator != nil {
			evaluator.Stop()
		}
		return nil, err
	}
	return &CHConsumer{consumer: c, stop: func() {
		batcher.Stop()
		if evaluator != nil {
			evaluator.Stop()
		}
	}}, nil
}

// NewCHEventConsumer consumes the events topic (Sentry-compatible events and
// raw envelopes) into ClickHouse.
func NewCHEventConsumer(ctx context.Context, cfg config.Config, clients *clickhouse.Clients, gdb *gorm.DB, rdb *redis.Client, recorder *metrics.RedisRecorder, geoip *enrich.GeoIP, stats *obs.Stats) (*CHConsumer, error) {
	gate := newDedupGate(rdb)
	var evaluator *alert.AsyncEvaluator
	if cfg.CHEnableSidecars && gdb != nil {
		evaluator = alert.NewAsyncEvaluator(alert.NewEngine(gdb, nil))
	}

	batcher := clickhouse.NewBatcher[chEventItem](
		clickhouse.BatcherOptions{
			BatchSize:     cfg.CHEventBatchSize,
			FlushInterval: cfg.CHEventFlushInterval,
			FlushTimeout:  cfg.CHFlushTimeout,
			DrainTimeout:  cfg.CHDrainTimeout,
			QueueSize:     cfg.CHQueueBufferSize,
			Shards:        cfg.CHWriteShards,
		},
		flushCHEvents(clients),
		func(items []clickhouse.Item[chEventItem]) {
			gate.record(dedupKeysOf(items))
			if !cfg.CHEnableSidecars {
				return
			}
			sidecarCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			for i := range items {
				row := items[i].Row.Event
				if recorder != nil {
					recorder.ObserveEvent(sidecarCtx, row.ProjectID, row.Level, row.DistinctID, row.DeviceID, row.OS, row.Timestamp)
					dims := map[string]string{"os": row.OS, "browser": items[i].Row.Browser}
					if geoip != nil && items[i].ClientIP != "" {
						if g, ok := geoip.Lookup(items[i].ClientIP); ok {
							dims["country"] = g.Country
							dims["region"] = g.Region
							dims["city"] = g.City
							dims["asn_org"] = g.ASNOrg
						}
					}
					recorder.ObserveEventDist(sidecarCtx, row.ProjectID, row.Timestamp, row.DistinctID, dims)
				}
				if evaluator != nil {
					evaluator.Submit(alert.InputFromEvent(row))
				}
			}
		},
	)

	handler := nsq.HandlerFunc(func(m *nsq.Message) error {
		m.DisableAutoResponse()
		msgStart := time.Now()
		finish := func(err error) {
			m.Finish()
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), err)
			}
		}

		var msg ingest.NSQMessage
		if err := json.Unmarshal(m.Body, &msg); err != nil {
			finish(nil)
			return nil
		}
		if msg.Type != "event" && msg.Type != "envelope" {
			finish(nil)
			return nil
		}
		var payload map[string]any
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			finish(nil)
			return nil
		}
		row, err := store.EventRowFromMap(msg.ProjectID, payload)
		if err != nil {
			log.Printf("ch consumer: dropping malformed event (project=%s): %v", msg.ProjectID, err)
			finish(err)
			return nil
		}

		tenantID := msg.TenantID
		if tenantID == "" {
			tenantID = string(tenant.DefaultTenantID)
		}
		dedupKey := ""
		if gate.enabled() && row.ID != uuid.Nil {
			dedupKey = fmt.Sprintf("dedup:%s:%d:%s", tenantID, row.ProjectID, row.ID)
			gateCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			seen := gate.seen(gateCtx, dedupKey)
			cancel()
			if seen {
				chConsumerMetrics.Add("dedup_dropped_total", 1)
				finish(nil)
				return nil
			}
		}

		item := clickhouse.Item[chEventItem]{
			Ctl:        m,
			Row:        chEventItem{Event: row, Browser: identity.ExtractBrowser(payload)},
			TenantID:   tenantID,
			DedupKey:   dedupKey,
			ClientIP:   messageClientIP(msg),
			EnqueuedAt: time.Now(),
			Attempts:   int(m.Attempts),
		}
		if err := batcher.Enqueue(item); err != nil {
			m.Requeue(0)
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), err)
			}
			return nil
		}
		if stats != nil {
			stats.ObserveConsumerMessage(time.Since(msgStart), nil)
		}
		return nil
	})

	c, err := newCHNSQConsumer(ctx, cfg, "events", cfg.NSQEventChannelCH, handler)
	if err != nil {
		batcher.Stop()
		if evaluator != nil {
			evaluator.Stop()
		}
		return nil, err
	}
	return &CHConsumer{consumer: c, stop: func() {
		batcher.Stop()
		if evaluator != nil {
			evaluator.Stop()
		}
	}}, nil
}

func flushCHLogs(clients *clickhouse.Clients) func(ctx context.Context, items []clickhouse.Item[model.Log]) error {
	return func(ctx context.Context, items []clickhouse.Item[model.Log]) error {
		rows := make([]model.Log, len(items))
		chRows := make([]clickhouse.LogRow, 0, len(items))
		tenantByIngest := make(map[uuid.UUID]string, len(items))
		for i := range items {
			rows[i] = items[i].Row
			r := clickhouse.LogRowsFromModel(rows[i : i+1])[0]
			r.WithTenant(tenant.ID(items[i].TenantID))
			chRows = append(chRows, r)
			if rows[i].IngestID != nil {
				tenantByIngest[*rows[i].IngestID] = items[i].TenantID
			}
		}
		if err := clickhouse.InsertLogs(ctx, clients.Write, chRows); err != nil {
			return err
		}
		te := store.TrackEventRowsFromLogs(rows)
		if len(te) == 0 {
			return nil
		}
		teRows := clickhouse.TrackEventRowsFromModel(te)
		for i := range teRows {
			if t, ok := tenantByIngest[teRows[i].IngestID]; ok {
				teRows[i].WithTenant(tenant.ID(t))
			} else {
				teRows[i].WithTenant(tenant.DefaultTenantID)
			}
		}
		return clickhouse.InsertTrackEvents(ctx, clients.Write, teRows)
	}
}

func flushCHEvents(clients *clickhouse.Clients) func(ctx context.Context, items []clickhouse.Item[chEventItem]) error {
	return func(ctx context.Context, items []clickhouse.Item[chEventItem]) error {
		events := make([]model.Event, len(items))
		for i := range items {
			events[i] = items[i].Row.Event
		}
		chRows := clickhouse.EventRowsFromModel(events)
		for i := range chRows {
			chRows[i].WithTenant(tenant.ID(items[i].TenantID))
		}
		return clickhouse.InsertEvents(ctx, clients.Write, chRows)
	}
}

func dedupKeysOf[T any](items []clickhouse.Item[T]) []string {
	keys := make([]string, 0, len(items))
	for i := range items {
		if items[i].DedupKey != "" {
			keys = append(keys, items[i].DedupKey)
		}
	}
	return keys
}

func messageClientIP(msg ingest.NSQMessage) string {
	if msg.Meta != nil {
		return msg.Meta.ClientIP
	}
	return ""
}

// newCHNSQConsumer mirrors newConsumer but with the CH-specific in-flight
// and channel settings (design §5.3).
func newCHNSQConsumer(ctx context.Context, cfg config.Config, topic, channel string, handler nsq.HandlerFunc) (*nsq.Consumer, error) {
	if channel == "" {
		channel = "ch-" + topic + "-consumer"
	}
	nsqCfg := nsq.NewConfig()
	nsqCfg.MaxInFlight = cfg.NSQMaxInFlightCH
	if nsqCfg.MaxInFlight <= 0 {
		nsqCfg.MaxInFlight = 50000
	}
	nsqCfg.MsgTimeout = 30 * time.Second
	cons, err := nsq.NewConsumer(topic, channel, nsqCfg)
	if err != nil {
		return nil, err
	}
	cons.SetLogger(log.New(log.Writer(), "nsq-ch ", log.LstdFlags), nsq.LogLevelInfo)
	concurrency := cfg.CHLogConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	cons.AddConcurrentHandlers(handler, concurrency)
	if err := connectToNSQDWithRetry(ctx, cons, cfg.NSQDAddress, topic, channel); err != nil {
		cons.Stop()
		return nil, err
	}
	return cons, nil
}
