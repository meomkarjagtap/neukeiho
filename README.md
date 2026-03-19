# NeuKeiho

**NeuKeiho** (警報) is a lightweight hybrid VM monitoring and alerting controller written in Go.  
It receives metrics from `neukeiho-agent` instances running on your nodes, evaluates thresholds, fires alerts, and optionally uses a local Ollama LLM to analyze your infrastructure and answer questions via Slack, Teams, Discord, or Webhook.

> Part of the [neurader](https://github.com/meomkarjagtap) ecosystem.

---

## Features

- Real-time metrics ingestion from agents (CPU, Memory, Disk, Network I/O)
- Threshold-based alerting — Slack, PagerDuty, Email, Webhook
- Optional **Ollama integration** — local LLM, fully air-gapped, infra-aware
  - Enriches alert messages with plain-English analysis
  - Answers ops team questions via chat (`neukeiho ask "..."`)
- Conversational bot endpoint — reply to alerts from Slack/Teams/Discord and get answers
- Ansible-based agent deployment via `neukeiho deploy`
- INI + TOML config (consistent with NeuRader)

---

## Architecture

```
neukeiho-agent (each node)
    └── POST /metrics every N seconds
            │
            ▼
neukeiho controller
    ├── collector      — ingests + stores metrics
    ├── threshold      — evaluates breach conditions
    ├── alerter        — dispatches to Slack/PagerDuty/Email/Webhook
    ├── ollama bridge  — enriches alerts, answers infra questions (optional)
    └── bot listener   — receives replies from chat platforms → routes to Ollama
```

---

## Installation

### Prerequisites

- Go 1.22+
- Ansible (for `neukeiho deploy`)
- Ollama (optional, for AI features)

### Build

```bash
git clone https://github.com/meomkarjagtap/neukeiho
cd neukeiho
go build -o neukeiho ./cmd/neukeiho
sudo mv neukeiho /usr/bin/neukeiho
```

---

## Quick Start

### 1. Initialize config

```bash
neukeiho init
```

This generates:
- `/etc/neukeiho/neukeiho.conf` — runtime settings (port, log path, Ollama)
- `/etc/neukeiho/neukeiho.toml` — nodes, thresholds, alert channels

### 2. Edit config

```bash
# Add your nodes and alert channel credentials
vim /etc/neukeiho/neukeiho.toml
```

### 3. Deploy agents to nodes

```bash
neukeiho deploy
```

Runs the bundled Ansible playbook, installs `neukeiho-agent` on every node defined in `neukeiho.toml`.

### 4. Start the controller

```bash
neukeiho start
```

### 5. Watch live status

```bash
neukeiho status
```

---

## CLI Reference

| Command | Description |
|---|---|
| `neukeiho init` | Generate config files interactively |
| `neukeiho start` | Start the controller |
| `neukeiho deploy` | Deploy agents via Ansible |
| `neukeiho status` | Live terminal view of all node metrics |
| `neukeiho ask "<question>"` | Ask Ollama about your infra |
| `neukeiho test-alert` | Fire a test alert to all configured channels |
| `neukeiho version` | Print version |

---

## Config Reference

### neukeiho.conf (INI)

```ini
[server]
port           = 9100
log_path       = /var/log/neukeiho/neukeiho.log
retention_days = 30

[ollama]
enabled = false
host    = localhost
port    = 11434
model   = llama3
```

### neukeiho.toml (TOML)

```toml
[nodes]
  [nodes.web-01]
  host = "192.168.1.10"
  tags = ["web", "production"]

[thresholds]
cpu_percent    = 85.0
memory_percent = 80.0
disk_percent   = 90.0
network_mbps   = 500.0

[alerts]
  [alerts.slack]
  enabled     = true
  webhook_url = "https://hooks.slack.com/services/..."
  bot_token   = "xoxb-..."
  channel     = "#infra-alerts"
```

---

## Ollama Integration

NeuKeiho can optionally use a locally running [Ollama](https://ollama.com) instance to:

1. **Enrich alerts** — instead of raw threshold messages, Ollama generates a plain-English explanation of what's likely happening and what to check.
2. **Answer questions** — from CLI (`neukeiho ask`) or via the conversational bot endpoint.

Ollama runs entirely on the controller node. No internet connection is required. NeuKeiho feeds it only your live infra metrics — it has no access to anything outside.

**Enable Ollama:**
```ini
[ollama]
enabled = true
host    = localhost
port    = 11434
model   = llama3
```

**Ask from CLI:**
```bash
neukeiho ask "Which node is under the most pressure right now?"
neukeiho ask "Is db-01 likely to run out of disk in the next hour?"
```

**Ask from Slack:**  
Reply to any NeuKeiho alert in Slack and the bot will route your question to Ollama and reply in the same thread.

---

## Conversational Bot Endpoint

NeuKeiho exposes `POST /bot/message` — a unified inbound handler for all chat platforms.

```json
{
  "platform": "slack",
  "user_id":  "U012AB3CD",
  "text":     "What is causing the CPU spike on web-01?",
  "reply_to": "#infra-alerts"
}
```

Configure your Slack/Teams/Discord bot to forward messages to this endpoint.

---

## Project Structure

```
neukeiho/
├── cmd/neukeiho/              # Controller entrypoint
├── internal/
│   ├── collector/             # Metrics ingestion from agents
│   ├── threshold/             # Threshold evaluation engine
│   ├── alerter/               # Slack, PagerDuty, Email, Webhook
│   ├── ollama/                # Ollama bridge + context manager
│   └── bot/                   # Inbound chat message handler
├── deploy/ansible/            # Agent deployment playbook
├── config/                    # Example config files
└── README.md
```

---

## Roadmap

- [x] v0.1 — Controller + agent ingestion + threshold alerting (Slack)
- [ ] v0.2 — PagerDuty + Email + Webhook + `test-alert`
- [ ] v0.3 — Ollama bridge + `neukeiho ask` + conversational bot
- [ ] v0.4 — `neukeiho init` interactive TUI
- [ ] v0.5 — `neukeiho status` live dashboard TUI
- [ ] v1.0 — Stable release, full documentation

---

## Related

- [neukeiho-agent](https://github.com/meomkarjagtap/neukeiho-agent) — lightweight agent binary deployed to monitored nodes
- [NeuRader](https://github.com/neurader/neurader) — Ansible execution observability

---

## License

MIT
