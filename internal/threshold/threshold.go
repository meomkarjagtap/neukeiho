package threshold

import (
	"fmt"
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

// Breach represents a threshold violation.
type Breach struct {
	NodeID    string
	Metric    string
	Value     float64
	Threshold float64
	At        time.Time
	Message   string
}

// Engine evaluates metrics against configured thresholds.
type Engine struct {
	cfg     Config
	OnBreach func(b Breach)
}

// New creates a new threshold Engine.
func New() *Engine {
	return &Engine{
		cfg: Config{
			CPUPercent:    85,
			MemoryPercent: 80,
			DiskPercent:   90,
			NetworkMbps:   500,
		},
	}
}

// WithConfig sets thresholds from loaded config.
func (e *Engine) WithConfig(cfg Config) *Engine {
	e.cfg = cfg
	return e
}

// Evaluate checks a metrics payload against thresholds and calls OnBreach for each violation.
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
		if c.value >= c.threshold {
			if e.OnBreach != nil {
				e.OnBreach(Breach{
					NodeID:    p.NodeID,
					Metric:    c.metric,
					Value:     c.value,
					Threshold: c.threshold,
					At:        p.Timestamp,
					Message: fmt.Sprintf(
						"[NeuKeiho] ALERT: %s on node %s is %.1f%% (threshold: %.1f%%)",
						c.metric, p.NodeID, c.value, c.threshold,
					),
				})
			}
		}
	}
}
