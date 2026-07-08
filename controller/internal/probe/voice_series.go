package probe

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// buildVoiceTrends assembles the VoiceTrends struct for the voice
// report. Returns nil if there are no probes or no bucket data so the
// JSON output stays clean.
//
// The Forward and Return series are now populated independently —
// Forward from the agent's own TrafficSim rows, Return from rows the
// remote source agent reports for the matching probe. The Combined
// field is preserved for backward compat (it's the union of both,
// bucketed together).
func buildVoiceTrends(ctx context.Context, ch *sql.DB, agentID uint, probes []Probe, from, to time.Time, thresholds VoiceThresholds) *VoiceTrends {
	// 1h buckets for windows up to 7 days, 4h buckets for longer.
	bucketMin := 60
	if to.Sub(from) > 7*24*time.Hour {
		bucketMin = 240
	}
	forward, reverse := fetchVoicePathSeriesSplit(ctx, ch, agentID, probes, from, to, bucketMin)
	if len(forward) == 0 && len(reverse) == 0 {
		return nil
	}

	// Combined series = union by bucket (used by the legacy single-line
	// chart and as a fallback for callers that don't care about
	// direction).
	combined := mergeBuckets(forward, reverse)

	// Issue markers: the timestamps whose MOS is below
	// (PoorMos - 0.1) — useful for the timeline chart overlay.
	issueCutoff := thresholds.PoorMos - 0.1
	issueBuckets := make([]string, 0, len(combined))
	for _, b := range combined {
		if b.Forward > 0 && b.Forward < issueCutoff {
			issueBuckets = append(issueBuckets, b.Timestamp)
		}
	}

	return &VoiceTrends{
		BucketMinutes: bucketMin,
		Forward:       forward,
		Return:        reverse,
		Combined:      combined,
		IssueBuckets:  issueBuckets,
	}
}

// mergeBuckets unions two parallel bucket series into one, averaging
// the Forward field across both directions at matching timestamps.
// Used by buildVoiceTrends to produce the Combined series.
func mergeBuckets(forward, reverse []VoiceBucket) []VoiceBucket {
	if len(forward) == 0 {
		return reverse
	}
	if len(reverse) == 0 {
		return forward
	}
	byKey := make(map[string]VoiceBucket, len(forward)+len(reverse))
	for _, b := range forward {
		byKey[b.Timestamp] = b
	}
	for _, b := range reverse {
		if existing, ok := byKey[b.Timestamp]; ok {
			merged := existing
			if existing.Forward == 0 {
				merged.Forward = b.Forward
			} else if b.Forward != 0 {
				merged.Forward = (existing.Forward + b.Forward) / 2
			}
			merged.Return = b.Forward
			byKey[b.Timestamp] = merged
		} else {
			byKey[b.Timestamp] = b
		}
	}
	out := make([]VoiceBucket, 0, len(byKey))
	for _, b := range byKey {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}

// fetchVoicePathSeriesPatterns returns a one-word time pattern
// classification derived from the per-bucket series ("business_hours",
// "off_hours", "periodic_30min", "constant", "improving", "worsening",
// "stable", or "" if no data).
//
// Used by ComputeAgentVoiceQuality to enrich the legacy pattern-from-
// issues heuristic. The chart renderer in reports/charts.go also
// reuses the per-bucket data via the VoiceTrends field on the summary.
func fetchVoicePathSeriesPatterns(ctx context.Context, ch *sql.DB, agentID uint, probes []Probe, from, to time.Time) string {
	buckets := fetchVoicePathSeries(ctx, ch, agentID, probes, from, to, 60) // 1h buckets
	if len(buckets) < 3 {
		return ""
	}
	return classifyTimePattern(buckets)
}

// fetchVoicePathSeries pulls a 1-bucket-per-interval MOS time series
// for the agent's voice-relevant probes. Used by the report timeline
// chart and the time-of-day pattern detector.
//
// Returns an empty slice (not nil error) when there is no data so
// callers can degrade gracefully.
func fetchVoicePathSeries(ctx context.Context, ch *sql.DB, agentID uint, probes []Probe, from, to time.Time, bucketMinutes int) []VoiceBucket {
	forward, _ := fetchVoicePathSeriesSplit(ctx, ch, agentID, probes, from, to, bucketMinutes)
	return forward
}

// fetchVoicePathSeriesSplit pulls per-direction MOS series from the
// TRAFFICSIM rows in `probe_data`. Forward rows are reported by the
// agent itself; reverse rows are reported by the far-end agent under
// the same probe ID (bidirectional AGENT probes attribute the return
// stream to the client's probe ID with the server's agent_id).
//
// Used by buildVoiceTrends to populate VoiceTrends.Forward and
// VoiceTrends.Return independently. Returns empty slices (not errors)
// when no data is available so callers can degrade gracefully.
//
// Direction is inferred from the row's reporting `agent_id`: rows
// reported by `agentID` itself are forward; rows another agent
// reported on our probe IDs are the return direction.
func fetchVoicePathSeriesSplit(ctx context.Context, ch *sql.DB, agentID uint, probes []Probe, from, to time.Time, bucketMinutes int) (forward, reverse []VoiceBucket) {
	if len(probes) == 0 {
		return nil, nil
	}

	probeIDs := make([]uint, 0, len(probes))
	for _, p := range probes {
		if p.ID != 0 {
			probeIDs = append(probeIDs, p.ID)
		}
	}
	if len(probeIDs) == 0 {
		return nil, nil
	}
	pidList := make([]string, len(probeIDs))
	for i, id := range probeIDs {
		pidList[i] = fmt.Sprintf("%d", id)
	}

	q := fmt.Sprintf(`
SELECT
    toStartOfInterval(created_at, INTERVAL %d MINUTE) as bucket,
    agent_id,
    avgIf(
        greatest(JSONExtractFloat(payload_raw, 'MOS'), JSONExtractFloat(payload_raw, 'mos')),
        greatest(JSONExtractFloat(payload_raw, 'MOS'), JSONExtractFloat(payload_raw, 'mos')) > 0) as mos_avg,
    avg(JSONExtractFloat(payload_raw, 'averageRTT'))/2.0 as lat_avg,
    avg(JSONExtractFloat(payload_raw, 'jitterAvg')) as jit_avg,
    avg(JSONExtractFloat(payload_raw, 'lossPercentage')) as loss_avg
FROM probe_data
WHERE type = 'TRAFFICSIM'
  AND probe_id IN (%s)
  AND created_at >= %s
  AND created_at <= %s
GROUP BY bucket, agent_id
ORDER BY bucket ASC
`, bucketMinutes, strings.Join(pidList, ","), chQuoteTime(from), chQuoteTime(to))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		log.Warnf("[voice] series query failed: %v", err)
		return nil, nil
	}
	defer rows.Close()

	forwardByBucket := make(map[string]VoiceBucket)
	reverseByBucket := make(map[string]VoiceBucket)

	for rows.Next() {
		var (
			bucket      time.Time
			reporterAID uint64
			mos         sql.NullFloat64
			lat         sql.NullFloat64
			jit         sql.NullFloat64
			loss        sql.NullFloat64
		)
		if err := rows.Scan(&bucket, &reporterAID, &mos, &lat, &jit, &loss); err != nil {
			continue
		}
		mosVal := mos.Float64
		if !mos.Valid || mosVal <= 0 {
			// Non-VoIP-mode cycles carry no MOS — derive it from the
			// bucket's latency/loss/jitter via the E-model.
			mosVal = computeMos(lat.Float64, loss.Float64, jit.Float64)
		}
		b := VoiceBucket{
			Timestamp: bucket.UTC().Format("2006-01-02T15:04:05Z"),
			Forward:   mosVal,
			Return:    mosVal,
			LatencyMs: lat.Float64,
			JitterMs:  jit.Float64,
			LossPct:   loss.Float64,
		}
		if uint(reporterAID) == agentID {
			forwardByBucket[b.Timestamp] = b
		} else {
			// The far-end agent reported on our probe — that's the
			// return direction of the bidirectional session.
			reverseByBucket[b.Timestamp] = b
		}
	}

	forward = bucketMapToSlice(forwardByBucket)
	reverse = bucketMapToSlice(reverseByBucket)
	return forward, reverse
}

func bucketMapToSlice(m map[string]VoiceBucket) []VoiceBucket {
	if len(m) == 0 {
		return nil
	}
	out := make([]VoiceBucket, 0, len(m))
	for _, b := range m {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}

// classifyTimePattern inspects a MOS series and returns a one-word
// pattern classification. Returns "" if the series is too short to
// classify.
//
// Logic (in order):
//  1. < 3 buckets → ""
//  2. monotone-with-noise (improving or worsening) if the start-vs-end
//     delta dominates the standard deviation
//  3. periodic with ~30 min period (proxy for business-hours-style
//     bursts) if there are at least two clear troughs spaced ~30 min
//     apart and the trough MOS < mean MOS
//  4. constant / stable otherwise
func classifyTimePattern(buckets []VoiceBucket) string {
	if len(buckets) < 3 {
		return ""
	}
	values := make([]float64, len(buckets))
	for i, b := range buckets {
		values[i] = b.Forward
	}
	start, end := values[0], values[len(values)-1]
	delta := end - start

	mean, stddev := meanStddev(values)
	_ = mean
	if stddev < 0.05 {
		return "stable"
	}
	if delta > stddev*1.5 {
		return "improving"
	}
	if -delta > stddev*1.5 {
		return "worsening"
	}
	if hasPeriodicTroughs(values) {
		return "periodic_30min"
	}
	return "constant"
}

// hasPeriodicTroughs returns true if the series shows at least two
// local minima spaced roughly evenly through the series. Used as a
// proxy for "recurring degradation" — e.g., a 30-minute cron job
// flooding the network every half hour.
func hasPeriodicTroughs(values []float64) bool {
	if len(values) < 5 {
		return false
	}
	meanSum := 0.0
	for _, v := range values {
		meanSum += v
	}
	mean := meanSum / float64(len(values))

	var troughCount int
	for i := 1; i < len(values)-1; i++ {
		if values[i] < values[i-1] && values[i] < values[i+1] && values[i] < mean {
			troughCount++
		}
	}
	// Heuristic: >= 2 troughs AND trough count is at least 25% of buckets.
	return troughCount >= 2 && troughCount*4 >= len(values)
}

func meanStddev(values []float64) (mean, stddev float64) {
	if len(values) == 0 {
		return 0, 0
	}
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	for _, v := range values {
		d := v - mean
		stddev += d * d
	}
	stddev = sqrtFloat(stddev / float64(len(values)))
	return
}

func sqrtFloat(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method is overkill — math.Sqrt is in stdlib but we keep
	// the dep surface small by handling it locally. Three Newton
	// iterations is more than enough for our use case.
	z := x / 2
	for i := 0; i < 8; i++ {
		z = (z + x/z) / 2
	}
	return z
}
