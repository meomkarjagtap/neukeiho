package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/meomkarjagtap/neukeiho/internal/alerter"
	"github.com/meomkarjagtap/neukeiho/internal/collector"
	"github.com/meomkarjagtap/neukeiho/internal/config"
	"github.com/meomkarjagtap/neukeiho/internal/ollama"
	"github.com/meomkarjagtap/neukeiho/internal/store"
	"github.com/meomkarjagtap/neukeiho/internal/threshold"
)

const (
	defaultConf = "/etc/neukeiho/neukeiho.conf"
	defaultTOML = "/etc/neukeiho/neukeiho.toml"
	version     = "v0.1.0"
	githubRepo  = "meomkarjagtap/neukeiho"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		runInit()
	case "start":
		runStart()
	case "deploy":
		runDeploy()
	case "status":
		runStatus()
	case "ask":
		runAsk()
	case "test-alert":
		runTestAlert()
	case "update":
		runUpdate()
	case "version":
		fmt.Println("neukeiho " + version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`NeuKeiho (警報) — Hybrid VM Monitoring & AI Alerting ` + version + `

Usage:
  neukeiho init          Generate neukeiho.conf and neukeiho.toml interactively
  neukeiho start         Start the controller
  neukeiho deploy        Deploy agents to nodes via Ansible
  neukeiho status        Live view of all node metrics
  neukeiho ask "<q>"     Ask Ollama about your infra
  neukeiho test-alert    Fire a test alert to all configured channels
  neukeiho update        Update to the latest version from GitHub
  neukeiho version       Print version`)
}

// ── start ─────────────────────────────────────────────────────────────────────

func runStart() {
	conf, tomlCfg := mustLoadConfig()
	setupLogging(conf.Server.LogPath)
	log.Printf("[neukeiho] starting %s on :%d", version, conf.Server.Port)

	db, err := store.New(conf.Storage.Path)
	if err != nil {
		log.Fatalf("[neukeiho] store: %v", err)
	}
	defer db.Close()

	col := collector.New()

	thr := threshold.New().WithConfig(threshold.Config{
		CPUPercent:    tomlCfg.Thresholds.CPUPercent,
		MemoryPercent: tomlCfg.Thresholds.MemoryPercent,
		DiskPercent:   tomlCfg.Thresholds.DiskPercent,
		NetworkMbps:   tomlCfg.Thresholds.NetworkMbps,
	})

	al := alerter.New().WithConfig(alerter.Config{
		Slack: alerter.SlackConfig{
			Enabled:    tomlCfg.Alerts.Slack.Enabled,
			WebhookURL: tomlCfg.Alerts.Slack.WebhookURL,
			BotToken:   tomlCfg.Alerts.Slack.BotToken,
			Channel:    tomlCfg.Alerts.Slack.Channel,
		},
		PagerDuty: alerter.PagerDutyConfig{
			Enabled:        tomlCfg.Alerts.PagerDuty.Enabled,
			IntegrationKey: tomlCfg.Alerts.PagerDuty.IntegrationKey,
		},
		Email: alerter.EmailConfig{
			Enabled:  tomlCfg.Alerts.Email.Enabled,
			SMTPHost: tomlCfg.Alerts.Email.SMTPHost,
			SMTPPort: tomlCfg.Alerts.Email.SMTPPort,
			From:     tomlCfg.Alerts.Email.From,
			To:       tomlCfg.Alerts.Email.To,
			Password: tomlCfg.Alerts.Email.Password,
		},
		Webhook: alerter.WebhookConfig{
			Enabled: tomlCfg.Alerts.Webhook.Enabled,
			URL:     tomlCfg.Alerts.Webhook.URL,
		},
	})

	ol := ollama.New(ollama.Config{
		Enabled: conf.Ollama.Enabled,
		Host:    conf.Ollama.Host,
		Port:    conf.Ollama.Port,
		Model:   conf.Ollama.Model,
	})

	// wire: breach/recovery → enrich → store → alert
	thr.OnBreach = func(b threshold.Breach) {
		log.Printf("[threshold] %s: %s", b.Type, b.Message)
		snapshot := col.Latest()
		ollamaCtx := db.BuildOllamaContext(b.NodeID, snapshot)
		enrichedMsg := ol.EnrichAlert(b.Message, snapshot)
		incID, err := db.CreateIncident(b, ollamaCtx)
		if err != nil {
			log.Printf("[store] incident: %v", err)
		}
		log.Printf("[store] incident #%d created", incID)
		b.Message = enrichedMsg
		al.Dispatch(b)
	}

	// wire: agent push → store + threshold
	col.OnMetrics = func(p collector.MetricsPayload) {
		if err := db.WriteMetrics(p); err != nil {
			log.Printf("[store] metrics: %v", err)
		}
		thr.Evaluate(p)
	}

	// hourly baseline recalculation
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			for nodeID := range col.Latest() {
				if err := db.RecalculateBaseline(nodeID); err != nil {
					log.Printf("[baseline] %s: %v", nodeID, err)
				}
			}
		}
	}()

	// nightly purge
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			if err := db.Purge(conf.Server.RetentionDays); err != nil {
				log.Printf("[store] purge: %v", err)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/metrics", col)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, version)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		snapshot := col.Latest()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{")
		first := true
		for id, m := range snapshot {
			if !first {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w,
				`"%s":{"cpu":%.1f,"memory":%.1f,"disk":%.1f,"net_rx":%.1f,"net_tx":%.1f}`,
				id, m.CPU, m.Memory, m.Disk, m.NetworkRx, m.NetworkTx,
			)
			first = false
		}
		fmt.Fprint(w, "}")
	})

	addr := fmt.Sprintf(":%d", conf.Server.Port)
	log.Printf("[neukeiho] listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[neukeiho] server: %v", err)
	}
}

// ── init ──────────────────────────────────────────────────────────────────────

func runInit() {
	fmt.Println("NeuKeiho init — generating configuration")
	fmt.Println()
	reader := bufio.NewReader(os.Stdin)
	prompt := func(label, def string) string {
		fmt.Printf("  %s [%s]: ", label, def)
		val, _ := reader.ReadString('\n')
		val = strings.TrimSpace(val)
		if val == "" {
			return def
		}
		return val
	}

	port := prompt("Controller port", "9100")
	logPath := prompt("Log path", "/var/log/neukeiho/neukeiho.log")
	dbPath := prompt("SQLite DB path", "/var/lib/neukeiho/neukeiho.db")
	ollamaEnabled := prompt("Enable Ollama (true/false)", "false")
	ollamaModel := prompt("Ollama model", "llama3")

	confContent := fmt.Sprintf(`[server]
port           = %s
log_path       = %s
retention_days = 90

[ollama]
enabled = %s
host    = localhost
port    = 11434
model   = %s

[storage]
backend = sqlite
path    = %s
`, port, logPath, ollamaEnabled, ollamaModel, dbPath)

	tomlContent := `[nodes]
  # [nodes.web-01]
  # host = "192.168.1.10"
  # tags = ["web", "production"]

[thresholds]
cpu_percent    = 85.0
memory_percent = 80.0
disk_percent   = 90.0
network_mbps   = 500.0

[alerts]
  [alerts.slack]
  enabled     = false
  webhook_url = "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
  bot_token   = "xoxb-your-bot-token"
  channel     = "#infra-alerts"

  [alerts.pagerduty]
  enabled         = false
  integration_key = "your-pagerduty-integration-key"

  [alerts.email]
  enabled   = false
  smtp_host = "smtp.example.com"
  smtp_port = 587
  from      = "neukeiho@example.com"
  to        = ["ops@example.com"]
  password  = ""

  [alerts.webhook]
  enabled = false
  url     = "https://your-endpoint.com/neukeiho"
`
	confDir := "/etc/neukeiho"
	if err := os.MkdirAll(confDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "cannot create %s: %v\n", confDir, err)
		os.Exit(1)
	}
	writeFile(confDir+"/neukeiho.conf", confContent)
	writeFile(confDir+"/neukeiho.toml", tomlContent)
	fmt.Printf("\n✅ Config written to %s/\n", confDir)
	fmt.Println("   Edit neukeiho.toml to add nodes and alert channels.")
	fmt.Println("   Then run: neukeiho start")
}

// ── deploy ────────────────────────────────────────────────────────────────────

func runDeploy() {
	fmt.Println("[neukeiho] running ansible-playbook deploy/ansible/playbook.yml")
	fmt.Println("[neukeiho] ensure ansible is installed and neukeiho.toml nodes are configured")
}

// ── status ────────────────────────────────────────────────────────────────────

func runStatus() {
	conf, _ := mustLoadConfig()
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/status", conf.Server.Port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "controller not reachable on :%d — is neukeiho running?\n", conf.Server.Port)
		os.Exit(1)
	}
	defer resp.Body.Close()
	fmt.Printf("%-20s %8s %8s %8s %10s %10s\n", "NODE", "CPU%", "MEM%", "DISK%", "RX Mbps", "TX Mbps")
	fmt.Println(strings.Repeat("─", 70))
	fmt.Println("(parse /status JSON and render rows)")
}

// ── ask ───────────────────────────────────────────────────────────────────────

func runAsk() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: neukeiho ask \"<question>\"")
		os.Exit(1)
	}
	conf, _ := mustLoadConfig()
	ol := ollama.New(ollama.Config{
		Enabled: conf.Ollama.Enabled,
		Host:    conf.Ollama.Host,
		Port:    conf.Ollama.Port,
		Model:   conf.Ollama.Model,
	})
	answer, err := ol.Ask(os.Args[2], nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ollama error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(answer)
}

// ── test-alert ────────────────────────────────────────────────────────────────

func runTestAlert() {
	_, tomlCfg := mustLoadConfig()
	al := alerter.New().WithConfig(alerter.Config{
		Slack: alerter.SlackConfig{
			Enabled:    tomlCfg.Alerts.Slack.Enabled,
			WebhookURL: tomlCfg.Alerts.Slack.WebhookURL,
		},
		PagerDuty: alerter.PagerDutyConfig{
			Enabled:        tomlCfg.Alerts.PagerDuty.Enabled,
			IntegrationKey: tomlCfg.Alerts.PagerDuty.IntegrationKey,
		},
		Email: alerter.EmailConfig{
			Enabled:  tomlCfg.Alerts.Email.Enabled,
			SMTPHost: tomlCfg.Alerts.Email.SMTPHost,
			SMTPPort: tomlCfg.Alerts.Email.SMTPPort,
			From:     tomlCfg.Alerts.Email.From,
			To:       tomlCfg.Alerts.Email.To,
			Password: tomlCfg.Alerts.Email.Password,
		},
		Webhook: alerter.WebhookConfig{
			Enabled: tomlCfg.Alerts.Webhook.Enabled,
			URL:     tomlCfg.Alerts.Webhook.URL,
		},
	})
	al.Dispatch(threshold.Breach{
		NodeID:    "test-node",
		Metric:    "CPU",
		Value:     99.9,
		Threshold: 85,
		Type:      threshold.BreachTypeAlert,
		At:        time.Now(),
		Message:   "🔴 [NeuKeiho] TEST ALERT — alerting is working correctly.",
	})
	fmt.Println("[neukeiho] test alert dispatched to all enabled channels")
}

// ── update ────────────────────────────────────────────────────────────────────

func runUpdate() {
	fmt.Println("[neukeiho] checking for latest version...")

	client := &http.Client{Timeout: 15 * time.Second}

	req, _ := http.NewRequest("GET",
		fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo), nil)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to check latest release: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse release info: %v\n", err)
		os.Exit(1)
	}

	if release.TagName == version {
		fmt.Printf("✅ already on latest version %s\n", version)
		return
	}

	fmt.Printf("[neukeiho] new version available: %s (current: %s)\n", release.TagName, version)

	// determine arch suffix
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "arm64"
	case "arm":
		arch = "arm"
	default:
		fmt.Fprintf(os.Stderr, "unsupported arch: %s\n", arch)
		os.Exit(1)
	}

	assetName := fmt.Sprintf("neukeiho-%s-%s", release.TagName, arch)

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		fmt.Fprintf(os.Stderr, "no binary found for arch %s in release %s\n", arch, release.TagName)
		os.Exit(1)
	}

	fmt.Printf("[neukeiho] downloading %s...\n", assetName)

	binResp, err := client.Get(downloadURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "download failed: %v\n", err)
		os.Exit(1)
	}
	defer binResp.Body.Close()

	// write to temp file
	tmpFile := "/tmp/neukeiho-update"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp file: %v\n", err)
		os.Exit(1)
	}
	if _, err := io.Copy(f, binResp.Body); err != nil {
		f.Close()
		fmt.Fprintf(os.Stderr, "failed to write binary: %v\n", err)
		os.Exit(1)
	}
	f.Close()

	// find current binary path
	currentBin, err := os.Executable()
	if err != nil {
		currentBin = "/usr/bin/neukeiho"
	}

	// try rename first (same filesystem), fallback to copy
	if err := os.Rename(tmpFile, currentBin); err != nil {
		if err := copyFile(tmpFile, currentBin); err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			os.Exit(1)
		}
		os.Remove(tmpFile)
	}

	fmt.Printf("✅ updated to %s — restart neukeiho to apply:\n", release.TagName)
	fmt.Println("   sudo pkill -f 'neukeiho start'")
	fmt.Println("   sudo nohup neukeiho start > /var/log/neukeiho/neukeiho.log 2>&1 &")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mustLoadConfig() (*config.Conf, *config.TOML) {
	conf, err := config.LoadConf(defaultConf)
	if err != nil {
		conf, err = config.LoadConf("config/neukeiho.conf.example")
		if err != nil {
			log.Fatalf("load conf: %v\nRun 'neukeiho init' first.", err)
		}
	}
	tomlCfg, err := config.LoadTOML(defaultTOML)
	if err != nil {
		tomlCfg, err = config.LoadTOML("config/neukeiho.toml.example")
		if err != nil {
			log.Fatalf("load toml: %v\nRun 'neukeiho init' first.", err)
		}
	}
	return conf, tomlCfg
}

func setupLogging(logPath string) {
	_ = os.MkdirAll("/var/log/neukeiho", 0755)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(f)
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func writeFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Printf("  wrote %s\n", path)
}
