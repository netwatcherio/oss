package admin

import (
	"context"
	"os"
	"strings"
	"time"

	"netwatcher-controller/internal/users"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// SiteAdminRole is the role value that grants site-wide admin access
const SiteAdminRole = "SITE_ADMIN"

// Config holds admin bootstrap configuration
type Config struct {
	DefaultAdminEmail    string
	DefaultAdminPassword string
}

// LoadConfigFromEnv loads admin configuration from environment variables
func LoadConfigFromEnv() Config {
	return Config{
		DefaultAdminEmail:    getenv("DEFAULT_ADMIN_EMAIL", "admin@netwatcher.local"),
		DefaultAdminPassword: os.Getenv("DEFAULT_ADMIN_PASSWORD"),
	}
}

// BootstrapDefaultAdmin creates a default admin user if no users exist in the database.
// This is intended to be called at startup to ensure there's always a way to access the admin panel.
func BootstrapDefaultAdmin(ctx context.Context, db *gorm.DB, cfg Config) error {
	var count int64
	if err := db.WithContext(ctx).Model(&users.User{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		log.Debug("Users exist, skipping default admin bootstrap")
		return nil
	}

	// No users exist - check if we should create default admin
	if cfg.DefaultAdminPassword == "" {
		log.Warn("No users exist and DEFAULT_ADMIN_PASSWORD not set. " +
			"First registered user will have normal privileges. " +
			"Set DEFAULT_ADMIN_PASSWORD to create a default admin on startup.")
		return nil
	}

	email := strings.ToLower(strings.TrimSpace(cfg.DefaultAdminEmail))
	if email == "" {
		email = "admin@netwatcher.local"
	}

	log.WithField("email", email).Info("Creating default site admin user")

	// Create the admin user
	user, err := users.Register(ctx, db, users.RegisterInput{
		Email:    email,
		Password: cfg.DefaultAdminPassword,
		Name:     "Site Admin",
		Role:     SiteAdminRole,
	})
	if err != nil {
		return err
	}

	// Mark as verified
	if err := users.MarkVerified(ctx, db, user.ID); err != nil {
		log.WithError(err).Warn("Failed to mark default admin as verified")
	}

	log.WithFields(log.Fields{
		"id":    user.ID,
		"email": user.Email,
	}).Info("Default site admin created successfully")

	return nil
}

// IsSiteAdmin checks if a user has site admin privileges
func IsSiteAdmin(u *users.User) bool {
	return u != nil && u.Role == SiteAdminRole
}

// Stats holds system-wide statistics
type Stats struct {
	TotalUsers      int64     `json:"total_users"`
	TotalWorkspaces int64     `json:"total_workspaces"`
	TotalAgents     int64     `json:"total_agents"`
	ActiveAgents    int64     `json:"active_agents"`
	TotalProbes     int64     `json:"total_probes"`
	GeneratedAt     time.Time `json:"generated_at"`
}

// GetStats retrieves system-wide statistics
func GetStats(ctx context.Context, db *gorm.DB) (*Stats, error) {
	stats := &Stats{
		GeneratedAt: time.Now(),
	}

	// Count users
	if err := db.WithContext(ctx).Table("users").Count(&stats.TotalUsers).Error; err != nil {
		return nil, err
	}

	// Count workspaces
	if err := db.WithContext(ctx).Table("workspaces").Where("deleted_at IS NULL").Count(&stats.TotalWorkspaces).Error; err != nil {
		return nil, err
	}

	// Count agents
	if err := db.WithContext(ctx).Table("agents").Count(&stats.TotalAgents).Error; err != nil {
		return nil, err
	}

	// Count active agents (seen in last 5 minutes)
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	if err := db.WithContext(ctx).Table("agents").Where("last_seen_at > ?", fiveMinutesAgo).Count(&stats.ActiveAgents).Error; err != nil {
		return nil, err
	}

	// Count probes
	if err := db.WithContext(ctx).Table("probes").Where("deleted_at IS NULL").Count(&stats.TotalProbes).Error; err != nil {
		return nil, err
	}

	return stats, nil
}

// WorkspaceStats holds per-workspace statistics
type WorkspaceStats struct {
	WorkspaceID   uint   `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	MemberCount   int64  `json:"member_count"`
	AgentCount    int64  `json:"agent_count"`
	ActiveAgents  int64  `json:"active_agents"`
	ProbeCount    int64  `json:"probe_count"`
}

// GetWorkspaceStats retrieves statistics for all workspaces
func GetWorkspaceStats(ctx context.Context, db *gorm.DB) ([]WorkspaceStats, error) {
	type workspace struct {
		ID   uint
		Name string
	}
	var workspaces []workspace
	if err := db.WithContext(ctx).Table("workspaces").Where("deleted_at IS NULL").Find(&workspaces).Error; err != nil {
		return nil, err
	}

	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	result := make([]WorkspaceStats, 0, len(workspaces))

	for _, ws := range workspaces {
		stats := WorkspaceStats{
			WorkspaceID:   ws.ID,
			WorkspaceName: ws.Name,
		}

		db.WithContext(ctx).Table("workspace_members").
			Where("workspace_id = ? AND deleted_at IS NULL", ws.ID).
			Count(&stats.MemberCount)

		db.WithContext(ctx).Table("agents").
			Where("workspace_id = ?", ws.ID).
			Count(&stats.AgentCount)

		db.WithContext(ctx).Table("agents").
			Where("workspace_id = ? AND last_seen_at > ?", ws.ID, fiveMinutesAgo).
			Count(&stats.ActiveAgents)

		db.WithContext(ctx).Table("probes").
			Where("workspace_id = ? AND deleted_at IS NULL", ws.ID).
			Count(&stats.ProbeCount)

		result = append(result, stats)
	}

	return result, nil
}

// AgentWithWorkspace represents an agent with its workspace info for admin views
type AgentWithWorkspace struct {
	ID            uint      `json:"id"`
	WorkspaceID   uint      `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Version       string    `json:"version"`
	Location      string    `json:"location"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	Initialized   bool      `json:"initialized"`
	CreatedAt     time.Time `json:"created_at"`
	IsOnline      bool      `json:"is_online"`
}

// ListAllAgents returns all agents across all workspaces for admin view
func ListAllAgents(ctx context.Context, db *gorm.DB, limit, offset int) ([]AgentWithWorkspace, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var total int64
	if err := db.WithContext(ctx).Table("agents").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)

	rows, err := db.WithContext(ctx).
		Table("agents").
		Select("agents.id, agents.workspace_id, workspaces.name as workspace_name, agents.name, agents.description, agents.version, agents.location, agents.last_seen_at, agents.initialized, agents.created_at").
		Joins("LEFT JOIN workspaces ON workspaces.id = agents.workspace_id").
		Order("agents.id DESC").
		Limit(limit).
		Offset(offset).
		Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []AgentWithWorkspace
	for rows.Next() {
		var a AgentWithWorkspace
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.WorkspaceName, &a.Name, &a.Description, &a.Version, &a.Location, &a.LastSeenAt, &a.Initialized, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		a.IsOnline = a.LastSeenAt.After(fiveMinutesAgo)
		result = append(result, a)
	}

	return result, total, nil
}

func getenv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
