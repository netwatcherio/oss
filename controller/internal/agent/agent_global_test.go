package agent

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newAgentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&Agent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func mustCreateAgentRow(t *testing.T, db *gorm.DB, a Agent) {
	t.Helper()
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("seed agent %d: %v", a.ID, err)
	}
}

// TestListGlobalAgentsForWorkspace_IncludesOwnWorkspace: the panel endpoint
// must surface the caller's own global agents too, so the user can mark them
// with a [Global] prefix in the probe-create dropdown. Previously the endpoint
// called the exclude-current-workspace variant, which hid own-workspace
// globals from the user.
func TestListGlobalAgentsForWorkspace_IncludesOwnWorkspace(t *testing.T) {
	db := newAgentTestDB(t)
	ctx := context.Background()

	mustCreateAgentRow(t, db, Agent{ID: 1, WorkspaceID: 1, Name: "local-global", IsGlobal: true})
	mustCreateAgentRow(t, db, Agent{ID: 2, WorkspaceID: 1, Name: "local-regular"})
	mustCreateAgentRow(t, db, Agent{ID: 3, WorkspaceID: 2, Name: "cross-global", IsGlobal: true})
	mustCreateAgentRow(t, db, Agent{ID: 4, WorkspaceID: 2, Name: "cross-regular"})

	got, err := ListGlobalAgentsForWorkspace(ctx, db)
	if err != nil {
		t.Fatalf("ListGlobalAgentsForWorkspace: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 global agents (own + cross), got %d", len(got))
	}
	seen := map[uint]bool{}
	for _, a := range got {
		seen[a.ID] = true
	}
	if !seen[1] {
		t.Error("missing own-workspace global (id=1)")
	}
	if !seen[3] {
		t.Error("missing cross-workspace global (id=3)")
	}
	if seen[2] || seen[4] {
		t.Error("non-global agents leaked into result")
	}
}

// TestListGlobalAgentsForWorkspace_Empty: with no global agents in the DB,
// the endpoint returns an empty slice (not nil) so the JSON response is a
// stable `{"data": []}` shape — keeps the panel's defensive `|| []` working.
func TestListGlobalAgentsForWorkspace_Empty(t *testing.T) {
	db := newAgentTestDB(t)
	mustCreateAgentRow(t, db, Agent{ID: 1, WorkspaceID: 1, Name: "regular", IsGlobal: false})

	got, err := ListGlobalAgentsForWorkspace(context.Background(), db)
	if err != nil {
		t.Fatalf("ListGlobalAgentsForWorkspace: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 globals, got %d", len(got))
	}
}

// TestListGlobalAgentsExcludingWorkspace_StillExcludesOwn: confirm the
// previous behaviour is preserved on the legacy function so any external
// system that links against it isn't silently changed.
func TestListGlobalAgentsExcludingWorkspace_StillExcludesOwn(t *testing.T) {
	db := newAgentTestDB(t)
	mustCreateAgentRow(t, db, Agent{ID: 1, WorkspaceID: 1, Name: "local-global", IsGlobal: true})
	mustCreateAgentRow(t, db, Agent{ID: 2, WorkspaceID: 2, Name: "cross-global", IsGlobal: true})

	got, err := ListGlobalAgentsExcludingWorkspace(context.Background(), db, 1)
	if err != nil {
		t.Fatalf("ListGlobalAgentsExcludingWorkspace: %v", err)
	}
	if len(got) != 1 || got[0].ID != 2 {
		t.Errorf("expected only cross-workspace global (id=2), got %+v", got)
	}
}
