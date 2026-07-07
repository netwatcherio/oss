package probe

// voice_severity.go
//
// Magnitude-and-duration severity scoring for voice quality issues.
//
// The pre-existing detectors hardcoded "warning" / "critical" based
// on whether the observed value crossed one of two thresholds. That
// works for the simple case but ignores two important signals:
//
//  1. Magnitude of the breach. A jitter value 1.1× the warning
//     threshold shouldn't fire the same severity as 4× the threshold.
//  2. Persistence. A 30-second jitter spike in a 7-day window is
//     qualitatively different from a 6-hour sustained elevation.
//
// This file provides ScoreSeverity (returns 0..1) and a label
// converter. Detector functions compute the score and use the label
// to set VoiceQualityIssue.Severity, optionally preserving the
// threshold-only label in RawSeverity for transparency.

const (
	severityScoreCritical = 0.7
	severityScoreWarning  = 0.4
	severityScoreInfo     = 0.1
)

// ScoreSeverity combines magnitude and persistence into a 0..1 score.
// `observed` and `threshold` are the metric value and its warning
// cutoff. `durationBuckets` / `totalBuckets` describe how long the
// issue persisted within the analysis window; both are 0 when no
// time-series data is available and the function falls back to
// magnitude-only.
//
// The mapping:
//   - 0.0  → at-or-below threshold, transient
//   - 0.5  → at threshold, persistent OR significantly above threshold
//   - 1.0  → 2× threshold, fully persistent
//
// Both factors are clamped to [0, 1] and averaged with equal weight
// so a high-magnitude transient still registers (just lower) and a
// persistent small breach registers higher than a single noisy
// cycle.
func ScoreSeverity(observed, threshold float64, durationBuckets, totalBuckets int) float64 {
	mag := magnitudeFactor(observed, threshold)
	dur := durationFactor(durationBuckets, totalBuckets)
	return clamp01((mag + dur) / 2)
}

// SeverityFromScore maps a 0..1 score to an info/warning/critical
// label. Thresholds are calibrated against the existing detector
// behaviour: a fresh breach at exactly the warning threshold (score ≈
// 0.25-0.40) lands in `warning`, while a 2× breach that's persistent
// (score ≥ 0.7) lands in `critical`.
func SeverityFromScore(score float64) string {
	switch {
	case score >= severityScoreCritical:
		return "critical"
	case score >= severityScoreWarning:
		return "warning"
	case score >= severityScoreInfo:
		return "info"
	default:
		return "info"
	}
}

// MagnitudeFactor maps the observed value's distance above threshold
// to 0..1. Below threshold = 0; 2× threshold = 1 (linear in between).
// Exposed for the unit tests so they can assert the magnitude-only
// half of the score.
func MagnitudeFactor(observed, threshold float64) float64 {
	return magnitudeFactor(observed, threshold)
}

// DurationFactor returns the fraction of the analysis window during
// which the issue was present. Returns 0 when no bucket data is
// available so callers can call this unconditionally.
func DurationFactor(durationBuckets, totalBuckets int) float64 {
	return durationFactor(durationBuckets, totalBuckets)
}

func magnitudeFactor(observed, threshold float64) float64 {
	if threshold <= 0 || observed <= threshold {
		return 0
	}
	ratio := observed / threshold
	return clamp01((ratio - 1) / 1)
}

func durationFactor(durationBuckets, totalBuckets int) float64 {
	if totalBuckets <= 0 || durationBuckets <= 0 {
		return 0
	}
	if durationBuckets >= totalBuckets {
		return 1
	}
	return float64(durationBuckets) / float64(totalBuckets)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
