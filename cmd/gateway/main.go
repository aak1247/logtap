package main

import (
	"context"
	"errors"
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
		d, err := db.NewGorm(ctx, cfg.PostgresURL)
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
		rdb, err := metrics.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		if err != nil {
			log.Fatalf("redis: %v", err)
		}
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		if err := rdb.Ping(pingCtx).Err(); err != nil {
			cancel()
			log.Fatalf("redis ping: %v", err)
		}
		cancel()
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
		eventConsumer, err = consumer.NewNSQEventConsumer(cfg, gdb, recorder, geoip)
		if err != nil {
			log.Fatalf("event consumer: %v", err)
		}
		logConsumer, err = consumer.NewNSQLogConsumer(cfg, gdb, recorder)
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
