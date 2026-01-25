// internal/email/queue.go
package email

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// EmailStatus represents the status of an email in the queue
type EmailStatus string

const (
	StatusPending    EmailStatus = "pending"
	StatusProcessing EmailStatus = "processing"
	StatusSent       EmailStatus = "sent"
	StatusFailed     EmailStatus = "failed"
)

// EmailType represents the type of email
type EmailType string

const (
	TypeInvite                   EmailType = "invite"
	TypeRegistrationConfirmation EmailType = "registration_confirmation"
	TypePasswordReset            EmailType = "password_reset"
	TypeEmailVerification        EmailType = "email_verification"
)

// EmailQueue represents an email in the queue
type EmailQueue struct {
	ID          uint        `gorm:"primaryKey" json:"id"`
	Type        EmailType   `gorm:"size:50;not null;index" json:"type"`
	ToEmail     string      `gorm:"size:255;not null;index" json:"to_email"`
	ToName      string      `gorm:"size:255" json:"to_name"`
	Subject     string      `gorm:"size:500;not null" json:"subject"`
	Body        string      `gorm:"type:text" json:"body"`
	BodyHTML    string      `gorm:"type:text" json:"body_html"`
	Status      EmailStatus `gorm:"size:20;not null;default:pending;index" json:"status"`
	Attempts    int         `gorm:"default:0" json:"attempts"`
	MaxAttempts int         `gorm:"default:3" json:"max_attempts"`
	LastError   string      `gorm:"type:text" json:"last_error,omitempty"`
	ScheduledAt *time.Time  `gorm:"index" json:"scheduled_at,omitempty"`
	ProcessedAt *time.Time  `json:"processed_at,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`

	// Optional metadata for tracking
	WorkspaceID *uint  `gorm:"index" json:"workspace_id,omitempty"`
	RelatedID   *uint  `json:"related_id,omitempty"` // e.g., member ID for invites
	RelatedType string `gorm:"size:50" json:"related_type,omitempty"`
}

func (EmailQueue) TableName() string { return "email_queue" }

// QueueStore handles email queue operations
type QueueStore struct {
	db *gorm.DB
}

// NewQueueStore creates a new email queue store
func NewQueueStore(db *gorm.DB) *QueueStore {
	return &QueueStore{db: db}
}

// AutoMigrate creates the email_queue table
func (s *QueueStore) AutoMigrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(&EmailQueue{})
}

// Enqueue adds an email to the queue
func (s *QueueStore) Enqueue(ctx context.Context, email *EmailQueue) error {
	if email.Status == "" {
		email.Status = StatusPending
	}
	if email.MaxAttempts == 0 {
		email.MaxAttempts = 3
	}
	return s.db.WithContext(ctx).Create(email).Error
}

// EnqueueInvite queues an invite email
func (s *QueueStore) EnqueueInvite(ctx context.Context, toEmail, toName, inviteToken, workspaceName string, workspaceID, memberID uint) error {
	vars := TemplateVars{
		ToEmail:       toEmail,
		ToName:        toName,
		WorkspaceID:   workspaceID,
		WorkspaceName: workspaceName,
		PanelEndpoint: GetPanelEndpoint(),
		InviteToken:   inviteToken,
		ActionURL:     GetPanelEndpoint() + "/invite/" + inviteToken,
	}

	subject, body, bodyHTML := DefaultInviteTemplate.Render(vars)

	return s.Enqueue(ctx, &EmailQueue{
		Type:        TypeInvite,
		ToEmail:     toEmail,
		ToName:      toName,
		Subject:     subject,
		Body:        body,
		BodyHTML:    bodyHTML,
		WorkspaceID: &workspaceID,
		RelatedID:   &memberID,
		RelatedType: "member",
	})
}

// EnqueueRegistrationConfirmation queues a registration confirmation email
func (s *QueueStore) EnqueueRegistrationConfirmation(ctx context.Context, toEmail, toName string) error {
	vars := TemplateVars{
		ToEmail:       toEmail,
		ToName:        toName,
		PanelEndpoint: GetPanelEndpoint(),
	}

	subject, body, bodyHTML := DefaultRegistrationTemplate.Render(vars)

	return s.Enqueue(ctx, &EmailQueue{
		Type:     TypeRegistrationConfirmation,
		ToEmail:  toEmail,
		ToName:   toName,
		Subject:  subject,
		Body:     body,
		BodyHTML: bodyHTML,
	})
}

// EnqueuePasswordReset queues a password reset email
func (s *QueueStore) EnqueuePasswordReset(ctx context.Context, toEmail, toName, resetToken string, userID uint) error {
	vars := TemplateVars{
		ToEmail:       toEmail,
		ToName:        toName,
		PanelEndpoint: GetPanelEndpoint(),
		ResetToken:    resetToken,
		ActionURL:     GetPanelEndpoint() + "/auth/reset/" + resetToken,
	}

	subject, body, bodyHTML := DefaultPasswordResetTemplate.Render(vars)

	return s.Enqueue(ctx, &EmailQueue{
		Type:        TypePasswordReset,
		ToEmail:     toEmail,
		ToName:      toName,
		Subject:     subject,
		Body:        body,
		BodyHTML:    bodyHTML,
		RelatedID:   &userID,
		RelatedType: "user",
	})
}

// EnqueueEmailVerification queues an email verification email
func (s *QueueStore) EnqueueEmailVerification(ctx context.Context, toEmail, toName, verificationToken string, userID uint) error {
	vars := TemplateVars{
		ToEmail:       toEmail,
		ToName:        toName,
		PanelEndpoint: GetPanelEndpoint(),
		ActionURL:     GetPanelEndpoint() + "/auth/verify-email/" + verificationToken,
	}

	subject, body, bodyHTML := DefaultEmailVerificationTemplate.Render(vars)

	return s.Enqueue(ctx, &EmailQueue{
		Type:        TypeEmailVerification,
		ToEmail:     toEmail,
		ToName:      toName,
		Subject:     subject,
		Body:        body,
		BodyHTML:    bodyHTML,
		RelatedID:   &userID,
		RelatedType: "user",
	})
}

// GetPendingEmails retrieves pending emails for processing
func (s *QueueStore) GetPendingEmails(ctx context.Context, limit int) ([]EmailQueue, error) {
	var emails []EmailQueue
	now := time.Now()

	err := s.db.WithContext(ctx).
		Where("status = ?", StatusPending).
		Where("attempts < max_attempts").
		Where("scheduled_at IS NULL OR scheduled_at <= ?", now).
		Order("created_at ASC").
		Limit(limit).
		Find(&emails).Error

	return emails, err
}

// MarkProcessing marks an email as processing
func (s *QueueStore) MarkProcessing(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).
		Model(&EmailQueue{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     StatusProcessing,
			"updated_at": time.Now(),
		}).Error
}

// MarkSent marks an email as sent
func (s *QueueStore) MarkSent(ctx context.Context, id uint) error {
	now := time.Now()
	return s.db.WithContext(ctx).
		Model(&EmailQueue{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       StatusSent,
			"processed_at": now,
			"updated_at":   now,
		}).Error
}

// MarkFailed marks an email as failed with error
func (s *QueueStore) MarkFailed(ctx context.Context, id uint, err error) error {
	now := time.Now()
	updates := map[string]any{
		"attempts":   gorm.Expr("attempts + 1"),
		"last_error": err.Error(),
		"updated_at": now,
	}

	// Check if we've exhausted attempts
	var email EmailQueue
	if dbErr := s.db.WithContext(ctx).First(&email, id).Error; dbErr == nil {
		if email.Attempts+1 >= email.MaxAttempts {
			updates["status"] = StatusFailed
		} else {
			updates["status"] = StatusPending // Reset to pending for retry
		}
	}

	return s.db.WithContext(ctx).
		Model(&EmailQueue{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// nameGreeting formats name for greeting
func nameGreeting(name string) string {
	if name != "" {
		return " " + name
	}
	return ""
}
