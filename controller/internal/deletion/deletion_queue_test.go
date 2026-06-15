package deletion

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&DeletionJob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestEnqueueAndListPending(t *testing.T) {
	db := newTestDB(t)
	store := NewQueueStore(db)
	ctx := context.Background()

	if err := store.Enqueue(ctx, EntityProbe, 42); err != nil {
		t.Fatalf("enqueue probe: %v", err)
	}
	if err := store.Enqueue(ctx, EntityAgent, 7); err != nil {
		t.Fatalf("enqueue agent: %v", err)
	}

	jobs, err := store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	for _, j := range jobs {
		if j.Status != StatusPending {
			t.Errorf("status = %q, want pending", j.Status)
		}
	}
}

func TestEnqueueValidation(t *testing.T) {
	db := newTestDB(t)
	store := NewQueueStore(db)
	ctx := context.Background()

	if err := store.Enqueue(ctx, "bogus", 1); err != ErrBadEntity {
		t.Errorf("enqueue bogus entity: err = %v, want ErrBadEntity", err)
	}
	if err := store.Enqueue(ctx, EntityProbe, 0); err != ErrBadID {
		t.Errorf("enqueue id=0: err = %v, want ErrBadID", err)
	}
}

func TestMarkProcessingClaimedOnlyOnce(t *testing.T) {
	db := newTestDB(t)
	store := NewQueueStore(db)
	ctx := context.Background()
	if err := store.Enqueue(ctx, EntityProbe, 1); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	jobs, err := store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if err := store.MarkProcessing(ctx, jobs[0].ID); err != nil {
		t.Fatalf("mark processing: %v", err)
	}
	if err := store.MarkProcessing(ctx, jobs[0].ID); err != ErrJobNotFound {
		t.Errorf("second mark processing: err = %v, want ErrJobNotFound", err)
	}
}

func TestMarkCompleted(t *testing.T) {
	db := newTestDB(t)
	store := NewQueueStore(db)
	ctx := context.Background()
	if err := store.Enqueue(ctx, EntityProbe, 2); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	jobs, err := store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := store.MarkProcessing(ctx, jobs[0].ID); err != nil {
		t.Fatalf("mark processing: %v", err)
	}
	if err := store.MarkCompleted(ctx, jobs[0].ID); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	n, err := store.CountCompletedForEntity(ctx, EntityProbe, 2)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1", n)
	}

	jobs, err = store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0", len(jobs))
	}
}

func TestMarkFailedRetriesWithinBudget(t *testing.T) {
	db := newTestDB(t)
	store := NewQueueStore(db)
	ctx := context.Background()
	if err := store.Enqueue(ctx, EntityProbe, 3); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	jobs, err := store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	id := jobs[0].ID
	if err := store.MarkProcessing(ctx, id); err != nil {
		t.Fatalf("mark processing: %v", err)
	}
	if err := store.MarkFailed(ctx, id, jobs[0].Attempts, jobs[0].MaxAttempts, errBoom{}, time.Second); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	var job DeletionJob
	if err := db.First(&job, id).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if job.Status != StatusPending {
		t.Errorf("status = %q, want pending", job.Status)
	}
	if job.ScheduledAt == nil {
		t.Errorf("scheduled_at is nil, want a future time")
	}
}

func TestMarkFailedExhaustsRetries(t *testing.T) {
	db := newTestDB(t)
	store := NewQueueStore(db)
	ctx := context.Background()
	if err := store.Enqueue(ctx, EntityProbe, 4); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	jobs, err := store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	id := jobs[0].ID
	if err := store.MarkProcessing(ctx, id); err != nil {
		t.Fatalf("mark processing: %v", err)
	}
	if err := store.MarkFailed(ctx, id, jobs[0].MaxAttempts, jobs[0].MaxAttempts, errBoom{}, 0); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	var job DeletionJob
	if err := db.First(&job, id).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if job.Status != StatusFailed {
		t.Errorf("status = %q, want failed", job.Status)
	}
}

func TestListPendingRespectsScheduledAt(t *testing.T) {
	db := newTestDB(t)
	store := NewQueueStore(db)
	ctx := context.Background()
	if err := store.Enqueue(ctx, EntityProbe, 5); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	jobs, err := store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	id := jobs[0].ID
	if err := store.MarkProcessing(ctx, id); err != nil {
		t.Fatalf("mark processing: %v", err)
	}
	future := time.Now().Add(1 * time.Hour)
	if err := store.MarkFailed(ctx, id, jobs[0].Attempts, jobs[0].MaxAttempts, errBoom{}, time.Until(future)); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	jobs, err = store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0 (future scheduled_at should be deferred)", len(jobs))
	}
}

func TestBackoffDurationCap(t *testing.T) {
	if got := BackoffDuration(0); got != 0 {
		t.Errorf("BackoffDuration(0) = %v, want 0", got)
	}
	if got := BackoffDuration(1); got != 2*time.Second {
		t.Errorf("BackoffDuration(1) = %v, want 2s", got)
	}
	if got := BackoffDuration(2); got != 4*time.Second {
		t.Errorf("BackoffDuration(2) = %v, want 4s", got)
	}
	if got := BackoffDuration(20); got != 10*time.Minute {
		t.Errorf("BackoffDuration(20) = %v, want 10m (capped)", got)
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
