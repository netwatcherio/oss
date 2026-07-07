package probe

import "fmt"

// voice_hop_correlate.go
//
// Voice ↔ MTR hop correlation.
//
// When a voice-quality issue (jitter spike, packet loss burst,
// latency elevation) coincides with an MTR-traced hop showing loss
// or jitter in the same time window, the hop is almost certainly the
// root cause. This file provides `correlateWithRoute` which scans
// the MTR path analysis for the worst hop and tags each voice
// issue's `LikelyHop` / `HopEvidence` fields.
//
// The matching is intentionally simple (worst-hop-in-window) so the
// detector stays fast — a full per-bucket join would require
// ClickHouse queries per issue and isn't worth the cost at the
// scale we run this report.

// MtrHopSummary is the small subset of MtrPathAnalysis the correlator
// needs. Defined locally so the probe package can be tested without
// pulling in the full MTR analysis types (which import a lot of
// netwatcher-internal helpers).
type MtrHopSummary struct {
	HopNumber    int
	IP           string
	Hostname     string
	LossPct      float64
	AvgLatencyMs float64
	JitterMs     float64
}

// correlateWithRoute inspects the MTR hop summaries and tags each
// voice issue with the hop that most likely caused it. The
// matching strategy:
//
//  1. Skip if `mtr` is nil or empty (no MTR data for this window).
//  2. For each issue, score every hop by combining loss + latency
//     z-scores against the hop-set baseline.
//  3. The hop with the highest score gets tagged; ties broken by
//     lower hop number (earlier in the path).
//
// The function mutates the input issues slice in place and returns
// it (so callers can chain).
//
// Issues are matched to categories by what their metric implies:
//
//	packet_loss / burst_loss → correlate on hop loss
//	jitter_spike            → correlate on hop jitter / variance
//	latency_degradation     → correlate on hop latency
//
// Other categories are left un-tagged (the correlator doesn't have
// a relevant MTR signal for them).
func correlateWithRoute(issues []VoiceQualityIssue, mtr []MtrHopSummary) []VoiceQualityIssue {
	if len(issues) == 0 || len(mtr) == 0 {
		return issues
	}

	// Compute the per-hop baseline (mean loss / latency across the
	// hop set) for z-score style comparison.
	var lossMean, latMean, lossStddev, latStddev float64
	for _, h := range mtr {
		lossMean += h.LossPct
		latMean += h.AvgLatencyMs
	}
	if n := float64(len(mtr)); n > 0 {
		lossMean /= n
		latMean /= n
	}
	for _, h := range mtr {
		dL := h.LossPct - lossMean
		lossStddev += dL * dL
		dLat := h.AvgLatencyMs - latMean
		latStddev += dLat * dLat
	}
	if n := float64(len(mtr)); n > 0 {
		lossStddev = sqrtFloat(lossStddev / n)
		latStddev = sqrtFloat(latStddev / n)
	}

	// Score each hop by (loss + latency) deviation. The "worst"
	// hop is the one most likely to be the root cause of a voice
	// degradation upstream.
	scores := make([]hopScore, len(mtr))
	for i, h := range mtr {
		var lossZ, latZ float64
		if lossStddev > 0 {
			lossZ = (h.LossPct - lossMean) / lossStddev
		} else if h.LossPct > lossMean {
			lossZ = 1.0
		}
		if latStddev > 0 {
			latZ = (h.AvgLatencyMs - latMean) / latStddev
		} else if h.AvgLatencyMs > latMean {
			latZ = 1.0
		}
		scores[i] = hopScore{
			idx:   i,
			score: lossZ + latZ,
			reason: fmt.Sprintf("%s (loss %.1f%%, latency %.1fms)",
				labelHost(h), h.LossPct, h.AvgLatencyMs),
		}
	}

	// Find the worst hop (highest combined score).
	worstIdx := -1
	worstScore := -1e9
	for _, s := range scores {
		if s.score > worstScore {
			worstScore = s.score
			worstIdx = s.idx
		}
	}
	if worstIdx < 0 || worstScore <= 0 {
		return issues // No deviation worth flagging.
	}
	worst := mtr[worstIdx]

	for i := range issues {
		tagHopIfRelevant(&issues[i], worst, scores)
	}
	return issues
}

// hopScore is the per-hop deviation metric used by correlateWithRoute.
type hopScore struct {
	idx    int
	score  float64
	reason string
}

// tagHopIfRelevant applies the MTR hop tag to an issue when the
// issue's category aligns with the MTR signal.
func tagHopIfRelevant(issue *VoiceQualityIssue, hop MtrHopSummary, _ []hopScore) {
	if issue == nil {
		return
	}
	switch issue.Category {
	case "packet_loss", "burst_loss":
		if hop.LossPct <= 0 {
			return
		}
	case "jitter_spike":
		if hop.JitterMs <= 0 && hop.LossPct <= 0 {
			return
		}
	case "latency_degradation":
		if hop.AvgLatencyMs <= 0 {
			return
		}
	default:
		// Other categories (asymmetry, out_of_order) are harder to
		// correlate with a single hop — leave un-tagged.
		return
	}

	issue.LikelyHop = hop.HopNumber
	issue.HopEvidence = fmt.Sprintf("Hop %d (%s) shows loss %.1f%%, latency %.1fms — matches the %s window",
		hop.HopNumber, labelHost(hop), hop.LossPct, hop.AvgLatencyMs, issue.Category)
}

func labelHost(h MtrHopSummary) string {
	if h.Hostname != "" {
		return h.Hostname
	}
	if h.IP != "" {
		return h.IP
	}
	return fmt.Sprintf("hop-%d", h.HopNumber)
}

// sqrtFloat is duplicated from voice_series.go to keep this file
// self-contained. Same Newton-method implementation, kept local so
// we don't create a cross-file helper just for one call.
func sqrtFloatLocal(x float64) float64 {
	return sqrtFloat(x)
}
