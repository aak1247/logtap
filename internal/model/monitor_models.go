package model

import (
	"time"

	"gorm.io/datatypes"
)

type MonitorDefinition struct {
	ID           int            `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ProjectID    int            `gorm:"not null;index:idx_monitor_project_enabled_next,priority:1;index;column:project_id" json:"project_id"`
	Name         string         `gorm:"type:varchar(200);not null;column:name" json:"name"`
	DetectorType string         `gorm:"type:varchar(64);not null;index;column:detector_type" json:"detector_type"`
	Config       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:config" json:"config"`
	IntervalSec  int            `gorm:"not null;default:60;column:interval_sec" json:"interval_sec"`
	TimeoutMS    int            `gorm:"not null;default:5000;column:timeout_ms" json:"timeout_ms"`
	Enabled      bool           `gorm:"not null;default:true;index:idx_monitor_project_enabled_next,priority:2;column:enabled" json:"enabled"`
	NextRunAt    time.Time      `gorm:"not null;index:idx_monitor_project_enabled_next,priority:3;index;column:next_run_at" json:"next_run_at"`
	LeaseOwner   string         `gorm:"type:varchar(128);not null;default:'';column:lease_owner" json:"lease_owner"`
	LeaseUntil   *time.Time     `gorm:"index;column:lease_until" json:"lease_until,omitempty"`
	CreatedAt    time.Time      `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (MonitorDefinition) TableName() string { return "monitor_definitions" }

type MonitorRun struct {
	ID          int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	MonitorID   int            `gorm:"not null;index:idx_monitor_runs_monitor_started,priority:1;column:monitor_id" json:"monitor_id"`
	ProjectID   int            `gorm:"not null;index;column:project_id" json:"project_id"`
	StartedAt   time.Time      `gorm:"not null;index:idx_monitor_runs_monitor_started,priority:2,sort:desc;column:started_at" json:"started_at"`
	FinishedAt  time.Time      `gorm:"not null;column:finished_at" json:"finished_at"`
	Status      string         `gorm:"type:varchar(16);not null;index;column:status" json:"status"`
	SignalCount int            `gorm:"not null;default:0;column:signal_count" json:"signal_count"`
	Error       string         `gorm:"type:text;not null;default:'';column:error" json:"error"`
	Result      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:result" json:"result"`
	CreatedAt   time.Time      `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
}

func (MonitorRun) TableName() string { return "monitor_runs" }
