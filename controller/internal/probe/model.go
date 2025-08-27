package probe

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ---------- Types ----------

type Type string

const (
	TypeRPerf        Type = "RPERF"
	TypeMTR          Type = "MTR"
	TypePing         Type = "PING"
	TypeSpeedtest    Type = "SPEEDTEST"
	TypeSpeedServers Type = "SPEEDTEST_SERVERS"
	TypeNetInfo      Type = "NETINFO"
	TypeSysInfo      Type = "SYSINFO"
	TypeTrafficSim   Type = "TRAFFICSIM"
	TypeAgent        Type = "AGENT" // meta-probe that expands to concrete reverse/client probes
)

// ---------- Models ----------

// Probe is the persisted job definition delivered to agents.
// ReverseOfProbeID links a generated reverse "virtual" probe back to its source.
type Probe struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"index" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	WorkspaceID uint `gorm:"index" json:"workspaceId"`      // optional/handy for multi-tenant scoping
	AgentID     uint `gorm:"index;not null" json:"agentId"` // owner/issuer agent (source)
	Type        Type `gorm:"type:varchar(24);index" json:"type"`

	// Flags & knobs
	Notifications bool       `gorm:"default:false" json:"notifications"`
	DurationSec   int        `gorm:"default:0" json:"durationSec"`
	Count         int        `gorm:"default:0" json:"count"`
	IntervalSec   int        `gorm:"default:0" json:"intervalSec"`
	Server        bool       `gorm:"default:false;index" json:"server"`
	PendingAt     *time.Time `json:"pendingAt"`

	// Reverse/meta
	ReverseOfProbeID *uint `gorm:"index" json:"reverseOfProbeId"` // if not nil, this record is a reverse-probe of source ID
	OriginalAgentID  *uint `gorm:"index" json:"originalAgentId"`  // for agent->agent probes, keep the originator

	// Free-form extras
	Labels   datatypes.JSON `gorm:"type:jsonb" json:"labels"`
	Metadata datatypes.JSON `gorm:"type:jsonb" json:"metadata"`

	// 1..N probe targets
	Targets []Target `json:"targets"`
}

func (Probe) TableName() string { return "probes" }

// Target normalizes your former ProbeTarget.
// Target may be a host/IP (e.g., "1.2.3.4:5201") OR empty when AgentID is used and will be resolved at dispatch.
type Target struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	ProbeID uint   `gorm:"index;not null" json:"probeId"`
	Target  string `gorm:"size:512" json:"target"` // IP/host[:port]
	AgentID *uint  `gorm:"index" json:"agentId"`   // if this is an agent-target, we'll resolve IP/port
	GroupID *uint  `gorm:"index" json:"groupId"`   // reserved for future grouping
	// You can add an "Order" column if needed:
	// Order int `gorm:"default:0" json:"order"`
}

func (Target) TableName() string { return "probe_targets" }
