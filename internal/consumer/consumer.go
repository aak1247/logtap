package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/enrich"
	"github.com/aak1247/logtap/internal/identity"
	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/obs"
	"github.com/aak1247/logtap/internal/store"
	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"
	"gorm.io/gorm"
)

type NSQConsumer struct {
	consumer *nsq.Consumer
	onStop   []func()
}

func NewNSQEventConsumer(ctx context.Context, cfg config.Config, db *gorm.DB, recorder *metrics.RedisRecorder, geoip *enrich.GeoIP, stats *obs.Stats) (*NSQConsumer, error) {
	channel := cfg.NSQEventChannel
	if channel == "" {
		channel = "event-consumer"
	}
	handler, cleanup := handleEventMessage(cfg, db, recorder, geoip, stats)
	c, err := newConsumer(ctx, cfg, "events", channel, cfg.NSQEventConcurrency, handler)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, err
	}
	if cleanup != nil {
		c.onStop = append(c.onStop, cleanup)
	}
	return c, nil
}

func NewNSQLogConsumer(ctx context.Context, cfg config.Config, db *gorm.DB, recorder *metrics.RedisRecorder, stats *obs.Stats) (*NSQConsumer, error) {
	channel := cfg.NSQLogChannel
	if channel == "" {
		channel = "log-consumer"
	}
	handler, cleanup := handleLogMessage(cfg, db, recorder, stats)
	c, err := newConsumer(ctx, cfg, "logs", channel, cfg.NSQLogConcurrency, handler)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, err
	}
	if cleanup != nil {
		c.onStop = append(c.onStop, cleanup)
	}
	return c, nil
}

func (c *NSQConsumer) Stop() {
	if c == nil || c.consumer == nil {
		return
	}
	c.consumer.Stop()
	<-c.consumer.StopChan
	for _, fn := range c.onStop {
		if fn != nil {
			fn()
		}
	}
}

func newConsumer(ctx context.Context, cfg config.Config, topic, channel string, concurrency int, handler nsq.HandlerFunc) (*NSQConsumer, error) {
	nsqCfg := nsq.NewConfig()
	if cfg.NSQMaxInFlight > 0 {
		nsqCfg.MaxInFlight = cfg.NSQMaxInFlight
	} else {
		nsqCfg.MaxInFlight = 200
	}
	nsqCfg.MsgTimeout = 30 * time.Second
	cons, err := nsq.NewConsumer(topic, channel, nsqCfg)
	if err != nil {
		return nil, err
	}
	cons.SetLogger(log.New(log.Writer(), "nsq ", log.LstdFlags), nsq.LogLevelInfo)
	if concurrency <= 0 {
		concurrency = 1
	}
	cons.AddConcurrentHandlers(handler, concurrency)

	if err := connectToNSQDWithRetry(ctx, cons, cfg.NSQDAddress, topic, channel); err != nil {
		cons.Stop()
		return nil, err
	}
	return &NSQConsumer{consumer: cons}, nil
}

func connectToNSQDWithRetry(ctx context.Context, cons *nsq.Consumer, addr, topic, channel string) error {
	const (
		totalWait = 2 * time.Minute
		maxDelay  = 5 * time.Second
	)
	deadline := time.Now().Add(totalWait)
	delay := 300 * time.Millisecond
	var lastErr error

	for {
		err := cons.ConnectToNSQD(addr)
		if err == nil {
			return nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("connect nsqd addr=%s topic=%s channel=%s: %w", addr, topic, channel, lastErr)
		}
		log.Printf("nsq connect failed (addr=%s topic=%s channel=%s): %v; retrying in %s", addr, topic, channel, lastErr, delay)

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func handleEventMessage(cfg config.Config, db *gorm.DB, recorder *metrics.RedisRecorder, geoip *enrich.GeoIP, stats *obs.Stats) (nsq.HandlerFunc, func()) {
	batcher := NewBatcher[model.Event](cfg.DBEventBatchSize, cfg.DBEventFlushInterval, 5*time.Second, func(ctx context.Context, rows []model.Event) error {
		start := time.Now()
		err := store.InsertEventsBatch(ctx, db, rows)
		if stats != nil {
			stats.ObserveDBFlush(len(rows), time.Since(start), err)
		}
		return err
	})

	return nsq.HandlerFunc(func(m *nsq.Message) error {
		msgStart := time.Now()
		var msg ingest.NSQMessage
		if err := json.Unmarshal(m.Body, &msg); err != nil {
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), nil)
			}
			return nil
		}

		switch msg.Type {
		case "event":
			var event map[string]any
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				return nil
			}
			row, err := store.EventRowFromMap(msg.ProjectID, event)
			if err != nil {
				if stats != nil {
					stats.ObserveConsumerMessage(time.Since(msgStart), err)
				}
				return err
			}

			if err := batcher.Add(row); err != nil {
				if stats != nil {
					stats.ObserveConsumerMessage(time.Since(msgStart), err)
				}
				return err
			}

			if recorder != nil {
				metricsCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				browser := identity.ExtractBrowser(event)
				recorder.ObserveEvent(metricsCtx, row.ProjectID, row.Level, row.DistinctID, row.DeviceID, row.OS, row.Timestamp)
				dims := map[string]string{
					"os":      row.OS,
					"browser": browser,
				}
				if geoip != nil && msg.Meta != nil && msg.Meta.ClientIP != "" {
					if g, ok := geoip.Lookup(msg.Meta.ClientIP); ok {
						dims["country"] = g.Country
						dims["region"] = g.Region
						dims["city"] = g.City
						dims["asn_org"] = g.ASNOrg
					}
				}
				recorder.ObserveEventDist(metricsCtx, row.ProjectID, row.Timestamp, dims)
			}
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), nil)
			}
			return nil
		case "envelope":
			// MVP: store raw envelope in events.data to avoid dropping it.
			var payload map[string]any
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				return nil
			}

			row, err := store.EventRowFromMap(msg.ProjectID, payload)
			if err != nil {
				if stats != nil {
					stats.ObserveConsumerMessage(time.Since(msgStart), err)
				}
				return err
			}
			if err := batcher.Add(row); err != nil {
				if stats != nil {
					stats.ObserveConsumerMessage(time.Since(msgStart), err)
				}
				return err
			}
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), nil)
			}
			return nil
		default:
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), nil)
			}
			return nil
		}
	}), batcher.Close
}

func handleLogMessage(cfg config.Config, db *gorm.DB, recorder *metrics.RedisRecorder, stats *obs.Stats) (nsq.HandlerFunc, func()) {
	batcher := NewBatcher[model.Log](cfg.DBLogBatchSize, cfg.DBLogFlushInterval, 5*time.Second, func(ctx context.Context, rows []model.Log) error {
		start := time.Now()
		err := store.InsertLogsAndTrackEventsBatch(ctx, db, rows)
		if stats != nil {
			stats.ObserveDBFlush(len(rows), time.Since(start), err)
		}
		return err
	})

	return nsq.HandlerFunc(func(m *nsq.Message) error {
		msgStart := time.Now()
		var msg ingest.NSQMessage
		if err := json.Unmarshal(m.Body, &msg); err != nil {
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), nil)
			}
			return nil
		}
		if msg.Type != "log" {
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), nil)
			}
			return nil
		}
		var lp ingest.CustomLogPayload
		if err := json.Unmarshal(msg.Payload, &lp); err != nil {
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), nil)
			}
			return nil
		}
		if lp.Timestamp == nil {
			now := time.Now().UTC()
			lp.Timestamp = &now
		}
		if lp.Message == "" {
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), nil)
			}
			return nil
		}

		var ingestID uuid.UUID
		copy(ingestID[:], m.ID[:])
		row, err := store.LogRowFromPayloadWithIngestID(msg.ProjectID, lp, ingestID)
		if err != nil {
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), err)
			}
			return err
		}
		if err := batcher.Add(row); err != nil {
			if stats != nil {
				stats.ObserveConsumerMessage(time.Since(msgStart), err)
			}
			return err
		}
		if recorder != nil {
			metricsCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			recorder.ObserveLog(metricsCtx, row.ProjectID, row.Level, row.DistinctID, row.DeviceID, row.Timestamp)
		}
		if stats != nil {
			stats.ObserveConsumerMessage(time.Since(msgStart), nil)
		}
		return nil
	}), batcher.Close
}
