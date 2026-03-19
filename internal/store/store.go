package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/meomkarjagtap/neukeiho/internal/collector"
	"github.com/meomkarjagtap/neukeiho/internal/threshold"
)

// Incident represents a fired alert stored in the DB.
type Incident struct {
	ID             int64
	NodeID         string
	Metric         string
	Value          float64
	Threshold      float64
	OllamaAnalysis string
	JiraTicket     string
	FiredAt        time.Time
	ResolvedAt     *time.Time
	Resolution     string
}

// Store wraps a SQLite database.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at path.
func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite3", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// migrate creates tables if they don't exist.
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS metrics (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id   TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			cpu       REAL,
			memory    REAL,
			disk      REAL,
			net_rx    REAL,
			net_tx    REAL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_node_time
			ON metrics(node_id, timestamp)`,

		`CREATE TABLE IF NOT EXISTS baselines (
			node_id      TEXT PRIMARY KEY,
			cpu_avg      REAL,
			cpu_max      REAL,
			memory_avg   REAL,
			disk_avg     REAL,
			updated_at   DATETIME
		)`,

		`CREATE TABLE IF NOT EXISTS incidents (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id         TEXT NOT NULL,
			metric          TEXT NOT NULL,
			value           REAL,
			threshold       REAL,
			ollama_analysis TEXT,
			jira_ticket     TEXT,
			fired_at        DATETIME,
			resolved_at     DATETIME,
			resolution      TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS conversations (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			incident_id INTEGER,
			role        TEXT,
			message     TEXT,
			timestamp   DATETIME
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
	}
	return nil
}

// WriteMetrics persists a metrics payload.
func (s *Store) WriteMetrics(p collector.MetricsPayload) error {
	_, err := s.db.Exec(
		`INSERT INTO metrics(node_id,timestamp,cpu,memory,disk,net_rx,net_tx)
		 VALUES(?,?,?,?,?,?,?)`,
		p.NodeID, p.Timestamp, p.CPU, p.Memory, p.Disk, p.NetworkRx, p.NetworkTx,
	)
	return err
}

// QueryMetrics returns metrics for a node within a time range.
func (s *Store) QueryMetrics(nodeID string, from, to time.Time) ([]collector.MetricsPayload, error) {
	rows, err := s.db.Query(
		`SELECT node_id,timestamp,cpu,memory,disk,net_rx,net_tx
		 FROM metrics WHERE node_id=? AND timestamp BETWEEN ? AND ?
		 ORDER BY timestamp ASC`,
		nodeID, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []collector.MetricsPayload
	for rows.Next() {
		var p collector.MetricsPayload
		if err := rows.Scan(&p.NodeID, &p.Timestamp, &p.CPU, &p.Memory, &p.Disk, &p.NetworkRx, &p.NetworkTx); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// CreateIncident stores a new incident and returns its ID.
func (s *Store) CreateIncident(b threshold.Breach, ollamaText string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO incidents(node_id,metric,value,threshold,ollama_analysis,fired_at)
		 VALUES(?,?,?,?,?,?)`,
		b.NodeID, b.Metric, b.Value, b.Threshold, ollamaText, b.At,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateJiraTicket saves the Jira ticket reference against an incident.
func (s *Store) UpdateJiraTicket(incidentID int64, ticket string) error {
	_, err := s.db.Exec(
		`UPDATE incidents SET jira_ticket=? WHERE id=?`,
		ticket, incidentID,
	)
	return err
}

// ResolveIncident marks an incident as resolved.
func (s *Store) ResolveIncident(id int64, resolution string) error {
	_, err := s.db.Exec(
		`UPDATE incidents SET resolved_at=?, resolution=? WHERE id=?`,
		time.Now(), resolution, id,
	)
	return err
}

// QueryIncidents returns recent incidents for a node.
func (s *Store) QueryIncidents(nodeID string, days int) ([]Incident, error) {
	rows, err := s.db.Query(
		`SELECT id,node_id,metric,value,threshold,ollama_analysis,
		        COALESCE(jira_ticket,''),fired_at,resolved_at,COALESCE(resolution,'')
		 FROM incidents
		 WHERE node_id=? AND fired_at > datetime('now',?)
		 ORDER BY fired_at DESC`,
		nodeID, fmt.Sprintf("-%d days", days),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Incident
	for rows.Next() {
		var i Incident
		var resolvedAt sql.NullTime
		if err := rows.Scan(
			&i.ID, &i.NodeID, &i.Metric, &i.Value, &i.Threshold,
			&i.OllamaAnalysis, &i.JiraTicket, &i.FiredAt, &resolvedAt, &i.Resolution,
		); err != nil {
			return nil, err
		}
		if resolvedAt.Valid {
			i.ResolvedAt = &resolvedAt.Time
		}
		out = append(out, i)
	}
	return out, nil
}

// SaveConversation stores a bot conversation message.
func (s *Store) SaveConversation(incidentID int64, role, message string) error {
	_, err := s.db.Exec(
		`INSERT INTO conversations(incident_id,role,message,timestamp) VALUES(?,?,?,?)`,
		incidentID, role, message, time.Now(),
	)
	return err
}

// BuildOllamaContext assembles a context string for Ollama from stored data.
func (s *Store) BuildOllamaContext(nodeID string, latest map[string]collector.MetricsPayload) string {
	ctx := "=== CURRENT INFRA STATE ===\n"
	for id, m := range latest {
		ctx += fmt.Sprintf(
			"Node: %-20s CPU: %5.1f%%  Mem: %5.1f%%  Disk: %5.1f%%  NetRx: %6.1f Mbps  NetTx: %6.1f Mbps\n",
			id, m.CPU, m.Memory, m.Disk, m.NetworkRx, m.NetworkTx,
		)
	}

	// baseline for this node
	row := s.db.QueryRow(
		`SELECT cpu_avg,cpu_max,memory_avg FROM baselines WHERE node_id=?`, nodeID,
	)
	var cpuAvg, cpuMax, memAvg float64
	if err := row.Scan(&cpuAvg, &cpuMax, &memAvg); err == nil {
		ctx += fmt.Sprintf(
			"\n=== BASELINE for %s ===\nCPU avg: %.1f%%  CPU max: %.1f%%  Memory avg: %.1f%%\n",
			nodeID, cpuAvg, cpuMax, memAvg,
		)
	}

	// recent incidents
	incidents, err := s.QueryIncidents(nodeID, 30)
	if err == nil && len(incidents) > 0 {
		ctx += fmt.Sprintf("\n=== PAST INCIDENTS for %s (last 30 days) ===\n", nodeID)
		for _, inc := range incidents {
			status := "OPEN"
			if inc.ResolvedAt != nil {
				status = "RESOLVED"
			}
			ctx += fmt.Sprintf(
				"[%s] %s %s=%.1f%% at %s\n",
				status, inc.NodeID, inc.Metric, inc.Value, inc.FiredAt.Format("2006-01-02 15:04"),
			)
		}
	}

	return ctx
}

// RecalculateBaseline computes and stores a fresh baseline for a node.
func (s *Store) RecalculateBaseline(nodeID string) error {
	row := s.db.QueryRow(
		`SELECT AVG(cpu), MAX(cpu), AVG(memory)
		 FROM metrics
		 WHERE node_id=? AND timestamp > datetime('now','-7 days')`,
		nodeID,
	)
	var cpuAvg, cpuMax, memAvg float64
	if err := row.Scan(&cpuAvg, &cpuMax, &memAvg); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO baselines(node_id,cpu_avg,cpu_max,memory_avg,updated_at)
		 VALUES(?,?,?,?,?)
		 ON CONFLICT(node_id) DO UPDATE SET
		   cpu_avg=excluded.cpu_avg,
		   cpu_max=excluded.cpu_max,
		   memory_avg=excluded.memory_avg,
		   updated_at=excluded.updated_at`,
		nodeID, cpuAvg, cpuMax, memAvg, time.Now(),
	)
	return err
}

// Purge deletes metrics older than retentionDays.
func (s *Store) Purge(retentionDays int) error {
	_, err := s.db.Exec(
		`DELETE FROM metrics WHERE timestamp < datetime('now',?)`,
		fmt.Sprintf("-%d days", retentionDays),
	)
	return err
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
