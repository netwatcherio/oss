package reports

import (
	"database/sql"
	"time"

	"netwatcher-controller/internal/probe"
)

// agent_report_data_types.go
//
// JSON wire types for the voice report live view. Mirrors the
// panel/src/components/voice-report/types.ts file 1:1 — any change
// here must be coordinated with the panel view (or vice versa).

// VoiceReportDataJSON is the top-level payload returned by:
//
//	GET /agents/{id}/reports/agent_detail/data
//	GET /probes/{id}/reports/voice/data
//	GET /workspaces/{id}/reports/voice/data
type VoiceReportDataJSON struct {
	Meta           VoiceReportMetaJSON          `json:"meta"`
	Summary        VoiceReportSummaryJSON       `json:"summary"`
	Thresholds     VoiceThresholdsJSON          `json:"thresholds"`
	Metrics        VoiceReportMetricsJSON       `json:"metrics,omitempty"`
	Quality        []VoiceReportQualityRowJSON  `json:"quality,omitempty"`
	Pairs          []VoicePairSummaryJSON       `json:"pairs,omitempty"`
	Timeseries     *VoiceReportTimeseriesJSON   `json:"timeseries,omitempty"`
	Traceroute     *VoiceReportTracerouteJSON   `json:"traceroute,omitempty"`
	Heatmap        []VoiceReportHeatmapCellJSON `json:"heatmap,omitempty"`
	TopIssues      []VoiceQualityIssueJSON      `json:"top_issues,omitempty"`
	Issues         []VoiceQualityIssueJSON      `json:"issues,omitempty"`
	CommonFailures []VoiceCommonFailureJSON     `json:"common_failures,omitempty"`
}

// VoiceCommonFailureJSON is one row in the workspace "common
// failures" block. Ranks recurring issue categories across all
// agents in the window. The panel renders the top-N as a
// recurring-patterns callout so operators see "jitter spike on
// 8/12 agents" without scanning the full list.
type VoiceCommonFailureJSON struct {
	Category       string                    `json:"category"`
	Title          string                    `json:"title"`
	Count          int                       `json:"count"`
	CriticalCount  int                       `json:"critical_count"`
	WarningCount   int                       `json:"warning_count"`
	AffectedAgents []VoiceCommonFailureAgent `json:"affected_agents"`
	SampleIssue    *VoiceQualityIssueJSON    `json:"sample_issue,omitempty"`
}

// VoiceCommonFailureAgent is the minimal context the panel shows
// per affected agent in the common-failures block.
type VoiceCommonFailureAgent struct {
	AgentID    uint    `json:"agent_id"`
	AgentName  string  `json:"agent_name"`
	PairID     string  `json:"pair_id,omitempty"`
	TargetName string  `json:"target_name,omitempty"`
	ProbeID    uint    `json:"probe_id,omitempty"`
	Severity   string  `json:"severity"`
	MOSImpact  float64 `json:"mos_impact"`
}

// VoiceReportMetaJSON is the report header (brand, ID, timestamp,
// source/target, window).
type VoiceReportMetaJSON struct {
	ReportID    string                       `json:"report_id"`
	GeneratedAt string                       `json:"generated_at"`
	ViewMode    string                       `json:"view_mode"` // "probe" | "agent" | "workspace" | "multi"
	Agent       *VoiceReportAgentRefJSON     `json:"agent,omitempty"`
	Target      *VoiceReportTargetRefJSON    `json:"target,omitempty"`
	Workspace   *VoiceReportWorkspaceRefJSON `json:"workspace,omitempty"`
	Test        *VoiceReportTestJSON         `json:"test,omitempty"`
	Window      string                       `json:"window,omitempty"`
}

// VoiceReportAgentRefJSON is the minimal agent reference embedded
// in the meta and per-pair blocks.
type VoiceReportAgentRefJSON struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	IP       string `json:"ip,omitempty"`
	Location string `json:"location,omitempty"`
}

// VoiceReportTargetRefJSON describes the remote end of a voice pair.
type VoiceReportTargetRefJSON struct {
	Name      string `json:"name"`
	Host      string `json:"host,omitempty"`
	IP        string `json:"ip,omitempty"`
	AgentID   uint   `json:"agent_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
}

// VoiceReportWorkspaceRefJSON is the workspace context for the
// workspace-level report.
type VoiceReportWorkspaceRefJSON struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// VoiceReportTestJSON describes the test profile (codec, packet
// size, duration). Populated when the agent's TrafficSim config
// reports it.
type VoiceReportTestJSON struct {
	Type        string `json:"type,omitempty"`
	Codec       string `json:"codec,omitempty"`
	Duration    string `json:"duration,omitempty"`
	Interval    string `json:"interval,omitempty"`
	PacketsSent int    `json:"packets_sent,omitempty"`
	DSCP        string `json:"dscp,omitempty"`
	PayloadSize string `json:"payload_size,omitempty"`
}

// VoiceReportSummaryJSON is the top-line score + verdict.
type VoiceReportSummaryJSON struct {
	MOS          float64 `json:"mos"`
	RFactor      float64 `json:"r_factor"`
	Grade        string  `json:"grade"`
	VerdictTitle string  `json:"verdict_title"`
	VerdictText  string  `json:"verdict_text"`
}

// VoiceReportMetricsJSON is the latency/jitter/packet rollup.
type VoiceReportMetricsJSON struct {
	Latency    VoiceStatJSON           `json:"latency"`
	Jitter     VoiceStatJSON           `json:"jitter"`
	OneWayUp   *VoiceStatJSON          `json:"one_way_up,omitempty"`
	OneWayDown *VoiceStatJSON          `json:"one_way_down,omitempty"`
	Packets    VoicePacketCountersJSON `json:"packets"`
}

// VoiceStatJSON is a single min/avg/max/stddev record.
type VoiceStatJSON struct {
	Min    float64 `json:"min"`
	Avg    float64 `json:"avg"`
	Max    float64 `json:"max"`
	StdDev float64 `json:"stddev"`
	Unit   string  `json:"unit"`
}

// VoicePacketCountersJSON is the per-window packet accounting.
type VoicePacketCountersJSON struct {
	Sent                  int     `json:"sent"`
	Received              int     `json:"received"`
	Lost                  int     `json:"lost"`
	LossPct               float64 `json:"loss_pct"`
	Duplicates            int     `json:"duplicates"`
	DupPct                float64 `json:"dup_pct"`
	OutOfOrder            int     `json:"out_of_order"`
	OooPct                float64 `json:"ooo_pct"`
	DiscardedJitterBuffer int     `json:"discarded_jitter_buffer"`
	DiscardPct            float64 `json:"discard_pct"`
}

// VoiceReportQualityRowJSON is one row in the Quality Score
// Breakdown table.
type VoiceReportQualityRowJSON struct {
	Component string `json:"component"`
	Value     string `json:"value"`
	Note      string `json:"note"`
}

// VoiceReportTimeseriesJSON is the chart data.
type VoiceReportTimeseriesJSON struct {
	RTT    []float64 `json:"rtt,omitempty"`
	Jitter []float64 `json:"jitter,omitempty"`
	Loss   []float64 `json:"loss,omitempty"`
}

// VoiceReportTracerouteJSON is the path-analysis block.
type VoiceReportTracerouteJSON struct {
	Protocol string                         `json:"protocol"`
	Hops     []VoiceReportTracerouteHopJSON `json:"hops"`
	Note     string                         `json:"note,omitempty"`
}

// VoiceReportTracerouteHopJSON is one hop in the traceroute.
type VoiceReportTracerouteHopJSON struct {
	Hop   int     `json:"hop"`
	Host  string  `json:"host"`
	IP    string  `json:"ip"`
	ASN   string  `json:"asn,omitempty"`
	Loss  float64 `json:"loss"`
	Sent  int     `json:"sent"`
	Last  float64 `json:"last"`
	Avg   float64 `json:"avg"`
	Best  float64 `json:"best"`
	Worst float64 `json:"worst"`
	StDev float64 `json:"stdev"`
}

// VoiceReportHeatmapCellJSON is one agent's row in the workspace
// heatmap.
type VoiceReportHeatmapCellJSON struct {
	AgentID      uint    `json:"agent_id"`
	AgentName    string  `json:"agent_name"`
	ForwardMOS   float64 `json:"forward_mos,omitempty"`
	ForwardGrade string  `json:"forward_grade,omitempty"`
	ReverseMOS   float64 `json:"reverse_mos,omitempty"`
	ReverseGrade string  `json:"reverse_grade,omitempty"`
}

// VoicePairSummaryJSON is the per-pair rollup used by the multi.html
// template. Mirrors probe.VoicePairSummary but with JSON-friendly
// nested types.
type VoicePairSummaryJSON struct {
	ID             string                   `json:"id"`
	Agent          VoiceReportAgentRefJSON  `json:"agent"`
	Target         VoiceReportTargetRefJSON `json:"target"`
	Forward        *VoicePathMetricsJSON    `json:"forward,omitempty"`
	Reverse        *VoicePathMetricsJSON    `json:"reverse,omitempty"`
	Issues         []VoiceQualityIssueJSON  `json:"issues"`
	Baseline       *BaselineDeltaJSON       `json:"baseline,omitempty"`
	Trends         *VoiceTrendsJSON         `json:"trends,omitempty"`
	RouteSignals   []RouteSignalJSON        `json:"route_signals,omitempty"`
	Thresholds     VoiceThresholdsJSON      `json:"thresholds"`
	OverallMOS     float64                  `json:"overall_mos"`
	OverallGrade   string                   `json:"overall_grade"`
	Recommendation string                   `json:"recommendation,omitempty"`
	TimePattern    string                   `json:"time_pattern,omitempty"`
}

// VoicePathMetricsJSON is the per-direction metrics for one pair.
type VoicePathMetricsJSON struct {
	Direction              string   `json:"direction"`
	TargetAgentID          uint     `json:"target_agent_id"`
	TargetAgentName        string   `json:"target_agent_name"`
	SourceAgentID          uint     `json:"source_agent_id"`
	SourceAgentName        string   `json:"source_agent_name"`
	ProbeID                uint     `json:"probe_id"`
	ProbeType              string   `json:"probe_type"`
	MosScore               float64  `json:"mos_score"`
	AvgLatencyMs           float64  `json:"avg_latency_ms"`
	P95LatencyMs           float64  `json:"p95_latency_ms"`
	MedianLatencyMs        float64  `json:"median_latency_ms"`
	JitterAvgMs            float64  `json:"jitter_avg_ms"`
	JitterMedianMs         float64  `json:"jitter_median_ms,omitempty"`
	JitterP95Ms            float64  `json:"jitter_p95_ms,omitempty"`
	PacketLossPct          float64  `json:"packet_loss_pct"`
	OutOfSequencePct       float64  `json:"out_of_sequence_pct"`
	DuplicatePct           float64  `json:"duplicate_pct"`
	SampleCount            int      `json:"sample_count"`
	MosContributingFactors []string `json:"mos_contributing_factors,omitempty"`
	CongestionLevel        string   `json:"congestion_level"`
	MaxConsecutiveLoss     int      `json:"max_consecutive_loss,omitempty"`
	TotalBursts            int      `json:"total_bursts,omitempty"`
}

// BaselineDeltaJSON is the per-pair baseline comparison.
type BaselineDeltaJSON struct {
	From            string  `json:"from"`
	To              string  `json:"to"`
	MosDelta        float64 `json:"mos_delta"`
	LatencyDeltaMs  float64 `json:"latency_delta_ms"`
	JitterDeltaMs   float64 `json:"jitter_delta_ms"`
	LossDeltaPct    float64 `json:"loss_delta_pct"`
	SampleCount     int     `json:"sample_count"`
	BaselineSamples int     `json:"baseline_samples"`
	Trend           string  `json:"trend"`
	PercentChange   float64 `json:"percent_change,omitempty"`
}

// VoiceTrendsJSON is the per-direction MOS time series.
type VoiceTrendsJSON struct {
	BucketMinutes int               `json:"bucket_minutes"`
	Forward       []VoiceBucketJSON `json:"forward,omitempty"`
	Return        []VoiceBucketJSON `json:"return,omitempty"`
	Combined      []VoiceBucketJSON `json:"combined,omitempty"`
	IssueBuckets  []string          `json:"issue_buckets,omitempty"`
}

// VoiceBucketJSON is one time-bucketed series sample.
type VoiceBucketJSON struct {
	Timestamp string  `json:"timestamp"`
	Forward   float64 `json:"forward"`
	Return    float64 `json:"return"`
	LatencyMs float64 `json:"latency_ms"`
	JitterMs  float64 `json:"jitter_ms"`
	LossPct   float64 `json:"loss_pct"`
}

// VoiceQualityIssueJSON is one detected voice-quality abnormality.
type VoiceQualityIssueJSON struct {
	ID              string   `json:"id"`
	Severity        string   `json:"severity"`
	Title           string   `json:"title"`
	Category        string   `json:"category"`
	AffectedPath    string   `json:"affected_path"`
	TargetAgentName string   `json:"target_agent_name"`
	SuspectedCause  string   `json:"suspected_cause"`
	Evidence        []string `json:"evidence"`
	TimePattern     string   `json:"time_pattern"`
	FirstDetected   string   `json:"first_detected,omitempty"`
	LastDetected    string   `json:"last_detected,omitempty"`
	MosDegradation  float64  `json:"mos_degradation"`
	MosBefore       float64  `json:"mos_before"`
	MosAfter        float64  `json:"mos_after"`
	Recommendations []string `json:"recommendations"`
	LossPattern     string   `json:"loss_pattern,omitempty"`
	LikelyHop       int      `json:"likely_hop,omitempty"`
	HopEvidence     string   `json:"hop_evidence,omitempty"`
	DurationBuckets int      `json:"duration_buckets,omitempty"`
	TotalBuckets    int      `json:"total_buckets,omitempty"`
}

// RouteSignalJSON is one MTR route change / routing signal.
type RouteSignalJSON struct {
	ProbeID    uint   `json:"probe_id"`
	ProbeType  string `json:"probe_type"`
	Type       string `json:"type"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	Evidence   string `json:"evidence"`
	DetectedAt string `json:"detected_at"`
}

// VoiceThresholdsJSON is the effective thresholds used for the run.
type VoiceThresholdsJSON struct {
	WarningJitterMs        float64 `json:"warning_jitter_ms"`
	CriticalJitterMs       float64 `json:"critical_jitter_ms"`
	JitterSpikeMultiplier  float64 `json:"jitter_spike_multiplier"`
	WarningLossPct         float64 `json:"warning_loss_pct"`
	CriticalLossPct        float64 `json:"critical_loss_pct"`
	NewLossBaselineMaxPct  float64 `json:"new_loss_baseline_max_pct"`
	NewLossCurrentMinPct   float64 `json:"new_loss_current_min_pct"`
	AsymmetryMosRatioMin   float64 `json:"asymmetry_mos_ratio_min"`
	AsymmetryMinForwardMos float64 `json:"asymmetry_min_forward_mos"`
	LatencyOnlyMinMs       float64 `json:"latency_only_min_ms"`
	LatencyOnlyMaxLossPct  float64 `json:"latency_only_max_loss_pct"`
	LatencyOnlyMaxMos      float64 `json:"latency_only_max_mos"`
	OutOfSequencePct       float64 `json:"out_of_sequence_pct"`
	ExcellentMos           float64 `json:"excellent_mos"`
	GoodMos                float64 `json:"good_mos"`
	FairMos                float64 `json:"fair_mos"`
	PoorMos                float64 `json:"poor_mos"`
	CongestionJitterMs     float64 `json:"congestion_jitter_ms"`
	CongestionLossPct      float64 `json:"congestion_loss_pct"`
	CongestionLatencyMs    float64 `json:"congestion_latency_ms"`
	Codec                  string  `json:"codec"`
}

// sqlDB is a thin alias for *sql.DB so the JSON builder helpers can
// accept either a real DB or nil (today they only use the gorm.DB).
type sqlDB = sql.DB

// ----- Converters from probe types to JSON wire types -----

// ToVoicePairSummaryJSON converts a probe.VoicePairSummary to its
// JSON wire form.
func ToVoicePairSummaryJSON(p probe.VoicePairSummary) VoicePairSummaryJSON {
	out := VoicePairSummaryJSON{
		ID:             p.ID,
		Agent:          VoiceReportAgentRefJSON{ID: p.Agent.ID, Name: p.Agent.Name, IP: p.Agent.IP, Location: p.Agent.Location},
		Target:         VoiceReportTargetRefJSON{Name: p.Target.Name, Host: p.Target.Host, IP: p.Target.IP, AgentID: p.Target.AgentID, AgentName: p.Target.AgentName},
		Issues:         ToVoiceQualityIssueJSONList(p.Issues),
		OverallMOS:     p.OverallMos,
		OverallGrade:   p.OverallGrade,
		Recommendation: p.Recommendation,
		TimePattern:    p.TimePattern,
		Thresholds:     ToVoiceThresholdsJSON(p.Thresholds),
	}
	if p.Forward != nil {
		f := ToVoicePathMetricsJSON(*p.Forward)
		out.Forward = &f
	}
	if p.Reverse != nil {
		r := ToVoicePathMetricsJSON(*p.Reverse)
		out.Reverse = &r
	}
	if p.Baseline != nil {
		b := ToBaselineDeltaJSON(*p.Baseline)
		out.Baseline = &b
	}
	if p.Trends != nil {
		t := ToVoiceTrendsJSON(*p.Trends)
		out.Trends = &t
	}
	for _, s := range p.RouteSignals {
		out.RouteSignals = append(out.RouteSignals, ToRouteSignalJSON(s))
	}
	return out
}

// ToVoiceQualityIssueJSONList converts a slice of probe issues.
func ToVoiceQualityIssueJSONList(in []probe.VoiceQualityIssue) []VoiceQualityIssueJSON {
	out := make([]VoiceQualityIssueJSON, 0, len(in))
	for _, i := range in {
		out = append(out, ToVoiceQualityIssueJSON(i))
	}
	return out
}

// ToVoiceQualityIssueJSON converts one probe issue to JSON.
func ToVoiceQualityIssueJSON(i probe.VoiceQualityIssue) VoiceQualityIssueJSON {
	return VoiceQualityIssueJSON{
		ID: i.ID, Severity: i.Severity, Title: i.Title, Category: i.Category,
		AffectedPath: string(i.AffectedPath), TargetAgentName: i.TargetAgentName,
		SuspectedCause: i.SuspectedCause, Evidence: i.Evidence, TimePattern: i.TimePattern,
		FirstDetected: formatTimePtr(i.FirstDetected), LastDetected: formatTimePtr(i.LastDetected),
		MosDegradation: i.MosDegradation, MosBefore: i.MosBefore, MosAfter: i.MosAfter,
		Recommendations: i.Recommendations,
		LossPattern:     i.LossPattern, LikelyHop: i.LikelyHop, HopEvidence: i.HopEvidence,
		DurationBuckets: i.DurationBuckets, TotalBuckets: i.TotalBuckets,
	}
}

// ToRouteSignalJSON converts one route signal.
func ToRouteSignalJSON(s probe.RouteSignal) RouteSignalJSON {
	return RouteSignalJSON{
		ProbeID: s.ProbeID, ProbeType: s.ProbeType, Type: s.Type, Severity: s.Severity,
		Title: s.Title, Evidence: s.Evidence,
		DetectedAt: s.DetectedAt.Format(time.RFC3339),
	}
}

// ToBaselineDeltaJSON converts a probe baseline delta.
func ToBaselineDeltaJSON(b probe.BaselineDelta) BaselineDeltaJSON {
	return BaselineDeltaJSON{
		From: b.From.Format(time.RFC3339), To: b.To.Format(time.RFC3339),
		MosDelta: b.MosDelta, LatencyDeltaMs: b.LatencyDeltaMs,
		JitterDeltaMs: b.JitterDeltaMs, LossDeltaPct: b.LossDeltaPct,
		SampleCount: b.SampleCount, BaselineSamples: b.BaselineSamples,
		Trend: b.Trend, PercentChange: b.PercentChange,
	}
}

// ToVoiceTrendsJSON converts trends.
func ToVoiceTrendsJSON(t probe.VoiceTrends) VoiceTrendsJSON {
	out := VoiceTrendsJSON{
		BucketMinutes: t.BucketMinutes,
		IssueBuckets:  append([]string(nil), t.IssueBuckets...),
	}
	for _, b := range t.Forward {
		out.Forward = append(out.Forward, ToVoiceBucketJSON(b))
	}
	for _, b := range t.Return {
		out.Return = append(out.Return, ToVoiceBucketJSON(b))
	}
	for _, b := range t.Combined {
		out.Combined = append(out.Combined, ToVoiceBucketJSON(b))
	}
	return out
}

// ToVoiceBucketJSON converts one trend bucket.
func ToVoiceBucketJSON(b probe.VoiceBucket) VoiceBucketJSON {
	return VoiceBucketJSON{
		Timestamp: b.Timestamp, Forward: b.Forward, Return: b.Return,
		LatencyMs: b.LatencyMs, JitterMs: b.JitterMs, LossPct: b.LossPct,
	}
}

// ToVoicePathMetricsJSON converts one direction's metrics.
func ToVoicePathMetricsJSON(p probe.VoicePathMetrics) VoicePathMetricsJSON {
	return VoicePathMetricsJSON{
		Direction:     string(p.Direction),
		TargetAgentID: p.TargetAgentID, TargetAgentName: p.TargetAgentName,
		SourceAgentID: p.SourceAgentID, SourceAgentName: p.SourceAgentName,
		ProbeID: p.ProbeID, ProbeType: p.ProbeType,
		MosScore: p.MosScore, AvgLatencyMs: p.AvgLatency, P95LatencyMs: p.P95Latency,
		MedianLatencyMs: p.MedianLatency,
		JitterAvgMs:     p.JitterAvg, JitterMedianMs: p.JitterMedian, JitterP95Ms: p.JitterP95,
		PacketLossPct: p.PacketLoss, OutOfSequencePct: p.OutOfSequence, DuplicatePct: p.Duplicates,
		SampleCount:            p.SampleCount,
		MosContributingFactors: p.MosContributors, CongestionLevel: string(p.CongestionLevel),
		MaxConsecutiveLoss: p.MaxConsecutiveLoss, TotalBursts: p.TotalBursts,
	}
}

// ToVoiceThresholdsJSON converts one thresholds record.
func ToVoiceThresholdsJSON(t probe.VoiceThresholds) VoiceThresholdsJSON {
	return VoiceThresholdsJSON{
		WarningJitterMs: t.WarningJitterMs, CriticalJitterMs: t.CriticalJitterMs,
		JitterSpikeMultiplier: t.JitterSpikeMultiplier,
		WarningLossPct:        t.WarningLossPct, CriticalLossPct: t.CriticalLossPct,
		NewLossBaselineMaxPct: t.NewLossBaselineMaxPct, NewLossCurrentMinPct: t.NewLossCurrentMinPct,
		AsymmetryMosRatioMin: t.AsymmetryMosRatioMin, AsymmetryMinForwardMos: t.AsymmetryMinForwardMos,
		LatencyOnlyMinMs: t.LatencyOnlyMinMs, LatencyOnlyMaxLossPct: t.LatencyOnlyMaxLossPct,
		LatencyOnlyMaxMos: t.LatencyOnlyMaxMos,
		OutOfSequencePct:  t.OutOfSequencePct,
		ExcellentMos:      t.ExcellentMos, GoodMos: t.GoodMos, FairMos: t.FairMos, PoorMos: t.PoorMos,
		CongestionJitterMs: t.CongestionJitterMs, CongestionLossPct: t.CongestionLossPct,
		CongestionLatencyMs: t.CongestionLatencyMs, Codec: t.Codec,
	}
}

func formatTimePtr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
