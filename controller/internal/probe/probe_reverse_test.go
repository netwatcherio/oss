package probe

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"netwatcher-controller/internal/agent"
)

// TestListReverseAgentProbes_SameWorkspaceOnly: a reverse AGENT probe whose
// owner lives in a different workspace must not leak into the target agent's
// reverse-probe listing. Cross-workspace is the scope the user explicitly
// rejected during planning.
func TestListReverseAgentProbes_SameWorkspaceOnly(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 10, Name: "target"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 10, Name: "owner-same-ws"})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 20, Name: "owner-other-ws"})

	// Same-workspace owner: should appear.
	mkAgentProbe(t, db, 10, 2, 1, false)
	// Cross-workspace owner: must NOT appear.
	mkAgentProbe(t, db, 20, 3, 1, false)

	got, err := ListReverseAgentProbes(ctx, db, 10, 1)
	if err != nil {
		t.Fatalf("ListReverseAgentProbes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 reverse probe (same-ws only), got %d", len(got))
	}
	if got[0].AgentID != 2 {
		t.Errorf("reverse owner = %d, want 2", got[0].AgentID)
	}
}

// TestListReverseAgentProbes_ExcludesDisabledAndDeleted: defensive filters —
// disabled and soft-deleted probes must not surface. The Probe model has
// gorm:"default:true" on the Enabled field, which makes GORM ignore an
// explicit `false` at INSERT time, so we toggle disabled probes after
// creation via a direct UPDATE.
func TestListReverseAgentProbes_ExcludesDisabledAndDeleted(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "target"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "owner-disabled"})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 1, Name: "owner-deleted"})

	mkAgentProbe(t, db, 1, 2, 1, false)
	mkAgentProbe(t, db, 1, 3, 1, false)
	// Disable probe owned by agent 2.
	if err := db.Model(&Probe{}).
		Where("agent_id = ? AND workspace_id = ?", uint(2), uint(1)).
		Update("enabled", false).Error; err != nil {
		t.Fatalf("disable: %v", err)
	}
	// Soft-delete probe owned by agent 3.
	if err := db.Delete(&Probe{}, "agent_id = ? AND workspace_id = ?", uint(3), uint(1)).Error; err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	got, err := ListReverseAgentProbes(ctx, db, 1, 1)
	if err != nil {
		t.Fatalf("ListReverseAgentProbes: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 reverse probes (all excluded), got %d", len(got))
	}
}

// TestListReverseAgentProbesWithOwners_PopulatesBidirectionalFlag: the
// owner-enriched view must pre-compute the bidirectional flag from each
// probe's metadata, so the panel can render badges without re-parsing JSON.
func TestListReverseAgentProbesWithOwners_PopulatesBidirectionalFlag(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	mustCreateAgent(t, db, agent.Agent{ID: 1, WorkspaceID: 1, Name: "target"})
	mustCreateAgent(t, db, agent.Agent{ID: 2, WorkspaceID: 1, Name: "bidir-owner"})
	mustCreateAgent(t, db, agent.Agent{ID: 3, WorkspaceID: 1, Name: "oneway-owner"})

	mkAgentProbeWithMetadata(t, db, 1, 2, 1, `{"bidirectional":true}`)
	mkAgentProbeWithMetadata(t, db, 1, 3, 1, `{}`)

	views, err := ListReverseAgentProbesWithOwners(ctx, db, 1, 1)
	if err != nil {
		t.Fatalf("ListReverseAgentProbesWithOwners: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(views))
	}
	byOwner := map[uint]ReverseProbeView{}
	for _, v := range views {
		byOwner[v.OwnerAgentID] = v
	}
	if v, ok := byOwner[2]; !ok {
		t.Error("missing owner 2 in views")
	} else {
		if !v.Bidirectional {
			t.Error("owner 2: expected Bidirectional=true")
		}
		if v.OwnerAgentName != "bidir-owner" {
			t.Errorf("owner 2: name = %q, want %q", v.OwnerAgentName, "bidir-owner")
		}
		if v.OwnerWorkspaceID != 1 {
			t.Errorf("owner 2: workspace = %d, want 1", v.OwnerWorkspaceID)
		}
	}
	if v, ok := byOwner[3]; !ok {
		t.Error("missing owner 3 in views")
	} else if v.Bidirectional {
		t.Error("owner 3: expected Bidirectional=false")
	}
}

// TestListReverseAgentProbes_InvalidArgs: zero workspace or agent IDs must
// return ErrBadInput rather than silently running an unbounded query.
func TestListReverseAgentProbes_InvalidArgs(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if _, err := ListReverseAgentProbes(ctx, db, 0, 1); err == nil {
		t.Error("expected error for workspaceID=0")
	}
	if _, err := ListReverseAgentProbes(ctx, db, 1, 0); err == nil {
		t.Error("expected error for targetAgentID=0")
	}
}

// ---- helpers ----

func mustCreateAgent(t *testing.T, db *gorm.DB, a agent.Agent) {
	t.Helper()
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("seed agent %d: %v", a.ID, err)
	}
}

// mkAgentProbe inserts an enabled AGENT-type probe owned by `ownerID` in
// `wsID` with one Target pointing at `targetID`. The `disabled` flag lets the
// caller test the enabled-filter branch.
func mkAgentProbe(t *testing.T, db *gorm.DB, wsID, ownerID, targetID uint, disabled bool) {
	t.Helper()
	enabled := !disabled
	p := &Probe{
		WorkspaceID: wsID,
		AgentID:     ownerID,
		Type:        TypeAgent,
		Enabled:     enabled,
	}
	if err := db.Create(p).Error; err != nil {
		t.Fatalf("create probe: %v", err)
	}
	tgt := targetID
	if err := db.Create(&Target{ProbeID: p.ID, AgentID: &tgt}).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
}

// mkAgentProbeWithMetadata is mkAgentProbe + a custom metadata blob (used to
// drive the bidirectional flag in the owner-enriched view).
func mkAgentProbeWithMetadata(t *testing.T, db *gorm.DB, wsID, ownerID, targetID uint, meta string) {
	t.Helper()
	p := &Probe{
		WorkspaceID: wsID,
		AgentID:     ownerID,
		Type:        TypeAgent,
		Enabled:     true,
		Metadata:    []byte(meta),
	}
	if err := db.Create(p).Error; err != nil {
		t.Fatalf("create probe: %v", err)
	}
	tgt := targetID
	if err := db.Create(&Target{ProbeID: p.ID, AgentID: &tgt}).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
}
