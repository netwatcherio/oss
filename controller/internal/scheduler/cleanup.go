package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// RetentionConfig holds data retention settings
type RetentionConfig struct {
	DataRetentionDays   int           // Days to keep probe data in ClickHouse
	SoftDeleteGraceDays int           // Days before hard-deleting soft-deleted entities
	CleanupInterval     time.Duration // How often cleanup runs
}

// LoadRetentionConfig loads retention settings from environment variables
func LoadRetentionConfig() *RetentionConfig {
	return &RetentionConfig{
		DataRetentionDays:   getEnvInt("DATA_RETENTION_DAYS", 90),
		SoftDeleteGraceDays: getEnvInt("SOFT_DELETE_GRACE_DAYS", 30),
		CleanupInterval:     time.Duration(getEnvInt("CLEANUP_INTERVAL_HOURS", 24)) * time.Hour,
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
	log.Infof("Starting cleanup scheduler (interval: %v, soft-delete grace: %d days, data retention: %d days)",
		s.config.CleanupInterval, s.config.SoftDeleteGraceDays, s.config.DataRetentionDays)

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

	elapsed := time.Since(startTime)
	if totalDeleted > 0 {
		log.Infof("Cleanup complete: hard-deleted %d total records in %v", totalDeleted, elapsed)
	} else {
		log.Debugf("Cleanup complete: no records to delete (took %v)", elapsed)
	}
}

// UpdateClickHouseTTL modifies table TTL to match configured retention
func UpdateClickHouseTTL(ctx context.Context, ch *sql.DB, table string, days int) error {
	query := fmt.Sprintf(
		"ALTER TABLE %s MODIFY TTL created_at + INTERVAL %d DAY DELETE",
		table, days,
	)
	_, err := ch.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update TTL for %s: %w", table, err)
	}
	log.Infof("Updated %s TTL to %d days", table, days)
	return nil
}

// EnsureClickHouseTTL ensures all relevant tables have correct TTL settings
func EnsureClickHouseTTL(ctx context.Context, ch *sql.DB, days int) error {
	tables := []string{
		"probe_data",
		"ip_geo_cache",
		"ip_whois_cache",
	}

	for _, table := range tables {
		if err := UpdateClickHouseTTL(ctx, ch, table, days); err != nil {
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
