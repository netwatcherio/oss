package metrics

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gorm.io/gorm"
)

type Metrics struct {
	AgentStatus    *prometheus.GaugeVec
	AgentLastSeen  *prometheus.GaugeVec
	ProbeStatus    *prometheus.GaugeVec
	AlertStatus    *prometheus.GaugeVec
	WorkspaceCount prometheus.Gauge
	ProbeDataTotal *prometheus.CounterVec
	HTTPRequestDur *prometheus.HistogramVec
}

var (
	global *Metrics
	once   sync.Once
)

func New(db *gorm.DB, ch *sql.DB) *Metrics {
	once.Do(func() {
		global = &Metrics{
			AgentStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "netwatcher",
				Subsystem: "agents",
				Name:      "status",
				Help:      "Number of agents by status (online/stale/offline)",
			}, []string{"workspace_id", "status"}),

			AgentLastSeen: promauto.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "netwatcher",
				Subsystem: "agents",
				Name:      "last_seen_unix",
				Help:      "Last seen timestamp of agents in unix seconds",
			}, []string{"agent_id", "workspace_id"}),

			ProbeStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "netwatcher",
				Subsystem: "probes",
				Name:      "status",
				Help:      "Number of probes by type and enabled state",
			}, []string{"workspace_id", "agent_id", "type", "enabled"}),

			AlertStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "netwatcher",
				Subsystem: "alerts",
				Name:      "status",
				Help:      "Number of alerts by status (active/acknowledged/resolved)",
			}, []string{"workspace_id", "status"}),

			WorkspaceCount: promauto.NewGauge(prometheus.GaugeOpts{
				Namespace: "netwatcher",
				Name:      "workspaces_total",
				Help:      "Total number of workspaces",
			}),

			ProbeDataTotal: promauto.NewCounterVec(prometheus.CounterOpts{
				Namespace: "netwatcher",
				Subsystem: "probe_data",
				Name:      "total",
				Help:      "Total number of probe data records ingested",
			}, []string{"type"}),

			HTTPRequestDur: promauto.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "netwatcher",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			}, []string{"method", "path", "status"}),
		}
	})
	return global
}

func Get() *Metrics {
	return global
}

func (m *Metrics) RegisterCollectors(db *gorm.DB, ch *sql.DB) {
	go m.collectLoop(db, ch)
}

func (m *Metrics) collectLoop(db *gorm.DB, ch *sql.DB) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	m.collect(db, ch)

	for range ticker.C {
		m.collect(db, ch)
	}
}

func (m *Metrics) collect(db *gorm.DB, ch *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	m.collectAgentMetrics(ctx, db)
	m.collectProbeMetrics(ctx, db)
	m.collectAlertMetrics(ctx, db)
	m.collectWorkspaceMetrics(ctx, db)
}

func (m *Metrics) collectAgentMetrics(ctx context.Context, db *gorm.DB) {
	type AgentRow struct {
		WorkspaceID uint   `gorm:"column:workspace_id"`
		Status      string `gorm:"column:status"`
		Count       int64  `gorm:"column:count"`
	}

	var rows []AgentRow
	if err := db.WithContext(ctx).Raw(`
		SELECT w.id as workspace_id,
			CASE
				WHEN a.last_seen_at > NOW() - INTERVAL '1 minute' THEN 'online'
				WHEN a.last_seen_at > NOW() - INTERVAL '5 minutes' THEN 'stale'
				ELSE 'offline'
			END as status,
			COUNT(*) as count
		FROM agents a
		JOIN workspaces w ON w.id = a.workspace_id
		WHERE a.deleted_at IS NULL
		GROUP BY w.id, status
	`).Scan(&rows).Error; err != nil {
		return
	}

	m.AgentStatus.Reset()
	for _, r := range rows {
		m.AgentStatus.WithLabelValues(
			formatUint(r.WorkspaceID), r.Status,
		).Set(float64(r.Count))
	}

	type LastSeenRow struct {
		AgentID     uint      `gorm:"column:agent_id"`
		WorkspaceID uint      `gorm:"column:workspace_id"`
		LastSeenAt  time.Time `gorm:"column:last_seen_at"`
	}
	var lastSeen []LastSeenRow
	if err := db.WithContext(ctx).Raw(`
		SELECT id as agent_id, workspace_id, last_seen_at
		FROM agents
		WHERE deleted_at IS NULL
	`).Scan(&lastSeen).Error; err != nil {
		return
	}

	for _, ls := range lastSeen {
		m.AgentLastSeen.WithLabelValues(
			formatUint(ls.AgentID),
			formatUint(ls.WorkspaceID),
		).Set(float64(ls.LastSeenAt.Unix()))
	}
}

func (m *Metrics) collectProbeMetrics(ctx context.Context, db *gorm.DB) {
	type ProbeRow struct {
		WorkspaceID uint   `gorm:"column:workspace_id"`
		AgentID     uint   `gorm:"column:agent_id"`
		Type        string `gorm:"column:type"`
		Enabled     bool   `gorm:"column:enabled"`
		Count       int64  `gorm:"column:count"`
	}

	var rows []ProbeRow
	if err := db.WithContext(ctx).Raw(`
		SELECT p.workspace_id, p.agent_id, p.type, p.enabled, COUNT(*) as count
		FROM probes p
		WHERE p.deleted_at IS NULL
		GROUP BY p.workspace_id, p.agent_id, p.type, p.enabled
	`).Scan(&rows).Error; err != nil {
		return
	}

	m.ProbeStatus.Reset()
	for _, r := range rows {
		enabledStr := "disabled"
		if r.Enabled {
			enabledStr = "enabled"
		}
		m.ProbeStatus.WithLabelValues(
			formatUint(r.WorkspaceID),
			formatUint(r.AgentID),
			r.Type,
			enabledStr,
		).Set(float64(r.Count))
	}
}

func (m *Metrics) collectAlertMetrics(ctx context.Context, db *gorm.DB) {
	type AlertRow struct {
		WorkspaceID uint   `gorm:"column:workspace_id"`
		Status      string `gorm:"column:status"`
		Count       int64  `gorm:"column:count"`
	}

	var rows []AlertRow
	if err := db.WithContext(ctx).Raw(`
		SELECT workspace_id, status, COUNT(*) as count
		FROM alerts
		GROUP BY workspace_id, status
	`).Scan(&rows).Error; err != nil {
		return
	}

	m.AlertStatus.Reset()
	for _, r := range rows {
		m.AlertStatus.WithLabelValues(
			formatUint(r.WorkspaceID),
			r.Status,
		).Set(float64(r.Count))
	}
}

func (m *Metrics) collectWorkspaceMetrics(ctx context.Context, db *gorm.DB) {
	var count int64
	if err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM workspaces WHERE deleted_at IS NULL`).Scan(&count).Error; err != nil {
		return
	}
	m.WorkspaceCount.Set(float64(count))
}

func formatUint(v uint) string {
	return formatUintImpl(v)
}

func formatUintImpl(v uint) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0') + byte(v%10)
		v /= 10
	}
	return string(b[i:])
}
