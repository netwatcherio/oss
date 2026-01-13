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
)

// NotificationPayload is the webhook payload structure
type NotificationPayload struct {
	AlertID     uint      `json:"alert_id"`
	WorkspaceID uint      `json:"workspace_id"`
	ProbeID     *uint     `json:"probe_id,omitempty"`
	AgentID     *uint     `json:"agent_id,omitempty"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Severity    string    `json:"severity"`
	Message     string    `json:"message"`
	TriggeredAt time.Time `json:"triggered_at"`
}

// DispatchNotifications sends notifications through all configured channels
func DispatchNotifications(ctx context.Context, db *gorm.DB, rule *AlertRule, alertInstance *Alert) {
	payload := NotificationPayload{
		AlertID:     alertInstance.ID,
		WorkspaceID: alertInstance.WorkspaceID,
		ProbeID:     alertInstance.ProbeID,
		AgentID:     alertInstance.AgentID,
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

	// Email notification (TODO: integrate with existing email queue)
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
// TODO: Integrate with the existing email queue system
func sendEmailNotification(ctx context.Context, db *gorm.DB, rule *AlertRule, alertInstance *Alert) {
	// For now, just log that we would send an email
	// Full implementation would:
	// 1. Get workspace members with email notifications enabled
	// 2. Queue emails via the email.QueueStore

	log.Infof("alert.sendEmailNotification: would send email for alert %d (workspace %d)",
		alertInstance.ID, alertInstance.WorkspaceID)

	// Future implementation:
	// members := workspace.GetMembersWithEmailAlerts(ctx, db, rule.WorkspaceID)
	// for _, member := range members {
	//     emailStore.Queue(email.AlertNotification{
	//         To: member.Email,
	//         Alert: alertInstance,
	//     })
	// }
}
