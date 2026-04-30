package builtin

import (
	"net/http"
	"time"

	"github.com/aak1247/logtap/internal/channel"
	"github.com/aak1247/logtap/internal/config"
)

// defaultHTTPClient is shared across built-in HTTP-based channels.
var defaultHTTPClient = &http.Client{
	Timeout: 5 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// RegisterAll registers all built-in channel plugins into the given registry.
// cfg provides SMTP settings for the email channel.
func RegisterAll(registry *channel.Registry, cfg config.Config) error {
	if err := registry.RegisterStatic(&WecomBotPlugin{HTTPClient: defaultHTTPClient}); err != nil {
		return err
	}
	if err := registry.RegisterStatic(&WebhookPlugin{HTTPClient: defaultHTTPClient}); err != nil {
		return err
	}
	if err := registry.RegisterStatic(&FeishuPlugin{HTTPClient: defaultHTTPClient}); err != nil {
		return err
	}
	if err := registry.RegisterStatic(&EmailPlugin{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		From:     cfg.SMTPFrom,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
	}); err != nil {
		return err
	}
	return nil
}
