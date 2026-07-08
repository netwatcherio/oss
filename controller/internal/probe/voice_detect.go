package probe

import "fmt"

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
//
// Single-pair wrapper kept for backward compat with callers that
// already have a specific forward/return pair in hand. New code that
// has the full map of probe-level metrics should call
// `detectVoiceQualityIssuesPerPair` directly so that the per-pair
// issue list is preserved (rather than collapsed onto the
// worst-offender).
func detectVoiceQualityIssues(forward, returnPath *VoicePathMetrics, baselineByProbeID, reverseBaselineByProbeID map[uint]*VoicePathMetrics, targetAgentName string, thresholds *VoiceThresholds) []VoiceQualityIssue {
	t := VoiceDefaultThresholds
	if thresholds != nil {
		t = *thresholds
	}
	// Apply codec-aware scaling before running the detectors so each
	// metric is judged against the codec's tolerance envelope.
	t = EffectiveThresholds(t)

	baselineFwd := baselineFor(forward, baselineByProbeID)
	baselineRet := baselineFor(returnPath, reverseBaselineByProbeID)

	var issues []VoiceQualityIssue
	if forward != nil {
		issues = append(issues, detectVoiceQualityIssuesForDirection(forward, baselineFwd, VoicePathForward, targetAgentName, t)...)
	}
	if returnPath != nil {
		issues = append(issues, detectVoiceQualityIssuesForDirection(returnPath, baselineRet, VoicePathReturn, targetAgentName, t)...)
	}

	// Burst loss: needs the forward and return path objects to
	// read maxConsecutiveLoss / totalBursts. Fires independently of
	// the per-direction suite when those values cross the burst
	// thresholds.
	if forward != nil {
		issues = append(issues, detectBurstLoss(forward, VoicePathForward, targetAgentName)...)
	}
	if returnPath != nil {
		issues = append(issues, detectBurstLoss(returnPath, VoicePathReturn, targetAgentName)...)
	}

	// Asymmetry needs both directions in hand; only fires when both are present.
	if forward != nil && returnPath != nil {
		issues = append(issues, detectAsymmetricVoiceDegradation(forward, returnPath, targetAgentName, t)...)
	}

	return issues
}

// detectVoiceQualityIssuesPerPair runs the full voice-issue heuristic
// suite against every probe in `forwardMap` and `reverseMap`,
// returning one issue slice per probe ID. The detector set is the same
// as the single-pair version; the per-probe shape lets the multi-target
// report show issues per destination without collapsing onto the
// worst-offender.
//
// `targetByProbeID` is the human-readable target name for each probe
// (most probes point at a single target; if absent the probe ID is
// used in the issue titles).
//
// The function is the new canonical entry point — the single-pair
// version above is now a thin wrapper that calls
// detectVoiceQualityIssuesForDirection per direction.
func detectVoiceQualityIssuesPerPair(forwardMap, reverseMap, baselineByProbeID, reverseBaselineByProbeID map[uint]*VoicePathMetrics, targetByProbeID map[uint]string, thresholds *VoiceThresholds) map[uint][]VoiceQualityIssue {
	t := VoiceDefaultThresholds
	if thresholds != nil {
		t = *thresholds
	}
	t = EffectiveThresholds(t)
	out := make(map[uint][]VoiceQualityIssue)

	for probeID, fwd := range forwardMap {
		if fwd == nil {
			continue
		}
		fwd.Direction = VoicePathForward
		targetName := targetNameFor(probeID, targetByProbeID, fwd)
		base := baselineFor(fwd, baselineByProbeID)
		out[probeID] = append(out[probeID], detectVoiceQualityIssuesForDirection(fwd, base, VoicePathForward, targetName, t)...)
		out[probeID] = append(out[probeID], detectBurstLoss(fwd, VoicePathForward, targetName)...)
	}

	for probeID, rev := range reverseMap {
		if rev == nil {
			continue
		}
		rev.Direction = VoicePathReturn
		targetName := targetNameFor(probeID, targetByProbeID, rev)
		base := baselineFor(rev, reverseBaselineByProbeID)
		// Asymmetry detection pairs the return path with its matching
		// forward path on the same probe, when one exists.
		if fwd, ok := forwardMap[probeID]; ok && fwd != nil {
			fwdBase := baselineFor(fwd, baselineByProbeID)
			out[probeID] = append(out[probeID], detectAsymmetricVoiceDegradation(fwd, rev, targetName, t)...)
			_ = fwdBase
		}
		out[probeID] = append(out[probeID], detectVoiceQualityIssuesForDirection(rev, base, VoicePathReturn, targetName, t)...)
		out[probeID] = append(out[probeID], detectBurstLoss(rev, VoicePathReturn, targetName)...)
	}

	return out
}

// detectVoiceQualityIssuesForDirection runs the single-direction
// suite (jitter, loss, latency-only, OOO) and is shared between the
// single-pair and per-pair entry points.
func detectVoiceQualityIssuesForDirection(path, baseline *VoicePathMetrics, direction VoicePathDirection, targetAgentName string, t VoiceThresholds) []VoiceQualityIssue {
	var issues []VoiceQualityIssue
	if path == nil {
		return issues
	}

	issues = append(issues, detectJitterAnomalies(path, baseline, direction, targetAgentName, t)...)
	issues = append(issues, detectPacketLossAnomalies(path, baseline, direction, targetAgentName, t)...)
	issues = append(issues, detectLatencyOnlyDegradation(path, baseline, direction, targetAgentName, t)...)

	// Out of sequence / packet reordering
	if path.OutOfSequence > t.OutOfSequencePct {
		dir := "forward"
		cause := "Packet reordering can indicate ECMP load balancing, suboptimal routing, or a problematic middlebox"
		recs := []string{
			"Run MTR with TCP mode (mtr -T) to check for ECMP hashing issues",
			"Compare route at different times to identify unstable hops",
		}
		if direction == VoicePathReturn {
			dir = "return"
			cause = "Asymmetric return routing may cause packet reordering"
			recs = []string{"Check if return path uses different ISP or routing path"}
		}
		issues = append(issues, VoiceQualityIssue{
			ID:              fmt.Sprintf("out_of_sequence_%d_%s", path.ProbeID, direction),
			Severity:        "warning",
			Title:           fmt.Sprintf("Packet reordering detected on %s path to %s", dir, targetAgentName),
			Category:        "out_of_order",
			AffectedPath:    direction,
			TargetAgentName: targetAgentName,
			SuspectedCause:  cause,
			Evidence: []string{
				fmt.Sprintf("Out of sequence: %.2f%%", path.OutOfSequence),
				fmt.Sprintf("Duplicates: %.2f%%", path.Duplicates),
				fmt.Sprintf("Jitter: %.1fms", path.JitterAvg),
			},
			Recommendations: recs,
		})
	}

	return issues
}

// targetNameFor resolves a probe ID to a target name, falling back to
// the metrics' own TargetAgentName when the per-probe name map is
// silent. Used by detectVoiceQualityIssuesPerPair so the issue
// titles always read against a real destination.
func targetNameFor(probeID uint, byProbeID map[uint]string, m *VoicePathMetrics) string {
	if byProbeID != nil {
		if name, ok := byProbeID[probeID]; ok && name != "" {
			return name
		}
	}
	if m != nil && m.TargetAgentName != "" {
		return m.TargetAgentName
	}
	return fmt.Sprintf("probe-%d", probeID)
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
// tags existing loss issues with burst vs steady. The actual
// classification lives in detectBurstLossPattern in voice_burst.go;
// this is the wiring point so the call site stays stable when the
// burst detector is upgraded.
func annotateLossPatterns(issues []VoiceQualityIssue, forward, returnPath *VoicePathMetrics) {
	if len(issues) == 0 {
		return
	}
	pattern := classifyLossPattern(forward, returnPath)
	if pattern == "" {
		return
	}
	for i := range issues {
		if issues[i].Category != "packet_loss" && issues[i].Category != "burst_loss" {
			continue
		}
		if issues[i].LossPattern == "" {
			issues[i].LossPattern = pattern
		}
	}
}

// classifyLossPattern returns "burst", "steady", or "mixed" based on
// the per-direction loss values. Empty when neither direction has
// data. Real burst-vs-steady classification is the responsibility of
// detectBurstLossPattern in voice_burst.go; this is a quick
// categorical hint for the issue annotation.
//
// For two values (forward + return), the coefficient-of-variation is
// bounded at 1.0 — so a 5× ratio between fwd and rev is the practical
// "burst" marker. We combine a max/min ratio check with the CV for
// richer cases.
func classifyLossPattern(forward, returnPath *VoicePathMetrics) string {
	var samples []float64
	if forward != nil && forward.SampleCount > 0 {
		samples = append(samples, forward.PacketLoss)
	}
	if returnPath != nil && returnPath.SampleCount > 0 {
		samples = append(samples, returnPath.PacketLoss)
	}
	if len(samples) < 2 {
		return ""
	}
	mean, stddev := meanStddev(samples)
	if mean <= 0 {
		return ""
	}
	cv := stddev / mean

	// Max/min ratio is the more intuitive burst signal for small
	// samples. Two values can never have a CV > 1, but a 5x ratio
	// between fwd and rev clearly indicates one direction is the
	// outlier.
	max, min := samples[0], samples[0]
	for _, v := range samples {
		if v > max {
			max = v
		}
		if v < min {
			min = v
		}
	}
	ratio := max / min

	switch {
	case ratio >= 5 || cv >= 0.9:
		return "burst"
	case ratio < 1.5 && cv < 0.25:
		return "steady"
	default:
		return "mixed"
	}
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
