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

	"gorm.io/gorm"
)

// sanitizeFloat replaces NaN and Infinity with 0
func sanitizeFloat(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
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
	mtrData, err := getWorkspaceMTRData(ctx, ch, agentIDs, from)
	if err != nil {
		// Non-fatal - MTR data is optional
		mtrData = []mtrHopData{}
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

type mtrHopData struct {
	AgentID     uint
	Target      string
	TargetAgent uint // 0 if not agent-to-agent probe
	HopNumber   int
	IP          string
	Hostname    string
	AvgLatency  float64
	PacketLoss  float64
	PathCount   int
}

func getWorkspaceMTRData(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time) ([]mtrHopData, error) {
	// Query raw MTR payloads filtered by workspace agents
	if len(agentIDs) == 0 {
		return []mtrHopData{}, nil
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
		return nil, err
	}
	defer rows.Close()

	var results []mtrHopData
	seenPaths := make(map[string]bool) // agent:target -> bool

	for rows.Next() {
		var agentID uint64
		var target string
		var targetAgent uint64
		var payloadRaw string

		if err := rows.Scan(&agentID, &target, &targetAgent, &payloadRaw); err != nil {
			continue
		}

		// Parse MTR payload
		var payload mtrPayload
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}

		pathKey := fmt.Sprintf("%d:%s", agentID, target)
		isNewPath := !seenPaths[pathKey]
		seenPaths[pathKey] = true

		// Extract hop data
		for i, hop := range payload.Report.Hops {
			hopNum := i + 1
			var ip, hostname string
			if len(hop.Hosts) > 0 {
				ip = hop.Hosts[0].IP
				hostname = hop.Hosts[0].Hostname
			}

			// Parse metrics
			avgLatency := parseFloat(hop.Avg)
			packetLoss := parseFloat(hop.LossPct)

			// Find or create hop entry
			found := false
			for j := range results {
				if results[j].IP == ip && results[j].HopNumber == hopNum && ip != "" {
					// Aggregate metrics
					results[j].AvgLatency = (results[j].AvgLatency*float64(results[j].PathCount) + avgLatency) / float64(results[j].PathCount+1)
					results[j].PacketLoss = (results[j].PacketLoss*float64(results[j].PathCount) + packetLoss) / float64(results[j].PathCount+1)
					results[j].PathCount++
					found = true
					break
				}
			}

			if !found {
				pathCount := 1
				if !isNewPath {
					pathCount = 0
				}
				results = append(results, mtrHopData{
					AgentID:     uint(agentID),
					Target:      target,
					TargetAgent: uint(targetAgent),
					HopNumber:   hopNum,
					IP:          ip,
					Hostname:    hostname,
					AvgLatency:  avgLatency,
					PacketLoss:  packetLoss,
					PathCount:   pathCount,
				})
			}
		}
	}

	return results, rows.Err()
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
	AvgRTT     float64
	PacketLoss float64
	Count      int
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
		totalRTT  float64
		totalLoss float64
		count     int
	}
	accum := make(map[string]*trafficAccum)

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
			accum[key] = &trafficAccum{}
		}
		accum[key].totalRTT += payload.AverageRTT
		accum[key].totalLoss += payload.LossPercentage
		accum[key].count++
	}

	results := make(map[string]trafficStats)
	for key, a := range accum {
		if a.count > 0 {
			results[key] = trafficStats{
				AvgRTT:     a.totalRTT / float64(a.count),
				PacketLoss: a.totalLoss / float64(a.count),
				Count:      a.count,
			}
		}
	}

	return results, rows.Err()
}

func buildNetworkMap(agents []agentInfo, mtrData []mtrHopData, pingMetrics map[string]pingStats, trafficMetrics map[string]trafficStats, workspaceID uint) *NetworkMapData {
	nodeMap := make(map[string]*NetworkMapNode)
	edgeMap := make(map[string]*NetworkMapEdge)

	// Track destination metrics for summary
	destMetrics := make(map[string]*DestinationSummary)
	destAgents := make(map[string]map[uint]bool)
	destProbes := make(map[string]map[string]bool)

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

	// Track destinations from MTR data
	for _, hop := range mtrData {
		if hop.Target != "" {
			if destMetrics[hop.Target] == nil {
				destMetrics[hop.Target] = &DestinationSummary{
					Target: hop.Target,
				}
				destAgents[hop.Target] = make(map[uint]bool)
				destProbes[hop.Target] = make(map[string]bool)
			}
			destAgents[hop.Target][hop.AgentID] = true
			destProbes[hop.Target]["MTR"] = true

			// Track max hop count
			if hop.HopNumber > destMetrics[hop.Target].HopCount {
				destMetrics[hop.Target].HopCount = hop.HopNumber
			}
		}
	}

	// Process MTR hops
	for _, hop := range mtrData {
		agentNodeID := fmt.Sprintf("agent:%d", hop.AgentID)

		// Create hop node
		var hopNodeID string
		if hop.IP != "" {
			hopNodeID = hop.IP
		} else {
			hopNodeID = fmt.Sprintf("unknown-hop-%d", hop.HopNumber)
		}

		// Determine status based on metrics
		hopStatus := "healthy"
		if hop.PacketLoss >= 50 {
			hopStatus = "critical"
		} else if hop.PacketLoss >= 10 || hop.AvgLatency > 100 {
			hopStatus = "degraded"
		} else if hop.IP == "" {
			hopStatus = "unknown"
		}

		pathID := fmt.Sprintf("%d:%s", hop.AgentID, hop.Target)

		if _, exists := nodeMap[hopNodeID]; !exists {
			label := fmt.Sprintf("%d", hop.HopNumber)
			if hop.IP == "" {
				label = "?"
			}
			nodeMap[hopNodeID] = &NetworkMapNode{
				ID:           hopNodeID,
				Type:         "hop",
				Label:        label,
				IP:           hop.IP,
				Hostname:     hop.Hostname,
				HopNumber:    hop.HopNumber,
				AvgLatency:   hop.AvgLatency,
				PacketLoss:   hop.PacketLoss,
				PathCount:    hop.PathCount,
				Layer:        hop.HopNumber,
				Status:       hopStatus,
				SharedAgents: []uint{hop.AgentID},
				PathIDs:      []string{pathID},
			}
		} else {
			// Aggregate with existing - this is a SHARED hop
			node := nodeMap[hopNodeID]
			if node.PathCount+hop.PathCount > 0 {
				node.AvgLatency = (node.AvgLatency*float64(node.PathCount) + hop.AvgLatency*float64(hop.PathCount)) / float64(node.PathCount+hop.PathCount)
				node.PacketLoss = (node.PacketLoss*float64(node.PathCount) + hop.PacketLoss*float64(hop.PathCount)) / float64(node.PathCount+hop.PathCount)
			}
			node.PathCount += hop.PathCount
			// Add agent to shared agents if not already present
			agentFound := false
			for _, a := range node.SharedAgents {
				if a == hop.AgentID {
					agentFound = true
					break
				}
			}
			if !agentFound {
				node.SharedAgents = append(node.SharedAgents, hop.AgentID)
			}
			// Add path ID
			node.PathIDs = append(node.PathIDs, pathID)
		}

		// Create edge from agent to first hop, or between hops
		var sourceID string
		if hop.HopNumber == 1 {
			sourceID = agentNodeID
		} else {
			// Find previous hop
			for _, prevHop := range mtrData {
				if prevHop.AgentID == hop.AgentID && prevHop.Target == hop.Target && prevHop.HopNumber == hop.HopNumber-1 {
					if prevHop.IP != "" {
						sourceID = prevHop.IP
					} else {
						sourceID = fmt.Sprintf("unknown-hop-%d", prevHop.HopNumber)
					}
					break
				}
			}
		}

		if sourceID != "" {
			edgeID := fmt.Sprintf("%s->%s", sourceID, hopNodeID)
			if _, exists := edgeMap[edgeID]; !exists {
				edgeMap[edgeID] = &NetworkMapEdge{
					ID:         edgeID,
					Source:     sourceID,
					Target:     hopNodeID,
					AvgLatency: hop.AvgLatency,
					PacketLoss: hop.PacketLoss,
					PathCount:  1,
				}
			} else {
				edge := edgeMap[edgeID]
				edge.PathCount++
			}
		}
	}

	// Add destination nodes
	for dest := range destMetrics {
		if dest == "" {
			continue
		}
		if _, exists := nodeMap[dest]; !exists {
			// Find hostname from any MTR hop that ended here
			hostname := dest
			for _, hop := range mtrData {
				if hop.IP == dest && hop.Hostname != "" {
					hostname = hop.Hostname
					break
				}
			}
			nodeMap[dest] = &NetworkMapNode{
				ID:        dest,
				Type:      "destination",
				Label:     hostname,
				IP:        dest,
				Hostname:  hostname,
				PathCount: 1,
				Layer:     100, // Destinations on far right
				Status:    "healthy",
			}
		}
	}

	// Create edges from last hop to destination for each MTR path
	// Group MTR data by agent + target to find last hop
	type pathKey struct {
		AgentID uint
		Target  string
	}
	lastHops := make(map[pathKey]mtrHopData)
	for _, hop := range mtrData {
		key := pathKey{AgentID: hop.AgentID, Target: hop.Target}
		if existing, ok := lastHops[key]; !ok || hop.HopNumber > existing.HopNumber {
			lastHops[key] = hop
		}
	}

	// Create edges from last hop to destination (or target agent)
	for key, lastHop := range lastHops {
		var lastHopID string
		if lastHop.IP != "" {
			lastHopID = lastHop.IP
		} else {
			lastHopID = fmt.Sprintf("unknown-hop-%d", lastHop.HopNumber)
		}

		// Determine the target node - use target agent if set, otherwise use destination IP
		var targetNodeID string
		if lastHop.TargetAgent > 0 {
			// Agent-to-agent probe - connect to target agent node
			targetNodeID = fmt.Sprintf("agent:%d", lastHop.TargetAgent)
		} else {
			// External destination
			targetNodeID = key.Target
		}

		// Only create edge if last hop != target
		if lastHopID != targetNodeID && lastHopID != "" {
			// Check if target node exists
			_, targetExists := nodeMap[targetNodeID]
			if targetExists {
				edgeID := fmt.Sprintf("%s->%s", lastHopID, targetNodeID)
				if _, exists := edgeMap[edgeID]; !exists {
					edgeMap[edgeID] = &NetworkMapEdge{
						ID:         edgeID,
						Source:     lastHopID,
						Target:     targetNodeID,
						AvgLatency: lastHop.AvgLatency,
						PacketLoss: lastHop.PacketLoss,
						PathCount:  1,
					}
				}
			}
		}
	}

	// Track which destinations have MTR paths from each agent
	mtrPaths := make(map[pathKey]bool)
	for key := range lastHops {
		mtrPaths[key] = true
	}

	// Process PING metrics - update destination summaries and create edges
	for key, stats := range pingMetrics {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		agentID := parseUint(parts[0])
		target := parts[1]

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
		pathK := pathKey{AgentID: agentID, Target: target}
		if !mtrPaths[pathK] {
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
		target := parts[1]

		if destMetrics[target] == nil {
			destMetrics[target] = &DestinationSummary{Target: target}
			destAgents[target] = make(map[uint]bool)
			destProbes[target] = make(map[string]bool)
		}
		destAgents[target][agentID] = true
		destProbes[target]["TRAFFICSIM"] = true

		// Update metrics
		if destMetrics[target].AvgLatency == 0 {
			destMetrics[target].AvgLatency = stats.AvgRTT
		} else {
			destMetrics[target].AvgLatency = (destMetrics[target].AvgLatency + stats.AvgRTT) / 2
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

		// Update node
		if node, exists := nodeMap[target]; exists {
			node.AvgLatency = (node.AvgLatency + stats.AvgRTT) / 2
			node.PacketLoss = (node.PacketLoss + stats.PacketLoss) / 2
		}

		// Create direct agent-to-destination edge if no MTR path exists
		pathK := pathKey{AgentID: agentID, Target: target}
		if !mtrPaths[pathK] {
			agentNodeID := fmt.Sprintf("agent:%d", agentID)
			edgeID := fmt.Sprintf("%s->%s", agentNodeID, target)
			if _, exists := edgeMap[edgeID]; !exists {
				edgeMap[edgeID] = &NetworkMapEdge{
					ID:         edgeID,
					Source:     agentNodeID,
					Target:     target,
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
