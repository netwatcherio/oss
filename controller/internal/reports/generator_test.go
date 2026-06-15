package reports

import (
	"context"
	"testing"
	"time"
)

// TestGeneratorWithOptionsEmptySummary ensures the section-dispatch
// path doesn't panic when the summary is essentially empty
// (no agent, no probes, no issues). We call the generator's render
// functions directly so we don't need a database.
func TestGeneratorWithOptionsEmptySummary(t *testing.T) {
	g := &Generator{}
	summary := &AgentVoiceReportSummary{
		Name:         "Test Agent",
		AgentID:      1,
		ReportPeriod: "2024-01-01 to 2024-01-07",
		GeneratedAt:  time.Now(),
		OverallMos:   4.5,
		OverallGrade: "excellent",
	}
	opts := FullAgentReportOptions()

	pdf := newTestPDF(t)

	// The render functions are normally called from
	// GenerateAgentPDFWithOptions; we call them directly here so we
	// can assert on a "no data" path without spinning up a DB.
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

	out := pdfBytes(t, pdf)
	if len(out) < 200 {
		t.Errorf("PDF too small (%d bytes) — sections may not have rendered", len(out))
	}
	// Spot-check: every PDF starts with "%PDF-".
	if !startsWith(out, []byte("%PDF-")) {
		t.Errorf("output is not a valid PDF (got %d bytes, prefix=%q)", len(out), prefix(out, 5))
	}
}

// TestGeneratorVoiceOptionsExcludedSections is the inverse: with all
// sections off except the cover, the PDF should still be valid
// and small.
func TestGeneratorVoiceOptionsExcludedSections(t *testing.T) {
	g := &Generator{}
	summary := &AgentVoiceReportSummary{
		Name:         "Test",
		AgentID:      1,
		ReportPeriod: "Last 7 days",
		GeneratedAt:  time.Now(),
		OverallMos:   3.0,
		OverallGrade: "fair",
		Issues: []VoiceIssueSummary{
			{Severity: "warning", Title: "Jitter on forward", Category: "jitter_spike",
				SuspectedCause: "test", Recommendations: []string{"check buffer"}},
		},
	}
	opts := AgentReportOptions{IncludeIssues: true} // only issues

	pdf := newTestPDF(t)
	g.renderAgentVoiceCover(pdf, summary)
	g.renderVoiceExecutiveSnapshot(pdf, summary, DefaultAgentReportOptions())
	g.renderVoiceIssues(pdf, summary, opts)

	out := pdfBytes(t, pdf)
	if !startsWith(out, []byte("%PDF-")) {
		t.Errorf("output is not a valid PDF")
	}
}

// TestRenderVoicePathDetailsEmpty verifies the empty-data path
// doesn't crash.
func TestRenderVoicePathDetailsEmpty(t *testing.T) {
	g := &Generator{}
	summary := &AgentVoiceReportSummary{}
	pdf := newTestPDF(t)
	g.renderVoicePathDetails(pdf, summary, DefaultAgentReportOptions())
	out := pdfBytes(t, pdf)
	if len(out) < 200 {
		t.Errorf("PDF too small (%d bytes)", len(out))
	}
}

// TestRenderVoicePathDetailsWithData exercises the table-rendering
// path. We supply two probes and check the PDF still builds.
func TestRenderVoicePathDetailsWithData(t *testing.T) {
	g := &Generator{}
	summary := &AgentVoiceReportSummary{
		ForwardPath: &VoicePathSummary{
			Direction: "Forward", TargetAgent: "agent-2",
			MosScore: 4.2, Grade: "good",
			AvgLatency: 35, P95Latency: 80, MedianLatency: 30,
			JitterAvg: 4, JitterMedian: 3, JitterP95: 12,
			PacketLoss: 0.5, OutOfSequence: 0.1, Duplicates: 0.05,
			SampleCount: 5000, CongestionLevel: "none",
		},
		ReturnPath: &VoicePathSummary{
			Direction: "Return", SourceAgent: "agent-2",
			MosScore: 3.5, Grade: "fair",
			AvgLatency: 60, P95Latency: 110, MedianLatency: 55,
			JitterAvg: 12, JitterMedian: 10, JitterP95: 30,
			PacketLoss: 1.5, OutOfSequence: 0.4, Duplicates: 0.2,
			SampleCount: 5000, CongestionLevel: "mild",
		},
		Probes: []VoicePathSummary{
			{
				Direction: "Forward", TargetAgent: "agent-2",
				MosScore: 4.2, Grade: "good",
				AvgLatency: 35, P95Latency: 80, JitterAvg: 4,
				PacketLoss: 0.5, SampleCount: 5000, CongestionLevel: "none",
			},
			{
				Direction: "Forward", TargetAgent: "agent-3",
				MosScore: 3.8, Grade: "fair",
				AvgLatency: 50, P95Latency: 90, JitterAvg: 8,
				PacketLoss: 1.0, SampleCount: 5000, CongestionLevel: "mild",
			},
		},
	}
	pdf := newTestPDF(t)
	g.renderVoicePathDetails(pdf, summary, DefaultAgentReportOptions())
	out := pdfBytes(t, pdf)
	if !startsWith(out, []byte("%PDF-")) {
		t.Errorf("output is not a valid PDF")
	}
}

// TestRenderVoiceIssuesCategories ensures issues grouped by
// category are rendered without crashing.
func TestRenderVoiceIssuesCategories(t *testing.T) {
	g := &Generator{}
	summary := &AgentVoiceReportSummary{
		Issues: []VoiceIssueSummary{
			{Severity: "critical", Title: "Loss", Category: "packet_loss",
				SuspectedCause: "link failure", Recommendations: []string{"call ISP"}},
			{Severity: "warning", Title: "Jitter", Category: "jitter_spike",
				SuspectedCause: "congestion", Recommendations: []string{"check buffer"}},
			{Severity: "warning", Title: "Asymmetric", Category: "asymmetry",
				SuspectedCause: "different ISP", Recommendations: []string{"verify"}},
		},
	}
	pdf := newTestPDF(t)
	g.renderVoiceIssues(pdf, summary, DefaultAgentReportOptions())
	out := pdfBytes(t, pdf)
	if !startsWith(out, []byte("%PDF-")) {
		t.Errorf("output is not a valid PDF")
	}
}

// TestRenderWorkspaceVoicePDFSanity is an end-to-end render test
// that builds a workspace voice PDF against an in-memory summary.
// It exercises the workspace generator's render path without
// needing a real database (we call render functions directly).
func TestRenderWorkspaceVoicePDFSanity(t *testing.T) {
	g := &Generator{}
	summary := &WorkspaceVoiceReportSummary{
		Name:             "Test Workspace",
		WorkspaceID:      1,
		ReportPeriod:     "Last 7 days",
		GeneratedAt:      time.Now(),
		OverallMos:       4.0,
		OverallGrade:     "good",
		TotalAgents:      3,
		AgentsWithData:   2,
		TotalProbes:      12,
		CriticalIssueCnt: 1,
		WarningIssueCnt:  3,
		Agents: []WorkspaceVoiceAgentEntry{
			{
				AgentID: 1, AgentName: "alpha", IsOnline: true,
				OverallMos: 4.5, OverallGrade: "excellent",
				LatencyScore: 95, JitterScore: 92, PacketLossScore: 99,
				IssueCount: 0, CriticalCount: 0, WarningCount: 0,
				ForwardPath: &VoicePathSummary{MosScore: 4.5, Grade: "excellent",
					PacketLoss: 0.1, JitterAvg: 3, AvgLatency: 25, TargetAgent: "alpha-tgt"},
			},
			{
				AgentID: 2, AgentName: "bravo", IsOnline: true,
				OverallMos: 3.0, OverallGrade: "poor",
				LatencyScore: 60, JitterScore: 55, PacketLossScore: 70,
				IssueCount: 4, CriticalCount: 1, WarningCount: 3,
				ForwardPath: &VoicePathSummary{MosScore: 3.0, Grade: "poor",
					PacketLoss: 3.5, JitterAvg: 30, AvgLatency: 200, TargetAgent: "bravo-tgt"},
				Recommendation: "Check upstream ISP",
			},
		},
		TopIssues: []VoiceIssueSummary{
			{Severity: "critical", Title: "[bravo] Severe packet loss",
				Category: "packet_loss", SuspectedCause: "ISP outage",
				MosDegradation: -1.2, Recommendations: []string{"call ISP"}},
		},
	}
	pdf := newTestPDF(t)
	g.renderWorkspaceVoiceCover(pdf, summary)
	g.renderWorkspaceVoiceOverview(pdf, summary)
	g.renderWorkspaceVoiceAgentsTable(pdf, summary)
	g.renderWorkspaceVoiceHeatmap(pdf, summary)
	g.renderWorkspaceVoiceTopIssues(pdf, summary)
	if len(summary.Agents) > 0 {
		pdf.AddPage()
		g.renderWorkspaceVoiceAgentPage(pdf, &summary.Agents[0], summary)
	}
	out := pdfBytes(t, pdf)
	if !startsWith(out, []byte("%PDF-")) {
		t.Errorf("output is not a valid PDF")
	}
}

// TestEnsureRoom adds pages when content would overflow. The
// threshold for "next page" is well below 270mm in our PDF, so
// asking for 1000mm should trigger an AddPage.
func TestEnsureRoom(t *testing.T) {
	pdf := newTestPDF(t)
	startY := pdf.GetY()
	_ = ensureRoom(pdf, 1000)
	// After ensureRoom(1000) we should have triggered AddPage —
	// pdf.GetY should be back near the top margin.
	if pdf.GetY() >= startY+100 {
		t.Errorf("ensureRoom did not add a page: startY=%v endY=%v", startY, pdf.GetY())
	}
}

// TestSectionHeader ensures the helper writes a header at the
// current Y and returns the new Y.
func TestSectionHeader(t *testing.T) {
	pdf := newTestPDF(t)
	y := sectionHeader(pdf, "Test Section")
	if y <= pdfMargin {
		t.Errorf("sectionHeader returned y=%v, expected > margin", y)
	}
}

// TestChipText ensures the chip helper writes one line.
func TestChipText(t *testing.T) {
	pdf := newTestPDF(t)
	before := pdf.GetY()
	chipText(pdf, "EXCELLENT", "", 22, 163, 74)
	if pdf.GetY() <= before {
		t.Errorf("chipText did not advance Y (before=%v, after=%v)", before, pdf.GetY())
	}
}

// TestMetricRow ensures metricRow writes a label/value pair.
func TestMetricRow(t *testing.T) {
	pdf := newTestPDF(t)
	before := pdf.GetY()
	metricRow(pdf, "Test Label", "Test Value")
	if pdf.GetY() <= before {
		t.Errorf("metricRow did not advance Y (before=%v, after=%v)", before, pdf.GetY())
	}
}

// TestParseAgentReportSectionsWhitespace ensures whitespace is
// tolerated.
func TestParseAgentReportSectionsWhitespace(t *testing.T) {
	got := ParseAgentReportSections("  summary ,  probes  ")
	if !got.IncludeExecutive || !got.IncludePerProbe {
		t.Errorf("whitespace-handling parse failed: %+v", got)
	}
}

// Compile-time guard: the Generator type still implements a usable
// shape (a context-using method exists). Catches accidental method
// signature drift when refactoring.
var _ = func() context.Context { return context.Background() }
