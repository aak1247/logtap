package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/consumer"
	"github.com/aak1247/logtap/internal/db"
	"github.com/aak1247/logtap/internal/enrich"
	"github.com/aak1247/logtap/internal/httpserver"
	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/migrate"
	"github.com/aak1247/logtap/internal/queue"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("config: %s", cfg.String())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	publisher, err := queue.NewNSQPublisher(cfg.NSQDAddress)
	if err != nil {
		log.Fatalf("nsq publisher: %v", err)
	}
	defer publisher.Stop()

	var gdb *gorm.DB
	if cfg.PostgresURL != "" {
		readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		d, err := waitForPostgres(readyCtx, cfg.PostgresURL)
		cancel()
		if err != nil {
			log.Fatalf("db: %v", err)
		}
		gdb = d
		sqlDB, err := gdb.DB()
		if err != nil {
			log.Fatalf("db: %v", err)
		}
		defer sqlDB.Close()

		migCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := migrate.AutoMigrate(migCtx, gdb); err != nil {
			cancel()
			log.Fatalf("db migrate: %v", err)
		}
		cancel()
	}

	var recorder *metrics.RedisRecorder
	if cfg.EnableMetrics {
		readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		rdb, err := waitForRedis(readyCtx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		cancel()
		if err != nil {
			log.Fatalf("redis: %v", err)
		}
		defer rdb.Close()
		recorder = metrics.NewRedisRecorder(rdb)
	}

	geoip, err := enrich.NewGeoIP(cfg.GeoIPCityMMDB, cfg.GeoIPASNMMDB)
	if err != nil {
		log.Fatalf("geoip: %v", err)
	}
	if geoip != nil {
		defer geoip.Close()
	}

	srv := httpserver.New(cfg, publisher, gdb, recorder)

	var eventConsumer *consumer.NSQConsumer
	var logConsumer *consumer.NSQConsumer
	if cfg.RunConsumers {
		if gdb == nil {
			log.Fatalf("POSTGRES_URL required when RUN_CONSUMERS=true")
		}
		eventConsumer, err = consumer.NewNSQEventConsumer(ctx, cfg, gdb, recorder, geoip)
		if err != nil {
			log.Fatalf("event consumer: %v", err)
		}
		logConsumer, err = consumer.NewNSQLogConsumer(ctx, cfg, gdb, recorder)
		if err != nil {
			log.Fatalf("log consumer: %v", err)
		}
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Printf("http listening on %s", cfg.HTTPAddr)

	if cfg.RunConsumers {
		log.Printf("consumers enabled (events/logs)")
	}

	select {
	case <-ctx.Done():
		log.Printf("shutdown requested")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	if cfg.RunConsumers {
		eventConsumer.Stop()
		logConsumer.Stop()
	}
}

func waitForPostgres(ctx context.Context, postgresURL string) (*gorm.DB, error) {
	const maxDelay = 5 * time.Second
	delay := 300 * time.Millisecond
	var lastErr error
	for {
		d, err := db.NewGorm(ctx, postgresURL)
		if err == nil {
			return d, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, fmt.Errorf("postgres not ready: %w (last error: %v)", ctx.Err(), lastErr)
		}
		log.Printf("postgres not ready: %v; retrying in %s", lastErr, delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("postgres not ready: %w (last error: %v)", ctx.Err(), lastErr)
		case <-timer.C:
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func waitForRedis(ctx context.Context, addr, password string, dbIndex int) (*redis.Client, error) {
	const maxDelay = 5 * time.Second
	delay := 300 * time.Millisecond
	var lastErr error
	for {
		rdb, err := metrics.NewRedisClient(addr, password, dbIndex)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			lastErr = rdb.Ping(pingCtx).Err()
			cancel()
			if lastErr == nil {
				return rdb, nil
			}
			_ = rdb.Close()
		} else {
			lastErr = err
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("redis not ready: %w (last error: %v)", ctx.Err(), lastErr)
		}
		log.Printf("redis not ready: %v; retrying in %s", lastErr, delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("redis not ready: %w (last error: %v)", ctx.Err(), lastErr)
		case <-timer.C:
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}
