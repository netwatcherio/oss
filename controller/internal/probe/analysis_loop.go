package probe

import (
	"context"
	"database/sql"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// AnalysisLoopConfig holds configuration for the background analysis loop
type AnalysisLoopConfig struct {
	Interval       time.Duration // How often to run analysis (default: 5 minutes)
	MaxConcurrent int           // Max parallel workspace analysis (default: 4 × GOMAXPROCS)
}

// LoadAnalysisLoopConfig loads config from environment variables
func LoadAnalysisLoopConfig() AnalysisLoopConfig {
	interval := 300 // default 5 minutes
	if v := os.Getenv("ANALYSIS_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			interval = n
		}
	}
	maxConcurrent := runtime.GOMAXPROCS(0) * 4
	if v := os.Getenv("ANALYSIS_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxConcurrent = n
		}
	}
	return AnalysisLoopConfig{
		Interval:       time.Duration(interval) * time.Second,
		MaxConcurrent: maxConcurrent,
	}
}

// StartAnalysisLoop runs workspace analysis periodically in the background.
// It evaluates all workspaces with active agents, computes health analysis,
// and fires alerts for any detected incidents matching alert rules.
// Large deployments are processed in parallel up to MaxConcurrent workers.
func StartAnalysisLoop(ctx context.Context, ch *sql.DB, pg *gorm.DB, config AnalysisLoopConfig) {
	log.Infof("[analysis_loop] starting background analysis (interval: %s, max_concurrent: %d)", config.Interval, config.MaxConcurrent)

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
			runAnalysisCycle(ctx, ch, pg, config)
		}
	}
}

func runAnalysisCycle(ctx context.Context, ch *sql.DB, pg *gorm.DB, config AnalysisLoopConfig) {
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

	// Parallel processing for large deployments (worker pool)
	if len(workspaceIDs) > 1 {
		runWorkspacesParallel(ctx, ch, pg, workspaceIDs, config.MaxConcurrent)
	} else {
		runSingleWorkspace(ctx, ch, pg, workspaceIDs[0])
	}

	elapsed := time.Since(start)
	log.Debugf("[analysis_loop] completed %d workspaces in %s", len(workspaceIDs), elapsed.Round(time.Millisecond))
}

func runSingleWorkspace(ctx context.Context, ch *sql.DB, pg *gorm.DB, wsID uint) {
	analysis, err := ComputeWorkspaceAnalysis(ctx, ch, pg, wsID, 60)
	if err != nil {
		log.Warnf("[analysis_loop] workspace %d analysis failed: %v", wsID, err)
		return
	}
	if err := SaveAnalysisSnapshot(ctx, ch, analysis); err != nil {
		log.Warnf("[analysis_loop] workspace %d snapshot save failed: %v", wsID, err)
	}
	if err := EvaluateAnalysisIncidents(ctx, pg, wsID, analysis); err != nil {
		log.Warnf("[analysis_loop] workspace %d alert eval failed: %v", wsID, err)
	}
}

func runWorkspacesParallel(ctx context.Context, ch *sql.DB, pg *gorm.DB, workspaceIDs []uint, maxConcurrent int) {
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	totalIncidents := 0

	for _, wsID := range workspaceIDs {
		wg.Add(1)
		go func(id uint) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }()  // release

			analysis, err := ComputeWorkspaceAnalysis(ctx, ch, pg, id, 60)
			if err != nil {
				log.Warnf("[analysis_loop] workspace %d analysis failed: %v", id, err)
				return
			}
			if err := SaveAnalysisSnapshot(ctx, ch, analysis); err != nil {
				log.Warnf("[analysis_loop] workspace %d snapshot save failed: %v", id, err)
			}
			if err := EvaluateAnalysisIncidents(ctx, pg, id, analysis); err != nil {
				log.Warnf("[analysis_loop] workspace %d alert eval failed: %v", id, err)
			}
			mu.Lock()
			totalIncidents += len(analysis.Incidents)
			mu.Unlock()
		}(wsID)
	}

	wg.Wait()
	if totalIncidents > 0 {
		log.Infof("[analysis_loop] completed %d workspaces in parallel (%d incidents detected)", len(workspaceIDs), totalIncidents)
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
