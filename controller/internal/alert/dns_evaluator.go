package alert

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
)

// DNSAlertPayload represents the DNS probe data fields needed for alert evaluation
type DNSAlertPayload struct {
	QueryTimeMs  float64 `json:"query_time_ms"`
	ResponseCode string  `json:"response_code"`
	Error        string  `json:"error,omitempty"`
}

// DNSMetrics holds extracted alertable metrics from DNS data
type DNSMetrics struct {
	QueryTimeMs float64 // DNS query response time in milliseconds
	IsFailure   float64 // 1.0 if the query failed (error or non-NOERROR response), 0.0 otherwise
	IsNXDomain  float64 // 1.0 if the response was NXDOMAIN, 0.0 otherwise
}

// ParseDNSAlertPayload parses raw DNS JSON payload
func ParseDNSAlertPayload(payloadJSON []byte) (*DNSAlertPayload, error) {
	var payload DNSAlertPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// ExtractDNSMetrics computes alertable values from a DNS payload
func ExtractDNSMetrics(payload *DNSAlertPayload) *DNSMetrics {
	metrics := &DNSMetrics{
		QueryTimeMs: payload.QueryTimeMs,
	}

	// Mark as failure if there's an explicit error or a non-NOERROR response code
	if payload.Error != "" || (payload.ResponseCode != "NOERROR" && payload.ResponseCode != "") {
		metrics.IsFailure = 1.0
	}

	if payload.ResponseCode == "NXDOMAIN" {
		metrics.IsNXDomain = 1.0
	}

	return metrics
}

// GetDNSMetricValue extracts a specific metric value from DNS metrics
func GetDNSMetricValue(metrics *DNSMetrics, metric Metric) *float64 {
	switch metric {
	case MetricDNSQueryTime:
		return &metrics.QueryTimeMs
	case MetricDNSQueryFailure:
		return &metrics.IsFailure
	case MetricDNSNXDomain:
		return &metrics.IsNXDomain
	default:
		return nil
	}
}

// evaluateDnsRule evaluates DNS-specific alert rules
func evaluateDnsRule(rule *AlertRule, pctx ProbeContext, payloadJSON []byte) *EvaluationResult {
	// Check if this rule uses DNS metrics
	if !isDnsMetric(rule.Metric) {
		return nil
	}

	payload, err := ParseDNSAlertPayload(payloadJSON)
	if err != nil {
		log.Warnf("alert.evaluateDnsRule: failed to parse DNS payload: %v", err)
		return nil
	}

	metrics := ExtractDNSMetrics(payload)
	value := GetDNSMetricValue(metrics, rule.Metric)
	if value == nil {
		return nil
	}

	triggered := ShouldTrigger(rule.Operator, *value, rule.Threshold)

	if triggered {
		return &EvaluationResult{
			Triggered: true,
			Value:     *value,
			Metric:    string(rule.Metric),
			Message:   formatDnsMessage(rule.Metric, *value, rule.Threshold, payload),
		}
	}

	return &EvaluationResult{Triggered: false}
}

// isDnsMetric checks if a metric is DNS-specific
func isDnsMetric(m Metric) bool {
	switch m {
	case MetricDNSQueryTime, MetricDNSQueryFailure, MetricDNSNXDomain:
		return true
	default:
		return false
	}
}

// formatDnsMessage creates a human-readable alert message for DNS metrics
func formatDnsMessage(metric Metric, value, threshold float64, payload *DNSAlertPayload) string {
	switch metric {
	case MetricDNSQueryTime:
		return fmt.Sprintf("DNS query time at %.1fms (threshold: %.1fms)", value, threshold)
	case MetricDNSQueryFailure:
		detail := payload.ResponseCode
		if payload.Error != "" {
			detail = payload.Error
		}
		return fmt.Sprintf("DNS query failed: %s", detail)
	case MetricDNSNXDomain:
		return "DNS returned NXDOMAIN (domain not found)"
	default:
		return fmt.Sprintf("%s exceeded threshold: %.2f (threshold: %.2f)", metric, value, threshold)
	}
}
