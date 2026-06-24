package probe

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

// Test the MtrPayload unmarshaling with actual agent JSON
func TestMtrPayloadUnmarshalFromAgent(t *testing.T) {
	// This is what the agent actually sends - time.Time fields get marshaled
	// to RFC3339Nano strings by the standard json package
	agentJSON := `{
		"start_timestamp": "2026-06-16T11:48:01.423973-07:00",
		"stop_timestamp": "2026-06-16T11:48:06.423973-07:00",
		"report": {
			"info": {
				"target": {
					"ip": "8.8.8.8",
					"hostname": "dns.google"
				}
			},
			"hops": [
				{
					"ttl": 1,
					"hosts": [{"ip": "192.168.1.1", "hostname": "router.local"}],
					"loss_pct": "0.0%",
					"sent": 5,
					"last": "1.2",
					"recv": 5,
					"avg": "1.2",
					"best": "1.0",
					"worst": "1.5",
					"stddev": "0.2",
					"jitter": "0.1",
					"javg": "0.1",
					"jmax": "0.2",
					"jint": "0.1"
				},
				{
					"ttl": 2,
					"hosts": [{"ip": "8.8.8.8", "hostname": "dns.google"}],
					"loss_pct": "0.0%",
					"sent": 5,
					"last": "10.5",
					"recv": 5,
					"avg": "10.5",
					"best": "10.0",
					"worst": "11.0",
					"stddev": "0.3",
					"jitter": "0.5",
					"javg": "0.5",
					"jmax": "0.8",
					"jint": "0.5"
				}
			]
		}
	}`

	var mp MtrPayload
	if err := json.Unmarshal([]byte(agentJSON), &mp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if mp.StartTimestamp == "" {
		t.Errorf("Expected StartTimestamp to be parsed")
	}
	if len(mp.Report.Hops) != 2 {
		t.Fatalf("Expected 2 hops, got %d", len(mp.Report.Hops))
	}

	first := mp.Report.Hops[0]
	if first.TTL != 1 {
		t.Errorf("Expected TTL 1, got %d", first.TTL)
	}
	if len(first.Hosts) == 0 || first.Hosts[0].IP != "192.168.1.1" {
		t.Errorf("Expected first hop IP 192.168.1.1, got %+v", first.Hosts)
	}

	// Test getMtrRouteSignature
	sig := getMtrRouteSignature(mp.Report.Hops)
	expected := "192.168.1.1->8.8.8.8"
	if sig != expected {
		t.Errorf("Expected signature %q, got %q", expected, sig)
	}

	t.Logf("Signature: %s", sig)
	t.Logf("First hop: %+v", first)
	t.Logf("Last hop latency: %v", parseLatency(mp.Report.Hops[1].Avg))
	t.Logf("Last hop loss: %v", parseLossPct(mp.Report.Hops[1].LossPct))
}

// Test buildHopDetails with agent matching
func TestBuildHopDetailsWithAgents(t *testing.T) {
	agentIPToID := map[string]uint{
		"192.168.1.1": 1,
	}
	agentByID := map[uint]agentInfo{
		1: {ID: 1, Name: "agent-1"},
	}

	mp := &MtrPayload{
		Report: MtrReport{
			Hops: []MtrHop{
				{Hosts: []MtrHopHost{{IP: "192.168.1.1"}}, TTL: 1, Avg: "1.0"},
				{Hosts: []MtrHopHost{{IP: "8.8.8.8"}}, TTL: 2, Avg: "10.0"},
			},
		},
	}

	details := buildHopDetails(mp, agentIPToID, agentByID)
	if len(details) != 2 {
		t.Fatalf("Expected 2 details, got %d", len(details))
	}
	if !details[0].IsAgent {
		t.Errorf("Expected first hop to be agent, got %+v", details[0])
	}
	if details[0].AgentName != "agent-1" {
		t.Errorf("Expected agent name 'agent-1', got %q", details[0].AgentName)
	}
	if !details[1].IsFinalHop {
		t.Errorf("Expected last hop to be final")
	}
}

func TestDecideRouteChangeStatus_BaselineMatch(t *testing.T) {
	hops := "1.1.1.1->2.2.2.2->3.3.3.3"
	hasChange, stability := decideRouteChangeStatus(
		hops, hops,
		map[string]int{"sig-A": 5, "sig-B": 1},
		6,
	)
	if hasChange {
		t.Errorf("Expected no route change when baseline hops match latest hops")
	}
	if stability != 100 {
		t.Errorf("Expected stability 100 when baseline matches, got %v", stability)
	}
}

func TestDecideRouteChangeStatus_BaselineMismatch(t *testing.T) {
	baselineHops := "1.1.1.1->2.2.2.2->3.3.3.3"
	latestHops := "1.1.1.1->2.2.2.2->99.99.99.99"
	hasChange, stability := decideRouteChangeStatus(
		latestHops, baselineHops,
		map[string]int{"sig-A": 1, "sig-B": 4},
		5,
	)
	if !hasChange {
		t.Errorf("Expected route change when latest path differs significantly from baseline")
	}
	if stability != 80 {
		t.Errorf("Expected stability 80 (4/5) from dominant signature, got %v", stability)
	}
}

func TestDecideRouteChangeStatus_EcmpTolerated(t *testing.T) {
	baselineHops := "1.1.1.1->2.2.2.2->3.3.3.3->4.4.4.4->5.5.5.5->6.6.6.6->7.7.7.7->8.8.8.8->9.9.9.9->10.10.10.10"
	latestHops := "1.1.1.1->2.2.2.2->3.3.3.3->4.4.4.4->5.5.5.5->6.6.6.6->7.7.7.7->8.8.8.8->9.9.9.9->99.99.99.99"
	hasChange, _ := decideRouteChangeStatus(
		latestHops, baselineHops,
		map[string]int{"sig-A": 1, "sig-B": 1},
		2,
	)
	if hasChange {
		t.Errorf("Expected no route change for single-hop ECMP swap on 10-hop path, got change=true")
	}
}

func TestDecideRouteChangeStatus_RealRouteChange(t *testing.T) {
	baselineHops := "1.1.1.1->2.2.2.2->3.3.3.3->4.4.4.4->5.5.5.5"
	latestHops := "10.10.10.10->20.20.20.20->30.30.30.30->40.40.40.40->50.50.50.50"
	hasChange, stability := decideRouteChangeStatus(
		latestHops, baselineHops,
		map[string]int{"sig-A": 1, "sig-B": 1},
		2,
	)
	if !hasChange {
		t.Errorf("Expected route change for completely different path, got change=false")
	}
	if stability != 50 {
		t.Errorf("Expected stability 50 (1/2) from dominant signature, got %v", stability)
	}
}

func TestDecideRouteChangeStatus_NoBaselineSingleSignature(t *testing.T) {
	hasChange, stability := decideRouteChangeStatus(
		"sig-A", "",
		map[string]int{"sig-A": 10},
		10,
	)
	if hasChange {
		t.Errorf("Expected no route change with single signature and no baseline")
	}
	if stability != 100 {
		t.Errorf("Expected stability 100 with single signature, got %v", stability)
	}
}

func TestDecideRouteChangeStatus_NoBaselineMultipleSignatures(t *testing.T) {
	hasChange, stability := decideRouteChangeStatus(
		"sig-A", "",
		map[string]int{"sig-A": 8, "sig-B": 2},
		10,
	)
	if !hasChange {
		t.Errorf("Expected route change fallback when multiple signatures and no baseline")
	}
	if stability != 80 {
		t.Errorf("Expected stability 80 (8/10) from dominant signature, got %v", stability)
	}
}

func TestDecideRouteChangeStatus_EmptySigsWithBaseline(t *testing.T) {
	hops := "1.1.1.1->2.2.2.2"
	hasChange, stability := decideRouteChangeStatus(
		hops, hops,
		map[string]int{},
		0,
	)
	if hasChange {
		t.Errorf("Expected no route change with matching baseline path even if sigs is empty")
	}
	if stability != 100 {
		t.Errorf("Expected stability 100, got %v", stability)
	}
}

func TestHopSetJaccard(t *testing.T) {
	cases := []struct {
		name     string
		a, b     []string
		expected float64
	}{
		{"identical", []string{"1.1.1.1", "2.2.2.2"}, []string{"1.1.1.1", "2.2.2.2"}, 1.0},
		{"disjoint", []string{"1.1.1.1", "2.2.2.2"}, []string{"3.3.3.3", "4.4.4.4"}, 0.0},
		{"one of two common", []string{"1.1.1.1", "2.2.2.2"}, []string{"1.1.1.1", "3.3.3.3"}, 1.0 / 3.0},
		{"empty both", []string{}, []string{}, 1.0},
		{"wildcard skipped", []string{"*", "1.1.1.1"}, []string{"*", "1.1.1.1"}, 1.0},
		{"ecmp swap one of three", []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}, []string{"1.1.1.1", "9.9.9.9", "3.3.3.3"}, 0.5},
		{"ecmp swap one of ten", []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "7.7.7.7", "8.8.8.8", "9.9.9.9", "10.10.10.10"}, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5", "6.6.6.6", "7.7.7.7", "8.8.8.8", "9.9.9.9", "99.99.99.99"}, 9.0 / 11.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hopSetJaccard(tc.a, tc.b)
			if math.Abs(got-tc.expected) > 1e-9 {
				t.Errorf("hopSetJaccard(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.expected)
			}
		})
	}
}

func TestParseHopPath(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected []string
	}{
		{"arrow separator", "1.1.1.1->2.2.2.2", []string{"1.1.1.1", "2.2.2.2"}},
		{"arrow with spaces", "1.1.1.1 -> 2.2.2.2", []string{"1.1.1.1", "2.2.2.2"}},
		{"empty", "", nil},
		{"wildcard", "*->1.1.1.1", []string{"*", "1.1.1.1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseHopPath(tc.input)
			if len(got) != len(tc.expected) {
				t.Fatalf("parseHopPath(%q) = %v, want %v", tc.input, got, tc.expected)
			}
			for i := range got {
				if got[i] != tc.expected[i] {
					t.Errorf("parseHopPath(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.expected[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AGENT-bidirectional per-direction attribution tests
//
// A single bidirectional AGENT probe (one probe_id, two rowsets: forward
// from owner→target, reverse from target→owner) must be presented as TWO
// independent tests in the workspace route analysis — one per direction:
//
//   - The FORWARD direction is attributed to the probe OWNER (A). It is
//     A's outbound path to the target (B).
//   - The REVERSE direction is attributed to the REPORTER (B). It is B's
//     outbound path to the target (A). The reverse is NOT folded into the
//     owner's route / hop / destination aggregates — doing so shows the
//     owner a route they are not actually traversing, and triggers
//     false-positive route_change incidents when the reverse path's ECMP
//     signatures differ from the forward's stable signature.
//
// What stays shared (keyed on probe_id, not on direction):
//
//   - totalRoutes counts unique probe_ids, not (probe, direction) tuples.
//   - commonTargets / sharedDestinations probe_count is per probe, not
//     per (probe, direction) — one probe still equals one probeCount
//     regardless of how many directions it's reported in.
// ---------------------------------------------------------------------------

func TestMTRPathKey_OwnerAndReverseDetection(t *testing.T) {
	cases := []struct {
		name          string
		key           mtrPathKey
		wantOwner     uint
		wantIsReverse bool
	}{
		{
			name:          "standalone MTR: reporter == owner",
			key:           mtrPathKey{probeID: 1, agentID: 10, targetAgent: 0, probeAgentID: 10},
			wantOwner:     10,
			wantIsReverse: false,
		},
		{
			name:          "AGENT probe forward (A→B): reporter == owner",
			key:           mtrPathKey{probeID: 793, agentID: 10, targetAgent: 20, probeAgentID: 10},
			wantOwner:     10,
			wantIsReverse: false,
		},
		{
			name:          "AGENT probe reverse (B→A): reporter != owner",
			key:           mtrPathKey{probeID: 793, agentID: 20, targetAgent: 10, probeAgentID: 10},
			wantOwner:     10,
			wantIsReverse: true,
		},
		{
			name:          "legacy row: probeAgentID == 0 falls back to reporter",
			key:           mtrPathKey{probeID: 2, agentID: 30, targetAgent: 0, probeAgentID: 0},
			wantOwner:     30,
			wantIsReverse: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.key.ownerAgent(); got != tc.wantOwner {
				t.Errorf("ownerAgent() = %d, want %d", got, tc.wantOwner)
			}
			if got := tc.key.isReverse(); got != tc.wantIsReverse {
				t.Errorf("isReverse() = %v, want %v", got, tc.wantIsReverse)
			}
		})
	}
}

// fixtureMTRRow builds a ProbeData for the MTR aggregator with a single
// hop list. Used by the AGENT-bidirectional tests below.
func fixtureMTRRow(probeID, agentID, probeAgentID, targetAgent uint, hops []string) ProbeData {
	mtrHops := make([]MtrHop, len(hops))
	for i, ip := range hops {
		mtrHops[i] = MtrHop{
			TTL:     i + 1,
			Hosts:   []MtrHopHost{{IP: ip}},
			Avg:     "10.0",
			LossPct: "0.0%",
		}
	}
	payload, _ := json.Marshal(MtrPayload{Report: MtrReport{Hops: mtrHops}})
	return ProbeData{
		CreatedAt:    time.Now().UTC(),
		Type:         TypeMTR,
		ProbeID:      probeID,
		ProbeAgentID: probeAgentID,
		AgentID:      agentID,
		TargetAgent:  targetAgent,
		Payload:      payload,
	}
}

// newAgentInfo returns an agentInfo for tests.
func newAgentInfo(id uint, name string) agentInfo {
	return agentInfo{ID: id, Name: name}
}

func TestMTRPathAgg_AGENTBidirectionalDoesNotInflate(t *testing.T) {
	// Two agents A=10 and B=20. One bidirectional AGENT probe 793
	// between them. ClickHouse holds two rowsets:
	//   - Forward: agent_id=10, target_agent=20, probe_agent_id=10
	//   - Reverse: agent_id=20, target_agent=10, probe_agent_id=10
	const (
		probeID uint = 793
		ownerA  uint = 10
		ownerB  uint = 20
	)
	transitHop := "1.1.1.1"

	ariA := &AgentRouteInfo{AgentID: ownerA, AgentName: "agent-A"}
	ariB := &AgentRouteInfo{AgentID: ownerB, AgentName: "agent-B"}
	ariByAgent := map[uint]*AgentRouteInfo{ownerA: ariA, ownerB: ariB}
	agentByID := map[uint]agentInfo{
		ownerA: newAgentInfo(ownerA, "agent-A"),
		ownerB: newAgentInfo(ownerB, "agent-B"),
	}

	routeByKey := make(map[routeKey]*ProbeRouteInfo)
	hopIndex := make(map[string]map[uint]HopMetrics)
	destAgg := make(map[string]*destStats)
	seenProbeIDs := make(map[uint]struct{})
	incidentProbeIDs := make(map[uint]struct{})

	agg := newMTRPathAgg(mtrPathAggConfig{
		ARIByAgent:       ariByAgent,
		AgentByID:        agentByID,
		AgentIPToID:      map[string]uint{},
		CommonTargetKey:  func(s string) string { return strings.ToLower(strings.TrimSpace(s)) },
		RouteByKey:       routeByKey,
		HopIndex:         hopIndex,
		DestAgg:          destAgg,
		SeenProbeIDs:     seenProbeIDs,
		IncidentProbeIDs: incidentProbeIDs,
		// No baseline → route stays "stable" (single signature, no baseline
		// to compare against → HasRouteChange=false).
		LoadBaselineForDir: func(uint, uint) (routeBaseline, bool) { return routeBaseline{}, false },
	})

	var routeIncidents []RouteIncident

	// Forward: A runs trace A→transit→B. probe 793, reporter A=10, owner A=10.
	forwardKey := mtrPathKey{
		probeID: probeID, agentID: ownerA, targetAgent: ownerB, probeAgentID: ownerA,
	}
	forwardRows := []ProbeData{
		fixtureMTRRow(probeID, ownerA, ownerA, ownerB, []string{"10.0.0.1", transitHop, "10.0.0.2"}),
	}

	// Reverse: B runs trace B→transit→A. Same probe 793, reporter B=20,
	// owner A=10.
	reverseKey := mtrPathKey{
		probeID: probeID, agentID: ownerB, targetAgent: ownerA, probeAgentID: ownerA,
	}
	reverseRows := []ProbeData{
		fixtureMTRRow(probeID, ownerB, ownerA, ownerA, []string{"10.0.0.2", transitHop, "10.0.0.1"}),
	}

	agg.process(forwardKey, forwardRows, &routeIncidents)
	agg.process(reverseKey, reverseRows, &routeIncidents)

	// (1) totalRoutes = unique probe IDs = 1, not 2.
	if len(seenProbeIDs) != 1 {
		t.Errorf("seenProbeIDs has %d entries, want 1 (one probe, two directions)", len(seenProbeIDs))
	}
	if _, ok := seenProbeIDs[probeID]; !ok {
		t.Errorf("seenProbeIDs missing probe %d", probeID)
	}

	// (2) Both directions still materialise as separate ProbeRouteInfo
	// entries so the UI can render forward + reverse. Each direction is
	// attributed to the agent that actually ran the trace: the forward
	// to the owner (A), the reverse to the reporter (B).
	if len(routeByKey) != 2 {
		t.Fatalf("routeByKey has %d entries, want 2 (forward + reverse)", len(routeByKey))
	}
	forwardPRI, forwardOK := routeByKey[routeKey{probeID: probeID, agentID: ownerA, targetAgent: ownerB}]
	if !forwardOK {
		t.Fatalf("forward ProbeRouteInfo missing for (A→B)")
	}
	if forwardPRI.AgentID != ownerA {
		t.Errorf("forward ProbeRouteInfo.AgentID = %d, want owner %d", forwardPRI.AgentID, ownerA)
	}
	reversePRI, reverseOK := routeByKey[routeKey{probeID: probeID, agentID: ownerB, targetAgent: ownerA}]
	if !reverseOK {
		t.Fatalf("reverse ProbeRouteInfo missing for (B→A)")
	}
	if reversePRI.AgentID != ownerB {
		t.Errorf("reverse ProbeRouteInfo.AgentID = %d, want reporter %d (reverse direction attributed to the agent running the test, not the owner)",
			reversePRI.AgentID, ownerB)
	}

	// (3) Transit hop (1.1.1.1) is attributed to BOTH reporters — A
	// traverses it on A→B and B traverses it on B→A. They are
	// independent traces; this is genuine shared-hop attribution, not
	// inflation from a single probe being double-counted.
	transitAgents, ok := hopIndex[transitHop]
	if !ok {
		t.Fatalf("hopIndex missing transit hop %s", transitHop)
	}
	if len(transitAgents) != 2 {
		t.Errorf("transit hop %s attributed to %d agents, want 2 (one per direction): %v",
			transitHop, len(transitAgents), transitAgents)
	}
	if _, ok := transitAgents[ownerA]; !ok {
		t.Errorf("transit hop %s not attributed to forward reporter %d: got %v",
			transitHop, ownerA, transitAgents)
	}
	if _, ok := transitAgents[ownerB]; !ok {
		t.Errorf("transit hop %s not attributed to reverse reporter %d: got %v",
			transitHop, ownerB, transitAgents)
	}

	// (4) commonTargets probe_count is per probe, not per direction.
	// The reverse direction's destination (A, from B's perspective)
	// and the forward direction's destination (B, from A's
	// perspective) are two distinct destAgg entries, but each must
	// have probeCount == 1 because one probe contributes to each.
	for target, ds := range destAgg {
		if ds.probeCount != 1 {
			t.Errorf("destAgg[%q].probeCount = %d, want 1 (one probe contributes both directions)",
				target, ds.probeCount)
		}
		// Each destAgg entry has exactly one reporter — the agent
		// actually running the MTR to that target. Forward → target=B
		// is attributed to A; reverse → target=A is attributed to B.
		if len(ds.agents) != 1 {
			t.Errorf("destAgg[%q] has %d agents, want 1 (the reporter for that direction): %v",
				target, len(ds.agents), ds.agents)
		}
	}
}

// Per-direction incident attribution: each direction of a bidirectional
// AGENT probe emits its own route_change incident (when warranted) and
// attributes it to the agent that actually ran the trace. The dedupe
// key is (probe_id, attribution_id), not probe_id.
func TestMTRPathAgg_AGENTBidirectionalPerDirectionIncidents(t *testing.T) {
	const (
		probeID uint = 793
		ownerA  uint = 10
		ownerB  uint = 20
	)
	ariByAgent := map[uint]*AgentRouteInfo{
		ownerA: {AgentID: ownerA, AgentName: "agent-A"},
		ownerB: {AgentID: ownerB, AgentName: "agent-B"},
	}
	agentByID := map[uint]agentInfo{
		ownerA: newAgentInfo(ownerA, "agent-A"),
		ownerB: newAgentInfo(ownerB, "agent-B"),
	}

	// Mirror production semantics: only the forward direction (where
	// reporter == owner) has a baseline. The reverse direction gets
	// no baseline, so a single-signature trace is stable but a
	// multi-signature trace (ECMP) flags as a change.
	mismatchBaseline := routeBaseline{
		Fingerprint: "different-fingerprint",
		RoutePath:   "99.99.99.99->88.88.88.88",
		HopCount:    2,
	}
	loadBaseline := func(pID, reporterID uint) (routeBaseline, bool) {
		if pID == probeID && reporterID == ownerA {
			return mismatchBaseline, true
		}
		return routeBaseline{}, false
	}

	agg := newMTRPathAgg(mtrPathAggConfig{
		ARIByAgent:         ariByAgent,
		AgentByID:          agentByID,
		AgentIPToID:        map[string]uint{},
		CommonTargetKey:    func(s string) string { return strings.ToLower(strings.TrimSpace(s)) },
		RouteByKey:         make(map[routeKey]*ProbeRouteInfo),
		HopIndex:           make(map[string]map[uint]HopMetrics),
		DestAgg:            make(map[string]*destStats),
		SeenProbeIDs:       make(map[uint]struct{}),
		IncidentProbeIDs:   make(map[uint]struct{}),
		LoadBaselineForDir: loadBaseline,
	})

	var incidents []RouteIncident
	forwardKey := mtrPathKey{probeID: probeID, agentID: ownerA, targetAgent: ownerB, probeAgentID: ownerA}
	reverseKey := mtrPathKey{probeID: probeID, agentID: ownerB, targetAgent: ownerA, probeAgentID: ownerA}

	// Forward has a baseline mismatch (route change).
	agg.process(forwardKey, []ProbeData{
		fixtureMTRRow(probeID, ownerA, ownerA, ownerB, []string{"10.0.0.1", "10.0.0.2"}),
	}, &incidents)
	// Reverse has multiple signatures (ECMP) and no baseline.
	agg.process(reverseKey, []ProbeData{
		fixtureMTRRow(probeID, ownerB, ownerA, ownerA, []string{"10.0.0.2", "10.0.0.1"}),
		fixtureMTRRow(probeID, ownerB, ownerA, ownerA, []string{"10.0.0.2", "10.0.0.99"}),
	}, &incidents)

	if len(incidents) != 2 {
		t.Fatalf("got %d route_change incidents, want 2 (one per direction, each attributed to its own reporter): %+v",
			len(incidents), incidents)
	}

	// Incident 1: forward direction, attributed to A.
	if incidents[0].ProbeID != probeID || incidents[0].AgentID != ownerA {
		t.Errorf("forward incident = {ProbeID: %d, AgentID: %d}, want {ProbeID: %d, AgentID: %d}",
			incidents[0].ProbeID, incidents[0].AgentID, probeID, ownerA)
	}
	// Forward has a baseline: structured change data should be populated.
	if incidents[0].BaselineFingerprint != mismatchBaseline.Fingerprint {
		t.Errorf("forward incident BaselineFingerprint = %q, want %q",
			incidents[0].BaselineFingerprint, mismatchBaseline.Fingerprint)
	}
	if incidents[0].BaselinePath != mismatchBaseline.RoutePath {
		t.Errorf("forward incident BaselinePath = %q, want %q",
			incidents[0].BaselinePath, mismatchBaseline.RoutePath)
	}
	if incidents[0].CurrentPath == "" {
		t.Errorf("forward incident CurrentPath should be populated, got empty")
	}
	if incidents[0].BaselineHopCount != mismatchBaseline.HopCount {
		t.Errorf("forward incident BaselineHopCount = %d, want %d",
			incidents[0].BaselineHopCount, mismatchBaseline.HopCount)
	}
	// The test's baseline is deliberately disjoint from the current
	// path, so Jaccard == 0 is the correct value (no hop overlap). The
	// important contract is that the field is populated, not that it's
	// non-zero. A separate test below covers the partial-overlap case.
	if incidents[0].Jaccard != 0 {
		t.Errorf("forward incident Jaccard = %v, want 0 (test baseline is disjoint from current path)", incidents[0].Jaccard)
	}
	if len(incidents[0].AddedHops) == 0 || len(incidents[0].RemovedHops) == 0 {
		t.Errorf("forward incident should have both AddedHops and RemovedHops, got added=%v removed=%v",
			incidents[0].AddedHops, incidents[0].RemovedHops)
	}
	if incidents[0].StabilityPct <= 0 {
		t.Errorf("forward incident StabilityPct = %v, want > 0", incidents[0].StabilityPct)
	}
	if incidents[0].TraceCount == 0 {
		t.Errorf("forward incident TraceCount should be > 0, got 0")
	}

	// Incident 2: reverse direction, attributed to B (the reporter
	// running the return-path MTR), not to the probe owner A.
	if incidents[1].ProbeID != probeID || incidents[1].AgentID != ownerB {
		t.Errorf("reverse incident = {ProbeID: %d, AgentID: %d}, want {ProbeID: %d, AgentID: %d} (reverse attributed to the reporter, not the owner)",
			incidents[1].ProbeID, incidents[1].AgentID, probeID, ownerB)
	}
	// Reverse has NO baseline: BaselinePath / BaselineFingerprint are
	// empty, but the structured current-path data should still be
	// populated so the UI can show the actual route IPs the reporter
	// observed.
	if incidents[1].BaselinePath != "" {
		t.Errorf("reverse incident BaselinePath should be empty (no baseline for reverse), got %q",
			incidents[1].BaselinePath)
	}
	if incidents[1].CurrentPath == "" {
		t.Errorf("reverse incident CurrentPath should be populated, got empty")
	}
	if incidents[1].CurrentFingerprint == "" {
		t.Errorf("reverse incident CurrentFingerprint should be populated, got empty")
	}
	if incidents[1].StabilityPct <= 0 {
		t.Errorf("reverse incident StabilityPct = %v, want > 0 (dominant signature share over multiple traces)",
			incidents[1].StabilityPct)
	}
	if incidents[1].TraceCount != 2 {
		t.Errorf("reverse incident TraceCount = %d, want 2 (two reverse rows were ingested)",
			incidents[1].TraceCount)
	}
}

// diffHopsOrdered returns IPs only in current (added) and only in
// baseline (removed) while preserving first-seen order. This is the
// ordered-difference helper the aggregator uses to populate the
// RouteIncident.AddedHops / RemovedHops fields.
func TestDiffHopsOrdered(t *testing.T) {
	cases := []struct {
		name              string
		baseline, current []string
		wantAdded         []string
		wantRemoved       []string
	}{
		{
			name:     "no change",
			baseline: []string{"1.1.1.1", "2.2.2.2"},
			current:  []string{"1.1.1.1", "2.2.2.2"},
		},
		{
			name:        "ECMP swap on a middle hop",
			baseline:    []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"},
			current:     []string{"1.1.1.1", "9.9.9.9", "3.3.3.3"},
			wantAdded:   []string{"9.9.9.9"},
			wantRemoved: []string{"2.2.2.2"},
		},
		{
			name:        "extra transit hop added",
			baseline:    []string{"1.1.1.1", "3.3.3.3"},
			current:     []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"},
			wantAdded:   []string{"2.2.2.2"},
			wantRemoved: nil,
		},
		{
			name:        "destination swapped",
			baseline:    []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"},
			current:     []string{"1.1.1.1", "2.2.2.2", "9.9.9.9"},
			wantAdded:   []string{"9.9.9.9"},
			wantRemoved: []string{"3.3.3.3"},
		},
		{
			name:      "no baseline (empty) — everything is added",
			baseline:  nil,
			current:   []string{"1.1.1.1", "2.2.2.2"},
			wantAdded: []string{"1.1.1.1", "2.2.2.2"},
		},
		{
			name:        "no current (empty) — everything is removed",
			baseline:    []string{"1.1.1.1", "2.2.2.2"},
			current:     nil,
			wantRemoved: []string{"1.1.1.1", "2.2.2.2"},
		},
		{
			name:      "wildcards and empty entries ignored",
			baseline:  []string{"1.1.1.1", "*", ""},
			current:   []string{"1.1.1.1", "2.2.2.2", ""},
			wantAdded: []string{"2.2.2.2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			added, removed := diffHopsOrdered(tc.baseline, tc.current)
			if !slicesEqual(added, tc.wantAdded) {
				t.Errorf("added = %v, want %v", added, tc.wantAdded)
			}
			if !slicesEqual(removed, tc.wantRemoved) {
				t.Errorf("removed = %v, want %v", removed, tc.wantRemoved)
			}
		})
	}
}

func slicesEqual(a, b []string) bool {
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

// User-reported bug: when the OWNER's forward path is stable (baseline
// match) but the REVERSE direction has ECMP (multiple signatures) and
// no baseline, the OWNER must not be flagged for a route change. The
// old behaviour attributed the reverse to the owner, which made the
// owner appear to have a route change they never actually observed.
func TestMTRPathAgg_AGENTBidirectional_StableForwardNoFalseIncident(t *testing.T) {
	const (
		probeID uint = 793
		ownerA  uint = 10
		ownerB  uint = 20
	)
	ariByAgent := map[uint]*AgentRouteInfo{
		ownerA: {AgentID: ownerA, AgentName: "agent-A"},
		ownerB: {AgentID: ownerB, AgentName: "agent-B"},
	}
	agentByID := map[uint]agentInfo{
		ownerA: newAgentInfo(ownerA, "agent-A"),
		ownerB: newAgentInfo(ownerB, "agent-B"),
	}

	// OWNER's baseline matches the forward trace exactly.
	matchingBaseline := routeBaseline{
		Fingerprint: "stable-fingerprint",
		RoutePath:   "10.0.0.1->10.0.0.2",
		HopCount:    2,
	}
	loadBaseline := func(pID, reporterID uint) (routeBaseline, bool) {
		if pID == probeID && reporterID == ownerA {
			return matchingBaseline, true
		}
		return routeBaseline{}, false
	}

	agg := newMTRPathAgg(mtrPathAggConfig{
		ARIByAgent:         ariByAgent,
		AgentByID:          agentByID,
		AgentIPToID:        map[string]uint{},
		CommonTargetKey:    func(s string) string { return strings.ToLower(strings.TrimSpace(s)) },
		RouteByKey:         make(map[routeKey]*ProbeRouteInfo),
		HopIndex:           make(map[string]map[uint]HopMetrics),
		DestAgg:            make(map[string]*destStats),
		SeenProbeIDs:       make(map[uint]struct{}),
		IncidentProbeIDs:   make(map[uint]struct{}),
		LoadBaselineForDir: loadBaseline,
	})

	var incidents []RouteIncident
	forwardKey := mtrPathKey{probeID: probeID, agentID: ownerA, targetAgent: ownerB, probeAgentID: ownerA}
	reverseKey := mtrPathKey{probeID: probeID, agentID: ownerB, targetAgent: ownerA, probeAgentID: ownerA}

	// Forward: stable, single signature, matches baseline.
	agg.process(forwardKey, []ProbeData{
		fixtureMTRRow(probeID, ownerA, ownerA, ownerB, []string{"10.0.0.1", "10.0.0.2"}),
	}, &incidents)
	// Reverse: ECMP (two different signatures) and no baseline.
	// Pre-fix this would cause HasRouteChange=true on the forward
	// direction's ProbeRouteInfo because the reverse's data was
	// attributed to the owner; the dedupe on probeID would then emit
	// a single incident for the owner.
	agg.process(reverseKey, []ProbeData{
		fixtureMTRRow(probeID, ownerB, ownerA, ownerA, []string{"10.0.0.2", "10.0.0.1"}),
		fixtureMTRRow(probeID, ownerB, ownerA, ownerA, []string{"10.0.0.2", "10.0.0.99"}),
	}, &incidents)

	// No incident should be attributed to the owner — their forward
	// path is stable.
	for _, inc := range incidents {
		if inc.AgentID == ownerA {
			t.Errorf("owner %d should not have a route_change incident, got: %+v", ownerA, inc)
		}
	}

	// The reverse direction (attributed to B) is free to surface its
	// own signal: 1 incident attributed to B is acceptable.
	if len(incidents) > 1 {
		t.Errorf("got %d incidents, want at most 1 (only the reverse reporter may flag): %+v", len(incidents), incidents)
	}
	for _, inc := range incidents {
		if inc.AgentID != ownerB {
			t.Errorf("incident attributed to agent %d, want only ownerB=%d: %+v", inc.AgentID, ownerB, inc)
		}
	}
}
