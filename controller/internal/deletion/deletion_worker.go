package deletion

import (
	"context"
	"database/sql"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Worker struct {
	db    *gorm.DB
	ch    CHOps
	store *QueueStore

	pollInterval time.Duration
	batchSize    int
	chTimeout    time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWorker wires the worker against a real *sql.DB-backed CHClient.
func NewWorker(db *gorm.DB, ch *sql.DB) *Worker {
	return NewWorkerWithOps(db, &CHClient{DB: ch})
}

// NewWorkerWithOps lets callers (mostly tests) inject a custom CHOps.
func NewWorkerWithOps(db *gorm.DB, ch CHOps) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	return &Worker{
		db:           db,
		ch:           ch,
		store:        NewQueueStore(db),
		pollInterval: pollInterval(),
		batchSize:    batchSize(),
		chTimeout:    chTimeout(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (w *Worker) Store() *QueueStore {
	return w.store
}

func (w *Worker) Start() error {
	log.WithFields(log.Fields{
		"poll_interval": w.pollInterval,
		"batch_size":    w.batchSize,
		"ch_timeout":    w.chTimeout,
	}).Info("Deletion worker started")

	w.wg.Add(1)
	go w.run()
	return nil
}

func (w *Worker) Stop() {
	w.cancel()
	w.wg.Wait()
	log.Info("Deletion worker stopped")
}

func (w *Worker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.processBatch()
		}
	}
}

func (w *Worker) processBatch() {
	jobs, err := w.store.ListPending(w.ctx, w.batchSize)
	if err != nil {
		log.WithError(err).Error("deletion: list pending failed")
		return
	}
	for _, j := range jobs {
		select {
		case <-w.ctx.Done():
			return
		default:
			w.processJob(j)
		}
	}
}

func (w *Worker) processJob(j DeletionJob) {
	logger := log.WithFields(log.Fields{
		"job_id":      j.ID,
		"entity_type": j.EntityType,
		"entity_id":   j.EntityID,
		"attempts":    j.Attempts,
	})

	if err := w.store.MarkProcessing(w.ctx, j.ID); err != nil {
		logger.WithError(err).Debug("deletion: mark processing failed (likely claimed by another worker)")
		return
	}

	ctx, cancel := context.WithTimeout(w.ctx, w.chTimeout)
	defer cancel()

	var runErr error
	switch j.EntityType {
	case EntityProbe:
		runErr = w.ch.DeleteProbeDataByProbeID(ctx, j.EntityID)
	case EntityAgent:
		runErr = w.ch.DeleteProbeDataByAgentID(ctx, j.EntityID)
	default:
		runErr = ErrBadEntity
	}

	if runErr != nil {
		newAttempts := j.Attempts + 1
		backoff := BackoffDuration(newAttempts)
		if markErr := w.store.MarkFailed(w.ctx, j.ID, newAttempts, j.MaxAttempts, runErr, backoff); markErr != nil {
			logger.WithError(markErr).Error("deletion: mark failed write failed")
		}
		if newAttempts >= j.MaxAttempts {
			logger.WithError(runErr).Error("deletion: job exhausted retries")
		} else {
			logger.WithError(runErr).WithField("backoff", backoff).Warn("deletion: job failed, will retry")
		}
		return
	}

	if err := w.store.MarkCompleted(w.ctx, j.ID); err != nil {
		logger.WithError(err).Error("deletion: mark completed failed")
		return
	}
	logger.Info("deletion: job completed")
}
