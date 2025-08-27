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
// Notes:
// - WorkspaceID is included for future join with workspaces/members.
// - LastSeenAt + HeartbeatIntervalSec supports SLA/expiry calculations.
// - Platform/Arch/Hostname/MAC/IPs help with inventory & targeting.
// - Labels is a small, queryable JSON map (e.g., {"role":"edge","pop":"SEA"}).
// - Metadata is free-form JSON payload from the agent (versions, caps, etc.).
type Agent struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"column:created_at;index" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"column:updated_at;index" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // soft delete

	// Ownership / scoping
	WorkspaceID uint `gorm:"column:workspace_id;index" json:"workspaceId"`
	SiteID      uint `gorm:"column:site_id;index" json:"siteId"`

	// Identity
	Name        string `gorm:"column:name;size:255;index" json:"name" form:"name"`
	Hostname    string `gorm:"column:hostname;size:255;index" json:"hostname"`
	Initialized bool   `gorm:"column:initialized;default:false;index" json:"initialized"`

	// Auth
	Pin       string `gorm:"column:pin;size:255" json:"pin"`
	PublicKey string `gorm:"column:public_key;size:4096" json:"publicKey"`

	// Network
	Location         string `gorm:"column:location;size:255" json:"location"`
	PublicIPOverride string `gorm:"column:public_ip_override;size:64" json:"public_ip_override"`
	DetectedPublicIP string `gorm:"column:detected_public_ip;size:64" json:"detectedPublicIp"`
	PrivateIP        string `gorm:"column:private_ip;size:64" json:"privateIp"`
	MACAddress       string `gorm:"column:mac_address;size:64;index" json:"macAddress"`

	// Runtime / versioning
	Version  string `gorm:"column:version;size:64;index" json:"version"`
	Platform string `gorm:"column:platform;size:64;index" json:"platform"` // linux, darwin, windows
	Arch     string `gorm:"column:arch;size:32;index" json:"arch"`         // amd64, arm64, etc.

	// Health / status
	Status               Status    `gorm:"column:status;type:varchar(16);index" json:"status"`
	LastSeenAt           time.Time `gorm:"column:last_seen_at;index" json:"lastSeenAt"`
	HeartbeatIntervalSec int       `gorm:"column:heartbeat_interval_sec;default:60" json:"heartbeatIntervalSec"`

	// Tags / labels
	Labels   datatypes.JSON `gorm:"column:labels;type:jsonb" json:"labels"`     // small, indexed map for filters
	Metadata datatypes.JSON `gorm:"column:metadata;type:jsonb" json:"metadata"` // free-form agent payload
}

// TableName (optional) if you want a custom table name
func (Agent) TableName() string { return "agents" }

// BeforeCreate hook example: ensure sane defaults
func (a *Agent) BeforeCreate(tx *gorm.DB) error {
	if a.Status == "" {
		a.Status = StatusUnknown
	}
	if a.HeartbeatIntervalSec <= 0 {
		a.HeartbeatIntervalSec = 60
	}
	return nil
}
