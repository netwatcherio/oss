package probe

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ── Agent Health Mesh ───────────────────────────────────────────────────────
//
// Pairwise agent↔agent health for the workspace, computed from every
// probe running between two agents (PING, MTR, TRAFFICSIM — both
// directions). Each directed link carries its own health vector so
// asymmetric paths render asymmetric ribbons; node health follows the
// same all-probes-both-directions policy the voice report and the
// per-agent analysis use. The output shape feeds a chord diagram
// directly: nodes are the arc segments, links are the ribbons.

// AgentMeshNode is one agent (one chord arc).
type AgentMeshNode struct {
	AgentID   uint         `json:"agent_id"`
	AgentName string       `json:"agent_name"`
	Location  string       `json:"location,omitempty"`
	IsOnline  bool         `json:"is_online"`
	Health    HealthVector `json:"health"`
	LinkCount int          `json:"link_count"`
}

// AgentMeshLink is one directed agent→agent path (one chord ribbon).
type AgentMeshLink struct {
	SourceAgentID   uint         `json:"source_agent_id"`
	SourceAgentName string       `json:"source_agent_name"`
	TargetAgentID   uint         `json:"target_agent_id"`
	TargetAgentName string       `json:"target_agent_name"`
	Health          HealthVector `json:"health"`
	Metrics         ProbeMetrics `json:"metrics"`
	ProbeTypes      []string     `json:"probe_types"`
}

// WorkspaceHealthMesh is the full chord-diagram payload.
type WorkspaceHealthMesh struct {
	WorkspaceID   uint            `json:"workspace_id"`
	Nodes         []AgentMeshNode `json:"nodes"`
	Links         []AgentMeshLink `json:"links"`
	OverallHealth HealthVector    `json:"overall_health"`
	GeneratedAt   time.Time       `json:"generated_at"`
}

// ComputeWorkspaceHealthMesh fetches the workspace's inter-agent
// metrics and builds the pairwise health mesh.
func ComputeWorkspaceHealthMesh(ctx context.Context, ch *sql.DB, pg *gorm.DB, workspaceID uint, lookbackMinutes int) (*WorkspaceHealthMesh, error) {
	if lookbackMinutes <= 0 {
		lookbackMinutes = 60
	}
	from := time.Now().UTC().Add(-time.Duration(lookbackMinutes) * time.Minute)

	agents, err := getWorkspaceAgents(ctx, pg, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get agents: %w", err)
	}

	agentIDs := make([]uint, len(agents))
	for i, a := range agents {
		agentIDs[i] = a.ID
	}

	pingMetrics, _ := getWorkspacePingMetrics(ctx, ch, agentIDs, from)
	mtrMetrics, _ := getWorkspaceMTRMetrics(ctx, ch, pg, agentIDs, from)
	trafficMetrics, _ := getWorkspaceTrafficSimMetrics(ctx, ch, agentIDs, from)

	mesh := buildHealthMesh(agents, pingMetrics, mtrMetrics, trafficMetrics)
	mesh.WorkspaceID = workspaceID
	return mesh, nil
}

// meshPairKey identifies one directed agent→agent path.
type meshPairKey struct {
	src uint
	dst uint
}

// meshPairAccum accumulates sample-weighted metrics for one pair.
type meshPairAccum struct {
	latencySum float64 // Σ latency × samples
	lossSum    float64
	jitterSum  float64
	samples    int
	probeTypes map[string]bool
}

// buildHealthMesh is the pure aggregation step — separated from the
// ClickHouse fetching so it can be unit-tested on plain metric maps.
// Metric map keys are "<srcAgentID>:<target>"; only entries whose
// TargetAgent is another workspace agent become links.
func buildHealthMesh(
	agents []agentInfo,
	pingMetrics map[string]pingStats,
	mtrMetrics map[string]mtrStats,
	trafficMetrics map[string]trafficStats,
) *WorkspaceHealthMesh {
	agentByID := make(map[uint]agentInfo, len(agents))
	for _, a := range agents {
		agentByID[a.ID] = a
	}

	pairs := make(map[meshPairKey]*meshPairAccum)
	accumulate := func(key string, targetAgent uint, probeType string, latency, loss, jitter float64, samples int) {
		if samples <= 0 {
			return
		}
		i := strings.IndexByte(key, ':')
		if i <= 0 {
			return
		}
		var srcID uint64
		if _, err := fmt.Sscanf(key[:i], "%d", &srcID); err != nil {
			return
		}
		src := uint(srcID)
		if _, ok := agentByID[src]; !ok {
			return
		}
		if _, ok := agentByID[targetAgent]; !ok || targetAgent == 0 || targetAgent == src {
			return
		}
		pk := meshPairKey{src: src, dst: targetAgent}
		acc, ok := pairs[pk]
		if !ok {
			acc = &meshPairAccum{probeTypes: make(map[string]bool)}
			pairs[pk] = acc
		}
		w := float64(samples)
		acc.latencySum += latency * w
		acc.lossSum += loss * w
		acc.jitterSum += jitter * w
		acc.samples += samples
		acc.probeTypes[probeType] = true
	}

	for key, s := range pingMetrics {
		accumulate(key, s.TargetAgent, "PING", s.AvgLatency, s.PacketLoss, 0, s.Count)
	}
	for key, s := range mtrMetrics {
		accumulate(key, s.TargetAgent, "MTR", s.AvgLatency, s.PacketLoss, s.Jitter, s.Count)
	}
	for key, s := range trafficMetrics {
		accumulate(key, s.TargetAgent, "TRAFFICSIM", s.AvgRTT, s.PacketLoss, 0, s.Count)
	}

	// Links: one per directed pair, worst health first so the chord
	// renderer can paint problem ribbons on top.
	links := make([]AgentMeshLink, 0, len(pairs))
	// Node rollup: sample-weighted health across every link touching
	// the agent, in either direction — the path toward an agent is as
	// much its health as the paths it originates.
	nodeScoreSum := make(map[uint]float64)
	nodeWeight := make(map[uint]float64)
	nodeLinks := make(map[uint]int)

	for pk, acc := range pairs {
		m := ProbeMetrics{
			AvgLatency:  acc.latencySum / float64(acc.samples),
			PacketLoss:  acc.lossSum / float64(acc.samples),
			JitterAvg:   acc.jitterSum / float64(acc.samples),
			SampleCount: acc.samples,
		}
		h := computeHealthVector(m, 100)

		types := make([]string, 0, len(acc.probeTypes))
		for t := range acc.probeTypes {
			types = append(types, t)
		}
		sort.Strings(types)

		links = append(links, AgentMeshLink{
			SourceAgentID:   pk.src,
			SourceAgentName: agentByID[pk.src].Name,
			TargetAgentID:   pk.dst,
			TargetAgentName: agentByID[pk.dst].Name,
			Health:          h,
			Metrics:         m,
			ProbeTypes:      types,
		})

		w := float64(acc.samples)
		for _, id := range []uint{pk.src, pk.dst} {
			nodeScoreSum[id] += h.OverallHealth * w
			nodeWeight[id] += w
			nodeLinks[id]++
		}
	}

	sort.Slice(links, func(i, j int) bool {
		if links[i].Health.OverallHealth != links[j].Health.OverallHealth {
			return links[i].Health.OverallHealth < links[j].Health.OverallHealth
		}
		if links[i].SourceAgentID != links[j].SourceAgentID {
			return links[i].SourceAgentID < links[j].SourceAgentID
		}
		return links[i].TargetAgentID < links[j].TargetAgentID
	})

	nodes := make([]AgentMeshNode, 0, len(agents))
	var overallSum, overallWeight float64
	for _, a := range agents {
		n := AgentMeshNode{
			AgentID:   a.ID,
			AgentName: a.Name,
			Location:  a.Location,
			IsOnline:  time.Since(a.UpdatedAt) < time.Minute,
			LinkCount: nodeLinks[a.ID],
		}
		if w := nodeWeight[a.ID]; w > 0 {
			score := clampScore(nodeScoreSum[a.ID] / w)
			n.Health = HealthVector{
				OverallHealth:  score,
				Grade:          gradeFromScore(score),
				RouteStability: 100,
				MosScore:       1.0,
			}
			overallSum += score
			overallWeight++
		} else {
			n.Health = HealthVector{Grade: "unknown", RouteStability: 100, MosScore: 1.0}
		}
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].AgentID < nodes[j].AgentID })

	overall := HealthVector{Grade: "unknown", RouteStability: 100, MosScore: 1.0}
	if overallWeight > 0 {
		score := clampScore(overallSum / overallWeight)
		overall = HealthVector{
			OverallHealth:  score,
			Grade:          gradeFromScore(score),
			RouteStability: 100,
			MosScore:       1.0,
		}
	}

	return &WorkspaceHealthMesh{
		Nodes:         nodes,
		Links:         links,
		OverallHealth: overall,
		GeneratedAt:   time.Now().UTC(),
	}
}
