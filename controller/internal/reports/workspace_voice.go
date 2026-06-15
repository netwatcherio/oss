package reports

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/probe"

	"github.com/jung-kurt/gofpdf"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// WorkspaceVoiceReportSummary is the PDF-shaped summary for a
// workspace-wide voice report. It rolls up every agent's voice
// quality into a top-level workspace score and lists the worst-N
// agents.
type WorkspaceVoiceReportSummary struct {
	Name              string
	WorkspaceID       uint
	ReportPeriod      string
	GeneratedAt       time.Time
	OverallMos        float64
	OverallGrade      string
	TotalAgents       int
	AgentsWithData    int
	TotalProbes       int
	CriticalIssueCnt  int
	WarningIssueCnt   int
	TopIssues         []VoiceIssueSummary
	Agents            []WorkspaceVoiceAgentEntry
}

// WorkspaceVoiceAgentEntry is one agent's slice of the workspace
// voice report — what shows up in the agents table and gets its
// own drill-down page.
type WorkspaceVoiceAgentEntry struct {
	AgentID         uint
	AgentName       string
	IsOnline        bool
	OverallMos      float64
	OverallGrade    string
	LatencyScore    float64
	JitterScore     float64
	PacketLossScore float64
	ForwardPath     *VoicePathSummary
	ReturnPath      *VoicePathSummary
	IssueCount      int
	CriticalCount   int
	WarningCount    int
	TimePattern     string
	Recommendation  string
}

// GenerateWorkspaceVoicePDF produces a workspace-wide voice report.
// It computes voice quality for every agent in the workspace, then
// renders a top-level heatmap + worst-N table + one condensed page
// per agent.
//
// Use `days` (1-365) for the analysis window. The full report is
// always rendered (no section toggles at the workspace level — the
// report is fixed-format).
func (g *Generator) GenerateWorkspaceVoicePDF(ctx context.Context, workspaceID uint, days int64) ([]byte, error) {
	if days <= 0 {
		days = 7
	}
	from := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	to := time.Now().UTC()

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() { pageFooter(pdf) })
	pdf.AddPage()

	summary, err := g.fetchWorkspaceVoiceSummary(ctx, workspaceID, days, from, to)
	if err != nil {
		log.Warnf("[reports] failed to fetch workspace voice summary: %v", err)
		summary = &WorkspaceVoiceReportSummary{Name: "Unknown", GeneratedAt: time.Now()}
	}

	g.renderWorkspaceVoiceCover(pdf, summary)
	g.renderWorkspaceVoiceOverview(pdf, summary)
	g.renderWorkspaceVoiceAgentsTable(pdf, summary)
	g.renderWorkspaceVoiceHeatmap(pdf, summary)
	g.renderWorkspaceVoiceTopIssues(pdf, summary)

	// Per-agent drill-down — one condensed page per agent.
	for i := range summary.Agents {
		pdf.AddPage()
		g.renderWorkspaceVoiceAgentPage(pdf, &summary.Agents[i], summary)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output failed: %w", err)
	}
	return buf.Bytes(), nil
}

func (g *Generator) fetchWorkspaceVoiceSummary(ctx context.Context, workspaceID uint, days int64, from, to time.Time) (*WorkspaceVoiceReportSummary, error) {
	// Pull the list of agents in this workspace.
	var wsName string
	if err := g.db.WithContext(ctx).Table("workspaces").Select("name").Where("id = ?", workspaceID).Scan(&wsName).Error; err != nil {
		wsName = "Unknown"
	}
	var agentIDs []uint
	if err := g.db.WithContext(ctx).Table("agents").Select("id").Where("workspace_id = ?", workspaceID).Scan(&agentIDs).Error; err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}

	out := &WorkspaceVoiceReportSummary{
		Name:         wsName,
		WorkspaceID:  workspaceID,
		ReportPeriod: fmt.Sprintf("Last %d days (%s to %s)", days, from.Format("2006-01-02"), to.Format("2006-01-02")),
		GeneratedAt:  time.Now(),
		TotalAgents:  len(agentIDs),
	}

	var allMOS []float64
	for _, id := range agentIDs {
		agentObj, err := agent.GetAgentByID(ctx, g.db, id)
		if err != nil {
			continue
		}
		vq, err := probe.ComputeAgentVoiceQuality(ctx, g.db, g.ch, id, from, to)
		if err != nil {
			log.Warnf("[reports] voice quality for agent %d failed: %v", id, err)
			continue
		}
		entry := WorkspaceVoiceAgentEntry{
			AgentID:         id,
			AgentName:       agentObj.Name,
			IsOnline:        agentObj.LastSeenAt.After(time.Now().UTC().Add(-5 * time.Minute)),
			OverallMos:      vq.OverallMos,
			OverallGrade:    vq.OverallGrade,
			LatencyScore:    vq.LatencyScore,
			JitterScore:     vq.JitterScore,
			PacketLossScore: vq.PacketLossScore,
			ForwardPath:     toVoicePathSummary(vq.ForwardPath),
			ReturnPath:      toVoicePathSummary(vq.ReturnPath),
			TimePattern:     vq.TimePattern,
			Recommendation:  vq.Recommendation,
		}
		for _, iss := range vq.Issues {
			entry.IssueCount++
			switch iss.Severity {
			case "critical":
				entry.CriticalCount++
				out.CriticalIssueCnt++
			case "warning":
				entry.WarningCount++
				out.WarningIssueCnt++
			}
		}
		out.Agents = append(out.Agents, entry)
		out.AgentsWithData++
		out.TotalProbes += len(vq.Probes)
		allMOS = append(allMOS, vq.OverallMos)

		// Track top issues for the workspace-wide "Top issues" table.
		for _, iss := range vq.Issues {
			out.TopIssues = append(out.TopIssues, VoiceIssueSummary{
				Severity:        iss.Severity,
				Title:           fmt.Sprintf("[%s] %s", agentObj.Name, iss.Title),
				Category:        iss.Category,
				SuspectedCause:  iss.SuspectedCause,
				TimePattern:     iss.TimePattern,
				MosDegradation:  iss.MosDegradation,
				Recommendations: iss.Recommendations,
			})
		}
	}

	// Sort agents worst-first so the table is action-oriented.
	sort.SliceStable(out.Agents, func(i, j int) bool {
		return out.Agents[i].OverallMos < out.Agents[j].OverallMos
	})

	// Sort top issues: critical first, then by degradation magnitude.
	sort.SliceStable(out.TopIssues, func(i, j int) bool {
		if out.TopIssues[i].Severity != out.TopIssues[j].Severity {
			return out.TopIssues[i].Severity == "critical"
		}
		return out.TopIssues[i].MosDegradation < out.TopIssues[j].MosDegradation
	})
	if len(out.TopIssues) > 20 {
		out.TopIssues = out.TopIssues[:20]
	}

	if len(allMOS) > 0 {
		var sum float64
		for _, m := range allMOS {
			sum += m
		}
		out.OverallMos = sum / float64(len(allMOS))
		out.OverallGrade = voiceGradeFromMos(out.OverallMos)
	} else {
		out.OverallMos = 4.5
		out.OverallGrade = "excellent"
	}

	return out, nil
}

func (g *Generator) renderWorkspaceVoiceCover(pdf *gofpdf.Fpdf, summary *WorkspaceVoiceReportSummary) {
	pdf.SetFont("Arial", "B", 28)
	pdf.SetTextColor(26, 54, 93)
	pdf.Ln(40)
	pdf.Cell(0, 15, "NetWatcher")
	pdf.Ln(15)

	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(50, 50, 50)
	pdf.Cell(0, 10, "Voice Quality Report — Workspace")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 14)
	pdf.SetTextColor(80, 80, 80)
	pdf.Cell(0, 8, summary.Name)
	pdf.Ln(8)

	pdf.SetFont("Arial", "", 11)
	pdf.Cell(0, 6, fmt.Sprintf("Workspace ID: %d", summary.WorkspaceID))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Report Period: %s", summary.ReportPeriod))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Generated: %s", summary.GeneratedAt.Format("Jan 2, 2006 15:04 UTC")))
	pdf.Ln(20)

	pdf.SetDrawColor(0, 136, 204)
	pdf.SetLineWidth(0.5)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(10)
}

func (g *Generator) renderWorkspaceVoiceOverview(pdf *gofpdf.Fpdf, summary *WorkspaceVoiceReportSummary) {
	gradeRGB := gradeRGBFor(summary.OverallGrade)
	chipText(pdf, fmt.Sprintf("Workspace MOS %.2f — %s", summary.OverallMos, strings.ToUpper(summary.OverallGrade)),
		"", gradeRGB[0], gradeRGB[1], gradeRGB[2])
	pdf.Ln(2)
	metricRow(pdf, "Agents", fmt.Sprintf("%d with data / %d total", summary.AgentsWithData, summary.TotalAgents))
	metricRow(pdf, "Probes", fmt.Sprintf("%d", summary.TotalProbes))
	metricRow(pdf, "Critical issues", fmt.Sprintf("%d", summary.CriticalIssueCnt))
	metricRow(pdf, "Warning issues", fmt.Sprintf("%d", summary.WarningIssueCnt))
	pdf.Ln(8)
}

func (g *Generator) renderWorkspaceVoiceAgentsTable(pdf *gofpdf.Fpdf, summary *WorkspaceVoiceReportSummary) {
	if len(summary.Agents) == 0 {
		sectionHeader(pdf, "Agents")
		pdf.SetFont("Arial", "I", 10)
		pdf.SetTextColor(120, 120, 120)
		pdf.Cell(0, 6, "No agent data available for this workspace.")
		pdf.Ln(8)
		return
	}
	sectionHeader(pdf, "Agents (sorted worst → best)")

	// 12 + 50 + 14 + 14 + 14 + 14 + 12 + 14 + 14 = 158mm; fits.
	colWidths := []float64{12, 50, 14, 14, 14, 14, 12, 14, 14}
	headers := []string{"#", "Agent", "MOS", "Lat", "Jit", "Loss", "Crit", "Warn", "Pattern"}
	for i, h := range headers {
		pdf.CellFormat(colWidths[i], 6, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(6)
	for i, a := range summary.Agents {
		pdf.SetFont("Arial", "", 8)
		pdf.SetTextColor(50, 50, 50)
		row := []string{
			fmt.Sprintf("%d", i+1),
			truncate(a.AgentName, 30),
			fmt.Sprintf("%.2f", a.OverallMos),
			fmt.Sprintf("%.0f", a.LatencyScore),
			fmt.Sprintf("%.0f", a.JitterScore),
			fmt.Sprintf("%.0f", a.PacketLossScore),
			fmt.Sprintf("%d", a.CriticalCount),
			fmt.Sprintf("%d", a.WarningCount),
			a.TimePattern,
		}
		for j, cell := range row {
			pdf.CellFormat(colWidths[j], 5, cell, "1", 0, "C", false, 0, "")
		}
		pdf.Ln(5)
	}
	pdf.Ln(4)
}

func (g *Generator) renderWorkspaceVoiceHeatmap(pdf *gofpdf.Fpdf, summary *WorkspaceVoiceReportSummary) {
	if len(summary.Agents) == 0 {
		return
	}
	sectionHeader(pdf, "Per-Agent MOS Heatmap")

	cells := make([]PerProbeCell, 0, len(summary.Agents))
	for _, a := range summary.Agents {
		cells = append(cells, PerProbeCell{
			Label: truncate(a.AgentName, 16),
			MOS:   a.OverallMos,
		})
	}
	png, err := RenderPerProbeHeatmap(cells, 6)
	if err != nil {
		pdf.SetFont("Arial", "I", 9)
		pdf.SetTextColor(180, 0, 0)
		pdf.MultiCell(0, 4, fmt.Sprintf("Heatmap failed to render: %v", err), "", "", false)
		pdf.Ln(2)
		return
	}
	if _, err := imageBytesFromPNG(pdf, fmt.Sprintf("ws-heatmap-%d", summary.WorkspaceID), png); err != nil {
		return
	}
	pdf.ImageOptions(fmt.Sprintf("ws-heatmap-%d", summary.WorkspaceID), pdfMargin, pdf.GetY(), 180, 60, false, gofpdf.ImageOptions{}, 0, "")
	pdf.Ln(64)
}

func (g *Generator) renderWorkspaceVoiceTopIssues(pdf *gofpdf.Fpdf, summary *WorkspaceVoiceReportSummary) {
	if len(summary.TopIssues) == 0 {
		return
	}
	sectionHeader(pdf, fmt.Sprintf("Top Issues (%d shown)", len(summary.TopIssues)))
	for _, iss := range summary.TopIssues {
		ensureRoom(pdf, 14)
		sevColor := [3]int{255, 193, 7}
		if iss.Severity == "critical" {
			sevColor = [3]int{220, 53, 69}
		}
		pdf.SetFillColor(sevColor[0], sevColor[1], sevColor[2])
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(0, 6, fmt.Sprintf("[%s] %s", strings.ToUpper(iss.Severity), iss.Title), "1", 0, "L", true, 0, "")
		pdf.Ln(6)
		pdf.SetTextColor(50, 50, 50)
		pdf.SetFont("Arial", "", 8)
		if iss.SuspectedCause != "" {
			pdf.Cell(4, 4, "")
			pdf.MultiCell(0, 4, "Cause: "+iss.SuspectedCause, "", "", false)
		}
		pdf.Ln(2)
	}
}

func (g *Generator) renderWorkspaceVoiceAgentPage(pdf *gofpdf.Fpdf, a *WorkspaceVoiceAgentEntry, _ *WorkspaceVoiceReportSummary) {
	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 10, fmt.Sprintf("Agent: %s", a.AgentName))
	pdf.Ln(12)

	gradeRGB := gradeRGBFor(a.OverallGrade)
	chipText(pdf, fmt.Sprintf("MOS %.2f — %s", a.OverallMos, strings.ToUpper(a.OverallGrade)),
		"", gradeRGB[0], gradeRGB[1], gradeRGB[2])

	metricRow(pdf, "Latency Score", fmt.Sprintf("%.1f / 100", a.LatencyScore))
	metricRow(pdf, "Jitter Score", fmt.Sprintf("%.1f / 100", a.JitterScore))
	metricRow(pdf, "Packet Loss Score", fmt.Sprintf("%.1f / 100", a.PacketLossScore))
	metricRow(pdf, "Issues", fmt.Sprintf("%d critical, %d warning", a.CriticalCount, a.WarningCount))
	metricRow(pdf, "Time pattern", a.TimePattern)

	if a.Recommendation != "" {
		pdf.Ln(2)
		pdf.SetFont("Arial", "B", 10)
		pdf.Cell(0, 6, "Recommendation:")
		pdf.Ln(6)
		pdf.SetFont("Arial", "", 10)
		pdf.MultiCell(0, 5, a.Recommendation, "", "", false)
	}

	// Forward / return cards
	pdf.Ln(4)
	pdf.SetFont("Arial", "B", 11)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 6, "Forward path")
	pdf.Ln(6)
	if a.ForwardPath != nil {
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(50, 50, 50)
		pdf.MultiCell(0, 4, fmt.Sprintf("%s  •  MOS %.2f (%s)  •  Loss %.2f%%  •  Jitter %.1fms  •  Latency %.0fms",
			a.ForwardPath.TargetAgent, a.ForwardPath.MosScore, a.ForwardPath.Grade,
			a.ForwardPath.PacketLoss, a.ForwardPath.JitterAvg, a.ForwardPath.AvgLatency), "", "", false)
	} else {
		pdf.SetFont("Arial", "I", 9)
		pdf.SetTextColor(120, 120, 120)
		pdf.Cell(0, 5, "(no data)")
		pdf.Ln(5)
	}

	pdf.Ln(2)
	pdf.SetFont("Arial", "B", 11)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 6, "Return path")
	pdf.Ln(6)
	if a.ReturnPath != nil {
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(50, 50, 50)
		pdf.MultiCell(0, 4, fmt.Sprintf("%s  •  MOS %.2f (%s)  •  Loss %.2f%%  •  Jitter %.1fms  •  Latency %.0fms",
			a.ReturnPath.SourceAgent, a.ReturnPath.MosScore, a.ReturnPath.Grade,
			a.ReturnPath.PacketLoss, a.ReturnPath.JitterAvg, a.ReturnPath.AvgLatency), "", "", false)
	} else {
		pdf.SetFont("Arial", "I", 9)
		pdf.SetTextColor(120, 120, 120)
		pdf.Cell(0, 5, "(no data)")
		pdf.Ln(5)
	}
}

// Compile-time check that the gorm import isn't accidentally
// dropped. We use it via the *gorm.DB passed to the Generator.
var _ *gorm.DB
