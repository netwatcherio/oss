package alert

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type HTTPAlertPayload struct {
	DNSLookupMs    float64 `json:"dns_lookup_ms"`
	TCPConnectMs   float64 `json:"tcp_connect_ms"`
	TLSHandshakeMs float64 `json:"tls_handshake_ms"`
	FirstByteMs    float64 `json:"first_byte_ms"`
	TotalMs        float64 `json:"total_ms"`
	StatusCode     int     `json:"status_code"`
	Error          string  `json:"error"`
}

type HTTPMetrics struct {
	TTFB   float64
	Total  float64
	Status float64
}

func ParseHTTPAlertPayload(payloadJSON []byte) (*HTTPAlertPayload, error) {
	var payload HTTPAlertPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func ExtractHTTPMetrics(payload *HTTPAlertPayload) *HTTPMetrics {
	return &HTTPMetrics{
		TTFB:   payload.FirstByteMs,
		Total:  payload.TotalMs,
		Status: float64(payload.StatusCode),
	}
}

func GetHTTPMetricValue(metrics *HTTPMetrics, m Metric) *float64 {
	switch m {
	case MetricHTTPTTFB:
		if metrics.TTFB > 0 {
			return &metrics.TTFB
		}
	case MetricHTTPTotalMs:
		if metrics.Total > 0 {
			return &metrics.Total
		}
	case MetricHTTPStatus:
		return &metrics.Status
	}
	return nil
}

func isHttpMetric(m Metric) bool {
	switch m {
	case MetricHTTPTTFB, MetricHTTPTotalMs, MetricHTTPStatus:
		return true
	default:
		return false
	}
}

func evaluateHttpRule(ctx context.Context, db *gorm.DB, ch *sql.DB, rule *AlertRule, pctx ProbeContext, payloadJSON []byte) *EvaluationResult {
	if !isHttpMetric(rule.Metric) {
		return nil
	}

	payload, err := ParseHTTPAlertPayload(payloadJSON)
	if err != nil {
		log.Warnf("alert.evaluateHttpRule: failed to parse HTTP payload: %v", err)
		return nil
	}

	metrics := ExtractHTTPMetrics(payload)
	value := GetHTTPMetricValue(metrics, rule.Metric)
	if value == nil {
		return nil
	}

	threshold := rule.Threshold
	if rule.ThresholdType != ThresholdTypeStatic && ch != nil {
		stats, err := GetProbeBaseline(ctx, ch, pctx.ProbeID, rule.Metric, rule.BaselineWindowDays)
		if err != nil {
			log.Warnf("alert.evaluateHttpRule: failed to get baseline for probe %d metric %s: %v", pctx.ProbeID, rule.Metric, err)
		} else if stats.Count > 0 {
			threshold = ComputeDynamicThreshold(stats, rule)
		}
	}

	triggered := ShouldTrigger(rule.Operator, *value, threshold)

	if triggered {
		var message string
		switch rule.Metric {
		case MetricHTTPTTFB:
			message = fmt.Sprintf("HTTP TTFB at %.1fms (threshold: %.1fms)", *value, threshold)
		case MetricHTTPTotalMs:
			message = fmt.Sprintf("HTTP total time at %.1fms (threshold: %.1fms)", *value, threshold)
		case MetricHTTPStatus:
			message = fmt.Sprintf("HTTP status code %d (expected %s %.0f)", int(*value), rule.Operator, threshold)
		default:
			message = fmt.Sprintf("HTTP metric %s exceeded threshold: %.2f (threshold: %.2f)", rule.Metric, *value, threshold)
		}
		return &EvaluationResult{
			Triggered: true,
			Value:     *value,
			Metric:    string(rule.Metric),
			Message:   message,
		}
	}

	return &EvaluationResult{Triggered: false}
}
