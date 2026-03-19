package alerter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"

	"github.com/meomkarjagtap/neukeiho/internal/threshold"
)

// Config holds all alert channel configs loaded from neukeiho.toml.
type Config struct {
	Slack     SlackConfig     `toml:"slack"`
	PagerDuty PagerDutyConfig `toml:"pagerduty"`
	Email     EmailConfig     `toml:"email"`
	Webhook   WebhookConfig   `toml:"webhook"`
}

type SlackConfig struct {
	Enabled    bool   `toml:"enabled"`
	WebhookURL string `toml:"webhook_url"`
	BotToken   string `toml:"bot_token"`
	Channel    string `toml:"channel"`
}

type PagerDutyConfig struct {
	Enabled        bool   `toml:"enabled"`
	IntegrationKey string `toml:"integration_key"`
}

type EmailConfig struct {
	Enabled  bool     `toml:"enabled"`
	SMTPHost string   `toml:"smtp_host"`
	SMTPPort int      `toml:"smtp_port"`
	From     string   `toml:"from"`
	To       []string `toml:"to"`
	Password string   `toml:"password"`
}

type WebhookConfig struct {
	Enabled bool   `toml:"enabled"`
	URL     string `toml:"url"`
}

// Alerter dispatches breach notifications to configured channels.
type Alerter struct {
	cfg Config
}

// New creates a new Alerter with default config.
func New() *Alerter {
	return &Alerter{}
}

// WithConfig sets alert channel config.
func (a *Alerter) WithConfig(cfg Config) *Alerter {
	a.cfg = cfg
	return a
}

// Dispatch sends a breach alert to all enabled channels.
func (a *Alerter) Dispatch(b threshold.Breach) {
	if a.cfg.Slack.Enabled {
		if err := a.sendSlack(b.Message); err != nil {
			fmt.Printf("[alerter] slack error: %v\n", err)
		}
	}
	if a.cfg.PagerDuty.Enabled {
		if err := a.sendPagerDuty(b); err != nil {
			fmt.Printf("[alerter] pagerduty error: %v\n", err)
		}
	}
	if a.cfg.Email.Enabled {
		if err := a.sendEmail(b.Message); err != nil {
			fmt.Printf("[alerter] email error: %v\n", err)
		}
	}
	if a.cfg.Webhook.Enabled {
		if err := a.sendWebhook(b); err != nil {
			fmt.Printf("[alerter] webhook error: %v\n", err)
		}
	}
}

func (a *Alerter) sendSlack(message string) error {
	payload, _ := json.Marshal(map[string]string{"text": message})
	resp, err := http.Post(a.cfg.Slack.WebhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (a *Alerter) sendPagerDuty(b threshold.Breach) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"routing_key":  a.cfg.PagerDuty.IntegrationKey,
		"event_action": "trigger",
		"payload": map[string]interface{}{
			"summary":  b.Message,
			"source":   b.NodeID,
			"severity": "critical",
		},
	})
	resp, err := http.Post(
		"https://events.pagerduty.com/v2/enqueue",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (a *Alerter) sendEmail(message string) error {
	addr := fmt.Sprintf("%s:%d", a.cfg.Email.SMTPHost, a.cfg.Email.SMTPPort)
	auth := smtp.PlainAuth("", a.cfg.Email.From, a.cfg.Email.Password, a.cfg.Email.SMTPHost)
	body := fmt.Sprintf("Subject: NeuKeiho Alert\r\n\r\n%s", message)
	return smtp.SendMail(addr, auth, a.cfg.Email.From, a.cfg.Email.To, []byte(body))
}

func (a *Alerter) sendWebhook(b threshold.Breach) error {
	payload, _ := json.Marshal(b)
	resp, err := http.Post(a.cfg.Webhook.URL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
