package probe

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// AnalysisLoopConfig holds configuration for the background analysis loop
type AnalysisLoopConfig struct {
	Interval time.Duration // How often to run analysis (default: 5 minutes)
}

// LoadAnalysisLoopConfig loads config from environment variables
func LoadAnalysisLoopConfig() AnalysisLoopConfig {
	interval := 300 // default 5 minutes
	if v := os.Getenv("ANALYSIS_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			interval = n
		}
	}
	return AnalysisLoopConfig{
		Interval: time.Duration(interval) * time.Second,
	}
}

// StartAnalysisLoop runs workspace analysis periodically in the background.
// It evaluates all workspaces with active agents, computes health analysis,
// and fires alerts for any detected incidents matching alert rules.
func StartAnalysisLoop(ctx context.Context, ch *sql.DB, pg *gorm.DB, config AnalysisLoopConfig) {
	log.Infof("[analysis_loop] starting background analysis (interval: %s)", config.Interval)

	// Initial delay to let the system settle after startup
	select {
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("[analysis_loop] shutting down")
			return
		case <-ticker.C:
			runAnalysisCycle(ctx, ch, pg)
		}
	}
}

func runAnalysisCycle(ctx context.Context, ch *sql.DB, pg *gorm.DB) {
	start := time.Now()

	// Get all workspace IDs that have at least one agent
	workspaceIDs, err := getActiveWorkspaceIDs(ctx, pg)
	if err != nil {
		log.Warnf("[analysis_loop] failed to get workspace IDs: %v", err)
		return
	}

	if len(workspaceIDs) == 0 {
		return
	}

	var totalIncidents int
	for _, wsID := range workspaceIDs {
		analysis, err := ComputeWorkspaceAnalysis(ctx, ch, pg, wsID, 60)
		if err != nil {
			log.Warnf("[analysis_loop] workspace %d analysis failed: %v", wsID, err)
			continue
		}

		totalIncidents += len(analysis.Incidents)

		// Evaluate analysis against alert rules
		if err := EvaluateAnalysisIncidents(ctx, pg, wsID, analysis); err != nil {
			log.Warnf("[analysis_loop] workspace %d alert eval failed: %v", wsID, err)
		}
	}

	elapsed := time.Since(start)
	if totalIncidents > 0 {
		log.Infof("[analysis_loop] completed %d workspaces in %s (%d incidents detected)",
			len(workspaceIDs), elapsed.Round(time.Millisecond), totalIncidents)
	} else {
		log.Debugf("[analysis_loop] completed %d workspaces in %s (no incidents)",
			len(workspaceIDs), elapsed.Round(time.Millisecond))
	}
}

func getActiveWorkspaceIDs(ctx context.Context, pg *gorm.DB) ([]uint, error) {
	var ids []uint
	err := pg.WithContext(ctx).
		Table("agents").
		Where("deleted_at IS NULL").
		Distinct("workspace_id").
		Pluck("workspace_id", &ids).Error
	return ids, err
}
