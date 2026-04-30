package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/aak1247/logtap/internal/channel"
)

// EmailPlugin sends notifications via SMTP.
type EmailPlugin struct {
	Host     string
	Port     int
	From     string
	Username string
	Password string
}

func (p *EmailPlugin) Type() string { return "email" }

func (p *EmailPlugin) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["recipients"],
		"properties": {
			"recipients": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Email addresses to send to"
			}
		}
	}`)
}

func (p *EmailPlugin) ValidateConfig(cfg json.RawMessage) error {
	var c struct {
		Recipients []string `json:"recipients"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if len(c.Recipients) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	return nil
}

func (p *EmailPlugin) Send(_ context.Context, msg channel.Message, cfg json.RawMessage) error {
	var c struct {
		Recipients []string `json:"recipients"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	host := strings.TrimSpace(p.Host)
	if host == "" {
		return fmt.Errorf("SMTP_HOST not configured")
	}
	port := p.Port
	if port <= 0 {
		port = 587
	}
	from := strings.TrimSpace(p.From)
	if from == "" {
		return fmt.Errorf("SMTP_FROM not configured")
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	subject := msg.Title
	body := msg.Content

	emailMsg := "To: " + strings.Join(c.Recipients, ", ") + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" + body + "\r\n"

	var auth smtp.Auth
	if strings.TrimSpace(p.Username) != "" {
		auth = smtp.PlainAuth("", p.Username, p.Password, host)
	}
	return smtp.SendMail(addr, auth, from, c.Recipients, []byte(emailMsg))
}
