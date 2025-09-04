package users

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// -----------------------------------------------------------------------------
// Models (Users + Sessions) — users-only, no agents
// -----------------------------------------------------------------------------

type Session struct {
	UserID    uint           `json:"user_id" gorm:"column:user_id;index;not null"`
	SessionID uint           `json:"session_id" gorm:"primaryKey;autoIncrement"`
	Expiry    time.Time      `json:"expiry" gorm:"column:expiry;index;not null"`
	Created   time.Time      `json:"created" gorm:"column:created;index;not null"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Session) TableName() string { return "sessions" }

// -----------------------------------------------------------------------------
// Errors
// -----------------------------------------------------------------------------

var (
	ErrInvalidToken    = errors.New("invalid token")
	ErrExpiredToken    = errors.New("expired token")
	ErrSessionNotFound = errors.New("session not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrBadPassword     = errors.New("incorrect password")
	ErrDuplicateEmail  = errors.New("email already in use")
)

// -----------------------------------------------------------------------------
// JWT (users only)
// -----------------------------------------------------------------------------

type Claims struct {
	SessionID uint `json:"sid"`
	UserID    uint `json:"uid"`
	jwt.RegisteredClaims
}

func signingKey() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte("dev-secret-change-me")
}

func SignUserToken(sessionID, userID uint, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := &Claims{
		SessionID: sessionID,
		UserID:    userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(signingKey())
}

func ParseUserToken(tokStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokStr, &Claims{}, func(*jwt.Token) (interface{}, error) {
		return signingKey(), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, ErrInvalidToken
	}
	if claims.ExpiresAt != nil && time.Now().After(claims.ExpiresAt.Time) {
		return nil, ErrExpiredToken
	}
	return claims, nil
}

// -----------------------------------------------------------------------------
// Simple, repo-less API (users-only)
//   All functions accept *gorm.DB and act directly.
// -----------------------------------------------------------------------------

type RegisterInput struct {
	Email    string         `json:"email"`
	Password string         `json:"password"`
	Name     string         `json:"name"`
	Role     string         `json:"role"` // optional; defaults to USER
	Labels   datatypes.JSON `json:"labels"`
	Metadata datatypes.JSON `json:"metadata"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterUser creates a user and an initial session, returning a JWT.
func RegisterUser(ctx context.Context, db *gorm.DB, in RegisterInput, ip string) (token string, u *User, sess *Session, err error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || strings.TrimSpace(in.Password) == "" {
		return "", nil, nil, errors.New("email and password are required")
	}

	var exists int64
	if err = db.WithContext(ctx).Model(&User{}).Where("email = ?", email).Count(&exists).Error; err != nil {
		return "", nil, nil, err
	}
	if exists > 0 {
		return "", nil, nil, ErrDuplicateEmail
	}

	pwHash, err := hashPassword(in.Password)
	if err != nil {
		return "", nil, nil, err
	}

	u = &User{
		Email:        email,
		PasswordHash: pwHash,
		Name:         strings.TrimSpace(in.Name),
		Role:         strings.TrimSpace(in.Role),
		Labels:       coalesceJSON(in.Labels),
		Metadata:     coalesceJSON(in.Metadata),
	}
	if u.Role == "" {
		u.Role = "USER"
	}

	if err = db.WithContext(ctx).Create(u).Error; err != nil {
		// best-effort duplicate detection
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return "", nil, nil, ErrDuplicateEmail
		}
		return "", nil, nil, err
	}

	sess, err = CreateUserSession(ctx, db, u.ID, 24*time.Hour)
	if err != nil {
		return "", nil, nil, err
	}

	token, err = SignUserToken(sess.SessionID, u.ID, time.Until(sess.Expiry))
	if err != nil {
		return "", nil, nil, err
	}

	// best-effort login timestamp
	_ = db.WithContext(ctx).Model(&User{}).Where("id = ?", u.ID).Update("last_login_at", time.Now()).Error

	return token, u, sess, nil
}

// LoginUser verifies password, creates a session, and returns a JWT.
func LoginUser(ctx context.Context, db *gorm.DB, in LoginInput, ip string) (token string, u *User, sess *Session, err error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || strings.TrimSpace(in.Password) == "" {
		return "", nil, nil, errors.New("email and password are required")
	}

	u = &User{}
	if err = db.WithContext(ctx).Where("email = ?", email).First(u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, nil, ErrUserNotFound
		}
		return "", nil, nil, err
	}

	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		return "", nil, nil, ErrBadPassword
	}

	sess, err = CreateUserSession(ctx, db, u.ID, 24*time.Hour)
	if err != nil {
		return "", nil, nil, err
	}

	token, err = SignUserToken(sess.SessionID, u.ID, time.Until(sess.Expiry))
	if err != nil {
		return "", nil, nil, err
	}

	_ = db.WithContext(ctx).Model(&User{}).Where("id = ?", u.ID).Update("last_login_at", time.Now()).Error
	return token, u, sess, nil
}

// CreateUserSession creates a session row for a user.
func CreateUserSession(ctx context.Context, db *gorm.DB, userID uint, ttl time.Duration) (*Session, error) {
	now := time.Now()
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	s := &Session{
		UserID:  userID,
		Created: now,
		Expiry:  now.Add(ttl),
	}
	if err := db.WithContext(ctx).Create(s).Error; err != nil {
		return nil, err
	}
	return s, nil
}

// GetSession fetches a session by SessionID (and confirms it’s not expired).
func GetSession(ctx context.Context, db *gorm.DB, sessionID uint) (*Session, error) {
	var out Session
	if err := db.WithContext(ctx).First(&out, "session_id = ?", sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	if time.Now().After(out.Expiry) {
		return nil, ErrExpiredToken
	}
	return &out, nil
}

// UpdateWSConn updates the WebSocket connection identifier for a session.
func UpdateWSConn(ctx context.Context, db *gorm.DB, sessionID uint, ws string) error {
	res := db.WithContext(ctx).Model(&Session{}).
		Where("session_id = ?", sessionID).
		Update("ws_conn", ws)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// GetSessionFromWSConn fetches a non-expired session by its WSConn value.
func GetSessionFromWSConn(ctx context.Context, db *gorm.DB, ws string) (*Session, error) {
	var out Session
	err := db.WithContext(ctx).
		Where("ws_conn = ?", ws).
		First(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(out.Expiry) {
		return nil, ErrExpiredToken
	}
	return &out, nil
}

// DeleteSession deletes a session by ID.
func DeleteSession(ctx context.Context, db *gorm.DB, sessionID uint) error {
	res := db.WithContext(ctx).Delete(&Session{}, "session_id = ?", sessionID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// GetUserFromToken parses a JWT, verifies the session, then fetches the user.
func GetUserFromToken(ctx context.Context, db *gorm.DB, token string) (*User, *Session, error) {
	claims, err := ParseUserToken(token)
	if err != nil {
		return nil, nil, err
	}
	sess, err := GetSession(ctx, db, claims.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if sess.UserID != claims.UserID {
		return nil, nil, errors.New("token/session user mismatch")
	}

	var u User
	if err := db.WithContext(ctx).First(&u, "id = ?", sess.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrUserNotFound
		}
		return nil, nil, err
	}
	return &u, sess, nil
}
