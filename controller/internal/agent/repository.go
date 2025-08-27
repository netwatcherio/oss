package agent

import (
	"context"
	"errors"
	"golang.org/x/crypto/ssh/agent"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("agent not found")

// Repository defines DB operations for Agent.
type Repository interface {
	Create(ctx context.Context, a *Agent) error
	GetByID(ctx context.Context, id uint) (*Agent, error)
	ListByWorkspace(ctx context.Context, workspaceID uint, limit, offset int) ([]Agent, int64, error)
	Update(ctx context.Context, a *Agent) error
	PatchFields(ctx context.Context, id uint, fields map[string]any) error
	UpdateHeartbeat(ctx context.Context, id uint, seenAt time.Time, newStatus Status) error
	RotatePin(ctx context.Context, id uint, newPin string) error
	Delete(ctx context.Context, id uint) error
	HardDelete(ctx context.Context, id uint) error
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, a *Agent) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *gormRepository) GetByID(ctx context.Context, id uint) (*Agent, error) {
	var out Agent
	err := r.db.WithContext(ctx).First(&out, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &out, err
}

func (r *gormRepository) ListByWorkspace(ctx context.Context, workspaceID uint, limit, offset int) ([]Agent, int64, error) {
	q := r.db.WithContext(ctx).Model(&Agent{}).Where("workspace_id = ?", workspaceID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []Agent
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Update replaces the full record (except zero-value semantics for JSON/strings apply).
func (r *gormRepository) Update(ctx context.Context, a *Agent) error {
	return r.db.WithContext(ctx).Save(a).Error
}

// PatchFields updates only provided columns (no zero-value clobber).
// Example: fields := map[string]any{"name":"new", "labels": datatypes.JSON(`{"role":"edge"}`)}
func (r *gormRepository) PatchFields(ctx context.Context, id uint, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	res := r.db.WithContext(ctx).Model(&Agent{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateHeartbeat bumps timestamps without disturbing other fields.
func (r *gormRepository) UpdateHeartbeat(ctx context.Context, id uint, seenAt time.Time, newStatus Status) error {
	updates := map[string]any{
		"last_seen_at": seenAt,
		"updated_at":   time.Now(),
	}
	if newStatus != "" {
		updates["status"] = newStatus
	}
	return r.PatchFields(ctx, id, updates)
}

// RotatePin replaces the PIN securely (caller should hash if needed before calling).
func (r *gormRepository) RotatePin(ctx context.Context, id uint, newPin string) error {
	return r.PatchFields(ctx, id, map[string]any{"pin": newPin})
}

func (r *gormRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&Agent{}, id) // soft delete
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) ListByWorkspaceSimple(ctx context.Context, wsID uint) ([]agent.Agent, error) {
	var out []agent.Agent
	err := r.db.WithContext(ctx).Where("workspace_id = ?", wsID).Find(&out).Error
	return out, err
}

func (r *gormRepository) HardDelete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Unscoped().Delete(&Agent{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
