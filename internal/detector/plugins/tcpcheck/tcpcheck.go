package tcpcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

type Plugin struct{}

func New() Plugin { return Plugin{} }

func (Plugin) Type() string { return "tcp_check" }

func (Plugin) ConfigSchema() json.RawMessage {
	const schema = `{
  "type": "object",
  "properties": {
    "host": {"type": "string"},
    "port": {"type": "integer", "minimum": 1, "maximum": 65535},
    "timeoutMs": {"type": "integer", "minimum": 100, "maximum": 60000}
  },
  "required": ["host", "port"],
  "additionalProperties": true
}`
	return json.RawMessage(schema)
}

type tcpCheckConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	TimeoutMS int    `json:"timeoutMs"`
}

func (Plugin) ValidateConfig(cfg json.RawMessage) error {
	raw := strings.TrimSpace(string(cfg))
	if raw == "" || raw == "null" {
		return errors.New("host and port are required")
	}
	var c tcpCheckConfig
	if err := json.Unmarshal(cfg, &c); err != nil {
		return errors.New("config must be valid json object")
	}
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("host is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if c.TimeoutMS < 0 {
		return errors.New("timeoutMs must be non-negative")
	}
	return nil
}

func (Plugin) Execute(ctx context.Context, req detector.ExecuteRequest) ([]detector.Signal, error) {
	var c tcpCheckConfig
	if len(req.Config) > 0 {
		if err := json.Unmarshal(req.Config, &c); err != nil {
			return nil, errors.New("config must be valid json object")
		}
	}
	c.Host = strings.TrimSpace(c.Host)
	if c.Host == "" {
		return nil, errors.New("host is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return nil, errors.New("port must be between 1 and 65535")
	}

	timeout := time.Duration(c.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	d := net.Dialer{Timeout: timeout}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))

	start := nowOrUTC(req.Now)
	conn, err := d.DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start)
	if conn != nil {
		_ = conn.Close()
	}

	labels := map[string]string{
		"host": c.Host,
		"port": strconv.Itoa(c.Port),
	}
	fields := map[string]any{
		"source_type": "tcp_check",
		"host":        c.Host,
		"port":        c.Port,
		"elapsed_ms":  elapsed.Milliseconds(),
	}

	severity := "info"
	status := "resolved"
	message := "tcp_check ok"

	if err != nil {
		severity = "error"
		status = "firing"
		message = fmt.Sprintf("tcp_check failed: %v", err)
		fields["error"] = err.Error()
	}

	occurredAt := nowOrUTC(req.Now)

	sig := detector.Signal{
		ProjectID:  req.ProjectID,
		Source:     "logs",
		SourceType: "tcp_check",
		Severity:   severity,
		Status:     status,
		Title:      fmt.Sprintf("TCP %s", addr),
		Message:    message,
		Labels:     labels,
		Fields:     fields,
		OccurredAt: occurredAt,
	}

	return []detector.Signal{sig}, nil
}

func nowOrUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
