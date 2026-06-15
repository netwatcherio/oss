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

// buildVoiceTrends assembles the VoiceTrends struct for the voice
// report. Returns nil if there are no probes or no bucket data so the
// JSON output stays clean.
func buildVoiceTrends(ctx context.Context, ch *sql.DB, agentID uint, probes []Probe, from, to time.Time, thresholds VoiceThresholds) *VoiceTrends {
	// 1h buckets for windows up to 7 days, 4h buckets for longer.
	bucketMin := 60
	if to.Sub(from) > 7*24*time.Hour {
		bucketMin = 240
	}
	buckets := fetchVoicePathSeries(ctx, ch, agentID, probes, from, to, bucketMin)
	if len(buckets) == 0 {
		return nil
	}

	// Issue markers: the timestamps whose MOS is below
	// (PoorMos - 0.1) — useful for the timeline chart overlay.
	issueCutoff := thresholds.PoorMos - 0.1
	issueBuckets := make([]string, 0, len(buckets))
	for _, b := range buckets {
		if b.Forward > 0 && b.Forward < issueCutoff {
			issueBuckets = append(issueBuckets, b.Timestamp)
		}
	}

	return &VoiceTrends{
		BucketMinutes: bucketMin,
		Forward:       buckets,
		Return:        buckets, // single series today; phase 2 returns will re-split
		Combined:      buckets,
		IssueBuckets:  issueBuckets,
	}
}

// fetchVoicePathSeries pulls a 1-bucket-per-interval MOS time series
// for the agent's voice-relevant probes. Used by the report timeline
// chart and the time-of-day pattern detector.
//
// Returns an empty slice (not nil error) when there is no data so
// callers can degrade gracefully.
func fetchVoicePathSeries(ctx context.Context, ch *sql.DB, agentID uint, probes []Probe, from, to time.Time, bucketMinutes int) []VoiceBucket {
	if len(probes) == 0 {
		return nil
	}

	// Build a list of probe IDs to query.
	probeIDs := make([]uint, 0, len(probes))
	for _, p := range probes {
		if p.ID != 0 {
			probeIDs = append(probeIDs, p.ID)
		}
	}
	if len(probeIDs) == 0 {
		return nil
	}
	pidList := make([]string, len(probeIDs))
	for i, id := range probeIDs {
		pidList[i] = fmt.Sprintf("%d", id)
	}

	q := fmt.Sprintf(`
SELECT 
    toStartOfInterval(created_at, INTERVAL %d MINUTE) as bucket,
    avg(mos_score) as mos_avg,
    avg(average_rtt)/2.0 as lat_avg,
    avg(jitter_avg) as jit_avg,
    avg(loss_pct) as loss_avg
FROM traffic_metrics
WHERE agent_id = %d
  AND probe_id IN (%s)
  AND created_at >= %s
  AND created_at <= %s
GROUP BY bucket
ORDER BY bucket ASC
`, bucketMinutes, agentID, strings.Join(pidList, ","), chQuoteTime(from), chQuoteTime(to))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		log.Warnf("[voice] series query failed: %v", err)
		return nil
	}
	defer rows.Close()

	var out []VoiceBucket
	for rows.Next() {
		var (
			bucket time.Time
			mos    sql.NullFloat64
			lat    sql.NullFloat64
			jit    sql.NullFloat64
			loss   sql.NullFloat64
		)
		_ = lat // column present in SELECT for future per-direction split
		if err := rows.Scan(&bucket, &mos, &lat, &jit, &loss); err != nil {
			continue
		}
		if !mos.Valid {
			continue
		}
		out = append(out, VoiceBucket{
			Timestamp: bucket.UTC().Format("2006-01-02T15:04:05Z"),
			Forward:   mos.Float64,
			Return:    mos.Float64, // symmetric for now; future phase: split by direction
			LatencyMs: lat.Float64,
			JitterMs:  jit.Float64,
			LossPct:   loss.Float64,
		})
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
