package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	NSQDAddress     string
	PostgresURL     string
	RunConsumers    bool
	NSQEventChannel string
	NSQLogChannel   string
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	EnableMetrics   bool
	GeoIPCityMMDB   string
	GeoIPASNMMDB    string
	AuthSecret      []byte
	AuthTokenTTL    time.Duration
	MaintenanceMode bool
}

func FromEnv() (Config, error) {
	authSecretRaw := strings.TrimSpace(os.Getenv("AUTH_SECRET"))
	var authSecret []byte
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

	cfg := Config{
		HTTPAddr:        getenvDefault("HTTP_ADDR", ":8080"),
		NSQDAddress:     getenvDefault("NSQD_ADDRESS", "127.0.0.1:4150"),
		PostgresURL:     strings.TrimSpace(os.Getenv("POSTGRES_URL")),
		NSQEventChannel: getenvDefault("NSQ_EVENT_CHANNEL", "event-consumer"),
		NSQLogChannel:   getenvDefault("NSQ_LOG_CHANNEL", "log-consumer"),
		RedisAddr:       strings.TrimSpace(os.Getenv("REDIS_ADDR")),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		RedisDB:         parseIntDefault(getenvDefault("REDIS_DB", "0"), 0),
		GeoIPCityMMDB:   strings.TrimSpace(os.Getenv("GEOIP_CITY_MMDB")),
		GeoIPASNMMDB:    strings.TrimSpace(os.Getenv("GEOIP_ASN_MMDB")),
		AuthSecret:      authSecret,
		MaintenanceMode: parseBoolDefault(getenvDefault("MAINTENANCE_MODE", "false"), false),
	}
	cfg.AuthTokenTTL = parseDurationDefault(getenvDefault("AUTH_TOKEN_TTL", "168h"), 168*time.Hour)

	cfg.RunConsumers = parseBoolDefault(getenvDefault("RUN_CONSUMERS", "true"), true)
	cfg.EnableMetrics = parseBoolDefault(getenvDefault("ENABLE_METRICS", "true"), true) && cfg.RedisAddr != ""
	if strings.TrimSpace(cfg.NSQDAddress) == "" {
		return Config{}, errors.New("NSQD_ADDRESS is required")
	}
	if cfg.RunConsumers && cfg.PostgresURL == "" {
		return Config{}, errors.New("POSTGRES_URL is required when RUN_CONSUMERS=true")
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
		"http=%s nsqd=%s consumers=%v pg=%s redis=%s metrics=%v geoip=%v auth=%v maintenance=%v channels(events=%s logs=%s)",
		c.HTTPAddr,
		c.NSQDAddress,
		c.RunConsumers,
		redactPostgresURL(c.PostgresURL),
		redactRedis(c.RedisAddr),
		c.EnableMetrics,
		c.GeoIPCityMMDB != "" || c.GeoIPASNMMDB != "",
		len(c.AuthSecret) > 0,
		c.MaintenanceMode,
		c.NSQEventChannel,
		c.NSQLogChannel,
	)
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
