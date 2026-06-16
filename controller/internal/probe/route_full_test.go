package probe

import (
	"encoding/json"
	"testing"
)

// Simulate the actual data flow:
// 1. Agent sends MtrPayload (with time.Time fields)
// 2. WebSocket receives, stores in ClickHouse via SaveRecordCH
// 3. Route analysis reads back and parses into MtrPayload (string timestamps)

func TestRouteAnalysisDataFlow(t *testing.T) {
	// Simulate the agent's mtrPayload (private struct from mtr.go)
	// This is what the agent actually sends
	agentJSON := []byte(`{
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
					"stddev": "0.2"
				},
				{
					"ttl": 2,
					"hosts": [{"ip": "10.0.0.1", "hostname": "isp-gw"}],
					"loss_pct": "0.0%",
					"sent": 5,
					"last": "5.0",
					"recv": 5,
					"avg": "5.0",
					"best": "4.8",
					"worst": "5.3",
					"stddev": "0.2"
				},
				{
					"ttl": 3,
					"hosts": [{"ip": "8.8.8.8", "hostname": "dns.google"}],
					"loss_pct": "0.0%",
					"sent": 5,
					"last": "10.5",
					"recv": 5,
					"avg": "10.5",
					"best": "10.0",
					"worst": "11.0",
					"stddev": "0.3"
				}
			]
		}
	}`)

	// Now simulate the route analysis parsing the data from ClickHouse
	// (which is just the stored JSON string)
	var mp MtrPayload
	if err := json.Unmarshal(agentJSON, &mp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify the parsed data
	if len(mp.Report.Hops) != 3 {
		t.Fatalf("Expected 3 hops, got %d", len(mp.Report.Hops))
	}

	// Check first hop
	hop1 := mp.Report.Hops[0]
	if hop1.Hosts[0].IP != "192.168.1.1" {
		t.Errorf("Expected first hop IP 192.168.1.1, got %s", hop1.Hosts[0].IP)
	}
	// Note: Hostname is not in MtrHopHost (public) so it's not parsed - that's expected

	// Check last hop (the destination)
	lastHop := mp.Report.Hops[len(mp.Report.Hops)-1]
	if lastHop.Hosts[0].IP != "8.8.8.8" {
		t.Errorf("Expected last hop IP 8.8.8.8, got %s", lastHop.Hosts[0].IP)
	}

	// Simulate the route analysis building LatestHops
	var latestHops []string
	for _, hop := range mp.Report.Hops {
		if len(hop.Hosts) > 0 && hop.Hosts[0].IP != "" {
			latestHops = append(latestHops, hop.Hosts[0].IP)
		}
	}
	if len(latestHops) != 3 {
		t.Errorf("Expected 3 latestHops, got %d", len(latestHops))
	}

	// Simulate agentIPToID with a couple of agents
	agentIPToID := map[string]uint{
		"192.168.1.1": 1, // agent-1 local IP
		"8.8.8.8":     2, // some agent's public IP
	}
	agentByID := map[uint]agentInfo{
		1: {ID: 1, Name: "agent-local"},
		2: {ID: 2, Name: "agent-remote"},
	}

	// Build hop details
	details := buildHopDetails(&mp, agentIPToID, agentByID)
	if len(details) != 3 {
		t.Errorf("Expected 3 details, got %d", len(details))
	}

	// Check the first hop - it's an agent
	if !details[0].IsAgent {
		t.Errorf("Expected first hop to be an agent")
	}
	if details[0].AgentName != "agent-local" {
		t.Errorf("Expected first hop agent name 'agent-local', got %q", details[0].AgentName)
	}

	// Check that the JSON tags work correctly when marshaled back
	jsonBytes, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	jsonStr := string(jsonBytes)
	t.Logf("Hop details JSON: %s", jsonStr)

	// Verify the JSON uses snake_case (is_agent, is_final_hop, agent_name)
	if !contains(jsonStr, `"is_agent"`) {
		t.Error("Expected is_agent in JSON")
	}
	if !contains(jsonStr, `"is_final_hop"`) {
		t.Error("Expected is_final_hop in JSON")
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRouteAnalysisProbeRouteInfoMarshaling(t *testing.T) {
	// Simulate a ProbeRouteInfo that would be returned
	pri := ProbeRouteInfo{
		ProbeID:         42,
		Target:          "8.8.8.8",
		LatestSignature: "192.168.1.1->10.0.0.1->8.8.8.8",
		LatestHops:      []string{"192.168.1.1", "10.0.0.1", "8.8.8.8"},
		LatestHopsDetail: []HopDetail{
			{IP: "192.168.1.1", IsAgent: true, AgentID: 1, AgentName: "agent-1", IsFinalHop: false},
			{IP: "10.0.0.1", IsAgent: false, IsFinalHop: false},
			{IP: "8.8.8.8", IsAgent: false, IsFinalHop: true},
		},
		HasRouteChange:    false,
		RouteStabilityPct: 100,
		TraceCount:        1,
		AvgEndHopLatency:  10.5,
		AvgEndHopLoss:     0.0,
		IntermediateHops: []HopMetric{
			{IP: "192.168.1.1", Loss: 0, Latency: 1.2, HopIndex: 0},
			{IP: "10.0.0.1", Loss: 0, Latency: 5.0, HopIndex: 1},
		},
	}

	jsonBytes, err := json.Marshal(pri)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	t.Logf("ProbeRouteInfo JSON: %s", string(jsonBytes))

	// Verify snake_case fields
	jsonStr := string(jsonBytes)
	expectedFields := []string{
		`"probe_id"`, `"target"`, `"latest_signature"`,
		`"latest_hops"`, `"latest_hops_detail"`,
		`"has_route_change"`, `"route_stability_pct"`,
		`"avg_end_hop_latency"`,
		`"intermediate_hops"`,
	}
	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("Expected field %s in JSON output", field)
		}
	}
	// avg_end_hop_loss has omitempty so 0.0 is omitted — that's the desired
	// behavior. Verify a non-zero value is included.
	pri.AvgEndHopLoss = 1.5
	jsonBytes2, _ := json.Marshal(pri)
	if !contains(string(jsonBytes2), `"avg_end_hop_loss":1.5`) {
		t.Error("Expected avg_end_hop_loss:1.5 in JSON output when non-zero")
	}
}
