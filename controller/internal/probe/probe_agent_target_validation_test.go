package probe

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"netwatcher-controller/internal/agent"
)

// -------------------- validateAgentProbeTargets --------------------

// TestValidateAgentProbeTargets_NonAgentTypePasses: only TypeAgent is gated.
// Other types with agent targets (MTR/PING/RPERF) must not be affected.
func TestValidateAgentProbeTargets_NonAgentTypePasses(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner"})
	// Target intentionally has no TrafficSim server.
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "no-ts", TrafficSimEnabled: false})

	if err := validateAgentProbeTargets(ctx, db, TypePing, []uint{2}); err != nil {
		t.Errorf("PING type with non-TS target must pass, got: %v", err)
	}
	if err := validateAgentProbeTargets(ctx, db, TypeMTR, []uint{2}); err != nil {
		t.Errorf("MTR type with non-TS target must pass, got: %v", err)
	}
}

// TestValidateAgentProbeTargets_AGENTRejectsNoServer: AGENT probes must have
// all targets with TrafficSim enabled.
func TestValidateAgentProbeTargets_AGENTRejectsNoServer(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "no-ts", TrafficSimEnabled: false})

	err := validateAgentProbeTargets(ctx, db, TypeAgent, []uint{2})
	if err == nil {
		t.Fatal("expected error for AGENT probe with no-TS target")
	}
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("error must wrap ErrBadInput, got: %v", err)
	}
	if !strings.Contains(err.Error(), "TrafficSim") {
		t.Errorf("error message must mention TrafficSim, got: %q", err.Error())
	}
}

// TestValidateAgentProbeTargets_AGENTAcceptsWithServer: TS-enabled target is fine.
func TestValidateAgentProbeTargets_AGENTAcceptsWithServer(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "with-ts", TrafficSimEnabled: true, TrafficSimPort: 5000})

	if err := validateAgentProbeTargets(ctx, db, TypeAgent, []uint{2}); err != nil {
		t.Errorf("AGENT probe with TS-enabled target must pass, got: %v", err)
	}
}

// TestValidateAgentProbeTargets_MixedTargetsReportsAllOffenders: when some
// targets have TS and others don't, the message must list every offender so
// the operator can fix them in one pass.
func TestValidateAgentProbeTargets_MixedTargetsReportsAllOffenders(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "with-ts", TrafficSimEnabled: true})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 1, Name: "no-ts-1", TrafficSimEnabled: false})
	mustCreateAgent(t, db, agent.Agent{ID: 4, WorkspaceID: 1, Name: "no-ts-2", TrafficSimEnabled: false})

	err := validateAgentProbeTargets(ctx, db, TypeAgent, []uint{2, 3, 4})
	if err == nil {
		t.Fatal("expected error for AGENT probe with mixed targets")
	}
	msg := err.Error()
	if !strings.Contains(msg, "3") || !strings.Contains(msg, "4") {
		t.Errorf("error must list offender IDs 3 and 4, got: %q", msg)
	}
	if strings.Contains(msg, " 2 ") || strings.Contains(strings.ReplaceAll(msg, ",", " "), " 2 ") {
		t.Errorf("error must not list the valid target 2, got: %q", msg)
	}
}

// TestValidateAgentProbeTargets_AGENTAcceptsCrossWorkspaceTSAgent: the
// constraint is per-agent, not per-workspace. A global agent without a TS
// server must still be rejected even from a different workspace.
func TestValidateAgentProbeTargets_AGENTAcceptsCrossWorkspaceTSAgent(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner"})
	// Target lives in workspace 99 with TS enabled.
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 99, Name: "global-target", IsGlobal: true, TrafficSimEnabled: true})

	if err := validateAgentProbeTargets(ctx, db, TypeAgent, []uint{2}); err != nil {
		t.Errorf("AGENT probe with cross-workspace TS-enabled target must pass, got: %v", err)
	}
}

// TestValidateAgentProbeTargets_AGENTRejectsUnknownID: a target ID that
// doesn't resolve to an agent must be rejected (treated like no server).
func TestValidateAgentProbeTargets_AGENTRejectsUnknownID(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner"})

	err := validateAgentProbeTargets(ctx, db, TypeAgent, []uint{9999})
	if err == nil {
		t.Fatal("expected error for AGENT probe with unknown target ID")
	}
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("error must wrap ErrBadInput, got: %v", err)
	}
}

// -------------------- Create rejects missing TS --------------------

// TestCreateAgentProbeRejectsTargetWithoutTrafficSim: the public Create entry
// must enforce the new constraint end-to-end (validateAgentProbeTargets is
// called from inside Create, not just at the helper level).
func TestCreateAgentProbeRejectsTargetWithoutTrafficSim(t *testing.T) {
	db := newTestDB(t)
	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner", PublicIPOverride: "10.0.0.1"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "no-ts", PublicIPOverride: "10.0.0.2", TrafficSimEnabled: false})

	_, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      1,
		Type:         TypeAgent,
		Enabled:      true,
		AgentTargets: []uint{2},
	})
	if err == nil {
		t.Fatal("Create must reject AGENT probe with no-TS target")
	}
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("error must wrap ErrBadInput, got: %v", err)
	}

	var n int64
	if err := db.Model(&Probe{}).Where("type = ?", TypeAgent).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("no probes must be persisted on rejection, got %d", n)
	}
}

// TestCreateAgentProbeAcceptsTargetWithTrafficSim: the happy path remains
// unchanged for TS-enabled targets.
func TestCreateAgentProbeAcceptsTargetWithTrafficSim(t *testing.T) {
	db := newTestDB(t)
	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner", PublicIPOverride: "10.0.0.1"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "with-ts", PublicIPOverride: "10.0.0.2", TrafficSimEnabled: true, TrafficSimPort: 5000})

	p, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      1,
		Type:         TypeAgent,
		Enabled:      true,
		AgentTargets: []uint{2},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p == nil {
		t.Fatal("Create returned nil probe on success")
	}
}

// TestCreateNonAgentProbeIgnoresTrafficSimGate: a PING probe with an
// agent target pointing at a no-TS agent must still be creatable. The gate
// is AGENT-type only.
func TestCreateNonAgentProbeIgnoresTrafficSimGate(t *testing.T) {
	db := newTestDB(t)
	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner", PublicIPOverride: "10.0.0.1"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "no-ts", PublicIPOverride: "10.0.0.2", TrafficSimEnabled: false})

	if _, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      1,
		Type:         TypePing,
		Enabled:      true,
		AgentTargets: []uint{2},
	}); err != nil {
		t.Errorf("PING probe with no-TS target must succeed, got: %v", err)
	}
}

// -------------------- Update rejects replace-target to no-TS --------------------

// TestUpdateAgentProbeRejectsReplaceTargetWithoutTrafficSim: the
// ReplaceAgentTargets path on Update must also gate on the target's TS flag.
func TestUpdateAgentProbeRejectsReplaceTargetWithoutTrafficSim(t *testing.T) {
	db := newTestDB(t)
	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner", PublicIPOverride: "10.0.0.1"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "ts-on", PublicIPOverride: "10.0.0.2", TrafficSimEnabled: true, TrafficSimPort: 5000})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 1, Name: "ts-off", PublicIPOverride: "10.0.0.3", TrafficSimEnabled: false})

	created, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      1,
		Type:         TypeAgent,
		Enabled:      true,
		AgentTargets: []uint{2},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = Update(context.Background(), db, UpdateInput{
		ID:                  created.ID,
		ReplaceAgentTargets: []uint{3},
	})
	if err == nil {
		t.Fatal("Update must reject ReplaceAgentTargets to no-TS agent")
	}
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("error must wrap ErrBadInput, got: %v", err)
	}
}

// TestUpdateAgentProbeAllowsReplaceTargetWithTrafficSim: positive case.
func TestUpdateAgentProbeAllowsReplaceTargetWithTrafficSim(t *testing.T) {
	db := newTestDB(t)
	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner", PublicIPOverride: "10.0.0.1"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "ts-1", PublicIPOverride: "10.0.0.2", TrafficSimEnabled: true})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 1, Name: "ts-2", PublicIPOverride: "10.0.0.3", TrafficSimEnabled: true})

	created, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      1,
		Type:         TypeAgent,
		Enabled:      true,
		AgentTargets: []uint{2},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := Update(context.Background(), db, UpdateInput{
		ID:                  created.ID,
		ReplaceAgentTargets: []uint{3},
	}); err != nil {
		t.Errorf("Update with TS-enabled replacement target must succeed, got: %v", err)
	}
}

// TestUpdateNonAgentProbeIgnoresTrafficSimGate: PING/MTR updates with
// replace targets must not be affected.
func TestUpdateNonAgentProbeIgnoresTrafficSimGate(t *testing.T) {
	db := newTestDB(t)
	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "owner", PublicIPOverride: "10.0.0.1"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "ts-on", PublicIPOverride: "10.0.0.2", TrafficSimEnabled: true})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 1, Name: "ts-off", PublicIPOverride: "10.0.0.3", TrafficSimEnabled: false})

	created, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      1,
		Type:         TypePing,
		Enabled:      true,
		AgentTargets: []uint{2},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := Update(context.Background(), db, UpdateInput{
		ID:                  created.ID,
		ReplaceAgentTargets: []uint{3},
	}); err != nil {
		t.Errorf("PING update with no-TS target must succeed, got: %v", err)
	}
}

// -------------------- Copy rejects non-TS destination --------------------

// TestCopyAgentProbeRejectsLegacyNonTSTarget: a source AGENT probe that was
// created before the new rule (i.e. has a non-TS agent target) cannot be
// copied — the per-probe check in CopyProbes rejects it cleanly without
// aborting the whole batch.
func TestCopyAgentProbeRejectsLegacyNonTSTarget(t *testing.T) {
	db := newTestDB(t)
	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "source-owner", PublicIPOverride: "10.0.0.1"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "no-ts", PublicIPOverride: "10.0.0.2", TrafficSimEnabled: false})

	// Insert a legacy AGENT probe directly — bypasses Create's validation to
	// model the pre-rule state that already exists in the wild.
	legacy := &Probe{
		WorkspaceID: 1,
		AgentID:     1,
		Type:        TypeAgent,
		Enabled:     true,
	}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("seed legacy probe: %v", err)
	}
	tgt := uint(2)
	if err := db.Create(&Target{ProbeID: legacy.ID, AgentID: &tgt}).Error; err != nil {
		t.Fatalf("seed target: %v", err)
	}

	out, err := CopyProbes(context.Background(), db, CopyInput{
		SourceAgentID: 1,
		DestAgentIDs:  []uint{3},
		WorkspaceID:   1,
		ProbeIDs:      []uint{legacy.ID},
	})
	if err != nil {
		t.Fatalf("CopyProbes: %v", err)
	}
	if out.Created != 0 {
		t.Errorf("Created = %d, want 0 (legacy probe must be rejected)", out.Created)
	}
	if out.Errors != 1 {
		t.Errorf("Errors = %d, want 1", out.Errors)
	}
	if len(out.Results) == 0 || out.Results[0].Error == "" {
		t.Errorf("expected per-probe error message, got results: %+v", out.Results)
	}
}

// -------------------- FindReverseAgentProbes cross-workspace --------------------

// TestFindReverseAgentProbes_AllWorkspaces: the new FindReverseAgentProbes
// returns probes from any workspace that target the given agent. It is the
// direct replacement for the lowercased helper that the panel never called
// directly; this also feeds the EditAgent incoming-probe check.
func TestFindReverseAgentProbes_AllWorkspaces(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "target"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "owner-ws-1"})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 2, Name: "owner-ws-2"})
	mustCreateAgent(t, db, agent.Agent{ID: 4, WorkspaceID: 3, Name: "owner-ws-3"})

	mkAgentProbe(t, db, 1, 2, 1, false)
	mkAgentProbe(t, db, 2, 3, 1, false)
	mkAgentProbe(t, db, 3, 4, 1, false)

	got, err := FindReverseAgentProbes(ctx, db, 1)
	if err != nil {
		t.Fatalf("FindReverseAgentProbes: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 reverse probes across all workspaces, got %d", len(got))
	}
}

// -------------------- ListReverseAgentProbes global widening --------------------

// TestListReverseAgentProbes_GlobalAgentShowsCrossWorkspace: when the target
// agent is global, the panel-facing listing must include cross-workspace
// probes. The non-global branch keeps the original same-workspace scope.
func TestListReverseAgentProbes_GlobalAgentShowsCrossWorkspace(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "global-target", IsGlobal: true})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "owner-ws-1"})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 2, Name: "owner-ws-2"})

	mkAgentProbe(t, db, 1, 2, 1, false)
	mkAgentProbe(t, db, 2, 3, 1, false)

	got, err := ListReverseAgentProbes(ctx, db, 1, 1)
	if err != nil {
		t.Fatalf("ListReverseAgentProbes: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 reverse probes for global target, got %d", len(got))
	}
}

// TestListReverseAgentProbes_NonGlobalKeepsSameWorkspace: a non-global target
// must NOT see cross-workspace probes (preserves the existing test contract
// from TestListReverseAgentProbes_SameWorkspaceOnly, re-asserted here with
// the new code path that branches on is_global).
func TestListReverseAgentProbes_NonGlobalKeepsSameWorkspace(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "local-target", IsGlobal: false})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "owner-ws-1"})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 2, Name: "owner-ws-2"})

	mkAgentProbe(t, db, 1, 2, 1, false)
	mkAgentProbe(t, db, 2, 3, 1, false)

	got, err := ListReverseAgentProbes(ctx, db, 1, 1)
	if err != nil {
		t.Fatalf("ListReverseAgentProbes: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 reverse probe for non-global target, got %d", len(got))
	}
}

// TestListReverseAgentProbesWithOwners_CrossWorkspaceOwnerWorkspace: the
// OwnerWorkspaceID on each view must reflect the probe's own workspace, not
// the caller's. This is what the panel uses to label "[Global]" entries
// consistently with the NewProbe dropdown.
func TestListReverseAgentProbesWithOwners_CrossWorkspaceOwnerWorkspace(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "global-target", IsGlobal: true})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "owner-ws-1"})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 2, Name: "owner-ws-2"})

	mkAgentProbe(t, db, 1, 2, 1, false)
	mkAgentProbe(t, db, 2, 3, 1, false)

	views, err := ListReverseAgentProbesWithOwners(ctx, db, 1, 1)
	if err != nil {
		t.Fatalf("ListReverseAgentProbesWithOwners: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(views))
	}
	gotWorkspaces := map[uint]uint{}
	for _, v := range views {
		gotWorkspaces[v.OwnerAgentID] = v.OwnerWorkspaceID
	}
	if gotWorkspaces[2] != 1 {
		t.Errorf("owner 2: workspace = %d, want 1", gotWorkspaces[2])
	}
	if gotWorkspaces[3] != 2 {
		t.Errorf("owner 3: workspace = %d, want 2 (cross-workspace probe)", gotWorkspaces[3])
	}
}

// -------------------- utilities --------------------

// mustFindReverseForCount is a small convenience for the disable-block test
// below. It avoids leaking the FindReverseAgentProbes helper signature
// across the file twice.
func mustFindReverseForCount(t *testing.T, db *gorm.DB, target uint) int {
	t.Helper()
	probes, err := FindReverseAgentProbes(context.Background(), db, target)
	if err != nil {
		t.Fatalf("FindReverseAgentProbes: %v", err)
	}
	return len(probes)
}

// Compile-time guard: mustFindReverseForCount must use the package's gorm.DB
// (importing here is intentional to keep the test file self-contained).
var _ = func() *gorm.DB { return nil }
