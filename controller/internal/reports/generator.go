package reports

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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

type ProbeDetailSummary struct {
	Name          string
	ReportPeriod  string
	GeneratedAt   time.Time
	ProbeName     string
	ProbeType     string
	Target        string
	AgentName     string
	AgentIP       string
	OverallHealth float64
	Grade         string
	AvgLatency    float64
	P95Latency    float64
	PacketLoss    float64
	Jitter        float64
	Uptime        float64
	SampleCount   int
	MinLatency    float64
	MaxLatency    float64
}

type SLASummary struct {
	Name               string
	ReportPeriod       string
	GeneratedAt        time.Time
	OverallUptime      float64
	Grade              string
	TotalProbes        int
	ProbesMeetingSLO   int
	ProbesViolatingSLO int
	SLOTarget          float64
	LatencySLO         float64
	PacketLossSLO      float64
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
	switch config.ReportType {
	case ReportTypeProbeDetail:
		return g.generateProbeDetailPDF(ctx, config, configJSON)
	case ReportTypeSLA:
		return g.generateSLAPDF(ctx, config, configJSON)
	default:
		return g.generateWorkspaceSummaryPDF(ctx, config, configJSON)
	}
}

func (g *Generator) generateWorkspaceSummaryPDF(ctx context.Context, config *ReportConfig, configJSON ReportConfigJSON) ([]byte, error) {
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

func (g *Generator) generateProbeDetailPDF(ctx context.Context, config *ReportConfig, configJSON ReportConfigJSON) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	if len(configJSON.ProbeIDs) == 0 {
		g.renderNoProbeSelected(pdf)
		var buf bytes.Buffer
		pdf.Output(&buf)
		return buf.Bytes(), nil
	}

	for _, probeID := range configJSON.ProbeIDs {
		detail, err := g.fetchProbeDetail(ctx, config.WorkspaceID, probeID, configJSON.TimeRangeDays)
		if err != nil {
			log.Warnf("[reports] failed to fetch probe detail for probe %d: %v", probeID, err)
			continue
		}

		g.renderProbeDetailCover(pdf, detail)
		g.renderProbeDetailMetrics(pdf, detail)

		if configJSON.IncludeAlerts {
			alerts, err := g.fetchProbeAlerts(ctx, config.WorkspaceID, probeID, configJSON.TimeRangeDays)
			if err == nil && len(alerts) > 0 {
				g.renderAlertHistory(pdf, alerts)
			}
		}

		if len(configJSON.ProbeIDs) > 1 {
			pdf.AddPage()
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output failed: %w", err)
	}

	return buf.Bytes(), nil
}

func (g *Generator) generateSLAPDF(ctx context.Context, config *ReportConfig, configJSON ReportConfigJSON) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	slaTarget := 99.5
	if configJSON.SLATarget > 0 {
		slaTarget = configJSON.SLATarget
	}

	summary, probeDetails, err := g.fetchSLAData(ctx, config.WorkspaceID, configJSON.TimeRangeDays, slaTarget)
	if err != nil {
		log.Warnf("[reports] failed to fetch SLA data: %v", err)
		summary = &SLASummary{Name: "SLA Report", GeneratedAt: time.Now()}
	}

	g.renderSLACover(pdf, summary)
	g.renderSLAOverview(pdf, summary)
	g.renderSLAProbeDetails(pdf, probeDetails)

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

		probeMetrics := g.fetchProbeMetricsFromCH(ctx, p.ID, 7)
		if probeMetrics == nil {
			metrics[i] = ProbeMetric{
				Name:       p.Name,
				Type:       p.Type,
				Target:     target,
				AvgLatency: 0,
				PacketLoss: 0,
				Uptime:     100,
			}
			continue
		}

		uptime := 100.0
		if probeMetrics.PacketLoss > 0 {
			uptime = 100.0 - probeMetrics.PacketLoss
		}

		metrics[i] = ProbeMetric{
			Name:       p.Name,
			Type:       p.Type,
			Target:     target,
			AvgLatency: probeMetrics.AvgLatency,
			PacketLoss: probeMetrics.PacketLoss,
			Uptime:     uptime,
		}
	}
	return metrics, nil
}

type simpleProbeMetrics struct {
	AvgLatency  float64
	P95Latency  float64
	PacketLoss  float64
	Jitter      float64
	SampleCount int
	MinLatency  float64
	MaxLatency  float64
}

func (g *Generator) fetchProbeMetricsFromCH(ctx context.Context, probeID uint, days int) *simpleProbeMetrics {
	if days <= 0 {
		days = 7
	}
	from := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	q := fmt.Sprintf(`
SELECT 
    payload_raw,
    created_at
FROM probe_data
WHERE type = 'PING'
  AND probe_id = %d
  AND created_at >= '%s'
ORDER BY created_at DESC
LIMIT 2000
`, probeID, from.Format("2006-01-02 15:04:05"))

	rows, err := g.ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var latencies []float64
	var totalLoss float64
	var totalJitter float64
	var count int

	for rows.Next() {
		var payloadRaw string
		var createdAt time.Time
		if err := rows.Scan(&payloadRaw, &createdAt); err != nil {
			continue
		}
		if payloadRaw == "" {
			continue
		}

		var payload struct {
			AvgRTT     int64   `json:"avg_rtt"`
			StdDevRTT  int64   `json:"std_dev_rtt"`
			PacketLoss float64 `json:"packet_loss"`
		}
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}

		latMs := float64(payload.AvgRTT) / 1_000_000.0
		jitterMs := float64(payload.StdDevRTT) / 1_000_000.0

		latencies = append(latencies, latMs)
		totalLoss += payload.PacketLoss
		totalJitter += jitterMs
		count++
	}

	if count == 0 {
		return nil
	}

	avgLat := average(latencies)
	p95Lat := percentile(latencies, 95)
	avgLoss := totalLoss / float64(count)
	avgJitter := totalJitter / float64(count)

	minLat := 0.0
	maxLat := 0.0
	if len(latencies) > 0 {
		minLat = latencies[0]
		maxLat = latencies[0]
		for _, l := range latencies {
			if l < minLat {
				minLat = l
			}
			if l > maxLat {
				maxLat = l
			}
		}
	}

	return &simpleProbeMetrics{
		AvgLatency:  avgLat,
		P95Latency:  p95Lat,
		PacketLoss:  avgLoss,
		Jitter:      avgJitter,
		SampleCount: count,
		MinLatency:  minLat,
		MaxLatency:  maxLat,
	}
}

func (g *Generator) fetchProbeDetail(ctx context.Context, workspaceID uint, probeID uint, days int64) (*ProbeDetailSummary, error) {
	if days <= 0 {
		days = 7
	}
	from := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	var probeInfo struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Target string `json:"target"`
	}
	if err := g.db.WithContext(ctx).Table("probes").Select("name, type, target").Where("id = ?", probeID).Find(&probeInfo).Error; err != nil {
		return nil, err
	}

	var ws workspace.Workspace
	if err := g.db.WithContext(ctx).First(&ws, workspaceID).Error; err != nil {
		return nil, err
	}

	agentName, agentIP := g.getProbeAgentInfo(ctx, probeID)

	metrics := g.fetchProbeMetricsFromCH(ctx, probeID, int(days))
	if metrics == nil {
		metrics = &simpleProbeMetrics{}
	}

	uptime := 100.0
	if metrics.PacketLoss > 0 {
		uptime = 100.0 - metrics.PacketLoss
	}

	overallHealth := 100.0
	if metrics.PacketLoss > 0 {
		overallHealth = overallHealth - (metrics.PacketLoss * 10)
	}
	if metrics.AvgLatency > 100 {
		overallHealth = overallHealth - 10
	}
	if overallHealth < 0 {
		overallHealth = 0
	}

	grade := "A"
	switch {
	case overallHealth >= 90:
		grade = "A"
	case overallHealth >= 80:
		grade = "B"
	case overallHealth >= 70:
		grade = "C"
	case overallHealth >= 60:
		grade = "D"
	default:
		grade = "F"
	}

	return &ProbeDetailSummary{
		Name:          ws.Name,
		ReportPeriod:  fmt.Sprintf("%s to %s", from.Format("Jan 2, 2006"), time.Now().Format("Jan 2, 2006")),
		GeneratedAt:   time.Now(),
		ProbeName:     probeInfo.Name,
		ProbeType:     probeInfo.Type,
		Target:        probeInfo.Target,
		AgentName:     agentName,
		AgentIP:       agentIP,
		OverallHealth: overallHealth,
		Grade:         grade,
		AvgLatency:    metrics.AvgLatency,
		P95Latency:    metrics.P95Latency,
		PacketLoss:    metrics.PacketLoss,
		Jitter:        metrics.Jitter,
		Uptime:        uptime,
		SampleCount:   metrics.SampleCount,
		MinLatency:    metrics.MinLatency,
		MaxLatency:    metrics.MaxLatency,
	}, nil
}

func (g *Generator) fetchSLAData(ctx context.Context, workspaceID uint, days int64, slaTarget float64) (*SLASummary, []ProbeMetric, error) {
	if days <= 0 {
		days = 7
	}
	from := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	var ws workspace.Workspace
	if err := g.db.WithContext(ctx).First(&ws, workspaceID).Error; err != nil {
		return nil, nil, err
	}

	type ProbeInfo struct {
		ID     uint   `json:"id"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Target string `json:"target"`
	}
	var probes []ProbeInfo
	if err := g.db.WithContext(ctx).Table("probes").Select("id, name, type, target").Where("workspace_id = ?", workspaceID).Find(&probes).Error; err != nil {
		return nil, nil, err
	}

	probesMeetingSLO := 0
	probesViolatingSLO := 0
	details := make([]ProbeMetric, 0, len(probes))

	for _, p := range probes {
		metrics := g.fetchProbeMetricsFromCH(ctx, p.ID, int(days))

		uptime := 100.0
		packetLoss := 0.0
		avgLatency := 0.0

		if metrics != nil {
			avgLatency = metrics.AvgLatency
			packetLoss = metrics.PacketLoss
			if metrics.PacketLoss > 0 {
				uptime = 100.0 - metrics.PacketLoss
			}
		}

		target := p.Target
		if target == "" {
			target = "N/A"
		}

		detail := ProbeMetric{
			Name:       p.Name,
			Type:       p.Type,
			Target:     target,
			AvgLatency: avgLatency,
			PacketLoss: packetLoss,
			Uptime:     uptime,
		}
		details = append(details, detail)

		if uptime >= slaTarget {
			probesMeetingSLO++
		} else {
			probesViolatingSLO++
		}
	}

	overallUptime := 100.0
	if len(details) > 0 {
		totalUptime := 0.0
		for _, d := range details {
			totalUptime += d.Uptime
		}
		overallUptime = totalUptime / float64(len(details))
	}

	grade := "A"
	switch {
	case overallUptime >= 99:
		grade = "A"
	case overallUptime >= 95:
		grade = "B"
	case overallUptime >= 90:
		grade = "C"
	case overallUptime >= 85:
		grade = "D"
	default:
		grade = "F"
	}

	summary := &SLASummary{
		Name:               ws.Name,
		ReportPeriod:       fmt.Sprintf("%s to %s", from.Format("Jan 2, 2006"), time.Now().Format("Jan 2, 2006")),
		GeneratedAt:        time.Now(),
		OverallUptime:      overallUptime,
		Grade:              grade,
		TotalProbes:        len(probes),
		ProbesMeetingSLO:   probesMeetingSLO,
		ProbesViolatingSLO: probesViolatingSLO,
		SLOTarget:          slaTarget,
		LatencySLO:         100,
		PacketLossSLO:      0.5,
	}

	return summary, details, nil
}

func (g *Generator) fetchProbeAlerts(ctx context.Context, workspaceID uint, probeID uint, days int64) ([]AlertEvent, error) {
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
		Where("workspace_id = ? AND probe_id = ? AND fired_at >= ?", workspaceID, probeID, from).
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

func (g *Generator) getProbeAgentInfo(ctx context.Context, probeID uint) (string, string) {
	type Result struct {
		AgentName string
		AgentIP   string
	}
	var r Result
	err := g.db.WithContext(ctx).
		Table("probes").
		Select("COALESCE(agents.name, '') as agent_name, COALESCE(agents.ip_address, '') as agent_ip").
		Joins("LEFT JOIN agents ON agents.id = probes.agent_id").
		Where("probes.id = ?", probeID).
		Find(&r).Error
	if err != nil {
		return "", ""
	}
	return r.AgentName, r.AgentIP
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

func (g *Generator) renderNoProbeSelected(pdf *gofpdf.Fpdf) {
	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(26, 54, 93)
	pdf.Ln(40)
	pdf.Cell(0, 10, "Probe Detail Report")
	pdf.Ln(15)

	pdf.SetFont("Arial", "", 12)
	pdf.SetTextColor(100, 100, 100)
	pdf.Cell(0, 8, "No probes selected for this report.")
	pdf.Ln(8)
	pdf.Cell(0, 8, "Please select specific probes when creating a Probe Detail report.")
}

func (g *Generator) renderProbeDetailCover(pdf *gofpdf.Fpdf, detail *ProbeDetailSummary) {
	pdf.SetFont("Arial", "B", 28)
	pdf.SetTextColor(26, 54, 93)
	pdf.Ln(30)
	pdf.Cell(0, 15, "NetWatcher")
	pdf.Ln(12)

	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(50, 50, 50)
	pdf.Cell(0, 10, "Probe Detail Report")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 14)
	pdf.SetTextColor(80, 80, 80)
	pdf.Cell(0, 8, detail.ProbeName)
	pdf.Ln(8)

	pdf.SetFont("Arial", "", 11)
	pdf.Cell(0, 6, fmt.Sprintf("Type: %s | Target: %s", detail.ProbeType, detail.Target))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Agent: %s (%s)", detail.AgentName, detail.AgentIP))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Period: %s", detail.ReportPeriod))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Generated: %s", detail.GeneratedAt.Format("Jan 2, 2006 15:04 UTC")))
	pdf.Ln(15)

	pdf.SetDrawColor(0, 136, 204)
	pdf.SetLineWidth(0.5)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(10)
}

func (g *Generator) renderProbeDetailMetrics(pdf *gofpdf.Fpdf, detail *ProbeDetailSummary) {
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 8, "Probe Performance")
	pdf.Ln(8)

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(50, 50, 50)

	metrics := [][2]string{
		{"Overall Health", fmt.Sprintf("%.1f%% (%s)", detail.OverallHealth, detail.Grade)},
		{"Uptime", fmt.Sprintf("%.2f%%", detail.Uptime)},
		{"Avg Latency", fmt.Sprintf("%.1f ms", detail.AvgLatency)},
		{"P95 Latency", fmt.Sprintf("%.1f ms", detail.P95Latency)},
		{"Min / Max Latency", fmt.Sprintf("%.1f / %.1f ms", detail.MinLatency, detail.MaxLatency)},
		{"Packet Loss", fmt.Sprintf("%.2f%%", detail.PacketLoss)},
		{"Jitter", fmt.Sprintf("%.1f ms", detail.Jitter)},
		{"Samples", fmt.Sprintf("%d", detail.SampleCount)},
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

func (g *Generator) renderSLACover(pdf *gofpdf.Fpdf, summary *SLASummary) {
	pdf.SetFont("Arial", "B", 28)
	pdf.SetTextColor(26, 54, 93)
	pdf.Ln(40)
	pdf.Cell(0, 15, "NetWatcher")
	pdf.Ln(15)

	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(50, 50, 50)
	pdf.Cell(0, 10, "SLA Report")
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

func (g *Generator) renderSLAOverview(pdf *gofpdf.Fpdf, summary *SLASummary) {
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 8, "SLA Overview")
	pdf.Ln(8)

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(50, 50, 50)

	metrics := [][2]string{
		{"Overall Uptime", fmt.Sprintf("%.2f%% (%s)", summary.OverallUptime, summary.Grade)},
		{"SLO Target", fmt.Sprintf("%.1f%%", summary.SLOTarget)},
		{"Total Probes", fmt.Sprintf("%d", summary.TotalProbes)},
		{"Probes Meeting SLO", fmt.Sprintf("%d", summary.ProbesMeetingSLO)},
		{"Probes Violating SLO", fmt.Sprintf("%d", summary.ProbesViolatingSLO)},
		{"Latency SLO", fmt.Sprintf("< %.0f ms", summary.LatencySLO)},
		{"Packet Loss SLO", fmt.Sprintf("< %.1f%%", summary.PacketLossSLO)},
	}

	for _, m := range metrics {
		pdf.SetFont("Arial", "B", 10)
		pdf.Cell(60, 6, m[0]+":")
		pdf.SetFont("Arial", "", 10)
		pdf.Cell(0, 6, m[1])
		pdf.Ln(6)
	}

	pdf.Ln(10)
}

func (g *Generator) renderSLAProbeDetails(pdf *gofpdf.Fpdf, probes []ProbeMetric) {
	if len(probes) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 8, "Per-Probe SLA Status")
	pdf.Ln(8)

	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(240, 240, 240)
	pdf.SetTextColor(50, 50, 50)
	pdf.CellFormat(50, 7, "Probe Name", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 7, "Type", "1", 0, "L", true, 0, "")
	pdf.CellFormat(45, 7, "Target", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 7, "Uptime", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 7, "Loss %", "1", 1, "L", true, 0, "")

	pdf.SetFont("Arial", "", 9)
	for _, p := range probes {
		statusColor := [3]int{0, 136, 0}
		if p.Uptime < 99.5 {
			statusColor = [3]int{180, 100, 0}
		}
		if p.Uptime < 95 {
			statusColor = [3]int{180, 0, 0}
		}

		pdf.CellFormat(50, 6, truncate(p.Name, 28), "1", 0, "L", false, 0, "")
		pdf.CellFormat(25, 6, p.Type, "1", 0, "L", false, 0, "")
		pdf.CellFormat(45, 6, truncate(p.Target, 25), "1", 0, "L", false, 0, "")
		pdf.SetTextColor(statusColor[0], statusColor[1], statusColor[2])
		pdf.CellFormat(25, 6, fmt.Sprintf("%.2f%%", p.Uptime), "1", 0, "L", false, 0, "")
		pdf.SetTextColor(50, 50, 50)
		pdf.CellFormat(25, 6, fmt.Sprintf("%.2f%%", p.PacketLoss), "1", 1, "L", false, 0, "")
	}

	pdf.Ln(10)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func percentile(values []float64, p int) float64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) == 1 {
		return values[0]
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	idx := (float64(p) / 100.0) * float64(len(sorted)-1)
	i := int(idx)
	f := idx - float64(i)

	if i >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}
	if i < 0 {
		return sorted[0]
	}

	return sorted[i]*(1-f) + sorted[i+1]*f
}
