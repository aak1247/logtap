package builtin

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/channel"
)

// FeishuPlugin sends notifications via Feishu (Lark) group bot webhooks.
type FeishuPlugin struct {
	HTTPClient *http.Client
}

func (p *FeishuPlugin) Type() string { return "feishu" }

func (p *FeishuPlugin) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["webhook_url"],
		"properties": {
			"webhook_url": {"type": "string", "description": "Feishu bot webhook URL"},
			"secret": {"type": "string", "description": "Bot signing secret (optional, enables signature verification)"}
		}
	}`)
}

func (p *FeishuPlugin) ValidateConfig(cfg json.RawMessage) error {
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

func (p *FeishuPlugin) Send(ctx context.Context, msg channel.Message, cfg json.RawMessage) error {
	var c struct {
		WebhookURL string `json:"webhook_url"`
		Secret     string `json:"secret"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	text := fmt.Sprintf("%s\n%s", msg.Title, msg.Content)
	payload := map[string]any{
		"msg_type": "text",
		"content": map[string]any{
			"text": text,
		},
	}

	// If secret is set, generate sign for verification.
	if strings.TrimSpace(c.Secret) != "" {
		ts := time.Now().Unix()
		sign := feishuSign(ts, c.Secret)
		payload["timestamp"] = fmt.Sprintf("%d", ts)
		payload["sign"] = sign
	}

	body, _ := json.Marshal(payload)

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

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	_ = json.NewDecoder(res.Body).Decode(&resp)
	if resp.Code != 0 {
		return fmt.Errorf("feishu error code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

func feishuSign(timestamp int64, secret string) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	h := hmac.New(sha256.New, []byte(stringToSign))
	h.Write(nil)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
