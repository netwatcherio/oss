// internal/probe/network_map.go
// Aggregation functions for workspace-level network topology visualization
package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

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
}

// NetworkMapEdge represents an edge (link) between nodes
type NetworkMapEdge struct {
	ID         string  `json:"id"`
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	AvgLatency float64 `json:"avg_latency"`
	PacketLoss float64 `json:"packet_loss"`
	PathCount  int     `json:"path_count"`
}

// NetworkMapData contains the complete topology data for a workspace
type NetworkMapData struct {
	Nodes       []NetworkMapNode `json:"nodes"`
	Edges       []NetworkMapEdge `json:"edges"`
	GeneratedAt time.Time        `json:"generated_at"`
	WorkspaceID uint             `json:"workspace_id"`
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
		lookbackMinutes = 15
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
		return nil, fmt.Errorf("get mtr data: %w", err)
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
	AgentID    uint
	Target     string
	HopNumber  int
	IP         string
	Hostname   string
	AvgLatency float64
	PacketLoss float64
	PathCount  int
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
		var payloadRaw string

		if err := rows.Scan(&agentID, &target, &payloadRaw); err != nil {
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
					AgentID:    uint(agentID),
					Target:     target,
					HopNumber:  hopNum,
					IP:         ip,
					Hostname:   hostname,
					AvgLatency: avgLatency,
					PacketLoss: packetLoss,
					PathCount:  pathCount,
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

	q := fmt.Sprintf(`
SELECT 
    agent_id,
    target,
    avg(JSONExtractFloat(payload_raw, 'latency')) as avg_latency,
    avg(JSONExtractFloat(payload_raw, 'packetLoss')) as avg_packet_loss,
    count() as cnt
FROM probe_data
WHERE type = 'PING'
  AND agent_id IN (%s)
  AND created_at >= %s
GROUP BY agent_id, target
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]pingStats)
	for rows.Next() {
		var agentID uint64
		var target string
		var avgLatency, avgPacketLoss float64
		var count int

		if err := rows.Scan(&agentID, &target, &avgLatency, &avgPacketLoss, &count); err != nil {
			continue
		}

		key := fmt.Sprintf("%d:%s", agentID, target)
		results[key] = pingStats{
			AvgLatency: avgLatency,
			PacketLoss: avgPacketLoss,
			Count:      count,
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

	q := fmt.Sprintf(`
SELECT 
    agent_id,
    target,
    avg(JSONExtractFloat(payload_raw, 'averageRTT')) as avg_rtt,
    avg(
        CASE 
            WHEN JSONExtractUInt(payload_raw, 'totalPackets') > 0 
            THEN JSONExtractUInt(payload_raw, 'lostPackets') * 100.0 / JSONExtractUInt(payload_raw, 'totalPackets')
            ELSE 0 
        END
    ) as avg_packet_loss,
    count() as cnt
FROM probe_data
WHERE type = 'TRAFFICSIM'
  AND agent_id IN (%s)
  AND created_at >= %s
GROUP BY agent_id, target
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]trafficStats)
	for rows.Next() {
		var agentID uint64
		var target string
		var avgRTT, avgPacketLoss float64
		var count int

		if err := rows.Scan(&agentID, &target, &avgRTT, &avgPacketLoss, &count); err != nil {
			continue
		}

		key := fmt.Sprintf("%d:%s", agentID, target)
		results[key] = trafficStats{
			AvgRTT:     avgRTT,
			PacketLoss: avgPacketLoss,
			Count:      count,
		}
	}

	return results, rows.Err()
}

func buildNetworkMap(agents []agentInfo, mtrData []mtrHopData, pingMetrics map[string]pingStats, trafficMetrics map[string]trafficStats, workspaceID uint) *NetworkMapData {
	nodeMap := make(map[string]*NetworkMapNode)
	edgeMap := make(map[string]*NetworkMapEdge)

	// Add agent nodes
	for _, agent := range agents {
		nodeID := fmt.Sprintf("agent:%d", agent.ID)
		isOnline := time.Since(agent.UpdatedAt) < time.Minute
		nodeMap[nodeID] = &NetworkMapNode{
			ID:        nodeID,
			Type:      "agent",
			Label:     agent.Name,
			AgentID:   &agent.ID,
			IP:        agent.PublicIPOverride,
			IsOnline:  isOnline,
			PathCount: 0,
		}
	}

	// Track destinations
	destinations := make(map[string]bool)

	// Process MTR hops
	for _, hop := range mtrData {
		agentNodeID := fmt.Sprintf("agent:%d", hop.AgentID)

		// Track destination
		destinations[hop.Target] = true

		// Create hop node
		var hopNodeID string
		if hop.IP != "" {
			hopNodeID = hop.IP
		} else {
			hopNodeID = fmt.Sprintf("unknown-hop-%d", hop.HopNumber)
		}

		if _, exists := nodeMap[hopNodeID]; !exists {
			label := fmt.Sprintf("%d", hop.HopNumber)
			if hop.IP == "" {
				label = "?"
			}
			nodeMap[hopNodeID] = &NetworkMapNode{
				ID:         hopNodeID,
				Type:       "hop",
				Label:      label,
				IP:         hop.IP,
				Hostname:   hop.Hostname,
				HopNumber:  hop.HopNumber,
				AvgLatency: hop.AvgLatency,
				PacketLoss: hop.PacketLoss,
				PathCount:  hop.PathCount,
			}
		} else {
			// Aggregate with existing
			node := nodeMap[hopNodeID]
			node.AvgLatency = (node.AvgLatency*float64(node.PathCount) + hop.AvgLatency*float64(hop.PathCount)) / float64(node.PathCount+hop.PathCount)
			node.PacketLoss = (node.PacketLoss*float64(node.PathCount) + hop.PacketLoss*float64(hop.PathCount)) / float64(node.PathCount+hop.PathCount)
			node.PathCount += hop.PathCount
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
	for dest := range destinations {
		if _, exists := nodeMap[dest]; !exists {
			nodeMap[dest] = &NetworkMapNode{
				ID:        dest,
				Type:      "destination",
				Label:     dest,
				IP:        dest,
				PathCount: 1,
			}
		}
	}

	// Overlay PING metrics onto edges
	for key, stats := range pingMetrics {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		agentNodeID := "agent:" + parts[0]
		target := parts[1]

		// Find edge from agent to destination
		edgeID := fmt.Sprintf("%s->%s", agentNodeID, target)
		if edge, exists := edgeMap[edgeID]; exists {
			// Average with existing metrics
			edge.AvgLatency = (edge.AvgLatency + stats.AvgLatency) / 2
			edge.PacketLoss = (edge.PacketLoss + stats.PacketLoss) / 2
		}
	}

	// Overlay TrafficSim metrics
	for key, stats := range trafficMetrics {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		agentNodeID := "agent:" + parts[0]
		target := parts[1]

		// Update destination node if exists
		if node, exists := nodeMap[target]; exists {
			node.AvgLatency = (node.AvgLatency + stats.AvgRTT) / 2
			node.PacketLoss = (node.PacketLoss + stats.PacketLoss) / 2
		}

		// Update edge metrics
		edgeID := fmt.Sprintf("%s->%s", agentNodeID, target)
		if edge, exists := edgeMap[edgeID]; exists {
			edge.AvgLatency = (edge.AvgLatency + stats.AvgRTT) / 2
			edge.PacketLoss = (edge.PacketLoss + stats.PacketLoss) / 2
		}
	}

	// Convert maps to slices
	nodes := make([]NetworkMapNode, 0, len(nodeMap))
	for _, node := range nodeMap {
		nodes = append(nodes, *node)
	}

	edges := make([]NetworkMapEdge, 0, len(edgeMap))
	for _, edge := range edgeMap {
		edges = append(edges, *edge)
	}

	return &NetworkMapData{
		Nodes:       nodes,
		Edges:       edges,
		GeneratedAt: time.Now().UTC(),
		WorkspaceID: workspaceID,
	}
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
