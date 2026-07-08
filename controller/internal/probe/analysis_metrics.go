package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// ── Speedtest / SysInfo / NetInfo Metric Fetchers ──

type speedtestStats struct {
	AvgDownload  float64 // Mbps
	AvgUpload    float64 // Mbps
	AvgLatency   float64 // ms
	AvgJitterAvg float64 // ms
	Count        int
}

func getWorkspaceSpeedtestMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) (map[string]speedtestStats, error) {
	if len(agentIDs) == 0 {
		return make(map[string]speedtestStats), nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	q := fmt.Sprintf(`
SELECT agent_id, target, payload_raw
FROM probe_data
WHERE type = 'SPEEDTEST'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 500
`, strings.Join(agentIDStrs, ", "), chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type accum struct {
		dlTotal, ulTotal, latTotal, jitterTotal float64
		count                                   int
	}
	acc := make(map[string]*accum)

	for rows.Next() {
		var agentID uint64
		var target, payloadRaw string
		if err := rows.Scan(&agentID, &target, &payloadRaw); err != nil || payloadRaw == "" {
			continue
		}
		var result SpeedTestResult
		if err := json.Unmarshal([]byte(payloadRaw), &result); err != nil || len(result.TestData) == 0 {
			continue
		}
		srv := result.TestData[0]
		key := fmt.Sprintf("%d:%s", agentID, target)
		if acc[key] == nil {
			acc[key] = &accum{}
		}
		a := acc[key]
		a.dlTotal += float64(srv.DLSpeed) // bytes/sec → will convert later
		a.ulTotal += float64(srv.ULSpeed)
		a.latTotal += float64(srv.Latency) / float64(time.Millisecond)
		a.jitterTotal += float64(srv.Jitter) / float64(time.Millisecond)
		a.count++
	}

	out := make(map[string]speedtestStats, len(acc))
	for k, a := range acc {
		if a.count == 0 {
			continue
		}
		out[k] = speedtestStats{
			AvgDownload:  (a.dlTotal / float64(a.count)) * 8 / 1_000_000, // bytes/s → Mbps
			AvgUpload:    (a.ulTotal / float64(a.count)) * 8 / 1_000_000,
			AvgLatency:   a.latTotal / float64(a.count),
			AvgJitterAvg: a.jitterTotal / float64(a.count),
			Count:        a.count,
		}
	}
	return out, nil
}

type sysInfoStats struct {
	CPUUsagePct   float64 // 0-100
	MemUsagePct   float64 // 0-100
	MemTotalBytes uint64
	MemUsedBytes  uint64
	Hostname      string
}

func getWorkspaceSysInfoMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) (map[string]sysInfoStats, error) {
	if len(agentIDs) == 0 {
		return make(map[string]sysInfoStats), nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	// Get only the latest per agent
	q := fmt.Sprintf(`
SELECT agent_id, payload_raw
FROM probe_data
WHERE type = 'SYSINFO'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 100
`, strings.Join(agentIDStrs, ", "), chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]sysInfoStats)
	seen := make(map[string]bool) // only keep latest per agent

	for rows.Next() {
		var agentID uint64
		var payloadRaw string
		if err := rows.Scan(&agentID, &payloadRaw); err != nil || payloadRaw == "" {
			continue
		}
		key := fmt.Sprintf("%d", agentID)
		if seen[key] {
			continue
		}
		seen[key] = true

		var p sysInfoPayload
		if err := json.Unmarshal([]byte(payloadRaw), &p); err != nil {
			continue
		}

		cpuTotal := p.CPUTimes.User + p.CPUTimes.System + p.CPUTimes.Idle + p.CPUTimes.IOWait + p.CPUTimes.Nice + p.CPUTimes.SoftIRQ + p.CPUTimes.Steal + p.CPUTimes.IRQ
		cpuBusy := cpuTotal - p.CPUTimes.Idle
		cpuPct := 0.0
		if cpuTotal > 0 {
			cpuPct = (float64(cpuBusy) / float64(cpuTotal)) * 100
		}

		memPct := 0.0
		if p.MemoryInfo.Total > 0 {
			memPct = (float64(p.MemoryInfo.Used) / float64(p.MemoryInfo.Total)) * 100
		}

		out[key] = sysInfoStats{
			CPUUsagePct:   cpuPct,
			MemUsagePct:   memPct,
			MemTotalBytes: p.MemoryInfo.Total,
			MemUsedBytes:  p.MemoryInfo.Used,
			Hostname:      p.HostInfo.Hostname,
		}
	}
	return out, nil
}

type netInfoChange struct {
	AgentID    uint
	Field      string // "public_ip", "isp", "interface"
	OldValue   string
	NewValue   string
	DetectedAt time.Time
}

// getLatestNetInfoForAgents returns the most recent netInfoPayload for each
// agent in agentIDs whose created_at >= from, in a single ClickHouse query.
// agent_id is not in the probe_data primary key, so per-agent round-trips
// become O(N×M) as the table grows. The query below filters by type and a
// tight created_at range — both indexed — then takes the newest row per
// agent with row_number(). An agent with no rows in the window is omitted
// from the result map.
func getLatestNetInfoForAgents(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) map[uint]*netInfoPayload {
	out := make(map[uint]*netInfoPayload)
	if len(agentIDs) == 0 {
		return out
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	q := fmt.Sprintf(`
SELECT agent_id, payload_raw
FROM (
    SELECT agent_id, payload_raw,
           row_number() OVER (PARTITION BY agent_id ORDER BY created_at DESC) AS rn
    FROM probe_data
    WHERE type = 'NETINFO'
      AND agent_id IN (%s)
      AND created_at >= %s
)
WHERE rn = 1
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		log.Warnf("[analysis] getLatestNetInfoForAgents query error: %v", err)
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var agentID uint64
		var payloadRaw string
		if err := rows.Scan(&agentID, &payloadRaw); err != nil || payloadRaw == "" {
			continue
		}
		var p netInfoPayload
		if err := json.Unmarshal([]byte(payloadRaw), &p); err != nil {
			continue
		}
		p.NormalizeFromLegacy()
		out[uint(agentID)] = &p
	}
	return out
}

func getWorkspaceNetInfoChanges(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) ([]netInfoChange, error) {
	if len(agentIDs) == 0 {
		return nil, nil
	}
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	// Get last 2 records per agent to detect changes. Using a window
	// function keeps the result set to at most 2*|agents| rows and lets
	// ClickHouse prune by (type, created_at) from the primary key before
	// the per-agent filter is applied.
	q := fmt.Sprintf(`
SELECT agent_id, payload_raw, created_at
FROM (
    SELECT agent_id, payload_raw, created_at,
           row_number() OVER (PARTITION BY agent_id ORDER BY created_at DESC) AS rn
    FROM probe_data
    WHERE type = 'NETINFO'
      AND agent_id IN (%s)
      AND created_at >= %s
)
WHERE rn <= 2
ORDER BY agent_id, created_at DESC
`, strings.Join(agentIDStrs, ", "), chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect per-agent: newest and second-newest
	type record struct {
		payload   netInfoPayload
		createdAt time.Time
	}
	byAgent := make(map[uint][]record)

	for rows.Next() {
		var agentID uint64
		var payloadRaw string
		var createdAt time.Time
		if err := rows.Scan(&agentID, &payloadRaw, &createdAt); err != nil || payloadRaw == "" {
			continue
		}
		var p netInfoPayload
		if err := json.Unmarshal([]byte(payloadRaw), &p); err != nil {
			continue
		}
		aid := uint(agentID)
		if len(byAgent[aid]) < 2 {
			byAgent[aid] = append(byAgent[aid], record{payload: p, createdAt: createdAt})
		}
	}

	var changes []netInfoChange
	for aid, records := range byAgent {
		if len(records) < 2 {
			continue
		}
		newer := records[0] // latest
		older := records[1] // previous

		if newer.payload.PublicAddress != older.payload.PublicAddress && newer.payload.PublicAddress != "" {
			changes = append(changes, netInfoChange{
				AgentID:    aid,
				Field:      "public_ip",
				OldValue:   older.payload.PublicAddress,
				NewValue:   newer.payload.PublicAddress,
				DetectedAt: newer.createdAt,
			})
		}
		newISP := newer.payload.GetISP()
		oldISP := older.payload.GetISP()
		if newISP != oldISP && newISP != "" && oldISP != "" {
			changes = append(changes, netInfoChange{
				AgentID:    aid,
				Field:      "isp",
				OldValue:   oldISP,
				NewValue:   newISP,
				DetectedAt: newer.createdAt,
			})
		}
	}
	return changes, nil
}

// ── Scoring Helpers ──

// latencyScore converts raw latency (ms) to a 0-100 score
func latencyScore(latMs float64) float64 {
	switch {
	case latMs <= 0:
		return 100
	case latMs < 20:
		return 100
	case latMs < 50:
		return 100 - (latMs-20)*0.17 // 95 at 50ms
	case latMs < 100:
		return 95 - (latMs-50)*0.3 // 80 at 100ms
	case latMs < 150:
		return 80 - (latMs-100)*0.4 // 60 at 150ms
	case latMs < 300:
		return 60 - (latMs-150)*0.2 // 30 at 300ms
	default:
		return math.Max(0, 30-(latMs-300)*0.1)
	}
}

// speedtestBandwidthScore scores download+upload bandwidth (Mbps), 0-100
func speedtestBandwidthScore(dlMbps, ulMbps float64) float64 {
	// Weighted: download 70%, upload 30%
	dlScore := bwScore(dlMbps)
	ulScore := bwScore(ulMbps)
	return 0.7*dlScore + 0.3*ulScore
}

func bwScore(mbps float64) float64 {
	switch {
	case mbps >= 100:
		return 100
	case mbps >= 50:
		return 90 + (mbps-50)*0.2 // 90-100
	case mbps >= 25:
		return 75 + (mbps-25)*0.6 // 75-90
	case mbps >= 10:
		return 50 + (mbps-10)*1.67 // 50-75
	case mbps >= 5:
		return 30 + (mbps-5)*4 // 30-50
	case mbps > 0:
		return mbps * 6 // 0-30
	default:
		return 0
	}
}

// sysInfoHealthScore converts CPU/memory usage to a health score (higher = healthier)
func sysInfoHealthScore(si sysInfoStats) float64 {
	// CPU: <50% = 100, 50-80% = 80-60, 80-95% = 60-20, >95% = critical
	cpuScore := 100.0
	switch {
	case si.CPUUsagePct > 95:
		cpuScore = 10
	case si.CPUUsagePct > 80:
		cpuScore = 60 - (si.CPUUsagePct-80)*2.67
	case si.CPUUsagePct > 50:
		cpuScore = 100 - (si.CPUUsagePct-50)*1.33
	}

	// Memory: <60% = 100, 60-85% = 80-50, 85-95% = 50-20, >95% = critical
	memScore := 100.0
	switch {
	case si.MemUsagePct > 95:
		memScore = 10
	case si.MemUsagePct > 85:
		memScore = 50 - (si.MemUsagePct-85)*3
	case si.MemUsagePct > 60:
		memScore = 100 - (si.MemUsagePct-60)*2
	}

	return 0.5*cpuScore + 0.5*memScore
}
