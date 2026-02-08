package model

import (
	"time"

	"gorm.io/datatypes"
)

type AlertContact struct {
	ID        int       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ProjectID int       `gorm:"not null;index;uniqueIndex:idx_alert_contacts_project_type_value,priority:1;column:project_id" json:"project_id"`
	Type      string    `gorm:"type:varchar(16);not null;index;uniqueIndex:idx_alert_contacts_project_type_value,priority:2;column:type" json:"type"`
	Name      string    `gorm:"type:varchar(200);not null;default:'';column:name" json:"name"`
	Value     string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_alert_contacts_project_type_value,priority:3;column:value" json:"value"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (AlertContact) TableName() string { return "alert_contacts" }

type AlertContactGroup struct {
	ID        int       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ProjectID int       `gorm:"not null;index;uniqueIndex:idx_alert_contact_groups_project_type_name,priority:1;column:project_id" json:"project_id"`
	Type      string    `gorm:"type:varchar(16);not null;index;uniqueIndex:idx_alert_contact_groups_project_type_name,priority:2;column:type" json:"type"`
	Name      string    `gorm:"type:varchar(200);not null;uniqueIndex:idx_alert_contact_groups_project_type_name,priority:3;column:name" json:"name"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (AlertContactGroup) TableName() string { return "alert_contact_groups" }

type AlertContactGroupMember struct {
	ID        int       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	GroupID   int       `gorm:"not null;index:idx_alert_group_members_group_contact,priority:1;column:group_id" json:"group_id"`
	ContactID int       `gorm:"not null;index:idx_alert_group_members_group_contact,priority:2;column:contact_id" json:"contact_id"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
}

func (AlertContactGroupMember) TableName() string { return "alert_contact_group_members" }

type AlertWecomBot struct {
	ID         int       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ProjectID  int       `gorm:"not null;index;uniqueIndex:idx_alert_wecom_bots_project_webhook,priority:1;column:project_id" json:"project_id"`
	Name       string    `gorm:"type:varchar(200);not null;column:name" json:"name"`
	WebhookURL string    `gorm:"type:text;not null;uniqueIndex:idx_alert_wecom_bots_project_webhook,priority:2;column:webhook_url" json:"webhook_url"`
	CreatedAt  time.Time `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt  time.Time `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (AlertWecomBot) TableName() string { return "alert_wecom_bots" }

type AlertWebhookEndpoint struct {
	ID        int       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ProjectID int       `gorm:"not null;index;uniqueIndex:idx_alert_webhook_endpoints_project_url,priority:1;column:project_id" json:"project_id"`
	Name      string    `gorm:"type:varchar(200);not null;column:name" json:"name"`
	URL       string    `gorm:"type:text;not null;uniqueIndex:idx_alert_webhook_endpoints_project_url,priority:2;column:url" json:"url"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (AlertWebhookEndpoint) TableName() string { return "alert_webhook_endpoints" }

type AlertRule struct {
	ID        int            `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ProjectID int            `gorm:"not null;index;column:project_id" json:"project_id"`
	Name      string         `gorm:"type:varchar(200);not null;column:name" json:"name"`
	Enabled   bool           `gorm:"not null;default:true;index;column:enabled" json:"enabled"`
	Source    string         `gorm:"type:varchar(16);not null;index;column:source" json:"source"`
	Match     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:match" json:"match"`
	Repeat    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:repeat" json:"repeat"`
	Targets   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:targets" json:"targets"`
	CreatedAt time.Time      `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (AlertRule) TableName() string { return "alert_rules" }

type AlertState struct {
	ID            int       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	RuleID        int       `gorm:"not null;index:idx_alert_states_rule_key,priority:1;column:rule_id" json:"rule_id"`
	KeyHash       string    `gorm:"type:varchar(64);not null;index:idx_alert_states_rule_key,priority:2;column:key_hash" json:"key_hash"`
	Occurrences   int       `gorm:"not null;default:0;column:occurrences" json:"occurrences"`
	BackoffExp    int       `gorm:"not null;default:0;column:backoff_exp" json:"backoff_exp"`
	LastSeenAt    time.Time `gorm:"not null;column:last_seen_at" json:"last_seen_at"`
	LastSentAt    time.Time `gorm:"not null;column:last_sent_at" json:"last_sent_at"`
	NextAllowedAt time.Time `gorm:"not null;column:next_allowed_at" json:"next_allowed_at"`
	CreatedAt     time.Time `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt     time.Time `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (AlertState) TableName() string { return "alert_states" }

type AlertDelivery struct {
	ID            int       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ProjectID     int       `gorm:"not null;index;column:project_id" json:"project_id"`
	RuleID        int       `gorm:"not null;index;column:rule_id" json:"rule_id"`
	ChannelType   string    `gorm:"type:varchar(16);not null;index;column:channel_type" json:"channel_type"`
	Target        string    `gorm:"type:text;not null;column:target" json:"target"`
	Title         string    `gorm:"type:text;not null;column:title" json:"title"`
	Content       string    `gorm:"type:text;not null;column:content" json:"content"`
	Status        string    `gorm:"type:varchar(16);not null;index;column:status" json:"status"`
	Attempts      int       `gorm:"not null;default:0;column:attempts" json:"attempts"`
	NextAttemptAt time.Time `gorm:"not null;index;column:next_attempt_at" json:"next_attempt_at"`
	LastError     string    `gorm:"type:text;not null;default:'';column:last_error" json:"last_error"`
	CreatedAt     time.Time `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt     time.Time `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (AlertDelivery) TableName() string { return "alert_deliveries" }
