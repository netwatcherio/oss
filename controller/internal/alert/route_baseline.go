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

// DeleteRouteBaseline removes the baseline for a probe
func DeleteRouteBaseline(ctx context.Context, db *gorm.DB, probeID uint) error {
	return db.WithContext(ctx).Where("probe_id = ?", probeID).Delete(&RouteBaseline{}).Error
}

// MigrateRouteBaseline creates the route_baselines table
func MigrateRouteBaseline(db *gorm.DB) error {
	return db.AutoMigrate(&RouteBaseline{})
}
