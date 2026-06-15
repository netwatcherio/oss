package deletion

import (
	"context"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
)

const (
	EntityProbe = "probe"
	EntityAgent = "agent"

	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

var (
	ErrJobNotFound = errors.New("deletion job not found")
	ErrBadEntity   = errors.New("invalid entity type")
	ErrBadID       = errors.New("invalid entity id")
)

type DeletionJob struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	EntityType  string     `gorm:"size:32;not null;index" json:"entity_type"`
	EntityID    uint       `gorm:"not null;index" json:"entity_id"`
	Status      string     `gorm:"size:20;not null;default:'pending';index" json:"status"`
	Attempts    int        `gorm:"default:0" json:"attempts"`
	MaxAttempts int        `gorm:"default:5" json:"max_attempts"`
	LastError   string     `gorm:"type:text" json:"last_error,omitempty"`
	ScheduledAt *time.Time `gorm:"index" json:"scheduled_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (DeletionJob) TableName() string { return "deletion_jobs" }

type QueueStore struct {
	db *gorm.DB
}

func NewQueueStore(db *gorm.DB) *QueueStore {
	return &QueueStore{db: db}
}

func (s *QueueStore) AutoMigrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(&DeletionJob{})
}

func (s *QueueStore) Enqueue(ctx context.Context, entityType string, entityID uint) error {
	if entityType != EntityProbe && entityType != EntityAgent {
		return ErrBadEntity
	}
	if entityID == 0 {
		return ErrBadID
	}

	now := time.Now()
	job := &DeletionJob{
		EntityType:  entityType,
		EntityID:    entityID,
		Status:      StatusPending,
		Attempts:    0,
		MaxAttempts: maxAttempts(),
		ScheduledAt: &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return s.db.WithContext(ctx).Create(job).Error
}

func (s *QueueStore) EnqueueBackfill(ctx context.Context, entityType string, entityID uint) error {
	if entityType != EntityProbe && entityType != EntityAgent {
		return ErrBadEntity
	}
	if entityID == 0 {
		return ErrBadID
	}

	now := time.Now()
	job := &DeletionJob{
		EntityType:  entityType,
		EntityID:    entityID,
		Status:      StatusPending,
		Attempts:    0,
		MaxAttempts: maxAttempts(),
		LastError:   "backfill: enqueued by cleanup scheduler",
		ScheduledAt: &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return s.db.WithContext(ctx).Create(job).Error
}

func (s *QueueStore) ListPending(ctx context.Context, limit int) ([]DeletionJob, error) {
	if limit <= 0 {
		limit = defaultBatchSize
	}
	now := time.Now()
	var jobs []DeletionJob
	err := s.db.WithContext(ctx).
		Where("status = ?", StatusPending).
		Where("attempts < max_attempts").
		Where("scheduled_at IS NULL OR scheduled_at <= ?", now).
		Order("created_at ASC").
		Limit(limit).
		Find(&jobs).Error
	return jobs, err
}

func (s *QueueStore) MarkProcessing(ctx context.Context, id uint) error {
	now := time.Now()
	res := s.db.WithContext(ctx).
		Model(&DeletionJob{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(map[string]any{
			"status":     StatusProcessing,
			"started_at": now,
			"attempts":   gorm.Expr("attempts + 1"),
			"updated_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrJobNotFound
	}
	return nil
}

func (s *QueueStore) MarkCompleted(ctx context.Context, id uint) error {
	now := time.Now()
	res := s.db.WithContext(ctx).
		Model(&DeletionJob{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       StatusCompleted,
			"completed_at": now,
			"last_error":   "",
			"updated_at":   now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrJobNotFound
	}
	return nil
}

func (s *QueueStore) MarkFailed(ctx context.Context, id uint, attempts, maxAttempts int, jobErr error, backoff time.Duration) error {
	now := time.Now()
	updates := map[string]any{
		"last_error": jobErr.Error(),
		"updated_at": now,
	}
	if attempts >= maxAttempts {
		updates["status"] = StatusFailed
		updates["completed_at"] = now
	} else {
		updates["status"] = StatusPending
		scheduled := now.Add(backoff)
		updates["scheduled_at"] = scheduled
	}
	res := s.db.WithContext(ctx).
		Model(&DeletionJob{}).
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrJobNotFound
	}
	return nil
}

func (s *QueueStore) CountCompletedForEntity(ctx context.Context, entityType string, entityID uint) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).
		Model(&DeletionJob{}).
		Where("entity_type = ? AND entity_id = ? AND status = ?", entityType, entityID, StatusCompleted).
		Count(&n).Error
	return n, err
}

// CountOpenJobsForEntity returns the number of non-terminal jobs (pending or
// processing) for an entity, regardless of attempt count. Used by the cleanup
// scheduler to avoid enqueuing duplicate work for an entity that still has
// an in-flight or scheduled job.
func (s *QueueStore) CountOpenJobsForEntity(ctx context.Context, entityType string, entityID uint) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).
		Model(&DeletionJob{}).
		Where("entity_type = ? AND entity_id = ? AND status IN ?", entityType, entityID, []string{StatusPending, StatusProcessing}).
		Count(&n).Error
	return n, err
}

func BackoffDuration(attempts int) time.Duration {
	if attempts <= 0 {
		return 0
	}
	secs := math.Pow(2, float64(attempts))
	if secs > 600 {
		secs = 600
	}
	return time.Duration(secs) * time.Second
}
