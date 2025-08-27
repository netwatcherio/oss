package workspace

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Role governs member permissions within a workspace.
type Role string

const (
	RoleReadOnly  Role = "READ_ONLY"  // view only
	RoleReadWrite Role = "READ_WRITE" // create/update non-destructive
	RoleAdmin     Role = "ADMIN"      // can delete agents/members (not owner)
	RoleOwner     Role = "OWNER"      // super admin (exactly one per workspace)
)

// Workspace represents a tenant/organization for agents and other resources.
type Workspace struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"index" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Name        string `gorm:"size:255;uniqueIndex:ux_workspace_name" json:"name"`
	Slug        string `gorm:"size:255;uniqueIndex:ux_workspace_slug" json:"slug"` // optional URL/key
	Description string `gorm:"type:text" json:"description"`
	Location    string `gorm:"size:255" json:"location"`

	OwnerUserID uint `gorm:"index" json:"ownerUserId"` // your "users" table PK (uint). If you don't have users yet, keep 0.

	Labels   datatypes.JSON `gorm:"type:jsonb" json:"labels"`   // small searchable tags
	Metadata datatypes.JSON `gorm:"type:jsonb" json:"metadata"` // flexible extras

	// Associations (optional preloading)
	Members []WorkspaceMember `json:"members"`
}

// TableName is optional.
func (Workspace) TableName() string { return "workspaces" }

// WorkspaceMember maps users to workspaces with a role.
// If you don't yet have a Users table, you can use Email-only invites where UserID=0 until accepted.
type WorkspaceMember struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"index" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	WorkspaceID uint   `gorm:"index;not null" json:"workspaceId"`
	UserID      uint   `gorm:"index" json:"userId"` // 0 = not yet linked (invite by email)
	Email       string `gorm:"size:320;index" json:"email"`

	Role Role `gorm:"type:varchar(16);index" json:"role"`

	// Invitation lifecycle (optional)
	InvitedAt  *time.Time `json:"invitedAt"`
	AcceptedAt *time.Time `json:"acceptedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`

	// Denormalized helpers
	DisplayName string `gorm:"size:255" json:"displayName"`
}

// Uniqueness rules:
//
// - A given (workspace_id, user_id) must be unique once UserID is non-zero.
// - If UserID is zero (invite only), enforce uniqueness on (workspace_id, email).
func (WorkspaceMember) TableName() string { return "workspace_members" }
