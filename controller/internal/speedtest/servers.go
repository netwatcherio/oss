package speedtest

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// -------------------- Model --------------------

// CachedServer represents a speedtest.net server cached from an agent.
type CachedServer struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"index" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	AgentID    uint      `gorm:"index;not null" json:"agent_id"`
	ServerID   string    `gorm:"size:64;index" json:"server_id"` // speedtest.net server ID
	Name       string    `gorm:"size:256" json:"name"`
	Sponsor    string    `gorm:"size:256" json:"sponsor"`
	Host       string    `gorm:"size:512" json:"host"`
	URL        string    `gorm:"size:512" json:"url"`
	Country    string    `gorm:"size:128" json:"country"`
	Lat        string    `gorm:"size:32" json:"lat"`
	Lon        string    `gorm:"size:32" json:"lon"`
	Distance   float64   `json:"distance"` // km from agent
	LastSeenAt time.Time `gorm:"index" json:"last_seen_at"`
}

func (CachedServer) TableName() string { return "agent_speedtest_servers" }

// -------------------- DTOs --------------------

// ServerInput is used when decoding server data from the agent.
type ServerInput struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Sponsor  string  `json:"sponsor"`
	Host     string  `json:"host"`
	URL      string  `json:"url"`
	Country  string  `json:"country"`
	Lat      string  `json:"lat"`
	Lon      string  `json:"lon"`
	Distance float64 `json:"distance"`
}

// -------------------- CRUD Operations --------------------

// UpsertServersForAgent replaces the cached server list for an agent.
// Uses upsert semantics: inserts new servers and updates existing ones.
func UpsertServersForAgent(ctx context.Context, db *gorm.DB, agentID uint, servers []ServerInput) error {
	if len(servers) == 0 {
		return nil
	}

	now := time.Now()
	var rows []CachedServer

	for _, s := range servers {
		rows = append(rows, CachedServer{
			AgentID:    agentID,
			ServerID:   s.ID,
			Name:       s.Name,
			Sponsor:    s.Sponsor,
			Host:       s.Host,
			URL:        s.URL,
			Country:    s.Country,
			Lat:        s.Lat,
			Lon:        s.Lon,
			Distance:   s.Distance,
			LastSeenAt: now,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}

	// Upsert: on conflict (agent_id, server_id), update fields
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}, {Name: "server_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "sponsor", "host", "url", "country", "lat", "lon", "distance", "last_seen_at", "updated_at"}),
	}).Create(&rows).Error
}

// ListServersForAgent returns the cached server list for an agent.
func ListServersForAgent(ctx context.Context, db *gorm.DB, agentID uint) ([]CachedServer, error) {
	var servers []CachedServer
	err := db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Order("distance ASC").
		Limit(100).
		Find(&servers).Error
	return servers, err
}

// DeleteServersForAgent removes all cached servers for an agent.
func DeleteServersForAgent(ctx context.Context, db *gorm.DB, agentID uint) error {
	return db.WithContext(ctx).Where("agent_id = ?", agentID).Delete(&CachedServer{}).Error
}

// GetServerByID retrieves a specific cached server.
func GetServerByID(ctx context.Context, db *gorm.DB, agentID uint, serverID string) (*CachedServer, error) {
	var server CachedServer
	err := db.WithContext(ctx).
		Where("agent_id = ? AND server_id = ?", agentID, serverID).
		First(&server).Error
	if err != nil {
		return nil, err
	}
	return &server, nil
}
