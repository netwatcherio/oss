package probe

import (
	"encoding/json"
	"math"
	"testing"
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
