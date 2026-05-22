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
	ProbeID      uint              `json:"probe_id"`
	ProbeType    string            `json:"probe_type"`
	Target       string            `json:"target"`
	AgentID      uint              `json:"agent_id"`
	AgentName    string            `json:"agent_name"`
	Health       HealthVector      `json:"health"`
	Metrics      ProbeMetrics      `json:"metrics"`
	PathAnalysis *MtrPathAnalysis  `json:"path_analysis,omitempty"`
	Reverse      *ProbeAnalysis    `json:"reverse,omitempty"`
	Signals      []AnalysisSignal  `json:"signals"`
	Findings     []AnalysisFinding `json:"findings"`
	GeneratedAt  time.Time         `json:"generated_at"`
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
		p95Lat = percentile(p95RTTs, 50) // p95RTTs contains P95 values from each cycle
	} else {
		_, p95Lat, _ = FallbackPercentiles(latencies)
	}
	if len(p99RTTs) > 0 {
		p99Lat = percentile(p99RTTs, 50) // p99RTTs contains P99 values from each cycle
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

	// Fetch PING metrics for this probe — source agent only
	metrics, err := probeAnalysisMetrics(ctx, ch, []uint{p.AgentID}, pingProbeID, from)
	if err != nil {
		log.Warnf("[Analysis] Failed to fetch PING metrics for probe %d: %v", pingProbeID, err)
		metrics = ProbeMetrics{}
	}

	log.Debugf("[Analysis] Probe %d (type=%s): PING samples=%d (queried pid=%d), avgLat=%.1f, loss=%.2f%%, agentIDs=%v",
		probeID, p.Type, metrics.SampleCount, pingProbeID, metrics.AvgLatency, metrics.PacketLoss, agentIDs)

	// Fetch MTR path analysis — source agent only
	pathAnalysis, mtrSignals, err := analyzeMtrForProbe(ctx, ch, []uint{p.AgentID}, mtrProbeID, from, agentIPToID, agentByID)
	if err != nil {
		log.Warnf("[Analysis] Failed to analyze MTR for probe %d: %v", mtrProbeID, err)
	}

	// For AGENT probes, also fetch TrafficSim data (same probe_id, different type) — source agent only
	// For non-AGENT probes, also fetch TrafficSim to use as fallback when MTR end-hop is needed
	tsMetrics := ProbeMetrics{}
	if p.Type == TypeAgent {
		tsMetrics = probeTrafficSimMetrics(ctx, ch, []uint{p.AgentID}, probeID, from)
		log.Debugf("[Analysis] Probe %d AGENT: TrafficSim samples=%d, avgRTT=%.1f, loss=%.2f%%",
			probeID, tsMetrics.SampleCount, tsMetrics.AvgLatency, tsMetrics.PacketLoss)
		if tsMetrics.SampleCount > 0 {
			// If PING data was empty, use TrafficSim as primary (not blended)
			if metrics.SampleCount == 0 {
				metrics.AvgLatency = tsMetrics.AvgLatency
				metrics.P95Latency = tsMetrics.P95Latency
				metrics.PacketLoss = tsMetrics.PacketLoss
				metrics.SampleCount = tsMetrics.SampleCount
			} else {
				// Blend: use worse of PING/TrafficSim loss, average the latencies
				if tsMetrics.PacketLoss > metrics.PacketLoss {
					metrics.PacketLoss = tsMetrics.PacketLoss
				}
			}
		}
	}

	// For non-AGENT probes (standalone MTR, etc.), if PING metrics are empty
	// but MTR path analysis found data, derive metrics from MTR end-hop stats.
	// This ensures standalone MTR probes get proper health scores.
	var fallbackSignals []AnalysisSignal
	if metrics.SampleCount == 0 && pathAnalysis != nil && pathAnalysis.TraceCount > 0 {
		log.Debugf("[Analysis] Probe %d (type=%s): No PING data, falling back to MTR end-hop metrics (traces=%d, lat=%.1f, loss=%.2f%%, jitter=%.1f)",
			probeID, p.Type, pathAnalysis.TraceCount, pathAnalysis.AvgEndHopLatency, pathAnalysis.AvgEndHopLoss, pathAnalysis.AvgEndHopJitterAvg)
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

	// Compute health vector
	health := computeHealthVector(metrics, routeStability)

	// Build combined signals
	var signals []AnalysisSignal
	signals = append(signals, mtrSignals...)
	signals = append(signals, fallbackSignals...)

	// Add metric-based signals
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

	// Build findings from signals
	findings := buildFindings(health, metrics, pathAnalysis, signals)

	result := &ProbeAnalysis{
		ProbeID:      probeID,
		ProbeType:    string(p.Type),
		Target:       targetName,
		AgentID:      p.AgentID,
		AgentName:    agentName,
		Health:       health,
		Metrics:      metrics,
		PathAnalysis: pathAnalysis,
		Signals:      signals,
		Findings:     findings,
		GeneratedAt:  time.Now().UTC(),
	}

	// Check for reverse/reciprocal probe
	if targetAgentID > 0 {
		reverseProbes, err := findReverseAgentProbes(ctx, pg, p.AgentID)
		if err == nil {
			for _, rp := range reverseProbes {
				if rp.AgentID == targetAgentID {
					// Found the reverse probe — compute its analysis using target agent data only
					revMetrics, _ := probeAnalysisMetrics(ctx, ch, []uint{targetAgentID}, rp.ID, from)
					revPath, revSignals, _ := analyzeMtrForProbe(ctx, ch, []uint{targetAgentID}, rp.ID, from, agentIPToID, agentByID)
					revRouteStab := 100.0
					if revPath != nil {
						revRouteStab = revPath.RouteStabilityPct
					}
					revHealth := computeHealthVector(revMetrics, revRouteStab)
					revFindings := buildFindings(revHealth, revMetrics, revPath, revSignals)

					revAgentName := ""
					if a, ok := agentByID[targetAgentID]; ok {
						revAgentName = a.Name
					}

					result.Reverse = &ProbeAnalysis{
						ProbeID:      rp.ID,
						ProbeType:    string(rp.Type),
						Target:       agentName, // reverse target is the original agent
						AgentID:      targetAgentID,
						AgentName:    revAgentName,
						Health:       revHealth,
						Metrics:      revMetrics,
						PathAnalysis: revPath,
						Signals:      revSignals,
						Findings:     revFindings,
						GeneratedAt:  time.Now().UTC(),
					}
					break
				}
			}
		}
	}

	return result, nil
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
	agentIPToID := buildAgentIPToIDMap(agentSummaries, agentByID)
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

// resolveTargetToName checks if the target IP matches any agent and returns the agent name
func resolveTargetToName(target string, agentByID map[uint]agentInfo, agentIPToID map[string]uint) string {
	if agentID, ok := agentIPToID[target]; ok {
		if agent, ok := agentByID[agentID]; ok {
			return agent.Name
		}
	}
	return target
}

// buildAgentIPToIDMap builds a map from agent IP addresses to agent IDs
func buildAgentIPToIDMap(agents []AgentHealthSummary, agentByID map[uint]agentInfo) map[string]uint {
	agentIPToID := make(map[string]uint)
	for _, agent := range agents {
		if a, ok := agentByID[agent.AgentID]; ok {
			if a.PublicIPOverride != "" {
				agentIPToID[a.PublicIPOverride] = agent.AgentID
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

func getWorkspaceNetInfoChanges(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) ([]netInfoChange, error) {
	if len(agentIDs) == 0 {
		return nil, nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	// Get last 2 records per agent to detect changes
	q := fmt.Sprintf(`
SELECT agent_id, payload_raw, created_at
FROM probe_data
WHERE type = 'NETINFO'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY agent_id, created_at DESC
LIMIT 200
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
	Target              string      `json:"target"`
	BaselineFingerprint string      `json:"baseline_fingerprint,omitempty"`
	BaselineHopCount    int         `json:"baseline_hop_count,omitempty"`
	BaselineRoutePath   string      `json:"baseline_route_path,omitempty"`
	LatestSignature     string      `json:"latest_signature,omitempty"`
	LatestHops          []string    `json:"latest_hops,omitempty"`        // IPs only (for signature computation)
	LatestHopsDetail    []HopDetail `json:"latest_hops_detail,omitempty"` // Enriched with agent names
	HasRouteChange      bool        `json:"has_route_change"`
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
	WorkspaceID uint             `json:"workspace_id"`
	Agents      []AgentRouteInfo `json:"agents"`
	SharedHops  []SharedHopInfo  `json:"shared_hops"`
	Incidents   []RouteIncident  `json:"incidents"`
	TotalAgents int              `json:"total_agents"`
	TotalRoutes int              `json:"total_routes"`
	GeneratedAt time.Time        `json:"generated_at"`
}

// ComputeWorkspaceRouteAnalysis aggregates route/path data across all agents in a workspace
// for the route/path matching visualization.
func ComputeWorkspaceRouteAnalysis(ctx context.Context, ch *sql.DB, pg *gorm.DB, workspaceID uint) (*WorkspaceRouteAnalysis, error) {
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

	// Look back 24 hours for MTR data and 1 hour for net-info changes
	mtrFrom := time.Now().UTC().Add(-24 * time.Hour)
	netInfoFrom := time.Now().UTC().Add(-60 * time.Minute)

	// 2. Get latest NETINFO per agent (for IP/ISP)
	netInfoByAgent := make(map[uint]*netInfoPayload)
	for _, a := range agents {
		row, err := GetLatestNetInfoForAgent(ctx, ch, uint64(a.ID), nil)
		if err != nil || row == nil {
			continue
		}
		var p netInfoPayload
		if err := json.Unmarshal(row.Payload, &p); err == nil {
			p.NormalizeFromLegacy()
			netInfoByAgent[a.ID] = &p
		}
	}

	// 3. Detect IP/ISP changes
	netInfoChanges, _ := getWorkspaceNetInfoChanges(ctx, ch, agentIDs, netInfoFrom)
	changeByAgent := make(map[uint][]netInfoChange)
	for _, c := range netInfoChanges {
		changeByAgent[c.AgentID] = append(changeByAgent[c.AgentID], c)
	}

	// 4. Get MTR probes per agent from Postgres
	agentRoutes := make([]AgentRouteInfo, 0, len(agents))
	hopIndex := make(map[string]map[uint]HopMetrics) // hopIP -> agentID -> metrics
	routeIncidents := make([]RouteIncident, 0)
	totalRoutes := 0

	for _, a := range agents {
		ari := AgentRouteInfo{
			AgentID:   a.ID,
			AgentName: a.Name,
		}

		// Populate IP/ISP from netinfo
		if ni := netInfoByAgent[a.ID]; ni != nil {
			ari.PublicIP = ni.PublicAddress
			ari.ISP = ni.GetISP()
		}

		// Check for changes
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

		// Get probes for this agent
		probes, err := ListByAgent(ctx, pg, a.ID)
		if err != nil {
			agentRoutes = append(agentRoutes, ari)
			continue
		}

		for _, p := range probes {
			if p.Type != TypeMTR || !p.Enabled {
				continue
			}
			// Determine target string
			target := ""
			for _, t := range p.Targets {
				if t.Target != "" {
					target = t.Target
					break
				}
			}

			pri := ProbeRouteInfo{
				ProbeID: p.ID,
				Target:  target,
			}

			// Get route baseline
			var baseline routeBaseline
			if err := pg.WithContext(ctx).Where("probe_id = ?", p.ID).First(&baseline).Error; err == nil {
				pri.BaselineFingerprint = baseline.Fingerprint
				pri.BaselineHopCount = baseline.HopCount
				pri.BaselineRoutePath = baseline.RoutePath
			}

			// Get latest MTR data for this probe
			mtrRows, err := GetProbeDataByProbe(ctx, ch, uint64(p.ID), nil, mtrFrom, time.Now().UTC(), false, 50)
			if err == nil && len(mtrRows) > 0 {
				// Build route signatures and compute stability
				sigs := make(map[string]int)
				var latestPayload *MtrPayload
				for i := range mtrRows {
					var mp MtrPayload
					if err := json.Unmarshal(mtrRows[i].Payload, &mp); err != nil {
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
						// Build enriched hop details with agent name matching
						pri.LatestHopsDetail = buildHopDetails(latestPayload, agentIPToID, agentByID)
					}
				}
				if latestPayload != nil {
					pri.TraceCount = len(mtrRows)
					if len(sigs) > 1 {
						maxCount := 0
						for _, c := range sigs {
							if c > maxCount {
								maxCount = c
							}
						}
						pri.RouteStabilityPct = math.Round(float64(maxCount)/float64(len(mtrRows))*100*10) / 10
						pri.HasRouteChange = true
					} else {
						pri.RouteStabilityPct = 100
					}
					// End-hop metrics
					if len(latestPayload.Report.Hops) > 0 {
						lastHop := latestPayload.Report.Hops[len(latestPayload.Report.Hops)-1]
						pri.AvgEndHopLatency = parseLatency(lastHop.Avg)
						pri.AvgEndHopLoss = parseLossPct(lastHop.LossPct)
					}
					// Intermediate hop metrics (all hops except the last/end hop)
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
			}

			// Index hops for shared-hop computation
			if len(pri.LatestHops) > 1 {
				for _, ip := range pri.LatestHops[:len(pri.LatestHops)-1] {
					if ip == "" || ip == "*" {
						continue
					}
					if hopIndex[ip] == nil {
						hopIndex[ip] = make(map[uint]HopMetrics)
					}
					metrics := HopMetrics{Count: 1}
					for _, ih := range pri.IntermediateHops {
						if ih.IP == ip {
							metrics.TotalLoss += ih.Loss
							metrics.TotalLatency += ih.Latency
							if ih.Loss > 0 || ih.Latency > 100 {
								metrics.HasIssues = true
							}
							break
						}
					}
					hopIndex[ip][a.ID] = metrics
				}
			}

			ari.Routes = append(ari.Routes, pri)
			totalRoutes++

			// Add route_change incident if detected
			if pri.HasRouteChange {
				routeIncidents = append(routeIncidents, RouteIncident{
					ID:        fmt.Sprintf("route_change_%d_%d", a.ID, p.ID),
					Type:      "route_change",
					Severity:  "warning",
					AgentID:   a.ID,
					AgentName: a.Name,
					ProbeID:   p.ID,
					Target:    target,
					Message:   fmt.Sprintf("Route changed for %s → %s (stability %.0f%%)", a.Name, target, pri.RouteStabilityPct),
					Evidence: []string{
						fmt.Sprintf("Baseline fingerprint: %s", pri.BaselineFingerprint),
						fmt.Sprintf("Current signature: %s", pri.LatestSignature),
						fmt.Sprintf("Route stability: %.0f%% over %d traces", pri.RouteStabilityPct, pri.TraceCount),
					},
				})
			}
		}

		agentRoutes = append(agentRoutes, ari)
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

	return &WorkspaceRouteAnalysis{
		WorkspaceID: workspaceID,
		Agents:      agentRoutes,
		SharedHops:  sharedHops,
		Incidents:   routeIncidents,
		TotalAgents: len(agents),
		TotalRoutes: totalRoutes,
		GeneratedAt: time.Now().UTC(),
	}, nil
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

// VoiceQualitySummary is the complete voice quality assessment for an agent
type VoiceQualitySummary struct {
	AgentID         uint                `json:"agent_id"`
	AgentName       string              `json:"agent_name"`
	OverallMos      float64             `json:"overall_mos"`       // weighted average of forward + return
	OverallGrade    string              `json:"overall_grade"`     // excellent/good/fair/poor/critical
	LatencyScore    float64             `json:"latency_score"`     // 0-100
	JitterScore     float64             `json:"jitter_score"`      // 0-100
	PacketLossScore float64             `json:"packet_loss_score"` // 0-100
	ForwardPath     *VoicePathMetrics   `json:"forward_path,omitempty"`
	ReturnPath      *VoicePathMetrics   `json:"return_path,omitempty"`
	Issues          []VoiceQualityIssue `json:"issues"`
	TimePattern     string              `json:"time_pattern"` // "constant", "mixed", "periodic", "unknown"
	Recommendation  string              `json:"recommendation"`
	GeneratedAt     time.Time           `json:"generated_at"`
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

// detectVoiceQualityIssues detects voice quality problems across forward and return paths
func detectVoiceQualityIssues(forward, returnPath *VoicePathMetrics, baselineForward, baselineReturn *VoicePathMetrics, targetAgentName string) []VoiceQualityIssue {
	var issues []VoiceQualityIssue

	// 1. Jitter spike detection (forward)
	if forward != nil {
		issues = append(issues, detectJitterAnomalies(forward, baselineForward, VoicePathForward, targetAgentName)...)
	}

	// 2. Jitter spike detection (return)
	if returnPath != nil {
		issues = append(issues, detectJitterAnomalies(returnPath, baselineReturn, VoicePathReturn, targetAgentName)...)
	}

	// 3. Packet loss burst detection
	if forward != nil {
		issues = append(issues, detectPacketLossAnomalies(forward, baselineForward, VoicePathForward, targetAgentName)...)
	}
	if returnPath != nil {
		issues = append(issues, detectPacketLossAnomalies(returnPath, baselineReturn, VoicePathReturn, targetAgentName)...)
	}

	// 4. Latency-only degradation (high latency but no packet loss — route issue)
	if forward != nil {
		issues = append(issues, detectLatencyOnlyDegradation(forward, baselineForward, VoicePathForward, targetAgentName)...)
	}
	if returnPath != nil {
		issues = append(issues, detectLatencyOnlyDegradation(returnPath, baselineReturn, VoicePathReturn, targetAgentName)...)
	}

	// 5. Out of sequence / packet reordering
	if forward != nil && forward.OutOfSequence > 1.0 {
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
	if returnPath != nil && returnPath.OutOfSequence > 1.0 {
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
		issues = append(issues, detectAsymmetricVoiceDegradation(forward, returnPath, targetAgentName)...)
	}

	return issues
}

// detectJitterAnomalies identifies jitter spikes above voice quality thresholds
func detectJitterAnomalies(path, baseline *VoicePathMetrics, direction VoicePathDirection, targetAgentName string) []VoiceQualityIssue {
	var issues []VoiceQualityIssue
	if path == nil || path.JitterAvg <= 0 {
		return issues
	}

	// Threshold: jitter > 15ms is problematic for voice
	// Threshold: jitter > 25ms is critical
	threshold := 15.0
	criticalThreshold := 25.0

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
			SuspectedCause:  "Very high jitter (>25ms) causes voice buffer underruns leading to choppy or garbled audio",
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

		cause := "Elevated jitter (>15ms) can cause voice quality degradation during calls"
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
	if baseline != nil && baseline.JitterAvg > 5 && path.JitterAvg > baseline.JitterAvg*2 {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("jitter_spike_%d_%s", path.ProbeID, direction),
			Severity:        "warning",
			Title:           fmt.Sprintf("Sudden jitter increase on %s path to %s", direction, targetAgentName),
			Category:        "jitter_spike",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  "Jitter more than doubled from baseline — possible network event, congestion, or route change",
			Evidence: []string{
				fmt.Sprintf("Current jitter: %.1fms vs baseline: %.1fms (2x increase)", path.JitterAvg, baseline.JitterAvg),
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
func detectPacketLossAnomalies(path, baseline *VoicePathMetrics, direction VoicePathDirection, targetAgentName string) []VoiceQualityIssue {
	var issues []VoiceQualityIssue
	if path == nil || path.PacketLoss <= 0 {
		return issues
	}

	// Voice-relevant thresholds: >2% is problematic, >5% is critical
	if path.PacketLoss > 5 {
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
			SuspectedCause:  "Packet loss >5%% will cause noticeable call quality issues — dropped words, robotic voice, call drops",
			Evidence: []string{
				fmt.Sprintf("Packet loss: %.2f%% (critical threshold: 5%%)", path.PacketLoss),
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
	} else if path.PacketLoss > 2 {
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

		cause := "Moderate packet loss (2-5%%) causes occasional dropouts and reduced call quality"
		if path.OutOfSequence > 1 {
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
				fmt.Sprintf("Packet loss: %.2f%% (warning threshold: 2%%)", path.PacketLoss),
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
	if baseline != nil && baseline.PacketLoss < 0.5 && path.PacketLoss > 2 {
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
func detectLatencyOnlyDegradation(path, baseline *VoicePathMetrics, direction VoicePathDirection, targetAgentName string) []VoiceQualityIssue {
	var issues []VoiceQualityIssue
	if path == nil || path.PacketLoss > 1 {
		return issues // Not latency-only if there's packet loss
	}

	// Latency-only issue: high latency but MOS degraded without packet loss
	if path.MosScore < 4.0 && path.AvgLatency > 100 && path.PacketLoss < 0.5 {
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("latency_only_%d_%s", path.ProbeID, direction),
			Severity:        "warning",
			Title:           fmt.Sprintf("Latency impacting voice quality on %s path to %s", direction, targetAgentName),
			Category:        "latency_degradation",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  "High latency (>100ms) with no packet loss suggests route inefficiency or distant peering point",
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

// detectAsymmetricVoiceDegradation compares forward vs return path for asymmetric issues
func detectAsymmetricVoiceDegradation(forward, returnPath *VoicePathMetrics, targetAgentName string) []VoiceQualityIssue {
	var issues []VoiceQualityIssue

	// Calculate MOS ratio
	var mosRatio float64
	if forward.MosScore > 0 {
		mosRatio = returnPath.MosScore / forward.MosScore
	}

	// Significant asymmetry: return path is much worse than forward
	if mosRatio < 0.75 && forward.MosScore > 3.5 {
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
	if mosRatio > 1.25 && returnPath.MosScore > 3.5 {
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
// including forward path probes and return path probes
func ComputeAgentVoiceQuality(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint, from, to time.Time) (*VoiceQualitySummary, error) {
	agentObj, err := agent.GetAgentByID(ctx, db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent %d: %w", agentID, err)
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
	var mtrProbes []Probe
	for _, p := range probes {
		if p.ID == 0 {
			continue // Skip virtual probes
		}
		switch p.Type {
		case TypeTrafficSim:
			trafficSimProbes = append(trafficSimProbes, p)
		case TypeMTR:
			mtrProbes = append(mtrProbes, p)
		}
	}

	// Fetch TrafficSim metrics for forward path
	forwardMetrics := make(map[uint]*VoicePathMetrics)
	for _, p := range trafficSimProbes {
		metrics, err := fetchVoicePathMetrics(ctx, ch, []uint{agentID}, p.ID, from)
		if err != nil || metrics == nil {
			continue
		}
		// Get target agent name
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
		// rap.AgentID is the source agent that owns this reverse probe
		sourceAgentID := rap.AgentID
		sourceAgent, err := agent.GetAgentByID(ctx, db, sourceAgentID)
		if err != nil {
			continue
		}
		// Expand the AGENT probe to get the target (which is this agent)
		for _, t := range rap.Targets {
			if t.AgentID != nil && *t.AgentID == agentID {
				// This reverse probe targets this agent
				// Look for TRAFFICSIM metrics from this source agent to our target
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

	// Determine best forward and return path metrics (by MOS score)
	var bestForward, bestReturn *VoicePathMetrics
	for _, m := range forwardMetrics {
		m.Direction = VoicePathForward
		if bestForward == nil || m.MosScore < bestForward.MosScore {
			bestForward = m
		}
	}
	for _, m := range reverseMetrics {
		if bestReturn == nil || m.MosScore < bestReturn.MosScore {
			bestReturn = m
		}
	}

	// Fetch baseline metrics for comparison
	var baselineForward, baselineReturn *VoicePathMetrics
	if len(forwardMetrics) > 0 {
		// Use first forward probe for baseline
		for _, p := range trafficSimProbes {
			bm, err := fetchVoicePathMetrics(ctx, ch, []uint{agentID}, p.ID, baselineFrom)
			if err == nil && bm != nil {
				baselineForward = bm
				break
			}
		}
	}

	// Determine target agent name for issue detection
	targetName := ""
	if bestForward != nil {
		targetName = bestForward.TargetAgentName
	} else if bestReturn != nil {
		targetName = bestReturn.TargetAgentName
	}

	// Detect voice quality issues
	issues := detectVoiceQualityIssues(bestForward, bestReturn, baselineForward, baselineReturn, targetName)

	// Compute overall MOS as weighted average
	var overallMos float64
	var totalWeight float64
	if bestForward != nil {
		overallMos += bestForward.MosScore * 1.0
		totalWeight += 1.0
	}
	if bestReturn != nil {
		overallMos += bestReturn.MosScore * 0.8 // Return path slightly less weight
		totalWeight += 0.8
	}
	if totalWeight > 0 {
		overallMos /= totalWeight
	} else {
		overallMos = 4.5 // Default to excellent if no data
	}

	// Compute scores
	var latencyScore, jitterScore, packetLossScore float64
	count := 0
	if bestForward != nil {
		latencyScore += scoreLatency(bestForward.AvgLatency, bestForward.P95Latency, bestForward.JitterAvg)
		jitterScore += jitterToScore(bestForward.JitterAvg)
		packetLossScore += scorePacketLoss(bestForward.PacketLoss)
		count++
	}
	if bestReturn != nil {
		latencyScore += scoreLatency(bestReturn.AvgLatency, bestReturn.P95Latency, bestReturn.JitterAvg)
		jitterScore += jitterToScore(bestReturn.JitterAvg)
		packetLossScore += scorePacketLoss(bestReturn.PacketLoss)
		count++
	}
	if count > 0 {
		latencyScore /= float64(count)
		jitterScore /= float64(count)
		packetLossScore /= float64(count)
	} else {
		latencyScore, jitterScore, packetLossScore = 100, 100, 100
	}

	timePattern := timePatternFromIncidents(issues)
	recommendation := buildVoiceQualityRecommendation(issues, bestForward, bestReturn)

	return &VoiceQualitySummary{
		AgentID:         agentID,
		AgentName:       agentObj.Name,
		OverallMos:      overallMos,
		OverallGrade:    voiceGradeFromMos(overallMos),
		LatencyScore:    latencyScore,
		JitterScore:     jitterScore,
		PacketLossScore: packetLossScore,
		ForwardPath:     bestForward,
		ReturnPath:      bestReturn,
		Issues:          issues,
		TimePattern:     timePattern,
		Recommendation:  recommendation,
		GeneratedAt:     time.Now().UTC(),
	}, nil
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
