package probe

import "testing"

// TestScoreSeverityMagnitudeOnly verifies that magnitude is the
// dominant factor when no time-series bucket data is available.
func TestScoreSeverityMagnitudeOnly(t *testing.T) {
	// Below threshold → 0
	if got := ScoreSeverity(10, 15, 0, 0); got != 0 {
		t.Errorf("below threshold: got %v, want 0", got)
	}
	// At threshold → 0 (magnitude factor only)
	if got := ScoreSeverity(15, 15, 0, 0); got != 0 {
		t.Errorf("at threshold: got %v, want 0", got)
	}
	// 1.5× threshold → magnitude 0.5 → score 0.25
	if got := ScoreSeverity(22.5, 15, 0, 0); got != 0.25 {
		t.Errorf("1.5x threshold: got %v, want 0.25", got)
	}
	// 2× threshold → magnitude 1 → score 0.5
	if got := ScoreSeverity(30, 15, 0, 0); got != 0.5 {
		t.Errorf("2x threshold: got %v, want 0.5", got)
	}
}

// TestScoreSeverityDurationOnly verifies duration factor when
// observed sits at threshold (magnitude = 0).
func TestScoreSeverityDurationOnly(t *testing.T) {
	// Half the window → duration 0.5 → score 0.25
	if got := ScoreSeverity(15, 15, 5, 10); got != 0.25 {
		t.Errorf("half-window duration: got %v, want 0.25", got)
	}
	// Full window → duration 1 → score 0.5
	if got := ScoreSeverity(15, 15, 10, 10); got != 0.5 {
		t.Errorf("full-window duration: got %v, want 0.5", got)
	}
}

// TestScoreSeverityCombined verifies that magnitude and duration
// combine into the final score.
func TestScoreSeverityCombined(t *testing.T) {
	// 2× magnitude + full duration → (1 + 1) / 2 = 1
	if got := ScoreSeverity(30, 15, 10, 10); got != 1 {
		t.Errorf("2x magnitude + full duration: got %v, want 1", got)
	}
}

// TestScoreSeverityClamping verifies that very large observed values
// don't blow past 1.0.
func TestScoreSeverityClamping(t *testing.T) {
	if got := ScoreSeverity(150, 15, 10, 10); got != 1 {
		t.Errorf("extreme magnitude: got %v, want 1 (clamped)", got)
	}
}

// TestSeverityFromScore verifies the threshold cutoffs.
func TestSeverityFromScore(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0.0, "info"},
		{0.05, "info"},
		{0.1, "info"},
		{0.4, "warning"},
		{0.5, "warning"},
		{0.69, "warning"},
		{0.7, "critical"},
		{0.85, "critical"},
		{1.0, "critical"},
	}
	for _, c := range cases {
		if got := SeverityFromScore(c.score); got != c.want {
			t.Errorf("SeverityFromScore(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}

// TestDurationFactorEdgeCases verifies boundary conditions.
func TestDurationFactorEdgeCases(t *testing.T) {
	cases := []struct {
		dur, total int
		want       float64
	}{
		{0, 0, 0},
		{5, 0, 0},
		{0, 5, 0},
		{5, 10, 0.5},
		{10, 10, 1},
		{15, 10, 1}, // capped at 1
	}
	for _, c := range cases {
		if got := DurationFactor(c.dur, c.total); got != c.want {
			t.Errorf("DurationFactor(%d, %d) = %v, want %v", c.dur, c.total, got, c.want)
		}
	}
}

// TestMagnitudeFactorEdgeCases verifies the magnitude mapping.
func TestMagnitudeFactorEdgeCases(t *testing.T) {
	cases := []struct {
		obs, thr, want float64
	}{
		{10, 15, 0},
		{15, 15, 0},
		{22.5, 15, 0.5},
		{30, 15, 1},
		{45, 15, 1},
		{10, 0, 0}, // invalid threshold
	}
	for _, c := range cases {
		if got := MagnitudeFactor(c.obs, c.thr); got != c.want {
			t.Errorf("MagnitudeFactor(%v, %v) = %v, want %v", c.obs, c.thr, got, c.want)
		}
	}
}
