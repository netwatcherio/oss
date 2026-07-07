package probe

import "testing"

// TestDetectBurstLossNoTriggers verifies the detector stays silent
// when the cycle reports no burst activity.
func TestDetectBurstLossNoTriggers(t *testing.T) {
	path := &VoicePathMetrics{
		ProbeID:            42,
		MaxConsecutiveLoss: 1,
		TotalBursts:        0,
		SampleCount:        50,
		PacketLoss:         0.1,
		MosScore:           4.4,
	}
	issues := detectBurstLoss(path, VoicePathForward, "test-target")
	if len(issues) != 0 {
		t.Errorf("expected no issues for clean path, got %d", len(issues))
	}
}

// TestDetectBurstLossWarning verifies that 3-5 consecutive losses
// fire a warning-level issue.
func TestDetectBurstLossWarning(t *testing.T) {
	path := &VoicePathMetrics{
		ProbeID:            42,
		MaxConsecutiveLoss: 4,
		TotalBursts:        1,
		SampleCount:        50,
		PacketLoss:         1.5,
		MosScore:           3.9,
	}
	issues := detectBurstLoss(path, VoicePathForward, "test-target")
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Severity != "warning" {
		t.Errorf("expected warning severity, got %q", issues[0].Severity)
	}
	if issues[0].Category != "burst_loss" {
		t.Errorf("expected burst_loss category, got %q", issues[0].Category)
	}
	if issues[0].LossPattern == "" {
		t.Errorf("expected non-empty loss pattern annotation")
	}
}

// TestDetectBurstLossCritical verifies that 6+ consecutive losses
// upgrade severity to critical.
func TestDetectBurstLossCritical(t *testing.T) {
	path := &VoicePathMetrics{
		ProbeID:            42,
		MaxConsecutiveLoss: 8,
		TotalBursts:        2,
		SampleCount:        50,
		PacketLoss:         2.5,
		MosScore:           3.4,
	}
	issues := detectBurstLoss(path, VoicePathForward, "test-target")
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Severity != "critical" {
		t.Errorf("expected critical severity, got %q", issues[0].Severity)
	}
}

// TestDetectBurstLossDensityOnly verifies that high density (cycles
// with bursts) can fire even when consecutive loss is below the
// absolute threshold.
func TestDetectBurstLossDensityOnly(t *testing.T) {
	path := &VoicePathMetrics{
		ProbeID:            42,
		MaxConsecutiveLoss: 1,
		TotalBursts:        30,
		SampleCount:        100, // 30% of cycles saw bursts
		PacketLoss:         3.0,
		MosScore:           3.7,
	}
	issues := detectBurstLoss(path, VoicePathForward, "test-target")
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue from density, got %d", len(issues))
	}
}

// TestDetectBurstLossLowSampleGuard verifies that the detector stays
// silent when there isn't enough sample data to compute density.
func TestDetectBurstLossLowSampleGuard(t *testing.T) {
	path := &VoicePathMetrics{
		ProbeID:            42,
		MaxConsecutiveLoss: 1,
		TotalBursts:        3,
		SampleCount:        5, // <10, density check skipped
	}
	issues := detectBurstLoss(path, VoicePathForward, "test-target")
	if len(issues) != 0 {
		t.Errorf("expected no issues for low-sample path, got %d", len(issues))
	}
}

// TestClassifyBurstLossPattern verifies the burst-vs-steady
// classifier.
func TestClassifyBurstLossPattern(t *testing.T) {
	// Steady: low CV
	both := &VoicePathMetrics{PacketLoss: 1.0, SampleCount: 100}
	out := ClassifyBurstLossPattern(both, both)
	if out != "steady" {
		t.Errorf("expected steady pattern for matching values, got %q", out)
	}

	// Burst: very high CV (one big loss, one near-zero)
	high := &VoicePathMetrics{PacketLoss: 50.0, SampleCount: 100}
	low := &VoicePathMetrics{PacketLoss: 0.01, SampleCount: 100}
	out = ClassifyBurstLossPattern(high, low)
	if out != "burst" {
		t.Errorf("expected burst pattern for high-variance, got %q", out)
	}

	// Mixed: moderate CV
	midA := &VoicePathMetrics{PacketLoss: 2.0, SampleCount: 100}
	midB := &VoicePathMetrics{PacketLoss: 0.8, SampleCount: 100}
	out = ClassifyBurstLossPattern(midA, midB)
	if out != "mixed" && out != "burst" {
		t.Errorf("expected mixed/burst for moderate variance, got %q", out)
	}
}
