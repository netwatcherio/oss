package alert

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	log "github.com/sirupsen/logrus"
)

type TLSAlertPayload struct {
	StartTimestamp   string   `json:"start_timestamp"`
	StopTimestamp    string   `json:"stop_timestamp"`
	RemoteAddr       string   `json:"remote_addr"`
	Protocol         string   `json:"protocol"`
	TLSVersion       string   `json:"tls_version"`        // e.g., "1.2", "1.3"
	TLSCipherSuite   string   `json:"tls_cipher_suite"`  // e.g., "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
	Certificate      *TLSCert `json:"certificate,omitempty"`
	CertificateChain []TLSCert `json:"certificate_chain,omitempty"`
	IsExpired        bool     `json:"is_expired"`
	IsExpiringSoon   bool     `json:"is_expiring_soon"`
	DaysUntilExpiry  int      `json:"days_until_expiry"`
	IssuerOrg        string   `json:"issuer_org"`
	CertType         string   `json:"cert_type"`
	CertFingerprint  string   `json:"cert_fingerprint"`
	Error            string   `json:"error,omitempty"`
}

type TLSCert struct {
	Subject         string `json:"subject"`
	Issuer          string `json:"issuer"`
	NotBefore       string `json:"not_before"`
	NotAfter        string `json:"not_after"`
	DaysUntilExpiry int    `json:"days_until_expiry"`
	IssuerOrg       string `json:"issuer_org"`
	Fingerprint     string `json:"fingerprint"`
}

type TLSMetrics struct {
	DaysUntilExpiry     int
	VersionScore        float64 // 0-100, lower is worse
	CipherScore          float64 // 0-100, lower is worse
	ChainValid           bool
	VersionMismatch      bool   // true if TLS version is below 1.2
	WeakCipher           bool   // true if cipher is considered weak
	HasChainIssue        bool   // true if certificate chain has problems
}

var weakCiphers = map[string]bool{
	"TLS_RSA_WITH_RC4_128_SHA":                true,
	"TLS_RSA_WITH_3DES_EDE_CBC_SHA":           true,
	"TLS_RSA_WITH_AES_128_CBC_SHA":            false, // okay but aging
	"TLS_RSA_WITH_AES_256_CBC_SHA":            false, // okay but aging
	"TLS_RSA_WITH_AES_128_GCM_SHA256":         false, // acceptable
	"TLS_RSA_WITH_AES_256_GCM_SHA384":         false, // acceptable
	"TLS_ECDHE_RSA_WITH_RC4_128_SHA":          true,
	"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":     true,
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":      false,
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":      false,
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":   false,
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   false,
}

var weakCipherPrefixes = []string{
	"RC4",
	"3DES",
	"MD5",
	"SHA1",
}

func ParseTLSAlertPayload(payloadJSON []byte) (*TLSAlertPayload, error) {
	var payload TLSAlertPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func ExtractTLSMetrics(payload *TLSAlertPayload) *TLSMetrics {
	metrics := &TLSMetrics{
		DaysUntilExpiry: payload.DaysUntilExpiry,
		ChainValid:      true,
	}

	// Version scoring
	metrics.VersionScore = getTLSVersionScore(payload.TLSVersion)
	metrics.VersionMismatch = payload.TLSVersion != "" && (payload.TLSVersion == "1.0" || payload.TLSVersion == "1.1" || payload.TLSVersion == "")

	// Cipher scoring
	metrics.CipherScore = getTLSCipherScore(payload.TLSCipherSuite)
	metrics.WeakCipher = isWeakCipher(payload.TLSCipherSuite)

	// Chain validation
	if len(payload.CertificateChain) == 0 && payload.Certificate == nil {
		metrics.ChainValid = false
		metrics.HasChainIssue = true
	} else if payload.Certificate != nil && payload.Certificate.Subject == payload.Certificate.Issuer {
		// Self-signed - not necessarily an issue but flagged
		if len(payload.CertificateChain) == 0 {
			metrics.HasChainIssue = true
		}
	}

	return metrics
}

func getTLSVersionScore(version string) float64 {
	switch version {
	case "1.3":
		return 100
	case "1.2":
		return 90
	case "1.1":
		return 40
	case "1.0":
		return 10
	default:
		return 0 // unknown/empty
	}
}

func isWeakCipher(cipher string) bool {
	if cipher == "" {
		return true // no cipher is weak
	}
	if _, ok := weakCiphers[cipher]; ok {
		return weakCiphers[cipher]
	}
	for _, prefix := range weakCipherPrefixes {
		if strings.Contains(cipher, prefix) {
			return true
		}
	}
	return false
}

func getTLSCipherScore(cipher string) float64 {
	if cipher == "" {
		return 0
	}
	if isWeakCipher(cipher) {
		return 20
	}
	// Check for AES GCM (good)
	if strings.Contains(cipher, "GCM") {
		return 100
	}
	// Check for AES CBC (acceptable)
	if strings.Contains(cipher, "AES") && !strings.Contains(cipher, "GCM") {
		return 70
	}
	// ChaCha20 (good)
	if strings.Contains(cipher, "CHACHA") {
		return 100
	}
	return 50 // default
}

func GetTLSMetricValue(metrics *TLSMetrics, m Metric) *float64 {
	switch m {
	case MetricTLSExpiryDays:
		return floatPtr(float64(metrics.DaysUntilExpiry))
	case MetricTLSVersionMismatch:
		if metrics.VersionMismatch {
			return floatPtr(1)
		}
		return floatPtr(0)
	case MetricTLSWeakCipher:
		if metrics.WeakCipher {
			return floatPtr(1)
		}
		return floatPtr(0)
	case MetricTLSChainIssue:
		if metrics.HasChainIssue {
			return floatPtr(1)
		}
		return floatPtr(0)
	}
	return nil
}

func floatPtr(v float64) *float64 {
	return &v
}

func isTlsMetric(m Metric) bool {
	switch m {
	case MetricTLSExpiryDays, MetricTLSVersionMismatch, MetricTLSWeakCipher, MetricTLSChainIssue:
		return true
	default:
		return false
	}
}

func evaluateTlsRule(rule *AlertRule, pctx ProbeContext, payloadJSON []byte) *EvaluationResult {
	if !isTlsMetric(rule.Metric) {
		return nil
	}

	payload, err := ParseTLSAlertPayload(payloadJSON)
	if err != nil {
		log.Warnf("alert.evaluateTlsRule: failed to parse TLS payload: %v", err)
		return nil
	}

	metrics := ExtractTLSMetrics(payload)
	value := GetTLSMetricValue(metrics, rule.Metric)
	if value == nil {
		return nil
	}

	triggered := ShouldTrigger(rule.Operator, *value, rule.Threshold)

	if triggered {
		return &EvaluationResult{
			Triggered: true,
			Value:     *value,
			Metric:    string(rule.Metric),
			Message:   formatTlsMessage(rule.Metric, *value, rule.Threshold, payload, metrics),
		}
	}

	return &EvaluationResult{Triggered: false}
}

func formatTlsMessage(metric Metric, value, threshold float64, payload *TLSAlertPayload, metrics *TLSMetrics) string {
	switch metric {
	case MetricTLSExpiryDays:
		return fmt.Sprintf("TLS certificate expires in %d days (threshold: %.0f)", payload.DaysUntilExpiry, threshold)
	case MetricTLSVersionMismatch:
		return fmt.Sprintf("TLS version %s is outdated (expected >= 1.2)", payload.TLSVersion)
	case MetricTLSWeakCipher:
		return fmt.Sprintf("TLS cipher %s is considered weak", payload.TLSCipherSuite)
	case MetricTLSChainIssue:
		if len(payload.CertificateChain) == 0 {
			return "TLS certificate chain is incomplete or missing"
		}
		return "TLS certificate chain validation failed"
	default:
		return fmt.Sprintf("%s exceeded threshold: %.2f (threshold: %.2f)", metric, value, threshold)
	}
}

func min(a, b float64) float64 {
	return math.Min(a, b)
}

func max(a, b float64) float64 {
	return math.Max(a, b)
}