package probe

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ── Voice Quality Types ──────────────────────────────────────────────────────

// VoicePathDirection distinguishes forward vs return path
type VoicePathDirection string

const (
	VoicePathForward VoicePathDirection = "forward"
	VoicePathReturn  VoicePathDirection = "return"
)

// CongestionLevel represents how congested a path is for voice traffic
type CongestionLevel string

const (
	CongestionNone     CongestionLevel = "none"
	CongestionMild     CongestionLevel = "mild"
	CongestionModerate CongestionLevel = "moderate"
	CongestionSevere   CongestionLevel = "severe"
)

// VoicePathMetrics holds voice-relevant metrics for one direction (forward or return)
type VoicePathMetrics struct {
	Direction       VoicePathDirection `json:"direction"`
	TargetAgentID   uint               `json:"target_agent_id"`
	TargetAgentName string             `json:"target_agent_name"`
	SourceAgentID   uint               `json:"source_agent_id"`
	SourceAgentName string             `json:"source_agent_name"`
	ProbeID         uint               `json:"probe_id"`
	ProbeType       string             `json:"probe_type"`
	MosScore        float64            `json:"mos_score"`      // 1.0-5.0
	AvgLatency      float64            `json:"avg_latency_ms"` // ms (one-way estimated as RTT/2)
	P95Latency      float64            `json:"p95_latency_ms"` // ms
	MedianLatency   float64            `json:"median_latency_ms"`
	JitterAvg       float64            `json:"jitter_avg_ms"` // ms
	JitterMedian    float64            `json:"jitter_median_ms"`
	JitterP95       float64            `json:"jitter_p95_ms"`
	PacketLoss      float64            `json:"packet_loss_pct"`
	OutOfSequence   float64            `json:"out_of_sequence_pct"`
	Duplicates      float64            `json:"duplicate_pct"`
	SampleCount     int                `json:"sample_count"`
	MosContributors []string           `json:"mos_contributing_factors"` // e.g., "latency>150ms", "jitter>20ms"
	CongestionLevel CongestionLevel    `json:"congestion_level"`

	// Burst-loss metrics captured from the agent's TrafficSim cycle
	// (max consecutive loss packets, total bursts in window). Surfaced
	// in the report and consumed by the burst-loss heuristic.
	MaxConsecutiveLoss int `json:"max_consecutive_loss,omitempty"`
	TotalBursts        int `json:"total_bursts,omitempty"`
}

// VoicePairSummary is the voice-quality report view of a single
// source-agent ↔ target pair. It is the per-pair shape that the panel
// renders in `voice-quality-report-multi.html`. One pair = one probe
// with optional forward/reverse directions.
type VoicePairSummary struct {
	ID             string              `json:"id"`
	Agent          AgentRef            `json:"agent"`
	Target         TargetRef           `json:"target"`
	Forward        *VoicePathMetrics   `json:"forward,omitempty"`
	Reverse        *VoicePathMetrics   `json:"reverse,omitempty"`
	Issues         []VoiceQualityIssue `json:"issues"`
	Baseline       *BaselineDelta      `json:"baseline,omitempty"`
	Trends         *VoiceTrends        `json:"trends,omitempty"`
	RouteSignals   []RouteSignal       `json:"route_signals,omitempty"`
	Thresholds     VoiceThresholds     `json:"thresholds"`
	OverallMos     float64             `json:"overall_mos"`
	OverallGrade   string              `json:"overall_grade"`
	Recommendation string              `json:"recommendation,omitempty"`
	TimePattern    string              `json:"time_pattern,omitempty"`
}

// AgentRef is a minimal agent reference embedded in VoicePairSummary.
// Avoids pulling the full agent row into the JSON payload.
type AgentRef struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	IP       string `json:"ip,omitempty"`
	Location string `json:"location,omitempty"`
}

// TargetRef describes the remote end of a voice pair. Either AgentID
// is set (when the target is a known netwatcher agent) or Host/IP are
// set (when it's an arbitrary SIP/VoIP endpoint).
type TargetRef struct {
	Name      string `json:"name"`
	Host      string `json:"host,omitempty"`
	IP        string `json:"ip,omitempty"`
	AgentID   uint   `json:"agent_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
}

// VoiceBucket is one time-bucketed series sample used for the
// MOS timeline / jitter-loss charts in the voice PDF report and for
// time-of-day pattern detection in the heuristics engine.
type VoiceBucket struct {
	Timestamp string  `json:"timestamp"` // ISO-8601 UTC
	Forward   float64 `json:"forward"`   // MOS for the forward direction
	Return    float64 `json:"return"`    // MOS for the return direction
	LatencyMs float64 `json:"latency_ms"`
	JitterMs  float64 `json:"jitter_ms"`
	LossPct   float64 `json:"loss_pct"`
}

// VoiceTrends groups the per-direction time-series data the
// analysis engine collected for the agent during the report window.
// All buckets are sample-weighted means within their time slice.
type VoiceTrends struct {
	BucketMinutes int           `json:"bucket_minutes"`
	Forward       []VoiceBucket `json:"forward"`
	Return        []VoiceBucket `json:"return"`
	Combined      []VoiceBucket `json:"combined"`
	IssueBuckets  []string      `json:"issue_buckets"` // timestamps of detected issues (for chart overlay)
}

// BaselineDelta captures the per-metric change between the analysis
// window and the prior 7-day baseline for the same agent. Positive
// numbers = better, negative = worse. `Trend` is a one-word summary
// ("improving", "stable", "worsening", "unknown") for executive
// summary text.
type BaselineDelta struct {
	From            time.Time `json:"from"`
	To              time.Time `json:"to"`
	MosDelta        float64   `json:"mos_delta"`
	LatencyDeltaMs  float64   `json:"latency_delta_ms"`
	JitterDeltaMs   float64   `json:"jitter_delta_ms"`
	LossDeltaPct    float64   `json:"loss_delta_pct"`
	SampleCount     int       `json:"sample_count"`
	BaselineSamples int       `json:"baseline_samples"`
	Trend           string    `json:"trend"` // improving | stable | worsening | unknown
	PercentChange   float64   `json:"percent_change"`
}

// WorkspaceIncidentContext is the slice of workspace-level incidents
// from ComputeWorkspaceAnalysis that touch this agent. It's surfaced
// in the voice report so the operator sees the workspace story, not
// just the per-agent slice.
type WorkspaceIncidentContext struct {
	AffectedCount int                `json:"affected_count"`
	CriticalCount int                `json:"critical_count"`
	WarningCount  int                `json:"warning_count"`
	Incidents     []DetectedIncident `json:"incidents"`
}

// RouteSignal is one MTR route change or routing-related signal
// associated with a probe the agent is monitoring. Surfaced in the
// voice report so route changes are not invisible.
type RouteSignal struct {
	ProbeID    uint      `json:"probe_id"`
	ProbeType  string    `json:"probe_type"`
	Type       string    `json:"type"`     // route_change, hop_count_change, isp_change, ip_change
	Severity   string    `json:"severity"` // info, warning, critical
	Title      string    `json:"title"`
	Evidence   string    `json:"evidence"`
	DetectedAt time.Time `json:"detected_at"`
}

// VoiceQualityIssue is a detected voice quality abnormality
type VoiceQualityIssue struct {
	ID              string             `json:"id"`
	Severity        string             `json:"severity"` // info, warning, critical
	Title           string             `json:"title"`
	Category        string             `json:"category"` // jitter_spike, packet_loss, latency_degradation, asymmetry, out_of_order, burst_loss
	AffectedPath    VoicePathDirection `json:"affected_path"`
	TargetAgentName string             `json:"target_agent_name"`
	SuspectedCause  string             `json:"suspected_cause"`
	Evidence        []string           `json:"evidence"`
	TimePattern     string             `json:"time_pattern"` // "constant", "business_hours", "off_hours", "periodic_30min", "unknown"
	FirstDetected   time.Time          `json:"first_detected"`
	LastDetected    time.Time          `json:"last_detected"`
	MosDegradation  float64            `json:"mos_degradation"` // delta from baseline (negative = worse)
	MosBefore       float64            `json:"mos_before"`
	MosAfter        float64            `json:"mos_after"`
	Recommendations []string           `json:"recommendations"`

	// Heuristic enrichment (populated by the new detectors):
	//
	// LossPattern classifies the temporal shape of packet-loss issues
	// once we have enough time-series samples: "burst", "steady", or
	// "mixed". Empty when the burst detector didn't run.
	LossPattern string `json:"loss_pattern,omitempty"`
	// LikelyHop is the MTR hop index most strongly correlated with this
	// issue (0 when no hop correlation is available). Populated by
	// correlateWithRoute.
	LikelyHop int `json:"likely_hop,omitempty"`
	// HopEvidence is a human-readable summary of what the correlated
	// hop showed during the issue's window.
	HopEvidence string `json:"hop_evidence,omitempty"`
	// DurationBuckets is how many of the time-series buckets in the
	// analysis window the issue was present in. Used by the severity
	// scorer to factor in "how persistent" the issue was.
	DurationBuckets int `json:"duration_buckets,omitempty"`
	// TotalBuckets is the size of the bucket series the duration is
	// computed against, so the panel can render "issue present in N/M
	// buckets" without a second pass.
	TotalBuckets int `json:"total_buckets,omitempty"`
}

// VoiceQualitySummary is the complete voice quality assessment for an agent.
//
// For backward compatibility, the fields `ForwardPath` and `ReturnPath`
// now point to the WORST-offender probe in each direction (the one most
// likely to explain any degradation observed). For sample-weighted
// aggregates across all probes, see `AggregateForward` / `AggregateReturn`.
// For a full per-probe breakdown, see `Probes`.
type VoiceQualitySummary struct {
	AgentID         uint    `json:"agent_id"`
	AgentName       string  `json:"agent_name"`
	OverallMos      float64 `json:"overall_mos"`       // weighted average of forward + return
	OverallGrade    string  `json:"overall_grade"`     // excellent/good/fair/poor/critical
	LatencyScore    float64 `json:"latency_score"`     // 0-100
	JitterScore     float64 `json:"jitter_score"`      // 0-100
	PacketLossScore float64 `json:"packet_loss_score"` // 0-100

	// Per-direction rollups. `*Path` is the worst-offender (preserved
	// name for backward compat); `Aggregate*` is the sample-weighted
	// average; `Probes` is the full per-probe list.
	ForwardPath      *VoicePathMetrics  `json:"forward_path,omitempty"`
	ReturnPath       *VoicePathMetrics  `json:"return_path,omitempty"`
	AggregateForward *VoicePathMetrics  `json:"aggregate_forward,omitempty"`
	AggregateReturn  *VoicePathMetrics  `json:"aggregate_return,omitempty"`
	Probes           []VoicePathMetrics `json:"probes"` // all probe-level metrics

	// Time series for the MOS timeline / jitter-loss charts. Optional
	// (only populated when ComputeAgentVoiceQuality is called with a
	// large enough window to bucket).
	Trends *VoiceTrends `json:"trends,omitempty"`

	// 7-day baseline comparison for the trend arrow in the executive
	// summary. Computed per-probe and rolled up; nil if no baseline data
	// was available.
	BaselineComparison *BaselineDelta `json:"baseline_comparison,omitempty"`

	// Route / MTR signals for probes the agent owns. Lets the report
	// show that a MOS drop correlated with a route change.
	RouteSignals []RouteSignal `json:"route_signals,omitempty"`

	// Workspace-level incidents that touch this agent. Lets the report
	// point the operator at workspace-wide issues (e.g., a peering
	// problem affecting multiple agents).
	WorkspaceContext *WorkspaceIncidentContext `json:"workspace_context,omitempty"`

	// Per-probe / per-pair rollup. One entry per (source-agent, target)
	// tuple, with both forward and reverse directions and the per-pair
	// issue list. This is the shape the multi-target voice report
	// template renders against; for single-target agents it has one
	// entry.
	Pairs []VoicePairSummary `json:"pairs,omitempty"`

	Issues         []VoiceQualityIssue `json:"issues"`
	TimePattern    string              `json:"time_pattern"` // "constant", "mixed", "periodic", "unknown"
	Recommendation string              `json:"recommendation"`
	ThresholdsUsed *VoiceThresholds    `json:"thresholds_used,omitempty"` // effective thresholds for this run
	GeneratedAt    time.Time           `json:"generated_at"`
}

// AgentAnalysis is per-agent analysis including voice quality
type AgentAnalysis struct {
	AgentID          uint                 `json:"agent_id"`
	AgentName        string               `json:"agent_name"`
	IsOnline         bool                 `json:"is_online"`
	Health           HealthVector         `json:"health"`
	VoiceQuality     *VoiceQualitySummary `json:"voice_quality,omitempty"`
	Probes           []ProbeAnalysis      `json:"probes"`
	ReturnPathProbes []ProbeAnalysis      `json:"return_path_probes"`
	Incidents        []DetectedIncident   `json:"incidents"`
	GeneratedAt      time.Time            `json:"generated_at"`
}

// buildWorkspaceIncidentContext pulls the workspace's recent
// DetectedIncident list and filters it down to ones that touch this
// agent. Used to give the agent voice report a workspace-level
// context section.
func buildWorkspaceIncidentContext(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint, agentName string, from time.Time) *WorkspaceIncidentContext {
	// Look up the workspace ID for this agent.
	var wsID uint
	if err := db.WithContext(ctx).Table("agents").Select("workspace_id").Where("id = ?", agentID).Scan(&wsID).Error; err != nil || wsID == 0 {
		return nil
	}
	wa, err := ComputeWorkspaceAnalysis(ctx, ch, db, wsID, int(time.Since(from).Minutes()))
	if err != nil || wa == nil {
		return nil
	}
	ctxOut := &WorkspaceIncidentContext{}
	for _, inc := range wa.Incidents {
		matched := false
		for _, a := range inc.AffectedAgents {
			if a == agentName || a == agentObjName(a) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		ctxOut.Incidents = append(ctxOut.Incidents, inc)
		if inc.Severity == "critical" {
			ctxOut.CriticalCount++
		} else if inc.Severity == "warning" {
			ctxOut.WarningCount++
		}
		ctxOut.AffectedCount++
	}
	if len(ctxOut.Incidents) == 0 {
		return nil
	}
	return ctxOut
}

// agentObjName is a no-op shim; kept for clarity in
// buildWorkspaceIncidentContext's "is this incident about THIS agent?"
// matching. AffectedAgents currently stores agent names verbatim, so
// equality is what we want; the shim is here so the call site reads
// naturally and we have one place to change if the matching logic
// evolves (e.g., agent ID matching).
func agentObjName(s string) string { return s }

// workspaceSettingsFor fetches the raw Settings JSON for the agent's
// workspace. Returns nil if the agent has no workspace (shouldn't
// happen in practice).
func workspaceSettingsFor(ctx context.Context, db *gorm.DB, agentID uint) []byte {
	var wsID uint
	if err := db.WithContext(ctx).Table("agents").Select("workspace_id").Where("id = ?", agentID).Scan(&wsID).Error; err != nil || wsID == 0 {
		return nil
	}
	var settings []byte
	row := db.WithContext(ctx).Table("workspaces").Select("settings").Where("id = ?", wsID).Row()
	if row == nil {
		return nil
	}
	if err := row.Scan(&settings); err != nil {
		return nil
	}
	return settings
}

// fetchMtrHopSummariesForVoice looks up the MTR hop data for the
// probe the worst-offender voice path is running over, and converts
// it to the small `MtrHopSummary` shape the voice hop correlator
// needs.
//
// Returns nil when MTR data is unavailable (no row, query failure,
// or no enriched hop details) — the voice report degrades cleanly
// without hop evidence rather than failing the whole report.
func fetchMtrHopSummariesForVoice(ctx context.Context, ch *sql.DB, agentID uint, path *VoicePathMetrics, from time.Time) []MtrHopSummary {
	if path == nil || path.ProbeID == 0 {
		return nil
	}
	// Best-effort fetch; failure logs and returns nil.
	defer func() {
		// MTR analysis can panic on malformed payloads (it reads
		// untrusted JSON). Recover to keep the voice report
		// resilient — the report is more valuable than missing hop
		// evidence.
		if r := recover(); r != nil {
			log.Warnf("[voice] MTR hop correlation panicked: %v", r)
		}
	}()

	agentIPToID, agentByID := buildAgentMaps(ctx, ch, []uint{agentID})
	pathAnalysis, _, err := analyzeMtrForProbe(ctx, ch, []uint{agentID}, path.ProbeID, from, agentIPToID, agentByID)
	if err != nil || pathAnalysis == nil {
		return nil
	}
	out := make([]MtrHopSummary, 0, len(pathAnalysis.LatestHopsDetail))
	for i, h := range pathAnalysis.LatestHopsDetail {
		if h.IP == "" {
			continue
		}
		out = append(out, MtrHopSummary{
			HopNumber:    i + 1,
			IP:           h.IP,
			Hostname:     h.Hostname,
			LossPct:      h.Loss,
			AvgLatencyMs: h.Latency,
		})
	}
	return out
}

// buildAgentMaps produces the agentIPToID and agentByID maps that
// analyzeMtrForProbe expects. Walks the agent table once and
// returns empty maps on failure (the MTR analyzer tolerates empty
// maps).
func buildAgentMaps(ctx context.Context, ch *sql.DB, agentIDs []uint) (map[string]uint, map[uint]agentInfo) {
	ipToID := make(map[string]uint)
	idToInfo := make(map[uint]agentInfo)
	if ch == nil {
		return ipToID, idToInfo
	}
	// Walk the agents table by id (best-effort — agentInfo is just
	// used to label hops with human names).
	idList := make([]string, 0, len(agentIDs))
	for _, id := range agentIDs {
		idList = append(idList, fmt.Sprintf("%d", id))
	}
	q := fmt.Sprintf(`
SELECT id, name
FROM agents
WHERE id IN (%s)
`, strings.Join(idList, ","))
	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return ipToID, idToInfo
	}
	defer rows.Close()
	for rows.Next() {
		var id uint
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		idToInfo[id] = agentInfo{ID: id, Name: name}
	}
	return ipToID, idToInfo
}

// mosToRFactor is reserved for future use; currently the panel
// derives R-Factor from MOS at render time. The server-side
// computeMos call uses G.107 simplified E-model which doesn't
// pre-compute R-Factor — the JSON report sends MOS and the panel
// converts to R-Factor via the standard monotonic formula in
// panel/src/utils/mos.ts.
//
// Keeping the helper here (commented out) so the inverse-mapping
// work we did during the AGENT-probe fallback isn't lost.

// func mosToRFactor(mos float64) float64 {
// 	if mos <= 1.0 {
// 		return 0
// 	}
// 	if mos >= 4.5 {
// 		return 93.2
// 	}
// 	switch {
// 	case mos >= 4.3:
// 		return 92 + (mos-4.3)*10
// 	case mos >= 4.0:
// 		return 80 + (mos-4.0)*40
// 	case mos >= 3.6:
// 		return 70 + (mos-3.6)*25
// 	case mos >= 3.1:
// 		return 60 + (mos-3.1)*20
// 	case mos >= 2.0:
// 		return 30 + (mos-2.0)*60
// 	default:
// 		return 0 + mos*15
// 	}
// }

// median returns the median of a slice. Sort-not-mutate variant.
func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := make([]float64, len(vals))
	copy(cp, vals)
	sort.Float64s(cp)
	if len(cp)%2 == 1 {
		return cp[len(cp)/2]
	}
	return (cp[len(cp)/2-1] + cp[len(cp)/2]) / 2
}
