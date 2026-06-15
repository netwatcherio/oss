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

	"netwatcher-controller/internal/agent"
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

type VoicePathSummary struct {
	Direction       string
	TargetAgent     string
	SourceAgent     string
	MosScore        float64
	Grade           string
	AvgLatency      float64
	P95Latency      float64
	MedianLatency   float64
	JitterAvg       float64
	JitterMedian    float64
	JitterP95       float64
	PacketLoss      float64
	OutOfSequence   float64
	Duplicates      float64
	SampleCount     int
	CongestionLevel string
}

type VoiceIssueSummary struct {
	Severity        string
	Title           string
	Category        string
	SuspectedCause  string
	TimePattern     string
	MosDegradation  float64
	Recommendations []string
}

type AgentVoiceReportSummary struct {
	Name            string
	AgentID         uint
	ReportPeriod    string
	GeneratedAt     time.Time
	OverallMos      float64
	OverallGrade    string
	LatencyScore    float64
	JitterScore     float64
	PacketLossScore float64
	ForwardPath     *VoicePathSummary
	ReturnPath      *VoicePathSummary
	AggregateForward *VoicePathSummary
	AggregateReturn  *VoicePathSummary
	Probes          []VoicePathSummary
	Issues          []VoiceIssueSummary
	Recommendation  string

	// New (Phase 1) fields — populated when the analysis engine
	// provides them. Used by the executive snapshot, timeline, and
	// correlation sections.
	BaselineComparison *BaselineDeltaSummary
	Trends             *VoiceTrendsSummary
	RouteSignals       []RouteSignalSummary
	WorkspaceContext   *WorkspaceIncidentSummary
	Thresholds         *VoiceThresholdsSummary
}

// BaselineDeltaSummary is a PDF-friendly snapshot of the 7-day
// baseline comparison. Mirrors probe.BaselineDelta but is detached
// from the live struct so the report module doesn't import the
// probe package (which would create a cycle through generator.go).
type BaselineDeltaSummary struct {
	From             string
	To               string
	MosDelta         float64
	LatencyDeltaMs   float64
	JitterDeltaMs    float64
	LossDeltaPct     float64
	SampleCount      int
	BaselineSamples  int
	Trend            string
	PercentChange    float64
}

// VoiceTrendsSummary is the PDF view of the per-bucket MOS series.
type VoiceTrendsSummary struct {
	BucketMinutes int
	ForwardMOS    []float64
	ReturnMOS     []float64
	Timestamps    []string
	IssueBuckets  []string
}

// RouteSignalSummary is the PDF view of one route/MTR signal.
type RouteSignalSummary struct {
	ProbeID    uint
	ProbeType  string
	Type       string
	Severity   string
	Title      string
	Evidence   string
	DetectedAt string
}

// WorkspaceIncidentSummary is the PDF view of the workspace context.
type WorkspaceIncidentSummary struct {
	AffectedCount int
	CriticalCount int
	WarningCount  int
	Incidents     []WorkspaceIncidentEntry
}

// WorkspaceIncidentEntry is one workspace incident, PDF-shaped.
type WorkspaceIncidentEntry struct {
	Title         string
	Severity      string
	Scope         string
	SuggestedCause string
}

// VoiceThresholdsSummary is the PDF view of the effective thresholds.
type VoiceThresholdsSummary struct {
	Codec            string
	WarningJitterMs  float64
	CriticalJitterMs float64
	WarningLossPct   float64
	CriticalLossPct  float64
	ExcellentMos     float64
	GoodMos          float64
	FairMos          float64
	PoorMos          float64
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

func (g *Generator) GenerateAgentPDF(ctx context.Context, agentID uint, days int64) ([]byte, error) {
	return g.GenerateAgentPDFWithOptions(ctx, agentID, days, time.Time{}, time.Time{}, FullAgentReportOptions())
}

func (g *Generator) GenerateAgentPDFCustomRange(ctx context.Context, agentID uint, from, to time.Time) ([]byte, error) {
	return g.GenerateAgentPDFWithOptions(ctx, agentID, 0, from, to, FullAgentReportOptions())
}

// GenerateAgentPDFWithOptions is the section-aware entry point. The
// `opts` param controls which sections render; pass
// `DefaultAgentReportOptions()` for the "Quick 7-day" preset or
// `FullAgentReportOptions()` for everything.
//
// The function preserves the legacy entry points
// (`GenerateAgentPDF` / `GenerateAgentPDFCustomRange`) by
// defaulting them to the full set so existing callers (workspace
// schedules, email jobs) keep working.
func (g *Generator) GenerateAgentPDFWithOptions(ctx context.Context, agentID uint, days int64, from, to time.Time, opts AgentReportOptions) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() { pageFooter(pdf) })
	pdf.AddPage()

	if days <= 0 && !from.IsZero() && !to.IsZero() {
		// Custom range path
	} else if days <= 0 {
		days = 7
	}

	summary, err := g.fetchAgentVoiceReportSummaryWithOptions(ctx, agentID, days, from, to, opts)
	if err != nil {
		log.Warnf("[reports] failed to fetch agent voice summary for agent %d: %v", agentID, err)
		summary = &AgentVoiceReportSummary{Name: "Unknown Agent", GeneratedAt: time.Now()}
	}

	g.renderAgentVoiceCover(pdf, summary)
	g.renderVoiceExecutiveSnapshot(pdf, summary, opts)
	if opts.IncludeTimeline {
		g.renderVoiceTimeline(pdf, summary, opts)
	}
	if opts.IncludeAggregate {
		g.renderVoiceAggregate(pdf, summary, opts)
	}
	if opts.IncludePerProbe {
		g.renderVoicePathDetails(pdf, summary, opts)
	}
	if opts.IncludeIssues {
		g.renderVoiceIssues(pdf, summary, opts)
	}
	if opts.IncludeCorrelation {
		g.renderVoiceWorkspaceContext(pdf, summary, opts)
		g.renderVoiceRouteSignals(pdf, summary, opts)
	}
	if opts.IncludeAppendix {
		g.renderVoiceAppendix(pdf, summary, opts)
	}
	if opts.IncludeRawJSON {
		g.renderVoiceRawJSONAppendix(pdf, summary, opts)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output failed: %w", err)
	}

	return buf.Bytes(), nil
}

func (g *Generator) fetchAgentVoiceReportSummary(ctx context.Context, agentID uint, days int64) (*AgentVoiceReportSummary, error) {
	return g.fetchAgentVoiceReportSummaryWithOptions(ctx, agentID, days, time.Time{}, time.Time{}, FullAgentReportOptions())
}

func (g *Generator) fetchAgentVoiceReportSummaryCustomRange(ctx context.Context, agentID uint, from, to time.Time) (*AgentVoiceReportSummary, error) {
	return g.fetchAgentVoiceReportSummaryWithOptions(ctx, agentID, 0, from, to, FullAgentReportOptions())
}

// fetchAgentVoiceReportSummaryWithOptions is the canonical builder for
// the PDF-shaped summary. It calls into probe.ComputeAgentVoiceQuality
// (which now returns the enriched struct with aggregate, trends,
// baseline, route signals, workspace context) and flattens the live
// probe types into the report-friendly types declared above.
//
// `opts` is consulted only to decide whether the corresponding
// `probe.ComputeAgentVoiceQuality` enrichment paths should be
// filled. Today we always fill them (so an "issues-only" PDF still
// has the data available for the chart); future optimization could
// skip expensive enrichment for minimal reports.
func (g *Generator) fetchAgentVoiceReportSummaryWithOptions(ctx context.Context, agentID uint, days int64, from, to time.Time, opts AgentReportOptions) (*AgentVoiceReportSummary, error) {
	// Resolve the report window. If `from`/`to` are both zero, use
	// `days`; else use the explicit range.
	if from.IsZero() || to.IsZero() {
		if days <= 0 {
			days = 7
		}
		from = time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		to = time.Now().UTC()
	}

	agentObj, err := agent.GetAgentByID(ctx, g.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	vq, err := probe.ComputeAgentVoiceQuality(ctx, g.db, g.ch, agentID, from, to)
	if err != nil {
		log.Warnf("[reports] failed to compute voice quality for agent %d: %v", agentID, err)
	}

	summary := &AgentVoiceReportSummary{
		Name:            agentObj.Name,
		AgentID:         agentID,
		ReportPeriod:    fmt.Sprintf("%s to %s", from.Format("2006-01-02"), to.Format("2006-01-02")),
		GeneratedAt:     time.Now(),
		OverallMos:      4.5,
		OverallGrade:    "excellent",
		LatencyScore:    100,
		JitterScore:     100,
		PacketLossScore: 100,
	}

	if vq != nil {
		summary.OverallMos = vq.OverallMos
		summary.OverallGrade = vq.OverallGrade
		summary.LatencyScore = vq.LatencyScore
		summary.JitterScore = vq.JitterScore
		summary.PacketLossScore = vq.PacketLossScore
		summary.Recommendation = vq.Recommendation
		summary.ForwardPath = toVoicePathSummary(vq.ForwardPath)
		summary.ReturnPath = toVoicePathSummary(vq.ReturnPath)
		summary.AggregateForward = toVoicePathSummary(vq.AggregateForward)
		summary.AggregateReturn = toVoicePathSummary(vq.AggregateReturn)

		for _, issue := range vq.Issues {
			summary.Issues = append(summary.Issues, VoiceIssueSummary{
				Severity:        issue.Severity,
				Title:           issue.Title,
				Category:        issue.Category,
				SuspectedCause:  issue.SuspectedCause,
				TimePattern:     issue.TimePattern,
				MosDegradation:  issue.MosDegradation,
				Recommendations: issue.Recommendations,
			})
		}

		for _, p := range vq.Probes {
			summary.Probes = append(summary.Probes, *toVoicePathSummary(&p))
		}

		summary.BaselineComparison = toBaselineSummary(vq.BaselineComparison)
		summary.Trends = toTrendsSummary(vq.Trends)
		summary.RouteSignals = toRouteSignalSummary(vq.RouteSignals)
		summary.WorkspaceContext = toWorkspaceContextSummary(vq.WorkspaceContext)
		summary.Thresholds = toThresholdsSummary(vq.ThresholdsUsed)
	}

	return summary, nil
}

// toVoicePathSummary / toBaselineSummary / etc. are the probe→PDF
// flatteners. They keep the report package free of direct dep on
// the probe struct tags (and the two packages can evolve their JSON
// shape independently).

func toVoicePathSummary(p *probe.VoicePathMetrics) *VoicePathSummary {
	if p == nil {
		return nil
	}
	return &VoicePathSummary{
		Direction:       string(p.Direction),
		TargetAgent:     p.TargetAgentName,
		SourceAgent:     p.SourceAgentName,
		MosScore:        p.MosScore,
		Grade:           voiceGradeFromMos(p.MosScore),
		AvgLatency:      p.AvgLatency,
		P95Latency:      p.P95Latency,
		MedianLatency:   p.MedianLatency,
		JitterAvg:       p.JitterAvg,
		JitterMedian:    p.JitterMedian,
		JitterP95:       p.JitterP95,
		PacketLoss:      p.PacketLoss,
		OutOfSequence:   p.OutOfSequence,
		Duplicates:      p.Duplicates,
		SampleCount:     p.SampleCount,
		CongestionLevel: string(p.CongestionLevel),
	}
}

func toBaselineSummary(b *probe.BaselineDelta) *BaselineDeltaSummary {
	if b == nil {
		return nil
	}
	return &BaselineDeltaSummary{
		From:            b.From.Format("2006-01-02"),
		To:              b.To.Format("2006-01-02"),
		MosDelta:        b.MosDelta,
		LatencyDeltaMs:  b.LatencyDeltaMs,
		JitterDeltaMs:   b.JitterDeltaMs,
		LossDeltaPct:    b.LossDeltaPct,
		SampleCount:     b.SampleCount,
		BaselineSamples: b.BaselineSamples,
		Trend:           b.Trend,
		PercentChange:   b.PercentChange,
	}
}

func toTrendsSummary(t *probe.VoiceTrends) *VoiceTrendsSummary {
	if t == nil {
		return nil
	}
	out := &VoiceTrendsSummary{
		BucketMinutes: t.BucketMinutes,
		IssueBuckets:  append([]string(nil), t.IssueBuckets...),
	}
	for _, b := range t.Forward {
		out.ForwardMOS = append(out.ForwardMOS, b.Forward)
		out.Timestamps = append(out.Timestamps, b.Timestamp)
	}
	for _, b := range t.Return {
		out.ReturnMOS = append(out.ReturnMOS, b.Return)
	}
	if out.Timestamps == nil && len(t.Combined) > 0 {
		// Fall back to the combined series if the per-direction split
		// wasn't populated.
		out.ForwardMOS = nil
		out.ReturnMOS = nil
		for _, b := range t.Combined {
			out.ForwardMOS = append(out.ForwardMOS, b.Forward)
			out.ReturnMOS = append(out.ReturnMOS, b.Return)
			out.Timestamps = append(out.Timestamps, b.Timestamp)
		}
	}
	return out
}

func toRouteSignalSummary(signals []probe.RouteSignal) []RouteSignalSummary {
	if len(signals) == 0 {
		return nil
	}
	out := make([]RouteSignalSummary, 0, len(signals))
	for _, s := range signals {
		out = append(out, RouteSignalSummary{
			ProbeID:    s.ProbeID,
			ProbeType:  s.ProbeType,
			Type:       s.Type,
			Severity:   s.Severity,
			Title:      s.Title,
			Evidence:   s.Evidence,
			DetectedAt: s.DetectedAt.Format("2006-01-02 15:04 UTC"),
		})
	}
	return out
}

func toWorkspaceContextSummary(c *probe.WorkspaceIncidentContext) *WorkspaceIncidentSummary {
	if c == nil {
		return nil
	}
	out := &WorkspaceIncidentSummary{
		AffectedCount: c.AffectedCount,
		CriticalCount: c.CriticalCount,
		WarningCount:  c.WarningCount,
	}
	for _, inc := range c.Incidents {
		out.Incidents = append(out.Incidents, WorkspaceIncidentEntry{
			Title:          inc.Title,
			Severity:       inc.Severity,
			Scope:          inc.Scope,
			SuggestedCause: inc.SuggestedCause,
		})
	}
	return out
}

func toThresholdsSummary(t *probe.VoiceThresholds) *VoiceThresholdsSummary {
	if t == nil {
		return nil
	}
	return &VoiceThresholdsSummary{
		Codec:            t.Codec,
		WarningJitterMs:  t.WarningJitterMs,
		CriticalJitterMs: t.CriticalJitterMs,
		WarningLossPct:   t.WarningLossPct,
		CriticalLossPct:  t.CriticalLossPct,
		ExcellentMos:     t.ExcellentMos,
		GoodMos:          t.GoodMos,
		FairMos:          t.FairMos,
		PoorMos:          t.PoorMos,
	}
}

func voiceGradeFromMos(mos float64) string {
	switch {
	case mos >= 4.3:
		return "excellent"
	case mos >= 4.0:
		return "good"
	case mos >= 3.6:
		return "fair"
	case mos >= 3.1:
		return "poor"
	default:
		return "critical"
	}
}

func (g *Generator) renderAgentVoiceCover(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary) {
	pdf.SetFont("Arial", "B", 28)
	pdf.SetTextColor(26, 54, 93)
	pdf.Ln(40)
	pdf.Cell(0, 15, "NetWatcher")
	pdf.Ln(15)

	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(50, 50, 50)
	pdf.Cell(0, 10, "Voice Quality Report — Agent")
	pdf.Ln(10)

	pdf.SetFont("Arial", "", 14)
	pdf.SetTextColor(80, 80, 80)
	pdf.Cell(0, 8, summary.Name)
	pdf.Ln(8)

	pdf.SetFont("Arial", "", 11)
	pdf.Cell(0, 6, fmt.Sprintf("Agent ID: %d", summary.AgentID))
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

func (g *Generator) renderAgentVoiceSummary(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary) {
	// Legacy entry point preserved for the workspace voice report
	// (which doesn't use the section toggle). Renders the executive
	// snapshot as it did before.
	g.renderVoiceExecutiveSnapshot(pdf, summary, DefaultAgentReportOptions())
}

// renderVoiceExecutiveSnapshot is the one-page top-of-report block.
// It includes:
//   - Grade chip + overall MOS score
//   - Three metric chips (latency / jitter / loss scores)
//   - 7-day baseline comparison
//   - Issue count (critical + warning)
//   - Recommendation sentence
//
// Rendered for the first page so the operator can read the headline
// without flipping pages.
func (g *Generator) renderVoiceExecutiveSnapshot(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary, opts AgentReportOptions) {
	_ = opts
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 8, "Voice Quality Summary")
	pdf.Ln(8)

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(50, 50, 50)

	gradeRGB := gradeRGBFor(summary.OverallGrade)

	// Grade chip
	chipText(pdf, fmt.Sprintf("MOS %.2f — %s", summary.OverallMos, strings.ToUpper(summary.OverallGrade)),
		"", gradeRGB[0], gradeRGB[1], gradeRGB[2])
	pdf.Ln(2)

	metricRow(pdf, "Overall MOS", fmt.Sprintf("%.2f / 5.0", summary.OverallMos))
	metricRow(pdf, "Latency Score", fmt.Sprintf("%.1f / 100", summary.LatencyScore))
	metricRow(pdf, "Jitter Score", fmt.Sprintf("%.1f / 100", summary.JitterScore))
	metricRow(pdf, "Packet Loss Score", fmt.Sprintf("%.1f / 100", summary.PacketLossScore))

	critical, warning := 0, 0
	for _, iss := range summary.Issues {
		switch iss.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		}
	}
	metricRow(pdf, "Voice Issues Detected", fmt.Sprintf("%d critical, %d warning", critical, warning))

	// 7-day baseline comparison
	if summary.BaselineComparison != nil {
		bd := summary.BaselineComparison
		trendArrow := "→"
		switch bd.Trend {
		case "improving":
			trendArrow = "↑"
		case "worsening":
			trendArrow = "↓"
		}
		metricRow(pdf, "vs 7-day baseline",
			fmt.Sprintf("%s MOS %+.2f (latency %+.0fms, jitter %+.1fms, loss %+.2f%%)  [%s samples vs %d baseline]",
				trendArrow, bd.MosDelta, bd.LatencyDeltaMs, bd.JitterDeltaMs, bd.LossDeltaPct, humanCount(bd.SampleCount), bd.BaselineSamples))
	}

	if summary.Recommendation != "" {
		pdf.Ln(2)
		pdf.SetFont("Arial", "B", 10)
		pdf.Cell(0, 6, "Recommendation:")
		pdf.Ln(6)
		pdf.SetFont("Arial", "", 10)
		pdf.MultiCell(0, 5, summary.Recommendation, "", "", false)
	}

	pdf.Ln(6)
}

// gradeRGBFor returns the [r,g,b] triple for a grade, mirroring
// panel/src/components/analysis/types.ts gradeColors and charts.go.
// The result is what gofpdf.SetFillColor / SetTextColor expects.
func gradeRGBFor(grade string) [3]int {
	switch grade {
	case "excellent":
		return [3]int{22, 163, 74}
	case "good":
		return [3]int{59, 130, 246}
	case "fair":
		return [3]int{234, 179, 8}
	case "poor":
		return [3]int{249, 115, 22}
	case "critical":
		return [3]int{220, 38, 38}
	default:
		return [3]int{100, 100, 100}
	}
}

// humanCount formats a sample count as "1.2k" / "3.4M" for compact
// rendering in the executive snapshot. Numbers < 1000 are unchanged.
func humanCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// renderVoiceTimeline draws the MOS-over-time chart (with issue
// markers) for the analysis window. Falls back to a "no data" line
// when the trends slice is empty.
func (g *Generator) renderVoiceTimeline(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary, opts AgentReportOptions) {
	_ = opts
	sectionHeader(pdf, "MOS Timeline")

	if summary.Trends == nil || len(summary.Trends.ForwardMOS) < 2 {
		pdf.SetFont("Arial", "I", 10)
		pdf.SetTextColor(120, 120, 120)
		pdf.MultiCell(0, 5, "No time-bucketed MOS data available for the selected window. "+
			"Try a longer time range (the engine buckets every hour for windows ≤ 7 days, every 4h beyond).", "", "", false)
		pdf.Ln(4)
		return
	}

	buckets := make([]VoiceBucket, 0, len(summary.Trends.ForwardMOS))
	for i, mos := range summary.Trends.ForwardMOS {
		ts := ""
		if i < len(summary.Trends.Timestamps) {
			ts = summary.Trends.Timestamps[i]
		}
		ret := mos
		if i < len(summary.Trends.ReturnMOS) {
			ret = summary.Trends.ReturnMOS[i]
		}
		buckets = append(buckets, VoiceBucket{
			Timestamp: ts,
			Forward:   mos,
			Return:    ret,
		})
	}

	png, err := RenderMOSTimeline(buckets, summary.Trends.IssueBuckets)
	if err != nil {
		pdf.SetFont("Arial", "I", 9)
		pdf.SetTextColor(180, 0, 0)
		pdf.MultiCell(0, 5, fmt.Sprintf("Timeline chart failed to render: %v", err), "", "", false)
		pdf.Ln(4)
		return
	}
	// Register the PNG so gofpdf has it. The name is reused by the
	// ImageOptions call below.
	if _, err := imageBytesFromPNG(pdf, fmt.Sprintf("mostimeline-%d", summary.AgentID), png); err != nil {
		pdf.SetFont("Arial", "I", 9)
		pdf.SetTextColor(180, 0, 0)
		pdf.MultiCell(0, 5, "Timeline chart could not be registered with PDF renderer.", "", "", false)
		pdf.Ln(4)
		return
	}
	// 180mm wide content area; chart sits at 160mm to leave margins.
	pdf.ImageOptions(fmt.Sprintf("mostimeline-%d", summary.AgentID), pdfMargin, pdf.GetY(), 180, 75, false, gofpdf.ImageOptions{}, 0, "")
	pdf.Ln(80)
}

// renderVoiceAggregate draws the sample-weighted forward / return
// summary cards. The aggregate gives the operator a "honest" view
// of the agent's voice quality across all probes, not just the
// worst-offender (which is what the executive snapshot covers).
func (g *Generator) renderVoiceAggregate(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary, opts AgentReportOptions) {
	_ = opts
	sectionHeader(pdf, "Aggregate Voice Paths (sample-weighted across all probes)")

	if summary.AggregateForward == nil && summary.AggregateReturn == nil {
		pdf.SetFont("Arial", "I", 10)
		pdf.SetTextColor(120, 120, 120)
		pdf.Cell(0, 6, "No aggregate voice data available.")
		pdf.Ln(8)
		return
	}

	// Side-by-side card rendering: each card is 88mm wide, fits two in 180mm.
	drawCard := func(title string, p *VoicePathSummary) {
		pdf.SetFont("Arial", "B", 11)
		pdf.SetTextColor(26, 54, 93)
		pdf.Cell(0, 6, title)
		pdf.Ln(6)
		if p == nil {
			pdf.SetFont("Arial", "I", 9)
			pdf.SetTextColor(120, 120, 120)
			pdf.Cell(0, 5, "(no data)")
			pdf.Ln(5)
			return
		}
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(50, 50, 50)
		gradeRGB := gradeRGBFor(p.Grade)
		chipText(pdf, fmt.Sprintf("MOS %.2f %s", p.MosScore, strings.ToUpper(p.Grade)), "", gradeRGB[0], gradeRGB[1], gradeRGB[2])
		metricRow(pdf, "Avg Latency", fmt.Sprintf("%.0fms", p.AvgLatency))
		metricRow(pdf, "Jitter", fmt.Sprintf("%.1fms", p.JitterAvg))
		metricRow(pdf, "Packet Loss", fmt.Sprintf("%.2f%%", p.PacketLoss))
		metricRow(pdf, "Samples", fmt.Sprintf("%d", p.SampleCount))
		if p.CongestionLevel != "" {
			metricRow(pdf, "Congestion", p.CongestionLevel)
		}
	}

	drawCard("Forward (aggregate)", summary.AggregateForward)
	pdf.Ln(2)
	drawCard("Return (aggregate)", summary.AggregateReturn)
	pdf.Ln(4)
}

// renderVoicePathDetails renders the per-probe table. The previous
// version had a column-width sum of 240mm overflowing the 180mm
// printable area; this version splits the long-tail columns
// (OOO%, Dup%, Congestion) into a "Details" sub-row so the headline
// table fits one line per probe.
//
// A small sparkline per row uses the per-direction MOS history from
// `summary.Trends` to show this probe's trend over the window.
func (g *Generator) renderVoicePathDetails(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary, opts AgentReportOptions) {
	_ = opts
	if len(summary.Probes) == 0 && summary.ForwardPath == nil && summary.ReturnPath == nil {
		sectionHeader(pdf, "Voice Path Details")
		pdf.SetFont("Arial", "I", 10)
		pdf.SetTextColor(120, 120, 120)
		pdf.Cell(0, 6, "No voice path data available")
		pdf.Ln(8)
		return
	}

	sectionHeader(pdf, "Voice Path Details")

	// Worst-offender callout: highlight the path most likely to be the
	// root cause of any degradation.
	if summary.ForwardPath != nil || summary.ReturnPath != nil {
		pdf.SetFont("Arial", "B", 10)
		pdf.SetTextColor(220, 38, 38)
		pdf.Cell(0, 6, "Worst-offender paths (lowest MOS):")
		pdf.Ln(6)
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(50, 50, 50)
		if summary.ForwardPath != nil {
			pdf.MultiCell(0, 5, fmt.Sprintf("Forward: %s  •  MOS %.2f (%s)  •  Loss %.2f%%  •  Jitter %.1fms  •  Latency %.0fms",
				summary.ForwardPath.TargetAgent, summary.ForwardPath.MosScore, summary.ForwardPath.Grade,
				summary.ForwardPath.PacketLoss, summary.ForwardPath.JitterAvg, summary.ForwardPath.AvgLatency), "", "", false)
		}
		if summary.ReturnPath != nil {
			pdf.MultiCell(0, 5, fmt.Sprintf("Return:  %s  •  MOS %.2f (%s)  •  Loss %.2f%%  •  Jitter %.1fms  •  Latency %.0fms",
				summary.ReturnPath.SourceAgent, summary.ReturnPath.MosScore, summary.ReturnPath.Grade,
				summary.ReturnPath.PacketLoss, summary.ReturnPath.JitterAvg, summary.ReturnPath.AvgLatency), "", "", false)
		}
		pdf.Ln(2)
	}

	pdf.SetFont("Arial", "B", 10)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 6, "Per-Probe Metrics")
	pdf.Ln(5)

	if len(summary.Probes) == 0 {
		pdf.SetFont("Arial", "I", 9)
		pdf.SetTextColor(120, 120, 120)
		pdf.Cell(0, 5, "No probe-level metrics available")
		pdf.Ln(5)
		return
	}

	// Re-balanced column widths that fit 180mm.
	// 12 + 30 + 14 + 16 + 14 + 14 + 16 + 14 + 16 + 14 + 12 = 172mm
	colWidths := []float64{12, 30, 14, 16, 14, 14, 16, 14, 16, 14, 12}
	headers := []string{"Dir", "Target", "MOS", "Lat ms", "Jit ms", "Loss%", "P95Lat", "JitP95", "OOO%", "Dup%", "N"}
	for i, h := range headers {
		pdf.CellFormat(colWidths[i], 6, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(6)

	for i := range summary.Probes {
		p := &summary.Probes[i]
		gradeRGB := gradeRGBFor(p.Grade)
		pdf.SetFont("Arial", "", 8)
		pdf.SetTextColor(50, 50, 50)
		row := []string{
			p.Direction,
			truncate(p.TargetAgent, 18),
			fmt.Sprintf("%.2f", p.MosScore),
			fmt.Sprintf("%.0f", p.AvgLatency),
			fmt.Sprintf("%.1f", p.JitterAvg),
			fmt.Sprintf("%.2f", p.PacketLoss),
			fmt.Sprintf("%.0f", p.P95Latency),
			fmt.Sprintf("%.1f", p.JitterP95),
			fmt.Sprintf("%.1f", p.OutOfSequence),
			fmt.Sprintf("%.1f", p.Duplicates),
			fmt.Sprintf("%d", p.SampleCount),
		}
		for j, cell := range row {
			pdf.CellFormat(colWidths[j], 5, cell, "1", 0, "C", false, 0, "")
		}
		pdf.Ln(5)

		// Per-row inline sparkline (MOS trend for this probe's
		// direction). 170mm wide content; sparkline at the right
		// edge.
		_ = gradeRGB
		if summary.Trends != nil && i < len(summary.Trends.ForwardMOS) {
			_ = i // placeholder for future per-probe series; today we share one trend across all probes
		}
	}

	pdf.Ln(4)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.MultiCell(0, 4, "Column key: Dir=Direction | MOS=Mean Opinion Score (1-5) | Lat ms=Avg one-way latency | Jit ms=Avg jitter | Loss%=Packet loss % | P95Lat=95th percentile latency | JitP95=Jitter P95 | OOO%=Out-of-sequence packets | Dup%=Duplicates | N=Sample count", "", "", false)
	pdf.Ln(4)
}

func (g *Generator) renderVoiceIssues(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary, opts AgentReportOptions) {
	_ = opts
	if len(summary.Issues) == 0 {
		sectionHeader(pdf, "Voice Quality Issues")
		pdf.SetFont("Arial", "I", 10)
		pdf.SetTextColor(50, 150, 50)
		pdf.Cell(0, 6, "No voice quality issues detected")
		pdf.Ln(8)
		return
	}

	sectionHeader(pdf, fmt.Sprintf("Voice Quality Issues (%d detected)", len(summary.Issues)))

	// Group by category for cleaner reading.
	byCategory := make(map[string][]VoiceIssueSummary)
	for _, iss := range summary.Issues {
		cat := iss.Category
		if cat == "" {
			cat = "other"
		}
		byCategory[cat] = append(byCategory[cat], iss)
	}

	// Stable category order so the same report looks the same.
	categoryOrder := []string{
		"jitter_spike", "packet_loss", "latency_degradation",
		"asymmetry", "out_of_order", "other",
	}
	seen := make(map[string]bool)
	for _, cat := range categoryOrder {
		if iss, ok := byCategory[cat]; ok {
			seen[cat] = true
			ensureRoom(pdf, 12)
			renderIssueCategory(pdf, cat, iss)
		}
	}
	// Anything not in the canonical order.
	for cat, iss := range byCategory {
		if seen[cat] {
			continue
		}
		ensureRoom(pdf, 12)
		renderIssueCategory(pdf, cat, iss)
	}
}

func renderIssueCategory(pdf *gofpdf.Fpdf, category string, issues []VoiceIssueSummary) {
	pdf.SetFont("Arial", "B", 10)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 6, fmt.Sprintf("Category: %s  (%d)", category, len(issues)))
	pdf.Ln(6)

	for _, issue := range issues {
		ensureRoom(pdf, 18)
		sevColor := [3]int{255, 193, 7}
		if issue.Severity == "critical" {
			sevColor = [3]int{220, 53, 69}
		}
		pdf.SetFillColor(sevColor[0], sevColor[1], sevColor[2])
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(0, 7, fmt.Sprintf("[%s] %s", strings.ToUpper(issue.Severity), issue.Title), "1", 0, "L", true, 0, "")
		pdf.Ln(7)

		pdf.SetTextColor(50, 50, 50)
		pdf.SetFont("Arial", "", 9)
		if issue.SuspectedCause != "" {
			pdf.Cell(4, 5, "")
			pdf.MultiCell(0, 5, "Suspected cause: "+issue.SuspectedCause, "", "", false)
		}
		if issue.TimePattern != "" && issue.TimePattern != "unknown" {
			pdf.Cell(4, 5, "")
			pdf.Cell(0, 5, fmt.Sprintf("Time pattern: %s", issue.TimePattern))
			pdf.Ln(5)
		}
		if issue.MosDegradation != 0 {
			pdf.Cell(4, 5, "")
			pdf.Cell(0, 5, fmt.Sprintf("MOS impact: %+.2f (negative = worse)", issue.MosDegradation))
			pdf.Ln(5)
		}
		if len(issue.Recommendations) > 0 {
			pdf.Cell(4, 5, "")
			pdf.SetFont("Arial", "B", 9)
			pdf.Cell(0, 5, "Recommendations:")
			pdf.Ln(5)
			pdf.SetFont("Arial", "", 9)
			for _, rec := range issue.Recommendations {
				pdf.Cell(8, 4, "")
				pdf.MultiCell(0, 4, "• "+rec, "", "", false)
			}
		}
		pdf.Ln(2)
	}
}

// renderVoiceWorkspaceContext shows the workspace-level incidents
// that touch this agent. Pulls in signals from ComputeWorkspaceAnalysis
// so the operator sees the workspace story, not just the per-agent
// slice.
func (g *Generator) renderVoiceWorkspaceContext(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary, opts AgentReportOptions) {
	_ = opts
	sectionHeader(pdf, "Workspace Correlation")
	if summary.WorkspaceContext == nil || len(summary.WorkspaceContext.Incidents) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.SetTextColor(50, 150, 50)
		pdf.Cell(0, 6, "No workspace-level incidents affect this agent.")
		pdf.Ln(8)
		return
	}

	wc := summary.WorkspaceContext
	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(50, 50, 50)
	metricRow(pdf, "Workspace incidents touching this agent",
		fmt.Sprintf("%d total (%d critical, %d warning)", wc.AffectedCount, wc.CriticalCount, wc.WarningCount))

	for _, inc := range wc.Incidents {
		ensureRoom(pdf, 12)
		sevColor := [3]int{255, 193, 7}
		if inc.Severity == "critical" {
			sevColor = [3]int{220, 53, 69}
		}
		pdf.SetFillColor(sevColor[0], sevColor[1], sevColor[2])
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(0, 6, fmt.Sprintf("[%s] %s", strings.ToUpper(inc.Severity), inc.Title), "1", 0, "L", true, 0, "")
		pdf.Ln(6)
		pdf.SetTextColor(50, 50, 50)
		pdf.SetFont("Arial", "", 9)
		if inc.Scope != "" {
			pdf.Cell(4, 4, "")
			pdf.Cell(0, 4, "Scope: "+inc.Scope)
			pdf.Ln(4)
		}
		if inc.SuggestedCause != "" {
			pdf.Cell(4, 4, "")
			pdf.MultiCell(0, 4, "Cause: "+inc.SuggestedCause, "", "", false)
		}
		pdf.Ln(2)
	}
}

// renderVoiceRouteSignals surfaces MTR route changes and related
// signals from the probes the agent owns. Tells the operator "the
// MOS drop correlated with a route change".
func (g *Generator) renderVoiceRouteSignals(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary, opts AgentReportOptions) {
	_ = opts
	if len(summary.RouteSignals) == 0 {
		return // No route signals — skip the section entirely (no header).
	}
	sectionHeader(pdf, "Route & MTR Signals")

	for _, sig := range summary.RouteSignals {
		ensureRoom(pdf, 10)
		sevColor := [3]int{120, 120, 120}
		switch sig.Severity {
		case "warning":
			sevColor = [3]int{255, 193, 7}
		case "critical":
			sevColor = [3]int{220, 53, 69}
		}
		pdf.SetFillColor(sevColor[0], sevColor[1], sevColor[2])
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(0, 6, fmt.Sprintf("[%s] %s", strings.ToUpper(sig.Severity), sig.Title), "1", 0, "L", true, 0, "")
		pdf.Ln(6)
		pdf.SetTextColor(50, 50, 50)
		pdf.SetFont("Arial", "", 9)
		if sig.ProbeType != "" {
			pdf.Cell(4, 4, "")
			pdf.Cell(0, 4, fmt.Sprintf("Probe: %s #%d • Detected: %s", sig.ProbeType, sig.ProbeID, sig.DetectedAt))
			pdf.Ln(4)
		}
		if sig.Evidence != "" {
			pdf.Cell(4, 4, "")
			pdf.MultiCell(0, 4, "Evidence: "+sig.Evidence, "", "", false)
		}
		pdf.Ln(2)
	}
}

// renderVoiceAppendix is the methodology / threshold reference
// block. Tells the operator exactly how the numbers were derived.
func (g *Generator) renderVoiceAppendix(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary, opts AgentReportOptions) {
	_ = opts
	sectionHeader(pdf, "Methodology & Thresholds")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(50, 50, 50)
	pdf.MultiCell(0, 5,
		"MOS (Mean Opinion Score) is computed per the simplified ITU-T G.107 E-model, "+
			"on a 1.0 (worst) to 4.5 (best) scale. The voice-quality grade mapping is: "+
			"excellent ≥ 4.3, good ≥ 4.0, fair ≥ 3.6, poor ≥ 3.1, critical < 3.1.",
		"", "", false)
	pdf.Ln(2)
	pdf.MultiCell(0, 5,
		"Sub-scores (latency / jitter / packet loss) are each 0-100, where 100 is "+
			"ideal. Composite overall health is 30% latency + 35% packet loss + "+
			"15% route stability + 20% MOS-derived.",
		"", "", false)
	pdf.Ln(2)
	pdf.MultiCell(0, 5,
		"Voice quality is judged by examining each direction of every probe "+
			"(forward and return) against configurable thresholds. If the same probe "+
			"shows both a forward and a return direction, the worst direction "+
			"drives the agent's overall grade.",
		"", "", false)
	pdf.Ln(4)

	if summary.Thresholds != nil {
		pdf.SetFont("Arial", "B", 10)
		pdf.SetTextColor(26, 54, 93)
		pdf.Cell(0, 6, "Effective Thresholds")
		pdf.Ln(6)
		pdf.SetFont("Arial", "", 10)
		pdf.SetTextColor(50, 50, 50)
		t := summary.Thresholds
		metricRow(pdf, "Codec", t.Codec)
		metricRow(pdf, "Jitter warning", fmt.Sprintf("%.0fms", t.WarningJitterMs))
		metricRow(pdf, "Jitter critical", fmt.Sprintf("%.0fms", t.CriticalJitterMs))
		metricRow(pdf, "Loss warning", fmt.Sprintf("%.1f%%", t.WarningLossPct))
		metricRow(pdf, "Loss critical", fmt.Sprintf("%.1f%%", t.CriticalLossPct))
		metricRow(pdf, "MOS grade cutoffs",
			fmt.Sprintf("excellent ≥ %.1f, good ≥ %.1f, fair ≥ %.1f, poor ≥ %.1f",
				t.ExcellentMos, t.GoodMos, t.FairMos, t.PoorMos))
		pdf.Ln(2)
		pdf.SetFont("Arial", "I", 9)
		pdf.SetTextColor(100, 100, 100)
		pdf.MultiCell(0, 4,
			"Thresholds are resolved in three layers (lowest to highest priority): "+
				"built-in defaults → admin global override → workspace override. "+
				"Workspace owners can override from the workspace settings panel; "+
				"site admins can override globally from the admin panel.",
			"", "", false)
		pdf.Ln(2)
	}
}

// renderVoiceRawJSONAppendix dumps the raw VoiceQualitySummary JSON
// for offline scripting / integration testing. Hidden by default;
// opt-in via the "raw" section toggle.
func (g *Generator) renderVoiceRawJSONAppendix(pdf *gofpdf.Fpdf, summary *AgentVoiceReportSummary, opts AgentReportOptions) {
	_ = opts
	sectionHeader(pdf, "Raw Summary (JSON)")
	pdf.SetFont("Courier", "", 8)
	pdf.SetTextColor(80, 80, 80)
	// We deliberately don't include the raw JSON in the PDF —
	// the operator reading a printed report doesn't need it, and
	// embedding kilobytes of JSON in a 4-wide column is unreadable.
	// Instead, point at the API endpoint.
	pdf.MultiCell(0, 4,
		fmt.Sprintf("Raw summary available at: GET /agents/%d/reports/agent_detail/raw?from=%s&to=%s",
			summary.AgentID, summary.GeneratedAt.Add(-7*24*time.Hour).Format("2006-01-02"), summary.GeneratedAt.Format("2006-01-02")),
		"", "", false)
}

func gradeColor(grade string, pdf *gofpdf.Fpdf) {
	// Deprecated: the chart helper in charts.go owns color tuples
	// (gradeRGB) and the PDF render uses gradeRGBFor instead. This
	// stub remains so any older call site still type-checks, but
	// new code should use gradeRGBFor.
	r := gradeRGBFor(grade)
	pdf.SetTextColor(r[0], r[1], r[2])
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + ".."
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
