package scheduler

import (
	"context"
	"os"
	"strconv"
	"time"

	"netwatcher-controller/internal/alert"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// AlertSchedulerConfig holds alert scheduler settings
type AlertSchedulerConfig struct {
	OfflineCheckInterval time.Duration // How often to check for offline agents
}

// LoadAlertSchedulerConfig loads alert scheduler settings from environment variables
func LoadAlertSchedulerConfig() *AlertSchedulerConfig {
	minutes := 1 // Default: check every minute
	if v := os.Getenv("OFFLINE_CHECK_INTERVAL_MINUTES"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			minutes = i
		}
	}
	return &AlertSchedulerConfig{
		OfflineCheckInterval: time.Duration(minutes) * time.Minute,
	}
}

// AlertScheduler handles periodic alert evaluations
type AlertScheduler struct {
	db     *gorm.DB
	config *AlertSchedulerConfig
}

// NewAlertScheduler creates a new alert scheduler
func NewAlertScheduler(db *gorm.DB, config *AlertSchedulerConfig) *AlertScheduler {
	return &AlertScheduler{
		db:     db,
		config: config,
	}
}

// Start begins the alert scheduler in a blocking loop
func (s *AlertScheduler) Start(ctx context.Context) {
	log.Infof("Starting alert scheduler (offline check interval: %v)", s.config.OfflineCheckInterval)

	// Run once on startup (with a brief delay to let other systems initialize)
	time.Sleep(5 * time.Second)
	s.runOfflineCheck(ctx)

	ticker := time.NewTicker(s.config.OfflineCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Alert scheduler stopped")
			return
		case <-ticker.C:
			s.runOfflineCheck(ctx)
		}
	}
}

// runOfflineCheck evaluates agent offline alerts
func (s *AlertScheduler) runOfflineCheck(ctx context.Context) {
	if err := alert.EvaluateAgentOffline(ctx, s.db); err != nil {
		log.Errorf("Alert offline check failed: %v", err)
	}
}
