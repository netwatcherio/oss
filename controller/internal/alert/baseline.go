package alert

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
)

type BaselineStats struct {
	ProbeID    uint      `json:"probe_id"`
	Metric     Metric    `json:"metric"`
	WindowDays int       `json:"window_days"`
	Mean       float64   `json:"mean"`
	StdDev     float64   `json:"std_dev"`
	P50        float64   `json:"p50"`
	P90        float64   `json:"p90"`
	P95        float64   `json:"p95"`
	P99        float64   `json:"p99"`
	Min        float64   `json:"min"`
	Max        float64   `json:"max"`
	Count      int64     `json:"count"`
	ComputedAt time.Time `json:"computed_at"`
}

type probeTypeResult struct {
	Type string
}

func GetProbeType(ctx context.Context, db *sql.DB, probeID uint) (string, error) {
	var result probeTypeResult
	err := db.QueryRowContext(ctx, "SELECT type FROM probes WHERE id = $1", probeID).Scan(&result.Type)
	return result.Type, err
}

func chQuoteTime(t time.Time) string {
	return fmt.Sprintf("'%s'", t.UTC().Format("2006-01-02 15:04:05"))
}

func computePercentiles(vals []float64, percentiles []float64) map[float64]float64 {
	if len(vals) == 0 {
		return nil
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)

	results := make(map[float64]float64)
	for _, p := range percentiles {
		idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		results[p] = sorted[idx]
	}
	return results
}

func computeStats(vals []float64) (mean, stdDev, min, max float64, count int64) {
	if len(vals) == 0 {
		return 0, 0, 0, 0, 0
	}
	var sum, sumSq float64
	min = vals[0]
	max = vals[0]
	for _, v := range vals {
		sum += v
		sumSq += v * v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	count = int64(len(vals))
	mean = sum / float64(count)
	if count > 1 {
		variance := (sumSq / float64(count)) - (mean * mean)
		if variance < 0 {
			variance = 0
		}
		stdDev = math.Sqrt(variance)
	}
	return mean, stdDev, min, max, count
}

func GetProbeBaseline(ctx context.Context, ch *sql.DB, probeID uint, metric Metric, windowDays int) (*BaselineStats, error) {
	if windowDays <= 0 {
		windowDays = 7
	}
	from := time.Now().UTC().Add(-time.Duration(windowDays) * 24 * time.Hour)

	var probeType string
	var target string
	var agentID uint64

	row := ch.QueryRowContext(ctx, `
		SELECT type, target, agent_id
		FROM probe_data
		WHERE probe_id = $1
		LIMIT 1`,
		probeID)
	if err := row.Scan(&probeType, &target, &agentID); err != nil {
		return nil, fmt.Errorf("failed to determine probe type: %w", err)
	}

	var vals []float64
	switch probeType {
	case "PING":
		vals = fetchPingMetric(ctx, ch, probeID, metric, from)
	case "DNS":
		vals = fetchDnsMetric(ctx, ch, probeID, metric, from)
	case "HTTP":
		vals = fetchHttpMetric(ctx, ch, probeID, metric, from)
	case "TLS":
		vals = fetchTlsMetric(ctx, ch, probeID, metric, from)
	case "SNMP":
		vals = fetchSnmpMetric(ctx, ch, probeID, metric, from)
	case "TRAFFICSIM":
		vals = fetchTrafficSimMetric(ctx, ch, probeID, metric, from)
	case "MTR":
		vals = fetchMtrMetric(ctx, ch, probeID, metric, from)
	case "SYSINFO":
		vals = fetchSysinfoMetric(ctx, ch, probeID, metric, from)
	default:
		return nil, fmt.Errorf("unsupported probe type for baseline: %s", probeType)
	}

	mean, stdDev, min, max, count := computeStats(vals)
	pcts := computePercentiles(vals, []float64{50, 90, 95, 99})

	stats := &BaselineStats{
		ProbeID:    probeID,
		Metric:     metric,
		WindowDays: windowDays,
		Mean:       mean,
		StdDev:     stdDev,
		P50:        getOrZero(pcts, 50),
		P90:        getOrZero(pcts, 90),
		P95:        getOrZero(pcts, 95),
		P99:        getOrZero(pcts, 99),
		Min:        min,
		Max:        max,
		Count:      count,
		ComputedAt: time.Now().UTC(),
	}

	return stats, nil
}

func getOrZero(pcts map[float64]float64, p float64) float64 {
	if v, ok := pcts[p]; ok {
		return v
	}
	return 0
}

type pingPayload struct {
	AvgRTT     int64   `json:"avg_rtt"`
	MinRTT     int64   `json:"min_rtt"`
	MaxRTT     int64   `json:"max_rtt"`
	PacketLoss float64 `json:"packet_loss"`
}

func fetchPingMetric(ctx context.Context, ch *sql.DB, probeID uint, metric Metric, from time.Time) []float64 {
	q := fmt.Sprintf(`
		SELECT payload_raw
		FROM probe_data
		WHERE probe_id = %d
		  AND type = 'PING'
		  AND created_at >= %s
		LIMIT 10000`, probeID, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p pingPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		switch metric {
		case MetricLatency:
			if p.AvgRTT > 0 {
				vals = append(vals, float64(p.AvgRTT)/1e6)
			}
		case MetricPacketLoss:
			if p.PacketLoss >= 0 {
				vals = append(vals, p.PacketLoss)
			}
		}
	}
	return vals
}

type dnsPayload struct {
	QueryTimeMs  float64 `json:"query_time_ms"`
	ResponseCode string  `json:"response_code"`
}

func fetchDnsMetric(ctx context.Context, ch *sql.DB, probeID uint, metric Metric, from time.Time) []float64 {
	q := fmt.Sprintf(`
		SELECT payload_raw
		FROM probe_data
		WHERE probe_id = %d
		  AND type = 'DNS'
		  AND created_at >= %s
		LIMIT 10000`, probeID, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p dnsPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		switch metric {
		case MetricDNSQueryTime:
			if p.QueryTimeMs > 0 {
				vals = append(vals, p.QueryTimeMs)
			}
		}
	}
	return vals
}

type httpPayload struct {
	DNSLookupMs    float64 `json:"dns_lookup_ms"`
	TCPConnectMs   float64 `json:"tcp_connect_ms"`
	TLSHandshakeMs float64 `json:"tls_handshake_ms"`
	FirstByteMs    float64 `json:"first_byte_ms"`
	TotalMs        float64 `json:"total_ms"`
	StatusCode     int     `json:"status_code"`
}

func fetchHttpMetric(ctx context.Context, ch *sql.DB, probeID uint, metric Metric, from time.Time) []float64 {
	q := fmt.Sprintf(`
		SELECT payload_raw
		FROM probe_data
		WHERE probe_id = %d
		  AND type = 'HTTP'
		  AND created_at >= %s
		LIMIT 10000`, probeID, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p httpPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		switch metric {
		case MetricHTTPTTFB:
			if p.FirstByteMs > 0 {
				vals = append(vals, p.FirstByteMs)
			}
		case MetricHTTPTotalMs:
			if p.TotalMs > 0 {
				vals = append(vals, p.TotalMs)
			}
		}
	}
	return vals
}

type tlsPayload struct {
	DaysUntilExpiry int  `json:"days_until_expiry"`
	IsExpired       bool `json:"is_expired"`
	IsExpiringSoon  bool `json:"is_expiring_soon"`
}

func fetchTlsMetric(ctx context.Context, ch *sql.DB, probeID uint, metric Metric, from time.Time) []float64 {
	q := fmt.Sprintf(`
		SELECT payload_raw
		FROM probe_data
		WHERE probe_id = %d
		  AND type = 'TLS'
		  AND created_at >= %s
		LIMIT 10000`, probeID, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p tlsPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		switch metric {
		case MetricTLSExpiryDays:
			vals = append(vals, float64(p.DaysUntilExpiry))
		}
	}
	return vals
}

type snmpPayload struct {
	QueryTimeMs float64 `json:"query_time_ms"`
	Error       string  `json:"error"`
}

func fetchSnmpMetric(ctx context.Context, ch *sql.DB, probeID uint, metric Metric, from time.Time) []float64 {
	q := fmt.Sprintf(`
		SELECT payload_raw
		FROM probe_data
		WHERE probe_id = %d
		  AND type = 'SNMP'
		  AND created_at >= %s
		LIMIT 10000`, probeID, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p snmpPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		switch metric {
		case MetricSNMPResponseMs:
			if p.QueryTimeMs > 0 && p.Error == "" {
				vals = append(vals, p.QueryTimeMs)
			}
		}
	}
	return vals
}

type trafficPayload struct {
	AverageRTT float64 `json:"averageRTT"`
}

func fetchTrafficSimMetric(ctx context.Context, ch *sql.DB, probeID uint, metric Metric, from time.Time) []float64 {
	q := fmt.Sprintf(`
		SELECT payload_raw
		FROM probe_data
		WHERE probe_id = %d
		  AND type = 'TRAFFICSIM'
		  AND created_at >= %s
		LIMIT 10000`, probeID, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p trafficPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		switch metric {
		case MetricLatency:
			if p.AverageRTT > 0 {
				vals = append(vals, p.AverageRTT)
			}
		}
	}
	return vals
}

type mtrHop struct {
	Avg string `json:"avg"`
}

type mtrReport struct {
	Hops []mtrHop `json:"hops"`
}

type mtrPayload struct {
	Report mtrReport `json:"report"`
}

func fetchMtrMetric(ctx context.Context, ch *sql.DB, probeID uint, metric Metric, from time.Time) []float64 {
	q := fmt.Sprintf(`
		SELECT payload_raw
		FROM probe_data
		WHERE probe_id = %d
		  AND type = 'MTR'
		  AND created_at >= %s
		LIMIT 5000`, probeID, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p mtrPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		if len(p.Report.Hops) == 0 {
			continue
		}
		lastHop := p.Report.Hops[len(p.Report.Hops)-1]
		latencyMs := parseMtrLatency(lastHop.Avg)
		if latencyMs > 0 {
			vals = append(vals, latencyMs)
		}
	}
	return vals
}

func parseMtrLatency(avg string) float64 {
	if avg == "" || avg == "*" {
		return 0
	}
	var val float64
	_, err := fmt.Sscanf(avg, "%fms", &val)
	if err != nil {
		_, err = fmt.Sscanf(avg, "%f", &val)
	}
	return val
}

type sysinfoPayload struct {
	CPUUsage float64 `json:"cpu_usage"`
}

func fetchSysinfoMetric(ctx context.Context, ch *sql.DB, probeID uint, metric Metric, from time.Time) []float64 {
	q := fmt.Sprintf(`
		SELECT payload_raw
		FROM probe_data
		WHERE probe_id = %d
		  AND type = 'SYSINFO'
		  AND created_at >= %s
		LIMIT 10000`, probeID, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var vals []float64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p sysinfoPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		switch metric {
		case MetricCpuUsage:
			if p.CPUUsage > 0 {
				vals = append(vals, p.CPUUsage)
			}
		}
	}
	return vals
}

func ComputeDynamicThreshold(stats *BaselineStats, rule *AlertRule) float64 {
	switch rule.ThresholdType {
	case ThresholdTypeDynamicStddev:
		multiplier := rule.StddevMultiplier
		if rule.Severity == SeverityCritical {
			multiplier = rule.StddevMultiplierCrit
		}
		return stats.Mean + (stats.StdDev * multiplier)

	case ThresholdTypeDynamicPercentile:
		percentile := rule.Percentile
		if percentile <= 0 {
			percentile = 95
		}
		switch percentile {
		case 50:
			return stats.P50
		case 90:
			return stats.P90
		case 95:
			return stats.P95
		case 99:
			return stats.P99
		default:
			return stats.P95
		}

	default:
		return rule.Threshold
	}
}
