package probe

import (
	"encoding/json"
	"testing"

	"gorm.io/datatypes"
)

func mdJSON(t *testing.T, m map[string]any) datatypes.JSON {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	return datatypes.JSON(b)
}

func decodeMD(t *testing.T, p Probe) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(p.Metadata, &m); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	return m
}

func TestAgentProbeHasBidirectional(t *testing.T) {
	cases := []struct {
		name     string
		metadata datatypes.JSON
		want     bool
	}{
		{"nil metadata", nil, false},
		{"empty object", datatypes.JSON([]byte(`{}`)), false},
		{"top-level true", mdJSON(t, map[string]any{"bidirectional": true}), true},
		{"top-level false", mdJSON(t, map[string]any{"bidirectional": false}), false},
		{"nested trafficsim true", mdJSON(t, map[string]any{
			"trafficsim": map[string]any{"bidirectional": true},
		}), true},
		{"nested trafficsim false", mdJSON(t, map[string]any{
			"trafficsim": map[string]any{"bidirectional": false},
		}), false},
		{"unrelated keys only", mdJSON(t, map[string]any{
			"trafficsim": map[string]any{"voip_mode": true},
		}), false},
		{"malformed json", datatypes.JSON([]byte(`{not-json`)), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Probe{Metadata: tc.metadata}
			if got := agentProbeHasBidirectional(p); got != tc.want {
				t.Errorf("agentProbeHasBidirectional() = %v, want %v", got, tc.want)
			}
		})
	}
	if agentProbeHasBidirectional(nil) {
		t.Error("agentProbeHasBidirectional(nil) = true, want false")
	}
}

func TestShouldIncludePingExpansion(t *testing.T) {
	const envKey = "AGENT_EXPANSION_INCLUDE_PING"

	cases := []struct {
		name     string
		envVal   string
		setEnv   bool
		metadata datatypes.JSON
		want     bool
	}{
		// Metadata-driven, env unset
		{"no metadata, env unset", "", false, nil, false},
		{"no metadata, env false", "false", true, nil, false},
		{"no metadata, env true", "true", true, nil, true},
		{"unrelated metadata, env true", "true", true,
			mdJSON(t, map[string]any{"trafficsim": map[string]any{"voip_mode": true}}), true},

		// Metadata override wins over env
		{"metadata true overrides env false", "false", true,
			mdJSON(t, map[string]any{"expansion": map[string]any{"include_ping": true}}), true},
		{"metadata false overrides env true", "true", true,
			mdJSON(t, map[string]any{"expansion": map[string]any{"include_ping": false}}), false},

		// Metadata without the expansion key falls back to env
		{"empty expansion object, env true", "true", true,
			mdJSON(t, map[string]any{"expansion": map[string]any{}}), true},
		{"expansion key with wrong type, env true", "true", true,
			mdJSON(t, map[string]any{"expansion": map[string]any{"include_ping": "yes"}}), true},
		{"expansion key with wrong type, env unset", "", false,
			mdJSON(t, map[string]any{"expansion": map[string]any{"include_ping": "yes"}}), false},

		// Malformed metadata falls back to env
		{"malformed metadata, env true", "true", true,
			datatypes.JSON([]byte(`{not-json`)), true},
		{"malformed metadata, env unset", "", false,
			datatypes.JSON([]byte(`{not-json`)), false},

		// Env truthy variants
		{"env '1' true", "1", true, nil, true},
		{"env 'yes' true", "yes", true, nil, true},
		{"env 'on' true", "on", true, nil, true},
		{"env 'TRUE' (case-insensitive)", "TRUE", true, nil, true},

		// Env falsy variants
		{"env '0' false", "0", true, nil, false},
		{"env 'no' false", "no", true, nil, false},
		{"env 'off' false", "off", true, nil, false},

		// Unknown env value → default (false)
		{"env 'maybe' falls back to default", "maybe", true, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				t.Setenv(envKey, tc.envVal)
			} else {
				t.Setenv(envKey, "") // isolate from host env
			}
			p := &Probe{Metadata: tc.metadata}
			if got := shouldIncludePingExpansion(p); got != tc.want {
				t.Errorf("shouldIncludePingExpansion() = %v, want %v", got, tc.want)
			}
		})
	}

	if shouldIncludePingExpansion(nil) {
		t.Error("shouldIncludePingExpansion(nil) = true, want false")
	}
}

func TestSetBidirectionalFlagPreservesExistingMetadata(t *testing.T) {
	src := Probe{Metadata: mdJSON(t, map[string]any{
		"trafficsim": map[string]any{
			"voip_mode":   true,
			"interval_ms": float64(20),
			"dscp":        float64(46),
		},
	})}

	out := setBidirectionalFlag(src, true)
	m := decodeMD(t, out)

	if m["bidirectional"] != true {
		t.Error("top-level bidirectional flag not set")
	}
	ts, _ := m["trafficsim"].(map[string]any)
	if ts == nil {
		t.Fatal("trafficsim section missing")
	}
	if ts["bidirectional"] != true {
		t.Error("trafficsim.bidirectional flag not set")
	}
	// VoIP settings must survive the flag write
	if ts["voip_mode"] != true || ts["interval_ms"] != float64(20) || ts["dscp"] != float64(46) {
		t.Errorf("VoIP settings clobbered: %v", ts)
	}
}

func TestSetBidirectionalServerMode(t *testing.T) {
	// Seeded with the client's metadata, as GetProbesForAgent now does for the
	// dynamically generated per-client server probe.
	src := Probe{Metadata: mdJSON(t, map[string]any{
		"trafficsim": map[string]any{
			"voip_mode":   true,
			"interval_ms": float64(20),
		},
	})}

	out := setBidirectionalServerMode(src, 42, 7)
	m := decodeMD(t, out)

	if m["bidirectional"] != true {
		t.Error("top-level bidirectional flag not set (agent extractVoIPOptions depends on it)")
	}
	ts, _ := m["trafficsim"].(map[string]any)
	if ts == nil {
		t.Fatal("trafficsim section missing")
	}
	if ts["bidirectional_server"] != true {
		t.Error("bidirectional_server not set")
	}
	if ts["client_probe_id"] != float64(42) {
		t.Errorf("client_probe_id = %v, want 42", ts["client_probe_id"])
	}
	if ts["client_agent_id"] != float64(7) {
		t.Errorf("client_agent_id = %v, want 7", ts["client_agent_id"])
	}
	// Inherited VoIP settings must survive so the server's reverse traffic
	// mirrors the client's pacing.
	if ts["voip_mode"] != true || ts["interval_ms"] != float64(20) {
		t.Errorf("inherited VoIP settings clobbered: %v", ts)
	}
}

// Regression: commit 12c30e4 passed the OWNER's agent ID as the expansion target,
// which made the agent-side self-target guard skip every bidirectional TrafficSim
// client probe and misattributed reported data (TargetAgent = self).
func TestCreateExpandedProbeTargetAttribution(t *testing.T) {
	src := &Probe{
		ID:          42,
		AgentID:     1,
		WorkspaceID: 3,
		Enabled:     true,
		Metadata:    mdJSON(t, map[string]any{"trafficsim": map[string]any{"voip_mode": true}}),
	}

	const targetAgentID = uint(2)
	out := createExpandedProbe(src, TypeTrafficSim, "2.2.2.2:5000", targetAgentID)

	if out.ID != 42 {
		t.Errorf("expanded probe ID = %d, want 42 (must keep source ID for data correlation)", out.ID)
	}
	if out.AgentID != 1 {
		t.Errorf("expanded probe AgentID = %d, want 1 (owner)", out.AgentID)
	}
	if len(out.Targets) != 1 {
		t.Fatalf("expanded probe has %d targets, want 1", len(out.Targets))
	}
	tgt := out.Targets[0]
	if tgt.Target != "2.2.2.2:5000" {
		t.Errorf("target = %q, want 2.2.2.2:5000", tgt.Target)
	}
	if tgt.AgentID == nil || *tgt.AgentID != targetAgentID {
		t.Errorf("Target.AgentID = %v, want %d (the TARGET agent, never the owner)", tgt.AgentID, targetAgentID)
	}
	if string(out.Metadata) != string(src.Metadata) {
		t.Error("metadata not inherited from source AGENT probe")
	}
}

// The bidirectional expansion must produce exactly ONE probe per type, owned by
// the requesting agent. A second target-owned "pair" probe would make the owner
// probe its own IP (forward path) or run duplicate return tests (reverse path).
func TestBidirectionalExpansionFlagsClientProbe(t *testing.T) {
	src := &Probe{
		ID:      42,
		AgentID: 1,
		Metadata: mdJSON(t, map[string]any{
			"trafficsim": map[string]any{"bidirectional": true},
		}),
	}

	out := createExpandedProbe(src, TypePing, "2.2.2.2", 2)
	out = setBidirectionalFlag(out, true)

	if out.AgentID != src.AgentID {
		t.Errorf("client probe AgentID = %d, want owner %d", out.AgentID, src.AgentID)
	}
	if !agentProbeHasBidirectional(&out) {
		t.Error("expanded bidirectional probe lost the bidirectional flag")
	}
}
