package logbasic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

type Plugin struct{}

func New() Plugin { return Plugin{} }

func (Plugin) Type() string { return "log_basic" }

func (Plugin) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":true}`)
}

func (Plugin) ValidateConfig(cfg json.RawMessage) error {
	raw := strings.TrimSpace(string(cfg))
	if raw == "" || raw == "null" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(cfg, &m); err != nil {
		return errors.New("config must be valid json object")
	}
	return nil
}

func (Plugin) Execute(_ context.Context, req detector.ExecuteRequest) ([]detector.Signal, error) {
	payload := map[string]any{}
	for k, v := range req.Payload {
		payload[k] = v
	}

	labels := map[string]string{}
	if rawLabels, ok := payload["labels"].(map[string]any); ok {
		for key, v := range rawLabels {
			labels[key] = strings.TrimSpace(toString(v))
		}
	}
	if rawLabels, ok := payload["labels"].(map[string]string); ok {
		for key, v := range rawLabels {
			labels[key] = strings.TrimSpace(v)
		}
	}

	now := req.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	source := strings.ToLower(strings.TrimSpace(toString(payload["source"])))
	if source == "" {
		source = "logs"
	}
	severity := strings.TrimSpace(toString(payload["severity"]))
	if severity == "" {
		severity = strings.TrimSpace(toString(payload["level"]))
	}
	if severity == "" {
		severity = "info"
	}
	status := strings.TrimSpace(toString(payload["status"]))
	if status == "" {
		status = "firing"
	}
	title := strings.TrimSpace(toString(payload["title"]))
	message := strings.TrimSpace(toString(payload["message"]))
	if message == "" {
		message = title
	}

	s := detector.Signal{
		ProjectID:  req.ProjectID,
		Source:     source,
		SourceType: "log_basic",
		Severity:   severity,
		Status:     status,
		Title:      title,
		Message:    message,
		Labels:     labels,
		Fields:     payload,
		OccurredAt: now,
	}
	return []detector.Signal{s}, nil
}

func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
