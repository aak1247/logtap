package metricthreshold

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

type Plugin struct{}

func New() Plugin { return Plugin{} }

func (Plugin) Type() string { return "metric_threshold" }

func (Plugin) ConfigSchema() json.RawMessage {
	const schema = `{
  "type": "object",
  "properties": {
    "field": {"type": "string"},
    "op": {"type": "string", "enum": [">", "<", ">=", "<=", "between"]},
    "value": {"type": "number"},
    "min": {"type": "number"},
    "max": {"type": "number"},
    "severityOnViolation": {"type": "string"}
  },
  "required": ["field", "op"],
  "additionalProperties": true
}`
	return json.RawMessage(schema)
}

type metricThresholdConfig struct {
	Field               string   `json:"field"`
	Op                  string   `json:"op"`
	Value               *float64 `json:"value,omitempty"`
	Min                 *float64 `json:"min,omitempty"`
	Max                 *float64 `json:"max,omitempty"`
	SeverityOnViolation string   `json:"severityOnViolation"`
}

func (Plugin) ValidateConfig(cfg json.RawMessage) error {
	raw := strings.TrimSpace(string(cfg))
	if raw == "" || raw == "null" {
		return errors.New("field and op are required")
	}
	var c metricThresholdConfig
	if err := json.Unmarshal(cfg, &c); err != nil {
		return errors.New("config must be valid json object")
	}
	if strings.TrimSpace(c.Field) == "" {
		return errors.New("field is required")
	}
	op := strings.TrimSpace(c.Op)
	if op == "" {
		return errors.New("op is required")
	}
	if !isSupportedOp(op) {
		return fmt.Errorf("unsupported op: %s", op)
	}
	if op == "between" {
		if c.Min == nil || c.Max == nil {
			return errors.New("min and max are required for between")
		}
		if *c.Min > *c.Max {
			return errors.New("min must be <= max")
		}
	} else {
		if c.Value == nil {
			return errors.New("value is required for comparison op")
		}
	}
	return nil
}

func (Plugin) Execute(ctx context.Context, req detector.ExecuteRequest) ([]detector.Signal, error) {
	_ = ctx // context is not used directly; kept for interface compatibility.

	var c metricThresholdConfig
	if len(req.Config) > 0 {
		if err := json.Unmarshal(req.Config, &c); err != nil {
			return nil, errors.New("config must be valid json object")
		}
	}
	if err := (Plugin{}).ValidateConfig(req.Config); err != nil {
		return nil, err
	}

	valueAny, ok := lookupField(req.Payload, c.Field)
	if !ok {
		return nil, fmt.Errorf("metric field %s not found", c.Field)
	}
	value, ok := toFloat(valueAny)
	if !ok {
		return nil, fmt.Errorf("metric field %s is not numeric", c.Field)
	}

	violated := isViolation(c, value)

	severity := "info"
	status := "resolved"
	message := "metric_threshold ok"
	if violated {
		severity = strings.TrimSpace(c.SeverityOnViolation)
		if severity == "" {
			severity = "error"
		}
		status = "firing"
		message = fmt.Sprintf("metric_threshold violated: field=%s value=%v op=%s", c.Field, value, c.Op)
	}

	fields := map[string]any{
		"source_type": "metric_threshold",
		"field":       c.Field,
		"value":       value,
		"op":          c.Op,
	}
	if c.Min != nil {
		fields["min"] = *c.Min
	}
	if c.Max != nil {
		fields["max"] = *c.Max
	}

	labels := map[string]string{
		"field": c.Field,
	}

	sig := detector.Signal{
		ProjectID:  req.ProjectID,
		Source:     "logs",
		SourceType: "metric_threshold",
		Severity:   severity,
		Status:     status,
		Title:      fmt.Sprintf("Metric threshold %s", c.Field),
		Message:    message,
		Labels:     labels,
		Fields:     fields,
		OccurredAt: nowOrUTC(req.Now),
	}

	return []detector.Signal{sig}, nil
}

func isSupportedOp(op string) bool {
	switch op {
	case ">", "<", ">=", "<=", "between":
		return true
	default:
		return false
	}
}

func isViolation(c metricThresholdConfig, v float64) bool {
	op := strings.TrimSpace(c.Op)
	switch op {
	case ">":
		return v > deref(c.Value)
	case "<":
		return v < deref(c.Value)
	case ">=":
		return v >= deref(c.Value)
	case "<=":
		return v <= deref(c.Value)
	case "between":
		return v < deref(c.Min) || v > deref(c.Max)
	default:
		return false
	}
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func lookupField(m map[string]any, path string) (any, bool) {
	if m == nil {
		return nil, false
	}
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 {
		return nil, false
	}
	var cur any = m
	for _, p := range parts {
		mp, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := mp[p]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case int32:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		if t == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func nowOrUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
