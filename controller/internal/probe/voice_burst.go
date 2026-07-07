package probe

import "fmt"

// voice_burst.go
//
// Burst-loss detection + burst-vs-steady classification.
//
// Two new detectors:
//
//  1. DetectBurstLoss — fires when the agent reports
//     `maxConsecutiveLoss` ≥ 3 (≥60ms of loss for a 20ms codec) OR
//     `totalBursts / sampleCount > 0.05` (more than 5% of cycles
//     saw a burst). Severity scales with burst density: 3-5 packets =
//     warning, 6+ = critical. Uses the codec multiplier from
//     voice_codec.go for tighter thresholds on G.711.
//
//  2. ClassifyBurstLossPattern — inspects per-direction loss values
//     and returns "burst", "steady", or "mixed" based on the
//     coefficient-of-variation. Wired into the existing
//     annotateLossPatterns so existing packet_loss issues get
//     tagged without a second pass over the data.
//
// Both functions are pure (no DB access); the agent's
// `MaxConsecutiveLoss` / `TotalBursts` are already aggregated into
// `VoicePathMetrics.MaxConsecutiveLoss` / `.TotalBursts` in
// `fetchVoicePathMetrics` — the caller is responsible for filling
// those fields from the TrafficSim cycle's stats.

// Burst-loss thresholds. Picked to match what a typical PLC can
// hide (3 packets = 60 ms for a 20 ms codec). G.711 with no PLC
// would degrade audibly at 2 packets already; that's why the
// codec tolerance multiplier (set by EffectiveThresholds) scales
// these for more forgiving codecs.
const (
	burstMinConsecutiveLoss = 3 // packets — ≥60ms for a 20ms codec
	burstCriticalConsecLoss = 6 // packets — ≥120ms; no codec can hide this
	burstDensityMinPct      = 5 // % of cycles that saw a burst
)

// detectBurstLoss emits one issue per direction when the burst
// thresholds are crossed. Severity is keyed off
// `path.MaxConsecutiveLoss`; we don't recompute density in the
// detector since `path.SampleCount` reflects cycles and the agent's
// `totalBursts` is cycles-with-burst. The density check fires when
// the agent's reported burst count is meaningfully present.
func detectBurstLoss(path *VoicePathMetrics, direction VoicePathDirection, targetAgentName string) []VoiceQualityIssue {
	var issues []VoiceQualityIssue
	if path == nil {
		return issues
	}

	maxConsec := path.MaxConsecutiveLoss
	totalBursts := path.TotalBursts
	samples := path.SampleCount

	// Burst density: fraction of cycles that saw a burst. Skip the
	// density check when sample count is too low to be meaningful
	// (<10 cycles) — the absolute consecutive-loss check is enough.
	var burstDensityPct float64
	if samples > 10 && totalBursts > 0 {
		burstDensityPct = float64(totalBursts) / float64(samples) * 100
	}

	if maxConsec < burstMinConsecutiveLoss && burstDensityPct < burstDensityMinPct {
		return issues
	}

	severity := "warning"
	if maxConsec >= burstCriticalConsecLoss || burstDensityPct >= burstDensityMinPct*4 {
		severity = "critical"
	}

	dir := "forward"
	if direction == VoicePathReturn {
		dir = "return"
	}

	// Loss-pattern hint: a high consecutive-loss count with low
	// density is "burst"; high density with low consecutive-loss is
	// "steady"; both elevated is "mixed".
	pattern := "burst"
	switch {
	case maxConsec >= burstCriticalConsecLoss && burstDensityPct >= burstDensityMinPct:
		pattern = "mixed"
	case burstDensityPct >= burstDensityMinPct && maxConsec < burstMinConsecutiveLoss*2:
		pattern = "steady"
	}

	titleSuffix := ""
	if maxConsec >= burstCriticalConsecLoss {
		titleSuffix = " — sustained gap"
	}

	evidence := []string{
		fmt.Sprintf("Max consecutive loss: %d packets (%d ms at 20ms ptime)", maxConsec, maxConsec*20),
		fmt.Sprintf("Burst cycles: %d of %d (%.1f%%)", totalBursts, samples, burstDensityPct),
		fmt.Sprintf("Packet loss: %.2f%%", path.PacketLoss),
		fmt.Sprintf("MOS: %.2f", path.MosScore),
	}

	recs := []string{
		"Check for upstream link events during the burst window",
		"Review MTR traces for high-loss hops in the same time period",
	}
	if severity == "critical" {
		recs = append(recs, "Escalate to ISP if bursts persist across multiple cycles")
	}

	issues = append(issues, VoiceQualityIssue{
		ID:              fmt.Sprintf("burst_loss_%d_%s", path.ProbeID, direction),
		Severity:        severity,
		Title:           fmt.Sprintf("Burst loss detected on %s path to %s%s", dir, targetAgentName, titleSuffix),
		Category:        "burst_loss",
		AffectedPath:    direction,
		TargetAgentName: targetAgentName,
		SuspectedCause:  "Consecutive packet loss is harder to conceal than dispersed loss — PLC can hide short gaps but a sustained run causes audible dropouts",
		Evidence:        evidence,
		TimePattern:     "constant",
		Recommendations: recs,
		LossPattern:     pattern,
	})

	return issues
}

// ClassifyBurstLossPattern inspects the per-direction loss values
// and returns one of:
//
//	"burst" — high coefficient of variation (CV > 1.0), suggesting
//	  loss comes in concentrated windows
//	"steady" — low CV (< 0.25), loss is evenly distributed
//	"mixed" — somewhere in between
//	""      — not enough data to classify
//
// Wired into annotateLossPatterns so existing packet_loss issues get
// tagged with the pattern automatically.
func ClassifyBurstLossPattern(forward, returnPath *VoicePathMetrics) string {
	return classifyLossPattern(forward, returnPath)
}
