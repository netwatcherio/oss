package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"netwatcher-controller/internal/deletion"
	"netwatcher-controller/internal/users"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// RetentionConfig holds data retention settings
type RetentionConfig struct {
	DataRetentionDays   int           // Days to keep probe data in ClickHouse
	SoftDeleteGraceDays int           // Days before hard-deleting soft-deleted entities
	CleanupInterval     time.Duration // How often cleanup runs
	BackfillGracePeriod time.Duration // How long after soft-delete before backfilling a CH deletion job
}

// LoadRetentionConfig loads retention settings from environment variables
func LoadRetentionConfig() *RetentionConfig {
	return &RetentionConfig{
		DataRetentionDays:   getEnvInt("DATA_RETENTION_DAYS", 90),
		SoftDeleteGraceDays: getEnvInt("SOFT_DELETE_GRACE_DAYS", 30),
		CleanupInterval:     time.Duration(getEnvInt("CLEANUP_INTERVAL_HOURS", 24)) * time.Hour,
		BackfillGracePeriod: time.Duration(getEnvInt("DELETION_BACKFILL_GRACE_HOURS", 1)) * time.Hour,
	}
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

// CleanupScheduler handles periodic cleanup of old data
type CleanupScheduler struct {
	db     *gorm.DB
	ch     *sql.DB
	config *RetentionConfig
}

// NewCleanupScheduler creates a new cleanup scheduler
func NewCleanupScheduler(db *gorm.DB, ch *sql.DB, config *RetentionConfig) *CleanupScheduler {
	return &CleanupScheduler{
		db:     db,
		ch:     ch,
		config: config,
	}
}

// Start begins the cleanup scheduler in a blocking loop
func (s *CleanupScheduler) Start(ctx context.Context) {
	log.Infof("Starting cleanup scheduler (interval: %v, soft-delete grace: %d days, data retention: %d days, backfill grace: %v)",
		s.config.CleanupInterval, s.config.SoftDeleteGraceDays, s.config.DataRetentionDays, s.config.BackfillGracePeriod)

	// Run once on startup
	s.runCleanup(ctx)

	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Cleanup scheduler stopped")
			return
		case <-ticker.C:
			s.runCleanup(ctx)
		}
	}
}

// runCleanup performs the actual cleanup operations
func (s *CleanupScheduler) runCleanup(ctx context.Context) {
	log.Info("Running scheduled cleanup...")
	startTime := time.Now()

	cutoff := time.Now().AddDate(0, 0, -s.config.SoftDeleteGraceDays)
	backfillCutoff := time.Now().Add(-s.config.BackfillGracePeriod)

	var totalDeleted int64

	// Hard-delete agents soft-deleted before cutoff
	result := s.db.WithContext(ctx).Unscoped().
		Where("deleted_at IS NOT NULL AND deleted_at < ?", cutoff).
		Delete(&agentModel{})
	if result.Error != nil {
		log.Errorf("Failed to cleanup agents: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Infof("Hard-deleted %d agents", result.RowsAffected)
		totalDeleted += result.RowsAffected
	}

	// Hard-delete probes soft-deleted before cutoff
	result = s.db.WithContext(ctx).Unscoped().
		Where("deleted_at IS NOT NULL AND deleted_at < ?", cutoff).
		Delete(&probeModel{})
	if result.Error != nil {
		log.Errorf("Failed to cleanup probes: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Infof("Hard-deleted %d probes", result.RowsAffected)
		totalDeleted += result.RowsAffected
	}

	// Hard-delete probe targets soft-deleted before cutoff
	result = s.db.WithContext(ctx).Unscoped().
		Where("deleted_at IS NOT NULL AND deleted_at < ?", cutoff).
		Delete(&targetModel{})
	if result.Error != nil {
		log.Errorf("Failed to cleanup probe targets: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Infof("Hard-deleted %d probe targets", result.RowsAffected)
		totalDeleted += result.RowsAffected
	}

	// Hard-delete consumed/expired PINs older than cutoff
	result = s.db.WithContext(ctx).Unscoped().
		Where("deleted_at IS NOT NULL AND deleted_at < ?", cutoff).
		Delete(&pinModel{})
	if result.Error != nil {
		log.Errorf("Failed to cleanup agent pins: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Infof("Hard-deleted %d agent pins", result.RowsAffected)
		totalDeleted += result.RowsAffected
	}

	// Backfill any soft-deleted probe/agent that does not have a completed
	// ClickHouse deletion job. Safety net in case the in-request enqueue
	// failed (e.g. CH was down or the controller crashed mid-transaction).
	store := deletion.NewQueueStore(s.db)
	s.backfillDeletions(ctx, store, backfillCutoff)

	// Sweep expired user tokens (password reset, email verification) so the
	// user_tokens table doesn't grow without bound. ValidateToken already
	// deletes on access, but tokens that are never validated linger.
	tokenResult := s.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&users.UserToken{})
	if tokenResult.Error != nil {
		log.Errorf("Failed to cleanup expired user tokens: %v", tokenResult.Error)
	} else if tokenResult.RowsAffected > 0 {
		log.Infof("Cleaned up %d expired user tokens", tokenResult.RowsAffected)
		totalDeleted += tokenResult.RowsAffected
	}

	elapsed := time.Since(startTime)
	if totalDeleted > 0 {
		log.Infof("Cleanup complete: hard-deleted %d total records in %v", totalDeleted, elapsed)
	} else {
		log.Debugf("Cleanup complete: no records to delete (took %v)", elapsed)
	}
}

// backfillDeletions enqueues CH cleanup jobs for any soft-deleted entity that
// has been deleted long enough ago to be considered "missed" by the inline
// enqueue path, and that has not yet been completed by the worker.
func (s *CleanupScheduler) backfillDeletions(ctx context.Context, store *deletion.QueueStore, cutoff time.Time) {
	type idRow struct {
		ID uint
	}

	var probeRows []idRow
	if err := s.db.WithContext(ctx).Unscoped().
		Model(&probeModel{}).
		Where("deleted_at IS NOT NULL AND deleted_at < ?", cutoff).
		Pluck("id", &probeRows).Error; err != nil {
		log.Errorf("backfill: list soft-deleted probes failed: %v", err)
	} else {
		enqueued := 0
		for _, r := range probeRows {
			n, err := store.CountCompletedForEntity(ctx, deletion.EntityProbe, r.ID)
			if err != nil {
				log.WithError(err).Warn("backfill: count completed probe jobs failed")
				continue
			}
			if n > 0 {
				continue
			}
			open, err := store.CountOpenJobsForEntity(ctx, deletion.EntityProbe, r.ID)
			if err != nil {
				log.WithError(err).Warn("backfill: count open probe jobs failed")
				continue
			}
			if open > 0 {
				continue
			}
			if err := store.EnqueueBackfill(ctx, deletion.EntityProbe, r.ID); err != nil {
				log.WithError(err).WithField("probe_id", r.ID).Warn("backfill: enqueue probe deletion failed")
				continue
			}
			enqueued++
		}
		if enqueued > 0 {
			log.Infof("Backfilled %d probe deletion jobs", enqueued)
		}
	}

	var agentRows []idRow
	if err := s.db.WithContext(ctx).Unscoped().
		Model(&agentModel{}).
		Where("deleted_at IS NOT NULL AND deleted_at < ?", cutoff).
		Pluck("id", &agentRows).Error; err != nil {
		log.Errorf("backfill: list soft-deleted agents failed: %v", err)
	} else {
		enqueued := 0
		for _, r := range agentRows {
			n, err := store.CountCompletedForEntity(ctx, deletion.EntityAgent, r.ID)
			if err != nil {
				log.WithError(err).Warn("backfill: count completed agent jobs failed")
				continue
			}
			if n > 0 {
				continue
			}
			open, err := store.CountOpenJobsForEntity(ctx, deletion.EntityAgent, r.ID)
			if err != nil {
				log.WithError(err).Warn("backfill: count open agent jobs failed")
				continue
			}
			if open > 0 {
				continue
			}
			if err := store.EnqueueBackfill(ctx, deletion.EntityAgent, r.ID); err != nil {
				log.WithError(err).WithField("agent_id", r.ID).Warn("backfill: enqueue agent deletion failed")
				continue
			}
			enqueued++
		}
		if enqueued > 0 {
			log.Infof("Backfilled %d agent deletion jobs", enqueued)
		}
	}
}

// UpdateClickHouseTTL modifies table TTL to match configured retention
func UpdateClickHouseTTL(ctx context.Context, ch *sql.DB, table string, ttlColumn string, days int) error {
	query := fmt.Sprintf(
		"ALTER TABLE %s MODIFY TTL %s + INTERVAL %d DAY DELETE",
		table, ttlColumn, days,
	)
	_, err := ch.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update TTL for %s: %w", table, err)
	}
	log.Infof("Updated %s TTL to %d days (column: %s)", table, days, ttlColumn)
	return nil
}

// EnsureClickHouseTTL ensures all relevant tables have correct TTL settings
func EnsureClickHouseTTL(ctx context.Context, ch *sql.DB, days int) error {
	// Table name -> TTL column name
	tables := map[string]string{
		"probe_data":     "created_at",
		"ip_geo_cache":   "lookup_time",
		"ip_whois_cache": "lookup_time",
	}

	for table, ttlCol := range tables {
		if err := UpdateClickHouseTTL(ctx, ch, table, ttlCol, days); err != nil {
			// Log but don't fail - table might not exist yet
			log.Warnf("Could not update TTL for %s: %v", table, err)
		}
	}
	return nil
}

// Model stubs for GORM (avoid import cycles)
type agentModel struct{}

func (agentModel) TableName() string { return "agents" }

type probeModel struct{}

func (probeModel) TableName() string { return "probes" }

type targetModel struct{}

func (targetModel) TableName() string { return "probe_targets" }

type pinModel struct{}

func (pinModel) TableName() string { return "agent_pins" }
