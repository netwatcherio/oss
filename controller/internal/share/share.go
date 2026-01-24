// internal/share/share.go
package share

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// -------------------- Errors --------------------

var (
	ErrShareLinkNotFound = errors.New("share link not found")
	ErrShareLinkExpired  = errors.New("share link has expired")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrPasswordRequired  = errors.New("password required")
)

// -------------------- ShareLink Model --------------------

// ShareLink represents a time-limited shareable link to an agent page.
type ShareLink struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Token is the unique identifier used in the public URL
	Token string `gorm:"size:64;uniqueIndex:ux_share_links_token" json:"token"`

	// Scope
	WorkspaceID uint `gorm:"index" json:"workspace_id"`
	AgentID     uint `gorm:"index:idx_share_links_agent" json:"agent_id"`

	// Creator
	CreatedByUserID uint `json:"created_by_user_id"`

	// Expiration
	ExpiresAt time.Time `gorm:"index:idx_share_links_expires" json:"expires_at"`

	// Optional password protection (bcrypt hash)
	PasswordHash string `gorm:"size:255" json:"-"`
	HasPassword  bool   `gorm:"-" json:"has_password"` // Computed field for API

	// Access tracking
	AccessCount    int        `gorm:"default:0" json:"access_count"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`

	// Computed: speedtest allowed only for short-term shares (<24h expiry)
	AllowSpeedtest bool `gorm:"-" json:"allow_speedtest"`
}

func (ShareLink) TableName() string { return "share_links" }

// -------------------- DTOs --------------------

// CreateInput is the input for creating a share link.
type CreateInput struct {
	WorkspaceID     uint
	AgentID         uint
	CreatedByUserID uint
	ExpiresIn       time.Duration // How long until expiration
	Password        string        // Optional plaintext password
}

// CreateOutput is returned after successful creation.
type CreateOutput struct {
	ShareLink *ShareLink `json:"share_link"`
	Token     string     `json:"token"` // Full token for URL construction
}

// ValidateInput is used to validate access to a share link.
type ValidateInput struct {
	Token    string
	Password string
}

// -------------------- Public API --------------------

// GenerateToken creates a cryptographically secure random token.
func GenerateToken() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// Create creates a new share link for an agent.
func Create(ctx context.Context, db *gorm.DB, in CreateInput) (*CreateOutput, error) {
	token, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	link := &ShareLink{
		Token:           token,
		WorkspaceID:     in.WorkspaceID,
		AgentID:         in.AgentID,
		CreatedByUserID: in.CreatedByUserID,
		ExpiresAt:       time.Now().Add(in.ExpiresIn),
	}

	// Hash password if provided
	if in.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		link.PasswordHash = string(hash)
	}

	if err := db.WithContext(ctx).Create(link).Error; err != nil {
		return nil, err
	}

	link.HasPassword = link.PasswordHash != ""
	link.AllowSpeedtest = time.Until(link.ExpiresAt) < 24*time.Hour

	return &CreateOutput{
		ShareLink: link,
		Token:     token,
	}, nil
}

// GetByToken retrieves a share link by its token without validation.
func GetByToken(ctx context.Context, db *gorm.DB, token string) (*ShareLink, error) {
	var link ShareLink
	err := db.WithContext(ctx).Where("token = ?", token).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrShareLinkNotFound
	}
	if err != nil {
		return nil, err
	}

	link.HasPassword = link.PasswordHash != ""
	link.AllowSpeedtest = time.Until(link.ExpiresAt) < 24*time.Hour
	return &link, nil
}

// Validate checks if a share link is valid and optionally verifies the password.
// Returns the link if valid, or an error if expired/invalid password.
func Validate(ctx context.Context, db *gorm.DB, in ValidateInput) (*ShareLink, error) {
	link, err := GetByToken(ctx, db, in.Token)
	if err != nil {
		return nil, err
	}

	// Check expiration
	if time.Now().After(link.ExpiresAt) {
		return nil, ErrShareLinkExpired
	}

	// Check password if required
	if link.PasswordHash != "" {
		if in.Password == "" {
			return nil, ErrPasswordRequired
		}
		if err := bcrypt.CompareHashAndPassword([]byte(link.PasswordHash), []byte(in.Password)); err != nil {
			return nil, ErrInvalidPassword
		}
	}

	return link, nil
}

// RecordAccess increments the access count and updates last accessed time.
func RecordAccess(ctx context.Context, db *gorm.DB, linkID uint) error {
	now := time.Now()
	return db.WithContext(ctx).
		Model(&ShareLink{}).
		Where("id = ?", linkID).
		Updates(map[string]any{
			"access_count":     gorm.Expr("access_count + 1"),
			"last_accessed_at": now,
		}).Error
}

// ListByAgent returns all share links for a given agent.
func ListByAgent(ctx context.Context, db *gorm.DB, workspaceID, agentID uint) ([]ShareLink, error) {
	var links []ShareLink
	err := db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ?", workspaceID, agentID).
		Order("created_at DESC").
		Find(&links).Error
	if err != nil {
		return nil, err
	}

	// Set computed fields
	for i := range links {
		links[i].HasPassword = links[i].PasswordHash != ""
		links[i].AllowSpeedtest = time.Until(links[i].ExpiresAt) < 24*time.Hour
	}

	return links, nil
}

// Delete removes a share link by ID.
func Delete(ctx context.Context, db *gorm.DB, workspaceID, agentID, linkID uint) error {
	result := db.WithContext(ctx).
		Where("id = ? AND workspace_id = ? AND agent_id = ?", linkID, workspaceID, agentID).
		Delete(&ShareLink{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrShareLinkNotFound
	}
	return nil
}

// DeleteExpired removes all expired share links (for cleanup jobs).
func DeleteExpired(ctx context.Context, db *gorm.DB) (int64, error) {
	result := db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&ShareLink{})
	return result.RowsAffected, result.Error
}
