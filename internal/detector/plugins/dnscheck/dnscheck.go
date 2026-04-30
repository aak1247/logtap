package dnscheck

import (
	"context"
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

func (Plugin) Type() string { return "dns_check" }

var _ detector.AggregatablePlugin = Plugin{}
var _ detector.LifecyclePlugin = Plugin{}
var _ detector.HealthCheckPlugin = Plugin{}

func (Plugin) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "domain": {"type": "string"},
    "recordType": {"type": "string", "enum": ["A", "AAAA", "CNAME", "MX", "TXT"]},
    "expectValues": {"type": "array", "items": {"type": "string"}},
    "expectCNAME": {"type": "string"},
    "nameserver": {"type": "string"},
    "timeoutMs": {"type": "integer", "minimum": 100, "maximum": 30000}
  },
  "required": ["domain"],
  "additionalProperties": true
}`)
}

type dnsCheckConfig struct {
	Domain       string   `json:"domain"`
	RecordType   string   `json:"recordType"`
	ExpectValues []string `json:"expectValues"`
	ExpectCNAME  string   `json:"expectCname"`
	Nameserver   string   `json:"nameserver"`
	TimeoutMS    int      `json:"timeoutMs"`
}

func (Plugin) ValidateConfig(cfg json.RawMessage) error {
	raw := strings.TrimSpace(string(cfg))
	if raw == "" || raw == "null" {
		return errors.New("domain is required")
	}
	var c dnsCheckConfig
	if err := json.Unmarshal(cfg, &c); err != nil {
		return errors.New("config must be valid json object")
	}
	if strings.TrimSpace(c.Domain) == "" {
		return errors.New("domain is required")
	}
	rt := strings.ToUpper(strings.TrimSpace(c.RecordType))
	if rt == "" {
		// default A
	} else {
		switch rt {
		case "A", "AAAA", "CNAME", "MX", "TXT":
		default:
			return fmt.Errorf("unsupported recordType: %s", rt)
		}
	}
	return nil
}

func (Plugin) Execute(ctx context.Context, req detector.ExecuteRequest) ([]detector.Signal, error) {
	var c dnsCheckConfig
	if len(req.Config) > 0 {
		if err := json.Unmarshal(req.Config, &c); err != nil {
			return nil, errors.New("config must be valid json object")
		}
	}
	c.Domain = strings.TrimSpace(c.Domain)
	if c.Domain == "" {
		return nil, errors.New("domain is required")
	}

	recordType := strings.ToUpper(strings.TrimSpace(c.RecordType))
	if recordType == "" {
		recordType = "A"
	}

	timeout := time.Duration(c.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	dialer := &net.Dialer{Timeout: timeout}
	var resolver *net.Resolver
	if c.Nameserver != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialer.DialContext(ctx, "udp", c.Nameserver+":53")
			},
		}
	} else {
		resolver = net.DefaultResolver
	}

	start := nowOrUTC(req.Now)
	var results []string
	var rtt time.Duration
	var lookupErr error

	lookupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch recordType {
	case "A":
		var addrs []string
		addrs, lookupErr = resolver.LookupHost(lookupCtx, c.Domain)
		if lookupErr == nil {
			results = addrs
		}
		rtt = time.Since(start)
	case "AAAA":
		// LookupHost returns both A and AAAA; filter for IPv6
		addrs, err := resolver.LookupHost(lookupCtx, c.Domain)
		if err != nil {
			lookupErr = err
		} else {
			for _, a := range addrs {
				if strings.Contains(a, ":") {
					results = append(results, a)
				}
			}
		}
		rtt = time.Since(start)
	case "CNAME":
		cname, err := resolver.LookupCNAME(lookupCtx, c.Domain)
		if err != nil {
			lookupErr = err
		} else {
			results = []string{cname}
		}
		rtt = time.Since(start)
	case "MX":
		mxRecords, err := resolver.LookupMX(lookupCtx, c.Domain)
		if err != nil {
			lookupErr = err
		} else {
			for _, mx := range mxRecords {
				results = append(results, fmt.Sprintf("%s pref=%d", mx.Host, mx.Pref))
			}
		}
		rtt = time.Since(start)
	case "TXT":
		txtRecords, err := resolver.LookupTXT(lookupCtx, c.Domain)
		if err != nil {
			lookupErr = err
		} else {
			results = txtRecords
		}
		rtt = time.Since(start)
	}

	severity := "info"
	status := "resolved"
	message := fmt.Sprintf("dns_check %s %s ok", recordType, c.Domain)

	fields := map[string]any{
		"source_type": "dns_check",
		"domain":      c.Domain,
		"record_type": recordType,
		"elapsed_ms":  rtt.Milliseconds(),
		"results":     results,
		"success":     true,
	}
	labels := map[string]string{
		"domain":      c.Domain,
		"record_type": recordType,
	}

	if lookupErr != nil {
		severity = "error"
		status = "firing"
		message = fmt.Sprintf("dns_check %s %s failed: %v", recordType, c.Domain, lookupErr)
		fields["error"] = lookupErr.Error()
		fields["success"] = false
	} else {
		// Validate expected values
		if len(c.ExpectValues) > 0 {
			found := false
			for _, expected := range c.ExpectValues {
				for _, actual := range results {
					if strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(expected)) {
						found = true
						break
					}
				}
				if !found {
					severity = "error"
					status = "firing"
					message = fmt.Sprintf("dns_check: expected value %s not found in results %v", expected, results)
					fields["success"] = false
					break
				}
			}
		}
		// Validate expected CNAME
		if c.ExpectCNAME != "" && recordType == "CNAME" {
			if len(results) == 0 || !strings.HasSuffix(strings.ToLower(results[0]), strings.ToLower(c.ExpectCNAME)) {
				severity = "error"
				status = "firing"
				message = fmt.Sprintf("dns_check: CNAME mismatch, expected %s, got %v", c.ExpectCNAME, results)
				fields["success"] = false
			}
		}
	}

	occurredAt := nowOrUTC(req.Now)
	sig := detector.Signal{
		ProjectID:  req.ProjectID,
		Source:     "logs",
		SourceType: "dns_check",
		Severity:   severity,
		Status:     status,
		Title:      fmt.Sprintf("DNS %s %s", recordType, c.Domain),
		Message:    message,
		Labels:     labels,
		Fields:     fields,
		OccurredAt: occurredAt,
	}

	return []detector.Signal{sig}, nil
}

func (Plugin) OnActivate(ctx context.Context, config json.RawMessage) error {
	return nil
}

func (Plugin) OnDeactivate(ctx context.Context) error {
	return nil
}

func (Plugin) HealthCheck(ctx context.Context) error {
	// Simple self-test: resolve localhost
	resolver := net.DefaultResolver
	_, err := resolver.LookupHost(ctx, "localhost")
	return err
}

func (Plugin) StoreResults(ctx context.Context, projectID int, results []detector.TypedResult) error {
	return nil
}

func (Plugin) QueryResults(ctx context.Context, projectID int, query detector.ResultQuery) ([]detector.TypedResult, error) {
	return nil, nil
}

func (Plugin) Aggregate(ctx context.Context, projectID int, tr detector.TimeRange, interval detector.AggregateInterval) ([]detector.MetricPoint, error) {
	return nil, fmt.Errorf("dns_check Aggregate: use ResultStore.AggregateAvgFloat directly")
}

func AggregateWithStore(ctx context.Context, store *detector.ResultStore, projectID int, tr detector.TimeRange, interval detector.AggregateInterval) (map[string][]detector.MetricPoint, error) {
	elapsed, err := store.AggregateAvgFloat(ctx, "dns_check", projectID, "elapsed_ms", tr, interval)
	if err != nil {
		return nil, err
	}
	success, err := store.AggregateSuccessRate(ctx, "dns_check", projectID, tr, interval)
	if err != nil {
		return nil, err
	}
	return map[string][]detector.MetricPoint{
		"elapsed_ms":   elapsed,
		"success_rate": success,
	}, nil
}

func nowOrUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
