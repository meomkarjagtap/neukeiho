package bot

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/meomkarjagtap/neukeiho/internal/collector"
	"github.com/meomkarjagtap/neukeiho/internal/ollama"
)

// InboundMessage represents a question received from any chat platform.
type InboundMessage struct {
	Platform string `json:"platform"` // slack | teams | discord | webhook
	UserID   string `json:"user_id"`
	Text     string `json:"text"`
	ReplyTo  string `json:"reply_to"` // channel or thread ID to respond to
}

// Listener handles inbound messages from chat platforms and routes them to Ollama.
type Listener struct {
	ollama    *ollama.Bridge
	collector *collector.Collector
	send      func(platform, replyTo, message string) error
}

// New creates a new bot Listener.
func New(ol *ollama.Bridge, col *collector.Collector) *Listener {
	return &Listener{
		ollama:    ol,
		collector: col,
	}
}

// WithSender sets the function used to send replies back to the chat platform.
func (l *Listener) WithSender(fn func(platform, replyTo, message string) error) *Listener {
	l.send = fn
	return l
}

// ServeHTTP handles POST /bot/message — a unified inbound endpoint for all platforms.
func (l *Listener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg InboundMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	go l.handle(msg)
	fmt.Fprintf(w, `{"status":"processing"}`)
}

func (l *Listener) handle(msg InboundMessage) {
	snapshot := l.collector.Latest()

	answer, err := l.ollama.Ask(msg.Text, snapshot)
	if err != nil {
		answer = fmt.Sprintf("NeuKeiho: Ollama is not available. Error: %v", err)
	}

	reply := fmt.Sprintf("🔍 *NeuKeiho:* %s", answer)

	if l.send != nil {
		if err := l.send(msg.Platform, msg.ReplyTo, reply); err != nil {
			fmt.Printf("[bot] failed to send reply: %v\n", err)
		}
	} else {
		fmt.Printf("[bot] reply (no sender configured): %s\n", reply)
	}
}
