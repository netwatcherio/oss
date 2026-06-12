package probe

import (
	"strings"
	"testing"
)

func TestBuildDirectionalitySignalsLossAsymmetry(t *testing.T) {
	fwd := ProbeMetrics{PacketLoss: 8, AvgLatency: 40, JitterAvg: 2, SampleCount: 10}
	rev := ProbeMetrics{PacketLoss: 0.5, AvgLatency: 41, JitterAvg: 2, SampleCount: 10}

	signals, findings := buildDirectionalitySignals(fwd, rev, "A → B", "B → A")

	var lossSignal *AnalysisSignal
	for i := range signals {
		if signals[i].Type == "loss_asymmetry" {
			lossSignal = &signals[i]
		}
	}
	if lossSignal == nil {
		t.Fatal("loss asymmetry not detected (8% vs 0.5%)")
	}
	if lossSignal.Severity != "critical" {
		t.Errorf("severity = %s, want critical for 8%% one-way loss", lossSignal.Severity)
	}

	found := false
	for _, f := range findings {
		if f.ID == "loss-asymmetry" {
			found = true
			if !strings.Contains(f.Title, "A → B") {
				t.Errorf("finding should name the degraded direction A → B, got %q", f.Title)
			}
		}
	}
	if !found {
		t.Error("loss-asymmetry finding missing")
	}
}

func TestBuildDirectionalitySignalsSymmetricHealthyIsQuiet(t *testing.T) {
	fwd := ProbeMetrics{PacketLoss: 0.3, AvgLatency: 40, JitterAvg: 3, SampleCount: 10}
	rev := ProbeMetrics{PacketLoss: 0.4, AvgLatency: 42, JitterAvg: 4, SampleCount: 10}

	signals, findings := buildDirectionalitySignals(fwd, rev, "A → B", "B → A")
	if len(signals) != 0 || len(findings) != 0 {
		t.Errorf("healthy symmetric link must not produce asymmetry noise: %d signals, %d findings", len(signals), len(findings))
	}
}

func TestBuildDirectionalitySignalsLatencyAsymmetry(t *testing.T) {
	fwd := ProbeMetrics{AvgLatency: 35, PacketLoss: 0, SampleCount: 10}
	rev := ProbeMetrics{AvgLatency: 140, PacketLoss: 0, SampleCount: 10}

	signals, _ := buildDirectionalitySignals(fwd, rev, "A → B", "B → A")
	found := false
	for _, s := range signals {
		if s.Type == "latency_asymmetry" {
			found = true
		}
	}
	if !found {
		t.Error("latency asymmetry not detected (35ms vs 140ms)")
	}
}

func TestBuildDirectionalitySignalsJitterAsymmetry(t *testing.T) {
	fwd := ProbeMetrics{AvgLatency: 40, JitterAvg: 2, SampleCount: 10}
	rev := ProbeMetrics{AvgLatency: 42, JitterAvg: 25, SampleCount: 10}

	signals, findings := buildDirectionalitySignals(fwd, rev, "A → B", "B → A")
	found := false
	for _, s := range signals {
		if s.Type == "jitter_asymmetry" {
			found = true
		}
	}
	if !found {
		t.Error("jitter asymmetry not detected (2ms vs 25ms)")
	}
	for _, f := range findings {
		if f.ID == "jitter-asymmetry" && !strings.Contains(f.Title, "B → A") {
			t.Errorf("finding should name the degraded direction B → A, got %q", f.Title)
		}
	}
}

func TestCombineDirectionHealthWeightsWorseDirection(t *testing.T) {
	good := HealthVector{OverallHealth: 95, LatencyScore: 95, PacketLossScore: 100, RouteStability: 100, MosScore: 4.4}
	bad := HealthVector{OverallHealth: 30, LatencyScore: 40, PacketLossScore: 20, RouteStability: 100, MosScore: 2.1}

	combined := combineDirectionHealth(good, bad)

	// 30*0.65 + 95*0.35 = 52.75 — closer to the bad direction than a plain avg (62.5)
	if combined.OverallHealth > 55 || combined.OverallHealth < 50 {
		t.Errorf("combined overall = %.1f, want ~52.75 (worse-direction weighted)", combined.OverallHealth)
	}
	if combined.MosScore != 2.1 {
		t.Errorf("combined MOS = %.1f, want the worse direction's 2.1", combined.MosScore)
	}
	if combined.Grade == "" {
		t.Error("combined grade not set")
	}

	// Order independence
	flipped := combineDirectionHealth(bad, good)
	if flipped.OverallHealth != combined.OverallHealth {
		t.Errorf("combine is order-dependent: %.2f vs %.2f", flipped.OverallHealth, combined.OverallHealth)
	}
}
