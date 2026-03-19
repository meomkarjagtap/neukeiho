package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

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
