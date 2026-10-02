package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	StorageBackendPostgres   = "postgres"
	StorageBackendDual       = "dual"
	StorageBackendClickHouse = "clickhouse"
)

const (
	CHDedupModeRedis = "redis"
	CHDedupModeNone  = "none"
)

type Config struct {
	HTTPAddr                     string
	NSQDAddress                  string
	NSQDHTTPAddress              string
	PostgresURL                  string
	RunConsumers                 bool
	RunAlertWorker               bool
	NSQEventChannel              string
	NSQLogChannel                string
	NSQMaxInFlight               int
	NSQEventConcurrency          int
	NSQLogConcurrency            int
	DBMaxOpenConns               int
	DBMaxIdleConns               int
	DBLogBatchSize               int
	DBLogFlushInterval           time.Duration
	DBEventBatchSize             int
	DBEventFlushInterval         time.Duration
	CleanupInterval              time.Duration
	CleanupPolicyLimit           int
	CleanupDeleteBatchSize       int
	CleanupMaxBatches            int
	CleanupBatchSleep            time.Duration
	RedisAddr                    string
	RedisPassword                string
	RedisDB                      int
	EnableMetrics                bool
	MetricsDayTTL                time.Duration
	MetricsDistTTL               time.Duration
	MetricsMonthTTL              time.Duration
	MetricsActiveWarmupDays      int
	MetricsActiveWarmupMonths    int
	MetricsActiveWarmupBatchSize int
	GeoIPCityMMDB                string
	GeoIPASNMMDB                 string
	AuthSecret                   []byte
	AuthTokenTTL                 time.Duration
	MaintenanceMode              bool
	LogtapProxySecret            string
	MigrationCloudURL            string
	EnableDebugEndpoints         bool
	DBRequireTimescale           bool
	DBMigrationTimeout           time.Duration
	DetectorPluginDirs           []string
	RunMonitorWorker             bool
	MonitorTickInterval          time.Duration
	MonitorBatchSize             int
	MonitorLeaseDuration         time.Duration

	// Storage backend selection: postgres | dual | clickhouse (design doc
	// docs/CLICKHOUSE_STORAGE_DESIGN.md §5.3). postgres keeps today's
	// behavior; dual fans messages out to a second ClickHouse consumer;
	// clickhouse stops the Postgres data-plane consumers.
	StorageBackend       string
	ClickHouseDSN        string
	ClickHouseReadDSN    string
	CHWriteShards        int
	CHLogBatchSize       int
	CHLogFlushInterval   time.Duration
	CHEventBatchSize     int
	CHEventFlushInterval time.Duration
	CHFlushTimeout       time.Duration
	CHDrainTimeout       time.Duration
	CHQueueBufferSize    int
	CHDedupMode          string // redis | none
	CHEnableSidecars     bool   // drive Redis recorder + alert evaluator from the CH consumer
	CHLogTTLDays         int
	// CHTextTokenizer selects the full-text tokenizer for the message text
	// index. The 26.3 stock build ships splitByNonAlpha/splitByString only;
	// builds with CJK tokenizers (e.g. jieba) can opt in via env.
	CHTextTokenizer string

	NSQMaxInFlightCH  int
	NSQLogChannelCH   string
	NSQEventChannelCH string
	CHLogConcurrency  int

	// Backpressure guard: when the observed NSQ topic depth exceeds this,
	// ingest endpoints answer 503 + Retry-After.
	NSQDepthAlertThreshold int

	// Query-side concurrency protection for the ClickHouse backend.
	CHShortQueryMaxConcurrent int
	CHHeavyQueryMaxConcurrent int

	// Query gray-release routing to ClickHouse (migration Phase 2).
	QueryBackendProjects []int
	QueryBackendPercent  int

	// Webhook security (optional). Defaults to denying loopback/private IPs.
	WebhookAllowLoopback   bool
	WebhookAllowPrivateIPs bool
	WebhookAllowlistCIDRs  []netip.Prefix

	// Alert cleanup (optional). Disabled when retention days <= 0.
	AlertCleanupInterval         time.Duration
	AlertDeliveriesRetentionDays int
	AlertStatesRetentionDays     int

	// Operational-telemetry retention (optional). monitor_runs and
	// detector_results grow one row per check; disabled when days <= 0.
	MonitorRunsRetentionDays     int
	DetectorResultsRetentionDays int

	// Alerting / notifications (optional).
	SMTPHost     string
	SMTPPort     int
	SMTPFrom     string
	SMTPUsername string
	SMTPPassword string

	SMSProvider string

	AliyunSMSAccessKeyID     string
	AliyunSMSAccessKeySecret string
	AliyunSMSSignName        string
	AliyunSMSTemplateCode    string
	AliyunSMSRegion          string

	TencentSMSSecretID   string
	TencentSMSSecretKey  string
	TencentSMSAppID      string
	TencentSMSSignName   string
	TencentSMSTemplateID string
	TencentSMSRegion     string
}

func FromEnv() (Config, error) {
	authSecretRaw := strings.TrimSpace(os.Getenv("AUTH_SECRET"))
	var authSecret []byte
	authSecretFile := strings.TrimSpace(os.Getenv("AUTH_SECRET_FILE"))
	if authSecretRaw == "" && authSecretFile != "" {
		raw, err := os.ReadFile(authSecretFile)
		if err != nil {
			return Config{}, fmt.Errorf("read AUTH_SECRET_FILE: %w", err)
		}
		authSecretRaw = strings.TrimSpace(string(raw))
	}
	if authSecretRaw != "" {
		b, err := decodeBase64Any(authSecretRaw)
		if err != nil {
			return Config{}, errors.New("invalid AUTH_SECRET (expected base64)")
		}
		if len(b) < 32 {
			return Config{}, errors.New("AUTH_SECRET too short (need >= 32 bytes)")
		}
		authSecret = b
	}
	if len(authSecret) == 0 {
		msg := `AUTH_SECRET is required (base64, decoded length >= 32 bytes).

How to fix:
- Generate: task auth:secret
- Or PowerShell: powershell -ExecutionPolicy Bypass -File scripts/gen-auth-secret.ps1
- Then set env AUTH_SECRET=<output> and restart.

Optional: set AUTH_SECRET_FILE=/path/to/secret (file contains the base64 secret).`
		return Config{}, errors.New(msg)
	}

	cfg := Config{
		HTTPAddr:                     getenvDefault("HTTP_ADDR", ":8080"),
		NSQDAddress:                  getenvDefault("NSQD_ADDRESS", "127.0.0.1:4150"),
		NSQDHTTPAddress:              strings.TrimSpace(os.Getenv("NSQD_HTTP_ADDRESS")),
		PostgresURL:                  strings.TrimSpace(os.Getenv("POSTGRES_URL")),
		NSQEventChannel:              getenvDefault("NSQ_EVENT_CHANNEL", "event-consumer"),
		NSQLogChannel:                getenvDefault("NSQ_LOG_CHANNEL", "log-consumer"),
		NSQMaxInFlight:               parseIntDefault(getenvDefault("NSQ_MAX_IN_FLIGHT", "2000"), 2000),
		NSQEventConcurrency:          parseIntDefault(getenvDefault("NSQ_EVENT_CONCURRENCY", "50"), 50),
		NSQLogConcurrency:            parseIntDefault(getenvDefault("NSQ_LOG_CONCURRENCY", "200"), 200),
		DBMaxOpenConns:               parseIntDefault(getenvDefault("DB_MAX_OPEN_CONNS", "25"), 25),
		DBMaxIdleConns:               parseIntDefault(getenvDefault("DB_MAX_IDLE_CONNS", "5"), 5),
		DBLogBatchSize:               parseIntDefault(getenvDefault("DB_LOG_BATCH_SIZE", "500"), 500),
		DBLogFlushInterval:           parseDurationDefault(getenvDefault("DB_LOG_FLUSH_INTERVAL", "20ms"), 20*time.Millisecond),
		DBEventBatchSize:             parseIntDefault(getenvDefault("DB_EVENT_BATCH_SIZE", "200"), 200),
		DBEventFlushInterval:         parseDurationDefault(getenvDefault("DB_EVENT_FLUSH_INTERVAL", "20ms"), 20*time.Millisecond),
		CleanupInterval:              parseDurationDefault(getenvDefault("CLEANUP_INTERVAL", "10m"), 10*time.Minute),
		CleanupPolicyLimit:           parseIntDefault(getenvDefault("CLEANUP_POLICY_LIMIT", "50"), 50),
		CleanupDeleteBatchSize:       parseIntDefault(getenvDefault("CLEANUP_DELETE_BATCH_SIZE", "5000"), 5000),
		CleanupMaxBatches:            parseIntDefault(getenvDefault("CLEANUP_MAX_BATCHES", "50"), 50),
		CleanupBatchSleep:            parseDurationDefault(getenvDefault("CLEANUP_BATCH_SLEEP", "0s"), 0),
		MonitorRunsRetentionDays:     parseIntDefault(getenvDefault("MONITOR_RUNS_RETENTION_DAYS", "0"), 0),
		DetectorResultsRetentionDays: parseIntDefault(getenvDefault("DETECTOR_RESULTS_RETENTION_DAYS", "0"), 0),
		RedisAddr:                    strings.TrimSpace(os.Getenv("REDIS_ADDR")),
		RedisPassword:                os.Getenv("REDIS_PASSWORD"),
		RedisDB:                      parseIntDefault(getenvDefault("REDIS_DB", "0"), 0),
		MetricsDayTTL:                parseDurationDefault(getenvDefault("METRICS_DAY_TTL", "4320h"), 180*24*time.Hour),
		MetricsDistTTL:               parseDurationDefault(getenvDefault("METRICS_DIST_TTL", "2160h"), 90*24*time.Hour),
		MetricsMonthTTL:              parseDurationDefault(getenvDefault("METRICS_MONTH_TTL", "13392h"), 18*31*24*time.Hour),
		MetricsActiveWarmupDays:      parseIntDefault(getenvDefault("METRICS_ACTIVE_WARMUP_DAYS", "30"), 30),
		MetricsActiveWarmupMonths:    parseIntDefault(getenvDefault("METRICS_ACTIVE_WARMUP_MONTHS", "6"), 6),
		MetricsActiveWarmupBatchSize: parseIntDefault(getenvDefault("METRICS_ACTIVE_WARMUP_BATCH_SIZE", "1000"), 1000),
		GeoIPCityMMDB:                strings.TrimSpace(os.Getenv("GEOIP_CITY_MMDB")),
		GeoIPASNMMDB:                 strings.TrimSpace(os.Getenv("GEOIP_ASN_MMDB")),
		AuthSecret:                   authSecret,
		MaintenanceMode:              parseBoolDefault(getenvDefault("MAINTENANCE_MODE", "false"), false),
		LogtapProxySecret:            strings.TrimSpace(os.Getenv("LOGTAP_PROXY_SECRET")),
		MigrationCloudURL:            strings.TrimRight(strings.TrimSpace(getenvDefault("LOGTAP_CLOUD_URL", "https://logtap.hivescale.net")), "/"),
		EnableDebugEndpoints:         parseBoolDefault(getenvDefault("ENABLE_DEBUG_ENDPOINTS", "false"), false),
		DBRequireTimescale:           parseBoolDefault(getenvDefault("DB_REQUIRE_TIMESCALE", "false"), false),
		DBMigrationTimeout:           parseDurationDefault(getenvDefault("DB_MIGRATION_TIMEOUT", "2m"), 2*time.Minute),
		DetectorPluginDirs:           parseStringListEnv(getenvDefault("DETECTOR_PLUGIN_DIRS", "")),
		RunMonitorWorker:             parseBoolDefault(getenvDefault("RUN_MONITOR_WORKER", "false"), false),
		MonitorTickInterval:          parseDurationDefault(getenvDefault("MONITOR_TICK_INTERVAL", "2s"), 2*time.Second),
		MonitorBatchSize:             parseIntDefault(getenvDefault("MONITOR_BATCH_SIZE", "20"), 20),
		MonitorLeaseDuration:         parseDurationDefault(getenvDefault("MONITOR_LEASE_DURATION", "60s"), 60*time.Second),
		StorageBackend:               strings.ToLower(getenvDefault("STORAGE_BACKEND", StorageBackendPostgres)),
		ClickHouseDSN:                strings.TrimSpace(os.Getenv("CLICKHOUSE_DSN")),
		ClickHouseReadDSN:            strings.TrimSpace(os.Getenv("CLICKHOUSE_READ_DSN")),
		CHWriteShards:                parseIntDefault(getenvDefault("CH_WRITE_SHARDS", "2"), 2),
		CHLogBatchSize:               parseIntDefault(getenvDefault("CH_LOG_BATCH_SIZE", "5000"), 5000),
		CHLogFlushInterval:           parseDurationDefault(getenvDefault("CH_LOG_FLUSH_INTERVAL", "500ms"), 500*time.Millisecond),
		CHEventBatchSize:             parseIntDefault(getenvDefault("CH_EVENT_BATCH_SIZE", "2000"), 2000),
		CHEventFlushInterval:         parseDurationDefault(getenvDefault("CH_EVENT_FLUSH_INTERVAL", "500ms"), 500*time.Millisecond),
		CHFlushTimeout:               parseDurationDefault(getenvDefault("CH_FLUSH_TIMEOUT", "10s"), 10*time.Second),
		CHDrainTimeout:               parseDurationDefault(getenvDefault("CH_DRAIN_TIMEOUT", "20s"), 20*time.Second),
		CHQueueBufferSize:            parseIntDefault(getenvDefault("CH_QUEUE_BUFFER_SIZE", "100000"), 100000),
		CHDedupMode:                  strings.ToLower(getenvDefault("CH_DEDUP_MODE", CHDedupModeRedis)),
		CHEnableSidecars:             parseBoolDefault(getenvDefault("CH_ENABLE_SIDECARS", "false"), false),
		CHLogTTLDays:                 parseIntDefault(getenvDefault("CH_LOG_TTL_DAYS", "30"), 30),
		CHTextTokenizer:              strings.ToLower(getenvDefault("CH_TEXT_TOKENIZER", "splitbynonalpha")),
		NSQMaxInFlightCH:             parseIntDefault(getenvDefault("NSQ_MAX_IN_FLIGHT_CH", "50000"), 50000),
		NSQLogChannelCH:              getenvDefault("NSQ_LOG_CHANNEL_CH", "ch-log-consumer"),
		NSQEventChannelCH:            getenvDefault("NSQ_EVENT_CHANNEL_CH", "ch-event-consumer"),
		CHLogConcurrency:             parseIntDefault(getenvDefault("CH_LOG_CONCURRENCY", "50"), 50),
		NSQDepthAlertThreshold:       parseIntDefault(getenvDefault("NSQ_DEPTH_ALERT_THRESHOLD", "100000"), 100000),
		CHShortQueryMaxConcurrent:    parseIntDefault(getenvDefault("CH_SHORT_QUERY_MAX_CONCURRENT", "64"), 64),
		CHHeavyQueryMaxConcurrent:    parseIntDefault(getenvDefault("CH_HEAVY_QUERY_MAX_CONCURRENT", "8"), 8),
		QueryBackendProjects:         parseIntListEnv(getenvDefault("QUERY_BACKEND_PROJECTS", "")),
		QueryBackendPercent:          parseIntDefault(getenvDefault("QUERY_BACKEND_PERCENT", "0"), 0),
		WebhookAllowLoopback:         parseBoolDefault(getenvDefault("WEBHOOK_ALLOW_LOOPBACK", "false"), false),
		WebhookAllowPrivateIPs:       parseBoolDefault(getenvDefault("WEBHOOK_ALLOW_PRIVATE_IPS", "false"), false),
		AlertCleanupInterval:         parseDurationDefault(getenvDefault("ALERT_CLEANUP_INTERVAL", "1h"), time.Hour),
		AlertDeliveriesRetentionDays: parseIntDefault(getenvDefault("ALERT_DELIVERIES_RETENTION_DAYS", "0"), 0),
		AlertStatesRetentionDays:     parseIntDefault(getenvDefault("ALERT_STATES_RETENTION_DAYS", "0"), 0),

		SMTPHost:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:     parseIntDefault(getenvDefault("SMTP_PORT", "587"), 587),
		SMTPFrom:     strings.TrimSpace(os.Getenv("SMTP_FROM")),
		SMTPUsername: strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),

		SMSProvider: strings.TrimSpace(os.Getenv("SMS_PROVIDER")),

		AliyunSMSAccessKeyID:     strings.TrimSpace(os.Getenv("ALIYUN_SMS_ACCESS_KEY_ID")),
		AliyunSMSAccessKeySecret: strings.TrimSpace(os.Getenv("ALIYUN_SMS_ACCESS_KEY_SECRET")),
		AliyunSMSSignName:        strings.TrimSpace(os.Getenv("ALIYUN_SMS_SIGN_NAME")),
		AliyunSMSTemplateCode:    strings.TrimSpace(os.Getenv("ALIYUN_SMS_TEMPLATE_CODE")),
		AliyunSMSRegion:          strings.TrimSpace(os.Getenv("ALIYUN_SMS_REGION")),

		TencentSMSSecretID:   strings.TrimSpace(os.Getenv("TENCENT_SMS_SECRET_ID")),
		TencentSMSSecretKey:  strings.TrimSpace(os.Getenv("TENCENT_SMS_SECRET_KEY")),
		TencentSMSAppID:      strings.TrimSpace(os.Getenv("TENCENT_SMS_APP_ID")),
		TencentSMSSignName:   strings.TrimSpace(os.Getenv("TENCENT_SMS_SIGN_NAME")),
		TencentSMSTemplateID: strings.TrimSpace(os.Getenv("TENCENT_SMS_TEMPLATE_ID")),
		TencentSMSRegion:     strings.TrimSpace(os.Getenv("TENCENT_SMS_REGION")),
	}
	cfg.AuthTokenTTL = parseDurationDefault(getenvDefault("AUTH_TOKEN_TTL", "168h"), 168*time.Hour)

	cfg.RunConsumers = parseBoolDefault(getenvDefault("RUN_CONSUMERS", "true"), true)
	cfg.RunAlertWorker = parseBoolDefault(getenvDefault("RUN_ALERT_WORKER", "false"), false)
	cfg.EnableMetrics = parseBoolDefault(getenvDefault("ENABLE_METRICS", "true"), true) && cfg.RedisAddr != ""
	cfg.WebhookAllowlistCIDRs = parseCIDRPrefixesEnv(getenvDefault("WEBHOOK_ALLOWLIST_CIDRS", ""))
	if strings.TrimSpace(cfg.NSQDAddress) == "" {
		return Config{}, errors.New("NSQD_ADDRESS is required")
	}
	if cfg.NSQDHTTPAddress == "" {
		cfg.NSQDHTTPAddress = deriveNSQDHTTPAddress(cfg.NSQDAddress)
	}
	if cfg.RunConsumers && cfg.PostgresURL == "" {
		return Config{}, errors.New("POSTGRES_URL is required when RUN_CONSUMERS=true")
	}
	if cfg.RunAlertWorker && cfg.PostgresURL == "" {
		return Config{}, errors.New("POSTGRES_URL is required when RUN_ALERT_WORKER=true")
	}
	if cfg.NSQMaxInFlight <= 0 {
		cfg.NSQMaxInFlight = 200
	}
	if cfg.NSQEventConcurrency <= 0 {
		cfg.NSQEventConcurrency = 1
	}
	if cfg.NSQLogConcurrency <= 0 {
		cfg.NSQLogConcurrency = 1
	}
	if cfg.DBMaxOpenConns <= 0 {
		cfg.DBMaxOpenConns = 10
	}
	if cfg.DBMaxIdleConns < 0 {
		cfg.DBMaxIdleConns = 1
	}
	if cfg.DBMaxIdleConns > cfg.DBMaxOpenConns {
		cfg.DBMaxIdleConns = cfg.DBMaxOpenConns
	}
	if cfg.DBLogBatchSize <= 0 {
		cfg.DBLogBatchSize = 200
	}
	if cfg.DBEventBatchSize <= 0 {
		cfg.DBEventBatchSize = 200
	}
	if cfg.DBLogFlushInterval <= 0 {
		cfg.DBLogFlushInterval = 50 * time.Millisecond
	}
	if cfg.DBEventFlushInterval <= 0 {
		cfg.DBEventFlushInterval = 50 * time.Millisecond
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 10 * time.Minute
	}
	if cfg.CleanupPolicyLimit <= 0 {
		cfg.CleanupPolicyLimit = 50
	}
	if cfg.CleanupDeleteBatchSize <= 0 {
		cfg.CleanupDeleteBatchSize = 5000
	}
	if cfg.CleanupMaxBatches <= 0 {
		cfg.CleanupMaxBatches = 50
	}
	if cfg.CleanupBatchSleep < 0 {
		cfg.CleanupBatchSleep = 0
	}
	if cfg.AlertCleanupInterval <= 0 {
		cfg.AlertCleanupInterval = time.Hour
	}
	if cfg.MonitorTickInterval <= 0 {
		cfg.MonitorTickInterval = 2 * time.Second
	}
	if cfg.MonitorBatchSize <= 0 {
		cfg.MonitorBatchSize = 20
	}
	if cfg.MonitorLeaseDuration <= 0 {
		cfg.MonitorLeaseDuration = 60 * time.Second
	}
	switch cfg.StorageBackend {
	case StorageBackendPostgres, StorageBackendDual, StorageBackendClickHouse:
	default:
		return Config{}, fmt.Errorf("STORAGE_BACKEND must be one of %s|%s|%s, got %q", StorageBackendPostgres, StorageBackendDual, StorageBackendClickHouse, cfg.StorageBackend)
	}
	if cfg.StorageBackend != StorageBackendPostgres && cfg.ClickHouseDSN == "" {
		return Config{}, fmt.Errorf("CLICKHOUSE_DSN is required when STORAGE_BACKEND=%s", cfg.StorageBackend)
	}
	if cfg.ClickHouseReadDSN == "" {
		cfg.ClickHouseReadDSN = cfg.ClickHouseDSN
	}
	if cfg.CHDedupMode != CHDedupModeRedis && cfg.CHDedupMode != CHDedupModeNone {
		return Config{}, fmt.Errorf("CH_DEDUP_MODE must be %s or %s, got %q", CHDedupModeRedis, CHDedupModeNone, cfg.CHDedupMode)
	}
	if cfg.NSQMaxInFlightCH <= 0 {
		cfg.NSQMaxInFlightCH = 50000
	}
	if cfg.CHWriteShards <= 0 {
		cfg.CHWriteShards = 1
	}
	if cfg.CHLogBatchSize <= 0 {
		cfg.CHLogBatchSize = 5000
	}
	if cfg.CHEventBatchSize <= 0 {
		cfg.CHEventBatchSize = 2000
	}
	if cfg.CHLogConcurrency <= 0 {
		cfg.CHLogConcurrency = 1
	}
	if cfg.NSQDepthAlertThreshold <= 0 {
		cfg.NSQDepthAlertThreshold = 100000
	}
	if cfg.CHShortQueryMaxConcurrent <= 0 {
		cfg.CHShortQueryMaxConcurrent = 64
	}
	if cfg.CHHeavyQueryMaxConcurrent <= 0 {
		cfg.CHHeavyQueryMaxConcurrent = 8
	}
	if cfg.CHLogTTLDays <= 0 {
		cfg.CHLogTTLDays = 30
	}
	if cfg.QueryBackendPercent < 0 || cfg.QueryBackendPercent > 100 {
		return Config{}, fmt.Errorf("QUERY_BACKEND_PERCENT must be within [0,100], got %d", cfg.QueryBackendPercent)
	}
	return cfg, nil
}

// FromEnvAlertWorker loads a minimal config for the standalone alert worker binary.
// It does not require AUTH_SECRET, Redis or NSQ settings.
func FromEnvAlertWorker() (Config, error) {
	cfg := Config{
		PostgresURL:    strings.TrimSpace(os.Getenv("POSTGRES_URL")),
		DBMaxOpenConns: parseIntDefault(getenvDefault("DB_MAX_OPEN_CONNS", "25"), 25),
		DBMaxIdleConns: parseIntDefault(getenvDefault("DB_MAX_IDLE_CONNS", "5"), 5),

		WebhookAllowLoopback:         parseBoolDefault(getenvDefault("WEBHOOK_ALLOW_LOOPBACK", "false"), false),
		WebhookAllowPrivateIPs:       parseBoolDefault(getenvDefault("WEBHOOK_ALLOW_PRIVATE_IPS", "false"), false),
		WebhookAllowlistCIDRs:        parseCIDRPrefixesEnv(getenvDefault("WEBHOOK_ALLOWLIST_CIDRS", "")),
		AlertCleanupInterval:         parseDurationDefault(getenvDefault("ALERT_CLEANUP_INTERVAL", "1h"), time.Hour),
		AlertDeliveriesRetentionDays: parseIntDefault(getenvDefault("ALERT_DELIVERIES_RETENTION_DAYS", "0"), 0),
		AlertStatesRetentionDays:     parseIntDefault(getenvDefault("ALERT_STATES_RETENTION_DAYS", "0"), 0),

		SMTPHost:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:     parseIntDefault(getenvDefault("SMTP_PORT", "587"), 587),
		SMTPFrom:     strings.TrimSpace(os.Getenv("SMTP_FROM")),
		SMTPUsername: strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),

		SMSProvider: strings.TrimSpace(os.Getenv("SMS_PROVIDER")),

		AliyunSMSAccessKeyID:     strings.TrimSpace(os.Getenv("ALIYUN_SMS_ACCESS_KEY_ID")),
		AliyunSMSAccessKeySecret: strings.TrimSpace(os.Getenv("ALIYUN_SMS_ACCESS_KEY_SECRET")),
		AliyunSMSSignName:        strings.TrimSpace(os.Getenv("ALIYUN_SMS_SIGN_NAME")),
		AliyunSMSTemplateCode:    strings.TrimSpace(os.Getenv("ALIYUN_SMS_TEMPLATE_CODE")),
		AliyunSMSRegion:          strings.TrimSpace(os.Getenv("ALIYUN_SMS_REGION")),

		TencentSMSSecretID:   strings.TrimSpace(os.Getenv("TENCENT_SMS_SECRET_ID")),
		TencentSMSSecretKey:  strings.TrimSpace(os.Getenv("TENCENT_SMS_SECRET_KEY")),
		TencentSMSAppID:      strings.TrimSpace(os.Getenv("TENCENT_SMS_APP_ID")),
		TencentSMSSignName:   strings.TrimSpace(os.Getenv("TENCENT_SMS_SIGN_NAME")),
		TencentSMSTemplateID: strings.TrimSpace(os.Getenv("TENCENT_SMS_TEMPLATE_ID")),
		TencentSMSRegion:     strings.TrimSpace(os.Getenv("TENCENT_SMS_REGION")),
	}
	if cfg.PostgresURL == "" {
		return Config{}, errors.New("POSTGRES_URL is required")
	}
	if cfg.DBMaxOpenConns <= 0 {
		cfg.DBMaxOpenConns = 10
	}
	if cfg.DBMaxIdleConns < 0 {
		cfg.DBMaxIdleConns = 1
	}
	if cfg.DBMaxIdleConns > cfg.DBMaxOpenConns {
		cfg.DBMaxIdleConns = cfg.DBMaxOpenConns
	}
	if cfg.SMTPPort <= 0 {
		cfg.SMTPPort = 587
	}
	if cfg.AlertCleanupInterval <= 0 {
		cfg.AlertCleanupInterval = time.Hour
	}
	return cfg, nil
}

func parseCIDRPrefixesEnv(raw string) []netip.Prefix {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]netip.Prefix, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pr, err := netip.ParsePrefix(p)
		if err != nil {
			continue
		}
		out = append(out, pr)
	}
	return out
}

func parseStringListEnv(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

// parseIntListEnv parses a comma/semicolon/space separated list of positive
// integers, skipping unparsable or non-positive entries.
func parseIntListEnv(raw string) []int {
	parts := parseStringListEnv(raw)
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			continue
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func getenvDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func parseBoolDefault(value string, defaultValue bool) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return defaultValue
	}
	return parsed
}

func parseIntDefault(value string, defaultValue int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return defaultValue
	}
	return parsed
}

func parseDurationDefault(value string, defaultValue time.Duration) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

func decodeBase64Any(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty")
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}

func (c Config) String() string {
	return fmt.Sprintf(
		"http=%s nsqd=%s nsqd_http=%s consumers=%v pg=%s redis=%s metrics=%v geoip=%v auth=%v maintenance=%v channels(events=%s logs=%s) nsq(max_in_flight=%d event_cc=%d log_cc=%d) db(max_open=%d max_idle=%d log_batch=%d/%s event_batch=%d/%s) cleanup(interval=%s limit=%d batch=%d max_batches=%d sleep=%s)",
		c.HTTPAddr,
		c.NSQDAddress,
		c.NSQDHTTPAddress,
		c.RunConsumers,
		redactPostgresURL(c.PostgresURL),
		redactRedis(c.RedisAddr),
		c.EnableMetrics,
		c.GeoIPCityMMDB != "" || c.GeoIPASNMMDB != "",
		len(c.AuthSecret) > 0,
		c.MaintenanceMode,
		c.NSQEventChannel,
		c.NSQLogChannel,
		c.NSQMaxInFlight,
		c.NSQEventConcurrency,
		c.NSQLogConcurrency,
		c.DBMaxOpenConns,
		c.DBMaxIdleConns,
		c.DBLogBatchSize,
		c.DBLogFlushInterval,
		c.DBEventBatchSize,
		c.DBEventFlushInterval,
		c.CleanupInterval,
		c.CleanupPolicyLimit,
		c.CleanupDeleteBatchSize,
		c.CleanupMaxBatches,
		c.CleanupBatchSleep,
	)
}

func deriveNSQDHTTPAddress(tcpAddr string) string {
	tcpAddr = strings.TrimSpace(tcpAddr)
	if tcpAddr == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(tcpAddr)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return ""
	}
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 || p >= 65535 {
		return ""
	}
	return fmt.Sprintf("%s:%d", host, p+1)
}

func redactPostgresURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "<none>"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "<set>"
	}
	user := ""
	if u.User != nil {
		user = u.User.Username()
	}
	host := u.Host
	db := strings.TrimPrefix(u.Path, "/")
	if user == "" && host == "" && db == "" {
		return "<set>"
	}
	if user == "" {
		user = "?"
	}
	if host == "" {
		host = "?"
	}
	if db == "" {
		db = "?"
	}
	return fmt.Sprintf("%s@%s/%s", user, host, db)
}

func redactRedis(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "<none>"
	}
	return addr
}
