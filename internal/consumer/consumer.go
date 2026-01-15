package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/enrich"
	"github.com/aak1247/logtap/internal/identity"
	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/project"
	"github.com/aak1247/logtap/internal/store"
	"github.com/nsqio/go-nsq"
	"gorm.io/gorm"
)

type NSQConsumer struct {
	consumer *nsq.Consumer
}

func NewNSQEventConsumer(ctx context.Context, cfg config.Config, db *gorm.DB, recorder *metrics.RedisRecorder, geoip *enrich.GeoIP) (*NSQConsumer, error) {
	channel := cfg.NSQEventChannel
	if channel == "" {
		channel = "event-consumer"
	}
	return newConsumer(ctx, cfg, "events", channel, handleEventMessage(db, recorder, geoip))
}

func NewNSQLogConsumer(ctx context.Context, cfg config.Config, db *gorm.DB, recorder *metrics.RedisRecorder) (*NSQConsumer, error) {
	channel := cfg.NSQLogChannel
	if channel == "" {
		channel = "log-consumer"
	}
	return newConsumer(ctx, cfg, "logs", channel, handleLogMessage(db, recorder))
}

func (c *NSQConsumer) Stop() {
	if c == nil || c.consumer == nil {
		return
	}
	c.consumer.Stop()
	<-c.consumer.StopChan
}

func newConsumer(ctx context.Context, cfg config.Config, topic, channel string, handler nsq.HandlerFunc) (*NSQConsumer, error) {
	nsqCfg := nsq.NewConfig()
	nsqCfg.MaxInFlight = 200
	nsqCfg.MsgTimeout = 30 * time.Second
	cons, err := nsq.NewConsumer(topic, channel, nsqCfg)
	if err != nil {
		return nil, err
	}
	cons.SetLogger(log.New(log.Writer(), "nsq ", log.LstdFlags), nsq.LogLevelInfo)
	cons.AddHandler(handler)

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

func handleEventMessage(db *gorm.DB, recorder *metrics.RedisRecorder, geoip *enrich.GeoIP) nsq.HandlerFunc {
	return nsq.HandlerFunc(func(m *nsq.Message) error {
		var msg ingest.NSQMessage
		if err := json.Unmarshal(m.Body, &msg); err != nil {
			return nil
		}

		switch msg.Type {
		case "event":
			var event map[string]any
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := store.InsertEvent(ctx, db, msg.ProjectID, event)
			if err != nil {
				return err
			}
			if recorder != nil {
				if pid, err := project.ParseID(msg.ProjectID); err == nil {
					distinctID, _ := identity.ExtractDistinctID(event)
					deviceID := identity.ExtractDeviceID(event)
					osName := identity.ExtractOS(event)
					browser := identity.ExtractBrowser(event)
					level, _ := event["level"].(string)
					ts := identity.ExtractTimestamp(event, time.Now().UTC())
					recorder.ObserveEvent(ctx, pid, level, distinctID, deviceID, osName, ts)
					dims := map[string]string{
						"os":      osName,
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
					recorder.ObserveEventDist(ctx, pid, ts, dims)
				}
			}
			return nil
		case "envelope":
			// MVP: store raw envelope in events.data to avoid dropping it.
			var payload map[string]any
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := store.InsertEvent(ctx, db, msg.ProjectID, payload)
			if err != nil {
				return err
			}
			return nil
		default:
			return nil
		}
	})
}

func handleLogMessage(db *gorm.DB, recorder *metrics.RedisRecorder) nsq.HandlerFunc {
	return nsq.HandlerFunc(func(m *nsq.Message) error {
		var msg ingest.NSQMessage
		if err := json.Unmarshal(m.Body, &msg); err != nil {
			return nil
		}
		if msg.Type != "log" {
			return nil
		}
		var lp ingest.CustomLogPayload
		if err := json.Unmarshal(msg.Payload, &lp); err != nil {
			return nil
		}
		if lp.Timestamp == nil {
			now := time.Now().UTC()
			lp.Timestamp = &now
		}
		if lp.Message == "" {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.InsertLog(ctx, db, msg.ProjectID, lp); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return err
		}
		if recorder != nil {
			if pid, err := project.ParseID(msg.ProjectID); err == nil {
				distinctID := ""
				if id := identity.ExtractUserID(map[string]any{"user": lp.User}); id != "" {
					distinctID = id
				} else if lp.DeviceID != "" {
					distinctID = lp.DeviceID
				}
				recorder.ObserveLog(ctx, pid, lp.Level, distinctID, lp.DeviceID, *lp.Timestamp)
			}
		}
		return nil
	})
}
