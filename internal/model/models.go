package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type User struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id"`
	Email        string    `gorm:"type:varchar(255);not null;uniqueIndex;column:email"`
	PasswordHash string    `gorm:"type:text;not null;column:password_hash"`
	CreatedAt    time.Time `gorm:"not null;autoCreateTime;column:created_at"`
}

func (User) TableName() string { return "users" }

type Project struct {
	ID          int       `gorm:"primaryKey;autoIncrement;column:id"`
	OwnerUserID int64     `gorm:"not null;index;column:owner_user_id"`
	Name        string    `gorm:"type:varchar(200);not null;column:name"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime;column:created_at"`
}

func (Project) TableName() string { return "projects" }

type ProjectKey struct {
	ID        int        `gorm:"primaryKey;autoIncrement;column:id"`
	ProjectID int        `gorm:"not null;index;column:project_id"`
	Name      string     `gorm:"type:varchar(200);not null;column:name"`
	Key       string     `gorm:"type:varchar(80);not null;uniqueIndex;column:key"`
	CreatedAt time.Time  `gorm:"not null;autoCreateTime;column:created_at"`
	RevokedAt *time.Time `gorm:"index;column:revoked_at"`
}

func (ProjectKey) TableName() string { return "project_keys" }

type Event struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;column:id"`
	ProjectID   int            `gorm:"not null;index:idx_events_project_ts,priority:1;column:project_id"`
	Timestamp   time.Time      `gorm:"not null;index:idx_events_project_ts,priority:2,sort:desc;column:timestamp"`
	Level       string         `gorm:"type:varchar(20);column:level"`
	DistinctID  string         `gorm:"type:varchar(255);index;column:distinct_id"`
	DeviceID    string         `gorm:"type:varchar(255);index;column:device_id"`
	OS          string         `gorm:"type:varchar(100);index;column:os"`
	Platform    string         `gorm:"type:varchar(50);column:platform"`
	ReleaseTag  string         `gorm:"type:varchar(100);column:release_tag"`
	Environment string         `gorm:"type:varchar(50);column:environment"`
	UserID      string         `gorm:"type:varchar(255);column:user_id"`
	Title       string         `gorm:"type:text;column:title"`
	Data        datatypes.JSON `gorm:"type:jsonb;not null;column:data"`
}

func (Event) TableName() string { return "events" }

type Log struct {
	ID         int64          `gorm:"primaryKey;autoIncrement;column:id"`
	ProjectID  int            `gorm:"not null;index:idx_logs_project_ts,priority:1;index:idx_logs_dedupe,unique,priority:1;column:project_id"`
	Timestamp  time.Time      `gorm:"not null;index:idx_logs_project_ts,priority:2,sort:desc;column:timestamp"`
	IngestID   *uuid.UUID     `gorm:"type:uuid;index:idx_logs_dedupe,unique,priority:2;column:ingest_id"`
	Level      string         `gorm:"type:varchar(20);column:level"`
	DistinctID string         `gorm:"type:varchar(255);index;column:distinct_id"`
	DeviceID   string         `gorm:"type:varchar(255);index;column:device_id"`
	TraceID    string         `gorm:"type:varchar(64);column:trace_id"`
	SpanID     string         `gorm:"type:varchar(64);column:span_id"`
	Message    string         `gorm:"type:text;not null;column:message"`
	Fields     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:fields"`
	// search_vector is created via DDL during migration for full-text search.
}

func (Log) TableName() string { return "logs" }

// TrackEvent is a denormalized table for analytics (funnel/top events).
// It stores track events derived from logs where level='event'.
type TrackEvent struct {
	ID         int64      `gorm:"primaryKey;autoIncrement;column:id"`
	ProjectID  int        `gorm:"not null;index:idx_track_events_project_ts,priority:1;index:idx_track_events_dedupe,unique,priority:1;index:idx_track_events_project_name_ts,priority:1;index:idx_track_events_project_user_ts,priority:1;column:project_id"`
	Timestamp  time.Time  `gorm:"not null;index:idx_track_events_project_ts,priority:2,sort:desc;index:idx_track_events_project_name_ts,priority:3,sort:desc;index:idx_track_events_project_user_ts,priority:3,sort:desc;column:timestamp"`
	IngestID   *uuid.UUID `gorm:"type:uuid;index:idx_track_events_dedupe,unique,priority:2;column:ingest_id"`
	Name       string     `gorm:"type:text;not null;index:idx_track_events_project_name_ts,priority:2;column:name"`
	DistinctID string     `gorm:"type:varchar(255);not null;index:idx_track_events_project_user_ts,priority:2;column:distinct_id"`
	DeviceID   string     `gorm:"type:varchar(255);column:device_id"`
}

func (TrackEvent) TableName() string { return "track_events" }

// TrackEventDaily is a rollup table used to accelerate event analytics queries.
// One row represents a (project_id, day, name, distinct_id) bucket with an event counter.
type TrackEventDaily struct {
	ProjectID  int       `gorm:"not null;uniqueIndex:idx_track_event_daily_key,priority:1;index:idx_track_event_daily_project_day,priority:1;column:project_id"`
	Day        string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_track_event_daily_key,priority:2;index:idx_track_event_daily_project_day,priority:2;column:day"`
	Name       string    `gorm:"type:text;not null;uniqueIndex:idx_track_event_daily_key,priority:3;index:idx_track_event_daily_project_name_day,priority:2;column:name"`
	DistinctID string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_track_event_daily_key,priority:4;index:idx_track_event_daily_project_user_day,priority:2;column:distinct_id"`
	Events     int64     `gorm:"not null;default:0;column:events"`
	CreatedAt  time.Time `gorm:"not null;autoCreateTime;column:created_at"`
	UpdatedAt  time.Time `gorm:"not null;autoUpdateTime;column:updated_at"`
}

func (TrackEventDaily) TableName() string { return "track_event_daily" }

type CleanupPolicy struct {
	ProjectID                int        `gorm:"primaryKey;column:project_id" json:"project_id"`
	Enabled                  bool       `gorm:"not null;default:false;column:enabled" json:"enabled"`
	LogsRetentionDays        int        `gorm:"not null;default:30;column:logs_retention_days" json:"logs_retention_days"`
	EventsRetentionDays      int        `gorm:"not null;default:30;column:events_retention_days" json:"events_retention_days"`
	TrackEventsRetentionDays int        `gorm:"not null;default:0;column:track_events_retention_days" json:"track_events_retention_days"`
	ScheduleHourUTC          int        `gorm:"not null;default:3;column:schedule_hour_utc" json:"schedule_hour_utc"`
	ScheduleMinuteUTC        int        `gorm:"not null;default:0;column:schedule_minute_utc" json:"schedule_minute_utc"`
	LastRunAt                *time.Time `gorm:"column:last_run_at" json:"last_run_at,omitempty"`
	NextRunAt                *time.Time `gorm:"index;column:next_run_at" json:"next_run_at,omitempty"`
	CreatedAt                time.Time  `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt                time.Time  `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (CleanupPolicy) TableName() string { return "cleanup_policies" }
