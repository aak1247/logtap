package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr               string
	NSQDAddress            string
	NSQDHTTPAddress        string
	PostgresURL            string
	RunConsumers           bool
	NSQEventChannel        string
	NSQLogChannel          string
	NSQMaxInFlight         int
	NSQEventConcurrency    int
	NSQLogConcurrency      int
	DBMaxOpenConns         int
	DBMaxIdleConns         int
	DBLogBatchSize         int
	DBLogFlushInterval     time.Duration
	DBEventBatchSize       int
	DBEventFlushInterval   time.Duration
	CleanupInterval        time.Duration
	CleanupPolicyLimit     int
	CleanupDeleteBatchSize int
	CleanupMaxBatches      int
	CleanupBatchSleep      time.Duration
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	EnableMetrics          bool
	MetricsDayTTL          time.Duration
	MetricsDistTTL         time.Duration
	MetricsMonthTTL        time.Duration
	GeoIPCityMMDB          string
	GeoIPASNMMDB           string
	AuthSecret             []byte
	AuthTokenTTL           time.Duration
	MaintenanceMode        bool
	LogtapProxySecret      string
	EnableDebugEndpoints   bool
	DBRequireTimescale     bool

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
		HTTPAddr:               getenvDefault("HTTP_ADDR", ":8080"),
		NSQDAddress:            getenvDefault("NSQD_ADDRESS", "127.0.0.1:4150"),
		NSQDHTTPAddress:        strings.TrimSpace(os.Getenv("NSQD_HTTP_ADDRESS")),
		PostgresURL:            strings.TrimSpace(os.Getenv("POSTGRES_URL")),
		NSQEventChannel:        getenvDefault("NSQ_EVENT_CHANNEL", "event-consumer"),
		NSQLogChannel:          getenvDefault("NSQ_LOG_CHANNEL", "log-consumer"),
		NSQMaxInFlight:         parseIntDefault(getenvDefault("NSQ_MAX_IN_FLIGHT", "200"), 200),
		NSQEventConcurrency:    parseIntDefault(getenvDefault("NSQ_EVENT_CONCURRENCY", "1"), 1),
		NSQLogConcurrency:      parseIntDefault(getenvDefault("NSQ_LOG_CONCURRENCY", "1"), 1),
		DBMaxOpenConns:         parseIntDefault(getenvDefault("DB_MAX_OPEN_CONNS", "10"), 10),
		DBMaxIdleConns:         parseIntDefault(getenvDefault("DB_MAX_IDLE_CONNS", "1"), 1),
		DBLogBatchSize:         parseIntDefault(getenvDefault("DB_LOG_BATCH_SIZE", "200"), 200),
		DBLogFlushInterval:     parseDurationDefault(getenvDefault("DB_LOG_FLUSH_INTERVAL", "50ms"), 50*time.Millisecond),
		DBEventBatchSize:       parseIntDefault(getenvDefault("DB_EVENT_BATCH_SIZE", "200"), 200),
		DBEventFlushInterval:   parseDurationDefault(getenvDefault("DB_EVENT_FLUSH_INTERVAL", "50ms"), 50*time.Millisecond),
		CleanupInterval:        parseDurationDefault(getenvDefault("CLEANUP_INTERVAL", "10m"), 10*time.Minute),
		CleanupPolicyLimit:     parseIntDefault(getenvDefault("CLEANUP_POLICY_LIMIT", "50"), 50),
		CleanupDeleteBatchSize: parseIntDefault(getenvDefault("CLEANUP_DELETE_BATCH_SIZE", "5000"), 5000),
		CleanupMaxBatches:      parseIntDefault(getenvDefault("CLEANUP_MAX_BATCHES", "50"), 50),
		CleanupBatchSleep:      parseDurationDefault(getenvDefault("CLEANUP_BATCH_SLEEP", "0s"), 0),
		RedisAddr:              strings.TrimSpace(os.Getenv("REDIS_ADDR")),
		RedisPassword:          os.Getenv("REDIS_PASSWORD"),
		RedisDB:                parseIntDefault(getenvDefault("REDIS_DB", "0"), 0),
		MetricsDayTTL:          parseDurationDefault(getenvDefault("METRICS_DAY_TTL", "4320h"), 180*24*time.Hour),
		MetricsDistTTL:         parseDurationDefault(getenvDefault("METRICS_DIST_TTL", "2160h"), 90*24*time.Hour),
		MetricsMonthTTL:        parseDurationDefault(getenvDefault("METRICS_MONTH_TTL", "13392h"), 18*31*24*time.Hour),
		GeoIPCityMMDB:          strings.TrimSpace(os.Getenv("GEOIP_CITY_MMDB")),
		GeoIPASNMMDB:           strings.TrimSpace(os.Getenv("GEOIP_ASN_MMDB")),
		AuthSecret:             authSecret,
		MaintenanceMode:        parseBoolDefault(getenvDefault("MAINTENANCE_MODE", "false"), false),
		LogtapProxySecret:      strings.TrimSpace(os.Getenv("LOGTAP_PROXY_SECRET")),
		EnableDebugEndpoints:   parseBoolDefault(getenvDefault("ENABLE_DEBUG_ENDPOINTS", "false"), false),
		DBRequireTimescale:     parseBoolDefault(getenvDefault("DB_REQUIRE_TIMESCALE", "false"), false),

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
	cfg.EnableMetrics = parseBoolDefault(getenvDefault("ENABLE_METRICS", "true"), true) && cfg.RedisAddr != ""
	if strings.TrimSpace(cfg.NSQDAddress) == "" {
		return Config{}, errors.New("NSQD_ADDRESS is required")
	}
	if cfg.NSQDHTTPAddress == "" {
		cfg.NSQDHTTPAddress = deriveNSQDHTTPAddress(cfg.NSQDAddress)
	}
	if cfg.RunConsumers && cfg.PostgresURL == "" {
		return Config{}, errors.New("POSTGRES_URL is required when RUN_CONSUMERS=true")
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
	return cfg, nil
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
