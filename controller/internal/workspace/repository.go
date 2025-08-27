package workspace

import (
	"context"
	"errors"
	"fmt"
	"netwatcher-controller/internal/agent"
	"time"

	"gorm.io/gorm"
)

var (
	ErrWorkspaceNotFound = errors.New("workspace not found")
	ErrMemberNotFound    = errors.New("workspace member not found")
	ErrRoleInvalid       = errors.New("invalid role")
	ErrOwnerExists       = errors.New("workspace already has an owner")
	ErrNoOwner           = errors.New("workspace must have an owner")
	ErrDuplicateMember   = errors.New("member already exists in workspace")
	ErrNotOwner          = errors.New("only the owner can perform this action")
)

type Repository interface {
	// Workspaces
	CreateWorkspace(ctx context.Context, ws *Workspace) error
	GetWorkspace(ctx context.Context, id uint) (*Workspace, error)
	UpdateWorkspace(ctx context.Context, ws *Workspace) error
	DeleteWorkspace(ctx context.Context, id uint) error
	ListWorkspacesForUser(ctx context.Context, userID uint, limit, offset int) ([]Workspace, int64, error)

	// Members
	AddMemberByUserID(ctx context.Context, workspaceID, userID uint, role Role) (*WorkspaceMember, error)
	InviteMemberByEmail(ctx context.Context, workspaceID uint, email string, role Role) (*WorkspaceMember, error)
	ListMembers(ctx context.Context, workspaceID uint) ([]WorkspaceMember, error)
	UpdateMemberRole(ctx context.Context, workspaceID, memberID uint, role Role) error
	RemoveMember(ctx context.Context, workspaceID, memberID uint) error
	TransferOwnership(ctx context.Context, workspaceID, newOwnerMemberID uint) error

	// Agents convenience
	ListAgentsByWorkspace(ctx context.Context, workspaceID uint, limit, offset int) ([]agent.Agent, int64, error)
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

// ---------- Workspaces ----------

func (r *gormRepo) CreateWorkspace(ctx context.Context, ws *Workspace) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Basic defaults
		if ws.Labels == nil {
			ws.Labels = []byte(`{}`)
		}
		if ws.Metadata == nil {
			ws.Metadata = []byte(`{}`)
		}

		// Enforce exactly one owner (OwnerUserID must be set)
		if ws.OwnerUserID == 0 {
			return ErrNoOwner
		}

		// Create WS
		if err := tx.Create(ws).Error; err != nil {
			return err
		}

		// Create OWNER member row
		owner := &WorkspaceMember{
			WorkspaceID: ws.ID,
			UserID:      ws.OwnerUserID,
			Email:       "",
			Role:        RoleOwner,
			InvitedAt:   nil,
			AcceptedAt:  ptrTime(time.Now()),
		}
		if err := tx.Create(owner).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *gormRepo) GetWorkspace(ctx context.Context, id uint) (*Workspace, error) {
	var ws Workspace
	err := r.db.WithContext(ctx).First(&ws, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWorkspaceNotFound
	}
	return &ws, err
}

func (r *gormRepo) UpdateWorkspace(ctx context.Context, ws *Workspace) error {
	return r.db.WithContext(ctx).Save(ws).Error
}

func (r *gormRepo) DeleteWorkspace(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&Workspace{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

func (r *gormRepo) ListWorkspacesForUser(ctx context.Context, userID uint, limit, offset int) ([]Workspace, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	var (
		items []Workspace
		total int64
	)

	// Join via workspace_members
	sub := r.db.WithContext(ctx).Model(&WorkspaceMember{}).
		Select("workspace_id").
		Where("user_id = ? AND revoked_at IS NULL", userID)

	q := r.db.WithContext(ctx).Model(&Workspace{}).Where("id IN (?)", sub)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ---------- Members ----------

func (r *gormRepo) AddMemberByUserID(ctx context.Context, workspaceID, userID uint, role Role) (*WorkspaceMember, error) {
	if role == "" {
		role = RoleReadOnly
	}
	if !isValidRole(role) {
		return nil, ErrRoleInvalid
	}
	m := &WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      userID,
		Role:        role,
		AcceptedAt:  ptrTime(time.Now()),
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// check duplicate by (workspace_id,user_id)
		var cnt int64
		if err := tx.Model(&WorkspaceMember{}).
			Where("workspace_id = ? AND user_id = ? AND revoked_at IS NULL", workspaceID, userID).
			Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			return ErrDuplicateMember
		}
		return tx.Create(m).Error
	})
	return m, err
}

func (r *gormRepo) InviteMemberByEmail(ctx context.Context, workspaceID uint, email string, role Role) (*WorkspaceMember, error) {
	if role == "" {
		role = RoleReadOnly
	}
	if !isValidRole(role) || role == RoleOwner {
		return nil, ErrRoleInvalid
	}
	now := time.Now()
	m := &WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      0,
		Email:       email,
		Role:        role,
		InvitedAt:   &now,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Soft uniqueness on (workspace_id,email) where not revoked/accepted
		var cnt int64
		if err := tx.Model(&WorkspaceMember{}).
			Where("workspace_id = ? AND email = ? AND revoked_at IS NULL AND accepted_at IS NULL", workspaceID, email).
			Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			return ErrDuplicateMember
		}
		return tx.Create(m).Error
	})
	return m, err
}

func (r *gormRepo) ListMembers(ctx context.Context, workspaceID uint) ([]WorkspaceMember, error) {
	var members []WorkspaceMember
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND revoked_at IS NULL", workspaceID).
		Order("role DESC, id ASC").
		Find(&members).Error
	return members, err
}

func (r *gormRepo) UpdateMemberRole(ctx context.Context, workspaceID, memberID uint, role Role) error {
	if !isValidRole(role) {
		return ErrRoleInvalid
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m WorkspaceMember
		if err := tx.First(&m, "id = ? AND workspace_id = ?", memberID, workspaceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMemberNotFound
			}
			return err
		}
		// Prevent creating multiple owners accidentally
		if role == RoleOwner {
			// ensure no other owner
			var cnt int64
			if err := tx.Model(&WorkspaceMember{}).
				Where("workspace_id = ? AND role = ? AND id <> ? AND revoked_at IS NULL", workspaceID, RoleOwner, memberID).
				Count(&cnt).Error; err != nil {
				return err
			}
			if cnt > 0 {
				return ErrOwnerExists
			}
		}
		return tx.Model(&WorkspaceMember{}).Where("id = ?", m.ID).Update("role", role).Error
	})
}

func (r *gormRepo) RemoveMember(ctx context.Context, workspaceID, memberID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m WorkspaceMember
		if err := tx.First(&m, "id = ? AND workspace_id = ?", memberID, workspaceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMemberNotFound
			}
			return err
		}
		// Can't remove the owner via plain removal; must transfer or delete workspace.
		if m.Role == RoleOwner {
			return ErrNotOwner
		}
		now := time.Now()
		return tx.Model(&WorkspaceMember{}).Where("id = ?", m.ID).
			Updates(map[string]any{
				"revoked_at": &now,
				"updated_at": time.Now(),
			}).Error
	})
}

func (r *gormRepo) TransferOwnership(ctx context.Context, workspaceID, newOwnerMemberID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Find current owner
		var currentOwner WorkspaceMember
		if err := tx.First(&currentOwner, "workspace_id = ? AND role = ? AND revoked_at IS NULL", workspaceID, RoleOwner).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoOwner
			}
			return err
		}
		// Find target
		var target WorkspaceMember
		if err := tx.First(&target, "id = ? AND workspace_id = ? AND revoked_at IS NULL", newOwnerMemberID, workspaceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMemberNotFound
			}
			return err
		}
		// Update roles
		if err := tx.Model(&WorkspaceMember{}).Where("id = ?", currentOwner.ID).Update("role", RoleAdmin).Error; err != nil {
			return err
		}
		if err := tx.Model(&WorkspaceMember{}).Where("id = ?", target.ID).Update("role", RoleOwner).Error; err != nil {
			return err
		}
		// Update denormalized OwnerUserID on workspace if target has UserID
		if target.UserID != 0 {
			if err := tx.Model(&Workspace{}).Where("id = ?", workspaceID).Update("owner_user_id", target.UserID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ---------- Agents convenience ----------

func (r *gormRepo) ListAgentsByWorkspace(ctx context.Context, workspaceID uint, limit, offset int) ([]agent.Agent, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	var (
		items []agent.Agent
		total int64
	)
	q := r.db.WithContext(ctx).Model(&agent.Agent{}).Where("workspace_id = ?", workspaceID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ---------- helpers ----------

func isValidRole(r Role) bool {
	switch r {
	case RoleReadOnly, RoleReadWrite, RoleAdmin, RoleOwner:
		return true
	default:
		return false
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// Optional: ensure indexes you care about exist beyond AutoMigrate.
func ensureIndexes(db *gorm.DB) error {
	type idx struct{ name, sql string }
	indexes := []idx{
		// Composite for (workspace_id, user_id) uniqueness when user bound
		{
			name: "idx_members_ws_user",
			sql:  "CREATE INDEX IF NOT EXISTS idx_members_ws_user ON workspace_members (workspace_id, user_id)",
		},
	}
	for _, ix := range indexes {
		if err := db.Exec(ix.sql).Error; err != nil {
			return fmt.Errorf("create index %s: %w", ix.name, err)
		}
	}
	return nil
}
