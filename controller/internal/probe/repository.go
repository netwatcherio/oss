package probe

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var (
	ErrNotFound = errors.New("probe not found")
)

type Repository interface {
	Create(ctx context.Context, p *Probe) error
	GetByID(ctx context.Context, id uint) (*Probe, error)
	ListByAgent(ctx context.Context, agentID uint) ([]Probe, error)
	ListByAgentAndType(ctx context.Context, agentID uint, t Type) ([]Probe, error)
	DeleteByAgent(ctx context.Context, agentID uint) error
	DeleteByID(ctx context.Context, id uint) error
	Update(ctx context.Context, p *Probe) error

	// helpers
	FindTrafficSimClients(ctx context.Context, serverAgentID uint) ([]Probe, error)
	ListAgentProbesTargetingAgent(ctx context.Context, ownerAgentID uint, targetAgentID uint) ([]Probe, error)
	ListAgentProbesOfType(ctx context.Context, ownerAgentID uint, t Type) ([]Probe, error)
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) Create(ctx context.Context, p *Probe) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *gormRepo) GetByID(ctx context.Context, id uint) (*Probe, error) {
	var p Probe
	err := r.db.WithContext(ctx).Preload("Targets").First(&p, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (r *gormRepo) ListByAgent(ctx context.Context, agentID uint) ([]Probe, error) {
	var out []Probe
	err := r.db.WithContext(ctx).
		Preload("Targets").
		Where("agent_id = ?", agentID).
		Order("id DESC").
		Find(&out).Error
	return out, err
}

func (r *gormRepo) ListByAgentAndType(ctx context.Context, agentID uint, t Type) ([]Probe, error) {
	var out []Probe
	err := r.db.WithContext(ctx).
		Preload("Targets").
		Where("agent_id = ? AND type = ?", agentID, t).
		Order("id DESC").
		Find(&out).Error
	return out, err
}

func (r *gormRepo) DeleteByAgent(ctx context.Context, agentID uint) error {
	// cascade delete targets with ON DELETE CASCADE in FK or do it manually
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("probe_id IN (?)",
			tx.Model(&Probe{}).Select("id").Where("agent_id = ?", agentID),
		).Delete(&Target{}).Error; err != nil {
			return err
		}
		return tx.Where("agent_id = ?", agentID).Delete(&Probe{}).Error
	})
}

func (r *gormRepo) DeleteByID(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("probe_id = ?", id).Delete(&Target{}).Error; err != nil {
			return err
		}
		return tx.Delete(&Probe{}, id).Error
	})
}

func (r *gormRepo) Update(ctx context.Context, p *Probe) error {
	// Save probe, then upsert targets (simplest: replace)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(p).Error; err != nil {
			return err
		}
		if err := tx.Where("probe_id = ?", p.ID).Delete(&Target{}).Error; err != nil {
			return err
		}
		for i := range p.Targets {
			p.Targets[i].ProbeID = p.ID
		}
		return tx.Create(&p.Targets).Error
	})
}

// ---------- domain helpers ----------

// TrafficSim clients: non-server TRAFFICSIM that target this server agent.
func (r *gormRepo) FindTrafficSimClients(ctx context.Context, serverAgentID uint) ([]Probe, error) {
	var out []Probe
	err := r.db.WithContext(ctx).
		Preload("Targets").
		Joins("JOIN probe_targets t ON t.probe_id = probes.id").
		Where("probes.type = ? AND probes.server = FALSE AND t.agent_id = ?", TypeTrafficSim, serverAgentID).
		Find(&out).Error
	return out, err
}

// All AGENT meta-probes owned by ownerAgentID that include targetAgentID in targets.
func (r *gormRepo) ListAgentProbesTargetingAgent(ctx context.Context, ownerAgentID uint, targetAgentID uint) ([]Probe, error) {
	var out []Probe
	err := r.db.WithContext(ctx).
		Preload("Targets").
		Joins("JOIN probe_targets t ON t.probe_id = probes.id").
		Where("probes.type = ? AND probes.agent_id = ? AND t.agent_id = ?", TypeAgent, ownerAgentID, targetAgentID).
		Find(&out).Error
	return out, err
}

// Convenience
func (r *gormRepo) ListAgentProbesOfType(ctx context.Context, ownerAgentID uint, t Type) ([]Probe, error) {
	var out []Probe
	err := r.db.WithContext(ctx).Preload("Targets").
		Where("agent_id = ? AND type = ?", ownerAgentID, t).
		Find(&out).Error
	return out, err
}
