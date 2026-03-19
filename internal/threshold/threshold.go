package threshold

import (
	"fmt"
	"sync"
	"time"

	"github.com/meomkarjagtap/neukeiho/internal/collector"
)

// Config holds alert threshold values loaded from neukeiho.toml.
type Config struct {
	CPUPercent    float64 `toml:"cpu_percent"`
	MemoryPercent float64 `toml:"memory_percent"`
	DiskPercent   float64 `toml:"disk_percent"`
	NetworkMbps   float64 `toml:"network_mbps"`
}

// BreachType indicates whether this is a new breach or a recovery.
type BreachType string

const (
	BreachTypeAlert    BreachType = "ALERT"
	BreachTypeRecovery BreachType = "RECOVERY"
)

// Breach represents a threshold violation or recovery event.
type Breach struct {
	NodeID    string
	Metric    string
	Value     float64
	Threshold float64
	Type      BreachType
	At        time.Time
	Message   string
}

// breachState tracks whether a node+metric is currently in breach.
type breachState struct {
	inBreach bool
	since    time.Time
}

// Engine evaluates metrics against configured thresholds.
type Engine struct {
	cfg      Config
	mu       sync.Mutex
	states   map[string]*breachState // key: "node-01:CPU"
	OnBreach func(b Breach)
}

// New creates a new threshold Engine with sensible defaults.
func New() *Engine {
	return &Engine{
		cfg: Config{
			CPUPercent:    85,
			MemoryPercent: 80,
			DiskPercent:   90,
			NetworkMbps:   500,
		},
		states: make(map[string]*breachState),
	}
}

// WithConfig sets thresholds from loaded config.
func (e *Engine) WithConfig(cfg Config) *Engine {
	e.cfg = cfg
	return e
}

// Evaluate checks a metrics payload and fires OnBreach only on:
// - first breach (transition from normal → breach)
// - recovery (transition from breach → normal)
func (e *Engine) Evaluate(p collector.MetricsPayload) {
	checks := []struct {
		metric    string
		value     float64
		threshold float64
	}{
		{"CPU", p.CPU, e.cfg.CPUPercent},
		{"Memory", p.Memory, e.cfg.MemoryPercent},
		{"Disk", p.Disk, e.cfg.DiskPercent},
		{"NetworkRx", p.NetworkRx, e.cfg.NetworkMbps},
		{"NetworkTx", p.NetworkTx, e.cfg.NetworkMbps},
	}

	for _, c := range checks {
		key := fmt.Sprintf("%s:%s", p.NodeID, c.metric)

		e.mu.Lock()
		state, exists := e.states[key]
		if !exists {
			state = &breachState{}
			e.states[key] = state
		}

		isBreaching := c.value >= c.threshold

		var breach *Breach

		if isBreaching && !state.inBreach {
			// transition: normal → breach — fire ALERT
			state.inBreach = true
			state.since = p.Timestamp
			breach = &Breach{
				NodeID:    p.NodeID,
				Metric:    c.metric,
				Value:     c.value,
				Threshold: c.threshold,
				Type:      BreachTypeAlert,
				At:        p.Timestamp,
				Message: fmt.Sprintf(
					"🔴 [NeuKeiho] ALERT: %s on %s is %.1f%% (threshold: %.1f%%)",
					c.metric, p.NodeID, c.value, c.threshold,
				),
			}
		} else if !isBreaching && state.inBreach {
			// transition: breach → normal — fire RECOVERY
			duration := p.Timestamp.Sub(state.since).Round(time.Second)
			state.inBreach = false
			breach = &Breach{
				NodeID:    p.NodeID,
				Metric:    c.metric,
				Value:     c.value,
				Threshold: c.threshold,
				Type:      BreachTypeRecovery,
				At:        p.Timestamp,
				Message: fmt.Sprintf(
					"✅ [NeuKeiho] RECOVERY: %s on %s recovered to %.1f%% (threshold: %.1f%%) — was breached for %s",
					c.metric, p.NodeID, c.value, c.threshold, duration,
				),
			}
		}
		// if already breaching or already normal — do nothing

		e.mu.Unlock()

		if breach != nil && e.OnBreach != nil {
			e.OnBreach(*breach)
		}
	}
}
