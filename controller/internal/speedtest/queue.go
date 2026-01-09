package speedtest

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// -------------------- Types & Constants --------------------

type QueueStatus string

const (
	StatusPending   QueueStatus = "pending"
	StatusRunning   QueueStatus = "running"
	StatusCompleted QueueStatus = "completed"
	StatusFailed    QueueStatus = "failed"
	StatusCancelled QueueStatus = "cancelled"
	StatusExpired   QueueStatus = "expired"
)

// DefaultExpirationDuration is how long a queue item remains valid before expiring.
const DefaultExpirationDuration = 15 * time.Minute

var (
	ErrQueueNotFound = errors.New("queue item not found")
	ErrQueueBadInput = errors.New("invalid input")
)

// -------------------- Models --------------------

// QueueItem represents a pending or completed speedtest request.
type QueueItem struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"index" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	WorkspaceID uint        `gorm:"index;not null" json:"workspace_id"`
	AgentID     uint        `gorm:"index;not null" json:"agent_id"`
	ServerID    string      `gorm:"size:64" json:"server_id"`    // speedtest.net server ID (empty = auto)
	ServerName  string      `gorm:"size:256" json:"server_name"` // display name
	Status      QueueStatus `gorm:"type:VARCHAR(32);index;default:'pending'" json:"status"`

	RequestedBy  uint       `json:"requested_by"` // user ID
	RequestedAt  time.Time  `gorm:"index" json:"requested_at"`
	ExpiresAt    time.Time  `gorm:"index" json:"expires_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ErrorMessage string     `gorm:"type:text" json:"error,omitempty"`
}

func (QueueItem) TableName() string { return "speedtest_queue" }

// -------------------- DTOs --------------------

type CreateQueueInput struct {
	WorkspaceID uint   `json:"workspace_id"`
	AgentID     uint   `json:"agent_id"`
	ServerID    string `json:"server_id"`
	ServerName  string `json:"server_name"`
	RequestedBy uint   `json:"requested_by"`
}

// -------------------- CRUD Operations --------------------

// CreateQueueItem adds a new speedtest to the queue.
func CreateQueueItem(ctx context.Context, db *gorm.DB, in CreateQueueInput) (*QueueItem, error) {
	if in.WorkspaceID == 0 || in.AgentID == 0 {
		return nil, ErrQueueBadInput
	}

	now := time.Now()
	item := &QueueItem{
		WorkspaceID: in.WorkspaceID,
		AgentID:     in.AgentID,
		ServerID:    in.ServerID,
		ServerName:  in.ServerName,
		Status:      StatusPending,
		RequestedBy: in.RequestedBy,
		RequestedAt: now,
		ExpiresAt:   now.Add(DefaultExpirationDuration),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

// GetQueueItem retrieves a single queue item by ID.
func GetQueueItem(ctx context.Context, db *gorm.DB, id uint) (*QueueItem, error) {
	var item QueueItem
	err := db.WithContext(ctx).First(&item, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrQueueNotFound
	}
	return &item, err
}

// ListPendingForAgent returns pending, non-expired queue items for a specific agent.
func ListPendingForAgent(ctx context.Context, db *gorm.DB, agentID uint) ([]QueueItem, error) {
	var items []QueueItem
	err := db.WithContext(ctx).
		Where("agent_id = ? AND status = ? AND expires_at > ?", agentID, StatusPending, time.Now()).
		Order("requested_at ASC").
		Find(&items).Error
	return items, err
}

// ExpirePendingItems marks all pending items past their expiration as expired.
// Returns the number of items expired.
func ExpirePendingItems(ctx context.Context, db *gorm.DB) (int64, error) {
	now := time.Now()
	res := db.WithContext(ctx).Model(&QueueItem{}).
		Where("status = ? AND expires_at <= ?", StatusPending, now).
		Updates(map[string]any{
			"status":     StatusExpired,
			"updated_at": now,
		})
	return res.RowsAffected, res.Error
}

// ListForAgent returns all queue items for an agent (optionally filtered by status).
func ListForAgent(ctx context.Context, db *gorm.DB, agentID uint, status *QueueStatus) ([]QueueItem, error) {
	var items []QueueItem
	q := db.WithContext(ctx).Where("agent_id = ?", agentID)
	if status != nil {
		q = q.Where("status = ?", *status)
	}
	err := q.Order("requested_at DESC").Limit(50).Find(&items).Error
	return items, err
}

// MarkRunning marks a queue item as running.
func MarkRunning(ctx context.Context, db *gorm.DB, id uint) error {
	now := time.Now()
	res := db.WithContext(ctx).Model(&QueueItem{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(map[string]any{
			"status":     StatusRunning,
			"started_at": now,
			"updated_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrQueueNotFound
	}
	return nil
}

// MarkCompleted marks a queue item as completed.
func MarkCompleted(ctx context.Context, db *gorm.DB, id uint) error {
	now := time.Now()
	res := db.WithContext(ctx).Model(&QueueItem{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       StatusCompleted,
			"completed_at": now,
			"updated_at":   now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrQueueNotFound
	}
	return nil
}

// MarkFailed marks a queue item as failed with an error message.
func MarkFailed(ctx context.Context, db *gorm.DB, id uint, errMsg string) error {
	now := time.Now()
	res := db.WithContext(ctx).Model(&QueueItem{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       StatusFailed,
			"completed_at": now,
			"error":        errMsg,
			"updated_at":   now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrQueueNotFound
	}
	return nil
}

// CancelQueueItem cancels a pending queue item.
func CancelQueueItem(ctx context.Context, db *gorm.DB, id uint) error {
	now := time.Now()
	res := db.WithContext(ctx).Model(&QueueItem{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(map[string]any{
			"status":     StatusCancelled,
			"updated_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrQueueNotFound
	}
	return nil
}

// DeleteQueueItem hard-deletes a queue item.
func DeleteQueueItem(ctx context.Context, db *gorm.DB, id uint) error {
	res := db.WithContext(ctx).Delete(&QueueItem{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrQueueNotFound
	}
	return nil
}
