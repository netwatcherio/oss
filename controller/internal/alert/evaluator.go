package alert

import (
	"context"
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ProbeDataPayload represents the parsed payload from PING/TRAFFICSIM probe data
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

// EvaluationResult holds the result of rule evaluation
type EvaluationResult struct {
	Triggered     bool
	Value         float64
	Metric        string
	Message       string
	IsRouteChange bool
}

// EvaluateProbeData checks probe data against alert rules and creates alerts if thresholds are exceeded
func EvaluateProbeData(ctx context.Context, db *gorm.DB, pctx ProbeContext, payloadJSON []byte) error {
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

	// Parse payload based on probe type
	var result *EvaluationResult
	for _, rule := range rules {
		switch pctx.ProbeType {
		case "MTR":
			result = evaluateMtrRule(ctx, db, &rule, pctx, payloadJSON)
		case "SYSINFO":
			result = evaluateSysInfoRule(&rule, pctx, payloadJSON)
		default:
			result = evaluateStandardRule(&rule, pctx, payloadJSON)
		}

		if result == nil {
			continue // Rule not applicable
		}

		if result.Triggered {
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
			actx := &AlertContext{
				ProbeID:     pctx.ProbeID,
				ProbeType:   pctx.ProbeType,
				ProbeName:   pctx.ProbeName,
				ProbeTarget: pctx.ProbeTarget,
				AgentID:     pctx.AgentID,
				AgentName:   pctx.AgentName,
			}

			alertInstance, err := CreateAlert(ctx, db, &rule, result.Value, result.Message, actx)
			if err != nil {
				log.Errorf("alert.EvaluateProbeData: failed to create alert: %v", err)
				continue
			}

			// Dispatch notifications
			go DispatchNotifications(ctx, db, &rule, alertInstance)

			log.Infof("Alert triggered: rule=%d, probe=%d (%s), metric=%s, value=%.2f",
				rule.ID, pctx.ProbeID, pctx.ProbeType, result.Metric, result.Value)
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

// evaluateStandardRule evaluates PING/TRAFFICSIM rules with optional compound conditions
func evaluateStandardRule(rule *AlertRule, pctx ProbeContext, payloadJSON []byte) *EvaluationResult {
	var payload ProbeDataPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		log.Warnf("alert.evaluateStandardRule: failed to parse payload: %v", err)
		return nil
	}

	// Evaluate primary condition
	value1 := getMetricValue(&payload, rule.Metric, pctx.ProbeType)
	if value1 == nil {
		return nil // Metric not applicable to this probe type
	}
	cond1 := shouldTrigger(rule.Operator, *value1, rule.Threshold)

	// If no secondary condition, use primary result
	if rule.Metric2 == nil {
		if cond1 {
			return &EvaluationResult{
				Triggered: true,
				Value:     *value1,
				Metric:    string(rule.Metric),
				Message:   fmt.Sprintf("%s exceeded threshold: %.2f (threshold: %.2f)", rule.Metric, *value1, rule.Threshold),
			}
		}
		return &EvaluationResult{Triggered: false}
	}

	// Evaluate secondary condition
	value2 := getMetricValue(&payload, *rule.Metric2, pctx.ProbeType)
	if value2 == nil {
		// Secondary metric not applicable, fall back to primary only
		if cond1 {
			return &EvaluationResult{
				Triggered: true,
				Value:     *value1,
				Metric:    string(rule.Metric),
				Message:   fmt.Sprintf("%s exceeded threshold: %.2f (threshold: %.2f)", rule.Metric, *value1, rule.Threshold),
			}
		}
		return &EvaluationResult{Triggered: false}
	}
	cond2 := shouldTrigger(*rule.Operator2, *value2, *rule.Threshold2)

	// Combine conditions based on logical operator
	var triggered bool
	var reportValue float64
	var reportMetric Metric
	var reportThreshold float64

	switch rule.LogicalOp {
	case LogicalOr:
		triggered = cond1 || cond2
		// Report the first failing condition
		if cond1 {
			reportValue = *value1
			reportMetric = rule.Metric
			reportThreshold = rule.Threshold
		} else if cond2 {
			reportValue = *value2
			reportMetric = *rule.Metric2
			reportThreshold = *rule.Threshold2
		}
	default: // AND (default)
		triggered = cond1 && cond2
		// Report whichever is worse (higher value typically worse for latency/loss)
		if *value1 > *value2 {
			reportValue = *value1
			reportMetric = rule.Metric
			reportThreshold = rule.Threshold
		} else {
			reportValue = *value2
			reportMetric = *rule.Metric2
			reportThreshold = *rule.Threshold2
		}
	}

	if triggered {
		var message string
		if rule.LogicalOp == LogicalOr {
			message = fmt.Sprintf("%s exceeded threshold: %.2f (threshold: %.2f)", reportMetric, reportValue, reportThreshold)
		} else {
			message = fmt.Sprintf("Compound condition met: %s=%.2f AND %s=%.2f",
				rule.Metric, *value1, *rule.Metric2, *value2)
		}
		return &EvaluationResult{
			Triggered: true,
			Value:     reportValue,
			Metric:    string(reportMetric),
			Message:   message,
		}
	}

	return &EvaluationResult{Triggered: false}
}

// evaluateMtrRule evaluates MTR-specific alert rules
func evaluateMtrRule(ctx context.Context, db *gorm.DB, rule *AlertRule, pctx ProbeContext, payloadJSON []byte) *EvaluationResult {
	// Check if this rule uses MTR metrics
	if !isMtrMetric(rule.Metric) {
		// Standard metrics on MTR data - not supported yet
		return nil
	}

	mtr, err := ParseMtrPayload(payloadJSON)
	if err != nil {
		log.Warnf("alert.evaluateMtrRule: failed to parse MTR payload: %v", err)
		return nil
	}

	metrics := ExtractMtrMetrics(mtr)

	// Handle route_change metric specially
	if rule.Metric == MetricRouteChange {
		return evaluateRouteChange(ctx, db, pctx, mtr)
	}

	// Get metric value
	value := GetMtrMetricValue(metrics, rule.Metric)
	if value == nil {
		return nil
	}

	triggered := shouldTrigger(rule.Operator, *value, rule.Threshold)

	if triggered {
		return &EvaluationResult{
			Triggered: true,
			Value:     *value,
			Metric:    string(rule.Metric),
			Message:   fmt.Sprintf("MTR %s exceeded threshold: %.2f (threshold: %.2f)", rule.Metric, *value, rule.Threshold),
		}
	}

	return &EvaluationResult{Triggered: false}
}

// evaluateRouteChange checks if the route has changed from baseline
func evaluateRouteChange(ctx context.Context, db *gorm.DB, pctx ProbeContext, mtr *MtrPayload) *EvaluationResult {
	currentFP := GetRouteFingerprint(mtr)
	currentPath := GetRoutePathString(mtr)

	// Get baseline
	baseline, err := GetRouteBaseline(ctx, db, pctx.ProbeID)
	if err != nil {
		// No baseline exists - create one from current route
		hopCount := len(mtr.Report.Hops)
		if setErr := SetRouteBaseline(ctx, db, pctx.ProbeID, currentFP, currentPath, hopCount); setErr != nil {
			log.Warnf("alert.evaluateRouteChange: failed to set initial baseline: %v", setErr)
		} else {
			log.Infof("Route baseline created for probe %d: %s", pctx.ProbeID, currentPath)
		}
		return &EvaluationResult{Triggered: false}
	}

	// Compare routes
	if CompareRoutes(baseline.Fingerprint, currentFP) {
		return &EvaluationResult{
			Triggered:     true,
			Value:         1, // Binary: 1 = changed
			Metric:        string(MetricRouteChange),
			Message:       fmt.Sprintf("Route changed from baseline. New path: %s", currentPath),
			IsRouteChange: true,
		}
	}

	return &EvaluationResult{Triggered: false}
}

// isMtrMetric checks if a metric is MTR-specific
func isMtrMetric(m Metric) bool {
	switch m {
	case MetricEndHopLoss, MetricEndHopLatency, MetricRouteChange, MetricWorstHopLoss:
		return true
	default:
		return false
	}
}

// getMetricValue extracts the relevant metric value from the PING/TRAFFICSIM payload
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
