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
	ID        int64          `gorm:"primaryKey;autoIncrement;column:id"`
	ProjectID int            `gorm:"not null;index:idx_logs_project_ts,priority:1;column:project_id"`
	Timestamp time.Time      `gorm:"not null;index:idx_logs_project_ts,priority:2,sort:desc;column:timestamp"`
	Level     string         `gorm:"type:varchar(20);column:level"`
	DistinctID string        `gorm:"type:varchar(255);index;column:distinct_id"`
	DeviceID  string         `gorm:"type:varchar(255);index;column:device_id"`
	TraceID   string         `gorm:"type:varchar(64);column:trace_id"`
	SpanID    string         `gorm:"type:varchar(64);column:span_id"`
	Message   string         `gorm:"type:text;not null;column:message"`
	Fields    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:fields"`
	// search_vector is created via DDL during migration for full-text search.
}

func (Log) TableName() string { return "logs" }
