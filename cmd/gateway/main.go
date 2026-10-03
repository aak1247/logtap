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
	"sync"
	"syscall"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/channel"
	"github.com/aak1247/logtap/internal/channel/builtin"
	"github.com/aak1247/logtap/internal/cleanup"
	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/consumer"
	"github.com/aak1247/logtap/internal/db"
	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/detector/packages/connectivity"
	"github.com/aak1247/logtap/internal/detector/plugins/httpcheck"
	"github.com/aak1247/logtap/internal/detector/plugins/logbasic"
	"github.com/aak1247/logtap/internal/detector/plugins/metricthreshold"
	"github.com/aak1247/logtap/internal/enrich"
	"github.com/aak1247/logtap/internal/httpserver"
	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/migrate"
	"github.com/aak1247/logtap/internal/monitor"
	"github.com/aak1247/logtap/internal/obs"
	"github.com/aak1247/logtap/internal/queue"
	"github.com/aak1247/logtap/internal/selflog"
	"github.com/aak1247/logtap/internal/store/clickhouse"
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
	if bp, ok := publisher.(queue.BatchPublisher); ok {
		asyncPub := queue.NewAsyncBufferedPublisher(bp)
		defer asyncPub.Stop()
		publisher = asyncPub
	}
	if cfg.NSQDHTTPAddress != "" {
		threshold := int64(cfg.NSQDepthAlertThreshold)
		// Track per-topic backlog: the poller reports logs and events in the
		// same tick, and backpressure must stay on while ANY topic is over
		// the threshold (not just the one seen last).
		var depthsMu sync.Mutex
		depths := map[string]int64{}
		// Run in the background: the poller loops until ctx is done and must
		// never block startup.
		go obs.StartNSQDepthPollerFunc(ctx, stats, cfg.NSQDHTTPAddress, 5*time.Second, func(topic string, total int64) {
			depthsMu.Lock()
			depths[topic] = total
			over := false
			for _, v := range depths {
				if v > threshold {
					over = true
					break
				}
			}
			depthsMu.Unlock()
			httpserver.SetIngestBackpressure(over)
		})
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

		migCtx, cancel := context.WithTimeout(ctx, cfg.DBMigrationTimeout)
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
		if gdb != nil && cfg.MetricsActiveWarmupDays > 0 {
			go func() {
				warmCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				defer cancel()
				if err := recorder.WarmActiveUsersFromDB(warmCtx, gdb, metrics.ActiveWarmupOptions{
					Days:      cfg.MetricsActiveWarmupDays,
					Months:    cfg.MetricsActiveWarmupMonths,
					BatchSize: cfg.MetricsActiveWarmupBatchSize,
				}); err != nil {
					log.Printf("metrics active warmup: %v", err)
				}
			}()
		}
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
	if err := detectorRegistry.RegisterStatic(metricthreshold.New()); err != nil {
		log.Printf("detector register static metric_threshold: %v", err)
	}
	if err := detectorRegistry.RegisterPackage(connectivity.New()); err != nil {
		log.Printf("detector register package connectivity: %v", err)
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

	var chClients *clickhouse.Clients
	// ClickHouse backend: connect (write + read) and run idempotent
	// migrations before the HTTP server starts, so query routing and the
	// consumers share one client pair (design §3.1/§3.2).
	if cfg.StorageBackend != config.StorageBackendPostgres && cfg.ClickHouseDSN != "" {
		readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		clients, err := clickhouse.WaitForClickHouse(readyCtx, cfg.ClickHouseDSN, cfg.ClickHouseReadDSN)
		cancel()
		if err != nil {
			log.Fatalf("clickhouse: %v", err)
		}
		chClients = clients
		defer chClients.Close()
		migCtx, cancel := context.WithTimeout(ctx, cfg.DBMigrationTimeout)
		if err := clickhouse.Migrate(migCtx, chClients.Write, clickhouse.MigrateOptions{
			LogTTLDays:    cfg.CHLogTTLDays,
			TextTokenizer: cfg.CHTextTokenizer,
		}); err != nil {
			cancel()
			log.Fatalf("clickhouse migrate: %v", err)
		}
		cancel()
	}
	var chRead chdriver.Conn
	if chClients != nil {
		chRead = chClients.Read
	}

	srv := httpserver.New(cfg, publisher, gdb, recorder, stats, detectorService, detectorStore, chRead)

	var eventConsumer *consumer.NSQConsumer
	var logConsumer *consumer.NSQConsumer
	// ClickHouse mode stops the Postgres data-plane consumers; postgres and
	// dual keep them (dual writes both backends through independent NSQ
	// channels).
	runPGDataConsumers := cfg.RunConsumers && cfg.StorageBackend != config.StorageBackendClickHouse
	if runPGDataConsumers {
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

	var chLogConsumer, chEventConsumer *consumer.CHConsumer
	if cfg.RunConsumers && cfg.StorageBackend != config.StorageBackendPostgres {
		if chClients == nil {
			log.Fatalf("CLICKHOUSE_DSN required when STORAGE_BACKEND=%s", cfg.StorageBackend)
		}

		var dedupRDB *redis.Client
		if cfg.CHDedupMode == config.CHDedupModeRedis && cfg.RedisAddr != "" {
			dedupRDB, err = metrics.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
			if err != nil {
				log.Printf("clickhouse dedup gate disabled (redis client: %v)", err)
				dedupRDB = nil
			} else {
				pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				if pingErr := dedupRDB.Ping(pingCtx).Err(); pingErr != nil {
					cancel()
					log.Printf("clickhouse dedup gate disabled (redis ping: %v)", pingErr)
					_ = dedupRDB.Close()
					dedupRDB = nil
				} else {
					cancel()
					defer dedupRDB.Close()
				}
			}
		} else {
			log.Printf("clickhouse dedup gate disabled (CH_DEDUP_MODE=%s REDIS_ADDR=%q)", cfg.CHDedupMode, cfg.RedisAddr)
		}

		chLogConsumer, err = consumer.NewCHLogConsumer(ctx, cfg, chClients, gdb, dedupRDB, recorder, geoip, stats)
		if err != nil {
			log.Fatalf("clickhouse log consumer: %v", err)
		}
		chEventConsumer, err = consumer.NewCHEventConsumer(ctx, cfg, chClients, gdb, dedupRDB, recorder, geoip, stats)
		if err != nil {
			log.Fatalf("clickhouse event consumer: %v", err)
		}
		log.Printf("clickhouse consumers enabled (backend=%s channels(logs=%s events=%s) batch=%d/%s shards=%d max_in_flight=%d)",
			cfg.StorageBackend, cfg.NSQLogChannelCH, cfg.NSQEventChannelCH, cfg.CHLogBatchSize, cfg.CHLogFlushInterval, cfg.CHWriteShards, cfg.NSQMaxInFlightCH)
	}

	if gdb != nil {
		w := cleanup.NewWorker(gdb)
		w.Interval = cfg.CleanupInterval
		w.Limit = cfg.CleanupPolicyLimit
		w.DeleteBatchSize = cfg.CleanupDeleteBatchSize
		w.MaxBatches = cfg.CleanupMaxBatches
		w.BatchSleep = cfg.CleanupBatchSleep
		w.Stats = stats
		w.MonitorRunsRetentionDays = cfg.MonitorRunsRetentionDays
		w.DetectorResultsRetentionDays = cfg.DetectorResultsRetentionDays
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
		log.Printf("consumers enabled (backend=%s events/logs)", cfg.StorageBackend)
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
	if runPGDataConsumers {
		eventConsumer.Stop()
		logConsumer.Stop()
	}
	// CH consumers drain their batchers on Stop: flush what fits inside
	// CH_DRAIN_TIMEOUT, Requeue(0) the rest (design §5.2.2).
	if chLogConsumer != nil {
		chLogConsumer.Stop()
	}
	if chEventConsumer != nil {
		chEventConsumer.Stop()
	}
	if chClients != nil {
		chClients.Close()
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
