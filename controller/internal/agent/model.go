package agent

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Status reflects a lightweight liveness/health state
type Status string

const (
	StatusUnknown  Status = "UNKNOWN"
	StatusOnline   Status = "ONLINE"
	StatusOffline  Status = "OFFLINE"
	StatusDegraded Status = "DEGRADED"
)

// Agent represents a deployed monitoring agent.
type Agent struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"index" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // soft delete

	// Ownership / scoping
	WorkspaceID uint `gorm:"index:idx_ws_pin,priority:1" json:"workspaceId"`

	// Identity
	Name        string `gorm:"size:255;index" json:"name" form:"name"`
	Hostname    string `gorm:"size:255;index" json:"hostname"`
	Initialized bool   `gorm:"default:false;index" json:"initialized"`

	// Auth (do NOT store plaintext PIN)
	PinHash string `gorm:"size:255;index:idx_ws_pin,priority:2" json:"-"`
	// PinIndex is sha256(pepper + ":" + pin) hex, set only while PIN is active/unconsumed.
	// We keep it UNIQUE to guarantee "never duplicated" among active pins.
	PinIndex      *string    `gorm:"size:64;uniqueIndex" json:"-"`
	PinConsumedAt *time.Time `json:"-"`

	// Ed25519 public key + fingerprint (sha256 hex)
	PublicKey   []byte `gorm:"type:bytea" json:"publicKey,omitempty"`
	PublicKeyFP string `gorm:"size:64;index" json:"publicKeyFingerprint,omitempty"`
	LastAuthAt  *time.Time
	LastAuthIP  string `gorm:"size:64"`

	// Network (kept: override; removed: detected/private/mac/platform/arch)
	Location         string `gorm:"size:255" json:"location"`
	PublicIPOverride string `gorm:"size:64" json:"public_ip_override"`

	// Runtime / versioning (kept minimal)
	Version string `gorm:"size:64;index" json:"version"`

	// Health / status
	Status               Status    `gorm:"type:varchar(16);index" json:"status"`
	LastSeenAt           time.Time `gorm:"index" json:"lastSeenAt"`
	HeartbeatIntervalSec int       `gorm:"default:60" json:"heartbeatIntervalSec"`

	// Tags / labels
	Labels   datatypes.JSON `gorm:"type:jsonb" json:"labels"`
	Metadata datatypes.JSON `gorm:"type:jsonb" json:"metadata"`
}

func (Agent) TableName() string { return "agents" }

func (a *Agent) BeforeCreate(tx *gorm.DB) error {
	if a.Status == "" {
		a.Status = StatusUnknown
	}
	if a.HeartbeatIntervalSec <= 0 {
		a.HeartbeatIntervalSec = 60
	}
	return nil
}
