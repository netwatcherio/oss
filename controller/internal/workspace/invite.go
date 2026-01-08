// internal/workspace/invite.go
package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Invite-specific errors
var (
	ErrInviteTokenExpired = errors.New("invite token expired")
	ErrInviteTokenInvalid = errors.New("invalid invite token")
)

// DefaultInviteExpiryHours is the default invite expiry (7 days)
const DefaultInviteExpiryHours = 168

// GetInviteExpiryHours reads EMAIL_INVITE_EXPIRY_HOURS env or returns default
func GetInviteExpiryHours() int {
	if v := os.Getenv("EMAIL_INVITE_EXPIRY_HOURS"); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h > 0 {
			return h
		}
	}
	return DefaultInviteExpiryHours
}

// generateToken creates a secure random token
func generateToken() (string, error) {
	bytes := make([]byte, 32) // 256-bit token
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// CreateInviteInput contains fields needed to create an invite
type CreateInviteInput struct {
	WorkspaceID uint
	Email       string
	Role        Role
	InvitedBy   uint // user ID of the inviter
}

// CreateInvite creates a member with an invite token
// Returns the member and the raw token (for sending in email)
func (s *Store) CreateInvite(ctx context.Context, in CreateInviteInput) (*Member, string, error) {
	if in.WorkspaceID == 0 || strings.TrimSpace(in.Email) == "" {
		return nil, "", ErrInvalidInput
	}
	if !in.Role.Valid() {
		return nil, "", ErrInvalidRole
	}
	if in.Role == RoleOwner {
		return nil, "", ErrForbidden
	}

	email := normEmail(in.Email)

	// Check if member already exists
	var existing Member
	err := s.db.WithContext(ctx).
		Where("workspace_id = ? AND email = ?", in.WorkspaceID, email).
		First(&existing).Error
	if err == nil {
		// Member already exists
		if existing.UserID != 0 {
			return nil, "", ErrAlreadyExists // already accepted
		}
		// Re-invite: generate new token
		token, err := generateToken()
		if err != nil {
			return nil, "", err
		}
		expiry := time.Now().Add(time.Duration(GetInviteExpiryHours()) * time.Hour)
		now := time.Now()

		updates := map[string]any{
			"invite_token":        token,
			"invite_token_expiry": expiry,
			"invited_at":          now,
			"role":                in.Role,
		}
		if err := s.db.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
			return nil, "", err
		}
		existing.InviteToken = token
		existing.InviteTokenExpiry = &expiry
		existing.InvitedAt = &now
		return &existing, token, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", err
	}

	// Ensure workspace exists
	if err := s.db.WithContext(ctx).First(&Workspace{}, "id = ?", in.WorkspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}

	// Generate token
	token, err := generateToken()
	if err != nil {
		return nil, "", err
	}

	expiry := time.Now().Add(time.Duration(GetInviteExpiryHours()) * time.Hour)
	now := time.Now()

	m := &Member{
		WorkspaceID:       in.WorkspaceID,
		UserID:            0, // no user yet
		Email:             email,
		Role:              in.Role,
		Meta:              jdefault(nil),
		InvitedAt:         &now,
		InviteToken:       token,
		InviteTokenExpiry: &expiry,
	}

	if err := s.db.WithContext(ctx).Create(m).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique") {
			return nil, "", ErrAlreadyExists
		}
		return nil, "", err
	}

	return m, token, nil
}

// GetMemberByInviteToken finds a member by their invite token
func (s *Store) GetMemberByInviteToken(ctx context.Context, token string) (*Member, error) {
	if token == "" {
		return nil, ErrInviteTokenInvalid
	}

	var m Member
	if err := s.db.WithContext(ctx).
		First(&m, "invite_token = ?", token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInviteTokenInvalid
		}
		return nil, err
	}

	// Check expiry
	if m.InviteTokenExpiry != nil && m.InviteTokenExpiry.Before(time.Now()) {
		return nil, ErrInviteTokenExpired
	}

	// Check if already accepted
	if m.UserID != 0 {
		return nil, ErrAlreadyExists
	}

	return &m, nil
}

// InviteInfo contains public info about an invite
type InviteInfo struct {
	WorkspaceID   uint   `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Email         string `json:"email"`
	Role          Role   `json:"role"`
}

// GetInviteInfo returns public info about an invite token (for the invite page)
func (s *Store) GetInviteInfo(ctx context.Context, token string) (*InviteInfo, error) {
	m, err := s.GetMemberByInviteToken(ctx, token)
	if err != nil {
		return nil, err
	}

	// Get workspace name
	var ws Workspace
	if err := s.db.WithContext(ctx).First(&ws, "id = ?", m.WorkspaceID).Error; err != nil {
		return nil, err
	}

	return &InviteInfo{
		WorkspaceID:   m.WorkspaceID,
		WorkspaceName: ws.Name,
		Email:         m.Email,
		Role:          m.Role,
	}, nil
}

// CompleteInviteWithToken completes an invite by linking to a user
func (s *Store) CompleteInviteWithToken(ctx context.Context, token string, userID uint) (*Member, error) {
	m, err := s.GetMemberByInviteToken(ctx, token)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	updates := map[string]any{
		"user_id":             userID,
		"accepted_at":         now,
		"invite_token":        "", // clear token
		"invite_token_expiry": nil,
	}

	if err := s.db.WithContext(ctx).Model(m).Updates(updates).Error; err != nil {
		if strings.Contains(err.Error(), "unique") {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}

	m.UserID = userID
	m.AcceptedAt = &now
	m.InviteToken = ""
	m.InviteTokenExpiry = nil

	return m, nil
}

// MarkInviteEmailSent marks that the invite email was sent
func (s *Store) MarkInviteEmailSent(ctx context.Context, memberID uint) error {
	return s.db.WithContext(ctx).
		Model(&Member{}).
		Where("id = ?", memberID).
		Update("invite_email_sent", true).Error
}
