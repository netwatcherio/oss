package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

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
		IncludeTrafficSim: p.Type == TypeAgent || p.Type == TypeTrafficSim,
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
		if reverseProbes, rerr := FindReverseAgentProbes(ctx, pg, p.AgentID); rerr == nil {
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
			IncludeTrafficSim: p.Type == TypeAgent || p.Type == TypeTrafficSim,
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
			// If PING data was empty, use TrafficSim as primary. Take
			// the WHOLE struct — the old field-by-field copy dropped
			// median/p99 latency and the jitter percentiles, which is
			// why the detailed analysis modal showed "—" for those
			// unless the page happened to have local TrafficSim rows
			// to patch from.
			if metrics.SampleCount == 0 {
				metrics = tsMetrics
			} else {
				// Blend: use worse of PING/TrafficSim loss, and fill
				// the percentile fields PING can't provide (per-cycle
				// medians/p95s only exist in TrafficSim payloads).
				if tsMetrics.PacketLoss > metrics.PacketLoss {
					metrics.PacketLoss = tsMetrics.PacketLoss
				}
				if metrics.MedianLatency == 0 {
					metrics.MedianLatency = tsMetrics.MedianLatency
				}
				if metrics.P99Latency == 0 {
					metrics.P99Latency = tsMetrics.P99Latency
				}
				if metrics.JitterMedian == 0 {
					metrics.JitterMedian = tsMetrics.JitterMedian
				}
				if metrics.JitterP95 == 0 {
					metrics.JitterP95 = tsMetrics.JitterP95
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
