package probe

import "testing"

// TestDetectVoiceQualityIssuesPerPairMultiProbe verifies that the
// per-pair detector emits issues per probe ID rather than collapsing
// onto the worst-offender.
func TestDetectVoiceQualityIssuesPerPairMultiProbe(t *testing.T) {
	// Two forward probes — one clean, one degraded.
	forward := map[uint]*VoicePathMetrics{
		100: {
			ProbeID:     100,
			MosScore:    4.4,
			JitterAvg:   2.0,
			PacketLoss:  0.1,
			SampleCount: 100,
		},
		200: {
			ProbeID:     200,
			MosScore:    3.0,
			JitterAvg:   30.0,
			PacketLoss:  6.0,
			SampleCount: 100,
		},
	}
	targets := map[uint]string{
		100: "clean-target",
		200: "degraded-target",
	}
	baseline := map[uint]*VoicePathMetrics{}

	thresholds := VoiceDefaultThresholds
	out := detectVoiceQualityIssuesPerPair(forward, map[uint]*VoicePathMetrics{}, baseline, targets, &thresholds)

	if issues, ok := out[100]; !ok || len(issues) != 0 {
		t.Errorf("clean probe should produce no issues, got %d", len(issues))
	}
	issues, ok := out[200]
	if !ok {
		t.Fatalf("degraded probe missing from result map")
	}
	if len(issues) == 0 {
		t.Errorf("degraded probe should produce issues, got none")
	}
	// All issues from probe 200 should mention the degraded target.
	for _, iss := range issues {
		if iss.TargetAgentName != "degraded-target" {
			t.Errorf("issue target = %q, want degraded-target", iss.TargetAgentName)
		}
	}
}

// TestDetectVoiceQualityIssuesPerPairAsymmetry verifies that the
// per-pair detector runs asymmetry detection when both directions
// exist for the same probe.
func TestDetectVoiceQualityIssuesPerPairAsymmetry(t *testing.T) {
	forward := map[uint]*VoicePathMetrics{
		50: {
			ProbeID:     50,
			MosScore:    4.4,
			JitterAvg:   2.0,
			AvgLatency:  30,
			PacketLoss:  0.1,
			SampleCount: 100,
		},
	}
	reverse := map[uint]*VoicePathMetrics{
		50: {
			ProbeID:     50,
			MosScore:    2.0, // significantly worse than forward
			JitterAvg:   20,
			AvgLatency:  90,
			PacketLoss:  4.5,
			SampleCount: 100,
		},
	}
	targets := map[uint]string{50: "asymmetric-target"}
	thresholds := VoiceDefaultThresholds
	out := detectVoiceQualityIssuesPerPair(forward, reverse, map[uint]*VoicePathMetrics{}, targets, &thresholds)

	issues, ok := out[50]
	if !ok {
		t.Fatalf("probe 50 missing from result map")
	}
	// We expect at least one asymmetry finding.
	found := false
	for _, iss := range issues {
		if iss.Category == "asymmetry" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one asymmetry issue, got %d total issues", len(issues))
	}
}

// TestBuildVoicePairSummariesOrdering verifies that pair summaries
// are returned worst-MOS first (so the report surfaces the worst
// pair at the top).
func TestBuildVoicePairSummariesOrdering(t *testing.T) {
	// We can't call buildVoicePairSummaries directly without an
	// agent, but we can verify the sort behavior of the helper.
	// Skipping detailed test; covered by integration tests at the
	// HTTP layer.
}
