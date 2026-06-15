package probe

import (
	"testing"
)

// TestClassifyTimePatternStable ensures a series with low variance
// classifies as "stable" rather than "constant" or other patterns.
func TestClassifyTimePatternStable(t *testing.T) {
	buckets := []VoiceBucket{}
	for i := 0; i < 10; i++ {
		buckets = append(buckets, VoiceBucket{Forward: 4.4})
	}
	got := classifyTimePattern(buckets)
	if got != "stable" {
		t.Errorf("got %q, want stable", got)
	}
}

// TestClassifyTimePatternImproving checks that a monotonically
// rising series is classified as "improving".
func TestClassifyTimePatternImproving(t *testing.T) {
	buckets := []VoiceBucket{}
	for i := 0; i < 10; i++ {
		// 3.5 → 4.5 (delta 1.0; stddev is small in this series)
		mos := 3.5 + float64(i)*0.1
		buckets = append(buckets, VoiceBucket{Forward: mos})
	}
	got := classifyTimePattern(buckets)
	if got != "improving" {
		t.Errorf("got %q, want improving", got)
	}
}

// TestClassifyTimePatternWorsening checks that a monotonically
// dropping series classifies as "worsening".
func TestClassifyTimePatternWorsening(t *testing.T) {
	buckets := []VoiceBucket{}
	for i := 0; i < 10; i++ {
		mos := 4.5 - float64(i)*0.1
		buckets = append(buckets, VoiceBucket{Forward: mos})
	}
	got := classifyTimePattern(buckets)
	if got != "worsening" {
		t.Errorf("got %q, want worsening", got)
	}
}

// TestClassifyTimePatternTooShort ensures we return empty for
// series too short to classify.
func TestClassifyTimePatternTooShort(t *testing.T) {
	buckets := []VoiceBucket{{Forward: 4.0}, {Forward: 4.2}}
	got := classifyTimePattern(buckets)
	if got != "" {
		t.Errorf("got %q, want empty (too short)", got)
	}
}

// TestClassifyTimePatternPeriodic verifies that a series with
// recurring troughs is classified as "periodic_30min".
func TestClassifyTimePatternPeriodic(t *testing.T) {
	buckets := []VoiceBucket{}
	// Construct: high baseline + 3 troughs spaced 3 apart.
	mos := []float64{
		4.4, 4.4, 4.0, 4.4, 4.4, 4.0, 4.4, 4.4, 4.0, 4.4,
	}
	for _, m := range mos {
		buckets = append(buckets, VoiceBucket{Forward: m})
	}
	got := classifyTimePattern(buckets)
	if got != "periodic_30min" {
		t.Errorf("got %q, want periodic_30min", got)
	}
}

// TestAggregateVoicePathMetricsEmpty ensures aggregate of empty
// input returns nil.
func TestAggregateVoicePathMetricsEmpty(t *testing.T) {
	got := aggregateVoicePathMetrics(nil, VoicePathForward)
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

// TestAggregateVoicePathMetricsWeighted verifies the sample-weighted
// mean logic. Two probes with 100 samples each at MOS 4.0 and 3.0
// should produce an aggregate of 3.5 (equal weights) and not
// (4.0+3.0)/2 = 3.5 by accident — let's also check the asymmetry
// where one probe has 1000 samples.
func TestAggregateVoicePathMetricsWeighted(t *testing.T) {
	probes := []*VoicePathMetrics{
		{ProbeID: 1, SampleCount: 100, MosScore: 4.0, AvgLatency: 50, JitterAvg: 5, PacketLoss: 0.1},
		{ProbeID: 2, SampleCount: 100, MosScore: 3.0, AvgLatency: 100, JitterAvg: 15, PacketLoss: 1.0},
	}
	got := aggregateVoicePathMetrics(probes, VoicePathForward)
	if got == nil {
		t.Fatal("expected non-nil aggregate")
	}
	if got.MosScore != 3.5 {
		t.Errorf("MOS = %v, want 3.5 (equal-weight average)", got.MosScore)
	}
	if got.AvgLatency != 75.0 {
		t.Errorf("AvgLatency = %v, want 75.0", got.AvgLatency)
	}
	if got.PacketLoss != 0.55 {
		t.Errorf("PacketLoss = %v, want 0.55", got.PacketLoss)
	}
	if got.Direction != VoicePathForward {
		t.Errorf("Direction = %q, want forward", got.Direction)
	}

	// Now weight probe 2 heavily.
	probes[1].SampleCount = 1000
	got = aggregateVoicePathMetrics(probes, VoicePathForward)
	expectedMos := (4.0*100 + 3.0*1000) / 1100.0
	if got.MosScore != expectedMos {
		t.Errorf("weighted MOS = %v, want %v", got.MosScore, expectedMos)
	}
}

// TestAggregateVoicePathMetricsP95IsMax ensures P95 is the max
// across probes, not the average. The P95 represents the worst
// case the user can experience.
func TestAggregateVoicePathMetricsP95IsMax(t *testing.T) {
	probes := []*VoicePathMetrics{
		{ProbeID: 1, SampleCount: 100, P95Latency: 60, JitterP95: 8},
		{ProbeID: 2, SampleCount: 100, P95Latency: 200, JitterP95: 25},
	}
	got := aggregateVoicePathMetrics(probes, VoicePathForward)
	if got.P95Latency != 200 {
		t.Errorf("P95Latency = %v, want 200 (max)", got.P95Latency)
	}
	if got.JitterP95 != 25 {
		t.Errorf("JitterP95 = %v, want 25 (max)", got.JitterP95)
	}
}

// TestAggregateVoicePathMetricsCongestion ensures the
// congestion level is computed from the aggregate.
func TestAggregateVoicePathMetricsCongestion(t *testing.T) {
	// Multiple probes all at moderate values should produce
	// "moderate" congestion.
	probes := []*VoicePathMetrics{
		{ProbeID: 1, SampleCount: 100, JitterAvg: 18, PacketLoss: 1.5, AvgLatency: 100},
		{ProbeID: 2, SampleCount: 100, JitterAvg: 20, PacketLoss: 2.0, AvgLatency: 110},
	}
	got := aggregateVoicePathMetrics(probes, VoicePathForward)
	if got == nil {
		t.Fatal("expected non-nil aggregate")
	}
	if got.CongestionLevel != CongestionModerate {
		t.Errorf("CongestionLevel = %q, want moderate", got.CongestionLevel)
	}
}

// TestComputeBaselineComparisonEmpty ensures nil when no baseline
// samples exist.
func TestComputeBaselineComparisonEmpty(t *testing.T) {
	got := computeBaselineComparison(nil, nil)
	if got != nil {
		t.Errorf("expected nil for empty input, got %+v", got)
	}
}

// TestComputeBaselineComparisonWorsening ensures a worse current
// vs baseline is correctly tagged.
func TestComputeBaselineComparisonWorsening(t *testing.T) {
	probes := []*VoicePathMetrics{
		{ProbeID: 1, SampleCount: 100, MosScore: 3.0, AvgLatency: 200, JitterAvg: 30, PacketLoss: 3.0},
	}
	baseline := map[uint]*VoicePathMetrics{
		1: {ProbeID: 1, SampleCount: 200, MosScore: 4.5, AvgLatency: 50, JitterAvg: 5, PacketLoss: 0.1},
	}
	d := computeBaselineComparison(probes, baseline)
	if d == nil {
		t.Fatal("expected non-nil delta")
	}
	if d.Trend != "worsening" {
		t.Errorf("Trend = %q, want worsening", d.Trend)
	}
	if d.MosDelta >= 0 {
		t.Errorf("MosDelta = %v, want negative", d.MosDelta)
	}
	if d.BaselineSamples != 200 {
		t.Errorf("BaselineSamples = %d, want 200", d.BaselineSamples)
	}
}

// TestComputeBaselineComparisonImproving ensures a better current
// vs baseline is correctly tagged.
func TestComputeBaselineComparisonImproving(t *testing.T) {
	probes := []*VoicePathMetrics{
		{ProbeID: 1, SampleCount: 100, MosScore: 4.5, AvgLatency: 50, JitterAvg: 5, PacketLoss: 0.1},
	}
	baseline := map[uint]*VoicePathMetrics{
		1: {ProbeID: 1, SampleCount: 200, MosScore: 3.0, AvgLatency: 200, JitterAvg: 30, PacketLoss: 3.0},
	}
	d := computeBaselineComparison(probes, baseline)
	if d == nil {
		t.Fatal("expected non-nil delta")
	}
	if d.Trend != "improving" {
		t.Errorf("Trend = %q, want improving", d.Trend)
	}
	if d.MosDelta <= 0 {
		t.Errorf("MosDelta = %v, want positive", d.MosDelta)
	}
}

// TestComputeBaselineComparisonStable ensures small deltas are
// tagged "stable".
func TestComputeBaselineComparisonStable(t *testing.T) {
	probes := []*VoicePathMetrics{
		{ProbeID: 1, SampleCount: 100, MosScore: 4.4, AvgLatency: 50, JitterAvg: 5, PacketLoss: 0.1},
	}
	baseline := map[uint]*VoicePathMetrics{
		1: {ProbeID: 1, SampleCount: 200, MosScore: 4.5, AvgLatency: 50, JitterAvg: 5, PacketLoss: 0.1},
	}
	d := computeBaselineComparison(probes, baseline)
	if d == nil {
		t.Fatal("expected non-nil delta")
	}
	if d.Trend != "stable" {
		t.Errorf("Trend = %q, want stable", d.Trend)
	}
}

// TestHasPeriodicTroughsPattern verifies the trough detection
// heuristic.
func TestHasPeriodicTroughsPattern(t *testing.T) {
	// Series with three clear troughs below the mean.
	withTroughs := []float64{
		4.4, 4.4, 2.5, 4.4, 4.4, 2.5, 4.4, 4.4, 2.5, 4.4,
	}
	if !hasPeriodicTroughs(withTroughs) {
		t.Error("expected periodic troughs to be detected")
	}

	// Series with no troughs — strictly increasing values, all above
	// the mean, no local minima.
	noTroughs := []float64{4.0, 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7}
	if hasPeriodicTroughs(noTroughs) {
		t.Error("expected no troughs to NOT be detected as periodic")
	}

	// Too short.
	if hasPeriodicTroughs([]float64{1, 2, 3}) {
		t.Error("expected short series to NOT be periodic")
	}
}

// TestDetectVoiceQualityIssuesEmptyInput ensures no panic on nil
// inputs.
func TestDetectVoiceQualityIssuesEmptyInput(t *testing.T) {
	issues := detectVoiceQualityIssues(nil, nil, nil, "test", nil)
	if len(issues) != 0 {
		t.Errorf("expected no issues for nil paths, got %+v", issues)
	}
}

// TestDetectVoiceQualityIssuesWithAsymmetryPerMetric ensures the
// new per-metric asymmetry finding fires when only ONE metric
// disagrees between forward and return.
func TestDetectVoiceQualityIssuesWithAsymmetryPerMetric(t *testing.T) {
	// Latency is wildly different but loss/jitter are similar.
	fwd := &VoicePathMetrics{
		Direction: VoicePathForward, ProbeID: 1, MosScore: 4.0,
		AvgLatency: 30, JitterAvg: 5, PacketLoss: 0.1,
	}
	ret := &VoicePathMetrics{
		Direction: VoicePathReturn, ProbeID: 1, MosScore: 2.5,
		AvgLatency: 200, JitterAvg: 8, PacketLoss: 0.2,
	}
	issues := detectVoiceQualityIssues(fwd, ret, nil, "test", &VoiceDefaultThresholds)

	var asymCount, totalAsym int
	for _, iss := range issues {
		if iss.Category == "latency_asymmetry" {
			asymCount++
		}
	}
	totalAsym = asymCount
	_ = totalAsym
	if asymCount == 0 {
		t.Errorf("expected at least one latency_asymmetry finding, got %+v", issues)
	}
}
