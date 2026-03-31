package reports

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"netwatcher-controller/internal/probe"
	"netwatcher-controller/internal/workspace"
)

type Generator struct {
	db *gorm.DB
	ch *sql.DB
}

func NewGenerator(db *gorm.DB, ch *sql.DB) *Generator {
	return &Generator{db: db, ch: ch}
}

type WorkspaceSummary struct {
	Name            string
	ReportPeriod    string
	GeneratedAt     time.Time
	OverallHealth   float64
	Grade           string
	AgentCount      int
	OnlineAgents    int
	ProbeCount      int
	IncidentCount   int
	LatencyScore    float64
	PacketLossScore float64
	RouteStability  float64
	MOSScore        float64
}

type AgentStatus struct {
	Name        string
	IP          string
	Status      string
	HealthScore float64
	LastSeen    time.Time
}

type ProbeMetric struct {
	Name       string
	Type       string
	Target     string
	AvgLatency float64
	PacketLoss float64
	Uptime     float64
}

type AlertEvent struct {
	Timestamp   time.Time
	ProbeName   string
	Target      string
	Description string
	Severity    string
}

func (g *Generator) GenerateWorkspacePDF(ctx context.Context, config *ReportConfig, configJSON ReportConfigJSON) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	summary, err := g.fetchWorkspaceSummary(ctx, config.WorkspaceID, configJSON.TimeRangeDays)
	if err != nil {
		log.Warnf("[reports] failed to fetch workspace summary: %v", err)
		summary = &WorkspaceSummary{Name: "Unknown", GeneratedAt: time.Now()}
	}

	agents, err := g.fetchAgentStatuses(ctx, config.WorkspaceID)
	if err != nil {
		log.Warnf("[reports] failed to fetch agent statuses: %v", err)
	}

	metrics, err := g.fetchProbeMetrics(ctx, config.WorkspaceID, configJSON)
	if err != nil {
		log.Warnf("[reports] failed to fetch probe metrics: %v", err)
	}

	g.renderCoverPage(pdf, summary)
	g.renderExecutiveSummary(pdf, summary)
	g.renderAgentStatus(pdf, agents)
	g.renderProbeMetrics(pdf, metrics)

	if configJSON.IncludeAlerts {
		alerts, err := g.fetchAlertEvents(ctx, config.WorkspaceID, configJSON.TimeRangeDays)
		if err == nil {
			g.renderAlertHistory(pdf, alerts)
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output failed: %w", err)
	}

	return buf.Bytes(), nil
}

func (g *Generator) fetchWorkspaceSummary(ctx context.Context, workspaceID uint, days int64) (*WorkspaceSummary, error) {
	if days <= 0 {
		days = 7
	}
	from := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	var ws workspace.Workspace
	if err := g.db.WithContext(ctx).First(&ws, workspaceID).Error; err != nil {
		return nil, err
	}

	snapshots, err := probe.GetAnalysisSnapshots(ctx, g.ch, workspaceID, from, time.Now().UTC(), 100)
	if err != nil || len(snapshots) == 0 {
		return &WorkspaceSummary{
			Name:         ws.Name,
			ReportPeriod: fmt.Sprintf("Last %d days", days),
			GeneratedAt:  time.Now(),
			Grade:        "N/A",
		}, nil
	}

	latest := snapshots[0]
	summary := &WorkspaceSummary{
		Name:            ws.Name,
		ReportPeriod:    fmt.Sprintf("%s to %s", from.Format("Jan 2, 2006"), time.Now().Format("Jan 2, 2006")),
		GeneratedAt:     time.Now(),
		OverallHealth:   latest.OverallHealth,
		Grade:           latest.Grade,
		AgentCount:      int(latest.TotalAgents),
		OnlineAgents:    int(latest.OnlineAgents),
		ProbeCount:      int(latest.TotalProbes),
		IncidentCount:   int(latest.IncidentCount),
		LatencyScore:    latest.LatencyScore,
		PacketLossScore: latest.PacketLossScore,
		RouteStability:  latest.RouteStability,
		MOSScore:        latest.MosScore,
	}

	return summary, nil
}

func (g *Generator) fetchAgentStatuses(ctx context.Context, workspaceID uint) ([]AgentStatus, error) {
	type Agent struct {
		ID        uint      `json:"id"`
		Name      string    `json:"name"`
		IPAddress string    `json:"ip_address"`
		LastSeen  time.Time `json:"last_seen"`
		Status    string    `json:"status"`
	}

	var agents []Agent
	err := g.db.WithContext(ctx).
		Table("agents").
		Select("agents.id, agents.name, agents.ip_address, agents.last_seen, 'online' as status").
		Where("workspace_id = ?", workspaceID).
		Find(&agents).Error
	if err != nil {
		return nil, err
	}

	statuses := make([]AgentStatus, len(agents))
	for i, a := range agents {
		status := "online"
		if a.LastSeen.Before(time.Now().UTC().Add(-5 * time.Minute)) {
			status = "offline"
		}
		statuses[i] = AgentStatus{
			Name:        a.Name,
			IP:          a.IPAddress,
			Status:      status,
			HealthScore: 100,
			LastSeen:    a.LastSeen,
		}
	}
	return statuses, nil
}

func (g *Generator) fetchProbeMetrics(ctx context.Context, workspaceID uint, configJSON ReportConfigJSON) ([]ProbeMetric, error) {
	type Probe struct {
		ID     uint   `json:"id"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Target string `json:"target"`
	}

	var probes []Probe
	query := g.db.WithContext(ctx).Table("probes").Select("id, name, type, target").Where("workspace_id = ?", workspaceID)
	if len(configJSON.ProbeIDs) > 0 {
		query = query.Where("id IN ?", configJSON.ProbeIDs)
	}
	if err := query.Find(&probes).Error; err != nil {
		return nil, err
	}

	metrics := make([]ProbeMetric, len(probes))
	for i, p := range probes {
		target := p.Target
		if target == "" {
			target = "N/A"
		}
		metrics[i] = ProbeMetric{
			Name:       p.Name,
			Type:       p.Type,
			Target:     target,
			AvgLatency: 0,
			PacketLoss: 0,
			Uptime:     100,
		}
	}
	return metrics, nil
}

func (g *Generator) fetchAlertEvents(ctx context.Context, workspaceID uint, days int64) ([]AlertEvent, error) {
	if days <= 0 {
		days = 7
	}
	from := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	type Alert struct {
		ID          uint      `json:"id"`
		WorkspaceID uint      `json:"workspace_id"`
		RuleName    string    `json:"rule_name"`
		Description string    `json:"description"`
		Severity    string    `json:"severity"`
		FiredAt     time.Time `json:"fired_at"`
	}

	var alerts []Alert
	err := g.db.WithContext(ctx).
		Table("alerts").
		Select("id, workspace_id, rule_name, description, severity, fired_at").
		Where("workspace_id = ? AND fired_at >= ?", workspaceID, from).
		Order("fired_at DESC").
		Limit(50).
		Find(&alerts).Error
	if err != nil {
		return nil, err
	}

	events := make([]AlertEvent, len(alerts))
	for i, a := range alerts {
		events[i] = AlertEvent{
			Timestamp:   a.FiredAt,
			ProbeName:   a.RuleName,
			Target:      "N/A",
			Description: a.Description,
			Severity:    a.Severity,
		}
	}
	return events, nil
}

func (g *Generator) renderCoverPage(pdf *gofpdf.Fpdf, summary *WorkspaceSummary) {
	pdf.SetFont("Arial", "B", 28)
	pdf.SetTextColor(26, 54, 93)
	pdf.Ln(40)
	pdf.Cell(0, 15, "NetWatcher")
	pdf.Ln(15)

	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(50, 50, 50)
	pdf.Cell(0, 10, "Workspace Report")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 14)
	pdf.SetTextColor(80, 80, 80)
	pdf.Cell(0, 8, summary.Name)
	pdf.Ln(8)

	pdf.SetFont("Arial", "", 11)
	pdf.Cell(0, 6, fmt.Sprintf("Report Period: %s", summary.ReportPeriod))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Generated: %s", summary.GeneratedAt.Format("Jan 2, 2006 15:04 UTC")))
	pdf.Ln(20)

	pdf.SetDrawColor(0, 136, 204)
	pdf.SetLineWidth(0.5)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(10)
}

func (g *Generator) renderExecutiveSummary(pdf *gofpdf.Fpdf, summary *WorkspaceSummary) {
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 8, "Executive Summary")
	pdf.Ln(8)

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(50, 50, 50)

	metrics := [][2]string{
		{"Overall Health", fmt.Sprintf("%.1f%% (%s)", summary.OverallHealth, summary.Grade)},
		{"Agents", fmt.Sprintf("%d / %d online", summary.OnlineAgents, summary.AgentCount)},
		{"Probes", fmt.Sprintf("%d configured", summary.ProbeCount)},
		{"Incidents", fmt.Sprintf("%d detected", summary.IncidentCount)},
		{"Latency Score", fmt.Sprintf("%.1f / 100", summary.LatencyScore)},
		{"Packet Loss Score", fmt.Sprintf("%.1f / 100", summary.PacketLossScore)},
		{"Route Stability", fmt.Sprintf("%.1f / 100", summary.RouteStability)},
		{"MOS Score", fmt.Sprintf("%.1f / 5.0", summary.MOSScore)},
	}

	for _, m := range metrics {
		pdf.SetFont("Arial", "B", 10)
		pdf.Cell(50, 6, m[0]+":")
		pdf.SetFont("Arial", "", 10)
		pdf.Cell(0, 6, m[1])
		pdf.Ln(6)
	}

	pdf.Ln(10)
}

func (g *Generator) renderAgentStatus(pdf *gofpdf.Fpdf, agents []AgentStatus) {
	if len(agents) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 8, "Agent Status")
	pdf.Ln(8)

	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(240, 240, 240)
	pdf.SetTextColor(50, 50, 50)
	pdf.CellFormat(60, 7, "Name", "1", 0, "L", true, 0, "")
	pdf.CellFormat(50, 7, "IP Address", "1", 0, "L", true, 0, "")
	pdf.CellFormat(30, 7, "Status", "1", 0, "L", true, 0, "")
	pdf.CellFormat(30, 7, "Last Seen", "1", 1, "L", true, 0, "")

	pdf.SetFont("Arial", "", 9)
	for _, a := range agents {
		statusColor := [3]int{0, 136, 0}
		if a.Status == "offline" {
			statusColor = [3]int{180, 0, 0}
		}

		name := a.Name
		if name == "" {
			name = "Unnamed"
		}

		pdf.CellFormat(60, 6, truncate(name, 30), "1", 0, "L", false, 0, "")
		pdf.CellFormat(50, 6, truncate(a.IP, 25), "1", 0, "L", false, 0, "")
		pdf.SetTextColor(statusColor[0], statusColor[1], statusColor[2])
		pdf.CellFormat(30, 6, strings.ToUpper(a.Status), "1", 0, "L", false, 0, "")
		pdf.SetTextColor(50, 50, 50)
		pdf.CellFormat(30, 6, a.LastSeen.Format("15:04"), "1", 1, "L", false, 0, "")
	}

	pdf.Ln(10)
}

func (g *Generator) renderProbeMetrics(pdf *gofpdf.Fpdf, metrics []ProbeMetric) {
	if len(metrics) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 8, "Probe Metrics")
	pdf.Ln(8)

	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(240, 240, 240)
	pdf.SetTextColor(50, 50, 50)
	pdf.CellFormat(50, 7, "Probe Name", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 7, "Type", "1", 0, "L", true, 0, "")
	pdf.CellFormat(55, 7, "Target", "1", 0, "L", true, 0, "")
	pdf.CellFormat(30, 7, "Avg Latency", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 7, "Loss %", "1", 1, "L", true, 0, "")

	pdf.SetFont("Arial", "", 9)
	for _, m := range metrics {
		pdf.CellFormat(50, 6, truncate(m.Name, 28), "1", 0, "L", false, 0, "")
		pdf.CellFormat(25, 6, m.Type, "1", 0, "L", false, 0, "")
		pdf.CellFormat(55, 6, truncate(m.Target, 30), "1", 0, "L", false, 0, "")
		pdf.CellFormat(30, 6, fmt.Sprintf("%.1f ms", m.AvgLatency), "1", 0, "L", false, 0, "")
		pdf.CellFormat(25, 6, fmt.Sprintf("%.2f%%", m.PacketLoss), "1", 1, "L", false, 0, "")
	}

	pdf.Ln(10)
}

func (g *Generator) renderAlertHistory(pdf *gofpdf.Fpdf, alerts []AlertEvent) {
	if len(alerts) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 8, "Recent Alerts")
	pdf.Ln(8)

	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(240, 240, 240)
	pdf.SetTextColor(50, 50, 50)
	pdf.CellFormat(40, 7, "Time", "1", 0, "L", true, 0, "")
	pdf.CellFormat(40, 7, "Rule", "1", 0, "L", true, 0, "")
	pdf.CellFormat(30, 7, "Severity", "1", 0, "L", true, 0, "")
	pdf.CellFormat(60, 7, "Description", "1", 1, "L", true, 0, "")

	pdf.SetFont("Arial", "", 9)
	for i, a := range alerts {
		if i >= 20 {
			pdf.CellFormat(0, 6, "... and more alerts", "", 1, "L", false, 0, "")
			break
		}

		sevColor := [3]int{50, 50, 50}
		if a.Severity == "critical" {
			sevColor = [3]int{180, 0, 0}
		} else if a.Severity == "warning" {
			sevColor = [3]int{180, 100, 0}
		}

		pdf.CellFormat(40, 6, a.Timestamp.Format("Jan 2 15:04"), "1", 0, "L", false, 0, "")
		pdf.CellFormat(40, 6, truncate(a.ProbeName, 25), "1", 0, "L", false, 0, "")
		pdf.SetTextColor(sevColor[0], sevColor[1], sevColor[2])
		pdf.CellFormat(30, 6, strings.ToUpper(a.Severity), "1", 0, "L", false, 0, "")
		pdf.SetTextColor(50, 50, 50)
		pdf.CellFormat(60, 6, truncate(a.Description, 35), "1", 1, "L", false, 0, "")
	}

	pdf.Ln(10)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
