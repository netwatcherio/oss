package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"netwatcher-controller/internal/agent"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

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
	ErrDuplicate    = errors.New("duplicate probe already exists")
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
	WorkspaceID   uint           `gorm:"index" json:"workspace_id"`
	AgentID       uint           `gorm:"index" json:"agent_id"`
	Type          Type           `gorm:"type:VARCHAR(64);index" json:"type"`
	Enabled       bool           `gorm:"default:true;index" json:"enabled,omitempty"`
	IntervalSec   int            `gorm:"default:60" json:"interval_sec,omitempty"`
	TimeoutSec    int            `gorm:"default:10" json:"timeout_sec,omitempty"`
	Count         int            `json:"count,omitempty"`
	DurationSec   int            `json:"duration_sec,omitempty"`
	Server        bool           `json:"server,omitempty"`
	Targets       []string       `json:"targets,omitempty"`
	AgentTargets  []uint         `json:"agent_targets,omitempty"`
	Labels        datatypes.JSON `gorm:"type:jsonb" json:"labels,omitempty"`
	Metadata      datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	Bidirectional bool           `json:"bidirectional,omitempty"` // Create matching probe on target agent(s)
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

// checkDuplicateProbe checks if a probe with the same agent, type, and targets already exists.
// Returns ErrDuplicate if a matching probe is found.
func checkDuplicateProbe(ctx context.Context, db *gorm.DB, in CreateInput) error {
	// Find existing probes with the same agent and type
	var existing []Probe
	err := db.WithContext(ctx).
		Preload("Targets").
		Where("agent_id = ? AND type = ?", in.AgentID, in.Type).
		Find(&existing).Error
	if err != nil {
		return err
	}

	// Build sets of incoming targets for comparison
	incomingLiteralTargets := make(map[string]bool)
	for _, t := range in.Targets {
		incomingLiteralTargets[t] = true
	}
	incomingAgentTargets := make(map[uint]bool)
	for _, aid := range in.AgentTargets {
		incomingAgentTargets[aid] = true
	}

	// Check each existing probe for matching targets
	for _, p := range existing {
		// Build sets of existing targets
		existingLiteralTargets := make(map[string]bool)
		existingAgentTargets := make(map[uint]bool)
		for _, t := range p.Targets {
			if t.AgentID != nil {
				existingAgentTargets[*t.AgentID] = true
			} else if t.Target != "" {
				existingLiteralTargets[t.Target] = true
			}
		}

		// Check if there's any overlap in targets
		hasLiteralOverlap := false
		for t := range incomingLiteralTargets {
			if existingLiteralTargets[t] {
				hasLiteralOverlap = true
				break
			}
		}

		hasAgentOverlap := false
		for aid := range incomingAgentTargets {
			if existingAgentTargets[aid] {
				hasAgentOverlap = true
				break
			}
		}

		// If any target overlaps, it's a duplicate
		if hasLiteralOverlap || hasAgentOverlap {
			log.Warnf("Duplicate probe detected: existing probe %d (agent=%d, type=%s) has overlapping targets",
				p.ID, p.AgentID, p.Type)
			return fmt.Errorf("%w: probe with same type and target already exists on agent %d (probe ID: %d)",
				ErrDuplicate, in.AgentID, p.ID)
		}
	}

	return nil
}

// -------------------- Public API (No repo/service layers) --------------------

// Create creates a probe and its targets in a single transaction.
// When Bidirectional is true and AgentTargets are specified, it also creates
// matching reverse probes on each target agent pointing back to the source.
func Create(ctx context.Context, db *gorm.DB, in CreateInput) (*Probe, error) {
	if in.WorkspaceID == 0 || in.AgentID == 0 || in.Type == "" {
		return nil, fmt.Errorf("%w: workspaceId/agentId/type required", ErrBadInput)
	}
	if len(in.Targets) == 0 && len(in.AgentTargets) == 0 {
		return nil, ErrNoTargets
	}

	// Check for duplicate probe (same agent, type, and targets)
	if err := checkDuplicateProbe(ctx, db, in); err != nil {
		return nil, err
	}

	now := time.Now()
	p := &Probe{
		WorkspaceID: in.WorkspaceID,
		AgentID:     in.AgentID,
		Type:        in.Type,
		Enabled:     boolOr(&in.Enabled, true),
		IntervalSec: ifZero(in.IntervalSec, 60),
		TimeoutSec:  ifZero(in.TimeoutSec, 10),
		Server:      in.Server, // TRAFFICSIM server mode
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
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}

		// Create reverse probes if bidirectional and there are agent targets
		if in.Bidirectional && len(in.AgentTargets) > 0 {
			for _, targetAgentID := range in.AgentTargets {
				// Create reverse probe owned by target agent, pointing to source
				reverseProbe := &Probe{
					WorkspaceID: in.WorkspaceID,
					AgentID:     targetAgentID, // Owned by target
					Type:        in.Type,
					Enabled:     boolOr(&in.Enabled, true),
					IntervalSec: ifZero(in.IntervalSec, 60),
					TimeoutSec:  ifZero(in.TimeoutSec, 10),
					Server:      in.Server,
					Labels:      coalesceJSON(in.Labels),
					Metadata:    coalesceJSON(in.Metadata),
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				if err := tx.Create(reverseProbe).Error; err != nil {
					return fmt.Errorf("failed to create reverse probe: %w", err)
				}
				// Add target pointing back to source agent
				sourceID := in.AgentID
				reverseTarget := Target{
					ProbeID:   reverseProbe.ID,
					Target:    "",
					AgentID:   &sourceID, // Points back to source
					CreatedAt: now,
					UpdatedAt: now,
				}
				if err := tx.Create(&reverseTarget).Error; err != nil {
					return fmt.Errorf("failed to create reverse target: %w", err)
				}
				log.Infof("[BIDIR] Created bidirectional pair: Primary probe %d (agent %d -> agent %d), Reverse probe %d (agent %d -> agent %d)",
					p.ID, in.AgentID, targetAgentID,
					reverseProbe.ID, targetAgentID, in.AgentID)
			}
		}

		return nil
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

	// 1. Get probes owned by this agent
	var allProbes []Probe
	err := db.WithContext(ctx).
		Preload("Targets").
		Where("agent_id = ?", agentID).
		Order("id DESC").
		Find(&allProbes).Error
	if err != nil {
		return nil, err
	}

	// cache public IP lookups per agent to avoid repeat DB hits
	pubIPCache := make(map[uint]string)

	// 2. Process owned probes - expand AGENT probes, resolve IPs for others
	for i := range allProbes {
		p := &allProbes[i]

		switch p.Type {
		case TypeAgent:
			// Expand this agent's own AGENT probe into MTR, PING, TRAFFICSIM
			// Target is the agent specified in the probe's targets
			for _, t := range p.Targets {
				if t.AgentID != nil {
					targetAgentID := *t.AgentID
					expanded, err := expandAgentProbeForOwner(ctx, db, ch, p, targetAgentID, pubIPCache)
					if err != nil {
						log.Warnf("ListForAgent: error expanding owned AGENT probe %d: %v", p.ID, err)
						continue
					}
					out = append(out, expanded...)
				}
			}
			// Don't add the raw AGENT probe to output

		default:
			// Resolve IP targets for non-AGENT probes
			for j := range p.Targets {
				t := &p.Targets[j]
				if (p.Type == TypeMTR || p.Type == TypePing) && t.Target == "" && t.AgentID != nil {
					aid := *t.AgentID
					ip, ok := pubIPCache[aid]
					if !ok {
						ip, err = getPublicIP(ctx, db, ch, aid)
						if err != nil {
							log.Error(err)
							continue
						}
						pubIPCache[aid] = ip
					}
					t.Target = ip
				}
			}

			out = append(out, *p)
		}
	}

	// Debug: log what probes are being sent to this agent
	log.Infof("[BIDIR-DEBUG] ListForAgent: agent %d receiving %d probes", agentID, len(out))
	for _, p := range out {
		targetStr := ""
		if len(p.Targets) > 0 {
			targetStr = p.Targets[0].Target
		}
		log.Infof("[BIDIR-DEBUG]   -> Probe ID=%d Type=%s Target=%s", p.ID, p.Type, targetStr)
	}

	return out, nil
}

// findReverseAgentProbes finds AGENT-type probes from other agents that target this agent.
func findReverseAgentProbes(ctx context.Context, db *gorm.DB, targetAgentID uint) ([]Probe, error) {
	var probes []Probe
	err := db.WithContext(ctx).
		Preload("Targets").
		Joins("JOIN probe_targets t ON t.probe_id = probes.id").
		Where("probes.type = ? AND t.agent_id = ? AND probes.agent_id <> ?",
			TypeAgent, targetAgentID, targetAgentID).
		Find(&probes).Error
	return probes, err
}

// expandAgentProbeForOwner expands an AGENT probe for the owning agent.
// The target agent's public IP is resolved and used as the destination.
func expandAgentProbeForOwner(ctx context.Context, db *gorm.DB, ch *sql.DB,
	agentProbe *Probe, targetAgentID uint, pubIPCache map[uint]string) ([]Probe, error) {

	// Get target agent's public IP
	targetIP, ok := pubIPCache[targetAgentID]
	if !ok {
		var err error
		targetIP, err = getPublicIP(ctx, db, ch, targetAgentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get target agent %d public IP: %w", targetAgentID, err)
		}
		pubIPCache[targetAgentID] = targetIP
	}

	if targetIP == "" {
		return nil, fmt.Errorf("target agent %d has no public IP", targetAgentID)
	}

	var expanded []Probe

	// Create MTR probe targeting the target agent
	expanded = append(expanded, createExpandedProbe(agentProbe, TypeMTR, targetIP, targetAgentID))

	// Create PING probe targeting the target agent
	expanded = append(expanded, createExpandedProbe(agentProbe, TypePing, targetIP, targetAgentID))

	// Create TRAFFICSIM probe only if target agent has a server
	if hasTrafficSimServer(ctx, db, targetAgentID) {
		tsProbe := createExpandedProbe(agentProbe, TypeTrafficSim, targetIP, targetAgentID)
		// Get the server port from the target's TrafficSim server config
		port := getTrafficSimServerPort(ctx, db, targetAgentID)
		if port != "" {
			tsProbe.Targets[0].Target = targetIP + ":" + port
		}
		expanded = append(expanded, tsProbe)
	}

	log.Infof("[BIDIR-DEBUG] expandAgentProbeForOwner: AGENT probe %d (owned by agent %d) expanded into %d probes (MTR/PING/TS) targeting agent %d @ %s",
		agentProbe.ID, agentProbe.AgentID, len(expanded), targetAgentID, targetIP)
	for _, ep := range expanded {
		log.Infof("[BIDIR-DEBUG]   -> Expanded probe: ID=%d Type=%s AgentID=%d Target=%s",
			ep.ID, ep.Type, ep.AgentID, ep.Targets[0].Target)
	}

	return expanded, nil
}

// expandAgentProbe expands an AGENT-type probe into concrete MTR, PING, and optionally TRAFFICSIM probes.
// The source agent's public IP is resolved and used as the target.
func expandAgentProbe(ctx context.Context, db *gorm.DB, ch *sql.DB,
	agentProbe *Probe, targetAgentID uint, pubIPCache map[uint]string) ([]Probe, error) {

	// Get source agent's public IP
	sourceAgentID := agentProbe.AgentID
	sourceIP, ok := pubIPCache[sourceAgentID]
	if !ok {
		var err error
		sourceIP, err = getPublicIP(ctx, db, ch, sourceAgentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get source agent %d public IP: %w", sourceAgentID, err)
		}
		pubIPCache[sourceAgentID] = sourceIP
	}

	if sourceIP == "" {
		return nil, fmt.Errorf("source agent %d has no public IP", sourceAgentID)
	}

	var expanded []Probe

	// Create MTR probe
	expanded = append(expanded, createExpandedProbe(agentProbe, TypeMTR, sourceIP, sourceAgentID))

	// Create PING probe
	expanded = append(expanded, createExpandedProbe(agentProbe, TypePing, sourceIP, sourceAgentID))

	// Create TRAFFICSIM probe only if source agent has a server
	if hasTrafficSimServer(ctx, db, sourceAgentID) {
		tsProbe := createExpandedProbe(agentProbe, TypeTrafficSim, sourceIP, sourceAgentID)
		// Get the server port from the source's TrafficSim server config
		port := getTrafficSimServerPort(ctx, db, sourceAgentID)
		if port != "" {
			tsProbe.Targets[0].Target = sourceIP + ":" + port
		}
		expanded = append(expanded, tsProbe)
	}

	return expanded, nil
}

// createExpandedProbe creates a concrete probe from an AGENT probe template.
// targetAgentID is the ID of the agent being targeted by this probe.
func createExpandedProbe(source *Probe, probeType Type, targetIP string, targetAgentID uint) Probe {
	return Probe{
		ID:          source.ID, // Keep original ID for data correlation
		CreatedAt:   source.CreatedAt,
		UpdatedAt:   source.UpdatedAt,
		WorkspaceID: source.WorkspaceID,
		AgentID:     source.AgentID, // Original source agent owns this probe
		Type:        probeType,
		Enabled:     source.Enabled,
		IntervalSec: source.IntervalSec,
		TimeoutSec:  source.TimeoutSec,
		Count:       source.Count,
		DurationSec: source.DurationSec,
		Labels:      source.Labels,
		Metadata:    source.Metadata,
		Targets: []Target{
			{
				ProbeID:   source.ID,
				Target:    targetIP,
				AgentID:   &targetAgentID, // TARGET agent ID - used for bidirectional detection
				CreatedAt: source.CreatedAt,
				UpdatedAt: source.UpdatedAt,
			},
		},
	}
}

// hasTrafficSimServer checks if an agent has a TrafficSim server probe configured.
func hasTrafficSimServer(ctx context.Context, db *gorm.DB, agentID uint) bool {
	var count int64
	db.WithContext(ctx).Model(&Probe{}).
		Where("agent_id = ? AND type = ? AND server = true", agentID, TypeTrafficSim).
		Count(&count)
	return count > 0
}

// getTrafficSimServerPort returns the port from an agent's TrafficSim server probe.
func getTrafficSimServerPort(ctx context.Context, db *gorm.DB, agentID uint) string {
	var probes []Probe
	err := db.WithContext(ctx).
		Preload("Targets").
		Where("agent_id = ? AND type = ? AND server = true", agentID, TypeTrafficSim).
		Limit(1).
		Find(&probes).Error
	if err != nil || len(probes) == 0 || len(probes[0].Targets) == 0 {
		return ""
	}

	// Extract port from target (format: "0.0.0.0:port" or ":port")
	target := probes[0].Targets[0].Target
	if strings.Contains(target, ":") {
		parts := strings.Split(target, ":")
		if len(parts) >= 2 {
			return parts[len(parts)-1]
		}
	}
	return ""
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
		if netInfoPayload == nil || netInfoPayload.Payload == nil {
			return "", fmt.Errorf("no netinfo payload found for agent %d", agentID)
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
