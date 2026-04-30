package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/channel"
)

// WebhookPlugin sends notifications via generic HTTP webhooks.
type WebhookPlugin struct {
	HTTPClient *http.Client
}

func (p *WebhookPlugin) Type() string { return "webhook" }

func (p *WebhookPlugin) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["url"],
		"properties": {
			"url": {"type": "string", "description": "Webhook endpoint URL"},
			"headers": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Custom HTTP headers"}
		}
	}`)
}

func (p *WebhookPlugin) ValidateConfig(cfg json.RawMessage) error {
	var c struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if strings.TrimSpace(c.URL) == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

func (p *WebhookPlugin) Send(ctx context.Context, msg channel.Message, cfg json.RawMessage) error {
	var c struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	body, _ := json.Marshal(map[string]any{
		"projectId": msg.ProjectID,
		"ruleId":    msg.RuleID,
		"title":     msg.Title,
		"content":   msg.Content,
		"level":     msg.Level,
		"source":    msg.Source,
	})

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("webhook http %d", res.StatusCode)
	}
	return nil
}
