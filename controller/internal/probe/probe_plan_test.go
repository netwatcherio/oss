package probe

import (
	"testing"

	"gorm.io/datatypes"
)

// TestExpandedAgentProbeTypes verifies the AGENT-probe expansion logic that
// drives the network map's "MTR / PING / TRAFFICSIM" enablement pills.
func TestExpandedAgentProbeTypes(t *testing.T) {
	tests := []struct {
		name            string
		metadata        string
		envDefault      string
		targetHasServer bool
		want            []string
	}{
		{
			name:            "nil probe returns nothing",
			metadata:        "",
			targetHasServer: false,
			want:            nil,
		},
		{
			name:            "AGENT probe with no metadata always emits MTR",
			metadata:        "",
			targetHasServer: false,
			want:            []string{"MTR"},
		},
		{
			name:            "target with no server — no TRAFFICSIM",
			metadata:        "",
			targetHasServer: false,
			want:            []string{"MTR"},
		},
		{
			name:            "target with server — TRAFFICSIM emitted",
			metadata:        "",
			targetHasServer: true,
			want:            []string{"MTR", "TRAFFICSIM"},
		},
		{
			name:            "PING opt-in via metadata",
			metadata:        `{"expansion":{"include_ping":true}}`,
			targetHasServer: true,
			want:            []string{"MTR", "PING", "TRAFFICSIM"},
		},
		{
			name:            "PING opt-out via metadata (env ignored)",
			metadata:        `{"expansion":{"include_ping":false}}`,
			envDefault:      "true",
			targetHasServer: true,
			want:            []string{"MTR", "TRAFFICSIM"},
		},
		{
			name:            "PING metadata absent — env var wins",
			metadata:        `{}`,
			envDefault:      "true",
			targetHasServer: true,
			want:            []string{"MTR", "PING", "TRAFFICSIM"},
		},
		{
			name:            "PING metadata absent and env unset — no PING",
			metadata:        `{}`,
			envDefault:      "",
			targetHasServer: true,
			want:            []string{"MTR", "TRAFFICSIM"},
		},
		{
			name:            "PING on, no server",
			metadata:        `{"expansion":{"include_ping":true}}`,
			targetHasServer: false,
			want:            []string{"MTR", "PING"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set the env-var default (unset = "").
			if tc.envDefault != "" {
				t.Setenv("AGENT_EXPANSION_INCLUDE_PING", tc.envDefault)
			} else {
				t.Setenv("AGENT_EXPANSION_INCLUDE_PING", "")
			}

			var p *Probe
			if tc.name != "nil probe returns nothing" {
				p = &Probe{Type: TypeAgent, Enabled: true}
				if tc.metadata != "" {
					p.Metadata = datatypes.JSON(tc.metadata)
				}
			}

			got := expandedAgentProbeTypes(p, tc.targetHasServer)
			if !sliceEqual(got, tc.want) {
				t.Errorf("expandedAgentProbeTypes() = %v, want %v", got, tc.want)
			}
		})
	}
}

func sliceEqual(a, b []string) bool {
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

// TestApplyProbePlan covers the merge semantics: data-derived flags stay
// (or are set to true) when the plan also says the type is enabled; they
// remain false when neither data nor plan has the type.
func TestApplyProbePlan(t *testing.T) {
	tests := []struct {
		name           string
		detail         *ProbeEndpointDetail
		plans          map[uint]map[uint][]string
		sourceAgentID  uint
		targetAgentID  uint
		wantMTR        bool
		wantPing       bool
		wantTrafficSim bool
	}{
		{
			name:           "nil detail is a no-op",
			detail:         nil,
			plans:          map[uint]map[uint][]string{1: {2: {"MTR"}}},
			sourceAgentID:  1,
			targetAgentID:  2,
			wantMTR:        false,
			wantPing:       false,
			wantTrafficSim: false,
		},
		{
			name:          "plan turns on MTR (data had nothing)",
			detail:        &ProbeEndpointDetail{},
			plans:         map[uint]map[uint][]string{1: {2: {"MTR"}}},
			sourceAgentID: 1,
			targetAgentID: 2,
			wantMTR:       true,
		},
		{
			name:           "plan turns on all three for an AGENT probe with PING",
			detail:         &ProbeEndpointDetail{},
			plans:          map[uint]map[uint][]string{1: {2: {"MTR", "PING", "TRAFFICSIM"}}},
			sourceAgentID:  1,
			targetAgentID:  2,
			wantMTR:        true,
			wantPing:       true,
			wantTrafficSim: true,
		},
		{
			name:          "no plan entry for (source, target) leaves detail alone",
			detail:        &ProbeEndpointDetail{HasMTR: true},
			plans:         map[uint]map[uint][]string{},
			sourceAgentID: 1,
			targetAgentID: 2,
			wantMTR:       true,
		},
		{
			name:          "data-derived flag persists when plan also says yes",
			detail:        &ProbeEndpointDetail{HasMTR: true, HasPing: true},
			plans:         map[uint]map[uint][]string{1: {2: {"MTR", "PING"}}},
			sourceAgentID: 1,
			targetAgentID: 2,
			wantMTR:       true,
			wantPing:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			applyProbePlan(tc.detail, tc.plans, tc.sourceAgentID, tc.targetAgentID)
			if tc.detail == nil {
				return
			}
			if tc.detail.HasMTR != tc.wantMTR {
				t.Errorf("HasMTR = %v, want %v", tc.detail.HasMTR, tc.wantMTR)
			}
			if tc.detail.HasPing != tc.wantPing {
				t.Errorf("HasPing = %v, want %v", tc.detail.HasPing, tc.wantPing)
			}
			if tc.detail.HasTrafficSim != tc.wantTrafficSim {
				t.Errorf("HasTrafficSim = %v, want %v", tc.detail.HasTrafficSim, tc.wantTrafficSim)
			}
		})
	}
}
