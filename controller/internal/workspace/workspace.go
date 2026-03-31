package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// --- Roles ---

type Role string

const (
	RoleViewer Role = "VIEWER"
	RoleUser   Role = "USER"
	RoleAdmin  Role = "ADMIN"
	RoleOwner  Role = "OWNER"
)

func (r Role) Valid() bool {
	switch r {
	case RoleViewer, RoleUser, RoleAdmin, RoleOwner:
		return true
	default:
		return false
	}
}

// --- Models ---

type Workspace struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"size:200;not null;uniqueIndex:uniq_ws_name" json:"name"`
	OwnerID   uint           `gorm:"not null;index" json:"owner_id"`
	Settings  datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"settings"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// denormalized convenience
	Description string `gorm:"size:255" json:"description"`
}

func (Workspace) TableName() string { return "workspaces" }

type Member struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	WorkspaceID uint           `gorm:"not null;index" json:"workspace_id"`
	UserID      uint           `gorm:"default:0;index" json:"user_id"` // 0 means invited by email only
	Email       string         `gorm:"size:320;default:'';index" json:"email"`
	Role        Role           `gorm:"size:20;not null;index" json:"role"`
	Meta        datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"meta"`

	CreatedAt time.Time      `json:"created_At"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	InvitedAt  *time.Time `json:"invited_at"`
	AcceptedAt *time.Time `json:"accepted_at"`
	RevokedAt  *time.Time `json:"revoked_at"`

	// Invite token fields
	InviteToken       string     `gorm:"size:64;index" json:"-"`
	InviteTokenExpiry *time.Time `json:"-"`
	InviteEmailSent   bool       `gorm:"default:false" json:"invite_email_sent"`
}

func (Member) TableName() string { return "workspace_members" }

// --- Public Errors ---

var (
	ErrNotFound             = errors.New("not found")
	ErrInvalidInput         = errors.New("invalid input")
	ErrInvalidRole          = errors.New("invalid role")
	ErrAlreadyExists        = errors.New("already exists")
	ErrForbidden            = errors.New("forbidden")
	ErrEmailRequired        = errors.New("email required")
	ErrWorkspaceHasOwner    = errors.New("workspace already has an owner")
	ErrMemberNotInWorkspace = errors.New("member does not belong to this workspace")
)

// --- Store (single simple entrypoint) ---

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// AutoMigrate applies schema and helpful indexes. Call once at startup.
func (s *Store) AutoMigrate(ctx context.Context) error {
	if err := s.db.WithContext(ctx).AutoMigrate(&Workspace{}, &Member{}); err != nil {
		return err
	}
	// helpful composite indexes
	stmts := []string{
		"CREATE INDEX IF NOT EXISTS idx_ws_owner ON workspaces (owner_id)",
		"CREATE INDEX IF NOT EXISTS idx_members_ws ON workspace_members (workspace_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS uniq_members_ws_user ON workspace_members (workspace_id, user_id) WHERE user_id <> 0",
		"CREATE UNIQUE INDEX IF NOT EXISTS uniq_members_ws_email ON workspace_members (workspace_id, email) WHERE email <> ''",
	}
	for _, sql := range stmts {
		if err := s.db.Exec(sql).Error; err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}
	return nil
}

// --- Helpers ---

func normEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}

func jdefault(j datatypes.JSON) datatypes.JSON {
	if len(j) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return j
}

// --- Workspace API ---

type CreateWorkspaceInput struct {
	Name        string         // required, unique
	OwnerID     uint           // required
	Description string         // optional
	Settings    datatypes.JSON // optional
}

func (s *Store) CreateWorkspace(ctx context.Context, in CreateWorkspaceInput) (*Workspace, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || in.OwnerID == 0 {
		return nil, ErrInvalidInput
	}
	ws := &Workspace{
		Name:        name,
		OwnerID:     in.OwnerID,
		Description: strings.TrimSpace(in.Description),
		Settings:    jdefault(in.Settings),
	}
	if err := s.db.WithContext(ctx).Create(ws).Error; err != nil {
		// unique name collision
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique") {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}

	// Ensure owner is a member with OWNER role
	ownerMember := &Member{
		WorkspaceID: ws.ID,
		UserID:      in.OwnerID,
		Role:        RoleOwner,
		Meta:        datatypes.JSON([]byte(`{}`)),
	}
	_ = s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(ownerMember).Error

	return ws, nil
}

func (s *Store) GetWorkspace(ctx context.Context, id uint) (*Workspace, error) {
	var ws Workspace
	if err := s.db.WithContext(ctx).First(&ws, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ws, nil
}

func (s *Store) GetWorkspaceByName(ctx context.Context, name string) (*Workspace, error) {
	var ws Workspace
	if err := s.db.WithContext(ctx).First(&ws, "name = ?", strings.TrimSpace(name)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ws, nil
}

type ListWorkspacesFilter struct {
	OwnerID uint   // optional
	Query   string // optional name/display filter (ILIKE)
	Limit   int    // default 50
	Offset  int
}

func (s *Store) ListWorkspaces(ctx context.Context, f ListWorkspacesFilter) ([]Workspace, error) {
	db := s.db.WithContext(ctx).Model(&Workspace{})
	if f.OwnerID != 0 {
		db = db.Where("owner_id = ?", f.OwnerID)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		pat := "%" + strings.ToLower(q) + "%"
		db = db.Where("LOWER(name) LIKE ?", pat, pat)
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []Workspace
	if err := db.Order("id DESC").Limit(limit).Offset(f.Offset).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ListWorkspacesByUserID returns all workspaces where the user is a member (any role)
func (s *Store) ListWorkspacesByUserID(ctx context.Context, userID uint) ([]Workspace, error) {
	if userID == 0 {
		return nil, ErrInvalidInput
	}

	var out []Workspace
	// Join with members table to find all workspaces where user is a member
	err := s.db.WithContext(ctx).
		Model(&Workspace{}).
		Joins("INNER JOIN workspace_members ON workspace_members.workspace_id = workspaces.id").
		Where("workspace_members.user_id = ? AND workspace_members.deleted_at IS NULL", userID).
		Order("workspaces.id DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

type UpdateWorkspaceInput struct {
	Name        *string
	Description *string
	Settings    *datatypes.JSON
}

func (s *Store) UpdateWorkspace(ctx context.Context, id uint, in UpdateWorkspaceInput) (*Workspace, error) {
	var ws Workspace
	if err := s.db.WithContext(ctx).First(&ws, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	updates := map[string]any{}
	if in.Description != nil {
		updates["description"] = strings.TrimSpace(*in.Description)
	}
	if in.Name != nil {
		updates["name"] = strings.TrimSpace(*in.Name)
	}
	if in.Settings != nil {
		updates["settings"] = jdefault(*in.Settings)
	}
	if len(updates) == 0 {
		return &ws, nil
	}
	if err := s.db.WithContext(ctx).Model(&ws).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &ws, nil
}

func (s *Store) DeleteWorkspace(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) Collect all agent IDs in this workspace
		var agentIDs []uint
		if err := tx.Model(&wsAgent{}).Where("workspace_id = ?", id).Pluck("id", &agentIDs).Error; err != nil {
			return err
		}

		if len(agentIDs) > 0 {
			// Collect all probe IDs owned by these agents
			var probeIDs []uint
			if err := tx.Model(&wsProbe{}).Where("agent_id IN ?", agentIDs).Pluck("id", &probeIDs).Error; err != nil {
				return err
			}

			if len(probeIDs) > 0 {
				// Delete probe targets
				if err := tx.Where("probe_id IN ?", probeIDs).Delete(&wsTarget{}).Error; err != nil {
					return err
				}
				// Delete alert rules referencing these probes
				if err := tx.Where("probe_id IN ?", probeIDs).Delete(&wsAlertRule{}).Error; err != nil {
					return err
				}
				// Delete alerts referencing these probes
				if err := tx.Where("probe_id IN ?", probeIDs).Delete(&wsAlert{}).Error; err != nil {
					return err
				}
				// Delete route baselines for these probes
				if err := tx.Where("probe_id IN ?", probeIDs).Delete(&wsRouteBaseline{}).Error; err != nil {
					return err
				}
				// Delete the probes
				if err := tx.Where("agent_id IN ?", agentIDs).Delete(&wsProbe{}).Error; err != nil {
					return err
				}
			}

			// Delete share links for these agents
			if err := tx.Where("agent_id IN ?", agentIDs).Delete(&wsShareLink{}).Error; err != nil {
				return err
			}
			// Delete speedtest servers for these agents
			if err := tx.Where("agent_id IN ?", agentIDs).Delete(&wsSpeedtestServer{}).Error; err != nil {
				return err
			}
			// Delete speedtest queue items for these agents
			if err := tx.Where("agent_id IN ?", agentIDs).Delete(&wsSpeedtestQueue{}).Error; err != nil {
				return err
			}
			// Delete agent auth PINs
			if err := tx.Where("agent_id IN ?", agentIDs).Delete(&wsAgentPin{}).Error; err != nil {
				return err
			}
			// Delete the agents
			if err := tx.Where("workspace_id = ?", id).Delete(&wsAgent{}).Error; err != nil {
				return err
			}
		}

		// 2) Delete workspace-level alert rules & alerts
		if err := tx.Where("workspace_id = ?", id).Delete(&wsAlertRule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("workspace_id = ?", id).Delete(&wsAlert{}).Error; err != nil {
			return err
		}

		// 3) Delete workspace share links (catch any not tied to specific agents)
		if err := tx.Where("workspace_id = ?", id).Delete(&wsShareLink{}).Error; err != nil {
			return err
		}

		// 4) Delete workspace speedtest queue items
		if err := tx.Where("workspace_id = ?", id).Delete(&wsSpeedtestQueue{}).Error; err != nil {
			return err
		}

		// 5) Delete workspace members
		if err := tx.Where("workspace_id = ?", id).Delete(&Member{}).Error; err != nil {
			return err
		}

		// 6) Delete the workspace itself
		res := tx.Delete(&Workspace{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// --- Member API ---

type AddMemberInput struct {
	WorkspaceID uint
	UserID      uint   // optional if Email set
	Email       string // optional if UserID set (invite)
	Role        Role   // required
	Meta        datatypes.JSON
}

func (s *Store) AddMember(ctx context.Context, in AddMemberInput) (*Member, error) {
	if in.WorkspaceID == 0 || !in.Role.Valid() {
		return nil, ErrInvalidInput
	}
	if in.UserID == 0 && strings.TrimSpace(in.Email) == "" {
		return nil, ErrEmailRequired
	}
	if in.Role == RoleOwner {
		return nil, ErrForbidden // owner is set via workspace owner id
	}
	// Ensure workspace exists
	if err := s.db.WithContext(ctx).First(&Workspace{}, "id = ?", in.WorkspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	email := normEmail(in.Email)

	// Check for existing member (including soft-deleted) - for re-adding removed members
	var existing Member
	query := s.db.WithContext(ctx).Unscoped().Where("workspace_id = ?", in.WorkspaceID)
	if in.UserID != 0 {
		query = query.Where("user_id = ?", in.UserID)
	} else if email != "" {
		query = query.Where("email = ?", email)
	}

	if err := query.First(&existing).Error; err == nil {
		// Member found - check if soft-deleted
		if existing.DeletedAt.Valid {
			// Restore the soft-deleted member with new role
			now := time.Now()
			updates := map[string]any{
				"deleted_at":  nil,
				"role":        in.Role,
				"revoked_at":  nil,
				"accepted_at": &now, // Re-accepting
			}
			if in.UserID != 0 && existing.UserID == 0 {
				updates["user_id"] = in.UserID
			}
			if err := s.db.WithContext(ctx).Unscoped().Model(&existing).Updates(updates).Error; err != nil {
				return nil, err
			}
			existing.DeletedAt = gorm.DeletedAt{}
			existing.Role = in.Role
			return &existing, nil
		}
		// Member exists and is not deleted
		return nil, ErrAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Create new member
	m := &Member{
		WorkspaceID: in.WorkspaceID,
		UserID:      in.UserID,
		Email:       email,
		Role:        in.Role,
		Meta:        jdefault(in.Meta),
	}
	if err := s.db.WithContext(ctx).Create(m).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique") {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	return m, nil
}

func (s *Store) ListMembers(ctx context.Context, workspaceID uint) ([]Member, error) {
	var ms []Member
	if err := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).
		Order("id ASC").Find(&ms).Error; err != nil {
		return nil, err
	}
	return ms, nil
}

func (s *Store) GetMemberByID(ctx context.Context, memberID uint) (*Member, error) {
	var m Member
	if err := s.db.WithContext(ctx).First(&m, "id = ?", memberID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (s *Store) UpdateMemberRole(ctx context.Context, workspaceID, memberID uint, newRole Role) (*Member, error) {
	if !newRole.Valid() || newRole == RoleOwner {
		return nil, ErrInvalidRole
	}
	var m Member
	if err := s.db.WithContext(ctx).First(&m, "id = ? AND workspace_id = ?", memberID, workspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&m).Update("role", newRole).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) RemoveMember(ctx context.Context, workspaceID, memberID uint) error {
	res := s.db.WithContext(ctx).Delete(&Member{}, "id = ? AND workspace_id = ?", memberID, workspaceID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AcceptInvite attaches an invited email-only member to a concrete UserID.
func (s *Store) AcceptInvite(ctx context.Context, workspaceID uint, email string, userID uint) (*Member, error) {
	if workspaceID == 0 || userID == 0 || strings.TrimSpace(email) == "" {
		return nil, ErrInvalidInput
	}
	email = normEmail(email)

	var m Member
	err := s.db.WithContext(ctx).First(&m, "workspace_id = ? AND email = ?", workspaceID, email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	now := time.Now()
	updates := map[string]any{
		"user_id":     userID,
		"accepted_at": &now,
	}
	if err := s.db.WithContext(ctx).Model(&m).Updates(updates).Error; err != nil {
		// handle unique collision with existing (workspace_id, user_id)
		if strings.Contains(err.Error(), "unique") {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	return &m, nil
}

// TransferOwnership changes the workspace OwnerID and ensures membership.
func (s *Store) TransferOwnership(ctx context.Context, workspaceID uint, newOwnerUserID uint) error {
	if workspaceID == 0 || newOwnerUserID == 0 {
		return ErrInvalidInput
	}
	// set owner on workspace
	if err := s.db.WithContext(ctx).Model(&Workspace{}).
		Where("id = ?", workspaceID).
		Update("owner_id", newOwnerUserID).Error; err != nil {
		return err
	}
	// ensure owner member entry exists
	ownerMember := &Member{
		WorkspaceID: workspaceID,
		UserID:      newOwnerUserID,
		Role:        RoleOwner,
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(ownerMember).Error
}

// --- Access Control ---

// UserHasAccess checks if a user is a member of a workspace.
// Returns true if the user is a member (any role), false otherwise.
func (s *Store) UserHasAccess(ctx context.Context, workspaceID, userID uint) bool {
	if workspaceID == 0 || userID == 0 {
		return false
	}
	var count int64
	s.db.WithContext(ctx).Model(&Member{}).
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		Count(&count)
	return count > 0
}

// GetMemberByUserID returns the member record for a user in a workspace.
// Returns ErrNotFound if the user is not a member.
func (s *Store) GetMemberByUserID(ctx context.Context, workspaceID, userID uint) (*Member, error) {
	if workspaceID == 0 || userID == 0 {
		return nil, ErrInvalidInput
	}
	var m Member
	if err := s.db.WithContext(ctx).
		First(&m, "workspace_id = ? AND user_id = ?", workspaceID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// UserHasRole checks if a user has at least the specified role in a workspace.
// Role hierarchy: OWNER > ADMIN > READ_WRITE > READ_ONLY
func (s *Store) UserHasRole(ctx context.Context, workspaceID, userID uint, minRole Role) bool {
	m, err := s.GetMemberByUserID(ctx, workspaceID, userID)
	if err != nil {
		return false
	}
	return roleAtLeast(m.Role, minRole)
}

// roleAtLeast returns true if role >= minRole in the hierarchy.
func roleAtLeast(role, minRole Role) bool {
	hierarchy := map[Role]int{
		RoleViewer: 1,
		RoleUser:   2,
		RoleAdmin:  3,
		RoleOwner:  4,
	}
	return hierarchy[role] >= hierarchy[minRole]
}

// -------------------- Local table models for cascade deletion --------------------
// These avoid circular imports with other packages.

type wsAgent struct {
	ID uint `gorm:"primaryKey"`
}

func (wsAgent) TableName() string { return "agents" }

type wsProbe struct {
	ID uint `gorm:"primaryKey"`
}

func (wsProbe) TableName() string { return "probes" }

type wsTarget struct {
	ID uint `gorm:"primaryKey"`
}

func (wsTarget) TableName() string { return "probe_targets" }

type wsAlertRule struct {
	ID uint `gorm:"primaryKey"`
}

func (wsAlertRule) TableName() string { return "alert_rules" }

type wsAlert struct {
	ID uint `gorm:"primaryKey"`
}

func (wsAlert) TableName() string { return "alerts" }

type wsRouteBaseline struct {
	ID uint `gorm:"primaryKey"`
}

func (wsRouteBaseline) TableName() string { return "route_baselines" }

type wsShareLink struct {
	ID uint `gorm:"primaryKey"`
}

func (wsShareLink) TableName() string { return "share_links" }

type wsSpeedtestServer struct {
	ID uint `gorm:"primaryKey"`
}

func (wsSpeedtestServer) TableName() string { return "agent_speedtest_servers" }

type wsSpeedtestQueue struct {
	ID uint `gorm:"primaryKey"`
}

func (wsSpeedtestQueue) TableName() string { return "speedtest_queue" }

type wsAgentPin struct {
	ID uint `gorm:"primaryKey"`
}

func (wsAgentPin) TableName() string { return "agent_pins" }

// --- Workspace API Keys ---

type WorkspaceAPIKey struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	WorkspaceID uint           `gorm:"not null;index" json:"workspace_id"`
	Name        string         `gorm:"size:100;not null" json:"name"`
	KeyHash     string         `gorm:"size:64;not null" json:"-"`         // SHA256 hash of the key
	KeyPrefix   string         `gorm:"size:8;not null" json:"key_prefix"` // First 8 chars for display
	CreatedAt   time.Time      `json:"created_at"`
	LastUsedAt  *time.Time     `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time     `gorm:"index" json:"expires_at,omitempty"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (WorkspaceAPIKey) TableName() string { return "workspace_api_keys" }

func (s *Store) AutoMigrateAPIKeys(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(&WorkspaceAPIKey{})
}

type CreateAPIKeyInput struct {
	WorkspaceID uint       `json:"workspace_id"`
	Name        string     `json:"name"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

func (s *Store) CreateAPIKey(ctx context.Context, in CreateAPIKeyInput) (*WorkspaceAPIKey, string, error) {
	if in.WorkspaceID == 0 || strings.TrimSpace(in.Name) == "" {
		return nil, "", ErrInvalidInput
	}

	rawKey, err := generateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("generate key: %w", err)
	}

	h := sha256.Sum256([]byte(rawKey))
	keyHash := fmt.Sprintf("%x", h)
	keyPrefix := rawKey[:8]

	ak := &WorkspaceAPIKey{
		WorkspaceID: in.WorkspaceID,
		Name:        strings.TrimSpace(in.Name),
		KeyHash:     keyHash,
		KeyPrefix:   keyPrefix,
		ExpiresAt:   in.ExpiresAt,
	}

	if err := s.db.WithContext(ctx).Create(ak).Error; err != nil {
		return nil, "", err
	}

	return ak, rawKey, nil
}

func (s *Store) ValidateAPIKey(ctx context.Context, workspaceID uint, rawKey string) (*WorkspaceAPIKey, error) {
	if workspaceID == 0 || rawKey == "" {
		return nil, ErrInvalidInput
	}

	h := sha256.Sum256([]byte(rawKey))
	keyHash := fmt.Sprintf("%x", h)

	var ak WorkspaceAPIKey
	err := s.db.WithContext(ctx).
		Where("workspace_id = ? AND key_hash = ? AND deleted_at IS NULL", workspaceID, keyHash).
		First(&ak).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if ak.ExpiresAt != nil && ak.ExpiresAt.Before(time.Now()) {
		return nil, nil
	}

	s.db.WithContext(ctx).Model(&ak).Update("last_used_at", time.Now())

	return &ak, nil
}

func (s *Store) ListAPIKeys(ctx context.Context, workspaceID uint) ([]WorkspaceAPIKey, error) {
	var keys []WorkspaceAPIKey
	err := s.db.WithContext(ctx).
		Where("workspace_id = ? AND deleted_at IS NULL", workspaceID).
		Order("created_at DESC").
		Find(&keys).Error
	return keys, err
}

func (s *Store) DeleteAPIKey(ctx context.Context, id, workspaceID uint) error {
	res := s.db.WithContext(ctx).
		Where("id = ? AND workspace_id = ?", id, workspaceID).
		Delete(&WorkspaceAPIKey{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
