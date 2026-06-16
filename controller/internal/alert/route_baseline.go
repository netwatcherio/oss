package alert

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RouteBaseline stores the expected route for a probe (used for route_change detection)
type RouteBaseline struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ProbeID     uint      `gorm:"uniqueIndex;not null" json:"probe_id"`
	Fingerprint string    `gorm:"size:64;not null" json:"fingerprint"`   // SHA256 prefix of route path
	RoutePath   string    `gorm:"size:2048" json:"route_path,omitempty"` // Human-readable path
	HopCount    int       `json:"hop_count"`                             // Number of hops
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
	UpdatedAt   time.Time `gorm:"index" json:"updated_at"`
}

func (RouteBaseline) TableName() string { return "route_baselines" }

// SetRouteBaseline creates or updates the baseline route for a probe
func SetRouteBaseline(ctx context.Context, db *gorm.DB, probeID uint, fingerprint, routePath string, hopCount int) error {
	baseline := RouteBaseline{
		ProbeID:     probeID,
		Fingerprint: fingerprint,
		RoutePath:   routePath,
		HopCount:    hopCount,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "probe_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"fingerprint", "route_path", "hop_count", "updated_at"}),
	}).Create(&baseline).Error
}

// GetRouteBaseline retrieves the baseline route for a probe
func GetRouteBaseline(ctx context.Context, db *gorm.DB, probeID uint) (*RouteBaseline, error) {
	var baseline RouteBaseline
	err := db.WithContext(ctx).Where("probe_id = ?", probeID).First(&baseline).Error
	if err != nil {
		return nil, err
	}
	return &baseline, nil
}

// EnsureRouteBaseline captures the baseline for a probe if one does not yet
// exist. It is safe to call on every MTR data point: if a baseline row is
// already present, this is a no-op so the first-observed route is preserved.
// This is intentionally independent of alert rule evaluation so the analysis
// view has a baseline to compare against even when no route_change rule is
// configured.
func EnsureRouteBaseline(ctx context.Context, db *gorm.DB, probeID uint, fingerprint, routePath string, hopCount int) (created bool, err error) {
	var existing RouteBaseline
	err = db.WithContext(ctx).Where("probe_id = ?", probeID).First(&existing).Error
	if err == nil {
		return false, nil
	}
	if err != gorm.ErrRecordNotFound {
		return false, err
	}
	if err := SetRouteBaseline(ctx, db, probeID, fingerprint, routePath, hopCount); err != nil {
		return false, err
	}
	return true, nil
}

// RefreshRouteBaselineIfStale rewrites the stored baseline to the supplied
// fingerprint/routePath/hopCount when the existing baseline's UpdatedAt is
// at least staleThreshold old. It is safe to call on every MTR data point.
//
// Purpose: pick up intentional long-term route changes (e.g. agent moves
// networks, ISP reroutes the path) after a stabilization period, so the
// user is not alerted indefinitely against an obsolete baseline. ECMP /
// transient single-hop variations are handled in the analysis view by
// Jaccard similarity, not here, so a stable new route will eventually
// overwrite the baseline and stop firing route-change alerts.
//
// staleThreshold <= 0 disables refresh.
func RefreshRouteBaselineIfStale(ctx context.Context, db *gorm.DB, probeID uint, fingerprint, routePath string, hopCount int, staleThreshold time.Duration) (refreshed bool, err error) {
	if staleThreshold <= 0 {
		return false, nil
	}
	var existing RouteBaseline
	if err := db.WithContext(ctx).Where("probe_id = ?", probeID).First(&existing).Error; err != nil {
		return false, err
	}
	if time.Since(existing.UpdatedAt) < staleThreshold {
		return false, nil
	}
	if err := SetRouteBaseline(ctx, db, probeID, fingerprint, routePath, hopCount); err != nil {
		return false, err
	}
	return true, nil
}

// DeleteRouteBaseline removes the baseline for a probe
func DeleteRouteBaseline(ctx context.Context, db *gorm.DB, probeID uint) error {
	return db.WithContext(ctx).Where("probe_id = ?", probeID).Delete(&RouteBaseline{}).Error
}

// MigrateRouteBaseline creates the route_baselines table
func MigrateRouteBaseline(db *gorm.DB) error {
	return db.AutoMigrate(&RouteBaseline{})
}
