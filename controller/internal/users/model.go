package users

import (
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusSuspended Status = "SUSPENDED"
	StatusInvited   Status = "INVITED"
)

// User is the account identity. Email is unique (case-insensitive enforced in code).
type User struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"column:created_at;index" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"column:updated_at;index" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Email       string `gorm:"column:email;size:320;uniqueIndex:ux_users_email" json:"email"`
	FirstName   string `gorm:"column:first_name;size:255" json:"firstName"`
	LastName    string `gorm:"column:last_name;size:255" json:"lastName"`
	Company     string `gorm:"column:company;size:255" json:"company"`
	PhoneNumber string `gorm:"column:phone_number;size:64" json:"phoneNumber"`

	// Access & security
	Admin      bool   `gorm:"column:admin;default:false" json:"admin"`
	Role       string `gorm:"column:role;size:64" json:"role"`          // app-defined (e.g., "user", "manager")
	Password   string `gorm:"column:password;size:255" json:"password"` // bcrypt hash
	Verified   bool   `gorm:"column:verified;default:false" json:"verified"`
	MFAEnabled bool   `gorm:"column:mfa_enabled;default:false" json:"mfaEnabled"`

	// State & telemetry
	Status      Status    `gorm:"column:status;type:varchar(16);default:'ACTIVE';index" json:"status"`
	LastLoginAt time.Time `gorm:"column:last_login_at" json:"lastLoginAt"`
	TZ          string    `gorm:"column:timezone;size:64" json:"timezone"`
	AvatarURL   string    `gorm:"column:avatar_url;size:512" json:"avatarUrl"`

	// Free-form data
	Labels   datatypes.JSON `gorm:"column:labels;type:jsonb" json:"labels"`
	Metadata datatypes.JSON `gorm:"column:metadata;type:jsonb" json:"metadata"`
}

func (User) TableName() string { return "users" }

// Normalize email & defaults
func (u *User) BeforeCreate(tx *gorm.DB) error {
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	if u.Status == "" {
		u.Status = StatusActive
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
