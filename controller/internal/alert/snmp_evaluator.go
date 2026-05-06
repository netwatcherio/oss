package alert

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
)

type SNMPAlertPayload struct {
	QueryTimeMs float64 `json:"query_time_ms"`
	Error       string  `json:"error"`
	IsTrap      bool    `json:"is_trap"`
	TrapOID     string  `json:"trap_oid,omitempty"`
	TrapValue   string  `json:"trap_value,omitempty"`
}

type SNMPMetrics struct {
	ResponseMs float64
	IsError     float64
	IsTrap      float64
}

func ParseSNMPAlertPayload(payloadJSON []byte) (*SNMPAlertPayload, error) {
	var payload SNMPAlertPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func ExtractSNMPMetrics(payload *SNMPAlertPayload) *SNMPMetrics {
	metrics := &SNMPMetrics{
		ResponseMs: payload.QueryTimeMs,
	}

	if payload.Error != "" {
		metrics.IsError = 1.0
	}

	if payload.IsTrap {
		metrics.IsTrap = 1.0
	}

	return metrics
}

func GetSNMPMetricValue(metrics *SNMPMetrics, m Metric) *float64 {
	switch m {
	case MetricSNMPResponseMs:
		if metrics.ResponseMs > 0 {
			return &metrics.ResponseMs
		}
	case MetricSNMPTrapReceived:
		return &metrics.IsTrap
	}
	return nil
}

func isSnmpMetric(m Metric) bool {
	switch m {
	case MetricSNMPResponseMs, MetricSNMPTrapReceived:
		return true
	default:
		return false
	}
}

func evaluateSnmpRule(rule *AlertRule, pctx ProbeContext, payloadJSON []byte) *EvaluationResult {
	if !isSnmpMetric(rule.Metric) {
		return nil
	}

	payload, err := ParseSNMPAlertPayload(payloadJSON)
	if err != nil {
		log.Warnf("alert.evaluateSnmpRule: failed to parse SNMP payload: %v", err)
		return nil
	}

	metrics := ExtractSNMPMetrics(payload)
	value := GetSNMPMetricValue(metrics, rule.Metric)
	if value == nil {
		return nil
	}

	triggered := ShouldTrigger(rule.Operator, *value, rule.Threshold)

	if triggered {
		return &EvaluationResult{
			Triggered: true,
			Value:     *value,
			Metric:    string(rule.Metric),
			Message:   formatSnmpMessage(rule.Metric, *value, rule.Threshold, payload),
		}
	}

	return &EvaluationResult{Triggered: false}
}

func formatSnmpMessage(metric Metric, value, threshold float64, payload *SNMPAlertPayload) string {
	switch metric {
	case MetricSNMPResponseMs:
		return fmt.Sprintf("SNMP response time at %.1fms (threshold: %.1fms)", value, threshold)
	case MetricSNMPTrapReceived:
		trapInfo := payload.TrapOID
		if trapInfo == "" {
			trapInfo = "unknown"
		}
		return fmt.Sprintf("SNMP trap received: %s", trapInfo)
	default:
		return fmt.Sprintf("%s exceeded threshold: %.2f (threshold: %.2f)", metric, value, threshold)
	}
}

// SnmpTrapConfig holds configuration for receiving SNMP traps
// Note: Full trap support requires a separate SNMP trap receiver service
// This is a placeholder for trap-based alerting
type SnmpTrapConfig struct {
	Enabled     bool
	ListenAddr  string
	Community   string
	OIDMappings map[string]string
}