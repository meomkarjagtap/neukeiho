package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/meomkarjagtap/neukeiho/internal/collector"
)

// Config holds Ollama connection settings from neukeiho.conf.
type Config struct {
	Enabled bool   `ini:"enabled"`
	Host    string `ini:"host"`
	Port    int    `ini:"port"`
	Model   string `ini:"model"`
}

// Bridge connects NeuKeiho to a local Ollama instance.
type Bridge struct {
	cfg Config
}

// New creates a new Ollama Bridge.
func New(cfg Config) *Bridge {
	return &Bridge{cfg: cfg}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

// Ask sends a question to Ollama with current infra context injected.
func (b *Bridge) Ask(question string, snapshot map[string]collector.MetricsPayload) (string, error) {
	if !b.cfg.Enabled {
		return "", fmt.Errorf("ollama is not enabled in neukeiho.conf")
	}

	context := b.buildContext(snapshot)
	prompt := fmt.Sprintf(`You are NeuKeiho's infra intelligence layer.
You only know about the infrastructure described below. Do not make up information.
Answer the user's question based only on this data.

--- CURRENT INFRA STATE ---
%s
--- END STATE ---

Question: %s`, context, question)

	body, _ := json.Marshal(ollamaRequest{
		Model:  b.cfg.Model,
		Prompt: prompt,
		Stream: false,
	})

	url := fmt.Sprintf("http://%s:%d/api/generate", b.cfg.Host, b.cfg.Port)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama response parse error: %w", err)
	}

	return result.Response, nil
}

// EnrichAlert asks Ollama to generate a human-readable explanation for an alert.
func (b *Bridge) EnrichAlert(rawAlert string, snapshot map[string]collector.MetricsPayload) string {
	if !b.cfg.Enabled {
		return rawAlert
	}

	question := fmt.Sprintf(
		"This alert just fired: \"%s\". In plain English, what could be causing this and what should the ops team check first?",
		rawAlert,
	)

	enriched, err := b.Ask(question, snapshot)
	if err != nil {
		fmt.Printf("[ollama] enrichment failed: %v\n", err)
		return rawAlert
	}

	return fmt.Sprintf("%s\n\n🤖 Ollama Analysis:\n%s", rawAlert, enriched)
}

// buildContext serialises the current infra snapshot into a readable string for the LLM prompt.
func (b *Bridge) buildContext(snapshot map[string]collector.MetricsPayload) string {
	var sb strings.Builder
	for nodeID, m := range snapshot {
		sb.WriteString(fmt.Sprintf(
			"Node: %s | CPU: %.1f%% | Memory: %.1f%% | Disk: %.1f%% | NetRx: %.1f Mbps | NetTx: %.1f Mbps | Last seen: %s\n",
			nodeID, m.CPU, m.Memory, m.Disk, m.NetworkRx, m.NetworkTx, m.Timestamp.Format("15:04:05"),
		))
	}
	return sb.String()
}
