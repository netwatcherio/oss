package alert

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NotificationPayload is the webhook payload structure
type NotificationPayload struct {
	AlertID     uint      `json:"alert_id"`
	WorkspaceID uint      `json:"workspace_id"`
	ProbeID     *uint     `json:"probe_id,omitempty"`
	AgentID     *uint     `json:"agent_id,omitempty"`
	ProbeType   string    `json:"probe_type,omitempty"`
	ProbeName   string    `json:"probe_name,omitempty"`
	ProbeTarget string    `json:"probe_target,omitempty"`
	AgentName   string    `json:"agent_name,omitempty"`
	PanelURL    string    `json:"panel_url,omitempty"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Severity    string    `json:"severity"`
	Message     string    `json:"message"`
	TriggeredAt time.Time `json:"triggered_at"`
}

// buildPanelURL constructs a deep link to the relevant agent/probe page
func buildPanelURL(a *Alert) string {
	if a.ProbeID != nil && a.AgentID != nil {
		return fmt.Sprintf("/workspaces/%d/agents/%d?probe=%d", a.WorkspaceID, *a.AgentID, *a.ProbeID)
	}
	if a.AgentID != nil {
		return fmt.Sprintf("/workspaces/%d/agents/%d", a.WorkspaceID, *a.AgentID)
	}
	return fmt.Sprintf("/workspaces/%d", a.WorkspaceID)
}

// DispatchNotifications sends notifications through all configured channels
func DispatchNotifications(ctx context.Context, db *gorm.DB, rule *AlertRule, alertInstance *Alert) {
	payload := NotificationPayload{
		AlertID:     alertInstance.ID,
		WorkspaceID: alertInstance.WorkspaceID,
		ProbeID:     alertInstance.ProbeID,
		AgentID:     alertInstance.AgentID,
		ProbeType:   alertInstance.ProbeType,
		ProbeName:   alertInstance.ProbeName,
		ProbeTarget: alertInstance.ProbeTarget,
		AgentName:   alertInstance.AgentName,
		PanelURL:    buildPanelURL(alertInstance),
		Metric:      string(alertInstance.Metric),
		Value:       alertInstance.Value,
		Threshold:   alertInstance.Threshold,
		Severity:    string(alertInstance.Severity),
		Message:     alertInstance.Message,
		TriggeredAt: alertInstance.TriggeredAt,
	}

	// Panel notifications are automatic (stored in DB, fetched by frontend)
	// No additional action needed for notify_panel

	// Webhook notification
	if rule.NotifyWebhook && rule.WebhookURL != "" {
		go sendWebhookNotification(rule.WebhookURL, rule.WebhookSecret, payload)
	}

	// Email notification
	if rule.NotifyEmail {
		go sendEmailNotification(ctx, db, rule, alertInstance)
	}
}

// sendWebhookNotification sends an HTTP POST to the configured webhook URL
func sendWebhookNotification(webhookURL, secret string, payload NotificationPayload) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("alert.sendWebhookNotification: failed to marshal payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Errorf("alert.sendWebhookNotification: failed to create request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NetWatcher-Alert/1.0")

	// Add HMAC signature if secret is configured
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(jsonPayload)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-NetWatcher-Signature", fmt.Sprintf("sha256=%s", signature))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("alert.sendWebhookNotification: request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Warnf("alert.sendWebhookNotification: webhook returned status %d for URL %s",
			resp.StatusCode, webhookURL)
	} else {
		log.Infof("alert.sendWebhookNotification: webhook delivered successfully to %s", webhookURL)
	}
}

// sendEmailNotification queues an email for workspace members
func sendEmailNotification(ctx context.Context, db *gorm.DB, rule *AlertRule, alertInstance *Alert) {
	members, err := getWorkspaceMembersWithEmailAlerts(ctx, db, rule.WorkspaceID)
	if err != nil {
		log.Errorf("alert.sendEmailNotification: failed to get workspace members: %v", err)
		return
	}

	for _, member := range members {
		alertEmail := buildAlertEmailContent(rule, alertInstance)
		emailEntry := &emailQueueEntry{
			ToEmail:     member.Email,
			ToName:      member.Name,
			Subject:     alertEmail.Subject,
			Body:        alertEmail.Body,
			BodyHTML:    alertEmail.BodyHTML,
			Type:        emailTypeAlert,
			WorkspaceID: &rule.WorkspaceID,
			RelatedID:   &alertInstance.ID,
			RelatedType: "alert",
		}
		if err := queueEmailEntry(ctx, db, emailEntry); err != nil {
			log.Warnf("alert.sendEmailNotification: failed to queue email for member %s: %v", member.Email, err)
		}
	}

	log.Infof("alert.sendEmailNotification: queued %d emails for alert %d (workspace %d)",
		len(members), alertInstance.ID, alertInstance.WorkspaceID)
}

type emailType string

const (
	emailTypeAlert       emailType = "alert"
	emailTypeAlertDigest emailType = "alert_digest"
)

type memberEmailInfo struct {
	Email string
	Name  string
}

func getWorkspaceMembersWithEmailAlerts(ctx context.Context, db *gorm.DB, workspaceID uint) ([]memberEmailInfo, error) {
	var members []struct {
		Email string
		Name  string
	}
	err := db.WithContext(ctx).
		Table("workspace_members").
		Select("COALESCE(users.name, workspace_members.email) as name, workspace_members.email").
		Joins("LEFT JOIN users ON users.id = workspace_members.user_id").
		Where("workspace_members.workspace_id = ? AND workspace_members.deleted_at IS NULL", workspaceID).
		Where("workspace_members.email != ''").
		Find(&members).Error
	if err != nil {
		return nil, err
	}

	result := make([]memberEmailInfo, 0, len(members))
	for _, m := range members {
		result = append(result, memberEmailInfo{Email: m.Email, Name: m.Name})
	}
	return result, nil
}

type emailQueueEntry struct {
	ToEmail     string
	ToName      string
	Subject     string
	Body        string
	BodyHTML    string
	Type        emailType
	WorkspaceID *uint
	RelatedID   *uint
	RelatedType string
}

func queueEmailEntry(ctx context.Context, db *gorm.DB, entry *emailQueueEntry) error {
	now := time.Now()
	queueEntry := emailQueueTableEntry{
		Type:        string(entry.Type),
		ToEmail:     entry.ToEmail,
		ToName:      entry.ToName,
		Subject:     entry.Subject,
		Body:        entry.Body,
		BodyHTML:    entry.BodyHTML,
		Status:      "pending",
		Attempts:    0,
		MaxAttempts: 3,
		WorkspaceID: entry.WorkspaceID,
		RelatedID:   entry.RelatedID,
		RelatedType: entry.RelatedType,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&queueEntry).Error
}

type emailQueueTableEntry struct {
	Type        string
	ToEmail     string
	ToName      string
	Subject     string
	Body        string
	BodyHTML    string
	Status      string
	Attempts    int
	MaxAttempts int
	WorkspaceID *uint
	RelatedID   *uint
	RelatedType string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (e emailQueueTableEntry) TableName() string { return "email_queue" }

type alertEmailContent struct {
	Subject  string
	Body     string
	BodyHTML string
}

func buildAlertEmailContent(rule *AlertRule, alertInstance *Alert) alertEmailContent {
	severityLabel := "Warning"
	if alertInstance.Severity == SeverityCritical {
		severityLabel = "Critical"
	}

	metricLabel := string(alertInstance.Metric)
	panelURL := buildPanelURL(alertInstance)

	subject := fmt.Sprintf("[%s] NetWatcher Alert: %s on %s", severityLabel, metricLabel, alertInstance.ProbeName)
	body := fmt.Sprintf(`NetWatcher Alert

Severity: %s
Metric: %s
Value: %.2f (threshold: %.2f)
Probe: %s (%s)
Target: %s
Message: %s

Time: %s

View details: %s

---
NetWatcher.io - Open Source Network Monitoring`,
		severityLabel, metricLabel, alertInstance.Value, alertInstance.Threshold,
		alertInstance.ProbeName, alertInstance.ProbeType, alertInstance.ProbeTarget,
		alertInstance.Message, alertInstance.TriggeredAt.Format(time.RFC822), panelURL)

	bodyHTML := buildAlertEmailHTML(severityLabel, metricLabel, alertInstance, panelURL)

	return alertEmailContent{
		Subject:  subject,
		Body:     body,
		BodyHTML: bodyHTML,
	}
}

func buildAlertEmailHTML(severityLabel, metricLabel string, alertInstance *Alert, panelURL string) string {
	bgColor := "#f59e0b" // warning amber
	if alertInstance.Severity == SeverityCritical {
		bgColor = "#ef4444" // critical red
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>NetWatcher Alert</title>
</head>
<body style="margin:0;padding:20px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background-color:#0a0e17;">
<div style="max-width:600px;margin:0 auto;background:#151b28;border-radius:12px;overflow:hidden;border:1px solid #1e2a3a;">
<div style="background:%s;padding:16px 24px;">
<h1 style="margin:0;color:#fff;font-size:18px;font-weight:600;">%s Alert</h1>
</div>
<div style="padding:24px;color:#8b9cb3;">
<table style="width:100%%;border-collapse:collapse;">
<tr><td style="padding:8px 0;font-weight:600;color:#f0f4f8;">Metric</td><td style="padding:8px 0;">%s</td></tr>
<tr><td style="padding:8px 0;font-weight:600;color:#f0f4f8;">Value</td><td style="padding:8px 0;">%.2f (threshold: %.2f)</td></tr>
<tr><td style="padding:8px 0;font-weight:600;color:#f0f4f8;">Probe</td><td style="padding:8px 0;">%s (%s)</td></tr>
<tr><td style="padding:8px 0;font-weight:600;color:#f0f4f8;">Target</td><td style="padding:8px 0;">%s</td></tr>
<tr><td style="padding:8px 0;font-weight:600;color:#f0f4f8;">Message</td><td style="padding:8px 0;">%s</td></tr>
<tr><td style="padding:8px 0;font-weight:600;color:#f0f4f8;">Time</td><td style="padding:8px 0;">%s</td></tr>
</table>
<div style="margin-top:24px;text-align:center;">
<a href="%s" style="display:inline-block;padding:12px 32px;background:#3b82f6;color:#fff;text-decoration:none;border-radius:8px;font-weight:600;">View Details</a>
</div>
</div>
<div style="padding:16px 24px;background:#1e2a3a;text-align:center;color:#5a6a7e;font-size:12px;">
&copy; 2026 NetWatcher.io
</div>
</div>
</body>
</html>`,
		bgColor, severityLabel, metricLabel,
		alertInstance.Value, alertInstance.Threshold,
		alertInstance.ProbeName, alertInstance.ProbeType,
		alertInstance.ProbeTarget, alertInstance.Message,
		alertInstance.TriggeredAt.Format(time.RFC822), panelURL)
}
