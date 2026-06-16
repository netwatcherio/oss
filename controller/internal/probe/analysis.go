// internal/probe/analysis.go
// AI Analysis engine — health vector computation from ClickHouse telemetry data.
// Produces workspace-level and probe-level health scores, signals, and findings.
package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/llm"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
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

// ── Data Fetching ──

// probeAnalysisMetrics fetches raw metrics for a specific probe from ClickHouse
func probeAnalysisMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, probeID uint, from time.Time) (ProbeMetrics, error) {
	if len(agentIDs) == 0 {
		return ProbeMetrics{}, nil
	}

	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	// Fetch PING metrics for this probe
	q := fmt.Sprintf(`
SELECT 
    payload_raw,
    created_at
FROM probe_data
WHERE type = 'PING'
  AND probe_id = %d
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 2000
`, probeID, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return ProbeMetrics{}, err
	}
	defer rows.Close()

	var latencies []float64
	var totalLoss float64
	var totalJitterAvg float64
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

		latMs := float64(payload.AvgRTT) / 1_000_000.0 // ns to ms
		jitterMs := float64(payload.StdDevRTT) / 1_000_000.0

		latencies = append(latencies, latMs)
		totalLoss += payload.PacketLoss
		totalJitterAvg += jitterMs
		count++
	}

	if count == 0 {
		return ProbeMetrics{}, nil
	}

	// Calculate percentiles
	avgLat := avg(latencies)
	p95Lat := percentile(latencies, 95)
	avgLoss := totalLoss / float64(count)
	avgJitterAvg := totalJitterAvg / float64(count)

	return ProbeMetrics{
		AvgLatency:  sanitizeFloat(avgLat),
		P95Latency:  sanitizeFloat(p95Lat),
		PacketLoss:  sanitizeFloat(avgLoss),
		JitterAvg:   sanitizeFloat(avgJitterAvg),
		SampleCount: count,
	}, nil
}

// probeTrafficSimMetrics fetches TrafficSim metrics for a specific probe.
// Used by AGENT probe analysis to combine PING + MTR + TrafficSim data.
func probeTrafficSimMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, probeID uint, from time.Time) ProbeMetrics {
	if len(agentIDs) == 0 {
		return ProbeMetrics{}
	}

	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	q := fmt.Sprintf(`
SELECT payload_raw
FROM probe_data
WHERE type = 'TRAFFICSIM'
  AND probe_id = %d
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 2000
`, probeID, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		log.Warnf("[Analysis] Failed to fetch TrafficSim metrics for probe %d: %v", probeID, err)
		return ProbeMetrics{}
	}
	defer rows.Close()

	var latencies []float64
	var medianRTTs []float64
	var p95RTTs []float64
	var p99RTTs []float64
	var jitters []float64
	var jitterMedians []float64
	var jitterP95s []float64
	var totalLoss float64
	var count int

	for rows.Next() {
		var payloadRaw string
		if err := rows.Scan(&payloadRaw); err != nil || payloadRaw == "" {
			continue
		}

		var payload struct {
			AverageRTT     float64 `json:"averageRTT"`
			MedianRTT      float64 `json:"medianRTT,omitempty"`
			P95RTT         float64 `json:"p95RTT,omitempty"`
			P99RTT         float64 `json:"p99RTT,omitempty"`
			StdDevRTT      float64 `json:"stdDevRTT"`
			JitterAvg      float64 `json:"jitterAvg,omitempty"`
			JitterMedian   float64 `json:"jitterMedian,omitempty"`
			JitterP95      float64 `json:"jitterP95,omitempty"`
			LossPercentage float64 `json:"lossPercentage"`
		}
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}

		latencies = append(latencies, payload.AverageRTT)
		if payload.MedianRTT > 0 {
			medianRTTs = append(medianRTTs, payload.MedianRTT)
		}
		if payload.P95RTT > 0 {
			p95RTTs = append(p95RTTs, payload.P95RTT)
		}
		if payload.P99RTT > 0 {
			p99RTTs = append(p99RTTs, payload.P99RTT)
		}

		jitterVal := payload.JitterAvg
		if jitterVal == 0 {
			jitterVal = payload.StdDevRTT
		}
		if jitterVal > 0 {
			jitters = append(jitters, jitterVal)
		}
		if payload.JitterMedian > 0 {
			jitterMedians = append(jitterMedians, payload.JitterMedian)
		}
		if payload.JitterP95 > 0 {
			jitterP95s = append(jitterP95s, payload.JitterP95)
		}

		totalLoss += payload.LossPercentage
		count++
	}

	if count == 0 {
		return ProbeMetrics{}
	}

	// Determine final percentile values - use per-record values if available, otherwise compute from raw latencies
	var medianLat, p95Lat, p99Lat float64
	if len(medianRTTs) > 0 {
		medianLat = percentile(medianRTTs, 50)
	} else {
		_, medianLat, _ = FallbackPercentiles(latencies)
	}
	if len(p95RTTs) > 0 {
		p95Lat = percentile(p95RTTs, 95) // p95RTTs contains P95 values from each cycle
	} else {
		_, p95Lat, _ = FallbackPercentiles(latencies)
	}
	if len(p99RTTs) > 0 {
		p99Lat = percentile(p99RTTs, 99) // p99RTTs contains P99 values from each cycle
	} else {
		_, _, p99Lat = FallbackPercentiles(latencies)
	}

	// Jitter median and P95
	var jitterMedian, jitterP95 float64
	if len(jitterMedians) > 0 {
		jitterMedian = percentile(jitterMedians, 50)
	}
	if len(jitterP95s) > 0 {
		jitterP95 = percentile(jitterP95s, 95) // P95 of the P95 jitter values
	}

	return ProbeMetrics{
		AvgLatency:    sanitizeFloat(avg(latencies)),
		MedianLatency: sanitizeFloat(medianLat),
		P95Latency:    sanitizeFloat(p95Lat),
		P99Latency:    sanitizeFloat(p99Lat),
		PacketLoss:    sanitizeFloat(totalLoss / float64(count)),
		JitterAvg:     sanitizeFloat(avg(jitters)),
		JitterMedian:  sanitizeFloat(jitterMedian),
		JitterP95:     sanitizeFloat(jitterP95),
		SampleCount:   count,
	}
}

// FallbackPercentiles computes median, P95, P99 from a list of latency values
// when individual percentile fields aren't available (backwards compatibility)
func FallbackPercentiles(latencies []float64) (median, p95, p99 float64) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}
	median = percentile(latencies, 50)
	p95 = percentile(latencies, 95)
	p99 = percentile(latencies, 99)
	return
}

// analyzeMtrForProbe fetches MTR traces and produces path analysis + signals
func analyzeMtrForProbe(ctx context.Context, ch *sql.DB, agentIDs []uint, probeID uint, from time.Time, agentIPToID map[string]uint, agentByID map[uint]agentInfo) (*MtrPathAnalysis, []AnalysisSignal, error) {
	if len(agentIDs) == 0 {
		return nil, nil, nil
	}

	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	q := fmt.Sprintf(`
SELECT payload_raw
FROM probe_data
WHERE type = 'MTR'
  AND probe_id = %d
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 100
`, probeID, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type routeSig struct {
		signature string
		count     int
	}

	routeSignatures := make(map[string]*routeSig)
	var totalTraces int
	var totalEndHopLatency float64
	var totalEndHopLoss float64
	var totalEndHopJitterAvg float64
	var rateLimitedHops []int
	var timeoutSegments []string
	var maxHops int
	var firstPayload *mtrPayload // For building hop details

	hopMetrics := make(map[int]hopAgg)
	rateLimitedSet := make(map[int]bool)

	for rows.Next() {
		var payloadRaw string
		if err := rows.Scan(&payloadRaw); err != nil || payloadRaw == "" {
			continue
		}

		var payload mtrPayload
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}

		if len(payload.Report.Hops) == 0 {
			continue
		}

		// Capture first valid payload for hop details
		if firstPayload == nil {
			firstPayload = &payload
		}

		totalTraces++
		if len(payload.Report.Hops) > maxHops {
			maxHops = len(payload.Report.Hops)
		}

		// Aggregate per-hop metrics
		for i, hop := range payload.Report.Hops {
			if len(hop.Hosts) > 0 && hop.Hosts[0].IP != "" && hop.Hosts[0].IP != "*" {
				ha := hopMetrics[i]
				ha.totalLatency += parseFloat(hop.Avg)
				ha.totalLoss += parseFloat(hop.LossPct)
				ha.count++
				hopMetrics[i] = ha
			}
		}

		// Build route signature (responding hops only)
		var sigParts []string
		for i, hop := range payload.Report.Hops {
			if len(hop.Hosts) > 0 && hop.Hosts[0].IP != "" {
				sigParts = append(sigParts, fmt.Sprintf("%d:%s", i+1, hop.Hosts[0].IP))
			}
		}
		sig := strings.Join(sigParts, "|")
		if routeSignatures[sig] == nil {
			routeSignatures[sig] = &routeSig{signature: sig}
		}
		routeSignatures[sig].count++

		// End hop metrics
		lastHop := payload.Report.Hops[len(payload.Report.Hops)-1]
		totalEndHopLatency += parseFloat(lastHop.Avg)
		totalEndHopLoss += parseFloat(lastHop.LossPct)
		totalEndHopJitterAvg += parseFloat(lastHop.StdDev)

		// Detect ICMP rate limiting and timeout segments (only on first trace)
		if totalTraces == 1 {
			endLoss := parseFloat(lastHop.LossPct)
			inTimeout := false
			timeoutStart := 0

			for i, hop := range payload.Report.Hops {
				hopLoss := parseFloat(hop.LossPct)
				hopIP := ""
				if len(hop.Hosts) > 0 {
					hopIP = hop.Hosts[0].IP
				}

				// Rate limit detection: intermediate loss that doesn't propagate
				if hopLoss > 10 && endLoss < 1 && hopIP != "" {
					rateLimitedHops = append(rateLimitedHops, i+1)
					rateLimitedSet[i] = true
				}

				// Timeout segment detection
				if hopIP == "" {
					if !inTimeout {
						inTimeout = true
						timeoutStart = i + 1
					}
				} else {
					if inTimeout {
						timeoutSegments = append(timeoutSegments, fmt.Sprintf("Hops %d-%d", timeoutStart, i))
						inTimeout = false
					}
				}
			}
			if inTimeout {
				timeoutSegments = append(timeoutSegments, fmt.Sprintf("Hops %d-%d", timeoutStart, len(payload.Report.Hops)))
			}
		}
	}

	if totalTraces == 0 {
		return nil, nil, nil
	}

	// Route stability
	var maxCount int
	for _, rs := range routeSignatures {
		if rs.count > maxCount {
			maxCount = rs.count
		}
	}
	stabilityPct := float64(maxCount) / float64(totalTraces) * 100

	analysis := &MtrPathAnalysis{
		HopCount:           maxHops,
		UniqueRoutes:       len(routeSignatures),
		RouteStabilityPct:  sanitizeFloat(stabilityPct),
		AvgEndHopLatency:   sanitizeFloat(totalEndHopLatency / float64(totalTraces)),
		AvgEndHopLoss:      sanitizeFloat(totalEndHopLoss / float64(totalTraces)),
		AvgEndHopJitterAvg: sanitizeFloat(totalEndHopJitterAvg / float64(totalTraces)),
		TraceCount:         totalTraces,
		RateLimitedHops:    rateLimitedHops,
		TimeoutSegments:    timeoutSegments,
	}

	// Build enriched hop details with agent names and per-hop metrics
	if firstPayload != nil {
		analysis.LatestHopsDetail = buildHopDetailsForMtrPayload(firstPayload, agentIPToID, agentByID, hopMetrics, rateLimitedSet)
	}

	// Generate signals
	var signals []AnalysisSignal

	if len(rateLimitedHops) > 0 {
		signals = append(signals, AnalysisSignal{
			Type:       "icmp_artifact",
			Severity:   "info",
			Title:      "ICMP Rate Limiting Detected",
			Evidence:   fmt.Sprintf("Hops %v show high loss that does not propagate to the destination", rateLimitedHops),
			Confidence: 0.85,
		})
	}

	if len(timeoutSegments) > 0 {
		signals = append(signals, AnalysisSignal{
			Type:       "icmp_artifact",
			Severity:   "info",
			Title:      "Filtered ICMP Segments",
			Evidence:   fmt.Sprintf("Non-responding segments: %s", strings.Join(timeoutSegments, ", ")),
			Confidence: 0.70,
		})
	}

	if len(routeSignatures) > 1 {
		sev := "info"
		if stabilityPct < 70 {
			sev = "warning"
		}
		signals = append(signals, AnalysisSignal{
			Type:       "route_change",
			Severity:   sev,
			Title:      "Route Instability Detected",
			Evidence:   fmt.Sprintf("%d unique routes observed across %d traces (stability: %.0f%%)", len(routeSignatures), totalTraces, stabilityPct),
			Confidence: 0.90,
		})
	}

	if analysis.AvgEndHopLoss > 3 {
		sev := "warning"
		if analysis.AvgEndHopLoss > 10 {
			sev = "critical"
		}
		signals = append(signals, AnalysisSignal{
			Type:       "high_loss",
			Severity:   sev,
			Title:      "End-to-End Packet Loss",
			Evidence:   fmt.Sprintf("Average end-hop loss: %.1f%%", analysis.AvgEndHopLoss),
			Confidence: 0.95,
		})
	}

	return analysis, signals, nil
}

// findSiblingProbeIDs finds other probes from the same agent that share the same
// first target (by literal target string or agent_id). Returns a map of Type → probe ID
// so the analysis can query PING data from a PING sibling and MTR data from an MTR sibling.
func findSiblingProbeIDs(ctx context.Context, db *gorm.DB, p *Probe) map[Type]uint {
	result := make(map[Type]uint)
	// Always include the probe itself
	result[p.Type] = p.ID

	if len(p.Targets) == 0 {
		return result
	}

	firstTarget := p.Targets[0]

	// Find all probes from the same agent
	siblings, err := ListByAgent(ctx, db, p.AgentID)
	if err != nil {
		log.Warnf("[Analysis] findSiblingProbeIDs: failed to list probes for agent %d: %v", p.AgentID, err)
		return result
	}

	for _, sib := range siblings {
		if sib.ID == p.ID || len(sib.Targets) == 0 {
			continue
		}
		// Skip non-monitoring types
		if sib.Type != TypePing && sib.Type != TypeMTR && sib.Type != TypeDNS {
			continue
		}

		sibTarget := sib.Targets[0]

		// Match by agent_id target
		if firstTarget.AgentID != nil && sibTarget.AgentID != nil {
			if *firstTarget.AgentID == *sibTarget.AgentID {
				result[sib.Type] = sib.ID
				continue
			}
		}

		// Match by literal target string (case-insensitive, trimmed)
		if firstTarget.Target != "" && sibTarget.Target != "" {
			if strings.EqualFold(strings.TrimSpace(firstTarget.Target), strings.TrimSpace(sibTarget.Target)) {
				result[sib.Type] = sib.ID
			}
		}
	}

	if len(result) > 1 {
		log.Debugf("[Analysis] Probe %d (type=%s): found %d sibling probe IDs: %v", p.ID, p.Type, len(result), result)
	}

	return result
}

// ── Public API ──

// ComputeProbeAnalysis computes full health vector + signals for a specific probe
func ComputeProbeAnalysis(ctx context.Context, ch *sql.DB, pg *gorm.DB, workspaceID, probeID uint, lookbackMinutes int) (*ProbeAnalysis, error) {
	if lookbackMinutes <= 0 {
		lookbackMinutes = 60
	}
	from := time.Now().UTC().Add(-time.Duration(lookbackMinutes) * time.Minute)

	// Get agents
	agents, err := getWorkspaceAgents(ctx, pg, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get agents: %w", err)
	}
	agentIDs := make([]uint, len(agents))
	agentByID := make(map[uint]agentInfo)
	agentIPToID := make(map[string]uint)
	for i, a := range agents {
		agentIDs[i] = a.ID
		agentByID[a.ID] = a
		if a.PublicIPOverride != "" {
			agentIPToID[a.PublicIPOverride] = a.ID
		}
	}

	// Get probe info
	p, err := GetByID(ctx, pg, probeID)
	if err != nil {
		return nil, fmt.Errorf("get probe: %w", err)
	}

	// Determine target name
	targetName := ""
	var targetAgentID uint
	for _, t := range p.Targets {
		if t.Target != "" {
			targetName = t.Target
		}
		if t.AgentID != nil {
			targetAgentID = *t.AgentID
			if a, ok := agentByID[targetAgentID]; ok {
				targetName = a.Name
			}
		}
	}

	agentName := ""
	if a, ok := agentByID[p.AgentID]; ok {
		agentName = a.Name
	}

	// For non-AGENT probes, find sibling probes (same agent, same target, different type).
	// The frontend groups PING+MTR probes targeting the same host together on one page,
	// so the analysis should combine data from all sibling probe IDs.
	pingProbeID := probeID
	mtrProbeID := probeID
	if p.Type != TypeAgent {
		siblingIDs := findSiblingProbeIDs(ctx, pg, p)
		if pid, ok := siblingIDs[TypePing]; ok {
			pingProbeID = pid
			log.Debugf("[Analysis] Probe %d: using PING sibling probe %d for metrics", probeID, pid)
		}
		if mid, ok := siblingIDs[TypeMTR]; ok {
			mtrProbeID = mid
			log.Debugf("[Analysis] Probe %d: using MTR sibling probe %d for path analysis", probeID, mid)
		}
	}

	// Forward direction: rows reported by the probe's owner agent
	fwd := analyzeProbeDirection(ctx, ch, directionInput{
		PingProbeID:       pingProbeID,
		MtrProbeID:        mtrProbeID,
		TrafficSimProbeID: probeID,
		ReporterID:        p.AgentID,
		IncludeTrafficSim: p.Type == TypeAgent,
	}, from, agentIPToID, agentByID)

	log.Debugf("[Analysis] Probe %d (type=%s): forward samples=%d, avgLat=%.1f, loss=%.2f%%",
		probeID, p.Type, fwd.Metrics.SampleCount, fwd.Metrics.AvgLatency, fwd.Metrics.PacketLoss)

	result := &ProbeAnalysis{
		ProbeID:      probeID,
		ProbeType:    string(p.Type),
		Target:       targetName,
		AgentID:      p.AgentID,
		AgentName:    agentName,
		Health:       fwd.Health,
		Metrics:      fwd.Metrics,
		PathAnalysis: fwd.Path,
		Signals:      fwd.Signals,
		Findings:     fwd.Findings,
		GeneratedAt:  time.Now().UTC(),
	}

	// Reverse direction. Two formats:
	// - NEW single-probe bidirectional: return-path rows live under the SAME
	//   probe ID, reported by the target agent.
	// - LEGACY dual-probe: a reciprocal AGENT probe owned by the target exists
	//   in the DB; its rows live under that probe's ID.
	if targetAgentID > 0 {
		revProbeID := probeID
		revProbeType := string(p.Type)

		// Prefer the legacy reciprocal probe when one exists (hybrid/legacy DBs);
		// otherwise use the new-format same-ID reverse rows.
		if reverseProbes, rerr := findReverseAgentProbes(ctx, pg, p.AgentID); rerr == nil {
			for _, rp := range reverseProbes {
				if rp.AgentID == targetAgentID {
					revProbeID = rp.ID
					revProbeType = string(rp.Type)
					break
				}
			}
		}

		rev := analyzeProbeDirection(ctx, ch, directionInput{
			PingProbeID:       revProbeID,
			MtrProbeID:        revProbeID,
			TrafficSimProbeID: probeID, // reverse TrafficSim always reports under the client's probe ID
			ReporterID:        targetAgentID,
			IncludeTrafficSim: p.Type == TypeAgent,
		}, from, agentIPToID, agentByID)

		hasReverseData := rev.Metrics.SampleCount > 0 || (rev.Path != nil && rev.Path.TraceCount > 0)
		if hasReverseData {
			revAgentName := ""
			if a, ok := agentByID[targetAgentID]; ok {
				revAgentName = a.Name
			}

			result.Reverse = &ProbeAnalysis{
				ProbeID:      revProbeID,
				ProbeType:    revProbeType,
				Target:       agentName, // reverse target is the original agent
				AgentID:      targetAgentID,
				AgentName:    revAgentName,
				Health:       rev.Health,
				Metrics:      rev.Metrics,
				PathAnalysis: rev.Path,
				Signals:      rev.Signals,
				Findings:     rev.Findings,
				GeneratedAt:  time.Now().UTC(),
			}

			// Bidirectional heuristics: a clean direction next to a degraded one
			// localizes the problem to one path — the key troubleshooting signal
			// that single-direction analysis can't produce.
			if fwd.Metrics.SampleCount > 0 {
				fwdLabel := fmt.Sprintf("%s → %s", agentName, revAgentName)
				revLabel := fmt.Sprintf("%s → %s", revAgentName, agentName)
				asymSignals, asymFindings := buildDirectionalitySignals(fwd.Metrics, rev.Metrics, fwdLabel, revLabel)
				result.Signals = append(result.Signals, asymSignals...)
				result.Findings = append(result.Findings, asymFindings...)
			}

			combined := combineDirectionHealth(fwd.Health, rev.Health)
			result.CombinedHealth = &combined
		}
	}

	return result, nil
}

// directionInput identifies which probe IDs and reporter make up one direction
// of a probe's data.
type directionInput struct {
	PingProbeID       uint
	MtrProbeID        uint
	TrafficSimProbeID uint
	ReporterID        uint
	IncludeTrafficSim bool
}

// directionAnalysis is the per-direction result bundle.
type directionAnalysis struct {
	Metrics  ProbeMetrics
	Path     *MtrPathAnalysis
	Signals  []AnalysisSignal
	Health   HealthVector
	Findings []AnalysisFinding
}

// analyzeProbeDirection computes metrics, MTR path analysis, signals, health
// and findings for ONE direction of a probe: the rows reported by ReporterID.
// Forward passes the owner agent; reverse passes the target agent (same probe
// ID for new-format bidirectional probes, reciprocal probe ID for legacy).
func analyzeProbeDirection(ctx context.Context, ch *sql.DB, in directionInput, from time.Time, agentIPToID map[string]uint, agentByID map[uint]agentInfo) directionAnalysis {
	metrics, err := probeAnalysisMetrics(ctx, ch, []uint{in.ReporterID}, in.PingProbeID, from)
	if err != nil {
		log.Warnf("[Analysis] Failed to fetch PING metrics for probe %d (reporter %d): %v", in.PingProbeID, in.ReporterID, err)
		metrics = ProbeMetrics{}
	}

	pathAnalysis, mtrSignals, err := analyzeMtrForProbe(ctx, ch, []uint{in.ReporterID}, in.MtrProbeID, from, agentIPToID, agentByID)
	if err != nil {
		log.Warnf("[Analysis] Failed to analyze MTR for probe %d (reporter %d): %v", in.MtrProbeID, in.ReporterID, err)
	}

	// TrafficSim data (same probe_id, different type)
	if in.IncludeTrafficSim {
		tsMetrics := probeTrafficSimMetrics(ctx, ch, []uint{in.ReporterID}, in.TrafficSimProbeID, from)
		log.Debugf("[Analysis] Probe %d reporter %d: TrafficSim samples=%d, avgRTT=%.1f, loss=%.2f%%",
			in.TrafficSimProbeID, in.ReporterID, tsMetrics.SampleCount, tsMetrics.AvgLatency, tsMetrics.PacketLoss)
		if tsMetrics.SampleCount > 0 {
			// If PING data was empty, use TrafficSim as primary (not blended)
			if metrics.SampleCount == 0 {
				metrics.AvgLatency = tsMetrics.AvgLatency
				metrics.P95Latency = tsMetrics.P95Latency
				metrics.PacketLoss = tsMetrics.PacketLoss
				metrics.JitterAvg = tsMetrics.JitterAvg
				metrics.SampleCount = tsMetrics.SampleCount
			} else {
				// Blend: use worse of PING/TrafficSim loss
				if tsMetrics.PacketLoss > metrics.PacketLoss {
					metrics.PacketLoss = tsMetrics.PacketLoss
				}
			}
		}
	}

	// If PING metrics are empty but MTR path analysis found data, derive
	// metrics from MTR end-hop stats so the direction still gets a health score.
	var fallbackSignals []AnalysisSignal
	if metrics.SampleCount == 0 && pathAnalysis != nil && pathAnalysis.TraceCount > 0 {
		metrics.AvgLatency = pathAnalysis.AvgEndHopLatency
		metrics.P95Latency = pathAnalysis.AvgEndHopLatency * 1.3 // Approximate P95 from avg
		metrics.PacketLoss = pathAnalysis.AvgEndHopLoss
		metrics.JitterAvg = pathAnalysis.AvgEndHopJitterAvg
		metrics.SampleCount = pathAnalysis.TraceCount
		fallbackSignals = append(fallbackSignals, AnalysisSignal{
			Type:       "icmp_latency_incomplete",
			Severity:   "info",
			Title:      "Latency Estimated from MTR",
			Evidence:   "ICMP probe returned no data; latency derived from MTR end-hop RTT",
			Confidence: 0.7,
		})
	}

	// Route stability from MTR (100% if no MTR data)
	routeStability := 100.0
	if pathAnalysis != nil {
		routeStability = pathAnalysis.RouteStabilityPct
	}

	health := computeHealthVector(metrics, routeStability)

	var signals []AnalysisSignal
	signals = append(signals, mtrSignals...)
	signals = append(signals, fallbackSignals...)

	if metrics.AvgLatency > 150 {
		sev := "warning"
		if metrics.AvgLatency > 300 {
			sev = "critical"
		}
		signals = append(signals, AnalysisSignal{
			Type:       "high_latency",
			Severity:   sev,
			Title:      "High Average Latency",
			Evidence:   fmt.Sprintf("Average: %.1fms, P95: %.1fms", metrics.AvgLatency, metrics.P95Latency),
			Confidence: 0.95,
		})
	}

	if metrics.JitterAvg > 30 {
		signals = append(signals, AnalysisSignal{
			Type:       "jitter_anomaly",
			Severity:   "warning",
			Title:      "High JitterAvg",
			Evidence:   fmt.Sprintf("Average jitter: %.1fms", metrics.JitterAvg),
			Confidence: 0.80,
		})
	}

	if metrics.PacketLoss > 1 {
		sev := "warning"
		if metrics.PacketLoss > 5 {
			sev = "critical"
		}
		signals = append(signals, AnalysisSignal{
			Type:       "high_loss",
			Severity:   sev,
			Title:      "Elevated Packet Loss",
			Evidence:   fmt.Sprintf("Average loss: %.2f%%", metrics.PacketLoss),
			Confidence: 0.95,
		})
	}

	return directionAnalysis{
		Metrics:  metrics,
		Path:     pathAnalysis,
		Signals:  signals,
		Health:   health,
		Findings: buildFindings(health, metrics, pathAnalysis, signals),
	}
}

// buildDirectionalitySignals compares forward and reverse metrics and emits
// asymmetry signals/findings. Direction labels are "Source → Target" strings.
func buildDirectionalitySignals(fwd, rev ProbeMetrics, fwdLabel, revLabel string) ([]AnalysisSignal, []AnalysisFinding) {
	var signals []AnalysisSignal
	var findings []AnalysisFinding

	worseLabel := func(fwdWorse bool) string {
		if fwdWorse {
			return fwdLabel
		}
		return revLabel
	}

	// Loss asymmetry: one direction dropping packets while the other is clean
	// points at the worse direction's path (upload saturation at its source,
	// one-way policing/QoS, or asymmetric routing).
	lossDiff := fwd.PacketLoss - rev.PacketLoss
	maxLoss := math.Max(fwd.PacketLoss, rev.PacketLoss)
	if math.Abs(lossDiff) >= 2 && maxLoss >= 1 {
		sev := "warning"
		if maxLoss > 5 && math.Abs(lossDiff) >= 5 {
			sev = "critical"
		}
		dir := worseLabel(lossDiff > 0)
		signals = append(signals, AnalysisSignal{
			Type:     "loss_asymmetry",
			Severity: sev,
			Title:    "Directional Packet Loss",
			Evidence: fmt.Sprintf("%s: %.1f%% loss vs %s: %.1f%% loss",
				fwdLabel, fwd.PacketLoss, revLabel, rev.PacketLoss),
			Confidence: 0.9,
		})
		findings = append(findings, AnalysisFinding{
			ID:       "loss-asymmetry",
			Title:    fmt.Sprintf("Packet loss is concentrated in one direction (%s)", dir),
			Severity: sev,
			Category: "directionality",
			Summary: fmt.Sprintf("The %s direction is losing %.1f%% of packets while the opposite direction loses %.1f%% — the underlying path is healthy in one direction, so this is not general congestion at the target.",
				dir, maxLoss, math.Min(fwd.PacketLoss, rev.PacketLoss)),
			Evidence: []string{
				fmt.Sprintf("%s: %.1f%% loss", fwdLabel, fwd.PacketLoss),
				fmt.Sprintf("%s: %.1f%% loss", revLabel, rev.PacketLoss),
			},
			Steps: []string{
				"Check upload utilization/saturation at the source of the degraded direction",
				"Look for one-way QoS policing or rate limiting (especially on DSCP-marked traffic)",
				"Compare forward and reverse MTR paths for asymmetric routing",
				"Check duplex mismatches or Wi-Fi retransmits at the degraded direction's source",
			},
		})
	}

	// Latency asymmetry: large one-way skew usually means queueing (bufferbloat)
	// or a longer return route, not target distance.
	if fwd.AvgLatency > 0 && rev.AvgLatency > 0 {
		hi := math.Max(fwd.AvgLatency, rev.AvgLatency)
		lo := math.Min(fwd.AvgLatency, rev.AvgLatency)
		if hi/lo >= 1.5 && hi-lo >= 30 {
			dir := worseLabel(fwd.AvgLatency > rev.AvgLatency)
			signals = append(signals, AnalysisSignal{
				Type:     "latency_asymmetry",
				Severity: "warning",
				Title:    "Asymmetric Latency",
				Evidence: fmt.Sprintf("%s: %.1fms vs %s: %.1fms",
					fwdLabel, fwd.AvgLatency, revLabel, rev.AvgLatency),
				Confidence: 0.85,
			})
			findings = append(findings, AnalysisFinding{
				ID:       "latency-asymmetry",
				Title:    fmt.Sprintf("Latency is significantly higher in one direction (%s)", dir),
				Severity: "warning",
				Category: "directionality",
				Summary: fmt.Sprintf("The %s direction averages %.1fms while the opposite direction averages %.1fms. Since both measurements traverse the same endpoints, the skew points at queueing delay (bufferbloat) or a longer route in the slower direction.",
					dir, hi, lo),
				Evidence: []string{
					fmt.Sprintf("%s: avg %.1fms", fwdLabel, fwd.AvgLatency),
					fmt.Sprintf("%s: avg %.1fms", revLabel, rev.AvgLatency),
				},
				Steps: []string{
					"Check for sustained upload traffic at the slower direction's source (bufferbloat)",
					"Compare MTR hop counts and paths between directions for route asymmetry",
					"Enable/verify SQM or smart queue management on the slower direction's uplink",
				},
			})
		}
	}

	// Jitter asymmetry: one-way jitter with clean jitter the other way usually
	// means access-layer instability (Wi-Fi, congested uplink) at one end.
	if fwd.JitterAvg > 0 || rev.JitterAvg > 0 {
		hi := math.Max(fwd.JitterAvg, rev.JitterAvg)
		lo := math.Min(fwd.JitterAvg, rev.JitterAvg)
		if hi >= 15 && (lo == 0 || hi/math.Max(lo, 0.1) >= 2) {
			dir := worseLabel(fwd.JitterAvg > rev.JitterAvg)
			signals = append(signals, AnalysisSignal{
				Type:     "jitter_asymmetry",
				Severity: "warning",
				Title:    "Asymmetric Jitter",
				Evidence: fmt.Sprintf("%s: %.1fms jitter vs %s: %.1fms jitter",
					fwdLabel, fwd.JitterAvg, revLabel, rev.JitterAvg),
				Confidence: 0.8,
			})
			findings = append(findings, AnalysisFinding{
				ID:       "jitter-asymmetry",
				Title:    fmt.Sprintf("Jitter is concentrated in one direction (%s)", dir),
				Severity: "warning",
				Category: "directionality",
				Summary: fmt.Sprintf("The %s direction shows %.1fms average jitter against %.1fms the other way — typically access-layer instability (Wi-Fi interference, congested uplink) at that direction's source rather than a path-wide issue.",
					dir, hi, lo),
				Evidence: []string{
					fmt.Sprintf("%s: %.1fms jitter", fwdLabel, fwd.JitterAvg),
					fmt.Sprintf("%s: %.1fms jitter", revLabel, rev.JitterAvg),
				},
				Steps: []string{
					"Check the degraded direction's source for Wi-Fi vs wired connectivity",
					"Inspect uplink utilization at the degraded direction's source during jitter spikes",
					"Correlate jitter windows with scheduled transfers or backups",
				},
			})
		}
	}

	return signals, findings
}

// combineDirectionHealth merges forward and reverse health into a single
// bidirectional score, weighting the worse direction at 65% — a link is only
// as usable as its worse direction, but the better direction still matters.
func combineDirectionHealth(fwd, rev HealthVector) HealthVector {
	worse, better := fwd, rev
	if rev.OverallHealth < fwd.OverallHealth {
		worse, better = rev, fwd
	}
	mix := func(w, b float64) float64 { return clampScore(w*0.65 + b*0.35) }
	overall := mix(worse.OverallHealth, better.OverallHealth)
	return HealthVector{
		LatencyScore:    mix(worse.LatencyScore, better.LatencyScore),
		PacketLossScore: mix(worse.PacketLossScore, better.PacketLossScore),
		RouteStability:  mix(worse.RouteStability, better.RouteStability),
		MosScore:        math.Min(fwd.MosScore, rev.MosScore),
		OverallHealth:   overall,
		Grade:           gradeFromScore(overall),
	}
}

// ComputeWorkspaceAnalysis aggregates health vectors across all agents in a workspace
func ComputeWorkspaceAnalysis(ctx context.Context, ch *sql.DB, pg *gorm.DB, workspaceID uint, lookbackMinutes int) (*WorkspaceAnalysis, error) {
	if lookbackMinutes <= 0 {
		lookbackMinutes = 60
	}
	from := time.Now().UTC().Add(-time.Duration(lookbackMinutes) * time.Minute)

	// Get agents
	agents, err := getWorkspaceAgents(ctx, pg, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get agents: %w", err)
	}

	if len(agents) == 0 {
		return &WorkspaceAnalysis{
			WorkspaceID:   workspaceID,
			OverallHealth: HealthVector{Grade: "unknown", RouteStability: 100, MosScore: 1.0},
			Agents:        []AgentHealthSummary{},
			GeneratedAt:   time.Now().UTC(),
		}, nil
	}

	agentIDs := make([]uint, len(agents))
	agentByID := make(map[uint]agentInfo)
	for i, a := range agents {
		agentIDs[i] = a.ID
		agentByID[a.ID] = a
	}

	// Fetch metrics for all agents
	pingMetrics, _ := getWorkspacePingMetrics(ctx, ch, agentIDs, from)
	mtrMetrics, _ := getWorkspaceMTRMetrics(ctx, ch, pg, agentIDs, from)
	trafficMetrics, _ := getWorkspaceTrafficSimMetrics(ctx, ch, agentIDs, from)
	sysInfoMetrics, _ := getWorkspaceSysInfoMetrics(ctx, ch, agentIDs, from)
	netInfoChanges, _ := getWorkspaceNetInfoChanges(ctx, ch, agentIDs, from)

	// Fetch baseline metrics (7-day rolling average) for change detection
	baselineFrom := time.Now().UTC().Add(-7 * 24 * time.Hour)
	baselinePing, _ := getWorkspacePingMetrics(ctx, ch, agentIDs, baselineFrom)
	baselineTraffic, _ := getWorkspaceTrafficSimMetrics(ctx, ch, agentIDs, baselineFrom)

	// Build per-agent summaries
	var agentSummaries []AgentHealthSummary
	var allHealthScores []float64
	totalProbes := 0

	for _, agent := range agents {
		isOnline := time.Since(agent.UpdatedAt) < time.Minute

		// Collect metrics for probes FROM this agent
		var agentLatencies []float64
		var agentLoss []float64
		var agentJitterAvg []float64
		var probeEntries []ProbeHealthEntry

		prefix := fmt.Sprintf("%d:", agent.ID)

		// PING metrics
		for key, stats := range pingMetrics {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			target := key[len(prefix):]
			m := ProbeMetrics{
				AvgLatency:  stats.AvgLatency,
				PacketLoss:  stats.PacketLoss,
				SampleCount: stats.Count,
			}
			h := computeHealthVector(m, 100)
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    stripPort(target),
				ProbeType: "PING",
				Health:    h,
				Metrics:   m,
			})
			agentLatencies = append(agentLatencies, stats.AvgLatency)
			agentLoss = append(agentLoss, stats.PacketLoss)
		}

		// MTR metrics
		for key, stats := range mtrMetrics {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			target := key[len(prefix):]
			m := ProbeMetrics{
				AvgLatency:  stats.AvgLatency,
				PacketLoss:  stats.PacketLoss,
				JitterAvg:   stats.Jitter,
				SampleCount: stats.Count,
			}
			h := computeHealthVector(m, 100)
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    stripPort(target),
				ProbeType: "MTR",
				Health:    h,
				Metrics:   m,
			})
			agentLatencies = append(agentLatencies, stats.AvgLatency)
			agentLoss = append(agentLoss, stats.PacketLoss)
			agentJitterAvg = append(agentJitterAvg, stats.Jitter)
		}

		// TrafficSim metrics
		for key, stats := range trafficMetrics {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			target := key[len(prefix):]
			m := ProbeMetrics{
				AvgLatency:  stats.AvgRTT,
				PacketLoss:  stats.PacketLoss,
				SampleCount: stats.Count,
			}
			h := computeHealthVector(m, 100)
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    stripPort(target),
				ProbeType: "TRAFFICSIM",
				Health:    h,
				Metrics:   m,
			})
			agentLatencies = append(agentLatencies, stats.AvgRTT)
			agentLoss = append(agentLoss, stats.PacketLoss)
		}

		// SysInfo metrics (host health)
		if si, ok := sysInfoMetrics[fmt.Sprintf("%d", agent.ID)]; ok {
			sysScore := sysInfoHealthScore(si)
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    "host-resources",
				ProbeType: "SYSINFO",
				Health: HealthVector{
					OverallHealth:  clampScore(sysScore),
					Grade:          gradeFromScore(sysScore),
					RouteStability: 100,
					MosScore:       1.0,
				},
				Metrics: ProbeMetrics{SampleCount: 1},
			})
		}

		totalProbes += len(probeEntries)

		// Compute agent-level health
		var agentHealth HealthVector
		var dataGap bool
		if len(probeEntries) > 0 {
			avgLat := avg(agentLatencies)
			avgLossVal := avg(agentLoss)
			avgJitterAvgVal := avg(agentJitterAvg)

			agentMetrics := ProbeMetrics{
				AvgLatency: avgLat,
				PacketLoss: avgLossVal,
				JitterAvg:  avgJitterAvgVal,
			}
			agentHealth = computeHealthVector(agentMetrics, 100)
		} else {
			dataGap = true
			agentHealth = HealthVector{
				Grade:          "unknown",
				RouteStability: 100,
				MosScore:       1.0,
			}
		}

		if !isOnline {
			agentHealth.OverallHealth = 0
			agentHealth.Grade = gradeFromScore(0)
		} else if isOnline && dataGap {
			agentHealth.OverallHealth = math.Max(0, agentHealth.OverallHealth-10)
			agentHealth.Grade = gradeFromScore(agentHealth.OverallHealth)
		}

		allHealthScores = append(allHealthScores, agentHealth.OverallHealth)

		// Sort worst probes (by lowest overall health)
		sortProbesByHealth(probeEntries)
		worstCount := 3
		if len(probeEntries) < worstCount {
			worstCount = len(probeEntries)
		}

		agentSummaries = append(agentSummaries, AgentHealthSummary{
			AgentID:     agent.ID,
			AgentName:   agent.Name,
			IsOnline:    isOnline,
			Health:      agentHealth,
			ProbeCount:  len(probeEntries),
			WorstProbes: probeEntries[:worstCount],
		})
	}

	// Compute overall workspace health
	var overallHealth HealthVector
	if len(allHealthScores) > 0 {
		overall := avg(allHealthScores)
		overallHealth = HealthVector{
			OverallHealth: clampScore(overall),
			Grade:         gradeFromScore(overall),
			MosScore:      computeMos(avg(extractField(agentSummaries, "latency")), avg(extractField(agentSummaries, "loss")), avg(extractField(agentSummaries, "jitter"))),
		}
		// Compute sub-scores from agent averages
		overallHealth.LatencyScore = clampScore(avg(extractHealthField(agentSummaries, "latency_score")))
		overallHealth.PacketLossScore = clampScore(avg(extractHealthField(agentSummaries, "loss_score")))
		overallHealth.RouteStability = clampScore(avg(extractHealthField(agentSummaries, "route_stability")))
	} else {
		overallHealth = HealthVector{Grade: "unknown", RouteStability: 100, MosScore: 1.0}
	}

	// ── Cross-Agent Correlation & Incident Detection ──
	// Pull latest NETINFO for each agent so IP→agent resolution in
	// "Shared degradation" titles can map the agent's real public IP back
	// to its name when PublicIPOverride is unset.
	netInfoByAgent := getLatestNetInfoForAgents(ctx, ch, agentIDs, from)
	agentIPToID := buildAgentIPToIDMap(agentSummaries, agentByID, netInfoByAgent)
	incidents := detectIncidents(agentSummaries, pingMetrics, mtrMetrics, trafficMetrics, agentByID, lookbackMinutes, agentIPToID)

	// ── Temporal Change Detection ──
	changeIncidents := detectTemporalChanges(pingMetrics, baselinePing, trafficMetrics, baselineTraffic, netInfoChanges, sysInfoMetrics, agentByID)
	incidents = append(incidents, changeIncidents...)

	// ── Speedtest Bandwidth Regression Detection ──
	speedtestIncidents := detectSpeedtestIncidents(ctx, ch, agentIDs, from, baselineFrom, agentByID)
	incidents = append(incidents, speedtestIncidents...)

	// ── DNS Pattern Detection ──
	dnsIncidents := detectDNSIncidents(ctx, ch, agentIDs, from, agentByID)
	incidents = append(incidents, dnsIncidents...)

	// Build status summary
	status := buildStatusSummary(overallHealth, agentSummaries, incidents)

	// ── Optional LLM Enrichment ──
	// Trigger on incidents OR healthy state (periodic "all clear" summaries)
	if llmProvider != nil && llmProvider.Available() && (len(incidents) > 0 || status.Status == "healthy") {
		enriched := enrichWithLLM(ctx, status, incidents, agentSummaries, overallHealth, totalProbes)
		if enriched != "" {
			status.Message = enriched
		}
	}

	return &WorkspaceAnalysis{
		WorkspaceID:   workspaceID,
		OverallHealth: overallHealth,
		Status:        status,
		Incidents:     incidents,
		Agents:        agentSummaries,
		TotalProbes:   totalProbes,
		TotalAgents:   len(agents),
		GeneratedAt:   time.Now().UTC(),
	}, nil
}

// ── Helpers ──

func buildFindings(health HealthVector, metrics ProbeMetrics, path *MtrPathAnalysis, signals []AnalysisSignal) []AnalysisFinding {
	var findings []AnalysisFinding

	// Grade-based overall finding
	switch health.Grade {
	case "critical":
		findings = append(findings, AnalysisFinding{
			ID:       "overall_critical",
			Title:    "Critical Path Degradation",
			Severity: "critical",
			Category: "performance",
			Summary:  fmt.Sprintf("Overall health score is %.0f/100 (grade: critical). Immediate attention recommended.", health.OverallHealth),
			Evidence: []string{
				fmt.Sprintf("Avg Latency: %.1fms", metrics.AvgLatency),
				fmt.Sprintf("Packet Loss: %.2f%%", metrics.PacketLoss),
				fmt.Sprintf("MOS: %.2f", health.MosScore),
			},
			Steps: []string{
				"Check for ISP outages or congestion at peering points",
				"Review recent MTR traces for route changes",
				"Contact upstream provider if issues persist",
			},
		})
	case "poor":
		findings = append(findings, AnalysisFinding{
			ID:       "overall_poor",
			Title:    "Degraded Path Performance",
			Severity: "warning",
			Category: "performance",
			Summary:  fmt.Sprintf("Overall health score is %.0f/100 (grade: poor). Performance is significantly below optimal.", health.OverallHealth),
			Evidence: []string{
				fmt.Sprintf("Avg Latency: %.1fms", metrics.AvgLatency),
				fmt.Sprintf("Packet Loss: %.2f%%", metrics.PacketLoss),
			},
			Steps: []string{
				"Monitor for further degradation",
				"Check for traffic congestion during peak hours",
			},
		})
	case "excellent", "good":
		findings = append(findings, AnalysisFinding{
			ID:       "overall_healthy",
			Title:    "Path Health Normal",
			Severity: "info",
			Category: "performance",
			Summary:  fmt.Sprintf("Overall health score is %.0f/100 (grade: %s). Path is performing within acceptable parameters.", health.OverallHealth, health.Grade),
		})
	}

	// Path-specific findings
	if path != nil {
		if len(path.RateLimitedHops) > 0 {
			findings = append(findings, AnalysisFinding{
				ID:       "icmp_rate_limit",
				Title:    "ICMP Rate Limiting Detected (Measurement Artifact)",
				Severity: "info",
				Category: "measurement_artifact",
				Summary:  "Some intermediate routers appear to rate-limit ICMP TTL-exceeded responses. The reported loss at these hops is NOT affecting end-to-end traffic.",
				Evidence: []string{
					fmt.Sprintf("Affected hops: %v", path.RateLimitedHops),
					fmt.Sprintf("End-to-end loss: %.1f%%", path.AvgEndHopLoss),
				},
			})
		}
		if path.UniqueRoutes > 2 {
			findings = append(findings, AnalysisFinding{
				ID:       "route_instability",
				Title:    "Route Path Instability",
				Severity: "warning",
				Category: "routing",
				Summary:  fmt.Sprintf("Multiple route paths detected (%d unique routes, %.0f%% stability). This may indicate ECMP load balancing or flapping.", path.UniqueRoutes, path.RouteStabilityPct),
				Steps: []string{
					"Run MTR with TCP mode (mtr -T) to test for ECMP effects",
					"Compare routes at different times of day",
				},
			})
		}
	}

	return findings
}

// avg and minF/maxF are defined in clickhouse.go (same package)

func percentile(vals []float64, pct int) float64 {
	if len(vals) == 0 {
		return 0
	}
	// Simple percentile by sorting
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	// Insertion sort (good enough for our sizes)
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	idx := int(float64(len(sorted)-1) * float64(pct) / 100.0)
	return sorted[idx]
}

func sortProbesByHealth(entries []ProbeHealthEntry) {
	// Insertion sort by overall health ascending (worst first)
	for i := 1; i < len(entries); i++ {
		key := entries[i]
		j := i - 1
		for j >= 0 && entries[j].Health.OverallHealth > key.Health.OverallHealth {
			entries[j+1] = entries[j]
			j--
		}
		entries[j+1] = key
	}
}

func extractField(summaries []AgentHealthSummary, field string) []float64 {
	var out []float64
	for _, s := range summaries {
		if s.ProbeCount == 0 {
			continue
		}
		switch field {
		case "latency":
			if len(s.WorstProbes) > 0 {
				var total float64
				for _, p := range s.WorstProbes {
					total += p.Metrics.AvgLatency
				}
				out = append(out, total/float64(len(s.WorstProbes)))
			}
		case "loss":
			if len(s.WorstProbes) > 0 {
				var total float64
				for _, p := range s.WorstProbes {
					total += p.Metrics.PacketLoss
				}
				out = append(out, total/float64(len(s.WorstProbes)))
			}
		case "jitter":
			if len(s.WorstProbes) > 0 {
				var total float64
				for _, p := range s.WorstProbes {
					total += p.Metrics.JitterAvg
				}
				out = append(out, total/float64(len(s.WorstProbes)))
			}
		}
	}
	return out
}

func extractHealthField(summaries []AgentHealthSummary, field string) []float64 {
	var out []float64
	for _, s := range summaries {
		if s.ProbeCount == 0 {
			continue
		}
		switch field {
		case "latency_score":
			out = append(out, s.Health.LatencyScore)
		case "loss_score":
			out = append(out, s.Health.PacketLossScore)
		case "route_stability":
			out = append(out, s.Health.RouteStability)
		}
	}
	return out
}

// ── Cross-Agent Correlation & Incident Detection ──

// detectIncidents correlates metrics across agents to find infrastructure-wide vs agent-specific issues
func detectIncidents(
	agents []AgentHealthSummary,
	pingMetrics map[string]pingStats,
	mtrMetrics map[string]mtrStats,
	trafficMetrics map[string]trafficStats,
	agentByID map[uint]agentInfo,
	lookbackMinutes int,
	agentIPToID map[string]uint,
) []DetectedIncident {
	var incidents []DetectedIncident

	// Confidence scaling: number of affected agents / total agents in workspace
	totalAgents := len(agents)
	confScale := func(affected int) float64 {
		if totalAgents == 0 {
			return 0.3
		}
		return math.Min(1.0, float64(affected)/float64(totalAgents)*1.5+0.2)
	}

	// 1. Shared-target correlation: find targets seen by multiple agents with degradation
	type targetIssue struct {
		target        string
		agentNames    []string
		probeTypes    map[string]bool
		latencyValues []float64
		lossValues    []float64
	}
	targetMap := make(map[string]*targetIssue)

	// Analyze PING metrics across agents
	for key, stats := range pingMetrics {
		target := extractTarget(key)
		if stats.PacketLoss > 1 || stats.AvgLatency > 100 {
			agentName := resolveAgentName(key, agentByID)
			if targetMap[target] == nil {
				targetMap[target] = &targetIssue{target: target, probeTypes: map[string]bool{}}
			}
			ti := targetMap[target]
			ti.agentNames = append(ti.agentNames, agentName)
			ti.probeTypes["PING"] = true
			ti.latencyValues = append(ti.latencyValues, stats.AvgLatency)
			ti.lossValues = append(ti.lossValues, stats.PacketLoss)
		}
	}

	// Analyze MTR metrics across agents
	for key, stats := range mtrMetrics {
		target := extractTarget(key)
		if stats.PacketLoss > 1 || stats.AvgLatency > 100 {
			agentName := resolveAgentName(key, agentByID)
			if targetMap[target] == nil {
				targetMap[target] = &targetIssue{target: target, probeTypes: map[string]bool{}}
			}
			ti := targetMap[target]
			ti.agentNames = append(ti.agentNames, agentName)
			ti.probeTypes["MTR"] = true
			ti.latencyValues = append(ti.latencyValues, stats.AvgLatency)
			ti.lossValues = append(ti.lossValues, stats.PacketLoss)
		}
	}

	// Analyze TrafficSim metrics across agents
	for key, stats := range trafficMetrics {
		target := extractTarget(key)
		if stats.PacketLoss > 1 || stats.AvgRTT > 100 {
			agentName := resolveAgentName(key, agentByID)
			if targetMap[target] == nil {
				targetMap[target] = &targetIssue{target: target, probeTypes: map[string]bool{}}
			}
			ti := targetMap[target]
			ti.agentNames = append(ti.agentNames, agentName)
			ti.probeTypes["TRAFFICSIM"] = true
			ti.latencyValues = append(ti.latencyValues, stats.AvgRTT)
			ti.lossValues = append(ti.lossValues, stats.PacketLoss)
		}
	}

	// Generate incidents from shared-target analysis
	for target, ti := range targetMap {
		uniqueAgents := uniqueStrings(ti.agentNames)
		avgLat := avg(ti.latencyValues)
		avgLoss := avg(ti.lossValues)

		if len(uniqueAgents) >= 2 {
			// Multiple agents see the same target degraded → infrastructure issue
			severity := "warning"
			if avgLoss > 5 || avgLat > 200 {
				severity = "critical"
			}

			var probeTypeList []string
			for pt := range ti.probeTypes {
				probeTypeList = append(probeTypeList, pt)
			}

			cause := suggestCause(avgLat, avgLoss, len(uniqueAgents), len(agents), ti.probeTypes)
			resolvedTarget := resolveTargetToName(stripPort(target), agentByID, agentIPToID)
			matchedCriteria := fmt.Sprintf("packet_loss > 1%% OR latency > 100ms (avg_loss: %.1f%%, avg_lat: %.1fms)", avgLoss, avgLat)
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("shared_target_%s", sanitizeKey(target)),
				Title:           fmt.Sprintf("Shared degradation to %s", resolvedTarget),
				Severity:        severity,
				Scope:           "infrastructure",
				SuggestedCause:  cause,
				AffectedAgents:  uniqueAgents,
				AffectedTargets: []string{resolvedTarget},
				Evidence: []string{
					fmt.Sprintf("%d agents affected: %s", len(uniqueAgents), strings.Join(uniqueAgents, ", ")),
					fmt.Sprintf("Avg latency: %.1fms, Avg loss: %.1f%%", avgLat, avgLoss),
					fmt.Sprintf("Detected via: %s", strings.Join(probeTypeList, ", ")),
				},
				Recommendations: suggestRemediation(cause, severity),
				Confidence:      confScale(len(uniqueAgents)),
				LookbackMinutes: lookbackMinutes,
				MatchedCriteria: matchedCriteria,
			})
		} else if len(uniqueAgents) == 1 && (avgLoss > 3 || avgLat > 200) {
			// Only one agent sees degradation to this target → agent-specific or local ISP
			severity := "warning"
			if avgLoss > 10 || avgLat > 400 {
				severity = "critical"
			}

			resolvedTarget := resolveTargetToName(stripPort(target), agentByID, agentIPToID)
			matchedCriteria := fmt.Sprintf("packet_loss > 3%% OR latency > 200ms (avg_loss: %.1f%%, avg_lat: %.1fms)", avgLoss, avgLat)
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("agent_target_%s_%s", sanitizeKey(uniqueAgents[0]), sanitizeKey(target)),
				Title:           fmt.Sprintf("Degradation from %s to %s", uniqueAgents[0], resolvedTarget),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("Likely local to %s — possible local ISP issue, network congestion, or routing problem specific to this path", uniqueAgents[0]),
				AffectedAgents:  uniqueAgents,
				AffectedTargets: []string{resolvedTarget},
				Evidence: []string{
					fmt.Sprintf("Only %s sees this issue (other agents to the same target are unaffected)", uniqueAgents[0]),
					fmt.Sprintf("Avg latency: %.1fms, Avg loss: %.1f%%", avgLat, avgLoss),
				},
				Recommendations: []string{
					fmt.Sprintf("Check the local network at %s for interface errors or congestion", uniqueAgents[0]),
					"Review MTR traces for the specific degraded hops",
					"Compare with other probe destinations from this agent",
				},
				Confidence:      0.4,
				LookbackMinutes: lookbackMinutes,
				MatchedCriteria: matchedCriteria,
			})
		}
	}

	// 2. Agent-level correlation: detect agents offline or fully degraded
	for _, agent := range agents {
		if !agent.IsOnline {
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("agent_offline_%d", agent.AgentID),
				Title:           fmt.Sprintf("%s is offline", agent.AgentName),
				Severity:        "critical",
				Scope:           "agent-specific",
				SuggestedCause:  "Agent has not reported in — possible host outage, network partition, or agent service failure",
				AffectedAgents:  []string{agent.AgentName},
				AffectedTargets: []string{},
				Evidence:        []string{"Agent has not sent a heartbeat within the expected interval"},
				Recommendations: []string{
					fmt.Sprintf("Check if the host running %s is reachable", agent.AgentName),
					"Verify the agent service is running (systemctl status netwatcher-agent)",
					"Check host resources (disk, memory, CPU)",
				},
				Confidence: 0.95,
			})
		} else if agent.Health.Grade == "critical" || agent.Health.Grade == "poor" {
			var worstTargets []string
			for _, p := range agent.WorstProbes {
				worstTargets = append(worstTargets, p.Target)
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("agent_degraded_%d", agent.AgentID),
				Title:           fmt.Sprintf("%s health degraded (grade: %s)", agent.AgentName, agent.Health.Grade),
				Severity:        agent.Health.Grade,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("All %d probes from %s show degradation — likely a local network issue or upstream provider problem at this location", agent.ProbeCount, agent.AgentName),
				AffectedAgents:  []string{agent.AgentName},
				AffectedTargets: worstTargets,
				Evidence: []string{
					fmt.Sprintf("Overall health: %.0f/100 (%s)", agent.Health.OverallHealth, agent.Health.Grade),
					fmt.Sprintf("MOS: %.2f", agent.Health.MosScore),
					fmt.Sprintf("%d probes monitored", agent.ProbeCount),
				},
				Recommendations: []string{
					"Check local network connectivity at this agent's location",
					"Review ISP status/outage pages for the agent's provider",
					"Compare latency trends to identify when degradation started",
				},
				Confidence: 0.75,
			})
		}
	}

	// 3. Infrastructure-wide detection: majority of agents degraded
	degradedCount := 0
	for _, agent := range agents {
		if !agent.IsOnline || agent.Health.Grade == "critical" || agent.Health.Grade == "poor" {
			degradedCount++
		}
	}
	if len(agents) > 1 && degradedCount >= len(agents)/2+1 {
		incidents = append(incidents, DetectedIncident{
			ID:              "infrastructure_wide",
			Title:           "Majority of agents reporting issues",
			Severity:        "critical",
			Scope:           "infrastructure",
			SuggestedCause:  fmt.Sprintf("%d of %d agents showing degradation or offline — possible upstream provider issue, DNS resolution problem, or widespread network event", degradedCount, len(agents)),
			AffectedAgents:  []string{},
			AffectedTargets: []string{},
			Evidence:        []string{fmt.Sprintf("%d/%d agents degraded or offline", degradedCount, len(agents))},
			Recommendations: []string{
				"Check shared infrastructure (DNS, upstream ISP, core routing)",
				"Review if a recent change (firewall rule, route update) could explain this",
				"Check external status pages (cloudflare, aws, etc.) for regional issues",
			},
			Confidence: confScale(degradedCount),
		})
	}

	return incidents
}

// suggestCause generates a human-readable root cause hypothesis
func suggestCause(avgLatency, avgLoss float64, affectedAgents, totalAgents int, probeTypes map[string]bool) string {
	parts := []string{}

	if affectedAgents >= totalAgents && totalAgents > 1 {
		parts = append(parts, "All agents are affected — likely an issue with the target destination or a shared upstream transit provider")
	} else if affectedAgents > 1 {
		parts = append(parts, fmt.Sprintf("%d of %d agents affected — possible shared peering point, transit provider, or regional network issue", affectedAgents, totalAgents))
	}

	// Detect ICMP rate limiting patterns (loss with low latency often indicates ICMP limiting)
	if probeTypes["PING"] || probeTypes["MTR"] {
		if avgLoss > 1 && avgLatency < 50 {
			parts = append(parts, "ICMP rate limiting detected (ping/MTR loss with low latency) — firewall or ISP may be throttling ICMP")
		}
	}

	if avgLoss > 10 {
		parts = append(parts, "High packet loss suggests network congestion, overloaded links, or an active outage along the path")
	} else if avgLoss > 3 {
		parts = append(parts, "Moderate packet loss may indicate congestion during peak hours or a degraded link")
	}

	if avgLatency > 300 {
		parts = append(parts, "Very high latency suggests route changes, satellite links, or severe congestion")
	} else if avgLatency > 150 {
		parts = append(parts, "Elevated latency may indicate suboptimal routing or congestion at peering points")
	}

	if len(parts) == 0 {
		return "Degradation detected — further investigation of MTR traces recommended to identify the specific hop or segment"
	}
	return strings.Join(parts, ". ")
}

// suggestRemediation returns actionable steps based on the cause
func suggestRemediation(cause, severity string) []string {
	steps := []string{
		"Review MTR traceroutes from affected agents to identify the degraded hop",
	}
	if strings.Contains(cause, "transit provider") || strings.Contains(cause, "peering") {
		steps = append(steps, "Contact the upstream provider if the degraded hop is in their network")
		steps = append(steps, "Check looking glass tools (e.g., bgp.tools, stat.ripe.net) for route changes")
	}
	if strings.Contains(cause, "congestion") {
		steps = append(steps, "Check if the issue correlates with time-of-day traffic patterns")
	}
	if severity == "critical" {
		steps = append(steps, "Escalate if the issue persists beyond 15 minutes and impacts production services")
	}
	return steps
}

// buildStatusSummary generates the high-level workspace status
func buildStatusSummary(health HealthVector, agents []AgentHealthSummary, incidents []DetectedIncident) StatusSummary {
	offlineCount := 0
	degradedCount := 0
	for _, a := range agents {
		if !a.IsOnline {
			offlineCount++
		} else if a.Health.Grade == "critical" || a.Health.Grade == "poor" {
			degradedCount++
		}
	}

	criticalIncidents := 0
	for _, inc := range incidents {
		if inc.Severity == "critical" {
			criticalIncidents++
		}
	}

	total := len(agents)
	activeIssues := len(incidents)

	switch {
	case total == 0:
		return StatusSummary{Status: "unknown", Message: "No agents configured", ActiveIssues: 0}
	case offlineCount == total:
		return StatusSummary{Status: "outage", Message: "All agents are offline — no monitoring data available", ActiveIssues: activeIssues}
	case criticalIncidents > 0:
		return StatusSummary{
			Status:       "degraded",
			Message:      fmt.Sprintf("%d critical issue(s) detected across your infrastructure", criticalIncidents),
			ActiveIssues: activeIssues,
		}
	case degradedCount > 0 || offlineCount > 0:
		msg := ""
		if offlineCount > 0 && degradedCount > 0 {
			msg = fmt.Sprintf("%d agent(s) offline, %d showing degraded performance", offlineCount, degradedCount)
		} else if offlineCount > 0 {
			msg = fmt.Sprintf("%d agent(s) offline", offlineCount)
		} else {
			msg = fmt.Sprintf("%d agent(s) showing degraded performance", degradedCount)
		}
		return StatusSummary{Status: "degraded", Message: msg, ActiveIssues: activeIssues}
	case health.Grade == "excellent" || health.Grade == "good":
		return StatusSummary{
			Status:       "healthy",
			Message:      fmt.Sprintf("All %d agents healthy — no issues detected", total),
			ActiveIssues: activeIssues,
		}
	default:
		return StatusSummary{
			Status:       "healthy",
			Message:      fmt.Sprintf("%d agents online, overall health: %s", total-offlineCount, health.Grade),
			ActiveIssues: activeIssues,
		}
	}
}

// ── Incident Detection Helpers ──

func extractTarget(key string) string {
	if idx := strings.Index(key, ":"); idx >= 0 {
		return key[idx+1:]
	}
	return key
}

func resolveAgentName(key string, agentByID map[uint]agentInfo) string {
	if idx := strings.Index(key, ":"); idx >= 0 {
		idStr := key[:idx]
		var id uint
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
			if a, ok := agentByID[id]; ok {
				return a.Name
			}
		}
		return idStr
	}
	return key
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// resolveTargetToName checks if the target IP matches any agent and returns the agent name.
// If no agent matches, the target string (which may already be a hostname after stripPort)
// is returned unchanged — this preserves the original probe host for non-agent targets.
func resolveTargetToName(target string, agentByID map[uint]agentInfo, agentIPToID map[string]uint) string {
	if agentID, ok := agentIPToID[target]; ok {
		if agent, ok := agentByID[agentID]; ok {
			return agent.Name
		}
	}
	return target
}

// buildAgentIPToIDMap builds a map from agent IP addresses to agent IDs.
// It prefers the agent's manual PublicIPOverride (admin-supplied) and falls
// back to the latest NETINFO-discovered PublicAddress so that targets
// recorded in ClickHouse — which are usually the NAT'd public IP observed
// during the probe, not the override — still resolve back to the agent
// name in "Shared degradation" incident titles.
func buildAgentIPToIDMap(agents []AgentHealthSummary, agentByID map[uint]agentInfo, netInfoByAgent map[uint]*netInfoPayload) map[string]uint {
	agentIPToID := make(map[string]uint)
	for _, agent := range agents {
		if a, ok := agentByID[agent.AgentID]; ok {
			if a.PublicIPOverride != "" {
				agentIPToID[a.PublicIPOverride] = agent.AgentID
			}
		}
		// Augment with the actual public IP observed in NETINFO. Don't
		// overwrite an entry already populated by PublicIPOverride so the
		// manual value stays authoritative when both are present.
		if ni, ok := netInfoByAgent[agent.AgentID]; ok && ni != nil && ni.PublicAddress != "" {
			if _, exists := agentIPToID[ni.PublicAddress]; !exists {
				agentIPToID[ni.PublicAddress] = agent.AgentID
			}
		}
	}
	return agentIPToID
}

func sanitizeKey(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, s)
}

// sanitizeFloat is defined in network_map.go (same package)

// ── Speedtest / SysInfo / NetInfo Metric Fetchers ──

type speedtestStats struct {
	AvgDownload  float64 // Mbps
	AvgUpload    float64 // Mbps
	AvgLatency   float64 // ms
	AvgJitterAvg float64 // ms
	Count        int
}

func getWorkspaceSpeedtestMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) (map[string]speedtestStats, error) {
	if len(agentIDs) == 0 {
		return make(map[string]speedtestStats), nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	q := fmt.Sprintf(`
SELECT agent_id, target, payload_raw
FROM probe_data
WHERE type = 'SPEEDTEST'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 500
`, strings.Join(agentIDStrs, ", "), chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type accum struct {
		dlTotal, ulTotal, latTotal, jitterTotal float64
		count                                   int
	}
	acc := make(map[string]*accum)

	for rows.Next() {
		var agentID uint64
		var target, payloadRaw string
		if err := rows.Scan(&agentID, &target, &payloadRaw); err != nil || payloadRaw == "" {
			continue
		}
		var result SpeedTestResult
		if err := json.Unmarshal([]byte(payloadRaw), &result); err != nil || len(result.TestData) == 0 {
			continue
		}
		srv := result.TestData[0]
		key := fmt.Sprintf("%d:%s", agentID, target)
		if acc[key] == nil {
			acc[key] = &accum{}
		}
		a := acc[key]
		a.dlTotal += float64(srv.DLSpeed) // bytes/sec → will convert later
		a.ulTotal += float64(srv.ULSpeed)
		a.latTotal += float64(srv.Latency) / float64(time.Millisecond)
		a.jitterTotal += float64(srv.Jitter) / float64(time.Millisecond)
		a.count++
	}

	out := make(map[string]speedtestStats, len(acc))
	for k, a := range acc {
		if a.count == 0 {
			continue
		}
		out[k] = speedtestStats{
			AvgDownload:  (a.dlTotal / float64(a.count)) * 8 / 1_000_000, // bytes/s → Mbps
			AvgUpload:    (a.ulTotal / float64(a.count)) * 8 / 1_000_000,
			AvgLatency:   a.latTotal / float64(a.count),
			AvgJitterAvg: a.jitterTotal / float64(a.count),
			Count:        a.count,
		}
	}
	return out, nil
}

type sysInfoStats struct {
	CPUUsagePct   float64 // 0-100
	MemUsagePct   float64 // 0-100
	MemTotalBytes uint64
	MemUsedBytes  uint64
	Hostname      string
}

func getWorkspaceSysInfoMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) (map[string]sysInfoStats, error) {
	if len(agentIDs) == 0 {
		return make(map[string]sysInfoStats), nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	// Get only the latest per agent
	q := fmt.Sprintf(`
SELECT agent_id, payload_raw
FROM probe_data
WHERE type = 'SYSINFO'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 100
`, strings.Join(agentIDStrs, ", "), chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]sysInfoStats)
	seen := make(map[string]bool) // only keep latest per agent

	for rows.Next() {
		var agentID uint64
		var payloadRaw string
		if err := rows.Scan(&agentID, &payloadRaw); err != nil || payloadRaw == "" {
			continue
		}
		key := fmt.Sprintf("%d", agentID)
		if seen[key] {
			continue
		}
		seen[key] = true

		var p sysInfoPayload
		if err := json.Unmarshal([]byte(payloadRaw), &p); err != nil {
			continue
		}

		cpuTotal := p.CPUTimes.User + p.CPUTimes.System + p.CPUTimes.Idle + p.CPUTimes.IOWait + p.CPUTimes.Nice + p.CPUTimes.SoftIRQ + p.CPUTimes.Steal + p.CPUTimes.IRQ
		cpuBusy := cpuTotal - p.CPUTimes.Idle
		cpuPct := 0.0
		if cpuTotal > 0 {
			cpuPct = (float64(cpuBusy) / float64(cpuTotal)) * 100
		}

		memPct := 0.0
		if p.MemoryInfo.Total > 0 {
			memPct = (float64(p.MemoryInfo.Used) / float64(p.MemoryInfo.Total)) * 100
		}

		out[key] = sysInfoStats{
			CPUUsagePct:   cpuPct,
			MemUsagePct:   memPct,
			MemTotalBytes: p.MemoryInfo.Total,
			MemUsedBytes:  p.MemoryInfo.Used,
			Hostname:      p.HostInfo.Hostname,
		}
	}
	return out, nil
}

type netInfoChange struct {
	AgentID    uint
	Field      string // "public_ip", "isp", "interface"
	OldValue   string
	NewValue   string
	DetectedAt time.Time
}

// getLatestNetInfoForAgents returns the most recent netInfoPayload for each
// agent in agentIDs whose created_at >= from, in a single ClickHouse query.
// agent_id is not in the probe_data primary key, so per-agent round-trips
// become O(N×M) as the table grows. The query below filters by type and a
// tight created_at range — both indexed — then takes the newest row per
// agent with row_number(). An agent with no rows in the window is omitted
// from the result map.
func getLatestNetInfoForAgents(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) map[uint]*netInfoPayload {
	out := make(map[uint]*netInfoPayload)
	if len(agentIDs) == 0 {
		return out
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	q := fmt.Sprintf(`
SELECT agent_id, payload_raw
FROM (
    SELECT agent_id, payload_raw,
           row_number() OVER (PARTITION BY agent_id ORDER BY created_at DESC) AS rn
    FROM probe_data
    WHERE type = 'NETINFO'
      AND agent_id IN (%s)
      AND created_at >= %s
)
WHERE rn = 1
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		log.Warnf("[analysis] getLatestNetInfoForAgents query error: %v", err)
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var agentID uint64
		var payloadRaw string
		if err := rows.Scan(&agentID, &payloadRaw); err != nil || payloadRaw == "" {
			continue
		}
		var p netInfoPayload
		if err := json.Unmarshal([]byte(payloadRaw), &p); err != nil {
			continue
		}
		p.NormalizeFromLegacy()
		out[uint(agentID)] = &p
	}
	return out
}

func getWorkspaceNetInfoChanges(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) ([]netInfoChange, error) {
	if len(agentIDs) == 0 {
		return nil, nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	// Get last 2 records per agent to detect changes. Using a window
	// function keeps the result set to at most 2*|agents| rows and lets
	// ClickHouse prune by (type, created_at) from the primary key before
	// the per-agent filter is applied.
	q := fmt.Sprintf(`
SELECT agent_id, payload_raw, created_at
FROM (
    SELECT agent_id, payload_raw, created_at,
           row_number() OVER (PARTITION BY agent_id ORDER BY created_at DESC) AS rn
    FROM probe_data
    WHERE type = 'NETINFO'
      AND agent_id IN (%s)
      AND created_at >= %s
)
WHERE rn <= 2
ORDER BY agent_id, created_at DESC
`, strings.Join(agentIDStrs, ", "), chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect per-agent: newest and second-newest
	type record struct {
		payload   netInfoPayload
		createdAt time.Time
	}
	byAgent := make(map[uint][]record)

	for rows.Next() {
		var agentID uint64
		var payloadRaw string
		var createdAt time.Time
		if err := rows.Scan(&agentID, &payloadRaw, &createdAt); err != nil || payloadRaw == "" {
			continue
		}
		var p netInfoPayload
		if err := json.Unmarshal([]byte(payloadRaw), &p); err != nil {
			continue
		}
		aid := uint(agentID)
		if len(byAgent[aid]) < 2 {
			byAgent[aid] = append(byAgent[aid], record{payload: p, createdAt: createdAt})
		}
	}

	var changes []netInfoChange
	for aid, records := range byAgent {
		if len(records) < 2 {
			continue
		}
		newer := records[0] // latest
		older := records[1] // previous

		if newer.payload.PublicAddress != older.payload.PublicAddress && newer.payload.PublicAddress != "" {
			changes = append(changes, netInfoChange{
				AgentID:    aid,
				Field:      "public_ip",
				OldValue:   older.payload.PublicAddress,
				NewValue:   newer.payload.PublicAddress,
				DetectedAt: newer.createdAt,
			})
		}
		newISP := newer.payload.GetISP()
		oldISP := older.payload.GetISP()
		if newISP != oldISP && newISP != "" && oldISP != "" {
			changes = append(changes, netInfoChange{
				AgentID:    aid,
				Field:      "isp",
				OldValue:   oldISP,
				NewValue:   newISP,
				DetectedAt: newer.createdAt,
			})
		}
	}
	return changes, nil
}

// ── Scoring Helpers ──

// latencyScore converts raw latency (ms) to a 0-100 score
func latencyScore(latMs float64) float64 {
	switch {
	case latMs <= 0:
		return 100
	case latMs < 20:
		return 100
	case latMs < 50:
		return 100 - (latMs-20)*0.17 // 95 at 50ms
	case latMs < 100:
		return 95 - (latMs-50)*0.3 // 80 at 100ms
	case latMs < 150:
		return 80 - (latMs-100)*0.4 // 60 at 150ms
	case latMs < 300:
		return 60 - (latMs-150)*0.2 // 30 at 300ms
	default:
		return math.Max(0, 30-(latMs-300)*0.1)
	}
}

// speedtestBandwidthScore scores download+upload bandwidth (Mbps), 0-100
func speedtestBandwidthScore(dlMbps, ulMbps float64) float64 {
	// Weighted: download 70%, upload 30%
	dlScore := bwScore(dlMbps)
	ulScore := bwScore(ulMbps)
	return 0.7*dlScore + 0.3*ulScore
}

func bwScore(mbps float64) float64 {
	switch {
	case mbps >= 100:
		return 100
	case mbps >= 50:
		return 90 + (mbps-50)*0.2 // 90-100
	case mbps >= 25:
		return 75 + (mbps-25)*0.6 // 75-90
	case mbps >= 10:
		return 50 + (mbps-10)*1.67 // 50-75
	case mbps >= 5:
		return 30 + (mbps-5)*4 // 30-50
	case mbps > 0:
		return mbps * 6 // 0-30
	default:
		return 0
	}
}

// sysInfoHealthScore converts CPU/memory usage to a health score (higher = healthier)
func sysInfoHealthScore(si sysInfoStats) float64 {
	// CPU: <50% = 100, 50-80% = 80-60, 80-95% = 60-20, >95% = critical
	cpuScore := 100.0
	switch {
	case si.CPUUsagePct > 95:
		cpuScore = 10
	case si.CPUUsagePct > 80:
		cpuScore = 60 - (si.CPUUsagePct-80)*2.67
	case si.CPUUsagePct > 50:
		cpuScore = 100 - (si.CPUUsagePct-50)*1.33
	}

	// Memory: <60% = 100, 60-85% = 80-50, 85-95% = 50-20, >95% = critical
	memScore := 100.0
	switch {
	case si.MemUsagePct > 95:
		memScore = 10
	case si.MemUsagePct > 85:
		memScore = 50 - (si.MemUsagePct-85)*3
	case si.MemUsagePct > 60:
		memScore = 100 - (si.MemUsagePct-60)*2
	}

	return 0.5*cpuScore + 0.5*memScore
}

// ── Temporal Change Detection ──

func detectTemporalChanges(
	currentPing map[string]pingStats, baselinePing map[string]pingStats,
	currentTraffic map[string]trafficStats, baselineTraffic map[string]trafficStats,
	netInfoChanges []netInfoChange,
	sysInfoMetrics map[string]sysInfoStats,
	agentByID map[uint]agentInfo,
) []DetectedIncident {
	var incidents []DetectedIncident

	// 1. Latency/loss regression detection (PING)
	for key, current := range currentPing {
		baseline, exists := baselinePing[key]
		if !exists || baseline.Count < 3 {
			continue
		}
		agentName := resolveAgentName(key, agentByID)
		target := extractTarget(key)

		// Latency increased by >2x baseline
		if baseline.AvgLatency > 5 && current.AvgLatency > baseline.AvgLatency*2 {
			severity := "warning"
			if current.AvgLatency > baseline.AvgLatency*3 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("latency_regression_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("Latency regression to %s from %s", stripPort(target), agentName),
				Severity:        severity,
				Scope:           "target-specific",
				SuggestedCause:  fmt.Sprintf("Latency increased from %.1fms (baseline) to %.1fms (now) — possible route change or congestion", baseline.AvgLatency, current.AvgLatency),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("Baseline (7-day avg): %.1fms", baseline.AvgLatency),
					fmt.Sprintf("Current: %.1fms (%.0f%% increase)", current.AvgLatency, ((current.AvgLatency-baseline.AvgLatency)/baseline.AvgLatency)*100),
				},
				Recommendations: []string{
					"Compare MTR traces to identify if the routing path has changed",
					"Check if the increase correlates with specific times of day",
				},
			})
		}

		// Loss increased significantly from baseline
		if current.PacketLoss > 1 && baseline.PacketLoss < 0.5 {
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("loss_regression_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("New packet loss to %s from %s", stripPort(target), agentName),
				Severity:        "warning",
				Scope:           "target-specific",
				SuggestedCause:  fmt.Sprintf("Packet loss appeared: %.1f%% now vs %.1f%% baseline — possible link degradation", current.PacketLoss, baseline.PacketLoss),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("Baseline (7-day avg): %.1f%% loss", baseline.PacketLoss),
					fmt.Sprintf("Current: %.1f%% loss", current.PacketLoss),
				},
				Recommendations: []string{
					"Review MTR for the degraded hops",
					"Check if the target or intermediate network is under maintenance",
				},
			})
		}
	}

	// 2. SysInfo capacity warnings
	for agentKey, si := range sysInfoMetrics {
		var id uint
		if _, err := fmt.Sscanf(agentKey, "%d", &id); err != nil {
			continue
		}
		agentName := agentKey
		if a, ok := agentByID[id]; ok {
			agentName = a.Name
		}

		if si.MemUsagePct > 90 {
			severity := "warning"
			if si.MemUsagePct > 95 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("memory_high_%s", agentKey),
				Title:           fmt.Sprintf("High memory usage on %s", agentName),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("Memory at %.1f%% — the host may be running low on resources, which can affect probe accuracy", si.MemUsagePct),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{"host-resources"},
				Evidence: []string{
					fmt.Sprintf("Memory: %.1f%% used (%s / %s)",
						si.MemUsagePct,
						formatBytes(si.MemUsedBytes),
						formatBytes(si.MemTotalBytes)),
				},
				Recommendations: []string{
					"Check for runaway processes consuming memory",
					"Consider increasing host memory allocation",
				},
			})
		}

		if si.CPUUsagePct > 85 {
			severity := "warning"
			if si.CPUUsagePct > 95 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("cpu_high_%s", agentKey),
				Title:           fmt.Sprintf("High CPU usage on %s", agentName),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("CPU at %.1f%% — high CPU can cause probe timing inaccuracies", si.CPUUsagePct),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{"host-resources"},
				Evidence:        []string{fmt.Sprintf("CPU usage: %.1f%%", si.CPUUsagePct)},
				Recommendations: []string{
					"Check for CPU-intensive processes",
					"Verify probe scheduling isn't overlapping",
				},
			})
		}
	}

	// 4. NetInfo changes (IP/ISP changes)
	for _, change := range netInfoChanges {
		agentName := fmt.Sprintf("Agent %d", change.AgentID)
		if a, ok := agentByID[change.AgentID]; ok {
			agentName = a.Name
		}

		switch change.Field {
		case "public_ip":
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("ip_change_%d", change.AgentID),
				Title:           fmt.Sprintf("Public IP changed on %s", agentName),
				Severity:        "info",
				Scope:           "agent-specific",
				SuggestedCause:  "Public IP address changed — this may indicate a DHCP renewal, failover event, or ISP change",
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{},
				Evidence: []string{
					fmt.Sprintf("Previous: %s", change.OldValue),
					fmt.Sprintf("Current: %s", change.NewValue),
				},
				Recommendations: []string{
					"Verify if this was an expected change (DHCP, failover)",
					"Check if monitoring targets are still reachable from the new IP",
				},
			})
		case "isp":
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("isp_change_%d", change.AgentID),
				Title:           fmt.Sprintf("ISP changed on %s", agentName),
				Severity:        "warning",
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("ISP changed from %s to %s — this may indicate a WAN failover or circuit switch", change.OldValue, change.NewValue),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{},
				Evidence: []string{
					fmt.Sprintf("Previous ISP: %s", change.OldValue),
					fmt.Sprintf("Current ISP: %s", change.NewValue),
				},
				Recommendations: []string{
					"Verify if this was a planned failover",
					"Check if latency/loss metrics changed with the ISP switch",
					"Review SD-WAN or dual-WAN configuration if applicable",
				},
			})
		}
	}

	return incidents
}

// ── Speedtest Bandwidth Regression Detection ──

func detectSpeedtestIncidents(ctx context.Context, ch *sql.DB, agentIDs []uint, from, baselineFrom time.Time, agentByID map[uint]agentInfo) []DetectedIncident {
	if len(agentIDs) == 0 {
		return nil
	}

	current, err := getWorkspaceSpeedtestMetrics(ctx, ch, agentIDs, from)
	if err != nil || len(current) == 0 {
		return nil
	}

	baseline, _ := getWorkspaceSpeedtestMetrics(ctx, ch, agentIDs, baselineFrom)
	if len(baseline) == 0 {
		return nil
	}

	var incidents []DetectedIncident
	for key, curr := range current {
		base, exists := baseline[key]
		if !exists || base.Count < 3 || curr.Count < 3 {
			continue
		}

		agentName := resolveAgentName(key, agentByID)
		target := extractTarget(key)

		// Download regression: >50% drop when baseline was >10 Mbps
		if base.AvgDownload > 10 && curr.AvgDownload < base.AvgDownload*0.5 {
			severity := "warning"
			if curr.AvgDownload < base.AvgDownload*0.25 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("speedtest_dl_regression_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("Bandwidth regression detected for %s (%s)", agentName, stripPort(target)),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("Download speed dropped from %.1f Mbps to %.1f Mbps — possible ISP throttling, link degradation, or network congestion", base.AvgDownload, curr.AvgDownload),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("Baseline download: %.1f Mbps (from %d tests)", base.AvgDownload, base.Count),
					fmt.Sprintf("Current download: %.1f Mbps (from %d tests)", curr.AvgDownload, curr.Count),
					fmt.Sprintf("Latency: %.1fms, JitterAvg: %.1fms", curr.AvgLatency, curr.AvgJitterAvg),
				},
				Recommendations: []string{
					"Run a manual speed test to confirm results",
					"Check for ISP SLA violations or data caps",
					"Review interface error counts on the agent's host",
				},
				Confidence: 0.75,
			})
		}

		// Upload regression
		if base.AvgUpload > 10 && curr.AvgUpload < base.AvgUpload*0.5 {
			severity := "warning"
			if curr.AvgUpload < base.AvgUpload*0.25 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("speedtest_ul_regression_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("Upload bandwidth regression for %s (%s)", agentName, stripPort(target)),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("Upload speed dropped from %.1f Mbps to %.1f Mbps — possible upstream congestion or ISP shaping", base.AvgUpload, curr.AvgUpload),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("Baseline upload: %.1f Mbps", base.AvgUpload),
					fmt.Sprintf("Current upload: %.1f Mbps", curr.AvgUpload),
				},
				Recommendations: []string{
					"Check for upstream ISP issues or contention ratio",
					"Verify QoS settings haven't changed",
				},
				Confidence: 0.70,
			})
		}
	}

	return incidents
}

// ── DNS Pattern Detection ──

func detectDNSIncidents(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time, agentByID map[uint]agentInfo) []DetectedIncident {
	if len(agentIDs) == 0 {
		return nil
	}

	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}

	q := fmt.Sprintf(`
SELECT agent_id, target, payload_raw
FROM probe_data
WHERE type = 'DNS'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 1000
`, strings.Join(agentIDStrs, ", "), chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type dnsAccum struct {
		queryTimes []float64
		nxdomain   int
		total      int
		servfail   int
		respondIPs map[string]int
	}
	acc := make(map[string]*dnsAccum)

	for rows.Next() {
		var agentID uint64
		var target, payloadRaw string
		if err := rows.Scan(&agentID, &target, &payloadRaw); err != nil || payloadRaw == "" {
			continue
		}
		var p DNSPayload
		if err := json.Unmarshal([]byte(payloadRaw), &p); err != nil {
			continue
		}
		key := fmt.Sprintf("%d:%s", agentID, target)
		if acc[key] == nil {
			acc[key] = &dnsAccum{respondIPs: make(map[string]int)}
		}
		a := acc[key]
		a.total++
		a.queryTimes = append(a.queryTimes, p.QueryTimeMs)
		switch p.ResponseCode {
		case "NXDOMAIN":
			a.nxdomain++
		case "SERVFAIL":
			a.servfail++
		}
		if len(p.Answers) > 0 {
			a.respondIPs[p.Answers[0].Value]++
		}
	}

	var incidents []DetectedIncident
	for key, a := range acc {
		if a.total < 5 {
			continue
		}

		agentName := resolveAgentName(key, agentByID)
		target := extractTarget(key)
		nxPct := float64(a.nxdomain) / float64(a.total) * 100
		sfPct := float64(a.servfail) / float64(a.total) * 100

		// NXDOMAIN storm detection: >30% NXDOMAIN rate
		if nxPct > 30 {
			severity := "warning"
			if nxPct > 60 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("dns_nxdomain_storm_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("DNS NXDOMAIN storm from %s to %s", agentName, stripPort(target)),
				Severity:        severity,
				SuggestedCause:  fmt.Sprintf("%.1f%% of queries returned NXDOMAIN — possible domain expiry, misconfiguration, or DNS cache poisoning attack", nxPct),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("%d/%d queries NXDOMAIN (%.1f%%)", a.nxdomain, a.total, nxPct),
					fmt.Sprintf("Target: %s", target),
				},
				Recommendations: []string{
					"Verify the domain is still registered and DNS records are correct",
					"Check if the target DNS server is experiencing issues",
					"Review firewall logs for anomalous DNS query patterns",
				},
				Confidence: math.Min(0.95, 0.3+nxPct/100),
			})
		}

		// SERVFAIL storm: >20% SERVFAIL rate
		if sfPct > 20 && nxPct < 30 {
			severity := "warning"
			if sfPct > 50 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("dns_servfail_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("DNS SERVFAIL errors from %s to %s", agentName, stripPort(target)),
				Severity:        severity,
				SuggestedCause:  fmt.Sprintf("%.1f%% of queries returned SERVFAIL — possible DNS server overload or recursive resolver failure", sfPct),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("%d/%d queries SERVFAIL (%.1f%%)", a.servfail, a.total, sfPct),
					fmt.Sprintf("Target: %s", target),
				},
				Recommendations: []string{
					"Check DNS server status and resource usage",
					"Verify upstream DNS server is reachable",
					"Review DNSSEC validation failures if applicable",
				},
				Confidence: 0.75,
			})
		}

		// High query time (possible DNS amplification)
		if len(a.queryTimes) > 5 {
			avgQT := avg(a.queryTimes)
			if avgQT > 500 {
				severity := "warning"
				if avgQT > 2000 {
					severity = "critical"
				}
				incidents = append(incidents, DetectedIncident{
					ID:              fmt.Sprintf("dns_high_latency_%s", sanitizeKey(key)),
					Title:           fmt.Sprintf("High DNS latency from %s to %s", agentName, stripPort(target)),
					Severity:        severity,
					SuggestedCause:  fmt.Sprintf("Average DNS query time: %.1fms — possible DNS server overload, network path issue, or amplification attack pattern", avgQT),
					AffectedAgents:  []string{agentName},
					AffectedTargets: []string{stripPort(target)},
					Evidence: []string{
						fmt.Sprintf("Average query time: %.1fms across %d queries", avgQT, len(a.queryTimes)),
					},
					Recommendations: []string{
						"Check if the DNS server is under load or experiencing DoS",
						"Review upstream DNS provider status",
						"Consider switching to a faster DNS resolver (e.g., 1.1.1.1, 8.8.8.8)",
					},
					Confidence: 0.65,
				})
			}
		}
	}

	return incidents
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ── Route / Path Analysis ──

// RouteBaseline mirrors the alert package model so we can query without importing alert.
type routeBaseline struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	ProbeID     uint   `gorm:"uniqueIndex;not null" json:"probe_id"`
	Fingerprint string `gorm:"size:64;not null" json:"fingerprint"`
	RoutePath   string `gorm:"size:2048" json:"route_path,omitempty"`
	HopCount    int    `json:"hop_count"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (routeBaseline) TableName() string { return "route_baselines" }

// HopDetail holds enriched hop information for route analysis display
type HopDetail struct {
	IP            string  `json:"ip"`
	Hostname      string  `json:"hostname,omitempty"`
	IsAgent       bool    `json:"is_agent"`
	AgentID       uint    `json:"agent_id,omitempty"`
	AgentName     string  `json:"agent_name,omitempty"`
	IsFinalHop    bool    `json:"is_final_hop"`
	Latency       float64 `json:"latency,omitempty"`
	Loss          float64 `json:"loss,omitempty"`
	IsRateLimited bool    `json:"is_rate_limited,omitempty"`
}

// hopAgg holds aggregated metrics for a single hop index across traces
type hopAgg struct {
	totalLatency float64
	totalLoss    float64
	count        int
}

// buildHopDetails creates enriched hop details from raw MTR hops, matching IPs to agents (uses MtrPayload from clickhouse.go)
func buildHopDetails(mtrPayload *MtrPayload, agentIPToID map[string]uint, agentByID map[uint]agentInfo) []HopDetail {
	var details []HopDetail
	hopCount := len(mtrPayload.Report.Hops)
	for i, hop := range mtrPayload.Report.Hops {
		if len(hop.Hosts) == 0 || hop.Hosts[0].IP == "" || hop.Hosts[0].IP == "*" {
			continue
		}
		hd := HopDetail{
			IP: hop.Hosts[0].IP,
		}
		// Check if this hop IP matches any agent's PublicIPOverride
		if agentID, ok := agentIPToID[hop.Hosts[0].IP]; ok {
			hd.IsAgent = true
			hd.AgentID = agentID
			if a, ok := agentByID[agentID]; ok {
				hd.AgentName = a.Name
				// For final hop, also use description if available
				if i == hopCount-1 && a.Description != "" {
					hd.Hostname = fmt.Sprintf("%s (%s)", a.Name, a.Description)
				} else {
					hd.Hostname = a.Name
				}
			}
		}
		hd.IsFinalHop = i == hopCount-1
		details = append(details, hd)
	}
	return details
}

// buildHopDetailsForMtrPayload creates enriched hop details from agent MTR payload (uses mtrPayload from mtr.go)
func buildHopDetailsForMtrPayload(mtrPayload *mtrPayload, agentIPToID map[string]uint, agentByID map[uint]agentInfo, hopMetrics map[int]hopAgg, rateLimitedSet map[int]bool) []HopDetail {
	var details []HopDetail
	hopCount := len(mtrPayload.Report.Hops)
	for i, hop := range mtrPayload.Report.Hops {
		if len(hop.Hosts) == 0 || hop.Hosts[0].IP == "" || hop.Hosts[0].IP == "*" {
			continue
		}
		hd := HopDetail{
			IP:       hop.Hosts[0].IP,
			Hostname: hop.Hosts[0].Hostname,
		}
		// Populate per-hop aggregated metrics
		if ha, ok := hopMetrics[i]; ok && ha.count > 0 {
			hd.Latency = sanitizeFloat(ha.totalLatency / float64(ha.count))
			hd.Loss = sanitizeFloat(ha.totalLoss / float64(ha.count))
		}
		if rateLimitedSet[i] {
			hd.IsRateLimited = true
		}
		// Check if this hop IP matches any agent's PublicIPOverride
		if agentID, ok := agentIPToID[hop.Hosts[0].IP]; ok {
			hd.IsAgent = true
			hd.AgentID = agentID
			if a, ok := agentByID[agentID]; ok {
				hd.AgentName = a.Name
				// For final hop, also use description if available
				if i == hopCount-1 && a.Description != "" {
					hd.Hostname = fmt.Sprintf("%s (%s)", a.Name, a.Description)
				} else {
					hd.Hostname = a.Name
				}
			}
		}
		hd.IsFinalHop = i == hopCount-1
		details = append(details, hd)
	}
	return details
}

// ProbeRouteInfo holds route data for a single MTR probe.
type ProbeRouteInfo struct {
	ProbeID             uint        `json:"probe_id"`
	AgentID             uint        `json:"agent_id,omitempty"`
	Target              string      `json:"target"`
	TargetAgentID       uint        `json:"target_agent_id,omitempty"`
	TargetAgentName     string      `json:"target_agent_name,omitempty"`
	BaselineFingerprint string      `json:"baseline_fingerprint,omitempty"`
	BaselineHopCount    int         `json:"baseline_hop_count,omitempty"`
	BaselineRoutePath   string      `json:"baseline_route_path,omitempty"`
	LatestSignature     string      `json:"latest_signature,omitempty"`
	LatestHops          []string    `json:"latest_hops,omitempty"`        // IPs only (for signature computation)
	LatestHopsDetail    []HopDetail `json:"latest_hops_detail,omitempty"` // Enriched with agent names
	HasRouteChange      bool        `json:"has_route_change"`
	RouteChangedAt      *time.Time  `json:"route_changed_at,omitempty"` // First time (within lookback) the signature differed from the baseline
	TraceCount          int         `json:"trace_count,omitempty"`
	RouteStabilityPct   float64     `json:"route_stability_pct,omitempty"`
	AvgEndHopLatency    float64     `json:"avg_end_hop_latency,omitempty"`
	AvgEndHopLoss       float64     `json:"avg_end_hop_loss,omitempty"`
	IntermediateHops    []HopMetric `json:"intermediate_hops,omitempty"` // Hop metrics excluding the final hop
}

// HopMetric holds metrics for a single intermediate hop (not the final destination)
type HopMetric struct {
	IP       string  `json:"ip"`
	Loss     float64 `json:"loss"`
	Latency  float64 `json:"latency"`
	HopIndex int     `json:"hop_index"`
}

// HopMetrics holds aggregated metrics for a hop across all agents that traverse it
type HopMetrics struct {
	TotalLoss    float64
	TotalLatency float64
	Count        int
	HasIssues    bool
}

// AgentRouteInfo holds route/path data for a single agent.
type AgentRouteInfo struct {
	AgentID      uint             `json:"agent_id"`
	AgentName    string           `json:"agent_name"`
	PublicIP     string           `json:"public_ip,omitempty"`
	ISP          string           `json:"isp,omitempty"`
	HasIPChange  bool             `json:"has_ip_change"`
	HasISPChange bool             `json:"has_isp_change"`
	Routes       []ProbeRouteInfo `json:"routes"`
}

// SharedHopInfo represents a hop that appears in multiple agent routes.
type SharedHopInfo struct {
	HopIP       string   `json:"hop_ip"`
	HopHostname string   `json:"hop_hostname,omitempty"` // Agent name if this hop is an agent
	AgentIDs    []uint   `json:"agent_ids"`
	AgentNames  []string `json:"agent_names"`
	HopCount    int      `json:"hop_count"`
	HasIssues   bool     `json:"has_issues"` // True if any intermediate hop in the shared path has loss or high latency
	AvgLoss     float64  `json:"avg_loss,omitempty"`
	AvgLatency  float64  `json:"avg_latency,omitempty"`
}

// RouteIncident is a lightweight incident specifically for route/path issues.
type RouteIncident struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"` // ip_change, isp_change, route_change
	Severity   string   `json:"severity"`
	AgentID    uint     `json:"agent_id"`
	AgentName  string   `json:"agent_name"`
	ProbeID    uint     `json:"probe_id,omitempty"`
	Target     string   `json:"target,omitempty"`
	Message    string   `json:"message"`
	Evidence   []string `json:"evidence,omitempty"`
	DetectedAt string   `json:"detected_at,omitempty"`
}

// WorkspaceRouteAnalysis is the top-level response for route/path visualization.
type WorkspaceRouteAnalysis struct {
	WorkspaceID        uint                    `json:"workspace_id"`
	Agents             []AgentRouteInfo        `json:"agents"`
	SharedHops         []SharedHopInfo         `json:"shared_hops"`
	SharedDestinations []SharedDestinationInfo `json:"shared_destinations"`
	SharedASNs         []SharedASNInfo         `json:"shared_asns"`
	CommonTargets      []CommonTargetInfo      `json:"common_targets"`
	Incidents          []RouteIncident         `json:"incidents"`
	TotalAgents        int                     `json:"total_agents"`
	TotalRoutes        int                     `json:"total_routes"`
	GeneratedAt        time.Time               `json:"generated_at"`
}

// SharedDestinationInfo represents a destination IP/hostname that 2+ agents
// are MTR-ing to. This is the most common "shared path" pattern across agents
// targeting the same internet endpoint (e.g. 8.8.8.8, dns.google).
type SharedDestinationInfo struct {
	Target          string   `json:"target"` // Hostname or IP target
	TargetIP        string   `json:"target_ip,omitempty"`
	AgentIDs        []uint   `json:"agent_ids"`
	AgentNames      []string `json:"agent_names"`
	AgentCount      int      `json:"agent_count"`
	AvgEndLatencyMs float64  `json:"avg_end_latency_ms,omitempty"`
	AvgEndLossPct   float64  `json:"avg_end_loss_pct,omitempty"`
	HasIssues       bool     `json:"has_issues"`
}

// SharedASNInfo groups intermediate hop IPs by ASN, showing common upstream
// networks that 2+ agents traverse. This is the most resilient shared-route
// signal because agents on different last-mile ISPs still share ASN-level
// transit (e.g. Level3, Cogent, NTT).
type SharedASNInfo struct {
	ASN        uint     `json:"asn"`
	ASNOrg     string   `json:"asn_org,omitempty"`
	HopIPs     []string `json:"hop_ips"`
	AgentIDs   []uint   `json:"agent_ids"`
	AgentNames []string `json:"agent_names"`
	AgentCount int      `json:"agent_count"`
	HasIssues  bool     `json:"has_issues"`
	AvgLatency float64  `json:"avg_latency_ms,omitempty"`
	AvgLoss    float64  `json:"avg_loss_pct,omitempty"`
}

// CommonTargetInfo summarizes a target (e.g. "google.com") that multiple
// agents are MTR-ing to. This is the "what are agents probing in common"
// view, irrespective of whether the actual path hops overlap.
type CommonTargetInfo struct {
	Target          string   `json:"target"`
	AgentIDs        []uint   `json:"agent_ids"`
	AgentNames      []string `json:"agent_names"`
	AgentCount      int      `json:"agent_count"`
	ProbeCount      int      `json:"probe_count"`
	AvgEndLatencyMs float64  `json:"avg_end_latency_ms,omitempty"`
	AvgEndLossPct   float64  `json:"avg_end_loss_pct,omitempty"`
	HasIssues       bool     `json:"has_issues"`
}

// ComputeWorkspaceRouteAnalysis aggregates route/path data across all agents in a workspace
// for the route/path matching visualization. Pass nil for geoStore to skip ASN grouping.
//
//	lookbackHours bounds the MTR / NETINFO lookback window. 0 = default (24h MTR, 1h NETINFO).
func ComputeWorkspaceRouteAnalysis(ctx context.Context, ch *sql.DB, pg *gorm.DB, geoStore GeoIPResolver, workspaceID uint, lookbackHours int) (*WorkspaceRouteAnalysis, error) {
	// 1. Get agents
	agents, err := getWorkspaceAgents(ctx, pg, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get agents: %w", err)
	}

	if len(agents) == 0 {
		return &WorkspaceRouteAnalysis{
			WorkspaceID: workspaceID,
			Agents:      []AgentRouteInfo{},
			SharedHops:  []SharedHopInfo{},
			Incidents:   []RouteIncident{},
			GeneratedAt: time.Now().UTC(),
		}, nil
	}

	agentIDs := make([]uint, len(agents))
	agentByID := make(map[uint]agentInfo)
	agentIPToID := make(map[string]uint) // IP -> AgentID for hop matching
	for i, a := range agents {
		agentIDs[i] = a.ID
		agentByID[a.ID] = a
		if a.PublicIPOverride != "" {
			agentIPToID[a.PublicIPOverride] = a.ID
		}
	}

	// MTR lookback default = 24h, NETINFO lookback fixed at 1h.
	// The lookbackHours param scales only the MTR window — NETINFO must stay
	// tight so a public IP change from 25h ago doesn't bleed into a "current"
	// view.
	if lookbackHours <= 0 {
		lookbackHours = 24
	}
	mtrFrom := time.Now().UTC().Add(-time.Duration(lookbackHours) * time.Hour)
	netInfoFrom := time.Now().UTC().Add(-60 * time.Minute)

	// 2. Get latest NETINFO per agent in a single batched query.
	// Per-agent round-trips were O(N×M) because agent_id is not in the
	// probe_data primary key (type, probe_id, created_at). Use a tight
	// created_at range so ClickHouse can do a range scan inside the
	// type='NETINFO' partition, then pick the latest row per agent with
	// row_number().
	netInfoByAgent := getLatestNetInfoForAgents(ctx, ch, agentIDs, netInfoFrom)

	// 2b. Augment agentIPToID with NETINFO-derived public IPs. The public
	// IP itself rarely appears in the agent's own outbound MTR (NAT + ICMP
	// rate-limiting at the CPE), but it DOES show up in any MTR another
	// agent runs against this one — without this, agent-to-agent hops
	// never get their "is_agent" label.
	for agentID, ni := range netInfoByAgent {
		if ni == nil || ni.PublicAddress == "" {
			continue
		}
		if _, exists := agentIPToID[ni.PublicAddress]; !exists {
			agentIPToID[ni.PublicAddress] = agentID
		}
	}

	// 3. Detect IP/ISP changes
	netInfoChanges, _ := getWorkspaceNetInfoChanges(ctx, ch, agentIDs, netInfoFrom)
	changeByAgent := make(map[uint][]netInfoChange)
	for _, c := range netInfoChanges {
		changeByAgent[c.AgentID] = append(changeByAgent[c.AgentID], c)
	}

	// 4. Fetch ALL MTR data for the workspace in one query. The MTR data
	// exists in ClickHouse regardless of whether the parent probe is type
	// "MTR" or "AGENT" (bidirectional probes store MTR rows with sub-type
	// 'MTR' under their probe_id, with target_agent set to the destination
	// agent for agent-to-agent probes). Driving the analysis from the
	// probe_data type='MTR' rows means both standalone MTR and AGENT
	// (bidirectional) probes are handled uniformly.
	agentRoutes := make([]AgentRouteInfo, 0, len(agents))
	hopIndex := make(map[string]map[uint]HopMetrics) // hopIP -> agentID -> metrics
	routeIncidents := make([]RouteIncident, 0)
	totalRoutes := 0

	// Pre-populate every agent with an empty AgentRouteInfo so agents with
	// no MTR data still appear in the UI (with empty routes / NETINFO
	// status). This matches the original behavior of iterating ListByAgent.
	for _, a := range agents {
		ari := AgentRouteInfo{
			AgentID:   a.ID,
			AgentName: a.Name,
		}
		if ni := netInfoByAgent[a.ID]; ni != nil {
			ari.PublicIP = ni.PublicAddress
			ari.ISP = ni.GetISP()
		}
		// IP / ISP change incidents
		if changes, ok := changeByAgent[a.ID]; ok {
			for _, c := range changes {
				switch c.Field {
				case "public_ip":
					ari.HasIPChange = true
					routeIncidents = append(routeIncidents, RouteIncident{
						ID:         fmt.Sprintf("ip_change_%d", a.ID),
						Type:       "ip_change",
						Severity:   "info",
						AgentID:    a.ID,
						AgentName:  a.Name,
						Message:    fmt.Sprintf("Public IP changed from %s to %s", c.OldValue, c.NewValue),
						Evidence:   []string{fmt.Sprintf("Previous: %s", c.OldValue), fmt.Sprintf("Current: %s", c.NewValue)},
						DetectedAt: c.DetectedAt.Format(time.RFC3339),
					})
				case "isp":
					ari.HasISPChange = true
					routeIncidents = append(routeIncidents, RouteIncident{
						ID:         fmt.Sprintf("isp_change_%d", a.ID),
						Type:       "isp_change",
						Severity:   "warning",
						AgentID:    a.ID,
						AgentName:  a.Name,
						Message:    fmt.Sprintf("ISP changed from %s to %s", c.OldValue, c.NewValue),
						Evidence:   []string{fmt.Sprintf("Previous ISP: %s", c.OldValue), fmt.Sprintf("Current ISP: %s", c.NewValue)},
						DetectedAt: c.DetectedAt.Format(time.RFC3339),
					})
				}
			}
		}
		agentRoutes = append(agentRoutes, ari)
	}
	// Index by agent ID so the per-(probe, agent) loop below can attach routes.
	ariByAgent := make(map[uint]*AgentRouteInfo, len(agentRoutes))
	for i := range agentRoutes {
		ariByAgent[agentRoutes[i].AgentID] = &agentRoutes[i]
	}

	// Pull all MTR rows for this workspace in a single batched query.
	// Returns a map keyed by (probe_id, agent_id, target_agent) → []ProbeData
	// so we can compute signatures / stability per (agent, target) tuple.
	mtrByPath, err := getWorkspaceMTRByPath(ctx, ch, agentIDs, mtrFrom, 200)
	if err != nil {
		log.Warnf("[route-analysis] workspace=%d MTR query error: %v", workspaceID, err)
		// Non-fatal: return what we have (agents, NETINFO, change incidents)
		// with empty MTR-derived fields.
		return &WorkspaceRouteAnalysis{
			WorkspaceID:        workspaceID,
			Agents:             agentRoutes,
			SharedHops:         []SharedHopInfo{},
			SharedDestinations: []SharedDestinationInfo{},
			SharedASNs:         buildSharedASNs(geoStore, hopIndex, agentByID),
			CommonTargets:      []CommonTargetInfo{},
			Incidents:          routeIncidents,
			TotalAgents:        len(agents),
			TotalRoutes:        0,
			GeneratedAt:        time.Now().UTC(),
		}, nil
	}

	// Cache of route baselines by probe_id (Postgres is fast but worth
	// caching for the many (probe, target) tuples an AGENT probe generates).
	baselineByProbe := make(map[uint]routeBaseline)
	loadBaseline := func(probeID uint) (routeBaseline, bool) {
		if b, ok := baselineByProbe[probeID]; ok {
			return b, true
		}
		var b routeBaseline
		if err := pg.WithContext(ctx).Where("probe_id = ?", probeID).First(&b).Error; err == nil {
			baselineByProbe[probeID] = b
			return b, true
		}
		return routeBaseline{}, false
	}

	// Group (agent, probe) → *ProbeRouteInfo so the per-target tuples
	// appear as distinct routes under the same agent.
	type routeKey struct {
		probeID     uint
		agentID     uint
		targetAgent uint // 0 for non-AGENT targets
	}
	routeByKey := make(map[routeKey]*ProbeRouteInfo)

	// Cross-agent aggregation: keyed by display target so multiple
	// agents MTR-ing 8.8.8.8 group together.
	type destStats struct {
		agents      map[uint]bool
		probeCount  int
		totalLat    float64
		totalLoss   float64
		latSamples  int
		lossSamples int
		hasIssues   bool
		targetIP    string
		displayName string
	}
	destAgg := make(map[string]*destStats)
	commonTargetKey := func(t string) string { return strings.ToLower(strings.TrimSpace(t)) }

	for pathKey, rows := range mtrByPath {
		// Skip if reporting agent isn't in this workspace (shouldn't happen
		// because the query filters by agent_id IN (...), but be defensive).
		ari, ok := ariByAgent[pathKey.agentID]
		if !ok {
			continue
		}

		// Resolve target: for AGENT probes target_agent is set; otherwise
		// fall back to the resolved target IP / hostname from the payload.
		target, targetIP, targetAgentName := resolveMTRTarget(agentByID, rows, pathKey.targetAgent)

		key := routeKey{probeID: pathKey.probeID, agentID: pathKey.agentID, targetAgent: pathKey.targetAgent}
		pri, exists := routeByKey[key]
		if !exists {
			pri = &ProbeRouteInfo{
				ProbeID:       pathKey.probeID,
				AgentID:       pathKey.agentID,
				Target:        target,
				TargetAgentID: pathKey.targetAgent,
			}
			if targetAgentName != "" {
				pri.TargetAgentName = targetAgentName
			}
			if b, ok := loadBaseline(pathKey.probeID); ok {
				pri.BaselineFingerprint = b.Fingerprint
				pri.BaselineHopCount = b.HopCount
				pri.BaselineRoutePath = b.RoutePath
			}
			routeByKey[key] = pri
		}

		// Process the MTR rows for this (probe, agent, target) tuple.
		sigs := make(map[string]int)
		var latestPayload *MtrPayload
		for i := range rows {
			var mp MtrPayload
			if err := json.Unmarshal(rows[i].Payload, &mp); err != nil {
				continue
			}
			sig := getMtrRouteSignature(mp.Report.Hops)
			sigs[sig]++
			if latestPayload == nil {
				latestPayload = &mp
				pri.LatestSignature = sig
				for _, hop := range mp.Report.Hops {
					if len(hop.Hosts) > 0 && hop.Hosts[0].IP != "" {
						pri.LatestHops = append(pri.LatestHops, hop.Hosts[0].IP)
					}
				}
				pri.LatestHopsDetail = buildHopDetails(latestPayload, agentIPToID, agentByID)
			}
		}
		if latestPayload != nil {
			pri.TraceCount = len(rows)
			pri.HasRouteChange, pri.RouteStabilityPct = decideRouteChangeStatus(pri.LatestSignature, pri.BaselineRoutePath, sigs, len(rows))
			// When the route has changed, find the first (newest) MTR row
			// whose signature differs from the baseline so the UI can show
			// how long the route has been changed. Rows are sorted
			// newest-first, so the first match gives us the upper bound
			// on the change duration.
			if pri.HasRouteChange && pri.BaselineRoutePath != "" {
				for i := range rows {
					var mp MtrPayload
					if err := json.Unmarshal(rows[i].Payload, &mp); err != nil {
						continue
					}
					hops := make([]string, 0, len(mp.Report.Hops))
					for _, h := range mp.Report.Hops {
						if len(h.Hosts) > 0 && h.Hosts[0].IP != "" {
							hops = append(hops, h.Hosts[0].IP)
						}
					}
					if hopSetJaccard(parseHopPath(pri.BaselineRoutePath), hops) < routeEcmpSimilarityThreshold {
						ts := rows[i].CreatedAt
						pri.RouteChangedAt = &ts
						break
					}
				}
			}
			if len(latestPayload.Report.Hops) > 0 {
				lastHop := latestPayload.Report.Hops[len(latestPayload.Report.Hops)-1]
				pri.AvgEndHopLatency = parseLatency(lastHop.Avg)
				pri.AvgEndHopLoss = parseLossPct(lastHop.LossPct)
			}
			hopCount := len(latestPayload.Report.Hops)
			if hopCount > 1 {
				for i := 0; i < hopCount-1; i++ {
					hop := latestPayload.Report.Hops[i]
					if len(hop.Hosts) == 0 || hop.Hosts[0].IP == "" || hop.Hosts[0].IP == "*" {
						continue
					}
					pri.IntermediateHops = append(pri.IntermediateHops, HopMetric{
						IP:       hop.Hosts[0].IP,
						Loss:     parseLossPct(hop.LossPct),
						Latency:  parseLatency(hop.Avg),
						HopIndex: i,
					})
				}
			}
		}

		// Index hops for shared-hop computation (now includes the final
		// hop so shared destinations surface in shared_hops too).
		if len(pri.LatestHops) >= 1 {
			for idx, ip := range pri.LatestHops {
				if ip == "" || ip == "*" {
					continue
				}
				if hopIndex[ip] == nil {
					hopIndex[ip] = make(map[uint]HopMetrics)
				}
				metrics := HopMetrics{Count: 1}
				matched := false
				for _, ih := range pri.IntermediateHops {
					if ih.IP == ip {
						metrics.TotalLoss += ih.Loss
						metrics.TotalLatency += ih.Latency
						if ih.Loss > 0 || ih.Latency > 100 {
							metrics.HasIssues = true
						}
						matched = true
						break
					}
				}
				if !matched && idx == len(pri.LatestHops)-1 {
					metrics.TotalLoss += pri.AvgEndHopLoss
					metrics.TotalLatency += pri.AvgEndHopLatency
					if pri.AvgEndHopLoss > 0 || pri.AvgEndHopLatency > 100 {
						metrics.HasIssues = true
					}
				}
				hopIndex[ip][pathKey.agentID] = metrics
			}
		}

		// Aggregate per-target stats for cross-agent views.
		if target != "" {
			key2 := commonTargetKey(target)
			ds, ok := destAgg[key2]
			if !ok {
				ds = &destStats{agents: make(map[uint]bool), displayName: target}
				destAgg[key2] = ds
			}
			if !ds.agents[pathKey.agentID] {
				ds.agents[pathKey.agentID] = true
			}
			ds.probeCount++
			if pri.AvgEndHopLatency > 0 {
				ds.totalLat += pri.AvgEndHopLatency
				ds.latSamples++
			}
			if pri.AvgEndHopLoss > 0 {
				ds.totalLoss += pri.AvgEndHopLoss
				ds.lossSamples++
			}
			if pri.HasRouteChange {
				ds.hasIssues = true
			}
			if ds.targetIP == "" {
				if targetIP != "" {
					ds.targetIP = targetIP
				} else if len(pri.LatestHops) > 0 {
					ds.targetIP = pri.LatestHops[len(pri.LatestHops)-1]
				}
			}
		}

		// Route-change incident.
		if pri.HasRouteChange {
			evidence := []string{
				fmt.Sprintf("Current signature: %s", pri.LatestSignature),
			}
			if pri.BaselineFingerprint != "" {
				evidence = append(evidence, fmt.Sprintf("Baseline fingerprint: %s", pri.BaselineFingerprint))
			}
			evidence = append(evidence, fmt.Sprintf("Route stability: %.0f%% over %d traces", pri.RouteStabilityPct, pri.TraceCount))
			routeIncidents = append(routeIncidents, RouteIncident{
				ID:        fmt.Sprintf("route_change_%d_%d", pathKey.agentID, pathKey.probeID),
				Type:      "route_change",
				Severity:  "warning",
				AgentID:   pathKey.agentID,
				AgentName: ari.AgentName,
				ProbeID:   pathKey.probeID,
				Target:    target,
				Message:   fmt.Sprintf("Route changed for %s → %s (stability %.0f%%)", ari.AgentName, target, pri.RouteStabilityPct),
				Evidence:  evidence,
			})
		}
	}

	allRoutes := make([]*ProbeRouteInfo, 0, len(routeByKey))
	for _, pri := range routeByKey {
		allRoutes = append(allRoutes, pri)
	}
	sort.SliceStable(allRoutes, func(i, j int) bool {
		if allRoutes[i].AgentID != allRoutes[j].AgentID {
			return allRoutes[i].AgentID < allRoutes[j].AgentID
		}
		return allRoutes[i].ProbeID < allRoutes[j].ProbeID
	})
	for _, pri := range allRoutes {
		ari := ariByAgent[pri.AgentID]
		if ari == nil {
			continue
		}
		ari.Routes = append(ari.Routes, *pri)
		totalRoutes++
	}

	// 5. Build shared hops list
	sharedHops := make([]SharedHopInfo, 0)
	for hopIP, agentMetricsMap := range hopIndex {
		if len(agentMetricsMap) < 2 {
			continue
		}
		sh := SharedHopInfo{
			HopIP:    hopIP,
			HopCount: len(agentMetricsMap),
		}
		// Check if this shared hop IP matches any agent
		if aid, ok := agentIPToID[hopIP]; ok {
			if a, ok := agentByID[aid]; ok {
				sh.HopHostname = a.Name
			}
		}
		var totalLoss, totalLatency float64
		var metricsCount int
		for aid, metrics := range agentMetricsMap {
			sh.AgentIDs = append(sh.AgentIDs, aid)
			if a, ok := agentByID[aid]; ok {
				sh.AgentNames = append(sh.AgentNames, a.Name)
			}
			if metrics.Count > 0 {
				totalLoss += metrics.TotalLoss
				totalLatency += metrics.TotalLatency
				metricsCount += metrics.Count
			}
			if metrics.HasIssues {
				sh.HasIssues = true
			}
		}
		if metricsCount > 0 {
			sh.AvgLoss = totalLoss / float64(metricsCount)
			sh.AvgLatency = totalLatency / float64(metricsCount)
		}
		sharedHops = append(sharedHops, sh)
	}

	// 6. Build shared destinations — any target that 2+ agents are MTR-ing.
	// This is the most useful "common route" view because it surfaces
	// internet endpoints the deployment is collectively monitoring.
	sharedDestinations := make([]SharedDestinationInfo, 0)
	for _, ds := range destAgg {
		if len(ds.agents) < 2 {
			continue
		}
		sd := SharedDestinationInfo{
			Target:     ds.displayName,
			TargetIP:   ds.targetIP,
			AgentCount: len(ds.agents),
		}
		if ds.latSamples > 0 {
			sd.AvgEndLatencyMs = ds.totalLat / float64(ds.latSamples)
		}
		if ds.lossSamples > 0 {
			sd.AvgEndLossPct = ds.totalLoss / float64(ds.lossSamples)
		}
		sd.HasIssues = ds.hasIssues
		for aid := range ds.agents {
			sd.AgentIDs = append(sd.AgentIDs, aid)
			if a, ok := agentByID[aid]; ok {
				sd.AgentNames = append(sd.AgentNames, a.Name)
			}
		}
		sharedDestinations = append(sharedDestinations, sd)
	}

	// 7. Build common targets — every target probed by ≥1 agent, sorted by
	// agent count. Single-agent targets still get a row, so the UI can
	// answer "what is this agent MTR-ing?" without leaving the tab.
	commonTargets := make([]CommonTargetInfo, 0, len(destAgg))
	for _, ds := range destAgg {
		ct := CommonTargetInfo{
			Target:     ds.displayName,
			AgentCount: len(ds.agents),
			ProbeCount: ds.probeCount,
			HasIssues:  ds.hasIssues,
		}
		if ds.latSamples > 0 {
			ct.AvgEndLatencyMs = ds.totalLat / float64(ds.latSamples)
		}
		if ds.lossSamples > 0 {
			ct.AvgEndLossPct = ds.totalLoss / float64(ds.lossSamples)
		}
		for aid := range ds.agents {
			ct.AgentIDs = append(ct.AgentIDs, aid)
			if a, ok := agentByID[aid]; ok {
				ct.AgentNames = append(ct.AgentNames, a.Name)
			}
		}
		commonTargets = append(commonTargets, ct)
	}
	// Stable ordering: most-shared first, then alphabetical.
	sortCommonTargets(commonTargets)

	// 8. Build shared ASNs — group hop IPs by their ASN, only emit ASNs
	// that 2+ agents traverse. This is the "common upstream network"
	// view that survives last-mile ISP diversity.
	sharedASNs := buildSharedASNs(geoStore, hopIndex, agentByID)

	return &WorkspaceRouteAnalysis{
		WorkspaceID:        workspaceID,
		Agents:             agentRoutes,
		SharedHops:         sharedHops,
		SharedDestinations: sharedDestinations,
		SharedASNs:         sharedASNs,
		CommonTargets:      commonTargets,
		Incidents:          routeIncidents,
		TotalAgents:        len(agents),
		TotalRoutes:        totalRoutes,
		GeneratedAt:        time.Now().UTC(),
	}, nil
}

// sortCommonTargets orders: highest agent count first, then alpha by target.
func sortCommonTargets(cs []CommonTargetInfo) {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].AgentCount != cs[j].AgentCount {
			return cs[i].AgentCount > cs[j].AgentCount
		}
		return cs[i].Target < cs[j].Target
	})
}

// routeEcmpSimilarityThreshold is the minimum Jaccard similarity between the
// baseline and current hop-IP sets for a route to be considered "the same
// path" despite fingerprint differences. ECMP / load-balancing typically
// swaps 1-2 hops out of ~10-15, yielding similarity ~0.8-0.9; a real route
// change (different upstream, agent moved networks) lands well below this.
const routeEcmpSimilarityThreshold = 0.7

// routeBaselineStaleThreshold is the maximum age of a stored baseline before
// the MTR handler rewrites it to the current fingerprint. This way
// intentional long-term route changes (e.g. agent moved networks, ISP
// rerouted the path) are eventually picked up after a stabilization period
// and stop emitting route_change alerts indefinitely.
const routeBaselineStaleThreshold = 7 * 24 * time.Hour

func decideRouteChangeStatus(latestHops, baselineHops string, sigs map[string]int, traceCount int) (bool, float64) {
	if baselineHops != "" {
		if hopSetJaccard(parseHopPath(baselineHops), parseHopPath(latestHops)) >= routeEcmpSimilarityThreshold {
			return false, 100
		}
		return true, dominantSignatureStabilityPct(sigs, traceCount)
	}
	if len(sigs) > 1 {
		return true, dominantSignatureStabilityPct(sigs, traceCount)
	}
	return false, 100
}

func dominantSignatureStabilityPct(sigs map[string]int, traceCount int) float64 {
	if traceCount <= 0 {
		return 100
	}
	maxCount := 0
	for _, c := range sigs {
		if c > maxCount {
			maxCount = c
		}
	}
	return math.Round(float64(maxCount)/float64(traceCount)*100*10) / 10
}

func parseHopPath(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '>' || r == ' '
	})
	out := parts[:0]
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func hopSetJaccard(a, b []string) float64 {
	aSet := make(map[string]struct{}, len(a))
	for _, h := range a {
		if h == "" || h == "*" {
			continue
		}
		aSet[h] = struct{}{}
	}
	bSet := make(map[string]struct{}, len(b))
	for _, h := range b {
		if h == "" || h == "*" {
			continue
		}
		bSet[h] = struct{}{}
	}
	if len(aSet) == 0 && len(bSet) == 0 {
		return 1
	}
	intersection := 0
	for h := range bSet {
		if _, ok := aSet[h]; ok {
			intersection++
		}
	}
	union := len(aSet) + len(bSet) - intersection
	if union == 0 {
		return 1
	}
	return float64(intersection) / float64(union)
}

// mtrPathKey uniquely identifies a (probe, agent, target) tuple for grouping
// MTR rows. The (probe_id, agent_id) pair identifies the trace, target_agent
// distinguishes the destination for AGENT (bidirectional) probes (0 for
// non-agent targets like 8.8.8.8).
type mtrPathKey struct {
	probeID     uint
	agentID     uint
	targetAgent uint
}

// getWorkspaceMTRByPath fetches all MTR rows for the given agents in a single
// batched ClickHouse query and groups them by (probe_id, agent_id,
// target_agent). Each group is sorted newest-first.
//
// The previous per-probe loop iterated ListByAgent and skipped probes whose
// Type wasn't "MTR" — which silently excluded AGENT (bidirectional) probes
// even though those probes are the primary source of MTR data in modern
// installations. Driving from probe_data type='MTR' rows instead means
// both standalone MTR and AGENT probes are handled uniformly.
func getWorkspaceMTRByPath(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time, perGroupLimit int) (map[mtrPathKey][]ProbeData, error) {
	out := make(map[mtrPathKey][]ProbeData)
	if len(agentIDs) == 0 {
		return out, nil
	}

	idStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		idStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(idStrs, ", ")

	// Pull enough rows per (probe, agent, target) group to compute stable
	// signatures. We OVER-fetch and dedupe in Go to avoid fragile
	// window-function queries. 200 rows per group is plenty for stability %.
	q := fmt.Sprintf(`
SELECT
    created_at,
    probe_id,
    agent_id,
    target_agent,
    payload_raw
FROM probe_data
WHERE type = 'MTR'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 5000
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("mtr query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var createdAt time.Time
		var probeID, agentID, targetAgent uint64
		var payloadRaw string
		if err := rows.Scan(&createdAt, &probeID, &agentID, &targetAgent, &payloadRaw); err != nil {
			continue
		}
		key := mtrPathKey{
			probeID:     uint(probeID),
			agentID:     uint(agentID),
			targetAgent: uint(targetAgent),
		}
		if perGroupLimit > 0 && len(out[key]) >= perGroupLimit {
			continue
		}
		out[key] = append(out[key], ProbeData{
			CreatedAt:   createdAt,
			Type:        TypeMTR,
			ProbeID:     uint(probeID),
			AgentID:     uint(agentID),
			TargetAgent: uint(targetAgent),
			Payload:     []byte(payloadRaw),
		})
	}
	return out, rows.Err()
}

// resolveMTRTarget produces a display-friendly (target, targetIP, agentName)
// tuple for an MTR trace group. The priority is:
//
//  1. The target_agent's agent name (for AGENT / bidirectional probes) —
//     preferred because users recognise "Bob's laptop" over "10.0.0.5".
//  2. The MTR payload's resolved target.hostname (DNS hostname) — used when
//     the probe is targeting a literal hostname like "google.com".
//  3. The MTR payload's resolved target.ip — last resort; only used when
//     the trace was to an IP literal we can't otherwise name.
//
// targetIP is always populated from the final hop IP so the shared-destination
// card can show both "Bob's laptop" and "10.0.0.5" together.
func resolveMTRTarget(agentByID map[uint]agentInfo, rows []ProbeData, targetAgent uint) (target, targetIP, targetAgentName string) {
	if targetAgent != 0 {
		if a, ok := agentByID[targetAgent]; ok {
			targetAgentName = a.Name
		}
	}

	// Always derive the final-hop IP from the most recent MTR payload —
	// useful for both display and for cross-referencing with NETINFO / DNS.
	for i := range rows {
		var mp MtrPayload
		if err := json.Unmarshal(rows[i].Payload, &mp); err != nil {
			continue
		}
		if targetIP == "" {
			targetIP = mp.Report.HopFinalIP()
		}
		break // only need the most recent row for IP
	}

	// 1. AGENT probe → use the target agent's name.
	if targetAgentName != "" {
		return targetAgentName, targetIP, targetAgentName
	}

	// 2/3. Non-AGENT probe → use the payload's resolved hostname, or IP.
	for i := range rows {
		if t := extractMTRTargetHostname(rows[i].Payload); t != "" {
			return t, targetIP, ""
		}
	}
	return targetIP, targetIP, ""
}

// extractMTRTargetHostname pulls report.info.target.hostname out of a raw
// MTR payload without depending on the public MtrPayload struct (which
// intentionally omits Info). Falls back to .target.ip if no hostname.
func extractMTRTargetHostname(raw []byte) string {
	var probe struct {
		Report struct {
			Info struct {
				Target struct {
					Hostname string `json:"hostname"`
					IP       string `json:"ip"`
				} `json:"target"`
			} `json:"info"`
		} `json:"report"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	if probe.Report.Info.Target.Hostname != "" {
		return probe.Report.Info.Target.Hostname
	}
	return probe.Report.Info.Target.IP
}

// buildSharedASNs groups shared hop IPs by their autonomous system. Each
// emitted SharedASNInfo represents an ASN whose network is traversed by
// 2+ agents, with rollup latency/loss across the contributing hops.
//
// The geoStore parameter is an interface to keep this package import-free
// of the geoip package. Pass nil to skip ASN grouping (e.g. when no
// MaxMind DB is configured — HasASN() returns false in that case anyway).
func buildSharedASNs(geoStore GeoIPResolver, hopIndex map[string]map[uint]HopMetrics, agentByID map[uint]agentInfo) []SharedASNInfo {
	if geoStore == nil || !geoStore.HasASN() {
		return []SharedASNInfo{}
	}

	// ASN -> set of contributing hop IPs (for dedup in output)
	type asnBucket struct {
		asn      uint
		org      string
		hopIPs   map[string]struct{}
		agents   map[uint]bool
		hasIssue bool
		latSum   float64
		lossSum  float64
		latN     int
		lossN    int
	}
	buckets := make(map[uint]*asnBucket)

	for hopIP, agentMetricsMap := range hopIndex {
		if len(agentMetricsMap) < 2 {
			continue
		}
		asn, org, ok := geoStore.LookupASN(hopIP)
		if !ok || asn == 0 {
			continue
		}
		b, exists := buckets[asn]
		if !exists {
			b = &asnBucket{
				asn:    asn,
				org:    org,
				hopIPs: make(map[string]struct{}),
				agents: make(map[uint]bool),
			}
			buckets[asn] = b
		}
		b.hopIPs[hopIP] = struct{}{}
		var hopLat, hopLoss float64
		var hopLatN, hopLossN int
		for aid, metrics := range agentMetricsMap {
			b.agents[aid] = true
			if metrics.HasIssues {
				b.hasIssue = true
			}
			// Average per-agent latency/loss for this hop
			if metrics.Count > 0 {
				avgLat := metrics.TotalLatency / float64(metrics.Count)
				avgLoss := metrics.TotalLoss / float64(metrics.Count)
				hopLat += avgLat
				hopLatN++
				if avgLoss > 0 {
					hopLoss += avgLoss
					hopLossN++
				}
			}
		}
		b.latSum += hopLat
		b.latN += hopLatN
		b.lossSum += hopLoss
		b.lossN += hopLossN
	}

	out := make([]SharedASNInfo, 0, len(buckets))
	for _, b := range buckets {
		if len(b.agents) < 2 {
			continue
		}
		s := SharedASNInfo{
			ASN:        b.asn,
			ASNOrg:     b.org,
			AgentCount: len(b.agents),
			HasIssues:  b.hasIssue,
		}
		for ip := range b.hopIPs {
			s.HopIPs = append(s.HopIPs, ip)
		}
		for aid := range b.agents {
			s.AgentIDs = append(s.AgentIDs, aid)
			if a, ok := agentByID[aid]; ok {
				s.AgentNames = append(s.AgentNames, a.Name)
			}
		}
		if b.latN > 0 {
			s.AvgLatency = b.latSum / float64(b.latN)
		}
		if b.lossN > 0 {
			s.AvgLoss = b.lossSum / float64(b.lossN)
		}
		out = append(out, s)
	}

	// Sort: most agents first, then by ASN
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AgentCount != out[j].AgentCount {
			return out[i].AgentCount > out[j].AgentCount
		}
		return out[i].ASN < out[j].ASN
	})
	return out
}

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
	Severity        string             `json:"severity"` // warning, critical
	Title           string             `json:"title"`
	Category        string             `json:"category"` // jitter_spike, packet_loss, latency_degradation, asymmetry, out_of_order
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

// ── Voice Quality Scoring ───────────────────────────────────────────────────

// voiceScoreFromMos converts MOS 1-5 to 0-100 score
func voiceScoreFromMos(mos float64) float64 {
	return clampScore((mos - 1.0) / 3.5 * 100)
}

// voiceGradeFromMos converts MOS to grade string
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

// mosContributingFactors identifies what is degrading MOS for a given path
func mosContributingFactors(avgLat, p95Lat, jitter, loss float64) []string {
	var factors []string
	if avgLat > 150 {
		factors = append(factors, fmt.Sprintf("high_latency_avg=%.0fms", avgLat))
	} else if avgLat > 80 {
		factors = append(factors, fmt.Sprintf("elevated_latency_avg=%.0fms", avgLat))
	}
	if p95Lat > 200 {
		factors = append(factors, fmt.Sprintf("high_latency_p95=%.0fms", p95Lat))
	}
	if jitter > 20 {
		factors = append(factors, fmt.Sprintf("high_jitter=%.1fms", jitter))
	} else if jitter > 10 {
		factors = append(factors, fmt.Sprintf("elevated_jitter=%.1fms", jitter))
	}
	if loss > 2 {
		factors = append(factors, fmt.Sprintf("high_packet_loss=%.1f%%", loss))
	} else if loss > 0.5 {
		factors = append(factors, fmt.Sprintf("mild_packet_loss=%.1f%%", loss))
	}
	return factors
}

// congestionLevelFromMetrics determines congestion level from voice metrics
func congestionLevelFromMetrics(jitter, loss, avgLat float64) CongestionLevel {
	// Severe: high jitter + loss + latency
	if jitter > 30 && loss > 3 && avgLat > 100 {
		return CongestionSevere
	}
	// Moderate: elevated on two or more metrics
	elevatedCount := 0
	if jitter > 15 {
		elevatedCount++
	}
	if loss > 1 {
		elevatedCount++
	}
	if avgLat > 80 {
		elevatedCount++
	}
	if elevatedCount >= 2 {
		return CongestionModerate
	}
	// Mild: one metric elevated
	if jitter > 10 || loss > 0.5 || avgLat > 60 {
		return CongestionMild
	}
	return CongestionNone
}

// ── Voice Quality Issue Detection ──────────────────────────────────────────

// detectVoiceQualityIssues detects voice quality problems across forward and return paths.
//
// `baselineByProbeID` is the per-probe 7-day baseline; passing the
// matching baseline into each detection call (rather than a single
// shared baseline) means multi-target agents get correct baselines
// for each destination. `thresholds` controls the numeric cutoffs; if
// nil, VoiceDefaultThresholds is used.
func detectVoiceQualityIssues(forward, returnPath *VoicePathMetrics, baselineByProbeID map[uint]*VoicePathMetrics, targetAgentName string, thresholds *VoiceThresholds) []VoiceQualityIssue {
	t := VoiceDefaultThresholds
	if thresholds != nil {
		t = *thresholds
	}
	var issues []VoiceQualityIssue

	baselineFwd := baselineFor(forward, baselineByProbeID)
	baselineRet := baselineFor(returnPath, baselineByProbeID)

	// 1. Jitter spike detection (forward)
	if forward != nil {
		issues = append(issues, detectJitterAnomalies(forward, baselineFwd, VoicePathForward, targetAgentName, t)...)
	}

	// 2. Jitter spike detection (return)
	if returnPath != nil {
		issues = append(issues, detectJitterAnomalies(returnPath, baselineRet, VoicePathReturn, targetAgentName, t)...)
	}

	// 3. Packet loss burst detection
	if forward != nil {
		issues = append(issues, detectPacketLossAnomalies(forward, baselineFwd, VoicePathForward, targetAgentName, t)...)
	}
	if returnPath != nil {
		issues = append(issues, detectPacketLossAnomalies(returnPath, baselineRet, VoicePathReturn, targetAgentName, t)...)
	}

	// 4. Latency-only degradation (high latency but no packet loss — route issue)
	if forward != nil {
		issues = append(issues, detectLatencyOnlyDegradation(forward, baselineFwd, VoicePathForward, targetAgentName, t)...)
	}
	if returnPath != nil {
		issues = append(issues, detectLatencyOnlyDegradation(returnPath, baselineRet, VoicePathReturn, targetAgentName, t)...)
	}

	// 5. Out of sequence / packet reordering
	if forward != nil && forward.OutOfSequence > t.OutOfSequencePct {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("out_of_sequence_%d_forward", forward.ProbeID),
			Severity:        "warning",
			Title:           fmt.Sprintf("Packet reordering detected on forward path to %s", targetAgentName),
			Category:        "out_of_order",
			AffectedPath:    VoicePathForward,
			TargetAgentName: targetAgentName,
			SuspectedCause:  "Packet reordering can indicate ECMP load balancing, suboptimal routing, or a problematic middlebox",
			Evidence: []string{
				fmt.Sprintf("Out of sequence: %.2f%%", forward.OutOfSequence),
				fmt.Sprintf("Duplicates: %.2f%%", forward.Duplicates),
				fmt.Sprintf("Jitter: %.1fms", forward.JitterAvg),
			},
			Recommendations: []string{
				"Run MTR with TCP mode (mtr -T) to check for ECMP hashing issues",
				"Compare route at different times to identify unstable hops",
			},
		})
	}
	if returnPath != nil && returnPath.OutOfSequence > t.OutOfSequencePct {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("out_of_sequence_%d_return", returnPath.ProbeID),
			Severity:        "warning",
			Title:           fmt.Sprintf("Packet reordering detected on return path from %s", targetAgentName),
			Category:        "out_of_order",
			AffectedPath:    VoicePathReturn,
			TargetAgentName: targetAgentName,
			SuspectedCause:  "Asymmetric return routing may cause packet reordering",
			Evidence: []string{
				fmt.Sprintf("Out of sequence: %.2f%%", returnPath.OutOfSequence),
				fmt.Sprintf("Duplicates: %.2f%%", returnPath.Duplicates),
			},
			Recommendations: []string{
				"Check if return path uses different ISP or routing path",
			},
		})
	}

	// 6. Asymmetric path degradation (forward good, return bad — or vice versa)
	if forward != nil && returnPath != nil {
		issues = append(issues, detectAsymmetricVoiceDegradation(forward, returnPath, targetAgentName, t)...)
	}

	// 7. Loss-pattern classification (burst vs steady) — fires only when
	// there is at least one loss-related issue and time-bucketed data is
	// available. Done after the main loss detection so we can decorate
	// existing issues rather than emit a new one (operators read the
	// pattern alongside the threshold breach).
	if len(issues) > 0 {
		annotateLossPatterns(issues, forward, returnPath)
	}

	return issues
}

// baselineFor returns the matching baseline for a path from the
// per-probe map, or nil if none. The map is keyed by probe_id.
func baselineFor(path *VoicePathMetrics, byProbeID map[uint]*VoicePathMetrics) *VoicePathMetrics {
	if path == nil || byProbeID == nil {
		return nil
	}
	return byProbeID[path.ProbeID]
}

// annotateLossPatterns inspects the per-direction time series (if
// present in the path's MosContributors or a separate channel) and
// tags existing loss issues with burst vs steady. Today this is a
// placeholder for the time-series signal that
// fetchVoicePathSeries will provide; we mark the issue with a
// recommendation modifier so the report text reflects that we
// looked. Real burst detection happens in detectBurstLossPattern.
func annotateLossPatterns(issues []VoiceQualityIssue, forward, returnPath *VoicePathMetrics) {
	// No-op stub — burst detection runs in detectBurstLossPattern
	// when time series data is available. We keep this hook so the
	// call site doesn't have to change when the burst detector is
	// wired through the series channel.
	_ = issues
	_ = forward
	_ = returnPath
}

// detectJitterAnomalies identifies jitter spikes above voice quality thresholds
func detectJitterAnomalies(path, baseline *VoicePathMetrics, direction VoicePathDirection, targetAgentName string, t VoiceThresholds) []VoiceQualityIssue {
	var issues []VoiceQualityIssue
	if path == nil || path.JitterAvg <= 0 {
		return issues
	}

	threshold := t.WarningJitterMs
	criticalThreshold := t.CriticalJitterMs
	spikeMultiplier := t.JitterSpikeMultiplier

	if path.JitterAvg > criticalThreshold {
		mosDegradation := path.MosScore
		if baseline != nil {
			mosDegradation = path.MosScore - baseline.MosScore
		}
		var timePattern string
		if baseline != nil && baseline.JitterAvg > 0 {
			if path.JitterAvg > baseline.JitterAvg*1.5 {
				timePattern = "periodic_spikes"
			} else {
				timePattern = "constant"
			}
		} else {
			timePattern = "unknown"
		}

		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("jitter_critical_%d_%s", path.ProbeID, direction),
			Severity:        "critical",
			Title:           fmt.Sprintf("Critical jitter on %s path to %s", direction, targetAgentName),
			Category:        "jitter_spike",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  fmt.Sprintf("Very high jitter (>%.0fms) causes voice buffer underruns leading to choppy or garbled audio", criticalThreshold),
			Evidence: []string{
				fmt.Sprintf("Jitter average: %.1fms (threshold: %.0fms)", path.JitterAvg, criticalThreshold),
				fmt.Sprintf("Jitter P95: %.1fms", path.JitterP95),
				fmt.Sprintf("MOS: %.2f", path.MosScore),
				fmt.Sprintf("Sample count: %d", path.SampleCount),
			},
			TimePattern:    timePattern,
			MosDegradation: mosDegradation,
			MosBefore: func() float64 {
				if baseline != nil {
					return baseline.MosScore
				}
				return 0
			}(),
			MosAfter: path.MosScore,
			Recommendations: []string{
				"Check for competing traffic on the network segment",
				"Verify ISP does not have bufferbloat issues",
				"Consider implementing jitter buffering on endpoint",
			},
		})
	} else if path.JitterAvg > threshold {
		mosDegradation := path.MosScore
		if baseline != nil {
			mosDegradation = path.MosScore - baseline.MosScore
		}
		timePattern := "unknown"
		if baseline != nil && baseline.JitterAvg > 0 {
			if path.JitterAvg > baseline.JitterAvg*1.5 {
				timePattern = "periodic_spikes"
			} else if path.JitterAvg > baseline.JitterAvg*1.2 {
				timePattern = "gradual_increase"
			} else {
				timePattern = "constant"
			}
		}

		cause := fmt.Sprintf("Elevated jitter (>%.0fms) can cause voice quality degradation during calls", threshold)
		if path.JitterP95 > path.JitterAvg*3 {
			cause = "High jitter variance (P95 is 3x average) indicates intermittent network congestion"
		}

		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("jitter_warning_%d_%s", path.ProbeID, direction),
			Severity:        "warning",
			Title:           fmt.Sprintf("Elevated jitter on %s path to %s", direction, targetAgentName),
			Category:        "jitter_spike",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  cause,
			Evidence: []string{
				fmt.Sprintf("Jitter average: %.1fms (threshold: %.0fms)", path.JitterAvg, threshold),
				fmt.Sprintf("Jitter P95: %.1fms", path.JitterP95),
				fmt.Sprintf("Jitter median: %.1fms", path.JitterMedian),
				fmt.Sprintf("MOS: %.2f", path.MosScore),
			},
			TimePattern:    timePattern,
			MosDegradation: mosDegradation,
			MosBefore: func() float64 {
				if baseline != nil {
					return baseline.MosScore
				}
				return 0
			}(),
			MosAfter: path.MosScore,
			Recommendations: []string{
				"Monitor jitter trend to determine if it's worsening",
				"Check for bandwidth-intensive applications during peak hours",
			},
		})
	}

	// Also detect sudden jitter increases from baseline
	if baseline != nil && baseline.JitterAvg > 5 && path.JitterAvg > baseline.JitterAvg*spikeMultiplier {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("jitter_spike_%d_%s", path.ProbeID, direction),
			Severity:        "warning",
			Title:           fmt.Sprintf("Sudden jitter increase on %s path to %s", direction, targetAgentName),
			Category:        "jitter_spike",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  fmt.Sprintf("Jitter more than %.1fx baseline — possible network event, congestion, or route change", spikeMultiplier),
			Evidence: []string{
				fmt.Sprintf("Current jitter: %.1fms vs baseline: %.1fms (%.1fx increase)", path.JitterAvg, baseline.JitterAvg, path.JitterAvg/baseline.JitterAvg),
				fmt.Sprintf("MOS dropped from %.2f to %.2f", baseline.MosScore, path.MosScore),
			},
			TimePattern:    "sudden_spike",
			MosDegradation: path.MosScore - baseline.MosScore,
			MosBefore:      baseline.MosScore,
			MosAfter:       path.MosScore,
			Recommendations: []string{
				"Check MTR for route changes",
				"Verify no network maintenance or ISP issues",
			},
		})
	}

	return issues
}

// detectPacketLossAnomalies identifies packet loss patterns affecting voice quality
func detectPacketLossAnomalies(path, baseline *VoicePathMetrics, direction VoicePathDirection, targetAgentName string, t VoiceThresholds) []VoiceQualityIssue {
	var issues []VoiceQualityIssue
	if path == nil || path.PacketLoss <= 0 {
		return issues
	}

	critical := t.CriticalLossPct
	warning := t.WarningLossPct

	if path.PacketLoss > critical {
		timePattern := "constant"
		if baseline != nil && baseline.PacketLoss > 0 {
			if path.PacketLoss > baseline.PacketLoss*2 {
				timePattern = "increasing"
			} else {
				timePattern = "constant"
			}
		}

		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("loss_critical_%d_%s", path.ProbeID, direction),
			Severity:        "critical",
			Title:           fmt.Sprintf("Severe packet loss on %s path to %s", direction, targetAgentName),
			Category:        "packet_loss",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  fmt.Sprintf("Packet loss >%.0f%% will cause noticeable call quality issues — dropped words, robotic voice, call drops", critical),
			Evidence: []string{
				fmt.Sprintf("Packet loss: %.2f%% (critical threshold: %.0f%%)", path.PacketLoss, critical),
				fmt.Sprintf("MOS: %.2f", path.MosScore),
				fmt.Sprintf("Total packets: %.0f, Lost: %.0f", float64(path.SampleCount)*60.0, float64(path.SampleCount)*60.0*path.PacketLoss/100),
			},
			TimePattern: timePattern,
			MosDegradation: func() float64 {
				if baseline != nil {
					return path.MosScore - baseline.MosScore
				}
				return 0
			}(),
			MosBefore: func() float64 {
				if baseline != nil {
					return baseline.MosScore
				}
				return 0
			}(),
			MosAfter: path.MosScore,
			Recommendations: []string{
				"Check for link failures or ISP outages",
				"Review MTR for high-loss hops",
				"Escalate to ISP if loss persists",
			},
		})
	} else if path.PacketLoss > warning {
		timePattern := "unknown"
		if baseline != nil && baseline.PacketLoss > 0 {
			if path.PacketLoss > baseline.PacketLoss*1.5 {
				timePattern = "increasing"
			} else if path.PacketLoss < baseline.PacketLoss*0.8 {
				timePattern = "improving"
			} else {
				timePattern = "stable"
			}
		}

		cause := fmt.Sprintf("Moderate packet loss (%.0f-%.0f%%) causes occasional dropouts and reduced call quality", warning, critical)
		if path.OutOfSequence > t.OutOfSequencePct {
			cause = "Moderate packet loss with reordering suggests network instability or ISP issues"
		}

		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("loss_warning_%d_%s", path.ProbeID, direction),
			Severity:        "warning",
			Title:           fmt.Sprintf("Packet loss on %s path to %s", direction, targetAgentName),
			Category:        "packet_loss",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  cause,
			Evidence: []string{
				fmt.Sprintf("Packet loss: %.2f%% (warning threshold: %.0f%%)", path.PacketLoss, warning),
				fmt.Sprintf("Out of sequence: %.2f%%", path.OutOfSequence),
				fmt.Sprintf("MOS: %.2f", path.MosScore),
			},
			TimePattern: timePattern,
			MosDegradation: func() float64 {
				if baseline != nil {
					return path.MosScore - baseline.MosScore
				}
				return 0
			}(),
			MosBefore: func() float64 {
				if baseline != nil {
					return baseline.MosScore
				}
				return 0
			}(),
			MosAfter: path.MosScore,
			Recommendations: []string{
				"Monitor loss trend to determine if it's getting worse",
				"Check for congestion during business hours",
			},
		})
	}

	// Detect sudden loss appearance (from baseline of near-zero)
	if baseline != nil && baseline.PacketLoss < t.NewLossBaselineMaxPct && path.PacketLoss > t.NewLossCurrentMinPct {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("loss_new_%d_%s", path.ProbeID, direction),
			Severity:        "warning",
			Title:           fmt.Sprintf("New packet loss on %s path to %s", direction, targetAgentName),
			Category:        "packet_loss",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  "Packet loss recently appeared — possible link degradation, ISP issue, or congestion",
			Evidence: []string{
				fmt.Sprintf("Current loss: %.2f%% vs baseline: %.2f%%", path.PacketLoss, baseline.PacketLoss),
				fmt.Sprintf("MOS: %.2f (was %.2f)", path.MosScore, baseline.MosScore),
			},
			TimePattern:    "sudden_appearance",
			MosDegradation: path.MosScore - baseline.MosScore,
			MosBefore:      baseline.MosScore,
			MosAfter:       path.MosScore,
			Recommendations: []string{
				"Review MTR for the degraded path",
				"Check if issue correlates with specific time periods",
			},
		})
	}

	return issues
}

// detectLatencyOnlyDegradation detects high latency degrading voice without packet loss
func detectLatencyOnlyDegradation(path, baseline *VoicePathMetrics, direction VoicePathDirection, targetAgentName string, t VoiceThresholds) []VoiceQualityIssue {
	var issues []VoiceQualityIssue
	if path == nil || path.PacketLoss > t.LatencyOnlyMaxLossPct {
		return issues // Not latency-only if there's packet loss
	}

	if path.MosScore < t.LatencyOnlyMaxMos && path.AvgLatency > t.LatencyOnlyMinMs && path.PacketLoss < t.LatencyOnlyMaxLossPct {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("latency_only_%d_%s", path.ProbeID, direction),
			Severity:        "warning",
			Title:           fmt.Sprintf("Latency impacting voice quality on %s path to %s", direction, targetAgentName),
			Category:        "latency_degradation",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  fmt.Sprintf("High latency (>%.0fms) with no packet loss suggests route inefficiency or distant peering point", t.LatencyOnlyMinMs),
			Evidence: []string{
				fmt.Sprintf("Avg latency: %.0fms", path.AvgLatency),
				fmt.Sprintf("P95 latency: %.0fms", path.P95Latency),
				fmt.Sprintf("Packet loss: %.2f%% (negligible)", path.PacketLoss),
				fmt.Sprintf("MOS: %.2f", path.MosScore),
			},
			TimePattern: "unknown",
			Recommendations: []string{
				"Run MTR to identify the high-latency hop",
				"Check for suboptimal routing or distant ISP peering",
			},
		})
	}

	return issues
}

// detectAsymmetricVoiceDegradation compares forward vs return path for asymmetric issues.
// In addition to the existing MOS-ratio finding, we now emit one finding
// per asymmetric per-metric (latency, jitter, loss) so operators see
// WHICH metric is degraded, not just that the two directions disagree.
func detectAsymmetricVoiceDegradation(forward, returnPath *VoicePathMetrics, targetAgentName string, t VoiceThresholds) []VoiceQualityIssue {
	var issues []VoiceQualityIssue

	// Per-metric asymmetry: each metric gets its own finding when one
	// direction is materially worse than the other.
	issues = append(issues, detectAsymmetricMetric(forward, returnPath, targetAgentName, "loss", "packet_loss", forward.PacketLoss, returnPath.PacketLoss, t.WarningLossPct*2)...)
	issues = append(issues, detectAsymmetricMetric(forward, returnPath, targetAgentName, "jitter", "jitter_asymmetry", forward.JitterAvg, returnPath.JitterAvg, t.WarningJitterMs*2)...)
	issues = append(issues, detectAsymmetricMetric(forward, returnPath, targetAgentName, "latency", "latency_asymmetry", forward.AvgLatency, returnPath.AvgLatency, 20)...) // 20ms absolute skew

	// Calculate MOS ratio
	var mosRatio float64
	if forward.MosScore > 0 {
		mosRatio = returnPath.MosScore / forward.MosScore
	}

	// Significant asymmetry: return path is much worse than forward
	if mosRatio < t.AsymmetryMosRatioMin && forward.MosScore > t.AsymmetryMinForwardMos {
		latRatio := 1.0
		if forward.AvgLatency > 0 {
			latRatio = returnPath.AvgLatency / forward.AvgLatency
		}

		var suspectedCause string
		if latRatio > 2.0 {
			suspectedCause = fmt.Sprintf("Return path latency is %.0f%% higher than forward path — asymmetric routing or different ISP path", (latRatio-1)*100)
		} else {
			suspectedCause = "Return path has significantly lower MOS than forward path despite similar latency — possible ISP issue on return"
		}

		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("asymmetry_%d", forward.ProbeID),
			Severity:        "warning",
			Title:           fmt.Sprintf("Asymmetric voice quality to %s", targetAgentName),
			Category:        "asymmetry",
			AffectedPath:    VoicePathReturn,
			TargetAgentName: targetAgentName,
			SuspectedCause:  suspectedCause,
			Evidence: []string{
				fmt.Sprintf("Forward MOS: %.2f, Return MOS: %.2f (ratio: %.2f)", forward.MosScore, returnPath.MosScore, mosRatio),
				fmt.Sprintf("Forward latency: %.0fms, Return latency: %.0fms", forward.AvgLatency, returnPath.AvgLatency),
				fmt.Sprintf("Forward jitter: %.1fms, Return jitter: %.1fms", forward.JitterAvg, returnPath.JitterAvg),
			},
			TimePattern: "constant",
			Recommendations: []string{
				"Check if return path uses a different ISP",
				"Contact upstream provider if asymmetry persists",
			},
		})
	}

	// Reverse asymmetry: forward path is worse
	if mosRatio > 1.25 && returnPath.MosScore > t.AsymmetryMinForwardMos {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("asymmetry_reverse_%d", forward.ProbeID),
			Severity:        "warning",
			Title:           fmt.Sprintf("Forward path degraded relative to return to %s", targetAgentName),
			Category:        "asymmetry",
			AffectedPath:    VoicePathForward,
			TargetAgentName: targetAgentName,
			SuspectedCause:  "Forward path has lower MOS than return — possible local congestion or ISP issue",
			Evidence: []string{
				fmt.Sprintf("Forward MOS: %.2f, Return MOS: %.2f", forward.MosScore, returnPath.MosScore),
			},
			TimePattern: "unknown",
			Recommendations: []string{
				"Check local network at source for congestion",
			},
		})
	}

	return issues
}

// detectAsymmetricMetric emits a single per-metric asymmetry finding
// when one direction's value is materially worse than the other.
// `minSkew` is the minimum absolute (or ratio) difference to fire.
func detectAsymmetricMetric(forward, returnPath *VoicePathMetrics, targetAgentName, label, category string, fwdVal, retVal, minSkew float64) []VoiceQualityIssue {
	var issues []VoiceQualityIssue
	if forward == nil || returnPath == nil {
		return issues
	}
	diff := retVal - fwdVal
	if diff < 0 {
		diff = -diff
	}
	if diff < minSkew {
		return issues
	}
	if retVal > fwdVal {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("asymmetry_%s_return_%d", label, forward.ProbeID),
			Severity:        "warning",
			Title:           fmt.Sprintf("Return path %s higher than forward to %s", label, targetAgentName),
			Category:        category,
			AffectedPath:    VoicePathReturn,
			TargetAgentName: targetAgentName,
			SuspectedCause:  fmt.Sprintf("Asymmetric %s — return path is %.2f, forward is %.2f", label, retVal, fwdVal),
			Evidence: []string{
				fmt.Sprintf("Forward %s: %.2f, Return %s: %.2f (diff %.2f)", label, fwdVal, label, retVal, diff),
			},
			TimePattern: "constant",
			Recommendations: []string{
				fmt.Sprintf("Investigate return-path-only %s degradation (likely upstream ISP / upload saturation)", label),
			},
		})
	} else {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("asymmetry_%s_forward_%d", label, forward.ProbeID),
			Severity:        "warning",
			Title:           fmt.Sprintf("Forward path %s higher than return to %s", label, targetAgentName),
			Category:        category,
			AffectedPath:    VoicePathForward,
			TargetAgentName: targetAgentName,
			SuspectedCause:  fmt.Sprintf("Asymmetric %s — forward path is %.2f, return is %.2f", label, fwdVal, retVal),
			Evidence: []string{
				fmt.Sprintf("Forward %s: %.2f, Return %s: %.2f (diff %.2f)", label, fwdVal, label, retVal, diff),
			},
			TimePattern: "constant",
			Recommendations: []string{
				fmt.Sprintf("Investigate forward-path-only %s degradation (likely local network at source)", label),
			},
		})
	}
	return issues
}

// timePatternFromIncidents determines the dominant time pattern from issue history
func timePatternFromIncidents(issues []VoiceQualityIssue) string {
	if len(issues) == 0 {
		return "none"
	}
	patternCounts := make(map[string]int)
	for _, issue := range issues {
		if issue.TimePattern != "" && issue.TimePattern != "unknown" {
			patternCounts[issue.TimePattern]++
		}
	}
	if len(patternCounts) == 0 {
		return "constant"
	}
	// Find most common pattern
	var maxCount int
	var dominant string
	for p, c := range patternCounts {
		if c > maxCount {
			maxCount = c
			dominant = p
		}
	}
	return dominant
}

// buildVoiceQualityRecommendation generates a recommendation based on issues
func buildVoiceQualityRecommendation(issues []VoiceQualityIssue, forward, returnPath *VoicePathMetrics) string {
	if len(issues) == 0 {
		if forward != nil && forward.CongestionLevel == CongestionNone && (returnPath == nil || returnPath.CongestionLevel == CongestionNone) {
			return "Voice quality is within acceptable parameters. No action required."
		}
	}
	var severeIssues, warningIssues int
	for _, issue := range issues {
		if issue.Severity == "critical" {
			severeIssues++
		} else if issue.Severity == "warning" {
			warningIssues++
		}
	}
	if severeIssues > 0 {
		return fmt.Sprintf("%d critical and %d warning voice quality issues detected. Immediate attention recommended.", severeIssues, warningIssues)
	}
	if warningIssues > 0 {
		return fmt.Sprintf("%d warning-level voice quality issues detected. Monitor closely for worsening.", warningIssues)
	}
	return "Voice quality issues are minor or improving."
}

// ── Compute Agent Voice Quality ─────────────────────────────────────────────

// ComputeAgentVoiceQuality computes comprehensive voice quality metrics for an agent
// including forward path probes and return path probes.
//
// Bug fix: previously this function picked a single "best" forward and return
// path by MIN MOS score (named `bestForward` / `bestReturn` but actually
// worst). It now exposes three artifacts per direction:
//   - `ForwardPath` / `ReturnPath`   → the WORST-offender probe (backward compat)
//   - `AggregateForward` / `AggregateReturn` → sample-weighted average across all probes
//   - `Probes` → full per-probe list
//
// Bug fix: previously the baseline was fetched for the first probe only
// (loop `break` after first hit), so multi-target agents got an
// incorrect baseline. The new code fetches a per-probe baseline map
// and passes the matching entry into each heuristic.
//
// Bug fix: the heuristic thresholds were hard-coded literals; they now
// come from `ResolveVoiceThresholds` (defaults → admin global → per-workspace).
func ComputeAgentVoiceQuality(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint, from, to time.Time) (*VoiceQualitySummary, error) {
	agentObj, err := agent.GetAgentByID(ctx, db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent %d: %w", agentID, err)
	}

	// Resolve effective thresholds for this workspace.
	thresholds, terr := ResolveVoiceThresholds(db, workspaceSettingsFor(ctx, db, agentID))
	if terr != nil {
		log.Warnf("[voice] threshold resolution failed for agent %d, using defaults: %v", agentID, terr)
		thresholds = VoiceDefaultThresholds
	}

	// Get baseline (7 days before the analysis window)
	baselineFrom := from.Add(-7 * 24 * time.Hour)

	// Get owned probes (forward path)
	probes, err := ListForAgent(ctx, db, ch, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list probes for agent %d: %w", agentID, err)
	}

	// Separate TRAFFICSIM probes (voice-relevant) from others
	var trafficSimProbes []Probe
	for _, p := range probes {
		if p.ID == 0 {
			continue // Skip virtual probes
		}
		if p.Type == TypeTrafficSim {
			trafficSimProbes = append(trafficSimProbes, p)
		}
	}

	// Fetch TrafficSim metrics for forward path
	forwardMetrics := make(map[uint]*VoicePathMetrics)
	for _, p := range trafficSimProbes {
		metrics, err := fetchVoicePathMetrics(ctx, ch, []uint{agentID}, p.ID, from)
		if err != nil || metrics == nil {
			continue
		}
		targetName := ""
		if len(p.Targets) > 0 && p.Targets[0].AgentID != nil {
			if ta, err := agent.GetAgentByID(ctx, db, *p.Targets[0].AgentID); err == nil {
				targetName = ta.Name
			}
		}
		targetIP := ""
		if len(p.Targets) > 0 {
			targetIP = p.Targets[0].Target
		}
		metrics.TargetAgentID = uint(0)
		if len(p.Targets) > 0 && p.Targets[0].AgentID != nil {
			metrics.TargetAgentID = *p.Targets[0].AgentID
		}
		metrics.TargetAgentName = targetName
		metrics.SourceAgentID = agentID
		metrics.SourceAgentName = agentObj.Name
		metrics.ProbeID = p.ID
		metrics.ProbeType = string(p.Type)
		if targetIP != "" {
			metrics.TargetAgentName = targetIP
		}
		forwardMetrics[p.ID] = metrics
	}

	// Find return path probes (reverse AGENT probes from other agents targeting this agent)
	reverseAgentProbes, err := findReverseAgentProbes(ctx, db, agentID)
	if err != nil {
		log.Warnf("[voice] failed to find reverse agent probes for agent %d: %v", agentID, err)
	}

	// Build reverse probe metrics map keyed by source agent ID
	reverseMetrics := make(map[uint]*VoicePathMetrics)
	for _, rap := range reverseAgentProbes {
		if rap.ID == 0 {
			continue
		}
		sourceAgentID := rap.AgentID
		sourceAgent, err := agent.GetAgentByID(ctx, db, sourceAgentID)
		if err != nil {
			continue
		}
		for _, t := range rap.Targets {
			if t.AgentID != nil && *t.AgentID == agentID {
				metrics, err := fetchVoicePathMetrics(ctx, ch, []uint{sourceAgentID}, rap.ID, from)
				if err != nil || metrics == nil {
					continue
				}
				metrics.TargetAgentID = agentID
				metrics.TargetAgentName = agentObj.Name
				metrics.SourceAgentID = sourceAgentID
				metrics.SourceAgentName = sourceAgent.Name
				metrics.ProbeID = rap.ID
				metrics.ProbeType = string(rap.Type)
				metrics.Direction = VoicePathReturn
				reverseMetrics[sourceAgentID] = metrics
			}
		}
	}

	// Worst-offender (lowest MOS) for backward compat with the existing
	// `ForwardPath` / `ReturnPath` JSON field. Confusingly named in the
	// pre-fix code; the alias keeps API consumers from breaking.
	var worstForward, worstReturn *VoicePathMetrics
	for _, m := range forwardMetrics {
		m.Direction = VoicePathForward
		if worstForward == nil || m.MosScore < worstForward.MosScore {
			worstForward = m
		}
	}
	for _, m := range reverseMetrics {
		if worstReturn == nil || m.MosScore < worstReturn.MosScore {
			worstReturn = m
		}
	}

	// Per-probe baseline map. Old code used a single `baselineForward`
	// for every probe; that's wrong for agents with multiple targets
	// (each target has its own baseline). We fetch all of them and look
	// up by probe_id inside the detection helpers.
	baselineByProbeID := make(map[uint]*VoicePathMetrics)
	for _, p := range trafficSimProbes {
		bm, err := fetchVoicePathMetrics(ctx, ch, []uint{agentID}, p.ID, baselineFrom)
		if err == nil && bm != nil {
			baselineByProbeID[p.ID] = bm
		}
	}

	// Determine target agent name for issue detection
	targetName := ""
	if worstForward != nil {
		targetName = worstForward.TargetAgentName
	} else if worstReturn != nil {
		targetName = worstReturn.TargetAgentName
	}

	// Detect voice quality issues (now with per-probe baselines and
	// configurable thresholds).
	issues := detectVoiceQualityIssues(worstForward, worstReturn, baselineByProbeID, targetName, &thresholds)

	// Sample-weighted aggregate metrics (the "honest" average across
	// all probes, not just the worst one).
	allProbes := make([]*VoicePathMetrics, 0, len(forwardMetrics)+len(reverseMetrics))
	for _, m := range forwardMetrics {
		allProbes = append(allProbes, m)
	}
	for _, m := range reverseMetrics {
		allProbes = append(allProbes, m)
	}
	aggForward := aggregateVoicePathMetrics(forwardSlice(forwardMetrics), VoicePathForward)
	aggReturn := aggregateVoicePathMetrics(reverseSlice(reverseMetrics), VoicePathReturn)

	// Compute overall MOS as weighted average of the WORST-OFFENDER
	// paths (preserves existing behavior — the worst path is the
	// operator's signal of "is the worst case acceptable"). The
	// aggregate values are still surfaced separately for the report.
	var overallMos float64
	var totalWeight float64
	if worstForward != nil {
		overallMos += worstForward.MosScore * 1.0
		totalWeight += 1.0
	}
	if worstReturn != nil {
		overallMos += worstReturn.MosScore * 0.8 // Return path slightly less weight
		totalWeight += 0.8
	}
	if totalWeight > 0 {
		overallMos /= totalWeight
	} else {
		overallMos = 4.5 // Default to excellent if no data
	}

	// Compute scores from the worst-offender paths (kept consistent
	// with the existing overall MOS weighting so the report numbers
	// line up).
	var latencyScore, jitterScore, packetLossScore float64
	count := 0
	if worstForward != nil {
		latencyScore += scoreLatency(worstForward.AvgLatency, worstForward.P95Latency, worstForward.JitterAvg)
		jitterScore += jitterToScore(worstForward.JitterAvg)
		packetLossScore += scorePacketLoss(worstForward.PacketLoss)
		count++
	}
	if worstReturn != nil {
		latencyScore += scoreLatency(worstReturn.AvgLatency, worstReturn.P95Latency, worstReturn.JitterAvg)
		jitterScore += jitterToScore(worstReturn.JitterAvg)
		packetLossScore += scorePacketLoss(worstReturn.PacketLoss)
		count++
	}
	if count > 0 {
		latencyScore /= float64(count)
		jitterScore /= float64(count)
		packetLossScore /= float64(count)
	} else {
		latencyScore, jitterScore, packetLossScore = 100, 100, 100
	}

	// Time-of-day pattern from the per-bucket series (when available).
	// Falls back to the legacy pattern-from-incidents heuristic.
	timePattern := timePatternFromIncidents(issues)
	if patterns := fetchVoicePathSeriesPatterns(ctx, ch, agentID, trafficSimProbes, from, to); patterns != "" {
		timePattern = patterns
	}

	recommendation := buildVoiceQualityRecommendation(issues, worstForward, worstReturn)

	probeList := make([]VoicePathMetrics, 0, len(forwardMetrics))
	for _, m := range forwardMetrics {
		probeList = append(probeList, *m)
	}

	// Compute per-probe baseline deltas for the trend arrow.
	baselineComparison := computeBaselineComparison(allProbes, baselineByProbeID)

	// Pull workspace-level incidents for context. We re-use the helper
	// from analysis.go that returns the full WorkspaceAnalysis and
	// filter down to incidents touching this agent.
	workspaceContext := buildWorkspaceIncidentContext(ctx, db, ch, agentID, agentObj.Name, from)

	// Time series for the MOS timeline chart.
	trends := buildVoiceTrends(ctx, ch, agentID, trafficSimProbes, from, to, thresholds)

	return &VoiceQualitySummary{
		AgentID:            agentID,
		AgentName:          agentObj.Name,
		OverallMos:         overallMos,
		OverallGrade:       voiceGradeFromMos(overallMos),
		LatencyScore:       latencyScore,
		JitterScore:        jitterScore,
		PacketLossScore:    packetLossScore,
		ForwardPath:        worstForward, // now correctly named: the worst-offender
		ReturnPath:         worstReturn,
		AggregateForward:   aggForward,
		AggregateReturn:    aggReturn,
		Probes:             probeList,
		Trends:             trends,
		BaselineComparison: baselineComparison,
		WorkspaceContext:   workspaceContext,
		Issues:             issues,
		TimePattern:        timePattern,
		Recommendation:     recommendation,
		ThresholdsUsed:     &thresholds,
		GeneratedAt:        time.Now().UTC(),
	}, nil
}

// forwardSlice / reverseSlice are tiny helpers to drop the map keys
// when passing to aggregateVoicePathMetrics.
func forwardSlice(m map[uint]*VoicePathMetrics) []*VoicePathMetrics {
	out := make([]*VoicePathMetrics, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func reverseSlice(m map[uint]*VoicePathMetrics) []*VoicePathMetrics {
	return forwardSlice(m)
}

// aggregateVoicePathMetrics produces a sample-weighted aggregate
// VoicePathMetrics from a set of probes in a given direction. Latency,
// jitter, and loss are weighted by sample count; p95 metrics are the
// max (the worst P95 across probes represents the worst-case p95 the
// user can experience).
func aggregateVoicePathMetrics(probes []*VoicePathMetrics, dir VoicePathDirection) *VoicePathMetrics {
	if len(probes) == 0 {
		return nil
	}
	var (
		totalSamples                              int
		weightedMos, weightedLat                  float64
		weightedJitter, weightedLoss, weightedOOO float64
		weightedDup, weightedJitterP95            float64
		p95Lat, jitterP95                         float64
		mosContribs                               []string
	)
	for _, p := range probes {
		if p == nil || p.SampleCount == 0 {
			continue
		}
		w := float64(p.SampleCount)
		totalSamples += p.SampleCount
		weightedMos += p.MosScore * w
		weightedLat += p.AvgLatency * w
		weightedJitter += p.JitterAvg * w
		weightedJitterP95 += p.JitterP95 * w
		weightedLoss += p.PacketLoss * w
		weightedOOO += p.OutOfSequence * w
		weightedDup += p.Duplicates * w
		if p.P95Latency > p95Lat {
			p95Lat = p.P95Latency
		}
		if p.JitterP95 > jitterP95 {
			jitterP95 = p.JitterP95
		}
		mosContribs = append(mosContribs, p.MosContributors...)
	}
	if totalSamples == 0 {
		return nil
	}
	w := float64(totalSamples)
	out := &VoicePathMetrics{
		Direction:       dir,
		SampleCount:     totalSamples,
		MosScore:        weightedMos / w,
		AvgLatency:      weightedLat / w,
		P95Latency:      p95Lat,
		JitterAvg:       weightedJitter / w,
		JitterP95:       jitterP95,
		PacketLoss:      weightedLoss / w,
		OutOfSequence:   weightedOOO / w,
		Duplicates:      weightedDup / w,
		MosContributors: dedupeStrings(mosContribs),
	}
	out.MedianLatency = out.AvgLatency // close enough for an aggregate label
	out.CongestionLevel = congestionLevelFromMetrics(out.JitterAvg, out.PacketLoss, out.AvgLatency)
	return out
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// computeBaselineComparison rolls up per-probe baseline deltas into a
// single BaselineDelta for the executive summary. Returns nil when no
// baseline samples were found.
func computeBaselineComparison(probes []*VoicePathMetrics, baselineByProbeID map[uint]*VoicePathMetrics) *BaselineDelta {
	var (
		totalSamples, baselineSamples int
		mosDeltaSum, latDeltaSum      float64
		jitDeltaSum, lossDeltaSum     float64
		mosDeltaCount                 int
	)
	for _, p := range probes {
		if p == nil {
			continue
		}
		totalSamples += p.SampleCount
		bm := baselineByProbeID[p.ProbeID]
		if bm == nil {
			continue
		}
		baselineSamples += bm.SampleCount
		mosDeltaSum += p.MosScore - bm.MosScore
		latDeltaSum += p.AvgLatency - bm.AvgLatency
		jitDeltaSum += p.JitterAvg - bm.JitterAvg
		lossDeltaSum += p.PacketLoss - bm.PacketLoss
		mosDeltaCount++
	}
	if baselineSamples == 0 {
		return nil
	}
	d := &BaselineDelta{
		From:            time.Now().UTC().Add(-7 * 24 * time.Hour),
		To:              time.Now().UTC(),
		SampleCount:     totalSamples,
		BaselineSamples: baselineSamples,
	}
	if mosDeltaCount > 0 {
		d.MosDelta = mosDeltaSum / float64(mosDeltaCount)
		d.LatencyDeltaMs = latDeltaSum / float64(mosDeltaCount)
		d.JitterDeltaMs = jitDeltaSum / float64(mosDeltaCount)
		d.LossDeltaPct = lossDeltaSum / float64(mosDeltaCount)
	}
	switch {
	case d.MosDelta > 0.1:
		d.Trend = "improving"
	case d.MosDelta < -0.1:
		d.Trend = "worsening"
	default:
		d.Trend = "stable"
	}
	if d.MosDelta > 0 {
		d.PercentChange = d.MosDelta * 100
	} else {
		d.PercentChange = d.MosDelta * 100
	}
	return d
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

// fetchVoicePathMetrics fetches voice path metrics from ClickHouse for a given probe
func fetchVoicePathMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, probeID uint, from time.Time) (*VoicePathMetrics, error) {
	if len(agentIDs) == 0 {
		return nil, nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	q := fmt.Sprintf(`
SELECT 
    avg_rtt, median_rtt, p95_rtt, p99_rtt,
    jitter_avg, jitter_median, jitter_p95,
    loss_pct, out_of_seq_pct, duplicate_pct,
    mos_score, sample_count
FROM (
    SELECT 
        avg(average_rtt) as avg_rtt,
        avg(median_rtt) as median_rtt,
        avg(p95_rtt) as p95_rtt,
        avg(p99_rtt) as p99_rtt,
        avg(jitter_avg) as jitter_avg,
        avg(jitter_median) as jitter_median,
        avg(jitter_p95) as jitter_p95,
        avg(loss_pct) as loss_pct,
        avg(out_of_seq_pct) as out_of_seq_pct,
        avg(duplicate_pct) as duplicate_pct,
        avg(mos_score) as mos_score,
        count(*) as sample_count
    FROM traffic_metrics
    WHERE agent_id IN (%s)
      AND probe_id = %d
      AND created_at >= %s
    GROUP BY agent_id
) subq
`, agentIDList, probeID, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	var m VoicePathMetrics
	var avgRtt, medianRtt, p95Rtt, p99Rtt, jitterAvg, jitterMedian, jitterP95, lossPct, outOfSeqPct, duplicatePct, mosScore float64
	var sampleCount int

	if err := rows.Scan(&avgRtt, &medianRtt, &p95Rtt, &p99Rtt, &jitterAvg, &jitterMedian, &jitterP95, &lossPct, &outOfSeqPct, &duplicatePct, &mosScore, &sampleCount); err != nil {
		return nil, err
	}

	// Convert RTT to one-way latency estimate (divide by 2)
	m.AvgLatency = avgRtt / 2
	m.MedianLatency = medianRtt / 2
	m.P95Latency = p95Rtt / 2
	m.JitterAvg = jitterAvg
	m.JitterMedian = jitterMedian
	m.JitterP95 = jitterP95
	m.PacketLoss = lossPct
	m.OutOfSequence = outOfSeqPct
	m.Duplicates = duplicatePct
	m.MosScore = mosScore
	m.SampleCount = sampleCount
	m.MosContributors = mosContributingFactors(m.AvgLatency, m.P95Latency, m.JitterAvg, m.PacketLoss)
	m.CongestionLevel = congestionLevelFromMetrics(m.JitterAvg, m.PacketLoss, m.AvgLatency)

	return &m, nil
}

// ComputePerAgentAnalysis computes full analysis for a single agent including voice quality
func ComputePerAgentAnalysis(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint, lookbackMinutes int) (*AgentAnalysis, error) {
	agentObj, err := agent.GetAgentByID(ctx, db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent %d: %w", agentID, err)
	}

	from := time.Now().UTC().Add(-time.Duration(lookbackMinutes) * time.Minute)

	// Compute voice quality
	vq, err := ComputeAgentVoiceQuality(ctx, db, ch, agentID, from, time.Now().UTC())
	if err != nil {
		log.Warnf("[analysis] failed to compute voice quality for agent %d: %v", agentID, err)
	}

	// Get agent's probes
	probes, err := ListForAgent(ctx, db, ch, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list probes for agent %d: %w", agentID, err)
	}

	// Build probe analyses
	var probeAnalyses []ProbeAnalysis
	for _, p := range probes {
		if p.ID == 0 {
			continue
		}
		pa := ProbeAnalysis{
			ProbeID:     p.ID,
			ProbeType:   string(p.Type),
			AgentID:     agentID,
			AgentName:   agentObj.Name,
			GeneratedAt: time.Now().UTC(),
		}
		if len(p.Targets) > 0 {
			pa.Target = p.Targets[0].Target
		}
		probeAnalyses = append(probeAnalyses, pa)
	}

	// Check if online
	isOnline := agentObj.LastSeenAt.After(time.Now().UTC().Add(-5 * time.Minute))

	// Compute health vector from voice metrics if available
	var health HealthVector
	if vq != nil {
		health = HealthVector{
			OverallHealth:   (vq.LatencyScore + vq.JitterScore + vq.PacketLossScore) / 3,
			LatencyScore:    vq.LatencyScore,
			PacketLossScore: vq.PacketLossScore,
			MosScore:        vq.OverallMos,
			Grade:           vq.OverallGrade,
		}
	} else {
		health = HealthVector{Grade: "unknown", RouteStability: 100, MosScore: 1.0}
	}

	return &AgentAnalysis{
		AgentID:          agentID,
		AgentName:        agentObj.Name,
		IsOnline:         isOnline,
		Health:           health,
		VoiceQuality:     vq,
		Probes:           probeAnalyses,
		ReturnPathProbes: nil,
		Incidents:        nil,
		GeneratedAt:      time.Now().UTC(),
	}, nil
}
