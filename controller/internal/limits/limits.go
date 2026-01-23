package limits

import (
	"context"
	"errors"
	"os"
	"strconv"

	"gorm.io/gorm"
)

// Limit-related errors
var (
	ErrMemberLimitReached    = errors.New("workspace member limit reached")
	ErrAgentLimitReached     = errors.New("workspace agent limit reached")
	ErrProbeLimitReached     = errors.New("agent probe limit reached")
	ErrWorkspaceLimitReached = errors.New("user has reached maximum workspace memberships")
)

// Config holds the limit configuration from environment variables
type Config struct {
	MaxMembersPerWorkspace int // 0 = unlimited
	MaxAgentsPerWorkspace  int // 0 = unlimited
	MaxProbesPerAgent      int // 0 = unlimited
	MaxWorkspacesPerUser   int // 0 = unlimited (memberships across all workspaces)
}

// LoadFromEnv loads limit configuration from environment variables.
// All limits default to 0 (unlimited) if not specified.
func LoadFromEnv() *Config {
	return &Config{
		MaxMembersPerWorkspace: getEnvInt("MAX_MEMBERS_PER_WORKSPACE", 0),
		MaxAgentsPerWorkspace:  getEnvInt("MAX_AGENTS_PER_WORKSPACE", 0),
		MaxProbesPerAgent:      getEnvInt("MAX_PROBES_PER_AGENT", 0),
		MaxWorkspacesPerUser:   getEnvInt("MAX_WORKSPACES_PER_USER", 0),
	}
}

// getEnvInt returns the int value of an environment variable, or the default if not set or invalid.
func getEnvInt(key string, def int) int {
	s := os.Getenv(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// CanAddMember checks if a workspace can accept another member.
// Returns nil if allowed, ErrMemberLimitReached if limit is reached.
func CanAddMember(ctx context.Context, db *gorm.DB, cfg *Config, workspaceID uint) error {
	if cfg == nil || cfg.MaxMembersPerWorkspace <= 0 {
		return nil // unlimited
	}

	var count int64
	err := db.WithContext(ctx).
		Table("workspace_members").
		Where("workspace_id = ? AND deleted_at IS NULL", workspaceID).
		Count(&count).Error
	if err != nil {
		return err
	}

	if int(count) >= cfg.MaxMembersPerWorkspace {
		return ErrMemberLimitReached
	}
	return nil
}

// CanJoinWorkspace checks if a user can join another workspace.
// Returns nil if allowed, ErrWorkspaceLimitReached if limit is reached.
func CanJoinWorkspace(ctx context.Context, db *gorm.DB, cfg *Config, userID uint) error {
	if cfg == nil || cfg.MaxWorkspacesPerUser <= 0 {
		return nil // unlimited
	}

	// Skip check for email-only invites (userID = 0)
	if userID == 0 {
		return nil
	}

	var count int64
	err := db.WithContext(ctx).
		Table("workspace_members").
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Count(&count).Error
	if err != nil {
		return err
	}

	if int(count) >= cfg.MaxWorkspacesPerUser {
		return ErrWorkspaceLimitReached
	}
	return nil
}

// CanAddAgent checks if a workspace can accept another agent.
// Returns nil if allowed, ErrAgentLimitReached if limit is reached.
func CanAddAgent(ctx context.Context, db *gorm.DB, cfg *Config, workspaceID uint) error {
	if cfg == nil || cfg.MaxAgentsPerWorkspace <= 0 {
		return nil // unlimited
	}

	var count int64
	err := db.WithContext(ctx).
		Table("agents").
		Where("workspace_id = ? AND deleted_at IS NULL", workspaceID).
		Count(&count).Error
	if err != nil {
		return err
	}

	if int(count) >= cfg.MaxAgentsPerWorkspace {
		return ErrAgentLimitReached
	}
	return nil
}

// CanAddProbe checks if an agent can accept another probe.
// Returns nil if allowed, ErrProbeLimitReached if limit is reached.
func CanAddProbe(ctx context.Context, db *gorm.DB, cfg *Config, agentID uint) error {
	if cfg == nil || cfg.MaxProbesPerAgent <= 0 {
		return nil // unlimited
	}

	var count int64
	err := db.WithContext(ctx).
		Table("probes").
		Where("agent_id = ? AND deleted_at IS NULL", agentID).
		Count(&count).Error
	if err != nil {
		return err
	}

	if int(count) >= cfg.MaxProbesPerAgent {
		return ErrProbeLimitReached
	}
	return nil
}

// CountMembersInWorkspace returns the number of active members in a workspace.
func CountMembersInWorkspace(ctx context.Context, db *gorm.DB, workspaceID uint) (int64, error) {
	var count int64
	err := db.WithContext(ctx).
		Table("workspace_members").
		Where("workspace_id = ? AND deleted_at IS NULL", workspaceID).
		Count(&count).Error
	return count, err
}

// CountAgentsInWorkspace returns the number of active agents in a workspace.
func CountAgentsInWorkspace(ctx context.Context, db *gorm.DB, workspaceID uint) (int64, error) {
	var count int64
	err := db.WithContext(ctx).
		Table("agents").
		Where("workspace_id = ? AND deleted_at IS NULL", workspaceID).
		Count(&count).Error
	return count, err
}

// CountProbesForAgent returns the number of active probes for an agent.
func CountProbesForAgent(ctx context.Context, db *gorm.DB, agentID uint) (int64, error) {
	var count int64
	err := db.WithContext(ctx).
		Table("probes").
		Where("agent_id = ? AND deleted_at IS NULL", agentID).
		Count(&count).Error
	return count, err
}

// CountUserMemberships returns the number of workspaces a user belongs to.
func CountUserMemberships(ctx context.Context, db *gorm.DB, userID uint) (int64, error) {
	var count int64
	err := db.WithContext(ctx).
		Table("workspace_members").
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Count(&count).Error
	return count, err
}
