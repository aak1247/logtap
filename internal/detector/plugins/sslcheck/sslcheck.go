package sslcheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

type Plugin struct{}

func New() Plugin { return Plugin{} }

func (Plugin) Type() string { return "ssl_check" }

var _ detector.AggregatablePlugin = Plugin{}
var _ detector.LifecyclePlugin = Plugin{}
var _ detector.HealthCheckPlugin = Plugin{}

func (Plugin) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "host": {"type": "string"},
    "port": {"type": "integer", "minimum": 1, "maximum": 65535},
    "timeoutMs": {"type": "integer", "minimum": 100, "maximum": 30000},
    "minValidDays": {"type": "integer", "minimum": 0, "maximum": 3650},
    "expectedSANs": {"type": "array", "items": {"type": "string"}},
    "allowSelfSigned": {"type": "boolean"}
  },
  "required": ["host"],
  "additionalProperties": true
}`)
}

type sslCheckConfig struct {
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	TimeoutMS      int      `json:"timeoutMs"`
	MinValidDays   int      `json:"minValidDays"`
	ExpectedSANs   []string `json:"expectedSANs"`
	AllowSelfSigned bool    `json:"allowSelfSigned"`
}

func (Plugin) ValidateConfig(cfg json.RawMessage) error {
	raw := strings.TrimSpace(string(cfg))
	if raw == "" || raw == "null" {
		return errors.New("host is required")
	}
	var c sslCheckConfig
	if err := json.Unmarshal(cfg, &c); err != nil {
		return errors.New("config must be valid json object")
	}
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("host is required")
	}
	if c.Port < 0 || c.Port > 65535 {
		return errors.New("port must be between 0 and 65535")
	}
	return nil
}

func (Plugin) Execute(ctx context.Context, req detector.ExecuteRequest) ([]detector.Signal, error) {
	var c sslCheckConfig
	if len(req.Config) > 0 {
		if err := json.Unmarshal(req.Config, &c); err != nil {
			return nil, errors.New("config must be valid json object")
		}
	}
	c.Host = strings.TrimSpace(c.Host)
	if c.Host == "" {
		return nil, errors.New("host is required")
	}
	if c.Port <= 0 {
		c.Port = 443
	}

	timeout := time.Duration(c.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	now := nowOrUTC(req.Now)

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp",
		net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port)),
		&tls.Config{
			InsecureSkipVerify: true, // We want to inspect even invalid certs
			ServerName:         c.Host,
		},
	)
	if err != nil {
		return []detector.Signal{{
			ProjectID:  req.ProjectID,
			Source:     "logs",
			SourceType: "ssl_check",
			Severity:   "error",
			Status:     "firing",
			Title:      fmt.Sprintf("SSL %s:%d", c.Host, c.Port),
			Message:    fmt.Sprintf("ssl_check connection failed: %v", err),
			Labels:     map[string]string{"host": c.Host, "port": fmt.Sprintf("%d", c.Port)},
			Fields:     map[string]any{"error": err.Error(), "success": false, "source_type": "ssl_check"},
			OccurredAt: now,
		}}, nil
	}
	_ = conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return []detector.Signal{{
			ProjectID:  req.ProjectID,
			Source:     "logs",
			SourceType: "ssl_check",
			Severity:   "error",
			Status:     "firing",
			Title:      fmt.Sprintf("SSL %s:%d", c.Host, c.Port),
			Message:    "no peer certificates found",
			Labels:     map[string]string{"host": c.Host, "port": fmt.Sprintf("%d", c.Port)},
			Fields:     map[string]any{"success": false, "source_type": "ssl_check"},
			OccurredAt: now,
		}}, nil
	}

	leaf := certs[0]

	severity := "info"
	status := "resolved"
	message := "ssl_check ok"
	fields := map[string]any{
		"source_type":    "ssl_check",
		"host":           c.Host,
		"port":           c.Port,
		"success":        true,
		"subject_cn":     leaf.Subject.CommonName,
		"not_before":     leaf.NotBefore.Format(time.RFC3339),
		"not_after":      leaf.NotAfter.Format(time.RFC3339),
		"cert_days_left": leaf.NotAfter.Sub(now).Hours() / 24,
		"issuer":         leaf.Issuer.CommonName,
		"sans":           leaf.DNSNames,
	}

	labels := map[string]string{
		"host": c.Host,
		"port": fmt.Sprintf("%d", c.Port),
	}

	// Check expiry
	daysLeft := leaf.NotAfter.Sub(now).Hours() / 24
	fields["cert_days_left"] = daysLeft
	if leaf.NotAfter.Before(now) {
		severity = "error"
		status = "firing"
		message = fmt.Sprintf("certificate expired on %s", leaf.NotAfter.Format(time.RFC3339))
		fields["success"] = false
	} else if c.MinValidDays > 0 && daysLeft < float64(c.MinValidDays) {
		severity = "error"
		status = "firing"
		message = fmt.Sprintf("certificate expiring soon: days_left=%.1f < min=%d", daysLeft, c.MinValidDays)
		fields["success"] = false
	}

	// Check self-signed
	isSelfSigned := isSelfSignedCert(leaf, certs)
	fields["is_self_signed"] = isSelfSigned
	if isSelfSigned && !c.AllowSelfSigned {
		severity = "error"
		status = "firing"
		message = "certificate is self-signed"
		fields["success"] = false
	}

	// Check SAN matching
	if len(c.ExpectedSANs) > 0 {
		for _, expected := range c.ExpectedSANs {
			found := false
			for _, san := range leaf.DNSNames {
				if strings.EqualFold(san, expected) {
					found = true
					break
				}
			}
			if !found {
				severity = "error"
				status = "firing"
				message = fmt.Sprintf("expected SAN %s not found", expected)
				fields["success"] = false
				break
			}
		}
	}

	// Check chain completeness (intermediate certs present)
	if len(certs) < 2 && !isSelfSigned {
		fields["chain_incomplete"] = true
		// Warning but not critical
		if severity == "info" {
			severity = "warning"
		}
	}

	sig := detector.Signal{
		ProjectID:  req.ProjectID,
		Source:     "logs",
		SourceType: "ssl_check",
		Severity:   severity,
		Status:     status,
		Title:      fmt.Sprintf("SSL %s:%d", c.Host, c.Port),
		Message:    message,
		Labels:     labels,
		Fields:     fields,
		OccurredAt: now,
	}
	return []detector.Signal{sig}, nil
}

func isSelfSignedCert(leaf *x509.Certificate, chain []*x509.Certificate) bool {
	if leaf.Issuer.CommonName == leaf.Subject.CommonName {
		return true
	}
	// Check if issuer is in the chain
	for _, cert := range chain[1:] {
		if cert.Subject.CommonName == leaf.Issuer.CommonName {
			// Check if the issuer cert is also signed by itself
			if cert.Issuer.CommonName == cert.Subject.CommonName {
				return true
			}
		}
	}
	return false
}

func (Plugin) OnActivate(ctx context.Context, config json.RawMessage) error {
	return nil
}

func (Plugin) OnDeactivate(ctx context.Context) error {
	return nil
}

func (Plugin) HealthCheck(ctx context.Context) error {
	// Simple self-test: try TLS to a known good host
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", "example.com:443", &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func (Plugin) StoreResults(ctx context.Context, projectID int, results []detector.TypedResult) error {
	return nil
}

func (Plugin) QueryResults(ctx context.Context, projectID int, query detector.ResultQuery) ([]detector.TypedResult, error) {
	return nil, nil
}

func (Plugin) Aggregate(ctx context.Context, projectID int, tr detector.TimeRange, interval detector.AggregateInterval) ([]detector.MetricPoint, error) {
	return nil, fmt.Errorf("ssl_check Aggregate: use ResultStore.AggregateAvgFloat directly")
}

func AggregateWithStore(ctx context.Context, store *detector.ResultStore, projectID int, tr detector.TimeRange, interval detector.AggregateInterval) (map[string][]detector.MetricPoint, error) {
	daysLeft, err := store.AggregateAvgFloat(ctx, "ssl_check", projectID, "cert_days_left", tr, interval)
	if err != nil {
		return nil, err
	}
	return map[string][]detector.MetricPoint{
		"cert_days_left": daysLeft,
	}, nil
}

func nowOrUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
