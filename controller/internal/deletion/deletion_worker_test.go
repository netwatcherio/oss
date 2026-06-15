package deletion

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type fakeCH struct {
	mu         sync.Mutex
	probeCalls []uint
	agentCalls []uint
	failNext   bool
	failErr    error
}

func (f *fakeCH) DeleteProbeDataByProbeID(_ context.Context, id uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		err := f.failErr
		f.failNext = false
		return err
	}
	f.probeCalls = append(f.probeCalls, id)
	return nil
}

func (f *fakeCH) DeleteProbeDataByAgentID(_ context.Context, id uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		err := f.failErr
		f.failNext = false
		return err
	}
	f.agentCalls = append(f.agentCalls, id)
	return nil
}

func TestWorkerProcessesPendingJobs(t *testing.T) {
	db := newTestDB(t)
	store := NewQueueStore(db)
	ctx := context.Background()
	if err := store.Enqueue(ctx, EntityProbe, 101); err != nil {
		t.Fatalf("enqueue probe: %v", err)
	}
	if err := store.Enqueue(ctx, EntityAgent, 202); err != nil {
		t.Fatalf("enqueue agent: %v", err)
	}

	ch := &fakeCH{}
	w := NewWorkerWithOps(db, ch)
	w.processBatch()

	if len(ch.probeCalls) != 1 || ch.probeCalls[0] != 101 {
		t.Errorf("probe calls = %v, want [101]", ch.probeCalls)
	}
	if len(ch.agentCalls) != 1 || ch.agentCalls[0] != 202 {
		t.Errorf("agent calls = %v, want [202]", ch.agentCalls)
	}

	n, err := store.CountCompletedForEntity(ctx, EntityProbe, 101)
	if err != nil {
		t.Fatalf("count probe: %v", err)
	}
	if n != 1 {
		t.Errorf("probe completed count = %d, want 1", n)
	}
	n, err = store.CountCompletedForEntity(ctx, EntityAgent, 202)
	if err != nil {
		t.Fatalf("count agent: %v", err)
	}
	if n != 1 {
		t.Errorf("agent completed count = %d, want 1", n)
	}
}

func TestWorkerRetriesOnCHFailure(t *testing.T) {
	db := newTestDB(t)
	store := NewQueueStore(db)
	ctx := context.Background()
	if err := store.Enqueue(ctx, EntityProbe, 303); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	ch := &fakeCH{failNext: true, failErr: errors.New("ch down")}
	w := NewWorkerWithOps(db, ch)
	w.processBatch()

	jobs, err := store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0 (failed job should be scheduled in the future, not immediately runnable)", len(jobs))
	}

	var job DeletionJob
	if err := db.First(&job).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if job.Status != StatusPending {
		t.Errorf("status = %q, want pending (within retry budget)", job.Status)
	}
	if job.LastError != "ch down" {
		t.Errorf("last_error = %q, want %q", job.LastError, "ch down")
	}
	if job.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", job.Attempts)
	}
}
