package alert

import "time"

type Source string

const (
	SourceLogs   Source = "logs"
	SourceEvents Source = "events"
	SourceBoth   Source = "both"
)

type MatchOp string

const (
	OpEquals   MatchOp = "eq"
	OpContains MatchOp = "contains"
	OpExists   MatchOp = "exists"
	OpIn       MatchOp = "in"
)

type FieldMatch struct {
	Path   string  `json:"path"`
	Op     MatchOp `json:"op"`
	Value  any     `json:"value,omitempty"`
	Values []any   `json:"values,omitempty"` // for "in"
}

// RuleMatch describes the triggering condition.
type RuleMatch struct {
	Levels          []string     `json:"levels,omitempty"`          // for logs: debug/info/warn/error/fatal/event; for events: error/fatal...
	EventNames      []string     `json:"eventNames,omitempty"`      // for track: log.level=event + log.message in names
	MessageKeywords []string     `json:"messageKeywords,omitempty"` // substring match on message/title
	FieldsAll       []FieldMatch `json:"fieldsAll,omitempty"`       // all must match
}

// RuleRepeat describes dedupe/backoff behavior.
type RuleRepeat struct {
	WindowSec       int      `json:"windowSec,omitempty"`       // rolling window to count repeats
	Threshold       int      `json:"threshold,omitempty"`       // only alert once repeats >= threshold
	BaseBackoffSec  int      `json:"baseBackoffSec,omitempty"`  // initial backoff after alert
	MaxBackoffSec   int      `json:"maxBackoffSec,omitempty"`   // cap
	DedupeByMessage *bool    `json:"dedupeByMessage,omitempty"` // default true when omitted
	DedupeFields    []string `json:"dedupeFields,omitempty"`    // JSON paths to include in dedupe key
}

type RuleTargets struct {
	EmailGroupIDs      []int `json:"emailGroupIds,omitempty"`
	EmailContactIDs    []int `json:"emailContactIds,omitempty"`
	SMSGroupIDs        []int `json:"smsGroupIds,omitempty"`
	SMSContactIDs      []int `json:"smsContactIds,omitempty"`
	WecomBotIDs        []int `json:"wecomBotIds,omitempty"`
	WebhookEndpointIDs []int `json:"webhookEndpointIds,omitempty"`
}

type Input struct {
	ProjectID int
	Source    Source
	Timestamp time.Time
	Level     string
	Message   string
	Fields    map[string]any
}
