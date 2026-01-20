// internal/probe/connectivity_matrix.go
// Aggregation functions for workspace connectivity matrix visualization
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
	"gorm.io/gorm"
)

// ProbeStatusSummary represents the status of a single probe type for a connection
type ProbeStatusSummary struct {
	Type        string  `json:"type"`         // MTR, PING, TRAFFICSIM
	Status      string  `json:"status"`       // healthy, degraded, critical, unknown
	AvgLatency  float64 `json:"avg_latency"`  // ms
	PacketLoss  float64 `json:"packet_loss"`  // percentage
	Jitter      float64 `json:"jitter"`       // ms (where available)
	LastUpdated string  `json:"last_updated"` // ISO timestamp
}

// ConnectivityMatrixEntry represents one cell in the connectivity matrix
type ConnectivityMatrixEntry struct {
	SourceAgentID   uint                 `json:"source_agent_id"`
	SourceAgentName string               `json:"source_agent_name"`
	TargetID        string               `json:"target_id"` // "agent:<id>" or IP/hostname
	TargetName      string               `json:"target_name"`
	TargetType      string               `json:"target_type"` // "agent" or "destination"
	ProbeStatus     []ProbeStatusSummary `json:"probe_status"`
}

// AgentSummary is a lightweight agent representation for the matrix
type AgentSummary struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	IsOnline bool   `json:"is_online"`
}

// TargetLabel represents a column header in the matrix
type TargetLabel struct {
	ID   string `json:"id"`   // "agent:<id>" or target string
	Name string `json:"name"` // Display label
	Type string `json:"type"` // "agent" or "destination"
}

// ConnectivityMatrix contains the complete matrix data for a workspace
type ConnectivityMatrix struct {
	SourceAgents []AgentSummary            `json:"source_agents"`
	TargetLabels []TargetLabel             `json:"target_labels"`
	Entries      []ConnectivityMatrixEntry `json:"entries"`
	GeneratedAt  time.Time                 `json:"generated_at"`
	WorkspaceID  uint                      `json:"workspace_id"`
}

// GetWorkspaceConnectivityMatrix builds the connectivity matrix from probe data
func GetWorkspaceConnectivityMatrix(ctx context.Context, ch *sql.DB, pg *gorm.DB, workspaceID uint, lookbackMinutes int) (*ConnectivityMatrix, error) {
	if lookbackMinutes <= 0 {
		lookbackMinutes = 15
	}

	from := time.Now().UTC().Add(-time.Duration(lookbackMinutes) * time.Minute)

	// 1. Get all agents in this workspace
	agents, err := getWorkspaceAgents(ctx, pg, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get agents: %w", err)
	}

	if len(agents) == 0 {
		return &ConnectivityMatrix{
			SourceAgents: []AgentSummary{},
			TargetLabels: []TargetLabel{},
			Entries:      []ConnectivityMatrixEntry{},
			GeneratedAt:  time.Now().UTC(),
			WorkspaceID:  workspaceID,
		}, nil
	}

	// Build agent lookup
	agentIDs := make([]uint, len(agents))
	agentByID := make(map[uint]agentInfo)
	for i, a := range agents {
		agentIDs[i] = a.ID
		agentByID[a.ID] = a
	}

	// Build source agents list
	sourceAgents := make([]AgentSummary, len(agents))
	for i, a := range agents {
		sourceAgents[i] = AgentSummary{
			ID:       a.ID,
			Name:     a.Name,
			IsOnline: time.Since(a.UpdatedAt) < time.Minute,
		}
	}

	// 2. Get PING metrics
	pingMetrics, err := getWorkspacePingMetrics(ctx, ch, agentIDs, from)
	if err != nil {
		pingMetrics = make(map[string]pingStats)
	}

	// 3. Get TrafficSim metrics
	trafficMetrics, err := getWorkspaceTrafficSimMetrics(ctx, ch, agentIDs, from)
	if err != nil {
		trafficMetrics = make(map[string]trafficStats)
	}

	// 4. Get MTR metrics
	mtrMetrics, err := getWorkspaceMTRMetrics(ctx, ch, pg, agentIDs, from)
	if err != nil {
		mtrMetrics = make(map[string]mtrStats)
	}

	// 5. Build the matrix entries
	// Collect all unique targets
	targetSet := make(map[string]TargetLabel)
	entriesMap := make(map[string]*ConnectivityMatrixEntry) // key: "sourceID:targetID"

	// Process PING data
	processProbeMetrics(pingMetrics, "PING", agentByID, targetSet, entriesMap, func(key string) (float64, float64, float64, time.Time) {
		s := pingMetrics[key]
		return s.AvgLatency, s.PacketLoss, 0, time.Now() // No jitter for PING
	})

	// Process TrafficSim data (no jitter available in current struct)
	processProbeMetrics(trafficMetrics, "TRAFFICSIM", agentByID, targetSet, entriesMap, func(key string) (float64, float64, float64, time.Time) {
		s := trafficMetrics[key]
		return s.AvgRTT, s.PacketLoss, 0, time.Now() // Jitter not available in trafficStats
	})

	// Process MTR data
	processProbeMetrics(mtrMetrics, "MTR", agentByID, targetSet, entriesMap, func(key string) (float64, float64, float64, time.Time) {
		s := mtrMetrics[key]
		return s.AvgLatency, s.PacketLoss, s.Jitter, s.LastUpdated
	})

	// Convert maps to slices
	targetLabels := make([]TargetLabel, 0, len(targetSet))
	for _, t := range targetSet {
		targetLabels = append(targetLabels, t)
	}

	entries := make([]ConnectivityMatrixEntry, 0, len(entriesMap))
	for _, e := range entriesMap {
		entries = append(entries, *e)
	}

	return &ConnectivityMatrix{
		SourceAgents: sourceAgents,
		TargetLabels: targetLabels,
		Entries:      entries,
		GeneratedAt:  time.Now().UTC(),
		WorkspaceID:  workspaceID,
	}, nil
}

// mtrStats holds aggregated MTR metrics for a source-target pair
type mtrStats struct {
	AvgLatency  float64
	PacketLoss  float64
	Jitter      float64
	Count       int
	TargetAgent uint
	LastUpdated time.Time
}

// getWorkspaceMTRMetrics fetches and aggregates MTR data for the matrix
func getWorkspaceMTRMetrics(ctx context.Context, ch *sql.DB, pg *gorm.DB, agentIDs []uint, from time.Time) (map[string]mtrStats, error) {
	if len(agentIDs) == 0 {
		return make(map[string]mtrStats), nil
	}

	// Build agent ID IN clause
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	q := fmt.Sprintf(`
SELECT 
    agent_id,
    target,
    target_agent,
    payload_raw,
    created_at
FROM probe_data
WHERE type = 'MTR'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 500
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		log.Printf("[ConnectivityMatrix] MTR query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	// Aggregate per agent+target
	type mtrAccum struct {
		totalLatency float64
		totalLoss    float64
		totalJitter  float64
		count        int
		targetAgent  uint
		lastUpdated  time.Time
	}
	accum := make(map[string]*mtrAccum)

	for rows.Next() {
		var agentID uint64
		var target string
		var targetAgent uint64
		var payloadRaw string
		var createdAt time.Time

		if err := rows.Scan(&agentID, &target, &targetAgent, &payloadRaw, &createdAt); err != nil {
			continue
		}

		if payloadRaw == "" {
			continue
		}

		// Parse MTR payload
		var payload mtrPayload
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}

		// Use target from payload if empty
		if target == "" {
			target = payload.Report.Info.Target.Hostname
			if target == "" {
				target = payload.Report.Info.Target.IP
			}
		}
		if target == "" {
			continue
		}

		// Calculate metrics from last hop
		if len(payload.Report.Hops) == 0 {
			continue
		}
		lastHop := payload.Report.Hops[len(payload.Report.Hops)-1]
		latency := parseFloat(lastHop.Avg)
		loss := parseFloat(lastHop.LossPct)
		jitter := parseFloat(lastHop.StdDev) // Use StdDev as jitter approximation

		key := fmt.Sprintf("%d:%s", agentID, target)
		if accum[key] == nil {
			accum[key] = &mtrAccum{targetAgent: uint(targetAgent), lastUpdated: createdAt}
		}
		accum[key].totalLatency += latency
		accum[key].totalLoss += loss
		accum[key].totalJitter += jitter
		accum[key].count++
		if createdAt.After(accum[key].lastUpdated) {
			accum[key].lastUpdated = createdAt
		}
	}

	results := make(map[string]mtrStats)
	for key, a := range accum {
		if a.count > 0 {
			results[key] = mtrStats{
				AvgLatency:  sanitizeFloat(a.totalLatency / float64(a.count)),
				PacketLoss:  sanitizeFloat(a.totalLoss / float64(a.count)),
				Jitter:      sanitizeFloat(a.totalJitter / float64(a.count)),
				Count:       a.count,
				TargetAgent: a.targetAgent,
				LastUpdated: a.lastUpdated,
			}
		}
	}

	return results, rows.Err()
}

// processProbeMetrics is a generic function to process probe metrics into matrix entries
func processProbeMetrics[T any](
	metrics map[string]T,
	probeType string,
	agentByID map[uint]agentInfo,
	targetSet map[string]TargetLabel,
	entriesMap map[string]*ConnectivityMatrixEntry,
	getMetrics func(key string) (latency, loss, jitter float64, lastUpdated time.Time),
) {
	for key := range metrics {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}

		var sourceAgentID uint
		fmt.Sscanf(parts[0], "%d", &sourceAgentID)
		target := stripPort(parts[1])

		if target == "" {
			continue
		}

		sourceAgent, ok := agentByID[sourceAgentID]
		if !ok {
			continue
		}

		// Determine if target is another agent
		targetID := target
		targetName := target
		targetType := "destination"

		// Check if target matches any agent's public IP
		for _, agent := range agentByID {
			if agent.PublicIPOverride == target {
				targetID = fmt.Sprintf("agent:%d", agent.ID)
				targetName = agent.Name
				targetType = "agent"
				break
			}
		}

		// Add to target set
		if _, exists := targetSet[targetID]; !exists {
			targetSet[targetID] = TargetLabel{
				ID:   targetID,
				Name: targetName,
				Type: targetType,
			}
		}

		// Get metrics
		latency, loss, jitter, lastUpdated := getMetrics(key)

		// Determine status
		status := calculateProbeStatus(latency, loss)

		// Create or update entry
		entryKey := fmt.Sprintf("%d:%s", sourceAgentID, targetID)
		if entriesMap[entryKey] == nil {
			entriesMap[entryKey] = &ConnectivityMatrixEntry{
				SourceAgentID:   sourceAgentID,
				SourceAgentName: sourceAgent.Name,
				TargetID:        targetID,
				TargetName:      targetName,
				TargetType:      targetType,
				ProbeStatus:     []ProbeStatusSummary{},
			}
		}

		// Check if this probe type already exists
		found := false
		for i, ps := range entriesMap[entryKey].ProbeStatus {
			if ps.Type == probeType {
				// Update with averaged metrics
				entriesMap[entryKey].ProbeStatus[i].AvgLatency = (ps.AvgLatency + latency) / 2
				entriesMap[entryKey].ProbeStatus[i].PacketLoss = (ps.PacketLoss + loss) / 2
				if jitter > 0 {
					entriesMap[entryKey].ProbeStatus[i].Jitter = (ps.Jitter + jitter) / 2
				}
				entriesMap[entryKey].ProbeStatus[i].Status = calculateProbeStatus(
					entriesMap[entryKey].ProbeStatus[i].AvgLatency,
					entriesMap[entryKey].ProbeStatus[i].PacketLoss,
				)
				found = true
				break
			}
		}

		if !found {
			entriesMap[entryKey].ProbeStatus = append(entriesMap[entryKey].ProbeStatus, ProbeStatusSummary{
				Type:        probeType,
				Status:      status,
				AvgLatency:  sanitizeFloat(latency),
				PacketLoss:  sanitizeFloat(loss),
				Jitter:      sanitizeFloat(jitter),
				LastUpdated: lastUpdated.Format(time.RFC3339),
			})
		}
	}
}

// calculateProbeStatus determines health status based on latency and packet loss
func calculateProbeStatus(latency, packetLoss float64) string {
	if math.IsNaN(latency) || math.IsNaN(packetLoss) {
		return "unknown"
	}

	// Critical: >25% loss or >200ms latency
	if packetLoss > 25 || latency > 200 {
		return "critical"
	}

	// Degraded: 5-25% loss or 100-200ms latency
	if packetLoss >= 5 || latency >= 100 {
		return "degraded"
	}

	// Healthy: <5% loss and <100ms latency
	return "healthy"
}
