package probe

import "testing"

// TestMtrVoiceSampleFinalHop verifies rtt/jitter/loss extraction from
// the final responding hop of an MTR payload.
func TestMtrVoiceSampleFinalHop(t *testing.T) {
	payload := `{
		"report": {
			"hops": [
				{"ttl": 1, "hosts": [{"ip": "192.168.1.1"}], "avg": "2.1", "stddev": "0.3", "loss_pct": "0.0"},
				{"ttl": 2, "hosts": [{"ip": "10.0.0.1"}], "avg": "12.4", "stddev": "1.1", "loss_pct": "0.0"},
				{"ttl": 3, "hosts": [{"ip": "203.0.113.9"}], "avg": "38.6", "stddev": "4.2", "loss_pct": "1.5"}
			]
		}
	}`
	rtt, jitter, loss, ok := mtrVoiceSample(payload)
	if !ok {
		t.Fatalf("expected ok=true for valid trace")
	}
	if rtt != 38.6 || jitter != 4.2 || loss != 1.5 {
		t.Errorf("got rtt=%v jitter=%v loss=%v, want 38.6/4.2/1.5", rtt, jitter, loss)
	}
}

// TestMtrVoiceSampleWalksBackPastSilentHops verifies that unanswering
// final hops (firewalled target) fall back to the last responding hop.
func TestMtrVoiceSampleWalksBackPastSilentHops(t *testing.T) {
	payload := `{
		"report": {
			"hops": [
				{"ttl": 1, "hosts": [{"ip": "192.168.1.1"}], "avg": "2.0", "stddev": "0.2", "loss_pct": "0.0"},
				{"ttl": 2, "hosts": [{"ip": "10.9.9.1"}], "avg": "25.0", "stddev": "3.0", "loss_pct": "0.5"},
				{"ttl": 3, "hosts": [{"ip": "*"}], "avg": "0", "stddev": "0", "loss_pct": "100.0"},
				{"ttl": 4, "hosts": [], "avg": "0", "stddev": "0", "loss_pct": "100.0"}
			]
		}
	}`
	rtt, _, _, ok := mtrVoiceSample(payload)
	if !ok {
		t.Fatalf("expected ok=true when an earlier hop responds")
	}
	if rtt != 25.0 {
		t.Errorf("rtt = %v, want 25.0 (last responding hop)", rtt)
	}
}

// TestMtrVoiceSampleEmptyAndGarbage verifies ok=false for unusable
// payloads instead of zero-valued samples polluting the aggregate.
func TestMtrVoiceSampleEmptyAndGarbage(t *testing.T) {
	for _, payload := range []string{
		``,
		`not-json`,
		`{"report": {"hops": []}}`,
		`{"report": {"hops": [{"ttl": 1, "hosts": [{"ip": "*"}], "avg": "0"}]}}`,
	} {
		if _, _, _, ok := mtrVoiceSample(payload); ok {
			t.Errorf("expected ok=false for payload %q", payload)
		}
	}
}
