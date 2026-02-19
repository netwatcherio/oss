package alert

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// AgentContext provides contextual information about the agent being evaluated
type AgentContext struct {
	AgentID     uint
	AgentName   string
	WorkspaceID uint
	LastSeenAt  time.Time
}

// EvaluateAgentOffline checks if agents are offline and creates alerts if needed.
// This should be called periodically (e.g., every minute) to check agent health.
func EvaluateAgentOffline(ctx context.Context, db *gorm.DB) error {
	// Get all enabled alert rules that use the offline metric
	var rules []AlertRule
	err := db.WithContext(ctx).
		Where("enabled = true AND metric = ?", MetricOffline).
		Find(&rules).Error
	if err != nil {
		return fmt.Errorf("failed to fetch offline alert rules: %w", err)
	}

	if len(rules) == 0 {
		return nil // No offline rules configured
	}

	// Group rules by workspace
	rulesByWorkspace := make(map[uint][]AlertRule)
	for _, rule := range rules {
		rulesByWorkspace[rule.WorkspaceID] = append(rulesByWorkspace[rule.WorkspaceID], rule)
	}

	now := time.Now()

	// Process each workspace
	for workspaceID, wsRules := range rulesByWorkspace {
		// Get all agents in this workspace
		type agentRow struct {
			ID         uint
			Name       string
			LastSeenAt time.Time
		}
		var agents []agentRow
		err := db.WithContext(ctx).
			Table("agents").
			Select("id, name, last_seen_at").
			Where("workspace_id = ? AND deleted_at IS NULL", workspaceID).
			Find(&agents).Error
		if err != nil {
			log.Warnf("alert.EvaluateAgentOffline: failed to fetch agents for workspace %d: %v", workspaceID, err)
			continue
		}

		// Evaluate each agent against each rule
		for _, agent := range agents {
			// Calculate minutes since last seen
			minutesSinceLastSeen := float64(0)
			if !agent.LastSeenAt.IsZero() {
				minutesSinceLastSeen = now.Sub(agent.LastSeenAt).Minutes()
			} else {
				// Never seen - treat as very long offline
				minutesSinceLastSeen = 999999
			}

			for _, rule := range wsRules {
				// Skip if rule is for a specific agent and this isn't it
				if rule.AgentID != nil && *rule.AgentID != agent.ID {
					continue
				}

				triggered := ShouldTrigger(rule.Operator, minutesSinceLastSeen, rule.Threshold)

				if triggered {
					// Check if there's already an active alert for this rule + agent
					var existing Alert
					err := db.WithContext(ctx).
						Where("alert_rule_id = ? AND agent_id = ? AND status = ?", rule.ID, agent.ID, StatusActive).
						First(&existing).Error

					if err == nil {
						// Active alert already exists, skip
						continue
					}

					// Create new alert
					message := fmt.Sprintf("Agent offline for %.0f minutes (threshold: %.0f min)",
						minutesSinceLastSeen, rule.Threshold)

					actx := &AlertContext{
						AgentID:   agent.ID,
						AgentName: agent.Name,
					}

					alertInstance, err := CreateAlert(ctx, db, &rule, minutesSinceLastSeen, message, actx)
					if err != nil {
						log.Errorf("alert.EvaluateAgentOffline: failed to create alert: %v", err)
						continue
					}

					// Dispatch notifications
					go DispatchNotifications(ctx, db, &rule, alertInstance)

					log.Infof("Agent offline alert triggered: rule=%d, agent=%d (%s), offline_minutes=%.0f, threshold=%.0f",
						rule.ID, agent.ID, agent.Name, minutesSinceLastSeen, rule.Threshold)
				} else {
					// Check if we should auto-resolve an active alert (agent came back online)
					var activeAlert Alert
					err := db.WithContext(ctx).
						Where("alert_rule_id = ? AND agent_id = ? AND status = ?", rule.ID, agent.ID, StatusActive).
						First(&activeAlert).Error

					if err == nil {
						// Has active alert but agent is now online - resolve it
						if err := ResolveAlert(ctx, db, activeAlert.ID); err != nil {
							log.Warnf("alert.EvaluateAgentOffline: failed to auto-resolve alert %d: %v", activeAlert.ID, err)
						} else {
							log.Infof("Agent offline alert auto-resolved: id=%d, rule=%d, agent=%d (%s)",
								activeAlert.ID, rule.ID, agent.ID, agent.Name)
						}
					}
				}
			}
		}
	}

	return nil
}
