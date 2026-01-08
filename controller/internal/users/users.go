package users

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// -----------------------------
// Model & constants
// -----------------------------

type User struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	Email        string         `json:"email" gorm:"uniqueIndex;size:255;not null"`
	PasswordHash string         `json:"-" gorm:"column:password_hash;size:255;not null"`
	Name         string         `json:"name" gorm:"size:255"`
	Role         string         `json:"role" gorm:"size:64;default:USER"`
	Verified     bool           `json:"verified" gorm:"default:false"`
	Labels       datatypes.JSON `json:"labels" gorm:"type:json"`
	Metadata     datatypes.JSON `json:"metadata" gorm:"type:json"`
	LastLoginAt  *time.Time     `json:"last_login_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	if u.Role == "" {
		u.Role = "USER"
	}
	if len(u.Labels) == 0 {
		u.Labels = datatypes.JSON([]byte(`{}`))
	}
	if len(u.Metadata) == 0 {
		u.Metadata = datatypes.JSON([]byte(`{}`))
	}
	return nil
}

func (u *User) BeforeUpdate(tx *gorm.DB) error {
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	return nil
}

// -----------------------------
// Errors
// -----------------------------

var (
	ErrNotFound = errors.New("user not found")
)

// -----------------------------
// Inputs
// -----------------------------

type UpdateProfileInput struct {
	Email    *string         `json:"email,omitempty"`
	Name     *string         `json:"name,omitempty"`
	Labels   *datatypes.JSON `json:"labels,omitempty"`
	Metadata *datatypes.JSON `json:"metadata,omitempty"`
}

type ChangePasswordInput struct {
	OldPassword string `json:"oldPassword,omitempty"` // optional: if provided, verify
	NewPassword string `json:"newPassword"`
}

// -----------------------------
// Helpers
// -----------------------------

func coalesceJSON(j datatypes.JSON) datatypes.JSON {
	if len(j) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return j
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// -----------------------------
// CRUD-like operations (no repo, no service)
// -----------------------------

// Register creates a new user after ensuring the email is unique.
func Register(ctx context.Context, db *gorm.DB, in RegisterInput) (*User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || strings.TrimSpace(in.Password) == "" {
		return nil, errors.New("email and password are required")
	}

	var count int64
	if err := db.WithContext(ctx).
		Model(&User{}).
		Where("email = ?", email).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrDuplicateEmail
	}

	pwHash, err := hashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	user := &User{
		Email:        email,
		PasswordHash: pwHash,
		Name:         strings.TrimSpace(in.Name),
		Role:         strings.TrimSpace(in.Role),
		Labels:       coalesceJSON(in.Labels),
		Metadata:     coalesceJSON(in.Metadata),
	}

	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		// Handle unique constraint race
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrDuplicateEmail
		}
		return nil, err
	}

	return user, nil
}

// Get fetches a user by ID.
func Get(ctx context.Context, db *gorm.DB, id uint) (*User, error) {
	var u User
	if err := db.WithContext(ctx).First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// GetByEmail fetches a user by email.
func GetByEmail(ctx context.Context, db *gorm.DB, email string) (*User, error) {
	var u User
	if err := db.WithContext(ctx).
		Where("email = ?", strings.ToLower(strings.TrimSpace(email))).
		First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// List returns a page of users and the total count. q searches email and name (case-insensitive).
func List(ctx context.Context, db *gorm.DB, limit, offset int, q string) ([]User, int64, error) {
	limit = clamp(limit, 1, 1000)
	offset = max(offset, 0)

	q = strings.TrimSpace(q)
	var (
		users []User
		count int64
		tx    = db.WithContext(ctx).Model(&User{})
	)

	if q != "" {
		qLike := "%" + strings.ToLower(q) + "%"
		tx = tx.Where("LOWER(email) LIKE ? OR LOWER(name) LIKE ?", qLike, qLike)
	}

	if err := tx.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("id DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, count, nil
}

// UpdateProfile updates basic profile fields. Email uniqueness is enforced.
func UpdateProfile(ctx context.Context, db *gorm.DB, id uint, in UpdateProfileInput) error {
	updates := map[string]any{}
	if in.Email != nil {
		newEmail := strings.ToLower(strings.TrimSpace(*in.Email))
		if newEmail == "" {
			return errors.New("email cannot be empty")
		}
		// ensure unique
		var count int64
		if err := db.WithContext(ctx).
			Model(&User{}).
			Where("email = ? AND id <> ?", newEmail, id).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrDuplicateEmail
		}
		updates["email"] = newEmail
	}
	if in.Name != nil {
		updates["name"] = strings.TrimSpace(*in.Name)
	}
	if in.Labels != nil {
		updates["labels"] = coalesceJSON(*in.Labels)
	}
	if in.Metadata != nil {
		updates["metadata"] = coalesceJSON(*in.Metadata)
	}

	if len(updates) == 0 {
		return nil
	}
	res := db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ChangePassword changes a user's password. If OldPassword is provided, verify it first.
func ChangePassword(ctx context.Context, db *gorm.DB, id uint, in ChangePasswordInput) error {
	if strings.TrimSpace(in.NewPassword) == "" {
		return errors.New("new password cannot be empty")
	}

	var u User
	if err := db.WithContext(ctx).First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	if in.OldPassword != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.OldPassword)); err != nil {
			return ErrBadPassword
		}
	}

	newHash, err := hashPassword(in.NewPassword)
	if err != nil {
		return err
	}

	res := db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("password_hash", newHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRole sets a user's role.
func SetRole(ctx context.Context, db *gorm.DB, id uint, role string) error {
	role = strings.TrimSpace(role)
	if role == "" {
		return errors.New("role cannot be empty")
	}
	res := db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("role", role)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkVerified sets verified=true.
func MarkVerified(ctx context.Context, db *gorm.DB, id uint) error {
	res := db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("verified", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordLogin updates the user's last_login_at timestamp.
func RecordLogin(ctx context.Context, db *gorm.DB, id uint, when time.Time) error {
	res := db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("last_login_at", when)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a user by ID.
func Delete(ctx context.Context, db *gorm.DB, id uint) error {
	res := db.WithContext(ctx).Delete(&User{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// -----------------------------
// Invite-related functions
// -----------------------------

// IsPendingUser checks if a user is in pending state (no password set)
func IsPendingUser(u *User) bool {
	return u.PasswordHash == ""
}

// CreatePendingUser creates a user without a password (invited state)
// This user cannot login until they complete registration
func CreatePendingUser(ctx context.Context, db *gorm.DB, email, name string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errors.New("email is required")
	}

	// Check if user already exists
	var existing User
	if err := db.WithContext(ctx).Where("email = ?", email).First(&existing).Error; err == nil {
		// User exists - return them
		return &existing, nil
	}

	user := &User{
		Email:        email,
		PasswordHash: "", // empty = pending
		Name:         strings.TrimSpace(name),
		Role:         "USER",
		Labels:       coalesceJSON(nil),
		Metadata:     coalesceJSON(nil),
	}

	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		// Handle unique constraint race
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique") {
			// Try to get the existing user
			if err := db.WithContext(ctx).Where("email = ?", email).First(&existing).Error; err == nil {
				return &existing, nil
			}
			return nil, ErrDuplicateEmail
		}
		return nil, err
	}

	return user, nil
}

// GetOrCreatePendingUser gets an existing user by email or creates a pending one
func GetOrCreatePendingUser(ctx context.Context, db *gorm.DB, email, name string) (*User, bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, false, errors.New("email is required")
	}

	// Try to get existing user
	var existing User
	if err := db.WithContext(ctx).Where("email = ?", email).First(&existing).Error; err == nil {
		return &existing, false, nil // false = not created
	}

	// Create pending user
	user, err := CreatePendingUser(ctx, db, email, name)
	if err != nil {
		return nil, false, err
	}
	return user, true, nil // true = created
}

// CompleteRegistration sets the password and name for a pending user
func CompleteRegistration(ctx context.Context, db *gorm.DB, id uint, name, password string) error {
	if strings.TrimSpace(password) == "" {
		return errors.New("password cannot be empty")
	}

	var u User
	if err := db.WithContext(ctx).First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	// Only allow completing registration if user has no password
	if u.PasswordHash != "" {
		return errors.New("user already has a password set")
	}

	pwHash, err := hashPassword(password)
	if err != nil {
		return err
	}

	updates := map[string]any{
		"password_hash": pwHash,
	}
	if strings.TrimSpace(name) != "" {
		updates["name"] = strings.TrimSpace(name)
	}

	res := db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// -----------------------------
// Small util
// -----------------------------

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
