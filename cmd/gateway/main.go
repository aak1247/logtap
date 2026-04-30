package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/channel"
	"github.com/aak1247/logtap/internal/channel/builtin"
	"github.com/aak1247/logtap/internal/cleanup"
	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/consumer"
	"github.com/aak1247/logtap/internal/db"
	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/detector/plugins/dnscheck"
	"github.com/aak1247/logtap/internal/detector/plugins/httpcheck"
	"github.com/aak1247/logtap/internal/detector/plugins/logbasic"
	"github.com/aak1247/logtap/internal/detector/plugins/metricthreshold"
	"github.com/aak1247/logtap/internal/detector/plugins/sslcheck"
	"github.com/aak1247/logtap/internal/detector/plugins/tcpcheck"
	"github.com/aak1247/logtap/internal/enrich"
	"github.com/aak1247/logtap/internal/httpserver"
	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/migrate"
	"github.com/aak1247/logtap/internal/monitor"
	"github.com/aak1247/logtap/internal/obs"
	"github.com/aak1247/logtap/internal/queue"
	"github.com/aak1247/logtap/internal/selflog"
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

	nsqPublisher, err := queue.NewNSQPublisher(cfg.NSQDAddress)
	if err != nil {
		log.Fatalf("nsq publisher: %v", err)
	}
	defer nsqPublisher.Stop()

	stats := obs.New()
	var publisher queue.Publisher = nsqPublisher
	publisher = queue.ObservePublisher(publisher, stats)
	if cfg.NSQDHTTPAddress != "" {
		go obs.StartNSQDepthPoller(ctx, stats, cfg.NSQDHTTPAddress, 5*time.Second)
	}

	var gdb *gorm.DB
	if cfg.PostgresURL != "" {
		readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		d, err := waitForPostgres(readyCtx, cfg.PostgresURL, db.Options{
			MaxOpenConns: cfg.DBMaxOpenConns,
			MaxIdleConns: cfg.DBMaxIdleConns,
		})
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
		if err := migrate.AutoMigrate(migCtx, gdb, migrate.Options{RequireTimescale: cfg.DBRequireTimescale}); err != nil {
			cancel()
			log.Fatalf("db migrate: %v", err)
		}
		cancel()
	}

	// Mirror service logs into the default project once it exists.
	if gdb != nil {
		base := log.Writer()
		log.SetOutput(io.MultiWriter(base, selflog.NewWriter(gdb, publisher, "logtap")))
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
		recorder = metrics.NewRedisRecorder(rdb, metrics.WithTTLs(cfg.MetricsDayTTL, cfg.MetricsDistTTL, cfg.MetricsMonthTTL))
	}

	geoip, err := enrich.NewGeoIP(cfg.GeoIPCityMMDB, cfg.GeoIPASNMMDB)
	if err != nil {
		log.Fatalf("geoip: %v", err)
	}
	if geoip != nil {
		defer geoip.Close()
	}

	channelReg, channelSvc, err := channel.Bootstrap(cfg)
	if err != nil {
		log.Fatalf("channel bootstrap: %v", err)
	}
	if err := builtin.RegisterAll(channelReg, cfg); err != nil {
		log.Fatalf("channel register builtins: %v", err)
	}

	detectorRegistry := detector.NewRegistry()
	detectorStore := detector.NewResultStore(gdb)
	if gdb != nil {
		if err := detectorStore.AutoMigrate(ctx); err != nil {
			log.Printf("detector store migrate: %v", err)
		}
	}
	if err := detectorRegistry.RegisterStatic(logbasic.New()); err != nil {
		log.Printf("detector register static log_basic: %v", err)
	}
	if err := detectorRegistry.RegisterStatic(httpcheck.New()); err != nil {
		log.Printf("detector register static http_check: %v", err)
	}
	if err := detectorRegistry.RegisterStatic(tcpcheck.New()); err != nil {
		log.Printf("detector register static tcp_check: %v", err)
	}
	if err := detectorRegistry.RegisterStatic(metricthreshold.New()); err != nil {
		log.Printf("detector register static metric_threshold: %v", err)
	}
	if err := detectorRegistry.RegisterStatic(dnscheck.New()); err != nil {
		log.Printf("detector register static dns_check: %v", err)
	}
	if err := detectorRegistry.RegisterStatic(sslcheck.New()); err != nil {
		log.Printf("detector register static ssl_check: %v", err)
	}
	dynamicLoaded := 0
	dynamicFailed := 0
	for _, dir := range cfg.DetectorPluginDirs {
		files, err := detector.PluginFiles(dir)
		if err != nil {
			log.Printf("detector plugin dir %q: %v", dir, err)
			dynamicFailed++
			continue
		}
		for _, path := range files {
			p, loadErr := detector.LoadPluginFile(path)
			if loadErr != nil {
				log.Printf("detector plugin load %q: %v", path, loadErr)
				dynamicFailed++
				continue
			}
			if regErr := detectorRegistry.RegisterDynamic(filepath.Clean(path), p); regErr != nil {
				log.Printf("detector plugin register %q: %v", path, regErr)
				dynamicFailed++
				continue
			}
			dynamicLoaded++
		}
	}
	log.Printf("detector registry initialized: total=%d dynamic_loaded=%d dynamic_failed=%d", len(detectorRegistry.List()), dynamicLoaded, dynamicFailed)
	detectorService := detector.NewService(detectorRegistry, detectorStore)

	srv := httpserver.New(cfg, publisher, gdb, recorder, stats, detectorService, detectorStore)

	var eventConsumer *consumer.NSQConsumer
	var logConsumer *consumer.NSQConsumer
	if cfg.RunConsumers {
		if gdb == nil {
			log.Fatalf("POSTGRES_URL required when RUN_CONSUMERS=true")
		}
		eventConsumer, err = consumer.NewNSQEventConsumer(ctx, cfg, gdb, recorder, geoip, stats)
		if err != nil {
			log.Fatalf("event consumer: %v", err)
		}
		logConsumer, err = consumer.NewNSQLogConsumer(ctx, cfg, gdb, recorder, geoip, stats)
		if err != nil {
			log.Fatalf("log consumer: %v", err)
		}
	}

	if gdb != nil {
		w := cleanup.NewWorker(gdb)
		w.Interval = cfg.CleanupInterval
		w.Limit = cfg.CleanupPolicyLimit
		w.DeleteBatchSize = cfg.CleanupDeleteBatchSize
		w.MaxBatches = cfg.CleanupMaxBatches
		w.BatchSleep = cfg.CleanupBatchSleep
		w.Stats = stats
		go w.Run(ctx)
		log.Printf("cleanup worker enabled")
	}

	if gdb != nil && cfg.RunAlertWorker {
		aw := alert.NewWorker(gdb, cfg)
		aw.ChannelSvc = channelSvc
		go func() {
			_ = aw.Run(ctx)
		}()
		log.Printf("alert worker enabled")
	}

	if gdb != nil && cfg.RunMonitorWorker {
		mw := monitor.NewWorker(gdb, detectorRegistry)
		mw.Store = detectorStore
		mw.TickInterval = cfg.MonitorTickInterval
		mw.BatchSize = cfg.MonitorBatchSize
		mw.LeaseDuration = cfg.MonitorLeaseDuration
		go func() {
			if err := mw.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("monitor worker: %v", err)
			}
		}()
		log.Printf("monitor worker enabled")
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
