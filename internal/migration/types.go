package migration

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const BundleSchemaVersion = "local-logtap-v1"

type Bundle struct {
	SchemaVersion              string          `json:"schema_version"`
	Source                     string          `json:"source"`
	ExportedAt                 time.Time       `json:"exported_at"`
	IncludeNotificationSecrets bool            `json:"include_notification_secrets"`
	Projects                   []ProjectBundle `json:"projects"`
}

type ProjectBundle struct {
	SourceProjectID int           `json:"source_project_id"`
	Name            string        `json:"name"`
	ExportedAt      time.Time     `json:"exported_at"`
	Counts          ProjectCounts `json:"counts"`
	Config          ProjectConfig `json:"config"`
	Logs            []LogRecord   `json:"logs"`
	Events          []EventRecord `json:"events"`
}

type ProjectCounts struct {
	Logs   int64 `json:"logs"`
	Events int64 `json:"events"`
}

type ProjectConfig struct {
	CleanupPolicy            *CleanupPolicyRecord            `json:"cleanup_policy,omitempty"`
	EventDefinitions         []EventDefinitionRecord         `json:"event_definitions,omitempty"`
	PropertyDefinitions      []PropertyDefinitionRecord      `json:"property_definitions,omitempty"`
	AnalysisViews            []AnalysisViewRecord            `json:"analysis_views,omitempty"`
	PluginPackageSettings    []PluginPackageSettingRecord    `json:"plugin_package_settings,omitempty"`
	AlertContacts            []AlertContactRecord            `json:"alert_contacts,omitempty"`
	AlertContactGroups       []AlertContactGroupRecord       `json:"alert_contact_groups,omitempty"`
	AlertContactGroupMembers []AlertContactGroupMemberRecord `json:"alert_contact_group_members,omitempty"`
	AlertWecomBots           []AlertWecomBotRecord           `json:"alert_wecom_bots,omitempty"`
	AlertWebhookEndpoints    []AlertWebhookEndpointRecord    `json:"alert_webhook_endpoints,omitempty"`
	AlertRules               []AlertRuleRecord               `json:"alert_rules,omitempty"`
	MonitorDefinitions       []MonitorDefinitionRecord       `json:"monitor_definitions,omitempty"`
}

type CleanupPolicyRecord struct {
	Enabled                  bool       `json:"enabled"`
	LogsRetentionDays        int        `json:"logs_retention_days"`
	EventsRetentionDays      int        `json:"events_retention_days"`
	TrackEventsRetentionDays int        `json:"track_events_retention_days"`
	ScheduleHourUTC          int        `json:"schedule_hour_utc"`
	ScheduleMinuteUTC        int        `json:"schedule_minute_utc"`
	LastRunAt                *time.Time `json:"last_run_at,omitempty"`
	NextRunAt                *time.Time `json:"next_run_at,omitempty"`
}

type EventDefinitionRecord struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Owner       string    `json:"owner"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PropertyDefinitionRecord struct {
	ID            int             `json:"id"`
	Key           string          `json:"key"`
	DisplayName   string          `json:"display_name"`
	Type          string          `json:"type"`
	Description   string          `json:"description"`
	Status        string          `json:"status"`
	EnumValues    json.RawMessage `json:"enum_values,omitempty"`
	ExampleValues json.RawMessage `json:"example_values,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type AnalysisViewRecord struct {
	ID           int             `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	AnalysisType string          `json:"analysis_type"`
	Query        json.RawMessage `json:"query"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type PluginPackageSettingRecord struct {
	ID        int64           `json:"id"`
	PackageID string          `json:"package_id"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type AlertContactRecord struct {
	ID        int       `json:"id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AlertContactGroupRecord struct {
	ID        int       `json:"id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AlertContactGroupMemberRecord struct {
	ID        int       `json:"id"`
	GroupID   int       `json:"group_id"`
	ContactID int       `json:"contact_id"`
	CreatedAt time.Time `json:"created_at"`
}

type AlertWecomBotRecord struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	WebhookURL string    `json:"webhook_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type AlertWebhookEndpointRecord struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AlertRuleRecord struct {
	ID        int             `json:"id"`
	Name      string          `json:"name"`
	Enabled   bool            `json:"enabled"`
	Source    string          `json:"source"`
	Match     json.RawMessage `json:"match"`
	Repeat    json.RawMessage `json:"repeat"`
	Targets   json.RawMessage `json:"targets"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type MonitorDefinitionRecord struct {
	ID           int             `json:"id"`
	Name         string          `json:"name"`
	DetectorType string          `json:"detector_type"`
	Config       json.RawMessage `json:"config"`
	IntervalSec  int             `json:"interval_sec"`
	TimeoutMS    int             `json:"timeout_ms"`
	Enabled      bool            `json:"enabled"`
	NextRunAt    time.Time       `json:"next_run_at"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type LogRecord struct {
	SourceID   int64           `json:"source_id"`
	Timestamp  time.Time       `json:"timestamp"`
	IngestID   *uuid.UUID      `json:"ingest_id,omitempty"`
	Level      string          `json:"level"`
	DistinctID string          `json:"distinct_id,omitempty"`
	DeviceID   string          `json:"device_id,omitempty"`
	TraceID    string          `json:"trace_id,omitempty"`
	SpanID     string          `json:"span_id,omitempty"`
	Message    string          `json:"message"`
	Fields     json.RawMessage `json:"fields"`
}

type EventRecord struct {
	ID          uuid.UUID       `json:"id"`
	Timestamp   time.Time       `json:"timestamp"`
	Level       string          `json:"level"`
	DistinctID  string          `json:"distinct_id,omitempty"`
	DeviceID    string          `json:"device_id,omitempty"`
	OS          string          `json:"os,omitempty"`
	Platform    string          `json:"platform,omitempty"`
	ReleaseTag  string          `json:"release_tag,omitempty"`
	Environment string          `json:"environment,omitempty"`
	UserID      string          `json:"user_id,omitempty"`
	Title       string          `json:"title,omitempty"`
	Data        json.RawMessage `json:"data"`
}

func NewBundle(includeNotificationSecrets bool) Bundle {
	return Bundle{
		SchemaVersion:              BundleSchemaVersion,
		Source:                     "local_logtap",
		ExportedAt:                 time.Now().UTC(),
		IncludeNotificationSecrets: includeNotificationSecrets,
	}
}
