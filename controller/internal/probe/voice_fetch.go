package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// fetchVoicePathMetrics fetches voice path metrics for a given probe
// from the TRAFFICSIM rows in `probe_data`. The agent reports each
// cycle's stats as a JSON payload (RTT/jitter in ms; MOS/RFactor only
// in VoIP mode, under capitalized "MOS"/"RFactor" keys — aggregated
// read-path rows use lowercase "mos"), so the rollup extracts fields
// server-side with JSONExtract.
//
// Bidirectional attribution: forward and reverse rows share the probe
// ID and differ only by agent_id — the client (probe owner) reports
// forward rows, the far-end server reports reverse rows under the
// client's probe ID. Callers pick the direction via `agentIDs`.
func fetchVoicePathMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, probeID uint, from time.Time) (*VoicePathMetrics, error) {
	if len(agentIDs) == 0 {
		return nil, nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	// The agent emits "outOfOrder" (raw cycles) while the aggregated
	// read path emits "outOfSequence" — extract both. Same for
	// "MOS"/"mos". Loss is recomputed from packet counters when
	// present so long windows aren't skewed by uneven cycle sizes.
	q := fmt.Sprintf(`
SELECT
    avg(JSONExtractFloat(payload_raw, 'averageRTT'))    as avg_rtt,
    avg(JSONExtractFloat(payload_raw, 'medianRTT'))     as median_rtt,
    avg(JSONExtractFloat(payload_raw, 'p95RTT'))        as p95_rtt,
    avg(JSONExtractFloat(payload_raw, 'jitterAvg'))     as jitter_avg,
    avg(JSONExtractFloat(payload_raw, 'jitterMedian'))  as jitter_median,
    avg(JSONExtractFloat(payload_raw, 'jitterP95'))     as jitter_p95,
    sum(JSONExtractUInt(payload_raw, 'lostPackets'))    as lost_packets,
    sum(JSONExtractUInt(payload_raw, 'totalPackets'))   as total_packets,
    avg(JSONExtractFloat(payload_raw, 'lossPercentage')) as loss_pct_avg,
    avg(greatest(
        JSONExtractFloat(payload_raw, 'outOfOrderPercent'),
        JSONExtractFloat(payload_raw, 'outOfSequencePercent'))) as out_of_seq_pct,
    avg(JSONExtractFloat(payload_raw, 'duplicatePercent')) as duplicate_pct,
    avgIf(
        greatest(JSONExtractFloat(payload_raw, 'MOS'), JSONExtractFloat(payload_raw, 'mos')),
        greatest(JSONExtractFloat(payload_raw, 'MOS'), JSONExtractFloat(payload_raw, 'mos')) > 0) as mos_score,
    count(*) as sample_count,
    max(JSONExtractUInt(payload_raw, 'maxConsecutiveLoss')) as max_consecutive_loss,
    sum(JSONExtractUInt(payload_raw, 'totalBursts'))    as total_bursts
FROM probe_data
WHERE type = 'TRAFFICSIM'
  AND agent_id IN (%s)
  AND probe_id = %d
  AND created_at >= %s
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
	var avgRtt, medianRtt, p95Rtt, jitterAvg, jitterMedian, jitterP95, lossPctAvg, outOfSeqPct, duplicatePct float64
	var mosScore sql.NullFloat64
	var lostPackets, totalPackets uint64
	var sampleCount int
	var maxConsecutiveLoss, totalBursts uint64

	if err := rows.Scan(&avgRtt, &medianRtt, &p95Rtt, &jitterAvg, &jitterMedian, &jitterP95, &lostPackets, &totalPackets, &lossPctAvg, &outOfSeqPct, &duplicatePct, &mosScore, &sampleCount, &maxConsecutiveLoss, &totalBursts); err != nil {
		return nil, err
	}
	if sampleCount == 0 {
		return nil, nil
	}

	// Convert RTT to one-way latency estimate (divide by 2)
	m.AvgLatency = avgRtt / 2
	m.MedianLatency = medianRtt / 2
	m.P95Latency = p95Rtt / 2
	m.JitterAvg = jitterAvg
	m.JitterMedian = jitterMedian
	m.JitterP95 = jitterP95
	if totalPackets > 0 {
		m.PacketLoss = float64(lostPackets) / float64(totalPackets) * 100.0
	} else {
		m.PacketLoss = lossPctAvg
	}
	m.OutOfSequence = outOfSeqPct
	m.Duplicates = duplicatePct
	if mosScore.Valid && mosScore.Float64 > 0 {
		m.MosScore = mosScore.Float64
	} else {
		// Non-VoIP-mode cycles don't carry MOS — derive it via the
		// E-model, same as the PING path does.
		m.MosScore = computeMos(m.AvgLatency, m.PacketLoss, m.JitterAvg)
	}
	m.SampleCount = sampleCount
	m.MaxConsecutiveLoss = int(maxConsecutiveLoss)
	m.TotalBursts = int(totalBursts)
	m.MosContributors = mosContributingFactors(m.AvgLatency, m.P95Latency, m.JitterAvg, m.PacketLoss)
	m.CongestionLevel = congestionLevelFromMetrics(m.JitterAvg, m.PacketLoss, m.AvgLatency)

	return &m, nil
}

// fetchPingVoiceMetrics is the PING-derived voice quality path. When
// an agent has no TrafficSim data (no VoIP server enabled, or the
// AGENT probe didn't expand to a TRAFFICSIM child because the target
// doesn't run one), the engine falls back to computing MOS from the
// PING / RTT samples via the E-model.
//
// This unlocks the per-agent / per-workspace voice report for agents
// whose only cross-agent monitoring is AGENT probes. Without this
// path, such agents rendered an empty-data fallback (MOS 4.5
// "excellent" defaults) and the report showed no pairs / no issues,
// even though ping RTT and jitter were plenty of signal.
//
// The numbers we have from each PING cycle are:
//
//	avg_rtt, std_dev_rtt (ns) → latency / jitter (ms)
//	packet_loss                → loss%
//
// We compute MOS via the simplified E-model (G.107) as computeMos
// does. R-Factor isn't available server-side without the agent's
// contribution, so we derive it from MOS via the G.107 reverse
// formula (just enough to grade the report — the operator's eye is
// on the trend and per-issue callouts, not the R-Factor digits).
func fetchPingVoiceMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, probeID uint, from time.Time) (*VoicePathMetrics, error) {
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
		return nil, err
	}
	defer rows.Close()

	var latencies, jitters []float64
	var totalLoss float64
	var count int
	var maxBursts uint64
	var maxConsLoss uint64

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
			AvgRTT             int64   `json:"avg_rtt"`
			StdDevRTT          int64   `json:"std_dev_rtt"`
			PacketLoss         float64 `json:"packet_loss"`
			MaxConsecutiveLoss uint64  `json:"max_consecutive_loss,omitempty"`
			TotalBursts        uint64  `json:"total_bursts,omitempty"`
		}
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}

		latMs := float64(payload.AvgRTT) / 1_000_000.0
		jitMs := float64(payload.StdDevRTT) / 1_000_000.0
		if latMs > 0 {
			latencies = append(latencies, latMs)
		}
		if jitMs > 0 {
			jitters = append(jitters, jitMs)
		}
		totalLoss += payload.PacketLoss
		if payload.MaxConsecutiveLoss > maxConsLoss {
			maxConsLoss = payload.MaxConsecutiveLoss
		}
		maxBursts += payload.TotalBursts
		count++
	}

	if count == 0 {
		return nil, nil
	}

	avgLat := avg(latencies)
	p95Lat := percentile(latencies, 95)
	avgJit := avg(jitters)
	p95Jit := percentile(jitters, 95)
	avgLoss := totalLoss / float64(count)

	// computeMos expects one-way latency; PING avg_rtt is round-trip.
	oneWayLat := avgLat / 2.0
	mos := computeMos(oneWayLat, avgLoss, avgJit)

	return &VoicePathMetrics{
		MosScore:           mos,
		AvgLatency:         oneWayLat,
		P95Latency:         p95Lat / 2.0,
		MedianLatency:      avg(latencies) / 2.0,
		JitterAvg:          avgJit,
		JitterMedian:       median(jitters),
		JitterP95:          p95Jit,
		PacketLoss:         avgLoss,
		SampleCount:        count,
		MosContributors:    mosContributingFactors(oneWayLat, p95Lat/2.0, avgJit, avgLoss),
		CongestionLevel:    congestionLevelFromMetrics(avgJit, avgLoss, oneWayLat),
		MaxConsecutiveLoss: int(maxConsLoss),
		TotalBursts:        int(maxBursts),
		ProbeID:            probeID,
	}, nil
}

// fetchMtrVoiceMetrics derives voice-path metrics from MTR traces —
// the last-resort fallback when a probe has neither TRAFFICSIM nor
// PING data. AGENT probes always expand an MTR child (PING is
// opt-in, TRAFFICSIM needs a server on the target), so for a
// server-less target MTR is often the only per-cycle data in either
// direction.
//
// The final responding hop's avg/stddev/loss_pct approximate
// RTT / jitter / end-to-end loss; MOS comes from the E-model like
// the PING path. When the true target doesn't answer ICMP the last
// responding hop is an upstream router, so treat these numbers as
// path health rather than endpoint truth — still far better signal
// than an empty report.
func fetchMtrVoiceMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, probeID uint, from time.Time) (*VoicePathMetrics, error) {
	if len(agentIDs) == 0 {
		return nil, nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}

	q := fmt.Sprintf(`
SELECT payload_raw
FROM probe_data
WHERE type = 'MTR'
  AND probe_id = %d
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 2000
`, probeID, strings.Join(agentIDStrs, ", "), chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rtts, jitters []float64
	var totalLoss float64
	var count int

	for rows.Next() {
		var payloadRaw string
		if err := rows.Scan(&payloadRaw); err != nil || payloadRaw == "" {
			continue
		}
		rtt, jitter, loss, ok := mtrVoiceSample(payloadRaw)
		if !ok {
			continue
		}
		rtts = append(rtts, rtt)
		if jitter > 0 {
			jitters = append(jitters, jitter)
		}
		totalLoss += loss
		count++
	}

	if count == 0 {
		return nil, nil
	}

	avgRtt := avg(rtts)
	p95Rtt := percentile(rtts, 95)
	avgJit := avg(jitters)
	avgLoss := totalLoss / float64(count)

	// computeMos expects one-way latency; MTR hop avg is round-trip.
	oneWayLat := avgRtt / 2.0
	mos := computeMos(oneWayLat, avgLoss, avgJit)

	return &VoicePathMetrics{
		MosScore:        mos,
		AvgLatency:      oneWayLat,
		P95Latency:      p95Rtt / 2.0,
		MedianLatency:   median(rtts) / 2.0,
		JitterAvg:       avgJit,
		JitterMedian:    median(jitters),
		JitterP95:       percentile(jitters, 95),
		PacketLoss:      avgLoss,
		SampleCount:     count,
		MosContributors: mosContributingFactors(oneWayLat, p95Rtt/2.0, avgJit, avgLoss),
		CongestionLevel: congestionLevelFromMetrics(avgJit, avgLoss, oneWayLat),
		ProbeID:         probeID,
	}, nil
}

// mtrVoiceSample extracts one (rttMs, jitterMs, lossPct) sample from
// an MTR payload's final responding hop. Returns ok=false when the
// trace is empty, unparseable, or no hop responded.
func mtrVoiceSample(payloadRaw string) (rtt, jitter, loss float64, ok bool) {
	var payload mtrPayload
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		return 0, 0, 0, false
	}
	hops := payload.Report.Hops
	for i := len(hops) - 1; i >= 0; i-- {
		h := hops[i]
		if len(h.Hosts) == 0 || h.Hosts[0].IP == "" || h.Hosts[0].IP == "*" {
			continue
		}
		rtt = parseFloat(h.Avg)
		if rtt <= 0 {
			continue // hop responded with no timing — keep walking back
		}
		return rtt, parseFloat(h.StdDev), parseFloat(h.LossPct), true
	}
	return 0, 0, 0, false
}

// fetchFallbackVoiceMetrics tries the non-TrafficSim sources in
// preference order: PING, then MTR. Used when a probe/target has no
// TRAFFICSIM data — per the data-source policy, AGENT/TrafficSim
// always wins and derived metrics only fill gaps.
func fetchFallbackVoiceMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, probeID uint, from time.Time) (*VoicePathMetrics, error) {
	metrics, err := fetchPingVoiceMetrics(ctx, ch, agentIDs, probeID, from)
	if err == nil && metrics != nil && metrics.SampleCount > 0 {
		return metrics, nil
	}
	return fetchMtrVoiceMetrics(ctx, ch, agentIDs, probeID, from)
}

// fillVoiceBaselines fetches 7-day-baseline metrics for any probe
// present in `current` but missing from `baselines`. Tries the
// TrafficSim rollup first, then the PING/MTR-derived paths,
// mirroring the fetch order used for the current-window metrics, so
// the baseline compares like with like.
func fillVoiceBaselines(ctx context.Context, ch *sql.DB, baselines map[uint]*VoicePathMetrics, current map[uint]*VoicePathMetrics, baselineFrom time.Time) {
	for probeID, m := range current {
		if m == nil {
			continue
		}
		if _, ok := baselines[probeID]; ok {
			continue
		}
		bm, err := fetchVoicePathMetrics(ctx, ch, []uint{m.SourceAgentID}, probeID, baselineFrom)
		if err != nil || bm == nil || bm.SampleCount == 0 {
			bm, err = fetchFallbackVoiceMetrics(ctx, ch, []uint{m.SourceAgentID}, probeID, baselineFrom)
		}
		if err == nil && bm != nil && bm.SampleCount > 0 {
			baselines[probeID] = bm
		}
	}
}
