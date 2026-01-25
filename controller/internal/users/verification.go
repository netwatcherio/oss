package users

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// TokenType represents the type of user token
type TokenType string

const (
	TokenTypeEmailVerification TokenType = "email_verification"
	TokenTypePasswordReset     TokenType = "password_reset"
)

// UserToken stores verification and password reset tokens
type UserToken struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"index;not null"`
	Token     string    `gorm:"uniqueIndex;size:255;not null"`
	Type      TokenType `gorm:"size:50;not null;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time
}

func (UserToken) TableName() string { return "user_tokens" }

// Token errors
var (
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenExpired  = errors.New("token expired")
	ErrTokenInvalid  = errors.New("invalid token")
)

// generateSecureToken creates a cryptographically secure random token
func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	// Use URL-safe base64 encoding
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// CreateToken generates and stores a new token for a user
func CreateToken(ctx context.Context, db *gorm.DB, userID uint, tokenType TokenType, expiryHours int) (*UserToken, error) {
	// Delete any existing tokens of the same type for this user
	if err := db.WithContext(ctx).
		Where("user_id = ? AND type = ?", userID, tokenType).
		Delete(&UserToken{}).Error; err != nil {
		return nil, err
	}

	// Generate secure token (32 bytes = 256 bits)
	tokenStr, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}

	token := &UserToken{
		UserID:    userID,
		Token:     tokenStr,
		Type:      tokenType,
		ExpiresAt: time.Now().Add(time.Duration(expiryHours) * time.Hour),
	}

	if err := db.WithContext(ctx).Create(token).Error; err != nil {
		return nil, err
	}

	return token, nil
}

// ValidateToken checks if a token is valid and not expired, returns the user ID
func ValidateToken(ctx context.Context, db *gorm.DB, tokenStr string, tokenType TokenType) (uint, error) {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return 0, ErrTokenInvalid
	}

	var token UserToken
	if err := db.WithContext(ctx).
		Where("token = ? AND type = ?", tokenStr, tokenType).
		First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrTokenNotFound
		}
		return 0, err
	}

	if time.Now().After(token.ExpiresAt) {
		// Clean up expired token
		_ = db.WithContext(ctx).Delete(&token)
		return 0, ErrTokenExpired
	}

	return token.UserID, nil
}

// ConsumeToken validates and deletes a token (one-time use)
func ConsumeToken(ctx context.Context, db *gorm.DB, tokenStr string, tokenType TokenType) (uint, error) {
	userID, err := ValidateToken(ctx, db, tokenStr, tokenType)
	if err != nil {
		return 0, err
	}

	// Delete the token after successful validation
	if err := db.WithContext(ctx).
		Where("token = ? AND type = ?", tokenStr, tokenType).
		Delete(&UserToken{}).Error; err != nil {
		return 0, err
	}

	return userID, nil
}

// GetPendingVerification checks if a user has a pending email verification
func GetPendingVerification(ctx context.Context, db *gorm.DB, userID uint) (*UserToken, error) {
	var token UserToken
	err := db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND expires_at > ?", userID, TokenTypeEmailVerification, time.Now()).
		First(&token).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// CleanupExpiredTokens removes all expired tokens (for periodic cleanup)
func CleanupExpiredTokens(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&UserToken{}).Error
}

// GetEmailVerificationExpiryHours returns configured expiry hours for email verification
func GetEmailVerificationExpiryHours() int {
	if v := os.Getenv("EMAIL_VERIFICATION_EXPIRY_HOURS"); v != "" {
		if hours, err := strconv.Atoi(v); err == nil && hours > 0 {
			return hours
		}
	}
	return 24 // default: 24 hours
}

// GetPasswordResetExpiryHours returns configured expiry hours for password reset
func GetPasswordResetExpiryHours() int {
	if v := os.Getenv("EMAIL_PASSWORD_RESET_EXPIRY_HOURS"); v != "" {
		if hours, err := strconv.Atoi(v); err == nil && hours > 0 {
			return hours
		}
	}
	return 1 // default: 1 hour
}
