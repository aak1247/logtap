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

// WecomBotPlugin sends notifications via WeCom group bot webhooks.
type WecomBotPlugin struct {
	HTTPClient *http.Client
}

func (p *WecomBotPlugin) Type() string { return "wecom_bot" }

func (p *WecomBotPlugin) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["webhook_url"],
		"properties": {
			"webhook_url": {"type": "string", "description": "WeCom bot webhook URL"}
		}
	}`)
}

func (p *WecomBotPlugin) ValidateConfig(cfg json.RawMessage) error {
	var c struct {
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if strings.TrimSpace(c.WebhookURL) == "" {
		return fmt.Errorf("webhook_url is required")
	}
	return nil
}

func (p *WecomBotPlugin) Send(ctx context.Context, msg channel.Message, cfg json.RawMessage) error {
	var c struct {
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	body, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text": map[string]any{
			"content": msg.Title + "\n" + msg.Content,
		},
	})

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("wecom http %d", res.StatusCode)
	}
	return nil
}
