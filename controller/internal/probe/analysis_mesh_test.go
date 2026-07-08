package probe

import (
	"testing"
	"time"
)

func meshTestAgents() []agentInfo {
	now := time.Now().UTC()
	return []agentInfo{
		{ID: 1, Name: "HQ", UpdatedAt: now},
		{ID: 2, Name: "Branch", UpdatedAt: now},
		{ID: 3, Name: "Fax Server", UpdatedAt: now},
	}
}

// TestBuildHealthMeshPairwiseLinks verifies that inter-agent metrics
// become directed links and that non-agent targets are excluded.
func TestBuildHealthMeshPairwiseLinks(t *testing.T) {
	ping := map[string]pingStats{
		"1:203.0.113.5":  {AvgLatency: 20, PacketLoss: 0, Count: 50, TargetAgent: 2},
		"2:198.51.100.7": {AvgLatency: 22, PacketLoss: 0.1, Count: 40, TargetAgent: 1},
		"1:8.8.8.8":      {AvgLatency: 15, PacketLoss: 0, Count: 60, TargetAgent: 0}, // internet target — no link
	}
	mtr := map[string]mtrStats{
		"2:203.0.113.9": {AvgLatency: 45, PacketLoss: 5, Jitter: 12, Count: 30, TargetAgent: 3},
	}

	mesh := buildHealthMesh(meshTestAgents(), ping, mtr, map[string]trafficStats{})

	if len(mesh.Links) != 3 {
		t.Fatalf("expected 3 directed links (1→2, 2→1, 2→3), got %d", len(mesh.Links))
	}
	// Worst link first: 2→3 has 5%% loss.
	if mesh.Links[0].SourceAgentID != 2 || mesh.Links[0].TargetAgentID != 3 {
		t.Errorf("worst link should be 2→3, got %d→%d", mesh.Links[0].SourceAgentID, mesh.Links[0].TargetAgentID)
	}
	for _, l := range mesh.Links {
		if l.TargetAgentID == 0 {
			t.Errorf("internet target leaked into links: %+v", l)
		}
		if l.Health.Grade == "" || l.Health.Grade == "unknown" {
			t.Errorf("link %d→%d has no grade", l.SourceAgentID, l.TargetAgentID)
		}
	}
}

// TestBuildHealthMeshNodeRollup verifies node health includes links in
// BOTH directions — a target-only agent gets a real grade from the
// paths toward it.
func TestBuildHealthMeshNodeRollup(t *testing.T) {
	mtr := map[string]mtrStats{
		"1:203.0.113.9": {AvgLatency: 40, PacketLoss: 4, Jitter: 10, Count: 30, TargetAgent: 3},
		"2:203.0.113.9": {AvgLatency: 42, PacketLoss: 6, Jitter: 14, Count: 30, TargetAgent: 3},
	}

	mesh := buildHealthMesh(meshTestAgents(), map[string]pingStats{}, mtr, map[string]trafficStats{})

	var fax *AgentMeshNode
	for i := range mesh.Nodes {
		if mesh.Nodes[i].AgentID == 3 {
			fax = &mesh.Nodes[i]
		}
	}
	if fax == nil {
		t.Fatalf("fax server node missing")
	}
	if fax.LinkCount != 2 {
		t.Errorf("fax server link count = %d, want 2 (two inbound paths)", fax.LinkCount)
	}
	if fax.Health.Grade == "unknown" {
		t.Errorf("target-only agent should get a grade from inbound links, got unknown")
	}
	if mesh.OverallHealth.Grade == "unknown" {
		t.Errorf("overall health should be graded when links exist")
	}
}

// TestBuildHealthMeshSampleWeighting verifies pair aggregation is
// sample-weighted across probe types, not a plain average.
func TestBuildHealthMeshSampleWeighting(t *testing.T) {
	ping := map[string]pingStats{
		"1:203.0.113.5": {AvgLatency: 10, PacketLoss: 0, Count: 90, TargetAgent: 2},
	}
	mtr := map[string]mtrStats{
		"1:203.0.113.5": {AvgLatency: 100, PacketLoss: 0, Jitter: 0, Count: 10, TargetAgent: 2},
	}

	mesh := buildHealthMesh(meshTestAgents(), ping, mtr, map[string]trafficStats{})

	if len(mesh.Links) != 1 {
		t.Fatalf("expected 1 merged link, got %d", len(mesh.Links))
	}
	l := mesh.Links[0]
	// 90 samples at 10ms + 10 samples at 100ms → 19ms weighted.
	if l.Metrics.AvgLatency < 18.9 || l.Metrics.AvgLatency > 19.1 {
		t.Errorf("weighted latency = %v, want ~19", l.Metrics.AvgLatency)
	}
	if len(l.ProbeTypes) != 2 {
		t.Errorf("probe types = %v, want [MTR PING]", l.ProbeTypes)
	}
	if l.Metrics.SampleCount != 100 {
		t.Errorf("sample count = %d, want 100", l.Metrics.SampleCount)
	}
}

// TestBuildHealthMeshEmptyWorkspace verifies clean empty output.
func TestBuildHealthMeshEmptyWorkspace(t *testing.T) {
	mesh := buildHealthMesh(nil, map[string]pingStats{}, map[string]mtrStats{}, map[string]trafficStats{})
	if len(mesh.Nodes) != 0 || len(mesh.Links) != 0 {
		t.Errorf("expected empty mesh, got %d nodes %d links", len(mesh.Nodes), len(mesh.Links))
	}
	if mesh.OverallHealth.Grade != "unknown" {
		t.Errorf("empty mesh grade = %q, want unknown", mesh.OverallHealth.Grade)
	}
}
