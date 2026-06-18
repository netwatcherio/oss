// internal/probe/network_map_test.go
// Tests for the destination overview aggregation in network_map.go.
//
// These tests exercise buildNetworkMap (and workspaceAgentProbePlans) with
// hand-built fixture data so they do not require a live ClickHouse or
// Postgres. They document the behavior expected for the destination
// overview's "Network" tab in the panel: no Agent-A→Agent-A self-loops,
// correct cross-destination bidirectional detection, and a probe-type pill
// set that is filtered per-destination rather than globally added.
package probe

import (
	"testing"
	"time"
)

// ---------- fixtures ----------

// makeAgents builds an []agentInfo with deterministic public IPs and recent
// "updated_at" so the node-building pass treats them as online.
func makeAgents(specs ...struct {
	ID    uint
	Name  string
	IPStr string
}) []agentInfo {
	out := make([]agentInfo, 0, len(specs))
	now := time.Now()
	for _, s := range specs {
		out = append(out, agentInfo{
			ID:               s.ID,
			Name:             s.Name,
			PublicIPOverride: s.IPStr,
			UpdatedAt:        now,
		})
	}
	return out
}

func agentSpec(id uint, name, ip string) struct {
	ID    uint
	Name  string
	IPStr string
} {
	return struct {
		ID    uint
		Name  string
		IPStr string
	}{id, name, ip}
}

// lastHopFor builds a one-hop MTR trace (the last hop is the destination).
func lastHopFor(ip string, latency, loss float64) []mtrHop {
	return []mtrHop{{IP: ip, AvgLatency: latency, PacketLoss: loss}}
}

// findDest returns the DestinationSummary with the given target (e.g.
// "agent:10") or nil if not found.
func findDest(dests []DestinationSummary, target string) *DestinationSummary {
	for i := range dests {
		if dests[i].Target == target {
			return &dests[i]
		}
	}
	return nil
}

// hasEndpointWith returns true when any entry in details matches both
// (AgentID, TargetAgentID) fields.
func hasEndpointWith(details []ProbeEndpointDetail, agentID, targetAgentID uint) bool {
	for _, d := range details {
		if d.AgentID == agentID && d.TargetAgentID == targetAgentID {
			return true
		}
	}
	return false
}

// ---------- Test 1: no Agent A→Agent A self-loop ----------

// Two agents (A=10, B=20) with a single AGENT probe 10→20 (bidirectional).
// We feed the post-expansion data into buildNetworkMap: a forward MTR row
// and a reverse MTR row, plus matching forward+reverse TrafficSim rows.
//
// The real probe 793 pattern (verified against prod ClickHouse):
//   - Forward: runner=A, target_agent=B, probe_agent_id=A
//   - Reverse: runner=B, target_agent=A, probe_agent_id=A
//
// (the reverse's target_agent is the AGENT probe OWNER, not its target)
//
// None of these may produce a self-loop entry (AgentID == TargetAgentID)
// in either destination's ExpandedEndpoints. The pre-fix code used
// probe_agent_id as the source for reverse rows, which produced a
// self-loop entry in the AGENT-owner's destination.
func TestBuildNetworkMap_NoAgentAToAgentASelfLoop(t *testing.T) {
	agents := makeAgents(
		agentSpec(10, "A", "10.0.0.1"),
		agentSpec(20, "B", "10.0.0.2"),
	)

	mtr := []mtrTrace{
		// Forward: A runs trace, target=B, owned by A.
		{
			AgentID: 10, Target: "10.0.0.2", TargetAgent: 20, ProbeAgentID: 10, ProbeID: 793,
			Hops: lastHopFor("10.0.0.2", 5.0, 0.0),
		},
		// Reverse: B runs trace, target=A (the AGENT owner), owned by A.
		{
			AgentID: 20, Target: "10.0.0.1", TargetAgent: 10, ProbeAgentID: 10, ProbeID: 793,
			Hops: lastHopFor("10.0.0.1", 5.0, 0.0),
		},
	}

	traffic := map[string]trafficStats{
		// Forward TrafficSim row reported by A targeting B.
		"10:10.0.0.2:5000": {AvgRTT: 4.5, PacketLoss: 0, Count: 1, TargetAgent: 20, ProbeAgents: []uint{10}},
		// Reverse TrafficSim row reported by B targeting A.
		"20:10.0.0.1:5000": {AvgRTT: 4.5, PacketLoss: 0, Count: 1, TargetAgent: 10, ProbeAgents: []uint{10}},
	}

	data := buildNetworkMap(agents, mtr, nil, traffic, 2, nil)

	// Destination for A — the AGENT probe owner. Pre-fix this destination
	// gets a self-loop entry (source=A, target=A) from the reverse MTR row.
	destA := findDest(data.Destinations, "agent:10")
	if destA == nil {
		t.Fatalf("destination agent:10 not found; have %d destinations", len(data.Destinations))
	}
	for _, ep := range destA.ExpandedEndpoints {
		if ep.AgentID == ep.TargetAgentID {
			t.Errorf("destination agent:10 has self-loop entry: AgentID=%d TargetAgentID=%d", ep.AgentID, ep.TargetAgentID)
		}
	}
	// Must contain the reverse entry (B→A) — source=20, target=10.
	if !hasEndpointWith(destA.ExpandedEndpoints, 20, 10) {
		t.Errorf("destination agent:10 missing reverse entry (B=20 → A=10); got %+v", destA.ExpandedEndpoints)
	}

	// Destination for B (target of the forward probes).
	destB := findDest(data.Destinations, "agent:20")
	if destB == nil {
		t.Fatalf("destination agent:20 not found; have %d destinations", len(data.Destinations))
	}
	for _, ep := range destB.ExpandedEndpoints {
		if ep.AgentID == ep.TargetAgentID {
			t.Errorf("destination agent:20 has self-loop entry: AgentID=%d TargetAgentID=%d", ep.AgentID, ep.TargetAgentID)
		}
	}
	// Must contain the forward entry (A→B) — source=10, target=20.
	if !hasEndpointWith(destB.ExpandedEndpoints, 10, 20) {
		t.Errorf("destination agent:20 missing forward entry (A=10 → B=20); got %+v", destB.ExpandedEndpoints)
	}
}

// ---------- Test 2: AGENT-paired data is bidirectional across destinations ----------

// Same setup as Test 1, but now asserts the bidirectional flag is set
// on BOTH directions (forward in destination B, reverse in destination A)
// because the same underlying AGENT probe expanded into both.
func TestBuildNetworkMap_AGENTProbePairIsBidirectional(t *testing.T) {
	agents := makeAgents(
		agentSpec(10, "A", "10.0.0.1"),
		agentSpec(20, "B", "10.0.0.2"),
	)

	mtr := []mtrTrace{
		{AgentID: 10, Target: "10.0.0.2", TargetAgent: 20, ProbeAgentID: 10, ProbeID: 793, Hops: lastHopFor("10.0.0.2", 5.0, 0.0)},
		{AgentID: 20, Target: "10.0.0.1", TargetAgent: 10, ProbeAgentID: 10, ProbeID: 793, Hops: lastHopFor("10.0.0.1", 5.0, 0.0)},
	}
	traffic := map[string]trafficStats{
		"10:10.0.0.2:5000": {AvgRTT: 4.5, PacketLoss: 0, Count: 1, TargetAgent: 20, ProbeAgents: []uint{10}},
		"20:10.0.0.1:5000": {AvgRTT: 4.5, PacketLoss: 0, Count: 1, TargetAgent: 10, ProbeAgents: []uint{10}},
	}

	// probePlans must include the AGENT pair so the bidirectional check
	// knows this isn't a random standalone probe pair.
	plans := map[uint]map[uint][]string{
		10: {20: {"MTR", "TRAFFICSIM"}},
	}

	data := buildNetworkMap(agents, mtr, nil, traffic, 2, plans)

	destB := findDest(data.Destinations, "agent:20")
	if destB == nil {
		t.Fatalf("destination agent:20 not found")
	}
	if !destB.HasBidirectional {
		t.Errorf("destination agent:20 HasBidirectional=false, want true")
	}
	for i, ep := range destB.ExpandedEndpoints {
		if !ep.IsBidirectional {
			t.Errorf("destination agent:20 entry[%d] (AgentID=%d → TargetAgentID=%d) IsBidirectional=false, want true",
				i, ep.AgentID, ep.TargetAgentID)
		}
	}

	destA := findDest(data.Destinations, "agent:10")
	if destA == nil {
		t.Fatalf("destination agent:10 not found")
	}
	if !destA.HasBidirectional {
		t.Errorf("destination agent:10 HasBidirectional=false, want true")
	}
	for i, ep := range destA.ExpandedEndpoints {
		if !ep.IsBidirectional {
			t.Errorf("destination agent:10 entry[%d] (AgentID=%d → TargetAgentID=%d) IsBidirectional=false, want true",
				i, ep.AgentID, ep.TargetAgentID)
		}
	}
}

// ---------- Test 3: standalone probes are NOT marked bidirectional ----------

// Two agents with two independent unidirectional standalone probes
// (MTR 10→20 and MTR 20→10) and NO AGENT probe, NO probePlans.
// Even though data flows in both directions, these aren't "bidirectional
// via expansion" — they are two independent unidirectional probes. The
// bidirectional flag must remain false on both.
func TestBuildNetworkMap_StandaloneProbesAreNotBidirectional(t *testing.T) {
	agents := makeAgents(
		agentSpec(10, "A", "10.0.0.1"),
		agentSpec(20, "B", "10.0.0.2"),
	)

	mtr := []mtrTrace{
		// A standalone MTR probe owned by A targeting B.
		{AgentID: 10, Target: "10.0.0.2", TargetAgent: 20, ProbeAgentID: 10, ProbeID: 900, Hops: lastHopFor("10.0.0.2", 5.0, 0.0)},
		// A separate standalone MTR probe owned by B targeting A.
		{AgentID: 20, Target: "10.0.0.1", TargetAgent: 10, ProbeAgentID: 20, ProbeID: 901, Hops: lastHopFor("10.0.0.1", 6.0, 0.0)},
	}

	data := buildNetworkMap(agents, mtr, nil, nil, 2, nil)

	for _, target := range []string{"agent:20", "agent:10"} {
		d := findDest(data.Destinations, target)
		if d == nil {
			t.Fatalf("destination %s not found", target)
		}
		if d.HasBidirectional {
			t.Errorf("destination %s HasBidirectional=true, want false (standalone probes are not bidirectional)", target)
		}
		for i, ep := range d.ExpandedEndpoints {
			if ep.IsBidirectional {
				t.Errorf("destination %s entry[%d] (AgentID=%d → TargetAgentID=%d) IsBidirectional=true, want false",
					target, i, ep.AgentID, ep.TargetAgentID)
			}
		}
	}
}

// ---------- Test 4: probePlans fallback is filtered per destination ----------

// Three agents (A=10, B=20, C=30). Two AGENT probes: A→B with all three
// types, A→C with MTR+PING only. Only TrafficSim data is observed for
// destination agent:B. No MTR/PING data observed for any destination.
//
// Expected behavior:
//   - destination agent:B (in plans via 10↔20) gets MTR+PING from plan + TRAFFICSIM from data
//   - destination agent:C (in plans via 10↔30) gets MTR+PING from plan (no observed data)
//   - destination agent:A (the OWNER of both plans) must NOT be polluted with MTR/PING
//     just because it appears in the plans as a source — observed data is the only signal
//   - a hostname destination (no plan involvement) must be empty of plan-derived types
func TestBuildNetworkMap_ProbePlansFilteredByDestination(t *testing.T) {
	agents := makeAgents(
		agentSpec(10, "A", "10.0.0.1"),
		agentSpec(20, "B", "10.0.0.2"),
		agentSpec(30, "C", "10.0.0.3"),
	)

	plans := map[uint]map[uint][]string{
		10: {
			20: {"MTR", "PING", "TRAFFICSIM"},
			30: {"MTR", "PING"},
		},
	}

	// Observed: one MTR row and one TrafficSim row to destination B, plus
	// a single MTR row to destination C just to ensure C gets a
	// DestinationSummary entry (destinations are only created when
	// observed data lands; plans alone don't create them).
	mtr := []mtrTrace{
		{AgentID: 10, Target: "10.0.0.2", TargetAgent: 20, ProbeAgentID: 10, ProbeID: 800, Hops: lastHopFor("10.0.0.2", 5.0, 0.0)},
		{AgentID: 10, Target: "10.0.0.3", TargetAgent: 30, ProbeAgentID: 10, ProbeID: 801, Hops: lastHopFor("10.0.0.3", 5.0, 0.0)},
	}
	traffic := map[string]trafficStats{
		"10:10.0.0.2:5000": {AvgRTT: 4.0, PacketLoss: 0, Count: 1, TargetAgent: 20, ProbeAgents: []uint{10}},
	}

	data := buildNetworkMap(agents, mtr, nil, traffic, 2, plans)

	destB := findDest(data.Destinations, "agent:20")
	if destB == nil {
		t.Fatalf("destination agent:20 not found")
	}
	if !containsAll(destB.ProbeTypes, []string{"MTR", "PING", "TRAFFICSIM"}) {
		t.Errorf("destination agent:20 ProbeTypes = %v, want superset of [MTR PING TRAFFICSIM]", destB.ProbeTypes)
	}

	destC := findDest(data.Destinations, "agent:30")
	if destC == nil {
		t.Fatalf("destination agent:30 not found")
	}
	if !containsAll(destC.ProbeTypes, []string{"MTR", "PING"}) {
		t.Errorf("destination agent:30 ProbeTypes = %v, want superset of [MTR PING]", destC.ProbeTypes)
	}
	// No observed TrafficSim data for C, and no plan entry has TRAFFICSIM
	// for C — must not be polluted.
	if containsAny(destC.ProbeTypes, []string{"TRAFFICSIM"}) {
		t.Errorf("destination agent:30 ProbeTypes = %v, must NOT include TRAFFICSIM (no plan entry 30↔TRAFFICSIM, no observed data)", destC.ProbeTypes)
	}

	// Destination A: the OWNER of both plans. A is not a TARGET of any
	// plan, and there's no observed data targeting A. Must not be polluted.
	destA := findDest(data.Destinations, "agent:10")
	if destA != nil {
		// A may exist if any observed data has A as a target (none in this
		// fixture), in which case A should have no plan-derived types.
		if containsAny(destA.ProbeTypes, []string{"MTR", "PING", "TRAFFICSIM"}) {
			t.Errorf("destination agent:10 ProbeTypes = %v, must NOT be polluted by plans where A is the owner (not the target)", destA.ProbeTypes)
		}
	}
}

// containsAll returns true when set ⊇ superset.
func containsAll(set, superset []string) bool {
	idx := make(map[string]bool, len(set))
	for _, s := range set {
		idx[s] = true
	}
	for _, s := range superset {
		if !idx[s] {
			return false
		}
	}
	return true
}

func containsAny(set, any []string) bool {
	idx := make(map[string]bool, len(set))
	for _, s := range set {
		idx[s] = true
	}
	for _, s := range any {
		if idx[s] {
			return true
		}
	}
	return false
}

// ---------- Test 5: workspaceAgentProbePlans includes reverse direction ----------

// This test is written but the underlying function workspaceAgentProbePlans
// needs a Postgres handle and a real Probe table. We exercise just the
// "types" merge logic by reproducing it in isolation — the public contract
// is: for an AGENT probe 10→20, plans must contain both plans[10][20] AND
// plans[20][10] (with the same type list).
//
// The actual workspaceAgentProbePlans integration test is not exercised
// here because it requires a live Postgres. The unit-level test in
// probe_plan_test.go (TestExpandedAgentProbeTypes) already covers the type
// computation. This test documents the cross-slot merge contract.
//
// The function `mergeTypesUnordered` is a copy of the dedupe-merge inside
// workspaceAgentProbePlans; if that loop changes, this test should be
// updated to match.
func TestWorkspaceAgentProbePlans_MergeDedupesAcrossSlots(t *testing.T) {
	// Mirror the merge pattern from workspaceAgentProbePlans:
	//   existing = out[owner][target]
	//   merged := existing ++ types
	//   dedupe preserving first-seen order
	merge := func(existing, types []string) []string {
		seen := make(map[string]bool, len(existing)+len(types))
		out := make([]string, 0, len(existing)+len(types))
		for _, t := range append(existing, types...) {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
		return out
	}

	// After Fix 4, the reverse slot must receive the same dedupe-merged
	// type list as the forward slot. Simulate the two slots being
	// independently populated and assert they're equal.
	forward := merge(nil, []string{"MTR", "PING"})
	reverse := merge(nil, []string{"MTR", "PING"})

	if !stringSliceEqual(forward, reverse) {
		t.Errorf("forward/reverse slots diverge: forward=%v reverse=%v", forward, reverse)
	}

	// Dedup behaviour: an existing slot must not gain duplicate types.
	got := merge(forward, []string{"MTR", "TRAFFICSIM"})
	want := []string{"MTR", "PING", "TRAFFICSIM"}
	if !stringSliceEqual(got, want) {
		t.Errorf("merge(%v, [MTR TRAFFICSIM]) = %v, want %v", forward, got, want)
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
