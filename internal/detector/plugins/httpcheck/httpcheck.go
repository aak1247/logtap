package httpcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

type Plugin struct{}

func New() Plugin { return Plugin{} }

func (Plugin) Type() string { return "http_check" }

// ConfigSchema returns a simple JSON schema describing http_check config.
// The schema is intentionally small to keep frontend integration simple.
func (Plugin) ConfigSchema() json.RawMessage {
	const schema = `{
  "type": "object",
  "properties": {
    "url": {"type": "string", "format": "uri"},
    "method": {"type": "string"},
    "headers": {"type": "object", "additionalProperties": {"type": "string"}},
    "body": {"type": "string"},
    "expectStatus": {"type": "array", "items": {"type": "integer"}},
    "expectBodySubstring": {"type": "string"},
    "timeoutMs": {"type": "integer", "minimum": 100, "maximum": 60000},
    "minTlsValidDays": {"type": "integer", "minimum": 0, "maximum": 3650}
  },
  "required": ["url"],
  "additionalProperties": true
}`
	return json.RawMessage(schema)
}

// httpCheckConfig mirrors the supported configuration for the plugin.
type httpCheckConfig struct {
	URL                 string            `json:"url"`
	Method              string            `json:"method"`
	Headers             map[string]string `json:"headers"`
	Body                string            `json:"body"`
	ExpectStatus        []int             `json:"expectStatus"`
	ExpectBodySubstring string            `json:"expectBodySubstring"`
	TimeoutMS           int               `json:"timeoutMs"`
	MinTLSValidDays     int               `json:"minTlsValidDays"`
}

func (Plugin) ValidateConfig(cfg json.RawMessage) error {
	// Empty config is invalid for http_check; url is required.
	raw := strings.TrimSpace(string(cfg))
	if raw == "" || raw == "null" {
		return errors.New("url is required")
	}

	var c httpCheckConfig
	if err := json.Unmarshal(cfg, &c); err != nil {
		return errors.New("config must be valid json object")
	}
	u := strings.TrimSpace(c.URL)
	if u == "" {
		return errors.New("url is required")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return errors.New("url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url must use http or https scheme")
	}
	if c.TimeoutMS < 0 {
		return errors.New("timeoutMs must be non-negative")
	}
	if c.MinTLSValidDays < 0 {
		return errors.New("minTlsValidDays must be non-negative")
	}
	return nil
}

// Execute performs a single HTTP request according to the config and
// produces exactly one detector.Signal describing the result.
func (Plugin) Execute(ctx context.Context, req detector.ExecuteRequest) ([]detector.Signal, error) {
	// Parse config (ValidateConfig should already have been called by the API layer).
	var c httpCheckConfig
	if len(req.Config) > 0 {
		if err := json.Unmarshal(req.Config, &c); err != nil {
			return nil, errors.New("config must be valid json object")
		}
	}
	c.URL = strings.TrimSpace(c.URL)
	if c.URL == "" {
		return nil, errors.New("url is required")
	}

	method := strings.ToUpper(strings.TrimSpace(c.Method))
	if method == "" {
		method = http.MethodGet
	}

	timeout := time.Duration(c.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	baseTransport := http.DefaultTransport
	if t, ok := baseTransport.(*http.Transport); ok {
		clone := t.Clone()
		clone.ResponseHeaderTimeout = timeout
		baseTransport = clone
	}
	httpClient := &http.Client{Timeout: timeout, Transport: baseTransport}

	// Build request bound to the supplied context.
	var body io.Reader
	if strings.TrimSpace(c.Body) != "" {
		body = strings.NewReader(c.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, c.URL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range c.Headers {
		httpReq.Header.Set(k, v)
	}

	start := nowOrUTC(req.Now)
	resp, err := httpClient.Do(httpReq)
	elapsed := time.Since(start)

	// Prepare base signal.
	labels := map[string]string{
		"url":    c.URL,
		"method": method,
	}
	fields := map[string]any{
		"source_type": "http_check",
		"url":         c.URL,
		"method":      method,
		"elapsed_ms":  elapsed.Milliseconds(),
	}

	severity := "info"
	status := "resolved"
	message := "http_check ok"

	var statusCode int
	var bodySnippet string

	if err != nil {
		// Network/timeout error.
		severity = "error"
		status = "firing"
		message = fmt.Sprintf("http_check failed: %v", err)
		fields["error"] = err.Error()
	} else {
		defer resp.Body.Close()
		statusCode = resp.StatusCode
		fields["status_code"] = statusCode

		// Read body only when we need to inspect substring or for small snippet.
		var bodyBytes []byte
		if c.ExpectBodySubstring != "" {
			// best-effort; ignore read errors for substring matching.
			bodyBytes, _ = io.ReadAll(io.LimitReader(resp.Body, 1024*64))
			bodySnippet = string(bodyBytes)
			fields["body_snippet"] = truncate(bodySnippet, 512)
		}

		// Status code expectation.
		if len(c.ExpectStatus) > 0 {
			if !containsInt(c.ExpectStatus, statusCode) {
				severity = "error"
				status = "firing"
				message = fmt.Sprintf("unexpected status code: got=%d", statusCode)
			}
		} else if statusCode < 200 || statusCode >= 300 {
			severity = "error"
			status = "firing"
			message = fmt.Sprintf("non-2xx status code: %d", statusCode)
		}

		// Body substring expectation.
		if c.ExpectBodySubstring != "" && bodySnippet != "" {
			if !strings.Contains(bodySnippet, c.ExpectBodySubstring) {
				severity = "error"
				status = "firing"
				message = fmt.Sprintf("body does not contain expected substring")
				fields["expected_substring"] = c.ExpectBodySubstring
			}
		}
	}

	if resp != nil && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		notAfter := resp.TLS.PeerCertificates[0].NotAfter.UTC()
		now := nowOrUTC(req.Now)
		daysLeft := notAfter.Sub(now).Hours() / 24
		fields["cert_not_after"] = notAfter.Format(time.RFC3339)
		fields["cert_days_left"] = daysLeft
		if c.MinTLSValidDays > 0 && daysLeft < float64(c.MinTLSValidDays) {
			if severity != "error" {
				severity = "error"
				status = "firing"
				message = fmt.Sprintf("certificate expiring soon: days_left=%.2f < min=%d", daysLeft, c.MinTLSValidDays)
			}
			fields["cert_expiring_soon"] = true
		}
	}

	occurredAt := nowOrUTC(req.Now)

	sig := detector.Signal{
		ProjectID:  req.ProjectID,
		Source:     "logs",
		SourceType: "http_check",
		Severity:   severity,
		Status:     status,
		Title:      fmt.Sprintf("HTTP %s %s", method, c.URL),
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

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}
