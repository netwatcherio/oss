package alert

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
)

// SysInfoPayload represents the SYSINFO probe data structure
type SysInfoPayload struct {
	HostInfo   HostInfo       `json:"hostInfo"`
	MemoryInfo HostMemoryInfo `json:"memoryInfo"`
	CPUTimes   CPUTimes       `json:"CPUTimes"`
}

// HostInfo contains basic host information
type HostInfo struct {
	Architecture string `json:"architecture"`
	Name         string `json:"name"`
	OS           OSInfo `json:"os"`
}

// OSInfo contains OS details
type OSInfo struct {
	Type     string `json:"type"`
	Family   string `json:"family"`
	Platform string `json:"platform"`
	Name     string `json:"name"`
	Version  string `json:"version"`
}

// HostMemoryInfo contains memory usage information
type HostMemoryInfo struct {
	TotalBytes     uint64 `json:"total_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	FreeBytes      uint64 `json:"free_Bytes"`
}

// CPUTimes contains CPU usage information
type CPUTimes struct {
	User    float64 `json:"user"`
	System  float64 `json:"system"`
	Idle    float64 `json:"idle"`
	IOWait  float64 `json:"iowait"`
	IRQ     float64 `json:"irq"`
	Nice    float64 `json:"nice"`
	SoftIRQ float64 `json:"softIRQ"`
	Steal   float64 `json:"steal"`
}

// SysInfoMetrics holds extracted metrics from SYSINFO data
type SysInfoMetrics struct {
	CpuUsage    float64 // CPU usage percentage (0-100)
	MemoryUsage float64 // Memory usage percentage (0-100)
}

// ParseSysInfoPayload parses raw SYSINFO JSON payload
func ParseSysInfoPayload(payloadJSON []byte) (*SysInfoPayload, error) {
	var sysinfo SysInfoPayload
	if err := json.Unmarshal(payloadJSON, &sysinfo); err != nil {
		return nil, err
	}
	return &sysinfo, nil
}

// ExtractSysInfoMetrics extracts alertable metrics from SYSINFO data
func ExtractSysInfoMetrics(sysinfo *SysInfoPayload) *SysInfoMetrics {
	metrics := &SysInfoMetrics{}

	// Calculate memory usage percentage
	if sysinfo.MemoryInfo.TotalBytes > 0 {
		metrics.MemoryUsage = float64(sysinfo.MemoryInfo.UsedBytes) / float64(sysinfo.MemoryInfo.TotalBytes) * 100
	}

	// Calculate CPU usage percentage from CPU times
	// CPU usage = (user + system + nice + irq + softirq + steal) / (user + system + idle + iowait + nice + irq + softirq + steal) * 100
	totalTime := sysinfo.CPUTimes.User + sysinfo.CPUTimes.System + sysinfo.CPUTimes.Idle +
		sysinfo.CPUTimes.IOWait + sysinfo.CPUTimes.Nice + sysinfo.CPUTimes.IRQ +
		sysinfo.CPUTimes.SoftIRQ + sysinfo.CPUTimes.Steal

	activeTime := sysinfo.CPUTimes.User + sysinfo.CPUTimes.System + sysinfo.CPUTimes.Nice +
		sysinfo.CPUTimes.IRQ + sysinfo.CPUTimes.SoftIRQ + sysinfo.CPUTimes.Steal

	if totalTime > 0 {
		metrics.CpuUsage = activeTime / totalTime * 100
	}

	return metrics
}

// GetSysInfoMetricValue extracts a specific metric value from SYSINFO data
func GetSysInfoMetricValue(metrics *SysInfoMetrics, metric Metric) *float64 {
	switch metric {
	case MetricCpuUsage:
		return &metrics.CpuUsage
	case MetricMemoryUsage:
		return &metrics.MemoryUsage
	default:
		return nil
	}
}

// evaluateSysInfoRule evaluates SYSINFO-specific alert rules
func evaluateSysInfoRule(rule *AlertRule, pctx ProbeContext, payloadJSON []byte) *EvaluationResult {
	// Check if this rule uses SYSINFO metrics
	if !isSysInfoMetric(rule.Metric) {
		return nil
	}

	sysinfo, err := ParseSysInfoPayload(payloadJSON)
	if err != nil {
		log.Warnf("alert.evaluateSysInfoRule: failed to parse SYSINFO payload: %v", err)
		return nil
	}

	metrics := ExtractSysInfoMetrics(sysinfo)
	value := GetSysInfoMetricValue(metrics, rule.Metric)
	if value == nil {
		return nil
	}

	triggered := ShouldTrigger(rule.Operator, *value, rule.Threshold)

	if triggered {
		return &EvaluationResult{
			Triggered: true,
			Value:     *value,
			Metric:    string(rule.Metric),
			Message:   formatSysInfoMessage(rule.Metric, *value, rule.Threshold),
		}
	}

	return &EvaluationResult{Triggered: false}
}

// isSysInfoMetric checks if a metric is SYSINFO-specific
func isSysInfoMetric(m Metric) bool {
	switch m {
	case MetricCpuUsage, MetricMemoryUsage:
		return true
	default:
		return false
	}
}

// formatSysInfoMessage creates a human-readable alert message for SYSINFO metrics
func formatSysInfoMessage(metric Metric, value, threshold float64) string {
	switch metric {
	case MetricCpuUsage:
		return fmt.Sprintf("CPU usage at %.1f%% (threshold: %.1f%%)", value, threshold)
	case MetricMemoryUsage:
		return fmt.Sprintf("Memory usage at %.1f%% (threshold: %.1f%%)", value, threshold)
	default:
		return fmt.Sprintf("%s exceeded threshold: %.2f (threshold: %.2f)", metric, value, threshold)
	}
}
