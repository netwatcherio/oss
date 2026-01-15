package alert

import (
	"context"
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ProbeDataPayload represents the parsed payload from probe data
type ProbeDataPayload struct {
	Latency    float64 `json:"latency"`
	AvgLatency float64 `json:"avgLatency"`
	PacketLoss float64 `json:"packetLoss"`
	Jitter     float64 `json:"jitter"`
	AverageRTT float64 `json:"averageRTT"`
}

// ProbeContext provides contextual information about the probe being evaluated
type ProbeContext struct {
	ProbeID     uint
	ProbeType   string
	ProbeName   string
	ProbeTarget string
	AgentID     uint
	AgentName   string
	WorkspaceID uint
}

// EvaluateProbeData checks probe data against alert rules and creates alerts if thresholds are exceeded
func EvaluateProbeData(ctx context.Context, db *gorm.DB, pctx ProbeContext, payloadJSON []byte) error {
	// Parse payload
	var payload ProbeDataPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		log.Warnf("alert.EvaluateProbeData: failed to parse payload: %v", err)
		return nil // Don't fail silently, just log
	}

	// Get relevant alert rules for this probe
	var rules []AlertRule
	err := db.WithContext(ctx).
		Where("enabled = true AND workspace_id = ? AND (probe_id = ? OR probe_id IS NULL)", pctx.WorkspaceID, pctx.ProbeID).
		Find(&rules).Error
	if err != nil {
		return fmt.Errorf("failed to fetch alert rules: %w", err)
	}

	if len(rules) == 0 {
		return nil // No rules to evaluate
	}

	for _, rule := range rules {
		value := getMetricValue(&payload, rule.Metric, pctx.ProbeType)
		if value == nil {
			continue // Metric not applicable to this probe type
		}

		if shouldTrigger(rule.Operator, *value, rule.Threshold) {
			// Check if there's already an active alert for this rule
			var existing Alert
			err := db.WithContext(ctx).
				Where("alert_rule_id = ? AND status = ?", rule.ID, StatusActive).
				First(&existing).Error

			if err == nil {
				// Active alert already exists, skip
				continue
			}

			// Create new alert with context
			message := fmt.Sprintf("%s exceeded threshold: %.2f (threshold: %.2f)",
				rule.Metric, *value, rule.Threshold)

			actx := &AlertContext{
				ProbeID:     pctx.ProbeID,
				ProbeType:   pctx.ProbeType,
				ProbeName:   pctx.ProbeName,
				ProbeTarget: pctx.ProbeTarget,
				AgentID:     pctx.AgentID,
				AgentName:   pctx.AgentName,
			}

			alertInstance, err := CreateAlert(ctx, db, &rule, *value, message, actx)
			if err != nil {
				log.Errorf("alert.EvaluateProbeData: failed to create alert: %v", err)
				continue
			}

			// Dispatch notifications
			go DispatchNotifications(ctx, db, &rule, alertInstance)

			log.Infof("Alert triggered: rule=%d, probe=%d (%s), metric=%s, value=%.2f, threshold=%.2f",
				rule.ID, pctx.ProbeID, pctx.ProbeType, rule.Metric, *value, rule.Threshold)
		} else {
			// Check if we should auto-resolve an active alert
			var activeAlert Alert
			err := db.WithContext(ctx).
				Where("alert_rule_id = ? AND probe_id = ? AND status = ?", rule.ID, pctx.ProbeID, StatusActive).
				First(&activeAlert).Error

			if err == nil {
				// Has active alert but condition is no longer met - resolve it
				if err := ResolveAlert(ctx, db, activeAlert.ID); err != nil {
					log.Warnf("alert.EvaluateProbeData: failed to auto-resolve alert %d: %v", activeAlert.ID, err)
				} else {
					log.Infof("Alert auto-resolved: id=%d, rule=%d, probe=%d", activeAlert.ID, rule.ID, pctx.ProbeID)
				}
			}
		}
	}

	return nil
}

// getMetricValue extracts the relevant metric value from the payload
func getMetricValue(payload *ProbeDataPayload, metric Metric, probeType string) *float64 {
	switch metric {
	case MetricPacketLoss:
		if probeType == "PING" || probeType == "TRAFFICSIM" {
			return &payload.PacketLoss
		}
	case MetricLatency:
		if probeType == "PING" {
			if payload.Latency > 0 {
				return &payload.Latency
			}
			return &payload.AvgLatency
		}
		if probeType == "TRAFFICSIM" {
			return &payload.AverageRTT
		}
	case MetricJitter:
		if probeType == "PING" || probeType == "TRAFFICSIM" {
			return &payload.Jitter
		}
	}
	return nil
}

// shouldTrigger checks if the value meets the threshold condition
func shouldTrigger(op Operator, value, threshold float64) bool {
	switch op {
	case OperatorGT:
		return value > threshold
	case OperatorGTE:
		return value >= threshold
	case OperatorLT:
		return value < threshold
	case OperatorLTE:
		return value <= threshold
	case OperatorEQ:
		return value == threshold
	default:
		return false
	}
}
