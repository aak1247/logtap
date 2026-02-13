package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/db"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.FromEnvAlertWorker()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	gdb, err := waitForPostgres(readyCtx, cfg.PostgresURL, db.Options{
		MaxOpenConns: cfg.DBMaxOpenConns,
		MaxIdleConns: cfg.DBMaxIdleConns,
	})
	cancel()
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer sqlDB.Close()

	w := alert.NewWorker(gdb, cfg)
	log.Printf("alert-worker started")
	err = w.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("alert-worker: %v", err)
	}
	log.Printf("alert-worker stopped")
}

func waitForPostgres(ctx context.Context, postgresURL string, opts db.Options) (*gorm.DB, error) {
	const maxDelay = 5 * time.Second
	delay := 300 * time.Millisecond
	var lastErr error
	for {
		d, err := db.NewGorm(ctx, postgresURL, opts)
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
