package probe

import "testing"

// TestCorrelateWithRouteNoMTR verifies the correlator is a no-op
// when no MTR data is available.
func TestCorrelateWithRouteNoMTR(t *testing.T) {
	issues := []VoiceQualityIssue{
		{ID: "jitter_x", Category: "jitter_spike"},
	}
	out := correlateWithRoute(issues, nil)
	if out[0].LikelyHop != 0 || out[0].HopEvidence != "" {
		t.Errorf("expected no hop tag when MTR data missing, got hop=%d evidence=%q",
			out[0].LikelyHop, out[0].HopEvidence)
	}
}

// TestCorrelateWithRoutePacketLoss verifies the correlator picks the
// worst-loss hop when a packet loss issue is being correlated.
func TestCorrelateWithRoutePacketLoss(t *testing.T) {
	mtr := []MtrHopSummary{
		{HopNumber: 1, IP: "10.0.0.1", LossPct: 0, AvgLatencyMs: 1},
		{HopNumber: 2, IP: "10.0.0.2", LossPct: 0.1, AvgLatencyMs: 5},
		{HopNumber: 3, IP: "10.0.0.3", LossPct: 5.0, AvgLatencyMs: 30}, // the worst
		{HopNumber: 4, IP: "10.0.0.4", LossPct: 0, AvgLatencyMs: 35},
	}
	issues := []VoiceQualityIssue{
		{ID: "loss_x", Category: "packet_loss"},
	}
	out := correlateWithRoute(issues, mtr)
	if out[0].LikelyHop != 3 {
		t.Errorf("expected hop 3 to be tagged as root cause, got %d", out[0].LikelyHop)
	}
	if out[0].HopEvidence == "" {
		t.Errorf("expected hop evidence text, got empty")
	}
}

// TestCorrelateWithRouteAsymmetryUntagged verifies that categories
// we don't have an MTR signal for are left un-tagged.
func TestCorrelateWithRouteAsymmetryUntagged(t *testing.T) {
	mtr := []MtrHopSummary{
		{HopNumber: 1, IP: "10.0.0.1", LossPct: 0, AvgLatencyMs: 1},
		{HopNumber: 2, IP: "10.0.0.2", LossPct: 10, AvgLatencyMs: 100},
	}
	issues := []VoiceQualityIssue{
		{ID: "asym_x", Category: "asymmetry"},
	}
	out := correlateWithRoute(issues, mtr)
	if out[0].LikelyHop != 0 {
		t.Errorf("asymmetry issues should not be tagged, got hop=%d", out[0].LikelyHop)
	}
}

// TestCorrelateWithRouteLatencyOnly verifies the correlator picks
// the worst-latency hop when a latency_degradation issue is being
// correlated.
func TestCorrelateWithRouteLatencyOnly(t *testing.T) {
	mtr := []MtrHopSummary{
		{HopNumber: 1, IP: "10.0.0.1", AvgLatencyMs: 1, LossPct: 0},
		{HopNumber: 2, IP: "10.0.0.2", AvgLatencyMs: 200, LossPct: 0}, // worst latency
		{HopNumber: 3, IP: "10.0.0.3", AvgLatencyMs: 5, LossPct: 0},
	}
	issues := []VoiceQualityIssue{
		{ID: "lat_x", Category: "latency_degradation"},
	}
	out := correlateWithRoute(issues, mtr)
	if out[0].LikelyHop != 2 {
		t.Errorf("expected hop 2 (worst latency) tagged, got %d", out[0].LikelyHop)
	}
}

// TestCorrelateWithRouteNoDeviation verifies the correlator stays
// silent when all hops are uniform (no deviation worth flagging).
func TestCorrelateWithRouteNoDeviation(t *testing.T) {
	mtr := []MtrHopSummary{
		{HopNumber: 1, IP: "10.0.0.1", LossPct: 1, AvgLatencyMs: 10},
		{HopNumber: 2, IP: "10.0.0.2", LossPct: 1, AvgLatencyMs: 10},
		{HopNumber: 3, IP: "10.0.0.3", LossPct: 1, AvgLatencyMs: 10},
	}
	issues := []VoiceQualityIssue{
		{ID: "loss_x", Category: "packet_loss"},
	}
	out := correlateWithRoute(issues, mtr)
	if out[0].LikelyHop != 0 {
		t.Errorf("uniform hops should not be flagged, got hop=%d", out[0].LikelyHop)
	}
}
