package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// MetricsPayload is what each neukeiho-agent POSTs to the controller.
type MetricsPayload struct {
	NodeID    string    `json:"node_id"`
	Timestamp time.Time `json:"timestamp"`
	CPU       float64   `json:"cpu_percent"`
	Memory    float64   `json:"memory_percent"`
	Disk      float64   `json:"disk_percent"`
	NetworkRx float64   `json:"network_rx_mbps"`
	NetworkTx float64   `json:"network_tx_mbps"`
}

// Collector holds the latest metrics for each node.
type Collector struct {
	mu        sync.RWMutex
	latest    map[string]MetricsPayload
	history   map[string][]MetricsPayload
	OnMetrics func(payload MetricsPayload) // called on every incoming payload
}

// New creates a new Collector.
func New() *Collector {
	return &Collector{
		latest:  make(map[string]MetricsPayload),
		history: make(map[string][]MetricsPayload),
	}
}

// ServeHTTP handles POST /metrics from agents.
func (c *Collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload MetricsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if payload.NodeID == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}

	if payload.Timestamp.IsZero() {
		payload.Timestamp = time.Now()
	}

	c.mu.Lock()
	c.latest[payload.NodeID] = payload
	c.history[payload.NodeID] = append(c.history[payload.NodeID], payload)
	// keep last 1000 samples per node in memory
	if len(c.history[payload.NodeID]) > 1000 {
		c.history[payload.NodeID] = c.history[payload.NodeID][1:]
	}
	c.mu.Unlock()

	// call hook (store + threshold) outside the lock
	if c.OnMetrics != nil {
		c.OnMetrics(payload)
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok"}`)
}

// Latest returns the latest metrics snapshot for all nodes.
func (c *Collector) Latest() map[string]MetricsPayload {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]MetricsPayload, len(c.latest))
	for k, v := range c.latest {
		out[k] = v
	}
	return out
}

// History returns recent metric history for a node.
func (c *Collector) History(nodeID string, last int) []MetricsPayload {
	c.mu.RLock()
	defer c.mu.RUnlock()
	h := c.history[nodeID]
	if len(h) <= last {
		return h
	}
	return h[len(h)-last:]
}
