package probe

import (
	"testing"
	"time"
)

// TestBuildAgentIPToIDMap_PrefersOverrideThenNetInfo verifies that
// `buildAgentIPToIDMap` populates the IP→agent map from both the agent's
// manual PublicIPOverride and its NETINFO-discovered PublicAddress, with
// the override winning when both are set.
func TestBuildAgentIPToIDMap_PrefersOverrideThenNetInfo(t *testing.T) {
	agents := []AgentHealthSummary{
		{AgentID: 1, AgentName: "alpha"},
		{AgentID: 2, AgentName: "beta"},
	}
	agentByID := map[uint]agentInfo{
		1: {ID: 1, Name: "alpha", PublicIPOverride: "198.51.100.10"},
		2: {ID: 2, Name: "beta"},
	}
	netInfoByAgent := map[uint]*netInfoPayload{
		1: {PublicAddress: "198.51.100.10", Timestamp: time.Now()}, // same as override
		2: {PublicAddress: "203.0.113.42", Timestamp: time.Now()},  // netinfo-only
	}

	got := buildAgentIPToIDMap(agents, agentByID, netInfoByAgent)

	if id, ok := got["198.51.100.10"]; !ok || id != 1 {
		t.Errorf("override+netinfo same IP: got (%d, %v), want (1, true)", id, ok)
	}
	if id, ok := got["203.0.113.42"]; !ok || id != 2 {
		t.Errorf("netinfo-only IP for agent 2: got (%d, %v), want (2, true)", id, ok)
	}
}

// TestBuildAgentIPToIDMap_NetInfoOnlyFillsGap exercises the original bug
// scenario: an agent with no PublicIPOverride but a NETINFO-discovered
// public IP. The probe target recorded in ClickHouse is the NETINFO IP,
// and we want it to resolve to the agent name in the incident title.
func TestBuildAgentIPToIDMap_NetInfoOnlyFillsGap(t *testing.T) {
	agents := []AgentHealthSummary{{AgentID: 7, AgentName: "no-override"}}
	agentByID := map[uint]agentInfo{7: {ID: 7, Name: "no-override"}}
	netInfoByAgent := map[uint]*netInfoPayload{
		7: {PublicAddress: "192.0.2.7", Timestamp: time.Now()},
	}

	got := buildAgentIPToIDMap(agents, agentByID, netInfoByAgent)

	id, ok := got["192.0.2.7"]
	if !ok || id != 7 {
		t.Fatalf("netinfo IP for agent 7 missing: got (%d, %v), want (7, true)", id, ok)
	}
	if resolved := resolveTargetToName("192.0.2.7", agentByID, got); resolved != "no-override" {
		t.Errorf("resolveTargetToName = %q, want %q", resolved, "no-override")
	}
}

// TestBuildAgentIPToIDMap_OverrideTakesPrecedence ensures a manual
// PublicIPOverride is preserved as the canonical entry when both are
// present. The two values are different in this test so we can tell
// which one "won" the slot.
func TestBuildAgentIPToIDMap_OverrideTakesPrecedence(t *testing.T) {
	agents := []AgentHealthSummary{{AgentID: 3, AgentName: "gamma"}}
	agentByID := map[uint]agentInfo{
		3: {ID: 3, Name: "gamma", PublicIPOverride: "198.51.100.99"},
	}
	netInfoByAgent := map[uint]*netInfoPayload{
		3: {PublicAddress: "203.0.113.99", Timestamp: time.Now()},
	}

	got := buildAgentIPToIDMap(agents, agentByID, netInfoByAgent)

	// Override IP must still resolve to the agent.
	if id, ok := got["198.51.100.99"]; !ok || id != 3 {
		t.Errorf("override IP missing: got (%d, %v), want (3, true)", id, ok)
	}
	// NETINFO IP must also resolve — this is what fixes the "Shared
	// degradation to <IP>" bug for the actual public IP observed in the
	// probe payload.
	if id, ok := got["203.0.113.99"]; !ok || id != 3 {
		t.Errorf("netinfo IP missing alongside override: got (%d, %v), want (3, true)", id, ok)
	}
	// And both resolve to the same agent name via the helper.
	if name := resolveTargetToName("198.51.100.99", agentByID, got); name != "gamma" {
		t.Errorf("override resolve = %q, want gamma", name)
	}
	if name := resolveTargetToName("203.0.113.99", agentByID, got); name != "gamma" {
		t.Errorf("netinfo resolve = %q, want gamma", name)
	}
}

// TestBuildAgentIPToIDMap_NoInfoFallsThrough confirms that when an agent
// has neither a PublicIPOverride nor a NETINFO record, the map stays
// empty and resolveTargetToName returns the target string unchanged
// (preserving the original probe host for non-agent targets like
// google.com).
func TestBuildAgentIPToIDMap_NoInfoFallsThrough(t *testing.T) {
	agents := []AgentHealthSummary{{AgentID: 9, AgentName: "unknown"}}
	agentByID := map[uint]agentInfo{9: {ID: 9, Name: "unknown"}}
	netInfoByAgent := map[uint]*netInfoPayload{} // empty

	got := buildAgentIPToIDMap(agents, agentByID, netInfoByAgent)

	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
	if name := resolveTargetToName("google.com", agentByID, got); name != "google.com" {
		t.Errorf("hostname target should pass through unchanged, got %q", name)
	}
	if name := resolveTargetToName("8.8.8.8", agentByID, got); name != "8.8.8.8" {
		t.Errorf("unmatched IP should pass through unchanged, got %q", name)
	}
}

// TestBuildAgentIPToIDMap_NetInfoNilEntryIgnored guards against a nil
// netInfoPayload pointer slipping into the loop.
func TestBuildAgentIPToIDMap_NetInfoNilEntryIgnored(t *testing.T) {
	agents := []AgentHealthSummary{{AgentID: 4, AgentName: "delta"}}
	agentByID := map[uint]agentInfo{4: {ID: 4, Name: "delta"}}
	netInfoByAgent := map[uint]*netInfoPayload{
		4: nil, // explicit nil — should be skipped, not panic
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil netinfo entry panicked: %v", r)
		}
	}()

	got := buildAgentIPToIDMap(agents, agentByID, netInfoByAgent)
	if len(got) != 0 {
		t.Errorf("expected empty map for nil netinfo, got %v", got)
	}
}
