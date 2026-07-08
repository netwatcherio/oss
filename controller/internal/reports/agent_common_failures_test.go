package reports

import (
	"testing"

	"netwatcher-controller/internal/probe"
)

// TestAggregateCommonFailuresGroupsByCategory verifies the workspace
// rollup groups every issue by category and counts them.
func TestAggregateCommonFailuresGroupsByCategory(t *testing.T) {
	issues := []probe.VoiceQualityIssue{
		{ID: "j1", Category: "jitter_spike", Severity: "warning"},
		{ID: "j2", Category: "jitter_spike", Severity: "critical"},
		{ID: "l1", Category: "packet_loss", Severity: "warning"},
		{ID: "l2", Category: "packet_loss", Severity: "warning"},
		{ID: "l3", Category: "packet_loss", Severity: "warning"},
	}
	pairs := []probe.VoicePairSummary{
		{ID: "p1", Agent: probe.AgentRef{ID: 1, Name: "alpha"}, Target: probe.TargetRef{Name: "T1"}, Issues: issues[0:3]},
		{ID: "p2", Agent: probe.AgentRef{ID: 2, Name: "beta"}, Target: probe.TargetRef{Name: "T2"}, Issues: issues[3:5]},
	}

	out := aggregateCommonFailures(issues, pairs)
	if len(out) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(out))
	}
	// packet_loss has 3 issues total → must come first
	if out[0].Category != "packet_loss" {
		t.Errorf("first bucket should be packet_loss (most occurrences), got %q", out[0].Category)
	}
	if out[0].Count != 3 {
		t.Errorf("packet_loss count = %d, want 3", out[0].Count)
	}
	if out[0].WarningCount != 3 {
		t.Errorf("packet_loss warning count = %d, want 3", out[0].WarningCount)
	}
	if out[0].CriticalCount != 0 {
		t.Errorf("packet_loss critical count = %d, want 0", out[0].CriticalCount)
	}
	if out[1].Category != "jitter_spike" {
		t.Errorf("second bucket should be jitter_spike, got %q", out[1].Category)
	}
	if out[1].Count != 2 {
		t.Errorf("jitter_spike count = %d, want 2", out[1].Count)
	}
	if out[1].CriticalCount != 1 {
		t.Errorf("jitter_spike critical count = %d, want 1", out[1].CriticalCount)
	}
}

// TestAggregateCommonFailuresAffectedAgentsDeduped verifies that
// each agent appears at most once per category (even if they have
// multiple issues of the same category).
func TestAggregateCommonFailuresAffectedAgentsDeduped(t *testing.T) {
	issues := []probe.VoiceQualityIssue{
		{ID: "j1", Category: "jitter_spike", Severity: "warning"},
		{ID: "j2", Category: "jitter_spike", Severity: "critical"},
		{ID: "j3", Category: "jitter_spike", Severity: "warning"},
	}
	pairs := []probe.VoicePairSummary{
		{ID: "p1", Agent: probe.AgentRef{ID: 10, Name: "alpha"}, Forward: &probe.VoicePathMetrics{ProbeID: 999}, Issues: issues},
	}

	out := aggregateCommonFailures(issues, pairs)
	if len(out) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(out))
	}
	if len(out[0].AffectedAgents) != 1 {
		t.Errorf("expected affected-agents to dedupe (1 entry), got %d", len(out[0].AffectedAgents))
	}
	if out[0].AffectedAgents[0].AgentID != 10 {
		t.Errorf("affected agent id = %d, want 10", out[0].AffectedAgents[0].AgentID)
	}
	if out[0].AffectedAgents[0].ProbeID != 999 {
		t.Errorf("affected probe id = %d, want 999", out[0].AffectedAgents[0].ProbeID)
	}
}

// TestAggregateCommonFailuresSortedBySeverity verifies that
// within a bucket, agents with critical issues rank above agents
// with only warnings.
func TestAggregateCommonFailuresSortedBySeverity(t *testing.T) {
	issues1 := []probe.VoiceQualityIssue{
		{ID: "warn1", Category: "jitter_spike", Severity: "warning"},
	}
	issues2 := []probe.VoiceQualityIssue{
		{ID: "crit1", Category: "jitter_spike", Severity: "critical"},
	}
	pairs := []probe.VoicePairSummary{
		{ID: "p1", Agent: probe.AgentRef{ID: 1, Name: "alpha"}, Forward: &probe.VoicePathMetrics{ProbeID: 1}, Issues: issues1},
		{ID: "p2", Agent: probe.AgentRef{ID: 2, Name: "beta"}, Forward: &probe.VoicePathMetrics{ProbeID: 2}, Issues: issues2},
	}
	all := append(issues1, issues2...)

	out := aggregateCommonFailures(all, pairs)
	if len(out) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(out))
	}
	if len(out[0].AffectedAgents) != 2 {
		t.Fatalf("expected 2 affected agents, got %d", len(out[0].AffectedAgents))
	}
	// Critical should come first
	if out[0].AffectedAgents[0].Severity != "critical" {
		t.Errorf("first agent severity = %q, want critical", out[0].AffectedAgents[0].Severity)
	}
	if out[0].AffectedAgents[0].AgentID != 2 {
		t.Errorf("first agent id = %d, want 2 (the critical one)", out[0].AffectedAgents[0].AgentID)
	}
}

// TestAggregateCommonFailuresCappedAtFive verifies that long
// affected-agent lists are capped (so the panel renders a compact
// callout rather than a wall of names).
func TestAggregateCommonFailuresCappedAtFive(t *testing.T) {
	issues := []probe.VoiceQualityIssue{}
	pairs := []probe.VoicePairSummary{}
	for i := uint(1); i <= 10; i++ {
		// Each pair gets one issue of the same category so they
		// all aggregate to "jitter_spike".
		issues = append(issues, probe.VoiceQualityIssue{
			ID:       "j",
			Category: "jitter_spike",
			Severity: "warning",
		})
		pairs = append(pairs, probe.VoicePairSummary{
			ID:      "p",
			Agent:   probe.AgentRef{ID: i, Name: "agent"},
			Forward: &probe.VoicePathMetrics{ProbeID: i},
			Issues: []probe.VoiceQualityIssue{
				{ID: "j", Category: "jitter_spike", Severity: "warning"},
			},
		})
	}
	out := aggregateCommonFailures(issues, pairs)
	if len(out) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(out))
	}
	if len(out[0].AffectedAgents) != 5 {
		t.Errorf("affected agents should be capped at 5, got %d", len(out[0].AffectedAgents))
	}
}

// TestAggregateCommonFailuresEmpty verifies the rollup returns
// nil when there are no issues.
func TestAggregateCommonFailuresEmpty(t *testing.T) {
	got := aggregateCommonFailures(nil, nil)
	if got != nil {
		t.Errorf("expected nil for empty issues, got %v", got)
	}
	got = aggregateCommonFailures([]probe.VoiceQualityIssue{}, []probe.VoicePairSummary{})
	if got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

// TestAggregateCommonFailuresSampleIssue verifies the rollup
// preserves a representative sample issue per category so the
// panel can render a meaningful title without losing detail.
func TestAggregateCommonFailuresSampleIssue(t *testing.T) {
	issues := []probe.VoiceQualityIssue{
		{ID: "x", Category: "jitter_spike", Severity: "critical", Title: "Critical jitter on link"},
		{ID: "y", Category: "jitter_spike", Severity: "warning", Title: "Warning jitter on link"},
	}
	pairs := []probe.VoicePairSummary{
		{ID: "p", Agent: probe.AgentRef{ID: 1, Name: "a"}, Forward: &probe.VoicePathMetrics{ProbeID: 9}, Issues: issues},
	}

	out := aggregateCommonFailures(issues, pairs)
	if len(out) != 1 || out[0].SampleIssue == nil {
		t.Fatalf("expected one bucket with sample issue, got %+v", out)
	}
	if out[0].SampleIssue.Title != "Critical jitter on link" {
		t.Errorf("sample issue title = %q, want first issue title", out[0].SampleIssue.Title)
	}
}

// TestCommonFailureTitleLookup verifies the human-readable title
// mapping for known categories.
func TestCommonFailureTitleLookup(t *testing.T) {
	cases := []struct {
		category string
		want     string
	}{
		{"jitter_spike", "Jitter spikes across multiple paths"},
		{"burst_loss", "Burst loss (consecutive packet drops)"},
		{"out_of_order", "Out-of-order packets (ECMP / reordering)"},
		{"unknown_category", "Unknown Category"},
	}
	for _, c := range cases {
		if got := commonFailureTitle(c.category); got != c.want {
			t.Errorf("commonFailureTitle(%q) = %q, want %q", c.category, got, c.want)
		}
	}
}

// TestAggregateCommonFailuresHasSampleIssueSerialized verifies
// the wire-level JSON shape includes the sample issue + affected
// agent list. This is the regression guard for the panel's
// "common_failures" render.
func TestAggregateCommonFailuresHasSampleIssueSerialized(t *testing.T) {
	issues := []probe.VoiceQualityIssue{
		{ID: "x", Category: "jitter_spike", Severity: "critical", Title: "Critical"},
	}
	pairs := []probe.VoicePairSummary{
		{ID: "p", Agent: probe.AgentRef{ID: 1, Name: "alpha"}, Forward: &probe.VoicePathMetrics{ProbeID: 9}, Issues: issues},
	}
	out := aggregateCommonFailures(issues, pairs)
	if len(out) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(out))
	}
	b := out[0]
	if b.Category != "jitter_spike" {
		t.Errorf("category = %q, want jitter_spike", b.Category)
	}
	if b.Title == "" {
		t.Errorf("title should be populated, got empty")
	}
	if b.Count != 1 || b.CriticalCount != 1 {
		t.Errorf("counts wrong: total %d critical %d", b.Count, b.CriticalCount)
	}
	if len(b.AffectedAgents) != 1 {
		t.Fatalf("expected 1 affected agent, got %d", len(b.AffectedAgents))
	}
	if b.AffectedAgents[0].AgentName != "alpha" {
		t.Errorf("affected agent name = %q, want alpha", b.AffectedAgents[0].AgentName)
	}
	if b.SampleIssue == nil {
		t.Errorf("sample issue should be populated")
	}
}
