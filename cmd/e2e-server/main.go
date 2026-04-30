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

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/detector/plugins/logbasic"
	"github.com/aak1247/logtap/internal/httpserver"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/monitor"
	"github.com/aak1247/logtap/internal/testkit"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	gin.SetMode(gin.ReleaseMode)

	cfg := config.Config{
		HTTPAddr:             getenvDefault("HTTP_ADDR", "127.0.0.1:18080"),
		AuthSecret:           []byte("01234567890123456789012345678901"),
		AuthTokenTTL:         24 * time.Hour,
		WebhookAllowLoopback: true,
	}

	db, err := openDB(getenvDefault("E2E_SQLITE_DSN", "file:e2e-ui?mode=memory&cache=shared"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	publisher := &testkit.InlinePublisher{DB: db}
	reg := detector.NewRegistry()
	if err := reg.RegisterStatic(logbasic.New()); err != nil {
		log.Fatalf("register static detector: %v", err)
	}
	detectorService := detector.NewService(reg, nil)

	srv := httpserver.New(cfg, publisher, db, nil, nil, detectorService, nil)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if parseBoolDefault(os.Getenv("RUN_MONITOR_WORKER"), false) {
		mw := monitor.NewWorker(db, reg)
		mw.TickInterval = 200 * time.Millisecond
		mw.BatchSize = 20
		mw.LeaseDuration = 10 * time.Second
		go func() {
			if err := mw.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("monitor worker stopped: %v", err)
			}
		}()
	}
	if parseBoolDefault(os.Getenv("RUN_ALERT_WORKER"), false) {
		aw := alert.NewWorker(db, cfg)
		go func() {
			if err := aw.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("alert worker stopped: %v", err)
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Printf("e2e server listening on %s", cfg.HTTPAddr)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func openDB(dsn string) (*gorm.DB, error) {
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)

	if err := gdb.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.ProjectKey{},
		&model.Event{},
		&model.Log{},
		&model.TrackEvent{},
		&model.TrackEventDaily{},
		&model.AlertContact{},
		&model.AlertContactGroup{},
		&model.AlertContactGroupMember{},
		&model.AlertWecomBot{},
		&model.AlertWebhookEndpoint{},
		&model.AlertRule{},
		&model.AlertState{},
		&model.AlertDelivery{},
		&model.MonitorDefinition{},
		&model.MonitorRun{},
	); err != nil {
		return nil, err
	}
	return gdb, nil
}

func getenvDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func parseBoolDefault(raw string, def bool) bool {
	switch raw {
	case "1", "true", "TRUE", "True", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "False", "no", "NO", "off", "OFF":
		return false
	default:
		return def
	}
}
