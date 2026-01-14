// internal/probe/network_map.go
// Aggregation functions for workspace-level network topology visualization
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

// sanitizeFloat replaces NaN and Infinity with 0
func sanitizeFloat(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// stripPort removes the port suffix from a target if present
// e.g., "108.165.150.19:5000" -> "108.165.150.19"
func stripPort(target string) string {
	// Handle IPv6 with port: [::1]:8080
	if strings.HasPrefix(target, "[") {
		if idx := strings.LastIndex(target, "]:"); idx != -1 {
			return target[:idx+1]
		}
		return target
	}
	// Handle IPv4/hostname with port
	if idx := strings.LastIndex(target, ":"); idx != -1 {
		// Ensure it's not just an IPv6 address without brackets
		if strings.Count(target, ":") == 1 {
			return target[:idx]
		}
	}
	return target
}

// NetworkMapNode represents a node in the network topology map
type NetworkMapNode struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"` // "agent", "hop", "destination"
	Label      string  `json:"label"`
	AgentID    *uint   `json:"agent_id,omitempty"`
	IP         string  `json:"ip,omitempty"`
	Hostname   string  `json:"hostname,omitempty"`
	HopNumber  int     `json:"hop_number,omitempty"`
	AvgLatency float64 `json:"avg_latency"`
	PacketLoss float64 `json:"packet_loss"`
	PathCount  int     `json:"path_count"`
	IsOnline   bool    `json:"is_online,omitempty"`
	// Visualization fields
	Layer  int    `json:"layer,omitempty"`  // 0=agent, 1-N=hops, 100=destination
	Status string `json:"status,omitempty"` // "healthy", "degraded", "critical"
	// Shared hop tracking
	SharedAgents []uint   `json:"shared_agents,omitempty"` // Agent IDs that traverse this hop
	PathIDs      []string `json:"path_ids,omitempty"`      // Traceroute paths through this hop
}

// NetworkMapEdge represents an edge (link) between nodes
type NetworkMapEdge struct {
	ID         string  `json:"id"`
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	AvgLatency float64 `json:"avg_latency"`
	PacketLoss float64 `json:"packet_loss"`
	PathCount  int     `json:"path_count"`
	PathID     string  `json:"path_id,omitempty"` // Unique path identifier (agent:target)
}

// DestinationSummary provides quick overview of a destination's health
type DestinationSummary struct {
	Target      string   `json:"target"`
	Hostname    string   `json:"hostname,omitempty"`
	HopCount    int      `json:"hop_count"`
	AvgLatency  float64  `json:"avg_latency"` // Combined from PING + TrafficSim + MTR
	PacketLoss  float64  `json:"packet_loss"`
	Status      string   `json:"status"`      // "healthy", "degraded", "critical"
	AgentCount  int      `json:"agent_count"` // Number of agents testing this
	ProbeTypes  []string `json:"probe_types"` // ["MTR", "PING", "TRAFFICSIM"]
	LastUpdated string   `json:"last_updated,omitempty"`
}

// NetworkMapData contains the complete topology data for a workspace
type NetworkMapData struct {
	Nodes        []NetworkMapNode     `json:"nodes"`
	Edges        []NetworkMapEdge     `json:"edges"`
	Destinations []DestinationSummary `json:"destinations"` // Quick overview panel
	GeneratedAt  time.Time            `json:"generated_at"`
	WorkspaceID  uint                 `json:"workspace_id"`
}

// Agent model for querying (simplified)
type agentInfo struct {
	ID               uint
	Name             string
	PublicIPOverride string `gorm:"column:public_ip_override"`
	Location         string
	UpdatedAt        time.Time
}

// GetWorkspaceNetworkMap builds aggregated network topology from MTR/PING/TrafficSim data
func GetWorkspaceNetworkMap(ctx context.Context, ch *sql.DB, pg *gorm.DB, workspaceID uint, lookbackMinutes int) (*NetworkMapData, error) {
	if lookbackMinutes <= 0 {
		lookbackMinutes = 60 // Default to 1 hour of data
	}

	from := time.Now().UTC().Add(-time.Duration(lookbackMinutes) * time.Minute)

	// 1. Get all agents in this workspace from Postgres
	agents, err := getWorkspaceAgents(ctx, pg, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get agents: %w", err)
	}

	// Extract agent IDs for filtering ClickHouse queries
	agentIDs := make([]uint, len(agents))
	for i, a := range agents {
		agentIDs[i] = a.ID
	}

	// If no agents, return empty map
	if len(agentIDs) == 0 {
		return &NetworkMapData{
			Nodes:       []NetworkMapNode{},
			Edges:       []NetworkMapEdge{},
			GeneratedAt: time.Now().UTC(),
			WorkspaceID: workspaceID,
		}, nil
	}

	// 2. Get MTR data from ClickHouse (filtered by workspace agents)
	mtrData, err := getWorkspaceMTRData(ctx, ch, pg, agentIDs, from)
	if err != nil {
		// Non-fatal - MTR data is optional
		mtrData = []mtrTrace{}
	}

	// 3. Get PING metrics for overlay
	pingMetrics, err := getWorkspacePingMetrics(ctx, ch, agentIDs, from)
	if err != nil {
		// Non-fatal, continue without ping overlay
		pingMetrics = make(map[string]pingStats)
	}

	// 4. Get TrafficSim metrics for overlay
	trafficMetrics, err := getWorkspaceTrafficSimMetrics(ctx, ch, agentIDs, from)
	if err != nil {
		// Non-fatal, continue without traffic sim overlay
		trafficMetrics = make(map[string]trafficStats)
	}

	// 5. Build the topology graph
	mapData := buildNetworkMap(agents, mtrData, pingMetrics, trafficMetrics, workspaceID)

	return mapData, nil
}

func getWorkspaceAgents(ctx context.Context, pg *gorm.DB, workspaceID uint) ([]agentInfo, error) {
	var agents []agentInfo
	err := pg.WithContext(ctx).
		Table("agents").
		Select("id, name, public_ip_override, location, updated_at").
		Where("workspace_id = ?", workspaceID).
		Scan(&agents).Error
	if err != nil {
		return nil, err
	}
	return agents, nil
}

// mtrHop represents a single hop in an MTR trace
type mtrHop struct {
	IP         string
	Hostname   string
	AvgLatency float64
	PacketLoss float64
}

// mtrTrace represents a complete MTR trace from agent to target
type mtrTrace struct {
	AgentID     uint
	Target      string
	TargetAgent uint // 0 if not agent-to-agent probe
	Hops        []mtrHop
}

func getWorkspaceMTRData(ctx context.Context, ch *sql.DB, pg *gorm.DB, agentIDs []uint, from time.Time) ([]mtrTrace, error) {
	// Query raw MTR payloads filtered by workspace agents
	if len(agentIDs) == 0 {
		return []mtrTrace{}, nil
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
    probe_id,
    payload_raw
FROM probe_data
WHERE type = 'MTR'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 1000
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		log.Printf("[NetworkMap] MTR query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	// Cache probe targets to avoid repeated DB lookups
	probeTargetCache := make(map[uint]string)

	// Keep only the latest trace per agent+target
	seenPaths := make(map[string]bool)
	var traces []mtrTrace
	rowCount := 0

	for rows.Next() {
		rowCount++
		var agentID uint64
		var target string
		var targetAgent uint64
		var probeID uint64
		var payloadRaw string

		if err := rows.Scan(&agentID, &target, &targetAgent, &probeID, &payloadRaw); err != nil {
			log.Printf("[NetworkMap] Row scan error: %v", err)
			continue
		}

		// Parse MTR payload first so we can extract target if empty
		var payload mtrPayload
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			log.Printf("[NetworkMap] JSON parse error for agent %d: %v", agentID, err)
			continue
		}

		// Target resolution priority:
		// 1. Database target column (if agent sent it)
		// 2. Probe definition target (original hostname like google.com)
		// 3. MTR payload resolved IP (fallback - may differ from original target)
		if target == "" && probeID > 0 && pg != nil {
			// Look up original target from probe definition
			if cachedTarget, ok := probeTargetCache[uint(probeID)]; ok {
				target = cachedTarget
			} else {
				var probeTargets []Target
				if err := pg.WithContext(ctx).Where("probe_id = ?", probeID).Limit(1).Find(&probeTargets).Error; err == nil && len(probeTargets) > 0 {
					if probeTargets[0].Target != "" {
						target = probeTargets[0].Target
					}
				}
				probeTargetCache[uint(probeID)] = target
			}
		}

		// Fallback to MTR payload if still empty
		if target == "" {
			target = payload.Report.Info.Target.Hostname
			if target == "" {
				target = payload.Report.Info.Target.IP
			}
		}

		// Skip if still no target
		if target == "" {
			log.Printf("[NetworkMap] Agent %d: no target found in DB, probe, or payload, skipping", agentID)
			continue
		}

		pathKey := fmt.Sprintf("%d:%s", agentID, target)
		if seenPaths[pathKey] {
			continue // Already have latest trace for this path
		}
		seenPaths[pathKey] = true

		log.Printf("[NetworkMap] Agent %d -> %s parsed: %d hops in payload.Report.Hops", agentID, target, len(payload.Report.Hops))

		// Build ordered hop list
		var hops []mtrHop
		for _, hop := range payload.Report.Hops {
			var ip, hostname string
			if len(hop.Hosts) > 0 {
				ip = hop.Hosts[0].IP
				hostname = hop.Hosts[0].Hostname
			}
			hops = append(hops, mtrHop{
				IP:         ip,
				Hostname:   hostname,
				AvgLatency: parseFloat(hop.Avg),
				PacketLoss: parseFloat(hop.LossPct),
			})
		}

		log.Printf("[NetworkMap] Trace: agent %d -> %s has %d hops", agentID, target, len(hops))
		traces = append(traces, mtrTrace{
			AgentID:     uint(agentID),
			Target:      target,
			TargetAgent: uint(targetAgent),
			Hops:        hops,
		})
	}

	log.Printf("[NetworkMap] MTR query returned %d rows, parsed %d traces", rowCount, len(traces))
	return traces, rows.Err()
}

type pingStats struct {
	AvgLatency float64
	PacketLoss float64
	Count      int
}

func getWorkspacePingMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) (map[string]pingStats, error) {
	if len(agentIDs) == 0 {
		return make(map[string]pingStats), nil
	}

	// Build agent ID IN clause
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	// Fetch raw payloads and aggregate in Go
	q := fmt.Sprintf(`
SELECT 
    agent_id,
    target,
    payload_raw
FROM probe_data
WHERE type = 'PING'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 5000
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Aggregate in Go
	type pingAccum struct {
		totalLatency float64
		totalLoss    float64
		count        int
	}
	accum := make(map[string]*pingAccum)

	for rows.Next() {
		var agentID uint64
		var target string
		var payloadRaw string

		if err := rows.Scan(&agentID, &target, &payloadRaw); err != nil {
			continue
		}

		if payloadRaw == "" {
			continue
		}

		// Parse ping payload
		var payload struct {
			AvgRTT     int64   `json:"avg_rtt"`     // nanoseconds
			PacketLoss float64 `json:"packet_loss"` // percentage
		}
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}

		key := fmt.Sprintf("%d:%s", agentID, target)
		if accum[key] == nil {
			accum[key] = &pingAccum{}
		}
		accum[key].totalLatency += float64(payload.AvgRTT) / 1000000.0 // ns to ms
		accum[key].totalLoss += payload.PacketLoss
		accum[key].count++
	}

	results := make(map[string]pingStats)
	for key, a := range accum {
		if a.count > 0 {
			results[key] = pingStats{
				AvgLatency: a.totalLatency / float64(a.count),
				PacketLoss: a.totalLoss / float64(a.count),
				Count:      a.count,
			}
		}
	}

	return results, rows.Err()
}

type trafficStats struct {
	AvgRTT      float64
	PacketLoss  float64
	Count       int
	TargetAgent uint // Track if this is targeting another agent
}

func getWorkspaceTrafficSimMetrics(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) (map[string]trafficStats, error) {
	if len(agentIDs) == 0 {
		return make(map[string]trafficStats), nil
	}

	// Build agent ID IN clause
	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(agentIDStrs, ", ")

	// Fetch raw payloads and aggregate in Go
	q := fmt.Sprintf(`
SELECT 
    agent_id,
    target,
    target_agent,
    payload_raw
FROM probe_data
WHERE type = 'TRAFFICSIM'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 5000
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Aggregate in Go
	type trafficAccum struct {
		totalRTT    float64
		totalLoss   float64
		count       int
		targetAgent uint
	}
	accum := make(map[string]*trafficAccum)

	for rows.Next() {
		var agentID uint64
		var target string
		var targetAgent uint64
		var payloadRaw string

		if err := rows.Scan(&agentID, &target, &targetAgent, &payloadRaw); err != nil {
			continue
		}

		if payloadRaw == "" {
			continue
		}

		// Parse trafficsim payload
		var payload struct {
			AverageRTT     float64 `json:"averageRTT"`     // milliseconds
			LossPercentage float64 `json:"lossPercentage"` // percentage
		}
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}

		key := fmt.Sprintf("%d:%s", agentID, target)
		if accum[key] == nil {
			accum[key] = &trafficAccum{targetAgent: uint(targetAgent)}
		}
		accum[key].totalRTT += payload.AverageRTT
		accum[key].totalLoss += payload.LossPercentage
		accum[key].count++
	}

	results := make(map[string]trafficStats)
	for key, a := range accum {
		if a.count > 0 {
			results[key] = trafficStats{
				AvgRTT:      a.totalRTT / float64(a.count),
				PacketLoss:  a.totalLoss / float64(a.count),
				Count:       a.count,
				TargetAgent: a.targetAgent,
			}
		}
	}

	return results, rows.Err()
}

func buildNetworkMap(agents []agentInfo, mtrData []mtrTrace, pingMetrics map[string]pingStats, trafficMetrics map[string]trafficStats, workspaceID uint) *NetworkMapData {
	nodeMap := make(map[string]*NetworkMapNode)
	edgeMap := make(map[string]*NetworkMapEdge)

	// Track destination metrics for summary
	destMetrics := make(map[string]*DestinationSummary)
	destAgents := make(map[string]map[uint]bool)
	destProbes := make(map[string]map[string]bool)

	// Create agent lookup for resolving target agent IPs
	agentByID := make(map[uint]agentInfo)
	for _, agent := range agents {
		agentByID[agent.ID] = agent
	}

	// Add agent nodes (Layer 0)
	for _, agent := range agents {
		nodeID := fmt.Sprintf("agent:%d", agent.ID)
		isOnline := time.Since(agent.UpdatedAt) < time.Minute
		status := "healthy"
		if !isOnline {
			status = "unknown"
		}
		nodeMap[nodeID] = &NetworkMapNode{
			ID:        nodeID,
			Type:      "agent",
			Label:     agent.Name,
			AgentID:   &agent.ID,
			IP:        agent.PublicIPOverride,
			IsOnline:  isOnline,
			PathCount: 0,
			Layer:     0,
			Status:    status,
		}
	}

	// Aggregate bidirectional metrics for each agent (probes targeting this agent)
	// Build a map of agent IP -> agent ID for reverse lookup
	agentIPToID := make(map[string]uint)
	for _, agent := range agents {
		if agent.PublicIPOverride != "" {
			agentIPToID[agent.PublicIPOverride] = agent.ID
		}
	}

	// Aggregate PING metrics targeting each agent
	for key, stats := range pingMetrics {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		target := stripPort(parts[1])
		if targetAgentID, ok := agentIPToID[target]; ok {
			nodeID := fmt.Sprintf("agent:%d", targetAgentID)
			if node, exists := nodeMap[nodeID]; exists {
				// Average in the new metrics
				if node.PathCount == 0 {
					node.AvgLatency = stats.AvgLatency
					node.PacketLoss = stats.PacketLoss
				} else {
					node.AvgLatency = (node.AvgLatency*float64(node.PathCount) + stats.AvgLatency) / float64(node.PathCount+1)
					node.PacketLoss = (node.PacketLoss*float64(node.PathCount) + stats.PacketLoss) / float64(node.PathCount+1)
				}
				node.PathCount++

				// Update status based on metrics
				if node.PacketLoss >= 50 {
					node.Status = "critical"
				} else if node.PacketLoss >= 10 || node.AvgLatency > 100 {
					node.Status = "degraded"
				} else if node.IsOnline {
					node.Status = "healthy"
				}
			}
		}
	}

	// Aggregate TrafficSim metrics targeting each agent
	for key, stats := range trafficMetrics {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		target := stripPort(parts[1])
		if targetAgentID, ok := agentIPToID[target]; ok {
			nodeID := fmt.Sprintf("agent:%d", targetAgentID)
			if node, exists := nodeMap[nodeID]; exists {
				if node.PathCount == 0 {
					node.AvgLatency = stats.AvgRTT
					node.PacketLoss = stats.PacketLoss
				} else {
					node.AvgLatency = (node.AvgLatency*float64(node.PathCount) + stats.AvgRTT) / float64(node.PathCount+1)
					node.PacketLoss = (node.PacketLoss*float64(node.PathCount) + stats.PacketLoss) / float64(node.PathCount+1)
				}
				node.PathCount++

				if node.PacketLoss >= 50 {
					node.Status = "critical"
				} else if node.PacketLoss >= 10 || node.AvgLatency > 100 {
					node.Status = "degraded"
				} else if node.IsOnline {
					node.Status = "healthy"
				}
			}
		}
	}

	// Aggregate MTR final hop metrics targeting each agent
	for _, trace := range mtrData {
		if trace.TargetAgent > 0 && len(trace.Hops) > 0 {
			// Use the last hop's metrics for the target agent
			lastHop := trace.Hops[len(trace.Hops)-1]
			nodeID := fmt.Sprintf("agent:%d", trace.TargetAgent)
			if node, exists := nodeMap[nodeID]; exists {
				if node.PathCount == 0 {
					node.AvgLatency = lastHop.AvgLatency
					node.PacketLoss = lastHop.PacketLoss
				} else {
					node.AvgLatency = (node.AvgLatency*float64(node.PathCount) + lastHop.AvgLatency) / float64(node.PathCount+1)
					node.PacketLoss = (node.PacketLoss*float64(node.PathCount) + lastHop.PacketLoss) / float64(node.PathCount+1)
				}
				node.PathCount++

				if node.PacketLoss >= 50 {
					node.Status = "critical"
				} else if node.PacketLoss >= 10 || node.AvgLatency > 100 {
					node.Status = "degraded"
				} else if node.IsOnline {
					node.Status = "healthy"
				}
			}
		}
	}

	// Track destinations and process MTR traces
	// Each trace is a complete path: agent → hop1 → hop2 → ... → destination
	for _, trace := range mtrData {
		if trace.Target == "" {
			continue
		}

		agentNodeID := fmt.Sprintf("agent:%d", trace.AgentID)
		pathID := fmt.Sprintf("%d:%s", trace.AgentID, trace.Target)

		// Determine destination key - for agent-to-agent, use target agent's NODE ID
		destKey := trace.Target
		destLabel := trace.Target
		isAgentTarget := false
		if trace.TargetAgent > 0 {
			if targetAgent, ok := agentByID[trace.TargetAgent]; ok {
				// Use agent node ID as destKey so edges connect to agent nodes
				destKey = fmt.Sprintf("agent:%d", trace.TargetAgent)
				destLabel = targetAgent.Name
				isAgentTarget = true
			}
		}

		// Track destination for this path
		if destMetrics[destKey] == nil {
			destMetrics[destKey] = &DestinationSummary{
				Target:   destKey,
				Hostname: destLabel,
			}
			destAgents[destKey] = make(map[uint]bool)
			destProbes[destKey] = make(map[string]bool)
		}
		destAgents[destKey][trace.AgentID] = true
		destProbes[destKey]["MTR"] = true
		destMetrics[destKey].HopCount = len(trace.Hops)

		// Process hops sequentially: agent → hop1 → hop2 → ... → lastHop
		prevNodeID := agentNodeID
		var lastHopID string

		log.Printf("[NetworkMap] Processing trace agent %d -> %s with %d hops", trace.AgentID, trace.Target, len(trace.Hops))

		// First pass: identify context for unknown hops (prev/next known IPs)
		// This allows unknowns to merge when they're between the same known infrastructure
		hopContexts := make([]struct {
			PrevKnownIP string
			NextKnownIP string
		}, len(trace.Hops))

		lastKnownIP := ""
		for i, hop := range trace.Hops {
			if hop.IP != "" {
				lastKnownIP = hop.IP
			}
			hopContexts[i].PrevKnownIP = lastKnownIP
		}

		// Reverse pass for next known IP
		nextKnownIP := destKey // destination is the final known IP
		for i := len(trace.Hops) - 1; i >= 0; i-- {
			if trace.Hops[i].IP != "" {
				nextKnownIP = trace.Hops[i].IP
			}
			hopContexts[i].NextKnownIP = nextKnownIP
		}

		for i, hop := range trace.Hops {
			// Generate hop node ID - known IPs merge by IP, unknowns merge by context
			var hopNodeID string
			isUnknown := hop.IP == ""

			if !isUnknown {
				hopNodeID = hop.IP // KEY BY IP for shared hop detection
			} else {
				// Unknown hop: key by surrounding known IPs so similar paths merge
				// Format: unknown:{prevKnownIP}:{nextKnownIP}
				ctx := hopContexts[i]
				hopNodeID = fmt.Sprintf("unknown:%s:%s", ctx.PrevKnownIP, ctx.NextKnownIP)
			}

			// Determine status based on metrics
			hopStatus := "healthy"
			if hop.PacketLoss >= 50 {
				hopStatus = "critical"
			} else if hop.PacketLoss >= 10 || hop.AvgLatency > 100 {
				hopStatus = "degraded"
			} else if isUnknown {
				hopStatus = "unknown"
			}

			// Create or update hop node
			if _, exists := nodeMap[hopNodeID]; !exists {
				label := hop.IP
				if isUnknown {
					label = "?"
				}
				nodeMap[hopNodeID] = &NetworkMapNode{
					ID:           hopNodeID,
					Type:         "hop",
					Label:        label,
					IP:           hop.IP,
					Hostname:     hop.Hostname,
					HopNumber:    0, // Don't track hop number since it varies by source
					AvgLatency:   hop.AvgLatency,
					PacketLoss:   hop.PacketLoss,
					PathCount:    1,
					Layer:        i + 1, // Use for initial positioning only
					Status:       hopStatus,
					SharedAgents: []uint{trace.AgentID},
					PathIDs:      []string{pathID},
				}
			} else {
				// Aggregate - this is a SHARED hop (same IP or same context from different paths)
				node := nodeMap[hopNodeID]
				node.AvgLatency = (node.AvgLatency*float64(node.PathCount) + hop.AvgLatency) / float64(node.PathCount+1)
				node.PacketLoss = (node.PacketLoss*float64(node.PathCount) + hop.PacketLoss) / float64(node.PathCount+1)
				node.PathCount++
				// Add agent to shared agents if not already present
				agentFound := false
				for _, a := range node.SharedAgents {
					if a == trace.AgentID {
						agentFound = true
						break
					}
				}
				if !agentFound {
					node.SharedAgents = append(node.SharedAgents, trace.AgentID)
				}
				node.PathIDs = append(node.PathIDs, pathID)
			}

			// Create edge from previous node to this hop
			edgeID := fmt.Sprintf("%s->%s", prevNodeID, hopNodeID)
			if _, exists := edgeMap[edgeID]; !exists {
				edgeMap[edgeID] = &NetworkMapEdge{
					ID:         edgeID,
					Source:     prevNodeID,
					Target:     hopNodeID,
					AvgLatency: hop.AvgLatency,
					PacketLoss: hop.PacketLoss,
					PathCount:  1,
					PathID:     pathID,
				}
			} else {
				edgeMap[edgeID].PathCount++
			}

			// Update for next iteration
			prevNodeID = hopNodeID
			lastHopID = hopNodeID
		}

		// Create destination node or upgrade existing hop to destination
		// Skip for agent targets - they already have agent nodes
		if !isAgentTarget {
			if existing, exists := nodeMap[destKey]; !exists {
				nodeMap[destKey] = &NetworkMapNode{
					ID:        destKey,
					Type:      "destination",
					Label:     destLabel,
					IP:        destKey,
					Hostname:  destLabel,
					PathCount: 1,
					Layer:     100, // Destinations on far right
					Status:    "healthy",
				}
			} else if existing.Type == "hop" {
				// Upgrade hop to destination - it's the final target
				existing.Type = "destination"
				existing.Label = destLabel
				existing.Hostname = destLabel
				existing.Layer = 100
			}
		}

		// Create edge from last hop to destination
		if lastHopID != "" && lastHopID != destKey {
			edgeID := fmt.Sprintf("%s->%s", lastHopID, destKey)
			if _, exists := edgeMap[edgeID]; !exists {
				edgeMap[edgeID] = &NetworkMapEdge{
					ID:        edgeID,
					Source:    lastHopID,
					Target:    destKey,
					PathCount: 1,
					PathID:    pathID,
				}
			} else {
				edgeMap[edgeID].PathCount++
			}
		} else if len(trace.Hops) == 0 {
			// No hops - direct connection from agent to destination
			edgeID := fmt.Sprintf("%s->%s", agentNodeID, destKey)
			if _, exists := edgeMap[edgeID]; !exists {
				edgeMap[edgeID] = &NetworkMapEdge{
					ID:        edgeID,
					Source:    agentNodeID,
					Target:    destKey,
					PathCount: 1,
					PathID:    pathID,
				}
			}
		}
	}

	// Track which destinations have MTR paths
	mtrDestinations := make(map[string]map[uint]bool) // dest -> agent IDs with MTR paths
	for _, trace := range mtrData {
		destKey := trace.Target
		if trace.TargetAgent > 0 {
			if targetAgent, ok := agentByID[trace.TargetAgent]; ok && targetAgent.PublicIPOverride != "" {
				destKey = targetAgent.PublicIPOverride
			}
		}
		if mtrDestinations[destKey] == nil {
			mtrDestinations[destKey] = make(map[uint]bool)
		}
		mtrDestinations[destKey][trace.AgentID] = true
	}

	// Use mtrDestinations to know which agent+dest pairs have MTR paths

	// Process PING metrics - update destination summaries and create edges
	for key, stats := range pingMetrics {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		agentID := parseUint(parts[0])
		target := stripPort(parts[1]) // Normalize target (remove port) for matching

		if destMetrics[target] == nil {
			destMetrics[target] = &DestinationSummary{Target: target}
			destAgents[target] = make(map[uint]bool)
			destProbes[target] = make(map[string]bool)
		}
		destAgents[target][agentID] = true
		destProbes[target]["PING"] = true

		// Average latency
		if destMetrics[target].AvgLatency == 0 {
			destMetrics[target].AvgLatency = stats.AvgLatency
		} else {
			destMetrics[target].AvgLatency = (destMetrics[target].AvgLatency + stats.AvgLatency) / 2
		}
		destMetrics[target].PacketLoss = (destMetrics[target].PacketLoss + stats.PacketLoss) / 2

		// Create destination node if not exists
		if _, exists := nodeMap[target]; !exists {
			nodeMap[target] = &NetworkMapNode{
				ID:        target,
				Type:      "destination",
				Label:     target,
				IP:        target,
				PathCount: 1,
				Layer:     100,
				Status:    "healthy",
			}
		}

		// Update node metrics
		if node, exists := nodeMap[target]; exists {
			node.AvgLatency = (node.AvgLatency + stats.AvgLatency) / 2
			node.PacketLoss = (node.PacketLoss + stats.PacketLoss) / 2
		}

		// Create direct agent-to-destination edge if no MTR path exists
		if mtrDestinations[target] == nil || !mtrDestinations[target][agentID] {
			agentNodeID := fmt.Sprintf("agent:%d", agentID)
			edgeID := fmt.Sprintf("%s->%s", agentNodeID, target)
			if _, exists := edgeMap[edgeID]; !exists {
				edgeMap[edgeID] = &NetworkMapEdge{
					ID:         edgeID,
					Source:     agentNodeID,
					Target:     target,
					AvgLatency: stats.AvgLatency,
					PacketLoss: stats.PacketLoss,
					PathCount:  1,
				}
			}
		}
	}

	// Process TrafficSim metrics
	for key, stats := range trafficMetrics {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		agentID := parseUint(parts[0])
		rawTarget := stripPort(parts[1]) // Normalize target (remove port) for matching

		// Resolve destination key - if targeting an agent, use agent node
		var destKey, destLabel string
		var destType string = "destination"

		if stats.TargetAgent > 0 {
			// Targeting another agent - use agent node ID
			destKey = fmt.Sprintf("agent:%d", stats.TargetAgent)
			destType = "agent"
			if targetAgent, ok := agentByID[stats.TargetAgent]; ok {
				destLabel = fmt.Sprintf("%s (%s)", targetAgent.Name, rawTarget)
				if targetAgent.PublicIPOverride != "" {
					rawTarget = targetAgent.PublicIPOverride
				}
			} else {
				destLabel = rawTarget
			}
		} else {
			// Regular destination
			destKey = rawTarget
			destLabel = rawTarget
		}

		if destMetrics[rawTarget] == nil {
			destMetrics[rawTarget] = &DestinationSummary{Target: rawTarget, Hostname: destLabel}
			destAgents[rawTarget] = make(map[uint]bool)
			destProbes[rawTarget] = make(map[string]bool)
		}
		destAgents[rawTarget][agentID] = true
		destProbes[rawTarget]["TRAFFICSIM"] = true

		// Update metrics
		if destMetrics[rawTarget].AvgLatency == 0 {
			destMetrics[rawTarget].AvgLatency = stats.AvgRTT
		} else {
			destMetrics[rawTarget].AvgLatency = (destMetrics[rawTarget].AvgLatency + stats.AvgRTT) / 2
		}
		destMetrics[rawTarget].PacketLoss = (destMetrics[rawTarget].PacketLoss + stats.PacketLoss) / 2

		// Only create destination node if NOT targeting an agent (agent nodes already exist)
		if destType == "destination" {
			if _, exists := nodeMap[destKey]; !exists {
				nodeMap[destKey] = &NetworkMapNode{
					ID:        destKey,
					Type:      "destination",
					Label:     destLabel,
					IP:        rawTarget,
					PathCount: 1,
					Layer:     100,
					Status:    "healthy",
				}
			}

			// Update node
			if node, exists := nodeMap[destKey]; exists {
				node.AvgLatency = (node.AvgLatency + stats.AvgRTT) / 2
				node.PacketLoss = (node.PacketLoss + stats.PacketLoss) / 2
			}
		}

		// Create direct agent-to-destination edge if no MTR path exists
		if mtrDestinations[rawTarget] == nil || !mtrDestinations[rawTarget][agentID] {
			agentNodeID := fmt.Sprintf("agent:%d", agentID)
			edgeID := fmt.Sprintf("%s->%s", agentNodeID, destKey)
			if _, exists := edgeMap[edgeID]; !exists {
				edgeMap[edgeID] = &NetworkMapEdge{
					ID:         edgeID,
					Source:     agentNodeID,
					Target:     destKey,
					AvgLatency: stats.AvgRTT,
					PacketLoss: stats.PacketLoss,
					PathCount:  1,
				}
			}
		}
	}

	// Build destination summaries
	destinations := make([]DestinationSummary, 0, len(destMetrics))
	for target, summary := range destMetrics {
		if target == "" {
			continue
		}
		summary.AgentCount = len(destAgents[target])
		summary.ProbeTypes = make([]string, 0, len(destProbes[target]))
		for pt := range destProbes[target] {
			summary.ProbeTypes = append(summary.ProbeTypes, pt)
		}

		// Determine status
		if summary.PacketLoss >= 50 {
			summary.Status = "critical"
		} else if summary.PacketLoss >= 10 || summary.AvgLatency > 100 {
			summary.Status = "degraded"
		} else {
			summary.Status = "healthy"
		}

		// Find hostname from node
		if node, exists := nodeMap[target]; exists {
			summary.Hostname = node.Hostname
			node.Status = summary.Status // Sync status
		}

		summary.AvgLatency = sanitizeFloat(summary.AvgLatency)
		summary.PacketLoss = sanitizeFloat(summary.PacketLoss)
		destinations = append(destinations, *summary)
	}

	// Convert maps to slices - sanitize floats
	nodes := make([]NetworkMapNode, 0, len(nodeMap))
	for _, node := range nodeMap {
		node.AvgLatency = sanitizeFloat(node.AvgLatency)
		node.PacketLoss = sanitizeFloat(node.PacketLoss)
		nodes = append(nodes, *node)
	}

	edges := make([]NetworkMapEdge, 0, len(edgeMap))
	for _, edge := range edgeMap {
		edge.AvgLatency = sanitizeFloat(edge.AvgLatency)
		edge.PacketLoss = sanitizeFloat(edge.PacketLoss)
		edges = append(edges, *edge)
	}

	return &NetworkMapData{
		Nodes:        nodes,
		Edges:        edges,
		Destinations: destinations,
		GeneratedAt:  time.Now().UTC(),
		WorkspaceID:  workspaceID,
	}
}

func parseUint(s string) uint {
	var u uint
	fmt.Sscanf(s, "%d", &u)
	return u
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
