package probe

import (
	"testing"

	"netwatcher-controller/internal/agent"
)

// TestBuildVoicePairSummariesReverseOnly verifies that a target-only
// agent (probes from other agents point at it, but it owns none)
// still gets a pair built from the reverse metrics — the path TO the
// agent is that agent's voice report.
func TestBuildVoicePairSummariesReverseOnly(t *testing.T) {
	reportAgent := &agent.Agent{ID: 65, Name: "Fax Server", Location: "HQ"}

	reverse := map[uint]*VoicePathMetrics{
		42: {
			ProbeID:         42,
			ProbeType:       string(TypeAgent),
			SourceAgentID:   7,
			SourceAgentName: "Branch Office",
			TargetAgentID:   65,
			TargetAgentName: "Fax Server",
			Direction:       VoicePathReturn,
			MosScore:        3.2,
			SampleCount:     120,
		},
	}
	issues := map[uint][]VoiceQualityIssue{
		42: {{Category: "jitter", Severity: "warning"}},
	}

	pairs := buildVoicePairSummaries(
		map[uint]*VoicePathMetrics{}, reverse, issues,
		map[uint]*VoicePathMetrics{}, map[uint]*VoicePathMetrics{},
		nil, reportAgent, 65,
		VoiceDefaultThresholds, nil,
	)

	if len(pairs) != 1 {
		t.Fatalf("expected 1 reverse-only pair, got %d", len(pairs))
	}
	p := pairs[0]
	if p.Forward != nil {
		t.Errorf("reverse-only pair should have nil Forward")
	}
	if p.Reverse == nil || p.Reverse.ProbeID != 42 {
		t.Fatalf("reverse metrics not attached to pair")
	}
	if p.Agent.ID != 65 || p.Agent.Name != "Fax Server" {
		t.Errorf("pair agent = %+v, want the report agent", p.Agent)
	}
	if p.Target.AgentID != 7 || p.Target.AgentName != "Branch Office" {
		t.Errorf("pair target = %+v, want the remote probe owner", p.Target)
	}
	if p.OverallMos != 3.2 {
		t.Errorf("pair MOS = %v, want 3.2 (reverse-only)", p.OverallMos)
	}
	if p.OverallGrade == "" || p.OverallGrade == "unknown" {
		t.Errorf("pair grade = %q, want a real grade", p.OverallGrade)
	}
	if len(p.Issues) != 1 {
		t.Errorf("pair issues = %d, want 1 (from perProbeIssues)", len(p.Issues))
	}
}

// TestBuildVoicePairSummariesReverseAttachedNotDuplicated verifies
// that a probe with both directions produces exactly one pair (the
// reverse metrics attach to the forward pair, not a second entry).
func TestBuildVoicePairSummariesReverseAttachedNotDuplicated(t *testing.T) {
	reportAgent := &agent.Agent{ID: 1, Name: "Source"}
	targetID := uint(2)

	forward := map[uint]*VoicePathMetrics{
		10: {ProbeID: 10, SourceAgentID: 1, MosScore: 4.2, SampleCount: 50},
	}
	reverse := map[uint]*VoicePathMetrics{
		10: {ProbeID: 10, SourceAgentID: 2, MosScore: 3.8, SampleCount: 50},
	}
	probes := []Probe{
		{ID: 10, AgentID: 1, Type: TypeTrafficSim, Targets: []Target{{ProbeID: 10, AgentID: &targetID}}},
	}

	pairs := buildVoicePairSummaries(
		forward, reverse, map[uint][]VoiceQualityIssue{},
		map[uint]*VoicePathMetrics{}, map[uint]*VoicePathMetrics{},
		probes, reportAgent, 1,
		VoiceDefaultThresholds, map[uint]string{2: "Target"},
	)

	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair for a bidirectional probe, got %d", len(pairs))
	}
	if pairs[0].Forward == nil || pairs[0].Reverse == nil {
		t.Errorf("pair should carry both directions")
	}
	want := (4.2 + 3.8) / 2
	if pairs[0].OverallMos != want {
		t.Errorf("pair MOS = %v, want %v", pairs[0].OverallMos, want)
	}
}

// TestBuildVoicePairSummariesRemoteAgentDedup verifies that when a
// forward pair already covers remote agent B, B's own AGENT probe
// toward us (a different probe ID) does not spawn a duplicate
// reverse-only pair — B's session is covered by the forward pair's
// same-probe reverse, and B's probe shows up in B's own report.
func TestBuildVoicePairSummariesRemoteAgentDedup(t *testing.T) {
	reportAgent := &agent.Agent{ID: 1, Name: "Source"}
	targetID := uint(2)

	forward := map[uint]*VoicePathMetrics{
		10: {ProbeID: 10, SourceAgentID: 1, TargetAgentID: 2, MosScore: 4.2, SampleCount: 50},
	}
	reverse := map[uint]*VoicePathMetrics{
		// Same-probe bidirectional reverse — attaches to the forward pair.
		10: {ProbeID: 10, SourceAgentID: 2, TargetAgentID: 1, MosScore: 3.9, SampleCount: 50},
		// B's own AGENT probe toward us — must NOT become a second pair.
		99: {ProbeID: 99, SourceAgentID: 2, SourceAgentName: "B", TargetAgentID: 1, MosScore: 3.5, SampleCount: 40},
	}
	probes := []Probe{
		{ID: 10, AgentID: 1, Type: TypeTrafficSim, Targets: []Target{{ProbeID: 10, AgentID: &targetID}}},
	}

	pairs := buildVoicePairSummaries(
		forward, reverse, map[uint][]VoiceQualityIssue{},
		map[uint]*VoicePathMetrics{}, map[uint]*VoicePathMetrics{},
		probes, reportAgent, 1,
		VoiceDefaultThresholds, map[uint]string{2: "B"},
	)

	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair (remote agent deduped), got %d", len(pairs))
	}
	if pairs[0].Reverse == nil || pairs[0].Reverse.ProbeID != 10 {
		t.Errorf("forward pair should carry the same-probe reverse (probe 10)")
	}
}

// TestFinalizeVoicePairReverseBaseline verifies the baseline delta is
// computed off the reverse path when no forward path exists, using
// the reverse-direction baseline map.
func TestFinalizeVoicePairReverseBaseline(t *testing.T) {
	pair := VoicePairSummary{
		Reverse: &VoicePathMetrics{
			ProbeID:     42,
			MosScore:    3.0,
			AvgLatency:  80,
			JitterAvg:   25,
			PacketLoss:  2.0,
			SampleCount: 100,
		},
	}
	reverseBaselines := map[uint]*VoicePathMetrics{
		42: {ProbeID: 42, MosScore: 4.0, AvgLatency: 40, JitterAvg: 5, PacketLoss: 0.2, SampleCount: 500},
	}

	finalizeVoicePair(&pair, map[uint]*VoicePathMetrics{}, reverseBaselines)

	if pair.Baseline == nil {
		t.Fatalf("expected baseline delta from reverse path")
	}
	if pair.Baseline.Trend != "worsening" {
		t.Errorf("trend = %q, want worsening (MOS delta %.2f)", pair.Baseline.Trend, pair.Baseline.MosDelta)
	}
	if pair.OverallMos != 3.0 {
		t.Errorf("pair MOS = %v, want 3.0", pair.OverallMos)
	}
}
