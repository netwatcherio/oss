package agent

import (
	"context"
	"errors"
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
	RotatePinHash(ctx context.Context, id uint, newHash string, newIndex string) error
	Delete(ctx context.Context, id uint) error
	HardDelete(ctx context.Context, id uint) error

	// Key-based bootstrap & auth helpers
	GetByWorkspaceAndID(ctx context.Context, wsID, id uint) (*Agent, error)
	GetUnclaimedByPinIndex(ctx context.Context, pinIndex string) (*Agent, error)
	MarkPinConsumedAndStoreKey(ctx context.Context, id uint, pub []byte, fp string) error

	// Nonces
	CreateNonce(ctx context.Context, agentID uint, nonce string, expiresAt time.Time) error
	UseNonce(ctx context.Context, nonce string) (agentID uint, err error)
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

func (r *gormRepository) Update(ctx context.Context, a *Agent) error {
	return r.db.WithContext(ctx).Save(a).Error
}

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

// RotatePinHash updates the PIN hash and (re)sets the PinIndex while clearing consumed flag.
func (r *gormRepository) RotatePinHash(ctx context.Context, id uint, newHash string, newIndex string) error {
	return r.PatchFields(ctx, id, map[string]any{
		"pin_hash":        newHash,
		"pin_index":       newIndex,
		"pin_consumed_at": nil, // allow bootstrap again
	})
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

// ---- key-based bootstrap helpers ----

func (r *gormRepository) GetByWorkspaceAndID(ctx context.Context, wsID, id uint) (*Agent, error) {
	var a Agent
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND id = ?", wsID, id).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &a, err
}

// GetUnclaimedByPinIndex finds any *active* (unconsumed) agent with the same pin index.
func (r *gormRepository) GetUnclaimedByPinIndex(ctx context.Context, pinIndex string) (*Agent, error) {
	var a Agent
	err := r.db.WithContext(ctx).
		Where("pin_index = ? AND pin_consumed_at IS NULL", pinIndex).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &a, err
}

func (r *gormRepository) MarkPinConsumedAndStoreKey(ctx context.Context, id uint, pub []byte, fp string) error {
	now := time.Now()
	// Also NULL-out pin_index so the value can be reused in the far future without colliding.
	res := r.db.WithContext(ctx).Model(&Agent{}).
		Where("id = ? AND public_key IS NULL AND pin_consumed_at IS NULL", id).
		Updates(map[string]any{
			"public_key":      pub,
			"public_key_fp":   fp,
			"pin_consumed_at": now,
			"pin_index":       gorm.Expr("NULL"),
			"initialized":     true,
			"updated_at":      now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Nonce ops ----

func (r *gormRepository) CreateNonce(ctx context.Context, agentID uint, nonce string, expiresAt time.Time) error {
	an := &AgentNonce{AgentID: agentID, Nonce: nonce, ExpiresAt: expiresAt}
	return r.db.WithContext(ctx).Create(an).Error
}

func (r *gormRepository) UseNonce(ctx context.Context, nonce string) (uint, error) {
	var an AgentNonce
	err := r.db.WithContext(ctx).
		Where("nonce = ? AND used_at IS NULL AND expires_at > NOW()", nonce).
		First(&an).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&an).Update("used_at", now).Error; err != nil {
		return 0, err
	}
	return an.AgentID, nil
}
