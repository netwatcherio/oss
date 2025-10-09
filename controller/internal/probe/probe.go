package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net"
	"netwatcher-controller/internal/agent"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// -------------------- Types & Constants --------------------

type Type string

const (
	TypeRPerf           Type = "RPERF"
	TypeMTR             Type = "MTR"
	TypePing            Type = "PING"
	TypeNetInfo         Type = "NETINFO"
	TypeSysInfo         Type = "SYSINFO"
	TypeSpeedtest       Type = "SPEEDTEST"
	TypeSpeedtestServer Type = "SPEEDTEST_SERVERS"
	// Inter-agent targeting (e.g., TRAFFICSIM server<->client)
	TypeAgent      Type = "AGENT"
	TypeTrafficSim Type = "TRAFFICSIM"
)

var (
	ErrNotFound     = errors.New("probe not found")
	ErrBadInput     = errors.New("invalid input")
	ErrNoTargets    = errors.New("no targets provided")
	ErrTargetFormat = errors.New("invalid target format")
)

// -------------------- Models --------------------

// Probe is owned by an agent, scoped to a workspace.
type Probe struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"index" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	WorkspaceID uint           `gorm:"index" json:"workspace_id"`
	AgentID     uint           `gorm:"index" json:"agent_id"`
	Type        Type           `gorm:"type:VARCHAR(64);index" json:"type"`
	Enabled     bool           `gorm:"default:true;index" json:"enabled"`
	IntervalSec int            `gorm:"default:60" json:"interval_sec"`
	TimeoutSec  int            `gorm:"default:10" json:"timeout_sec"`
	Count       int            `json:"count"`
	DurationSec int            `json:"duration_sec"`
	Server      bool           `json:"server"`
	Labels      datatypes.JSON `gorm:"type:jsonb" json:"labels"`
	Metadata    datatypes.JSON `gorm:"type:jsonb" json:"metadata"`

	Targets []Target `json:"targets"` // eager-loaded as needed
}

func (Probe) TableName() string { return "probes" }

// Target can be a literal host[:port], or reference another agent via AgentID.
type Target struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	ProbeID uint `gorm:"index;not null" json:"probe_id"`

	// If AgentID is non-nil, it's an inter-agent target (the controller will resolve IP/port).
	Target  string `gorm:"size:512" json:"target"` // ip/host[:port] (leave empty when AgentID is set)
	AgentID *uint  `gorm:"index" json:"agent_id"`  // target agent
	GroupID *uint  `gorm:"index" json:"group_id"`  // optional grouping/batching
}

func (Target) TableName() string { return "probe_targets" }

// -------------------- DTOs --------------------

type CreateInput struct {
	WorkspaceID  uint           `gorm:"index" json:"workspace_id"`
	AgentID      uint           `gorm:"index" json:"agent_id"`
	Type         Type           `gorm:"type:VARCHAR(64);index" json:"type"`
	Enabled      bool           `gorm:"default:true;index" json:"enabled,omitempty"`
	IntervalSec  int            `gorm:"default:60" json:"interval_sec,omitempty"`
	TimeoutSec   int            `gorm:"default:10" json:"timeout_sec,omitempty"`
	Count        int            `json:"count,omitempty"`
	DurationSec  int            `json:"duration_sec,omitempty"`
	Server       bool           `json:"server,omitempty"`
	Targets      []string       `json:"targets,omitempty"`
	AgentTargets []uint         `json:"agent_targets,omitempty"`
	Labels       datatypes.JSON `gorm:"type:jsonb" json:"labels,omitempty"`
	Metadata     datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
}

type UpdateInput struct {
	ID          uint
	Enabled     *bool
	IntervalSec *int
	TimeoutSec  *int
	Labels      *datatypes.JSON
	Metadata    *datatypes.JSON

	// Optional full replacement of targets in one shot
	ReplaceTargets      []string
	ReplaceAgentTargets []uint
}

// -------------------- Helpers --------------------

func coalesceJSON(j datatypes.JSON) datatypes.JSON {
	if len(j) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return j
}

func boolOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// very light target validation; accept "host", "host:port", "ip", "ip:port"
func validateLiteralTarget(s string) bool {
	if s == "" {
		return false
	}
	// If it contains a colon, try to parse port
	h, p, err := net.SplitHostPort(s)
	if err == nil {
		if h == "" {
			return false
		}
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
		return true
	}
	// No port: allow host or IP
	return true
}

// -------------------- Public API (No repo/service layers) --------------------

// Create creates a probe and its targets in a single transaction.
func Create(ctx context.Context, db *gorm.DB, in CreateInput) (*Probe, error) {
	if in.WorkspaceID == 0 || in.AgentID == 0 || in.Type == "" {
		return nil, fmt.Errorf("%w: workspaceId/agentId/type required", ErrBadInput)
	}
	if len(in.Targets) == 0 && len(in.AgentTargets) == 0 {
		return nil, ErrNoTargets
	}

	now := time.Now()
	p := &Probe{
		WorkspaceID: in.WorkspaceID,
		AgentID:     in.AgentID,
		Type:        in.Type,
		Enabled:     boolOr(&in.Enabled, true),
		IntervalSec: ifZero(in.IntervalSec, 60),
		TimeoutSec:  ifZero(in.TimeoutSec, 10),
		Labels:      coalesceJSON(in.Labels),
		Metadata:    coalesceJSON(in.Metadata),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create parent
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		// Build targets
		var rows []Target
		for _, t := range in.Targets {
			if !validateLiteralTarget(t) {
				return fmt.Errorf("%w: %q", ErrTargetFormat, t)
			}
			rows = append(rows, Target{
				ProbeID:   p.ID,
				Target:    t,
				AgentID:   nil,
				CreatedAt: now,
				UpdatedAt: now,
			})
		}
		for _, agentID := range in.AgentTargets {
			aid := agentID // capture
			rows = append(rows, Target{
				ProbeID:   p.ID,
				Target:    "", // left blank; resolved later
				AgentID:   &aid,
				CreatedAt: now,
				UpdatedAt: now,
			})
		}
		if len(rows) == 0 {
			return ErrNoTargets
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return nil, err
	}

	// Eager load targets
	var out Probe
	if err := db.WithContext(ctx).Preload("Targets").First(&out, p.ID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func GetByID(ctx context.Context, db *gorm.DB, id uint) (*Probe, error) {
	var p Probe
	err := db.WithContext(ctx).Preload("Targets").First(&p, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &p, err
}

func ListByAgent(ctx context.Context, db *gorm.DB, agentID uint) ([]Probe, error) {
	var out []Probe
	err := db.WithContext(ctx).
		Preload("Targets").
		Where("agent_id = ?", agentID).
		Order("id DESC").
		Find(&out).Error

	return out, err
}

func ListForAgent(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint) ([]Probe, error) {
	var out []Probe

	err := db.WithContext(ctx).
		Preload("Targets").
		Where("agent_id = ?", agentID).
		Order("id DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}

	// cache public IP lookups per agent to avoid repeat DB hits
	pubIPCache := make(map[uint]string)

	for i := range out {
		p := &out[i]

		for j := range p.Targets {
			t := &p.Targets[j]

			switch p.Type {
			case TypeAgent:
				// TODO: create multiple probes for agent type for all available target agents (left as-is)
			case TypeMTR, TypePing:
				// fill target when it's empty and we have a target agent
				if t.Target == "" && t.AgentID != nil {
					aid := *t.AgentID

					ip, ok := pubIPCache[aid]
					if !ok {
						var err2 error
						ip, err2 = getPublicIP(ctx, db, ch, aid)
						if err2 != nil {
							log.Error(err2)
							continue
						}
						pubIPCache[aid] = ip
					}

					t.Target = ip
				}
			}
		}
	}

	return out, nil
}

func getPublicIP(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint) (string, error) {
	var publicIP string

	agentByID, err := agent.GetAgentByID(ctx, db, agentID)
	if err != nil {
		return "", err
	}

	if agentByID.PublicIPOverride != "" {
		publicIP = agentByID.PublicIPOverride
	} else {
		netInfoPayload, err := GetLatestNetInfoForAgent(ctx, ch, uint64(agentID), nil)
		if err != nil {
			return "", err
		}

		var netInfo = struct {
			LocalAddress     string    `json:"local_address" bson:"local_address"`
			DefaultGateway   string    `json:"default_gateway" bson:"default_gateway"`
			PublicAddress    string    `json:"public_address" bson:"public_address"`
			InternetProvider string    `json:"internet_provider" bson:"internet_provider"`
			Lat              string    `json:"lat" bson:"lat"`
			Long             string    `json:"long" bson:"long"`
			Timestamp        time.Time `json:"timestamp" bson:"timestamp"`
		}{}

		err = json.Unmarshal(netInfoPayload.Payload, &netInfo)
		if err != nil {
			return "", err
		}

		publicIP = netInfo.PublicAddress
	}
	return publicIP, nil
}

// ListByAgentWithReverse returns probes owned by agentID,
// and also “reverse” probes owned by others that target agentID via Target.AgentID.
func ListByAgentWithReverse(ctx context.Context, db *gorm.DB, agentID uint) (owned []Probe, reverse []Probe, err error) {
	ownedErr := db.WithContext(ctx).Preload("Targets").Where("agent_id = ?", agentID).Order("id DESC").Find(&owned).Error
	if ownedErr != nil {
		return nil, nil, ownedErr
	}
	// Reverse = someone else’s probe (often TypeAgent/TypeTrafficSim) whose target.agent_id = agentID
	revErr := db.WithContext(ctx).Preload("Targets").
		Joins("JOIN probe_targets t ON t.probe_id = probes.id").
		Where("t.agent_id = ? AND probes.agent_id <> ?", agentID, agentID).
		Find(&reverse).Error
	if revErr != nil {
		return nil, nil, revErr
	}
	return owned, reverse, nil
}

// Update patches fields and (optionally) replaces targets atomically.
func Update(ctx context.Context, db *gorm.DB, in UpdateInput) (*Probe, error) {
	if in.ID == 0 {
		return nil, fmt.Errorf("%w: id required", ErrBadInput)
	}

	now := time.Now()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"updated_at": now}
		if in.Enabled != nil {
			updates["enabled"] = *in.Enabled
		}
		if in.IntervalSec != nil {
			updates["interval_sec"] = *in.IntervalSec
		}
		if in.TimeoutSec != nil {
			updates["timeout_sec"] = *in.TimeoutSec
		}
		if in.Labels != nil {
			updates["labels"] = coalesceJSON(*in.Labels)
		}
		if in.Metadata != nil {
			updates["metadata"] = coalesceJSON(*in.Metadata)
		}

		res := tx.Model(&Probe{}).Where("id = ?", in.ID).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}

		// Replace targets if requested
		if len(in.ReplaceTargets) > 0 || len(in.ReplaceAgentTargets) > 0 {
			if err := tx.Where("probe_id = ?", in.ID).Delete(&Target{}).Error; err != nil {
				return err
			}
			var rows []Target
			for _, t := range in.ReplaceTargets {
				if !validateLiteralTarget(t) {
					return fmt.Errorf("%w: %q", ErrTargetFormat, t)
				}
				rows = append(rows, Target{
					ProbeID:   in.ID,
					Target:    t,
					AgentID:   nil,
					CreatedAt: now,
					UpdatedAt: now,
				})
			}
			for _, aid := range in.ReplaceAgentTargets {
				a := aid
				rows = append(rows, Target{
					ProbeID:   in.ID,
					Target:    "",
					AgentID:   &a,
					CreatedAt: now,
					UpdatedAt: now,
				})
			}
			if len(rows) == 0 {
				return ErrNoTargets
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return GetByID(ctx, db, in.ID)
}

// Delete hard-deletes the probe and its targets.
func Delete(ctx context.Context, db *gorm.DB, id uint) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("probe_id = ?", id).Delete(&Target{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&Probe{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// DeleteByAgent deletes all probes owned by an agent (and their targets).
func DeleteByAgent(ctx context.Context, db *gorm.DB, agentID uint) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uint
		if err := tx.Model(&Probe{}).Where("agent_id = ?", agentID).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Where("probe_id IN ?", ids).Delete(&Target{}).Error; err != nil {
			return err
		}
		return tx.Where("agent_id = ?", agentID).Delete(&Probe{}).Error
	})
}

// ListOfType returns all probes for an agent of a given type.
func ListOfType(ctx context.Context, db *gorm.DB, agentID uint, t Type) ([]Probe, error) {
	var out []Probe
	err := db.WithContext(ctx).
		Preload("Targets").
		Where("agent_id = ? AND type = ?", agentID, t).
		Find(&out).Error
	return out, err
}

// FindTrafficSimClients returns client probes that target the given server agent.
// (i.e., other agents with TypeTrafficSim pointing to serverAgentID via Target.AgentID)
func FindTrafficSimClients(ctx context.Context, db *gorm.DB, serverAgentID uint) ([]Probe, error) {
	var out []Probe
	err := db.WithContext(ctx).Preload("Targets").
		Joins("JOIN probe_targets t ON t.probe_id = probes.id").
		Where("probes.type = ? AND t.agent_id = ?", TypeTrafficSim, serverAgentID).
		Find(&out).Error
	return out, err
}

// -------------------- Small utilities --------------------

func ifZero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// Nice stringer for debugging/logs.
func (p Probe) String() string {
	var ts []string
	for _, t := range p.Targets {
		if t.AgentID != nil {
			ts = append(ts, fmt.Sprintf("agent:%d", *t.AgentID))
		} else {
			ts = append(ts, t.Target)
		}
	}
	return fmt.Sprintf("Probe{id=%d, ws=%d, agent=%d, type=%s, enabled=%t, every=%ds, timeout=%ds, targets=[%s]}",
		p.ID, p.WorkspaceID, p.AgentID, p.Type, p.Enabled, p.IntervalSec, p.TimeoutSec, strings.Join(ts, ","))
}
