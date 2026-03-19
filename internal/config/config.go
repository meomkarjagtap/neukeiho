package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"gopkg.in/ini.v1"
)

// Conf holds runtime settings from neukeiho.conf
type Conf struct {
	Server  ServerConf  `ini:"server"`
	Ollama  OllamaConf  `ini:"ollama"`
	Storage StorageConf `ini:"storage"`
}

type ServerConf struct {
	Port          int    `ini:"port"`
	LogPath       string `ini:"log_path"`
	RetentionDays int    `ini:"retention_days"`
}

type OllamaConf struct {
	Enabled bool   `ini:"enabled"`
	Host    string `ini:"host"`
	Port    int    `ini:"port"`
	Model   string `ini:"model"`
}

type StorageConf struct {
	Backend string `ini:"backend"`
	Path    string `ini:"path"`
}

// TOML holds nodes, thresholds, alerts from neukeiho.toml
type TOML struct {
	Nodes      map[string]NodeConf `toml:"nodes"`
	Thresholds ThresholdConf       `toml:"thresholds"`
	Alerts     AlertsConf          `toml:"alerts"`
}

type NodeConf struct {
	Host string   `toml:"host"`
	Tags []string `toml:"tags"`
}

type ThresholdConf struct {
	CPUPercent    float64 `toml:"cpu_percent"`
	MemoryPercent float64 `toml:"memory_percent"`
	DiskPercent   float64 `toml:"disk_percent"`
	NetworkMbps   float64 `toml:"network_mbps"`
}

type AlertsConf struct {
	Slack     SlackConf     `toml:"slack"`
	PagerDuty PagerDutyConf `toml:"pagerduty"`
	Email     EmailConf     `toml:"email"`
	Webhook   WebhookConf   `toml:"webhook"`
}

type SlackConf struct {
	Enabled    bool   `toml:"enabled"`
	WebhookURL string `toml:"webhook_url"`
	BotToken   string `toml:"bot_token"`
	Channel    string `toml:"channel"`
}

type PagerDutyConf struct {
	Enabled        bool   `toml:"enabled"`
	IntegrationKey string `toml:"integration_key"`
}

type EmailConf struct {
	Enabled  bool     `toml:"enabled"`
	SMTPHost string   `toml:"smtp_host"`
	SMTPPort int      `toml:"smtp_port"`
	From     string   `toml:"from"`
	To       []string `toml:"to"`
	Password string   `toml:"password"`
}

type WebhookConf struct {
	Enabled bool   `toml:"enabled"`
	URL     string `toml:"url"`
}

// LoadConf loads neukeiho.conf (INI)
func LoadConf(path string) (*Conf, error) {
	cfg := &Conf{
		Server: ServerConf{
			Port:          9100,
			LogPath:       "/var/log/neukeiho/neukeiho.log",
			RetentionDays: 90,
		},
		Ollama: OllamaConf{
			Enabled: false,
			Host:    "localhost",
			Port:    11434,
			Model:   "llama3",
		},
		Storage: StorageConf{
			Backend: "sqlite",
			Path:    "/var/lib/neukeiho/neukeiho.db",
		},
	}

	f, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load conf: %w", err)
	}
	if err := f.MapTo(cfg); err != nil {
		return nil, fmt.Errorf("map conf: %w", err)
	}
	return cfg, nil
}

// LoadTOML loads neukeiho.toml
func LoadTOML(path string) (*TOML, error) {
	var t TOML
	if _, err := toml.DecodeFile(path, &t); err != nil {
		return nil, fmt.Errorf("load toml: %w", err)
	}
	// defaults
	if t.Thresholds.CPUPercent == 0 {
		t.Thresholds.CPUPercent = 85
	}
	if t.Thresholds.MemoryPercent == 0 {
		t.Thresholds.MemoryPercent = 80
	}
	if t.Thresholds.DiskPercent == 0 {
		t.Thresholds.DiskPercent = 90
	}
	if t.Thresholds.NetworkMbps == 0 {
		t.Thresholds.NetworkMbps = 500
	}
	return &t, nil
}
