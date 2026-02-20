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
	AvgLatency  float64 `json:"avg_latency"` // ms
	P95Latency  float64 `json:"p95_latency"` // ms
	PacketLoss  float64 `json:"packet_loss"` // percentage
	Jitter      float64 `json:"jitter"`      // ms (stddev)
	SampleCount int     `json:"sample_count"`
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
	HopCount          int      `json:"hop_count"`
	UniqueRoutes      int      `json:"unique_routes"`
	RouteStabilityPct float64  `json:"route_stability_pct"`
	AvgEndHopLatency  float64  `json:"avg_end_hop_latency"`
	AvgEndHopLoss     float64  `json:"avg_end_hop_loss"`
	RateLimitedHops   []int    `json:"rate_limited_hops"`
	TimeoutSegments   []string `json:"timeout_segments"`
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
	latScore := scoreLatency(metrics.AvgLatency, metrics.P95Latency, metrics.Jitter)
	lossScore := scorePacketLoss(metrics.PacketLoss)
	mos := computeMos(metrics.AvgLatency, metrics.PacketLoss, metrics.Jitter)

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

		latMs := float64(payload.AvgRTT) / 1_000_000.0 // ns to ms
		jitterMs := float64(payload.StdDevRTT) / 1_000_000.0

		latencies = append(latencies, latMs)
		totalLoss += payload.PacketLoss
		totalJitter += jitterMs
		count++
	}

	if count == 0 {
		return ProbeMetrics{}, nil
	}

	// Calculate percentiles
	avgLat := avg(latencies)
	p95Lat := percentile(latencies, 95)
	avgLoss := totalLoss / float64(count)
	avgJitter := totalJitter / float64(count)

	return ProbeMetrics{
		AvgLatency:  sanitizeFloat(avgLat),
		P95Latency:  sanitizeFloat(p95Lat),
		PacketLoss:  sanitizeFloat(avgLoss),
		Jitter:      sanitizeFloat(avgJitter),
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
	var totalLoss float64
	var count int

	for rows.Next() {
		var payloadRaw string
		if err := rows.Scan(&payloadRaw); err != nil || payloadRaw == "" {
			continue
		}

		var payload struct {
			AverageRTT     float64 `json:"averageRTT"`
			LossPercentage float64 `json:"lossPercentage"`
		}
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}

		latencies = append(latencies, payload.AverageRTT)
		totalLoss += payload.LossPercentage
		count++
	}

	if count == 0 {
		return ProbeMetrics{}
	}

	return ProbeMetrics{
		AvgLatency:  sanitizeFloat(avg(latencies)),
		P95Latency:  sanitizeFloat(percentile(latencies, 95)),
		PacketLoss:  sanitizeFloat(totalLoss / float64(count)),
		SampleCount: count,
	}
}

// analyzeMtrForProbe fetches MTR traces and produces path analysis + signals
func analyzeMtrForProbe(ctx context.Context, ch *sql.DB, agentIDs []uint, probeID uint, from time.Time) (*MtrPathAnalysis, []AnalysisSignal, error) {
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
	var rateLimitedHops []int
	var timeoutSegments []string
	var maxHops int

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

		totalTraces++
		if len(payload.Report.Hops) > maxHops {
			maxHops = len(payload.Report.Hops)
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
		HopCount:          maxHops,
		UniqueRoutes:      len(routeSignatures),
		RouteStabilityPct: sanitizeFloat(stabilityPct),
		AvgEndHopLatency:  sanitizeFloat(totalEndHopLatency / float64(totalTraces)),
		AvgEndHopLoss:     sanitizeFloat(totalEndHopLoss / float64(totalTraces)),
		RateLimitedHops:   rateLimitedHops,
		TimeoutSegments:   timeoutSegments,
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
	for i, a := range agents {
		agentIDs[i] = a.ID
		agentByID[a.ID] = a
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

	// Fetch PING metrics
	metrics, err := probeAnalysisMetrics(ctx, ch, agentIDs, probeID, from)
	if err != nil {
		log.Warnf("[Analysis] Failed to fetch PING metrics for probe %d: %v", probeID, err)
		metrics = ProbeMetrics{}
	}

	log.Debugf("[Analysis] Probe %d (type=%s): PING samples=%d, avgLat=%.1f, loss=%.2f%%, agentIDs=%v",
		probeID, p.Type, metrics.SampleCount, metrics.AvgLatency, metrics.PacketLoss, agentIDs)

	// Fetch MTR path analysis
	pathAnalysis, mtrSignals, err := analyzeMtrForProbe(ctx, ch, agentIDs, probeID, from)
	if err != nil {
		log.Warnf("[Analysis] Failed to analyze MTR for probe %d: %v", probeID, err)
	}

	// For AGENT probes, also fetch TrafficSim data (same probe_id, different type)
	if p.Type == TypeAgent {
		tsMetrics := probeTrafficSimMetrics(ctx, ch, agentIDs, probeID, from)
		log.Debugf("[Analysis] Probe %d AGENT: TrafficSim samples=%d, avgRTT=%.1f, loss=%.2f%%",
			probeID, tsMetrics.SampleCount, tsMetrics.AvgLatency, tsMetrics.PacketLoss)
		if tsMetrics.SampleCount > 0 {
			// If PING data was empty, use TrafficSim as primary metrics
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

	if metrics.Jitter > 30 {
		signals = append(signals, AnalysisSignal{
			Type:       "jitter_anomaly",
			Severity:   "warning",
			Title:      "High Jitter",
			Evidence:   fmt.Sprintf("Average jitter: %.1fms", metrics.Jitter),
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
					// Found the reverse probe — compute its analysis
					revMetrics, _ := probeAnalysisMetrics(ctx, ch, agentIDs, rp.ID, from)
					revPath, revSignals, _ := analyzeMtrForProbe(ctx, ch, agentIDs, rp.ID, from)
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
		var agentJitter []float64
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
				Jitter:      stats.Jitter,
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
			agentJitter = append(agentJitter, stats.Jitter)
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
		if len(probeEntries) > 0 {
			avgLat := avg(agentLatencies)
			avgLossVal := avg(agentLoss)
			avgJitterVal := avg(agentJitter)

			agentMetrics := ProbeMetrics{
				AvgLatency: avgLat,
				PacketLoss: avgLossVal,
				Jitter:     avgJitterVal,
			}
			agentHealth = computeHealthVector(agentMetrics, 100)
		} else {
			agentHealth = HealthVector{
				Grade:          "unknown",
				RouteStability: 100,
				MosScore:       1.0,
			}
		}

		if !isOnline && agentHealth.Grade != "unknown" {
			// Penalize offline agents
			agentHealth.OverallHealth = math.Max(0, agentHealth.OverallHealth-20)
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
	incidents := detectIncidents(agentSummaries, pingMetrics, mtrMetrics, trafficMetrics, agentByID)

	// ── Temporal Change Detection ──
	changeIncidents := detectTemporalChanges(pingMetrics, baselinePing, trafficMetrics, baselineTraffic, netInfoChanges, sysInfoMetrics, agentByID)
	incidents = append(incidents, changeIncidents...)

	// Build status summary
	status := buildStatusSummary(overallHealth, agentSummaries, incidents)

	// ── Optional LLM Enrichment ──
	if llmProvider != nil && llmProvider.Available() && len(incidents) > 0 {
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
					total += p.Metrics.Jitter
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
) []DetectedIncident {
	var incidents []DetectedIncident

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

			cause := suggestCause(avgLat, avgLoss, len(uniqueAgents), len(agents))
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("shared_target_%s", sanitizeKey(target)),
				Title:           fmt.Sprintf("Shared degradation to %s", stripPort(target)),
				Severity:        severity,
				Scope:           "infrastructure",
				SuggestedCause:  cause,
				AffectedAgents:  uniqueAgents,
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("%d agents affected: %s", len(uniqueAgents), strings.Join(uniqueAgents, ", ")),
					fmt.Sprintf("Avg latency: %.1fms, Avg loss: %.1f%%", avgLat, avgLoss),
					fmt.Sprintf("Detected via: %s", strings.Join(probeTypeList, ", ")),
				},
				Recommendations: suggestRemediation(cause, severity),
			})
		} else if len(uniqueAgents) == 1 && (avgLoss > 3 || avgLat > 200) {
			// Only one agent sees degradation to this target → agent-specific or local ISP
			severity := "warning"
			if avgLoss > 10 || avgLat > 400 {
				severity = "critical"
			}

			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("agent_target_%s_%s", sanitizeKey(uniqueAgents[0]), sanitizeKey(target)),
				Title:           fmt.Sprintf("Degradation from %s to %s", uniqueAgents[0], stripPort(target)),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("Likely local to %s — possible local ISP issue, network congestion, or routing problem specific to this path", uniqueAgents[0]),
				AffectedAgents:  uniqueAgents,
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("Only %s sees this issue (other agents to the same target are unaffected)", uniqueAgents[0]),
					fmt.Sprintf("Avg latency: %.1fms, Avg loss: %.1f%%", avgLat, avgLoss),
				},
				Recommendations: []string{
					fmt.Sprintf("Check the local network at %s for interface errors or congestion", uniqueAgents[0]),
					"Review MTR traces for the specific degraded hops",
					"Compare with other probe destinations from this agent",
				},
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
		})
	}

	return incidents
}

// suggestCause generates a human-readable root cause hypothesis
func suggestCause(avgLatency, avgLoss float64, affectedAgents, totalAgents int) string {
	parts := []string{}

	if affectedAgents >= totalAgents && totalAgents > 1 {
		parts = append(parts, "All agents are affected — likely an issue with the target destination or a shared upstream transit provider")
	} else if affectedAgents > 1 {
		parts = append(parts, fmt.Sprintf("%d of %d agents affected — possible shared peering point, transit provider, or regional network issue", affectedAgents, totalAgents))
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
	AvgDownload float64 // Mbps
	AvgUpload   float64 // Mbps
	AvgLatency  float64 // ms
	AvgJitter   float64 // ms
	Count       int
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
			AvgDownload: (a.dlTotal / float64(a.count)) * 8 / 1_000_000, // bytes/s → Mbps
			AvgUpload:   (a.ulTotal / float64(a.count)) * 8 / 1_000_000,
			AvgLatency:  a.latTotal / float64(a.count),
			AvgJitter:   a.jitterTotal / float64(a.count),
			Count:       a.count,
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
