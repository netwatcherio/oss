// === agent/default_probes.go (same package: agent) ===
package agent

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Minimal local mirror of your probe tables to avoid circular imports.
type dbProbe struct {
	ID               uint           `gorm:"primaryKey;autoIncrement"`
	CreatedAt        time.Time      `gorm:"index"`
	UpdatedAt        time.Time      `gorm:"index"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	WorkspaceID      uint           `gorm:"index"`
	AgentID          uint           `gorm:"index;not null"`
	Type             string         `gorm:"type:varchar(24);index"`
	Notifications    bool           `gorm:"default:false"`
	DurationSec      int            `gorm:"default:0"`
	Count            int            `gorm:"default:0"`
	IntervalSec      int            `gorm:"default:0"`
	Server           bool           `gorm:"default:false;index"`
	PendingAt        *time.Time
	ReverseOfProbeID *uint          `gorm:"index"`
	OriginalAgentID  *uint          `gorm:"index"`
	Labels           datatypes.JSON `gorm:"type:jsonb"`
	Metadata         datatypes.JSON `gorm:"type:jsonb"`
}

func (dbProbe) TableName() string { return "probes" }

type dbTarget struct {
	ID        uint `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	ProbeID   uint           `gorm:"index;not null"`
	Target    string         `gorm:"size:512"`
	AgentID   *uint          `gorm:"index"`
	GroupID   *uint          `gorm:"index"`
}

func (dbTarget) TableName() string { return "probe_targets" }

func (s *service) createDefaultProbesTx(tx *gorm.DB, a *Agent) error {
	now := time.Now()
	jsonEmpty := datatypes.JSON([]byte(`{}`))

	// NETINFO
	p1 := dbProbe{
		WorkspaceID: a.WorkspaceID, AgentID: a.ID, Type: "NETINFO",
		CreatedAt: now, UpdatedAt: now, Labels: jsonEmpty, Metadata: jsonEmpty,
	}
	// SYSINFO
	p2 := dbProbe{
		WorkspaceID: a.WorkspaceID, AgentID: a.ID, Type: "SYSINFO",
		CreatedAt: now, UpdatedAt: now, Labels: jsonEmpty, Metadata: jsonEmpty,
	}
	// SPEEDTEST_SERVERS
	p3 := dbProbe{
		WorkspaceID: a.WorkspaceID, AgentID: a.ID, Type: "SPEEDTEST_SERVERS",
		CreatedAt: now, UpdatedAt: now, Labels: jsonEmpty, Metadata: jsonEmpty,
	}
	// SPEEDTEST with default target "ok"
	p4 := dbProbe{
		WorkspaceID: a.WorkspaceID, AgentID: a.ID, Type: "SPEEDTEST",
		CreatedAt: now, UpdatedAt: now, Labels: jsonEmpty, Metadata: jsonEmpty,
	}

	// Insert probes
	for _, p := range []*dbProbe{&p1, &p2, &p3, &p4} {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
	}

	// Targets for SPEEDTEST ("ok")
	t := dbTarget{
		ProbeID:   p4.ID,
		Target:    "ok",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&t).Error; err != nil {
		return err
	}

	return nil
}
