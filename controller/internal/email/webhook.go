// internal/email/webhook.go
package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// WebhookPayload is the payload sent to webhook endpoints
type WebhookPayload struct {
	// Email type
	Type EmailType `json:"type"`

	// Recipient
	ToEmail string `json:"to_email"`
	ToName  string `json:"to_name"`

	// Rendered content (for convenience)
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	BodyHTML string `json:"body_html"`

	// All template variables for custom processing
	Variables TemplateVars `json:"variables"`

	// Metadata
	QueueID     uint   `json:"queue_id"`
	WorkspaceID *uint  `json:"workspace_id,omitempty"`
	RelatedID   *uint  `json:"related_id,omitempty"`
	RelatedType string `json:"related_type,omitempty"`
	Timestamp   string `json:"timestamp"`
}

// WebhookConfig holds webhook configuration
type WebhookConfig struct {
	Enabled   bool
	URL       string
	AuthToken string // Optional authorization header
	Timeout   time.Duration
}

// LoadWebhookConfigFromEnv loads webhook configuration from environment
func LoadWebhookConfigFromEnv() *WebhookConfig {
	url := os.Getenv("EMAIL_WEBHOOK_URL")
	if url == "" {
		return &WebhookConfig{Enabled: false}
	}

	timeout := 30 * time.Second
	if t := os.Getenv("EMAIL_WEBHOOK_TIMEOUT"); t != "" {
		if d, err := time.ParseDuration(t); err == nil {
			timeout = d
		}
	}

	return &WebhookConfig{
		Enabled:   true,
		URL:       url,
		AuthToken: os.Getenv("EMAIL_WEBHOOK_AUTH_TOKEN"),
		Timeout:   timeout,
	}
}

// WebhookSender sends emails via webhook
type WebhookSender struct {
	config *WebhookConfig
	client *http.Client
}

// NewWebhookSender creates a new webhook sender
func NewWebhookSender(config *WebhookConfig) *WebhookSender {
	return &WebhookSender{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Send sends an email via webhook
func (w *WebhookSender) Send(email *EmailQueue, vars TemplateVars) error {
	if !w.config.Enabled {
		return fmt.Errorf("webhook not enabled")
	}

	payload := WebhookPayload{
		Type:        email.Type,
		ToEmail:     email.ToEmail,
		ToName:      email.ToName,
		Subject:     email.Subject,
		Body:        email.Body,
		BodyHTML:    email.BodyHTML,
		Variables:   vars,
		QueueID:     email.ID,
		WorkspaceID: email.WorkspaceID,
		RelatedID:   email.RelatedID,
		RelatedType: email.RelatedType,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", w.config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NetWatcher-EmailWebhook/1.0")

	if w.config.AuthToken != "" {
		// Support both "Bearer token" and plain "token"
		token := w.config.AuthToken
		if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = "Bearer " + token
		}
		req.Header.Set("Authorization", token)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}
