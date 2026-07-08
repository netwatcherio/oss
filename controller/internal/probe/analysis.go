// internal/probe/analysis.go
// AI Analysis engine — health vector computation from ClickHouse telemetry data.
// Produces workspace-level and probe-level health scores, signals, and findings.
package probe

import (
	"context"
	"math"
	"time"

	"netwatcher-controller/internal/llm"

	log "github.com/sirupsen/logrus"
)

// llmProvider is the optional LLM provider for enriching analysis summaries.
// Nil by default (disabled). Set via SetLLMProvider during startup.
var llmProvider llm.Provider

// GeoIPResolver is a minimal interface satisfied by *geoip.Store. Decoupling
// keeps the probe package free of an import cycle on geoip (which itself
// depends on agent types) and lets tests inject a stub.
type GeoIPResolver interface {
	HasASN() bool
	LookupASN(ipStr string) (asn uint, org string, ok bool)
}

// SetLLMProvider configures the optional LLM provider for analysis enrichment.
func SetLLMProvider(p llm.Provider) {
	llmProvider = p
	if p != nil && p.Available() {
		log.Infof("[analysis] LLM enrichment enabled (provider: %s)", p.Name())
	}
}

// enrichWithLLM attempts to get a natural language summary from the LLM.
// Returns empty string on any error (caller falls back to rule-based message).
func enrichWithLLM(ctx context.Context, status StatusSummary, incidents []DetectedIncident, agents []AgentHealthSummary, health HealthVector, totalProbes int) string {
	incidentSummaries := make([]llm.IncidentSummary, len(incidents))
	for i, inc := range incidents {
		incidentSummaries[i] = llm.IncidentSummary{
			Title:           inc.Title,
			Severity:        inc.Severity,
			Scope:           inc.Scope,
			SuggestedCause:  inc.SuggestedCause,
			AffectedAgents:  inc.AffectedAgents,
			AffectedTargets: inc.AffectedTargets,
			Evidence:        inc.Evidence,
		}
	}

	onlineCount := 0
	for _, a := range agents {
		if a.IsOnline {
			onlineCount++
		}
	}

	req := llm.SummarizeRequest{
		Incidents:    incidentSummaries,
		HealthScore:  health.OverallHealth,
		HealthGrade:  health.Grade,
		Status:       status.Status,
		TotalAgents:  len(agents),
		OnlineAgents: onlineCount,
		TotalProbes:  totalProbes,
	}

	enriched, err := llmProvider.Summarize(ctx, req)
	if err != nil {
		log.Warnf("[analysis] LLM enrichment failed (falling back to rule-based): %v", err)
		return ""
	}
	return enriched
}

// ── Health Vector Model ──

// HealthVector is a multi-dimensional health score for any monitored path
type HealthVector struct {
	LatencyScore    float64 `json:"latency_score"`     // 0-100
	PacketLossScore float64 `json:"packet_loss_score"` // 0-100
	RouteStability  float64 `json:"route_stability"`   // 0-100
	MosScore        float64 `json:"mos_score"`         // 1.0-4.5
	OverallHealth   float64 `json:"overall_health"`    // 0-100
	Grade           string  `json:"grade"`             // excellent/good/fair/poor/critical
}

// ProbeMetrics holds raw metrics for a single probe direction
type ProbeMetrics struct {
	AvgLatency    float64 `json:"avg_latency"`    // ms
	MedianLatency float64 `json:"median_latency"` // ms
	P95Latency    float64 `json:"p95_latency"`    // ms
	P99Latency    float64 `json:"p99_latency"`    // ms
	PacketLoss    float64 `json:"packet_loss"`    // percentage
	JitterAvg     float64 `json:"jitter_avg"`     // ms (stddev)
	JitterMedian  float64 `json:"jitter_median"`  // ms
	JitterP95     float64 `json:"jitter_p95"`     // ms
	SampleCount   int     `json:"sample_count"`
}

// AnalysisSignal represents a detected signal (anomaly, artifact, etc.)
type AnalysisSignal struct {
	Type       string  `json:"type"`     // icmp_artifact, route_change, high_loss, high_latency, jitter_anomaly
	Severity   string  `json:"severity"` // info, warning, critical
	Title      string  `json:"title"`
	Evidence   string  `json:"evidence"`
	Confidence float64 `json:"confidence"` // 0-1.0
	HopNumber  int     `json:"hop_number,omitempty"`
}

// AnalysisFinding is a diagnostic conclusion from data analysis
type AnalysisFinding struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Severity string   `json:"severity"` // info, warning, critical
	Category string   `json:"category"` // performance, routing, measurement_artifact
	Summary  string   `json:"summary"`
	Evidence []string `json:"evidence"`
	Steps    []string `json:"recommended_steps"`
}

// MtrPathAnalysis contains route-level diagnostic data from MTR traces
type MtrPathAnalysis struct {
	HopCount           int         `json:"hop_count"`
	UniqueRoutes       int         `json:"unique_routes"`
	RouteStabilityPct  float64     `json:"route_stability_pct"`
	AvgEndHopLatency   float64     `json:"avg_end_hop_latency"`
	AvgEndHopLoss      float64     `json:"avg_end_hop_loss"`
	AvgEndHopJitterAvg float64     `json:"avg_end_hop_jitter"` // stddev from end hop
	TraceCount         int         `json:"trace_count"`        // number of MTR traces analysed
	RateLimitedHops    []int       `json:"rate_limited_hops"`
	TimeoutSegments    []string    `json:"timeout_segments"`
	LatestHopsDetail   []HopDetail `json:"latest_hops_detail,omitempty"` // Enriched hop info with agent names
}

// ── Probe-level Analysis ──

// ProbeAnalysis is the complete analysis result for a single probe direction
type ProbeAnalysis struct {
	ProbeID      uint             `json:"probe_id"`
	ProbeType    string           `json:"probe_type"`
	Target       string           `json:"target"`
	AgentID      uint             `json:"agent_id"`
	AgentName    string           `json:"agent_name"`
	Health       HealthVector     `json:"health"`
	Metrics      ProbeMetrics     `json:"metrics"`
	PathAnalysis *MtrPathAnalysis `json:"path_analysis,omitempty"`
	Reverse      *ProbeAnalysis   `json:"reverse,omitempty"`
	// CombinedHealth merges forward and reverse health for bidirectional probes,
	// weighted toward the worse direction (a path is only as usable as its worse
	// direction). Nil when no reverse data exists.
	CombinedHealth *HealthVector     `json:"combined_health,omitempty"`
	Signals        []AnalysisSignal  `json:"signals"`
	Findings       []AnalysisFinding `json:"findings"`
	GeneratedAt    time.Time         `json:"generated_at"`
}

// ── Workspace-level Analysis ──

// ProbeHealthEntry is a lightweight probe summary for workspace health
type ProbeHealthEntry struct {
	ProbeID   uint         `json:"probe_id"`
	Target    string       `json:"target"`
	ProbeType string       `json:"probe_type"`
	Health    HealthVector `json:"health"`
	Metrics   ProbeMetrics `json:"metrics"`
}

// AgentHealthSummary is the health summary for a single agent
type AgentHealthSummary struct {
	AgentID     uint               `json:"agent_id"`
	AgentName   string             `json:"agent_name"`
	IsOnline    bool               `json:"is_online"`
	Health      HealthVector       `json:"health"`
	ProbeCount  int                `json:"probe_count"`
	WorstProbes []ProbeHealthEntry `json:"worst_probes"`
}

// DetectedIncident is a correlated event detected across agents/probes
type DetectedIncident struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Severity        string   `json:"severity"` // info, warning, critical
	Scope           string   `json:"scope"`    // infrastructure, agent-specific, target-specific
	SuggestedCause  string   `json:"suggested_cause"`
	AffectedAgents  []string `json:"affected_agents"`
	AffectedTargets []string `json:"affected_targets"`
	Evidence        []string `json:"evidence"`
	Recommendations []string `json:"recommendations"`
	Confidence      float64  `json:"confidence"`       // 0-1.0, based on proportion of agents affected
	LookbackMinutes int      `json:"lookback_minutes"` // time window being analyzed
	MatchedCriteria string   `json:"matched_criteria"` // what triggered the incident (e.g., "packet_loss > 1%")
}

// StatusSummary is a high-level "what's happening right now" overview
type StatusSummary struct {
	Status       string `json:"status"`  // healthy, degraded, outage, unknown
	Message      string `json:"message"` // Human-readable summary sentence
	ActiveIssues int    `json:"active_issues"`
}

// WorkspaceAnalysis is the complete workspace-level AI status overview
type WorkspaceAnalysis struct {
	WorkspaceID   uint                 `json:"workspace_id"`
	OverallHealth HealthVector         `json:"overall_health"`
	Status        StatusSummary        `json:"status"`
	Incidents     []DetectedIncident   `json:"incidents"`
	Agents        []AgentHealthSummary `json:"agents"`
	TotalProbes   int                  `json:"total_probes"`
	TotalAgents   int                  `json:"total_agents"`
	GeneratedAt   time.Time            `json:"generated_at"`
}

// ── Scoring Functions ──

// scoreLatency converts avg latency (ms) into 0-100 score
func scoreLatency(avgMs, p95Ms, jitterMs float64) float64 {
	// Composite: 50% avg, 30% p95, 20% jitter
	avgScore := latencyToScore(avgMs)
	p95Score := latencyToScore(p95Ms)
	jitterScore := jitterToScore(jitterMs)
	return clampScore(avgScore*0.5 + p95Score*0.3 + jitterScore*0.2)
}

func latencyToScore(ms float64) float64 {
	if ms <= 0 {
		return 100
	}
	switch {
	case ms < 30:
		return 100 - (ms/30)*5 // 95-100
	case ms < 80:
		return 95 - ((ms-30)/50)*15 // 80-95
	case ms < 150:
		return 80 - ((ms-80)/70)*20 // 60-80
	case ms < 300:
		return 60 - ((ms-150)/150)*30 // 30-60
	default:
		return math.Max(0, 30-((ms-300)/200)*30) // 0-30
	}
}

func jitterToScore(ms float64) float64 {
	if ms <= 0 {
		return 100
	}
	switch {
	case ms < 5:
		return 100
	case ms < 15:
		return 90 - ((ms-5)/10)*10
	case ms < 30:
		return 80 - ((ms-15)/15)*20
	case ms < 50:
		return 60 - ((ms-30)/20)*20
	default:
		return math.Max(0, 40-((ms-50)/50)*40)
	}
}

// scorePacketLoss converts loss % into 0-100 score
func scorePacketLoss(lossPct float64) float64 {
	if lossPct <= 0 {
		return 100
	}
	switch {
	case lossPct < 0.1:
		return 100
	case lossPct < 1:
		return 95 - ((lossPct-0.1)/0.9)*10
	case lossPct < 3:
		return 85 - ((lossPct-1)/2)*15
	case lossPct < 5:
		return 70 - ((lossPct-3)/2)*20
	default:
		return math.Max(0, 50-((lossPct-5)/10)*50)
	}
}

// computeMos computes E-model MOS from latency, loss, jitter
// Simplified ITU-T G.107 E-model
func computeMos(latencyMs, lossPct, jitterMs float64) float64 {
	// Effective latency including jitter buffer
	effectiveLatency := latencyMs + jitterMs*2 + 10 // 10ms codec delay

	// R-factor calculation (simplified)
	r := 93.2 - effectiveLatency/40.0

	// Loss impact
	if lossPct > 0 {
		r -= lossPct * 2.5
	}

	// Clamp R
	if r < 0 {
		r = 0
	}
	if r > 100 {
		r = 100
	}

	// R to MOS conversion
	mos := 1.0 + 0.035*r + r*(r-60)*(100-r)*7e-6
	if mos < 1.0 {
		mos = 1.0
	}
	if mos > 4.5 {
		mos = 4.5
	}
	return math.Round(mos*100) / 100
}

// gradeFromScore converts an overall 0-100 score into a grade string
func gradeFromScore(score float64) string {
	switch {
	case score >= 90:
		return "excellent"
	case score >= 75:
		return "good"
	case score >= 55:
		return "fair"
	case score >= 35:
		return "poor"
	default:
		return "critical"
	}
}

func clampScore(s float64) float64 {
	if s < 0 {
		return 0
	}
	if s > 100 {
		return 100
	}
	return math.Round(s*10) / 10
}

// computeHealthVector builds a HealthVector from raw metrics
func computeHealthVector(metrics ProbeMetrics, routeStability float64) HealthVector {
	latScore := scoreLatency(metrics.AvgLatency, metrics.P95Latency, metrics.JitterAvg)
	lossScore := scorePacketLoss(metrics.PacketLoss)
	mos := computeMos(metrics.AvgLatency, metrics.PacketLoss, metrics.JitterAvg)

	// Weighted composite: 30% latency, 35% loss, 15% route stability, 20% MOS-derived
	mosScore := (mos - 1.0) / 3.5 * 100 // Normalize MOS 1-4.5 to 0-100
	overall := clampScore(latScore*0.30 + lossScore*0.35 + routeStability*0.15 + mosScore*0.20)

	return HealthVector{
		LatencyScore:    clampScore(latScore),
		PacketLossScore: clampScore(lossScore),
		RouteStability:  clampScore(routeStability),
		MosScore:        mos,
		OverallHealth:   overall,
		Grade:           gradeFromScore(overall),
	}
}
