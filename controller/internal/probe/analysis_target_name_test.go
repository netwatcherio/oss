package probe

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

// TestBuildTargetNameByProbeIDEmptyProbes verifies the helper
// short-circuits when the probe list is empty. Previously this
// would still try to look up agents; today it returns immediately
// so the function is safe to call with no probes configured.
func TestBuildTargetNameByProbeIDEmptyProbes(t *testing.T) {
	got := buildTargetNameByProbeID(context.Background(), nil, nil, "fallback")
	if got == nil {
		t.Fatalf("expected non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}

	got = buildTargetNameByProbeID(context.Background(), nil, []Probe{}, "fallback")
	if got == nil {
		t.Fatalf("expected non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map for empty slice, got %d entries", len(got))
	}
}

// TestBuildTargetNameByProbeIDNilDBSafe verifies the helper does
// not panic when called with a nil *gorm.DB. This is the regression
// fix for the production stack — the helper used to call
// agent.GetAgentByID with a nil db, which then dereferenced the nil
// receiver inside gorm.WithContext.
//
// With a nil DB the helper returns host/agent-name fallbacks for
// any target that has a Target string; cross-agent targets fall
// back to the source name.
func TestBuildTargetNameByProbeIDNilDBSafe(t *testing.T) {
	t.Run("host-only targets", func(t *testing.T) {
		probes := []Probe{
			{ID: 10, Targets: []Target{{Target: "sip.example-pbx.com"}}},
			{ID: 11, Targets: []Target{{Target: "trunk.carrier-a.net"}}},
		}
		got := buildTargetNameByProbeID(context.Background(), nil, probes, "source-agent")
		if got[10] != "sip.example-pbx.com" {
			t.Errorf("probe 10 = %q, want sip.example-pbx.com", got[10])
		}
		if got[11] != "trunk.carrier-a.net" {
			t.Errorf("probe 11 = %q, want trunk.carrier-a.net", got[11])
		}
	})

	t.Run("cross-agent target without DB falls back to source name", func(t *testing.T) {
		agentID := uint(99)
		probes := []Probe{
			{ID: 20, Targets: []Target{{AgentID: &agentID}}},
		}
		got := buildTargetNameByProbeID(context.Background(), nil, probes, "source-agent")
		// With nil DB, the agent-name lookup can't run, so the
		// helper falls back to the source-agent name. This is the
		// old behaviour for the no-DB path; the production fix
		// is the same code path with a real DB.
		if got[20] != "source-agent" {
			t.Errorf("probe 20 = %q, want source-agent", got[20])
		}
	})
}

// TestBuildTargetNameByProbeIDNilTargets verifies the helper
// tolerates probes with a nil Targets slice.
func TestBuildTargetNameByProbeIDNilTargets(t *testing.T) {
	probes := []Probe{
		{ID: 30, Targets: nil},
		{ID: 31, Targets: []Target{}},
	}
	got := buildTargetNameByProbeID(context.Background(), nil, probes, "src")
	if _, ok := got[30]; ok {
		t.Errorf("probe 30 should not be in result (nil Targets)")
	}
	if _, ok := got[31]; ok {
		t.Errorf("probe 31 should not be in result (empty Targets)")
	}
}

// TestBuildTargetNameByProbeIDBatchLookup verifies the helper does
// a single batched SELECT when the DB is provided, not one query
// per probe. We can't easily count queries against sqlite-in-memory
// here, so we just verify the resolution works end-to-end against
// a real (in-memory) DB.
func TestBuildTargetNameByProbeIDBatchLookup(t *testing.T) {
	db := newTestDB(t)

	// Seed three agents that the probes will reference. seedAgent
	// uses a fixed name; we seed and then rename in-place so the
	// resolved names are distinguishable.
	for i, name := range []string{"pbx-host", "carrier-A", "carrier-B"} {
		seedAgent(t, db, uint(100+i), "", false, 0)
		if err := db.Table("agents").Where("id = ?", uint(100+i)).Update("name", name).Error; err != nil {
			t.Fatalf("rename agent: %v", err)
		}
	}

	p100 := uint(100)
	p101 := uint(101)
	p102 := uint(102)
	probes := []Probe{
		{ID: 1, Targets: []Target{{AgentID: &p100}}},
		{ID: 2, Targets: []Target{{AgentID: &p101}}},
		{ID: 3, Targets: []Target{{AgentID: &p102}}},
		{ID: 4, Targets: []Target{{Target: "fallback.example.com"}}},
		{ID: 5, Targets: nil},
	}

	got := buildTargetNameByProbeID(context.Background(), db, probes, "src")
	cases := []struct {
		id   uint
		want string
	}{
		{1, "pbx-host"},
		{2, "carrier-A"},
		{3, "carrier-B"},
		{4, "fallback.example.com"},
	}
	for _, c := range cases {
		if got[c.id] != c.want {
			t.Errorf("probe %d = %q, want %q", c.id, got[c.id], c.want)
		}
	}
	// Probe 5 has nil Targets — it shouldn't appear in the map
	// (the helper skips probes with no resolvable target).
	if _, ok := got[5]; ok {
		t.Errorf("probe 5 should not be in result, got %q", got[5])
	}
}

// TestBuildTargetNameByProbeIDSkipNonExistentAgent verifies the
// helper falls back gracefully when the referenced agent ID doesn't
// exist in the DB (race / deleted agent).
func TestBuildTargetNameByProbeIDSkipNonExistentAgent(t *testing.T) {
	db := newTestDB(t)
	missingID := uint(999)
	probes := []Probe{
		{ID: 1, Targets: []Target{{AgentID: &missingID}}},
	}
	got := buildTargetNameByProbeID(context.Background(), db, probes, "src")
	if got[1] != "src" {
		t.Errorf("probe 1 = %q, want fallback to source name", got[1])
	}
}

// TestResolveProbeTargetHostOnly verifies the resolver uses the
// probe's own Target string when no agent ID is set, and does
// not need the agent-name map.
func TestResolveProbeTargetHostOnly(t *testing.T) {
	p := Probe{
		ID: 1,
		Targets: []Target{
			{Target: "sip.example-pbx.com"},
		},
	}
	got := resolveProbeTarget(p, nil, "fallback-source")
	if got.Name != "sip.example-pbx.com" {
		t.Errorf("Name = %q, want sip.example-pbx.com", got.Name)
	}
	if got.Host != "sip.example-pbx.com" {
		t.Errorf("Host = %q, want sip.example-pbx.com", got.Host)
	}
	if got.AgentID != 0 {
		t.Errorf("AgentID = %d, want 0", got.AgentID)
	}
}

// TestResolveProbeTargetEmptyTargets verifies the resolver uses
// the fallback name when the probe has no targets.
func TestResolveProbeTargetEmptyTargets(t *testing.T) {
	p := Probe{ID: 1, Targets: nil}
	got := resolveProbeTarget(p, nil, "fallback-source")
	if got.Name != "fallback-source" {
		t.Errorf("Name = %q, want fallback-source", got.Name)
	}
}

// TestResolveProbeTargetWithAgentID verifies the resolver picks
// up the agent name from the pre-loaded name map.
func TestResolveProbeTargetWithAgentID(t *testing.T) {
	p100 := uint(100)
	p := Probe{
		ID: 1,
		Targets: []Target{
			{Target: "10.0.0.1", AgentID: &p100},
		},
	}
	got := resolveProbeTarget(p, map[uint]string{100: "pbx-host"}, "src")
	if got.Name != "pbx-host" {
		t.Errorf("Name = %q, want pbx-host (agent name wins over host)", got.Name)
	}
	if got.Host != "10.0.0.1" {
		t.Errorf("Host = %q, want 10.0.0.1", got.Host)
	}
	if got.AgentID != 100 {
		t.Errorf("AgentID = %d, want 100", got.AgentID)
	}
	if got.AgentName != "pbx-host" {
		t.Errorf("AgentName = %q, want pbx-host", got.AgentName)
	}
}

// TestBatchLoadAgentNamesNilDB verifies the batched loader returns
// an empty map (not panic) when given a nil DB.
func TestBatchLoadAgentNamesNilDB(t *testing.T) {
	got := batchLoadAgentNames(context.Background(), (*gorm.DB)(nil), map[uint]struct{}{1: {}})
	if got == nil {
		t.Fatalf("expected non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}

// TestBatchLoadAgentNamesEmptyWant verifies the batched loader
// short-circuits when no IDs are requested.
func TestBatchLoadAgentNamesEmptyWant(t *testing.T) {
	db := newTestDB(t)
	got := batchLoadAgentNames(context.Background(), db, map[uint]struct{}{})
	if got == nil {
		t.Fatalf("expected non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}

// TestBuildVoicePairAgentNameMapNoProbes verifies the helper
// returns an empty map when the probe list is empty.
func TestBuildVoicePairAgentNameMapNoProbes(t *testing.T) {
	got := buildVoicePairAgentNameMap(context.Background(), nil, nil)
	if got == nil {
		t.Fatalf("expected non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}

// TestComputeAgentVoiceQualityNilDBDoesNotPanic is the
// reproduction of the production stack trace:
//
//	buildTargetNameByProbeID → agent.GetAgentByID(ctx, nil, id)
//	  → db.WithContext(ctx) on nil db → panic
//
// When ComputeAgentVoiceQuality is called with a nil *gorm.DB and
// at least one probe has an agent-ID target, the function should
// not panic. The current contract for a nil DB is "best-effort
// with host fallbacks"; the test pins that behaviour.
func TestComputeAgentVoiceQualityNilDBDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ComputeAgentVoiceQuality panicked on nil db: %v", r)
		}
	}()

	// Synthesise a probe with an agent target. The full
	// ComputeAgentVoiceQuality path requires a real DB + ClickHouse,
	// so we can't run the whole function — but we can invoke the
	// failing helper directly to reproduce the original panic.
	agentID := uint(99)
	probes := []Probe{
		{ID: 1, Targets: []Target{{AgentID: &agentID}}},
	}
	_ = buildTargetNameByProbeID(context.Background(), nil, probes, "src")
}
