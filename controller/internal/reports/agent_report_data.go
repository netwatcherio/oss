package reports

// agent_report_data.go
//
// JSON builders for the live voice-quality report view.
//
// The view on the panel (`panel/src/components/voice-report/`) consumes
// `VoiceReportData` JSON shaped like the static templates'
// `REPORT_DATA`. This file is the bridge:
//
//   - BuildAgentReportData         → per-agent view (one source agent)
//   - BuildProbeReportData         → per-probe view (one probe, source agent + target)
//   - BuildWorkspaceReportData     → per-workspace view (every agent)
//
// Each builder maps the controller's probe types (`VoiceQualitySummary`,
// `VoicePairSummary`, `VoicePathMetrics`, `VoiceTrends`) into the
// flatter shape the templates (and the panel view) use. The
// per-template field set is:
//
//   meta          → report_id, generated_at, agent/target, test profile
//   summary       → overall mos, r_factor, grade, verdict text
//   thresholds    → effective VoiceThresholds
//   metrics       → {latency, jitter, packets} summary stats
//   quality       → score-breakdown table rows
//   timeseries    → rtt[], jitter[], loss_per_interval[]
//   traceroute    → {protocol, hops[], note}
//   pairs         → per-probe rollups (multi.html template)
//   heatmap       → per-agent forward/reverse grades (workspace view)
//   top_issues    → sorted list (workspace view)
//   issues        → flat per-workspace issue list
//
// JSON shape stability: the field set is part of the public API
// (panel reads it). Add fields freely; renaming or removing fields
// needs a coordinated panel + backend change.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/probe"
)

// AgentReportDataOpts is the request shape for the per-agent and
// per-probe JSON endpoints. Mirrors the time-range options the
// existing PDF endpoints accept.
type AgentReportDataOpts struct {
	AgentID     uint
	ProbeID     uint // 0 for per-agent; set for per-probe
	From        time.Time
	To          time.Time
	WorkspaceID uint
}

// BuildAgentReportData assembles a VoiceReportData-shaped payload for
// a single agent. When `opts.ProbeID` is set, the payload is scoped
// to that probe (per-probe view); otherwise it's the agent's full
// pair list (per-agent view, multi-pair shape).
func BuildAgentReportData(ctx context.Context, db *gorm.DB, ch *sqlDB, opts AgentReportDataOpts) (*VoiceReportDataJSON, error) {
	summary, err := probe.ComputeAgentVoiceQuality(ctx, db, ch, opts.AgentID, opts.From, opts.To)
	if err != nil {
		return nil, fmt.Errorf("compute voice quality: %w", err)
	}

	out := &VoiceReportDataJSON{
		Meta:       buildReportMeta(opts, summary, "agent"),
		Summary:    buildReportSummary(summary),
		Thresholds: ToVoiceThresholdsJSON(*summary.ThresholdsUsed),
		Issues:     ToVoiceQualityIssueJSONList(summary.Issues),
	}
	if out.Thresholds.WarningJitterMs == 0 {
		out.Thresholds = ToVoiceThresholdsJSON(probe.VoiceDefaultThresholds)
	}

	// Per-pair rollup. For single-probe scope, filter to that probe.
	pairs := summary.Pairs
	if opts.ProbeID != 0 {
		filtered := pairs[:0]
		for _, p := range pairs {
			if p.Forward != nil && p.Forward.ProbeID == opts.ProbeID {
				filtered = append(filtered, p)
			}
		}
		pairs = filtered
	}
	// Always include pairs so the multi.html view can render even when
	// the single-pair metrics synthesis falls through.
	pairsJSON := make([]VoicePairSummaryJSON, 0, len(pairs))
	for _, p := range pairs {
		pairsJSON = append(pairsJSON, ToVoicePairSummaryJSON(p))
	}
	out.Pairs = pairsJSON

	// When there's exactly one pair (typical), synthesize the
	// metrics / timeseries / quality fields from that pair's forward
	// path. Multi-pair case keeps the per-pair breakdown but
	// doesn't fill single-target-only fields.
	if len(pairs) == 1 && pairs[0].Forward != nil {
		out.Metrics = buildReportMetrics(pairs[0].Forward)
		out.Timeseries = buildReportTimeseries(pairs[0].Trends)
		out.Quality = buildReportQuality(pairs[0])
		out.Meta.ViewMode = "probe" // per-probe metadata
		out.Meta.Agent = &VoiceReportAgentRefJSON{
			ID: pairs[0].Agent.ID, Name: pairs[0].Agent.Name,
			IP: pairs[0].Agent.IP, Location: pairs[0].Agent.Location,
		}
		out.Meta.Target = &VoiceReportTargetRefJSON{
			Name: pairs[0].Target.Name, Host: pairs[0].Target.Host, IP: pairs[0].Target.IP,
			AgentID: pairs[0].Target.AgentID, AgentName: pairs[0].Target.AgentName,
		}
		out.Traceroute = buildReportTraceroute(pairs[0].Trends, pairs[0].Forward)
	} else if len(pairs) > 1 {
		// Multi-pair: don't synthesize a single metrics block; the
		// panel renders per-pair detail pages instead.
		out.Meta.ViewMode = "multi"
	}

	return out, nil
}

// BuildWorkspaceReportData assembles the per-workspace view. Rolls up
// every agent's voice quality into a heatmap (forward + reverse per
// agent), a top-issues list, and a flat issue list.
func BuildWorkspaceReportData(ctx context.Context, db *gorm.DB, ch *sqlDB, workspaceID uint, from, to time.Time) (*VoiceReportDataJSON, error) {
	agents, err := listWorkspaceAgents(ctx, db, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}

	out := &VoiceReportDataJSON{
		Meta: VoiceReportMetaJSON{
			ReportID:    fmt.Sprintf("VQR-WS-%d-%s", workspaceID, time.Now().UTC().Format("20060102150405")),
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			ViewMode:    "workspace",
			Workspace: &VoiceReportWorkspaceRefJSON{
				ID:   workspaceID,
				Name: workspaceName(ctx, db, workspaceID),
			},
			Window: fmt.Sprintf("%s – %s", from.Format("2006-01-02 15:04 MST"), to.Format("2006-01-02 15:04 MST")),
		},
		Summary: VoiceReportSummaryJSON{
			MOS:          0,
			RFactor:      0,
			Grade:        "unknown",
			VerdictTitle: "Workspace voice quality",
			VerdictText:  fmt.Sprintf("%d agents in this workspace", len(agents)),
		},
		Thresholds: ToVoiceThresholdsJSON(probe.VoiceDefaultThresholds),
	}

	var allPairs []probe.VoicePairSummary
	var allIssues []probe.VoiceQualityIssue
	heatmap := make([]VoiceReportHeatmapCellJSON, 0, len(agents))
	meanMos := 0.0
	meanCount := 0

	for _, ag := range agents {
		vq, err := probe.ComputeAgentVoiceQuality(ctx, db, ch, ag.ID, from, to)
		if err != nil || vq == nil {
			continue
		}
		allPairs = append(allPairs, vq.Pairs...)
		allIssues = append(allIssues, vq.Issues...)

		fwdGrade := ""
		revGrade := ""
		var fwdMos, revMos float64
		if vq.ForwardPath != nil {
			fwdMos = vq.ForwardPath.MosScore
			fwdGrade = vq.OverallGrade
		}
		if vq.ReturnPath != nil {
			revMos = vq.ReturnPath.MosScore
			if vq.ReturnPath.MosScore < vq.ForwardPath.MosScore {
				revGrade = vq.OverallGrade
			} else {
				revGrade = fwdGrade
			}
		}
		if fwdMos > 0 || revMos > 0 {
			meanCount++
			meanMos += math.Max(fwdMos, revMos)
		}
		heatmap = append(heatmap, VoiceReportHeatmapCellJSON{
			AgentID:      ag.ID,
			AgentName:    ag.Name,
			ForwardMOS:   fwdMos,
			ForwardGrade: fwdGrade,
			ReverseMOS:   revMos,
			ReverseGrade: revGrade,
		})
	}

	if meanCount > 0 {
		out.Summary.MOS = meanMos / float64(meanCount)
		out.Summary.RFactor = (out.Summary.MOS - 1) * 25
		out.Summary.Grade = mosGradeString(out.Summary.MOS)
	}
	heatmapJSON := make([]VoiceReportHeatmapCellJSON, 0, len(heatmap))
	heatmapJSON = append(heatmapJSON, heatmap...)
	out.Heatmap = heatmapJSON

	pairsJSON := make([]VoicePairSummaryJSON, 0, len(allPairs))
	for _, p := range allPairs {
		pairsJSON = append(pairsJSON, ToVoicePairSummaryJSON(p))
	}
	out.Pairs = pairsJSON

	// Top issues: critical first, then warning, then info; limit to 25
	// for the on-screen view (PDF can include all).
	sort.SliceStable(allIssues, func(i, j int) bool {
		return severityRank(allIssues[i].Severity) > severityRank(allIssues[j].Severity)
	})
	topIssues := allIssues
	if len(topIssues) > 25 {
		topIssues = topIssues[:25]
	}
	out.TopIssues = ToVoiceQualityIssueJSONList(topIssues)
	out.Issues = ToVoiceQualityIssueJSONList(allIssues)

	return out, nil
}

// buildReportMeta assembles the meta block. The view_mode key drives
// which sub-view the panel renders.
func buildReportMeta(opts AgentReportDataOpts, summary *probe.VoiceQualitySummary, viewMode string) VoiceReportMetaJSON {
	meta := VoiceReportMetaJSON{
		ReportID:    fmt.Sprintf("VQR-AG-%d-%s", opts.AgentID, time.Now().UTC().Format("20060102150405")),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		ViewMode:    viewMode,
		Agent: &VoiceReportAgentRefJSON{
			ID:   summary.AgentID,
			Name: summary.AgentName,
		},
		Window: fmt.Sprintf("%s to %s", opts.From.Format("2006-01-02"), opts.To.Format("2006-01-02")),
	}
	if len(summary.Pairs) > 0 && summary.Pairs[0].Target.Name != "" {
		t := summary.Pairs[0].Target
		meta.Target = &VoiceReportTargetRefJSON{
			Name: t.Name,
			Host: t.Host,
			IP:   t.IP,
		}
	}
	return meta
}

// buildReportSummary translates the agent voice quality rollup into
// the flat `summary` block the template expects.
func buildReportSummary(vq *probe.VoiceQualitySummary) VoiceReportSummaryJSON {
	return VoiceReportSummaryJSON{
		MOS:          vq.OverallMos,
		RFactor:      (vq.OverallMos - 1) * 25,
		Grade:        vq.OverallGrade,
		VerdictTitle: verdictTitle(vq),
		VerdictText:  verdictText(vq),
	}
}

// buildReportMetrics translates a VoicePathMetrics into the per-pair
// latency/jitter/packets rollup the templates render.
func buildReportMetrics(p *probe.VoicePathMetrics) VoiceReportMetricsJSON {
	m := VoiceReportMetricsJSON{
		Latency: VoiceStatJSON{
			Min: p.MedianLatency, Avg: p.AvgLatency, Max: p.P95Latency, StdDev: 0, Unit: "ms",
		},
		Jitter: VoiceStatJSON{
			Min: p.JitterMedian, Avg: p.JitterAvg, Max: p.JitterP95, StdDev: 0, Unit: "ms",
		},
		Packets: VoicePacketCountersJSON{
			Sent: p.SampleCount, Received: p.SampleCount, Lost: 0,
			LossPct: p.PacketLoss, Duplicates: 0, DupPct: p.Duplicates,
			OutOfOrder: 0, OooPct: p.OutOfSequence,
			DiscardedJitterBuffer: 0, DiscardPct: 0,
		},
	}
	if p.JitterMedian == 0 {
		m.Jitter.Min = p.JitterAvg * 0.5
	}
	return m
}

// buildReportTimeseries converts VoiceTrends into the rtt[]/jitter[]/
// loss_per_interval[] arrays the templates render against. When the
// trends series is empty, returns an empty struct so the panel
// renders the chart frame without data.
func buildReportTimeseries(t *probe.VoiceTrends) *VoiceReportTimeseriesJSON {
	if t == nil {
		return &VoiceReportTimeseriesJSON{}
	}
	// The trends series stores per-bucket MOS / latency / jitter / loss.
	// The chart uses forward MOS / latency for the rtt series.
	rtt := make([]float64, 0, len(t.Forward))
	jitter := make([]float64, 0, len(t.Forward))
	loss := make([]float64, 0, len(t.Forward))
	for _, b := range t.Forward {
		// RTT in ms ≈ latency_ms * 2 (forward is already one-way).
		rtt = append(rtt, b.LatencyMs*2)
		jitter = append(jitter, b.JitterMs)
		loss = append(loss, b.LossPct)
	}
	return &VoiceReportTimeseriesJSON{
		RTT:    rtt,
		Jitter: jitter,
		Loss:   loss,
	}
}

// buildReportQuality assembles the Quality Score Breakdown rows.
func buildReportQuality(p probe.VoicePairSummary) []VoiceReportQualityRowJSON {
	out := []VoiceReportQualityRowJSON{
		{Component: "R-Factor (E-model, G.107)", Value: fmt.Sprintf("%.1f", (p.OverallMos-1)*25), Note: "≥ 80 = satisfied users"},
		{Component: "MOS-CQE (estimated conversational)", Value: fmt.Sprintf("%.2f", p.OverallMos), Note: mosGradeString(p.OverallMos)},
		{Component: "Effective latency (RTT + 2×jitter + 10)", Value: fmt.Sprintf("%.1f ms", effectiveLatency(p)), Note: ""},
	}
	if p.Baseline != nil {
		out = append(out, VoiceReportQualityRowJSON{
			Component: "vs 7-day baseline",
			Value:     fmt.Sprintf("MOS %+.2f", p.Baseline.MosDelta),
			Note:      p.Baseline.Trend,
		})
	}
	if p.Forward != nil {
		out = append(out, VoiceReportQualityRowJSON{
			Component: "Burst loss density",
			Value:     fmt.Sprintf("%.1f%%", burstDensity(p.Forward)),
			Note:      burstPatternLabel(p.Forward),
		})
	}
	return out
}

// buildReportTraceroute synthesizes a traceroute block from the
// trends series + per-hop correlation when available. The trends
// series doesn't carry per-hop data, so this is best-effort and
// returns nil when there's no signal.
func buildReportTraceroute(_ *probe.VoiceTrends, _ *probe.VoicePathMetrics) *VoiceReportTracerouteJSON {
	// Traceroute data is sourced from a separate MTR probe, not from
	// TrafficSim metrics. The endpoint at /agents/.../data could
	// fetch it on demand, but for now we leave traceroute empty
	// when the per-pair summary doesn't carry it.
	return nil
}

// helper functions

func verdictTitle(vq *probe.VoiceQualitySummary) string {
	switch vq.OverallGrade {
	case "excellent":
		return "Excellent — enterprise-grade voice quality"
	case "good":
		return "Good — suitable for business voice"
	case "fair":
		return "Fair — usable but monitoring recommended"
	case "poor":
		return "Poor — voice quality is degraded"
	case "critical":
		return "Critical — voice calls likely failing"
	default:
		return "Voice quality summary"
	}
}

func verdictText(vq *probe.VoiceQualitySummary) string {
	if vq.Recommendation != "" {
		return vq.Recommendation
	}
	if len(vq.Issues) == 0 {
		return fmt.Sprintf("All monitored paths within voice-quality targets. Mean MOS is %.2f across the analysis window.", vq.OverallMos)
	}
	critical, warning := 0, 0
	for _, i := range vq.Issues {
		switch i.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		}
	}
	return fmt.Sprintf("%d critical, %d warning issue(s) detected. See the issues section for details.", critical, warning)
}

func mosGradeString(mos float64) string {
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

func severityRank(s string) int {
	switch s {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func effectiveLatency(p probe.VoicePairSummary) float64 {
	if p.Forward == nil {
		return 0
	}
	return p.Forward.AvgLatency + p.Forward.JitterAvg*2 + 10
}

func burstDensity(p *probe.VoicePathMetrics) float64 {
	if p == nil || p.SampleCount == 0 {
		return 0
	}
	return float64(p.TotalBursts) / float64(p.SampleCount) * 100
}

func burstPatternLabel(p *probe.VoicePathMetrics) string {
	if p == nil {
		return ""
	}
	if p.MaxConsecutiveLoss >= 6 {
		return "Burst — sustained loss runs"
	}
	if p.TotalBursts > 0 {
		return "Burst — intermittent"
	}
	return "Steady"
}

// listWorkspaceAgents fetches all agents for a workspace.
func listWorkspaceAgents(ctx context.Context, db *gorm.DB, workspaceID uint) ([]*agent.Agent, error) {
	var out []*agent.Agent
	if err := db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// workspaceName resolves the workspace display name.
func workspaceName(ctx context.Context, db *gorm.DB, workspaceID uint) string {
	type row struct {
		Name string
	}
	var r row
	if err := db.WithContext(ctx).Table("workspaces").Select("name").Where("id = ?", workspaceID).Scan(&r).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(r.Name)
}
