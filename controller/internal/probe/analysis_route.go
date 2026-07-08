package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ── Route / Path Analysis ──

// RouteBaseline mirrors the alert package model so we can query without importing alert.
type routeBaseline struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	ProbeID     uint   `gorm:"uniqueIndex;not null" json:"probe_id"`
	Fingerprint string `gorm:"size:64;not null" json:"fingerprint"`
	RoutePath   string `gorm:"size:2048" json:"route_path,omitempty"`
	HopCount    int    `json:"hop_count"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (routeBaseline) TableName() string { return "route_baselines" }

// HopDetail holds enriched hop information for route analysis display
type HopDetail struct {
	IP            string  `json:"ip"`
	Hostname      string  `json:"hostname,omitempty"`
	IsAgent       bool    `json:"is_agent"`
	AgentID       uint    `json:"agent_id,omitempty"`
	AgentName     string  `json:"agent_name,omitempty"`
	IsFinalHop    bool    `json:"is_final_hop"`
	Latency       float64 `json:"latency,omitempty"`
	Loss          float64 `json:"loss,omitempty"`
	IsRateLimited bool    `json:"is_rate_limited,omitempty"`
}

// hopAgg holds aggregated metrics for a single hop index across traces
type hopAgg struct {
	totalLatency float64
	totalLoss    float64
	count        int
}

// buildHopDetails creates enriched hop details from raw MTR hops, matching IPs to agents (uses MtrPayload from clickhouse.go)
func buildHopDetails(mtrPayload *MtrPayload, agentIPToID map[string]uint, agentByID map[uint]agentInfo) []HopDetail {
	var details []HopDetail
	hopCount := len(mtrPayload.Report.Hops)
	for i, hop := range mtrPayload.Report.Hops {
		if len(hop.Hosts) == 0 || hop.Hosts[0].IP == "" || hop.Hosts[0].IP == "*" {
			continue
		}
		hd := HopDetail{
			IP: hop.Hosts[0].IP,
		}
		// Check if this hop IP matches any agent's PublicIPOverride
		if agentID, ok := agentIPToID[hop.Hosts[0].IP]; ok {
			hd.IsAgent = true
			hd.AgentID = agentID
			if a, ok := agentByID[agentID]; ok {
				hd.AgentName = a.Name
				// For final hop, also use description if available
				if i == hopCount-1 && a.Description != "" {
					hd.Hostname = fmt.Sprintf("%s (%s)", a.Name, a.Description)
				} else {
					hd.Hostname = a.Name
				}
			}
		}
		hd.IsFinalHop = i == hopCount-1
		details = append(details, hd)
	}
	return details
}

// buildHopDetailsForMtrPayload creates enriched hop details from agent MTR payload (uses mtrPayload from mtr.go)
func buildHopDetailsForMtrPayload(mtrPayload *mtrPayload, agentIPToID map[string]uint, agentByID map[uint]agentInfo, hopMetrics map[int]hopAgg, rateLimitedSet map[int]bool) []HopDetail {
	var details []HopDetail
	hopCount := len(mtrPayload.Report.Hops)
	for i, hop := range mtrPayload.Report.Hops {
		if len(hop.Hosts) == 0 || hop.Hosts[0].IP == "" || hop.Hosts[0].IP == "*" {
			continue
		}
		hd := HopDetail{
			IP:       hop.Hosts[0].IP,
			Hostname: hop.Hosts[0].Hostname,
		}
		// Populate per-hop aggregated metrics
		if ha, ok := hopMetrics[i]; ok && ha.count > 0 {
			hd.Latency = sanitizeFloat(ha.totalLatency / float64(ha.count))
			hd.Loss = sanitizeFloat(ha.totalLoss / float64(ha.count))
		}
		if rateLimitedSet[i] {
			hd.IsRateLimited = true
		}
		// Check if this hop IP matches any agent's PublicIPOverride
		if agentID, ok := agentIPToID[hop.Hosts[0].IP]; ok {
			hd.IsAgent = true
			hd.AgentID = agentID
			if a, ok := agentByID[agentID]; ok {
				hd.AgentName = a.Name
				// For final hop, also use description if available
				if i == hopCount-1 && a.Description != "" {
					hd.Hostname = fmt.Sprintf("%s (%s)", a.Name, a.Description)
				} else {
					hd.Hostname = a.Name
				}
			}
		}
		hd.IsFinalHop = i == hopCount-1
		details = append(details, hd)
	}
	return details
}

// ProbeRouteInfo holds route data for a single MTR probe.
type ProbeRouteInfo struct {
	ProbeID             uint        `json:"probe_id"`
	AgentID             uint        `json:"agent_id,omitempty"`
	Target              string      `json:"target"`
	TargetAgentID       uint        `json:"target_agent_id,omitempty"`
	TargetAgentName     string      `json:"target_agent_name,omitempty"`
	BaselineFingerprint string      `json:"baseline_fingerprint,omitempty"`
	BaselineHopCount    int         `json:"baseline_hop_count,omitempty"`
	BaselineRoutePath   string      `json:"baseline_route_path,omitempty"`
	LatestSignature     string      `json:"latest_signature,omitempty"`
	LatestHops          []string    `json:"latest_hops,omitempty"`        // IPs only (for signature computation)
	LatestHopsDetail    []HopDetail `json:"latest_hops_detail,omitempty"` // Enriched with agent names
	HasRouteChange      bool        `json:"has_route_change"`
	RouteChangedAt      *time.Time  `json:"route_changed_at,omitempty"` // First time (within lookback) the signature differed from the baseline
	TraceCount          int         `json:"trace_count,omitempty"`
	RouteStabilityPct   float64     `json:"route_stability_pct,omitempty"`
	AvgEndHopLatency    float64     `json:"avg_end_hop_latency,omitempty"`
	AvgEndHopLoss       float64     `json:"avg_end_hop_loss,omitempty"`
	IntermediateHops    []HopMetric `json:"intermediate_hops,omitempty"` // Hop metrics excluding the final hop
}

// HopMetric holds metrics for a single intermediate hop (not the final destination)
type HopMetric struct {
	IP       string  `json:"ip"`
	Loss     float64 `json:"loss"`
	Latency  float64 `json:"latency"`
	HopIndex int     `json:"hop_index"`
}

// HopMetrics holds aggregated metrics for a hop across all agents that traverse it
type HopMetrics struct {
	TotalLoss    float64
	TotalLatency float64
	Count        int
	HasIssues    bool
}

// AgentRouteInfo holds route/path data for a single agent.
type AgentRouteInfo struct {
	AgentID      uint             `json:"agent_id"`
	AgentName    string           `json:"agent_name"`
	PublicIP     string           `json:"public_ip,omitempty"`
	ISP          string           `json:"isp,omitempty"`
	HasIPChange  bool             `json:"has_ip_change"`
	HasISPChange bool             `json:"has_isp_change"`
	Routes       []ProbeRouteInfo `json:"routes"`
}

// SharedHopInfo represents a hop that appears in multiple agent routes.
type SharedHopInfo struct {
	HopIP       string   `json:"hop_ip"`
	HopHostname string   `json:"hop_hostname,omitempty"` // Agent name if this hop is an agent
	AgentIDs    []uint   `json:"agent_ids"`
	AgentNames  []string `json:"agent_names"`
	HopCount    int      `json:"hop_count"`
	HasIssues   bool     `json:"has_issues"` // True if any intermediate hop in the shared path has loss or high latency
	AvgLoss     float64  `json:"avg_loss,omitempty"`
	AvgLatency  float64  `json:"avg_latency,omitempty"`
}

// RouteIncident is a lightweight incident specifically for route/path issues.
type RouteIncident struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"` // ip_change, isp_change, route_change
	Severity   string   `json:"severity"`
	AgentID    uint     `json:"agent_id"`
	AgentName  string   `json:"agent_name"`
	ProbeID    uint     `json:"probe_id,omitempty"`
	Target     string   `json:"target,omitempty"`
	Message    string   `json:"message"`
	Evidence   []string `json:"evidence,omitempty"`
	DetectedAt string   `json:"detected_at,omitempty"`

	// Structured change data for route_change incidents. Lets the UI
	// render a "before / after" diff without re-parsing the legacy
	// Evidence string list. Optional on other incident types.
	BaselineFingerprint string   `json:"baseline_fingerprint,omitempty"`
	CurrentFingerprint  string   `json:"current_fingerprint,omitempty"`
	BaselinePath        string   `json:"baseline_path,omitempty"` // Human-readable baseline hop list ("1.2.3.4 -> 5.6.7.8")
	CurrentPath         string   `json:"current_path,omitempty"`  // Human-readable current hop list
	BaselineHopCount    int      `json:"baseline_hop_count,omitempty"`
	CurrentHopCount     int      `json:"current_hop_count,omitempty"`
	AddedHops           []string `json:"added_hops,omitempty"`    // IPs that appear in current but not baseline (preserves order)
	RemovedHops         []string `json:"removed_hops,omitempty"`  // IPs that appear in baseline but not current
	Jaccard             float64  `json:"jaccard,omitempty"`       // 0..1 similarity between baseline and current hop sets
	StabilityPct        float64  `json:"stability_pct,omitempty"` // Dominant signature's share of recent traces
	TraceCount          int      `json:"trace_count,omitempty"`   // Traces considered for this change detection
}

// WorkspaceRouteAnalysis is the top-level response for route/path visualization.
type WorkspaceRouteAnalysis struct {
	WorkspaceID        uint                    `json:"workspace_id"`
	Agents             []AgentRouteInfo        `json:"agents"`
	SharedHops         []SharedHopInfo         `json:"shared_hops"`
	SharedDestinations []SharedDestinationInfo `json:"shared_destinations"`
	SharedASNs         []SharedASNInfo         `json:"shared_asns"`
	CommonTargets      []CommonTargetInfo      `json:"common_targets"`
	Incidents          []RouteIncident         `json:"incidents"`
	TotalAgents        int                     `json:"total_agents"`
	TotalRoutes        int                     `json:"total_routes"`
	GeneratedAt        time.Time               `json:"generated_at"`
}

// SharedDestinationInfo represents a destination IP/hostname that 2+ agents
// are MTR-ing to. This is the most common "shared path" pattern across agents
// targeting the same internet endpoint (e.g. 8.8.8.8, dns.google).
type SharedDestinationInfo struct {
	Target          string   `json:"target"` // Hostname or IP target
	TargetIP        string   `json:"target_ip,omitempty"`
	AgentIDs        []uint   `json:"agent_ids"`
	AgentNames      []string `json:"agent_names"`
	AgentCount      int      `json:"agent_count"`
	AvgEndLatencyMs float64  `json:"avg_end_latency_ms,omitempty"`
	AvgEndLossPct   float64  `json:"avg_end_loss_pct,omitempty"`
	HasIssues       bool     `json:"has_issues"`
}

// SharedASNInfo groups intermediate hop IPs by ASN, showing common upstream
// networks that 2+ agents traverse. This is the most resilient shared-route
// signal because agents on different last-mile ISPs still share ASN-level
// transit (e.g. Level3, Cogent, NTT).
type SharedASNInfo struct {
	ASN        uint     `json:"asn"`
	ASNOrg     string   `json:"asn_org,omitempty"`
	HopIPs     []string `json:"hop_ips"`
	AgentIDs   []uint   `json:"agent_ids"`
	AgentNames []string `json:"agent_names"`
	AgentCount int      `json:"agent_count"`
	HasIssues  bool     `json:"has_issues"`
	AvgLatency float64  `json:"avg_latency_ms,omitempty"`
	AvgLoss    float64  `json:"avg_loss_pct,omitempty"`
}

// CommonTargetInfo summarizes a target (e.g. "google.com") that multiple
// agents are MTR-ing to. This is the "what are agents probing in common"
// view, irrespective of whether the actual path hops overlap.
type CommonTargetInfo struct {
	Target          string   `json:"target"`
	AgentIDs        []uint   `json:"agent_ids"`
	AgentNames      []string `json:"agent_names"`
	AgentCount      int      `json:"agent_count"`
	ProbeCount      int      `json:"probe_count"`
	AvgEndLatencyMs float64  `json:"avg_end_latency_ms,omitempty"`
	AvgEndLossPct   float64  `json:"avg_end_loss_pct,omitempty"`
	HasIssues       bool     `json:"has_issues"`
}

// ComputeWorkspaceRouteAnalysis aggregates route/path data across all agents in a workspace
// for the route/path matching visualization. Pass nil for geoStore to skip ASN grouping.
//
//	lookbackHours bounds the MTR / NETINFO lookback window. 0 = default (24h MTR, 1h NETINFO).
func ComputeWorkspaceRouteAnalysis(ctx context.Context, ch *sql.DB, pg *gorm.DB, geoStore GeoIPResolver, workspaceID uint, lookbackHours int) (*WorkspaceRouteAnalysis, error) {
	// 1. Get agents
	agents, err := getWorkspaceAgents(ctx, pg, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get agents: %w", err)
	}

	if len(agents) == 0 {
		return &WorkspaceRouteAnalysis{
			WorkspaceID: workspaceID,
			Agents:      []AgentRouteInfo{},
			SharedHops:  []SharedHopInfo{},
			Incidents:   []RouteIncident{},
			GeneratedAt: time.Now().UTC(),
		}, nil
	}

	agentIDs := make([]uint, len(agents))
	agentByID := make(map[uint]agentInfo)
	agentIPToID := make(map[string]uint) // IP -> AgentID for hop matching
	for i, a := range agents {
		agentIDs[i] = a.ID
		agentByID[a.ID] = a
		if a.PublicIPOverride != "" {
			agentIPToID[a.PublicIPOverride] = a.ID
		}
	}

	// MTR lookback default = 24h, NETINFO lookback fixed at 1h.
	// The lookbackHours param scales only the MTR window — NETINFO must stay
	// tight so a public IP change from 25h ago doesn't bleed into a "current"
	// view.
	if lookbackHours <= 0 {
		lookbackHours = 24
	}
	mtrFrom := time.Now().UTC().Add(-time.Duration(lookbackHours) * time.Hour)
	netInfoFrom := time.Now().UTC().Add(-60 * time.Minute)

	// 2. Get latest NETINFO per agent in a single batched query.
	// Per-agent round-trips were O(N×M) because agent_id is not in the
	// probe_data primary key (type, probe_id, created_at). Use a tight
	// created_at range so ClickHouse can do a range scan inside the
	// type='NETINFO' partition, then pick the latest row per agent with
	// row_number().
	netInfoByAgent := getLatestNetInfoForAgents(ctx, ch, agentIDs, netInfoFrom)

	// 2b. Augment agentIPToID with NETINFO-derived public IPs. The public
	// IP itself rarely appears in the agent's own outbound MTR (NAT + ICMP
	// rate-limiting at the CPE), but it DOES show up in any MTR another
	// agent runs against this one — without this, agent-to-agent hops
	// never get their "is_agent" label.
	for agentID, ni := range netInfoByAgent {
		if ni == nil || ni.PublicAddress == "" {
			continue
		}
		if _, exists := agentIPToID[ni.PublicAddress]; !exists {
			agentIPToID[ni.PublicAddress] = agentID
		}
	}

	// 3. Detect IP/ISP changes
	netInfoChanges, _ := getWorkspaceNetInfoChanges(ctx, ch, agentIDs, netInfoFrom)
	changeByAgent := make(map[uint][]netInfoChange)
	for _, c := range netInfoChanges {
		changeByAgent[c.AgentID] = append(changeByAgent[c.AgentID], c)
	}

	// 4. Fetch ALL MTR data for the workspace in one query. The MTR data
	// exists in ClickHouse regardless of whether the parent probe is type
	// "MTR" or "AGENT" (bidirectional probes store MTR rows with sub-type
	// 'MTR' under their probe_id, with target_agent set to the destination
	// agent for agent-to-agent probes). Driving the analysis from the
	// probe_data type='MTR' rows means both standalone MTR and AGENT
	// (bidirectional) probes are handled uniformly.
	agentRoutes := make([]AgentRouteInfo, 0, len(agents))
	hopIndex := make(map[string]map[uint]HopMetrics) // hopIP -> agentID -> metrics
	routeIncidents := make([]RouteIncident, 0)
	totalRoutes := 0

	// Pre-populate every agent with an empty AgentRouteInfo so agents with
	// no MTR data still appear in the UI (with empty routes / NETINFO
	// status). This matches the original behavior of iterating ListByAgent.
	for _, a := range agents {
		ari := AgentRouteInfo{
			AgentID:   a.ID,
			AgentName: a.Name,
		}
		if ni := netInfoByAgent[a.ID]; ni != nil {
			ari.PublicIP = ni.PublicAddress
			ari.ISP = ni.GetISP()
		}
		// IP / ISP change incidents
		if changes, ok := changeByAgent[a.ID]; ok {
			for _, c := range changes {
				switch c.Field {
				case "public_ip":
					ari.HasIPChange = true
					routeIncidents = append(routeIncidents, RouteIncident{
						ID:         fmt.Sprintf("ip_change_%d", a.ID),
						Type:       "ip_change",
						Severity:   "info",
						AgentID:    a.ID,
						AgentName:  a.Name,
						Message:    fmt.Sprintf("Public IP changed from %s to %s", c.OldValue, c.NewValue),
						Evidence:   []string{fmt.Sprintf("Previous: %s", c.OldValue), fmt.Sprintf("Current: %s", c.NewValue)},
						DetectedAt: c.DetectedAt.Format(time.RFC3339),
					})
				case "isp":
					ari.HasISPChange = true
					routeIncidents = append(routeIncidents, RouteIncident{
						ID:         fmt.Sprintf("isp_change_%d", a.ID),
						Type:       "isp_change",
						Severity:   "warning",
						AgentID:    a.ID,
						AgentName:  a.Name,
						Message:    fmt.Sprintf("ISP changed from %s to %s", c.OldValue, c.NewValue),
						Evidence:   []string{fmt.Sprintf("Previous ISP: %s", c.OldValue), fmt.Sprintf("Current ISP: %s", c.NewValue)},
						DetectedAt: c.DetectedAt.Format(time.RFC3339),
					})
				}
			}
		}
		agentRoutes = append(agentRoutes, ari)
	}
	// Index by agent ID so the per-(probe, agent) loop below can attach routes.
	ariByAgent := make(map[uint]*AgentRouteInfo, len(agentRoutes))
	for i := range agentRoutes {
		ariByAgent[agentRoutes[i].AgentID] = &agentRoutes[i]
	}

	// Pull all MTR rows for this workspace in a single batched query.
	// Returns a map keyed by (probe_id, agent_id, target_agent) → []ProbeData
	// so we can compute signatures / stability per (agent, target) tuple.
	mtrByPath, err := getWorkspaceMTRByPath(ctx, ch, agentIDs, mtrFrom, 200)
	if err != nil {
		log.Warnf("[route-analysis] workspace=%d MTR query error: %v", workspaceID, err)
		// Non-fatal: return what we have (agents, NETINFO, change incidents)
		// with empty MTR-derived fields.
		return &WorkspaceRouteAnalysis{
			WorkspaceID:        workspaceID,
			Agents:             agentRoutes,
			SharedHops:         []SharedHopInfo{},
			SharedDestinations: []SharedDestinationInfo{},
			SharedASNs:         buildSharedASNs(geoStore, hopIndex, agentByID),
			CommonTargets:      []CommonTargetInfo{},
			Incidents:          routeIncidents,
			TotalAgents:        len(agents),
			TotalRoutes:        0,
			GeneratedAt:        time.Now().UTC(),
		}, nil
	}

	// Cache of route baselines by probe_id (Postgres is fast but worth
	// caching for the many (probe, target) tuples an AGENT probe generates).
	// For bidirectional AGENT probes both A→B (from A) and B→A (from B) share
	// the same probe_id, so we also cache the probe owner and only attach the
	// baseline to the FORWARD direction (reporting agent == probe owner).
	// Otherwise the reverse path would be compared against the forward
	// baseline and falsely flagged as a route change.
	baselineByProbe := make(map[uint]routeBaseline)
	probeOwnerByID := make(map[uint]uint)
	loadBaselineForDirection := func(probeID, reportingAgentID uint) (routeBaseline, bool) {
		owner, known := probeOwnerByID[probeID]
		if !known {
			var o uint
			if err := pg.WithContext(ctx).Model(&Probe{}).Select("agent_id").Where("id = ?", probeID).Scan(&o).Error; err != nil {
				log.Warnf("[route-analysis] failed to read probe owner for probe %d: %v", probeID, err)
				return routeBaseline{}, false
			}
			owner = o
			probeOwnerByID[probeID] = owner
		}
		// No owner recorded (probe missing) or this is a reverse-direction row
		// → no baseline applies.
		if owner == 0 || owner != reportingAgentID {
			return routeBaseline{}, false
		}
		if b, ok := baselineByProbe[probeID]; ok {
			return b, true
		}
		var b routeBaseline
		if err := pg.WithContext(ctx).Where("probe_id = ?", probeID).First(&b).Error; err == nil {
			baselineByProbe[probeID] = b
			return b, true
		}
		return routeBaseline{}, false
	}

	// routeKey is defined at package scope (above) so the per-pathKey
	// aggregator (mtrPathAgg.process) can share the type with the call site.
	routeByKey := make(map[routeKey]*ProbeRouteInfo)

	// destStats is defined at package scope (above) so the per-pathKey
	// aggregator can share the type with the call site.
	destAgg := make(map[string]*destStats)
	commonTargetKey := func(t string) string { return strings.ToLower(strings.TrimSpace(t)) }

	// Track the unique probe IDs we've seen so totalRoutes counts probes, not
	// (probe, direction) tuples — a single bidirectional AGENT probe must
	// only inflate this counter by 1 even though both A→B and B→A rows
	// materialise as separate ProbeRouteInfo entries.
	seenProbeIDs := make(map[uint]struct{})

	// Track which (probe_id, attribution_id) pairs have already
	// emitted a route_change incident. Keyed on probeID<<32 | agentID
	// so the forward and reverse directions of a single bidirectional
	// AGENT probe — which have different attribution IDs (owner vs.
	// reporter) — collapse into one incident per direction instead of
	// one per probe.
	incidentProbeIDs := make(map[uint]struct{})

	agg := newMTRPathAgg(mtrPathAggConfig{
		ARIByAgent:         ariByAgent,
		AgentByID:          agentByID,
		AgentIPToID:        agentIPToID,
		CommonTargetKey:    commonTargetKey,
		RouteByKey:         routeByKey,
		HopIndex:           hopIndex,
		DestAgg:            destAgg,
		SeenProbeIDs:       seenProbeIDs,
		IncidentProbeIDs:   incidentProbeIDs,
		LoadBaselineForDir: loadBaselineForDirection,
	})

	for pathKey, rows := range mtrByPath {
		agg.process(pathKey, rows, &routeIncidents)
	}

	allRoutes := make([]*ProbeRouteInfo, 0, len(routeByKey))
	for _, pri := range routeByKey {
		allRoutes = append(allRoutes, pri)
	}
	sort.SliceStable(allRoutes, func(i, j int) bool {
		if allRoutes[i].AgentID != allRoutes[j].AgentID {
			return allRoutes[i].AgentID < allRoutes[j].AgentID
		}
		return allRoutes[i].ProbeID < allRoutes[j].ProbeID
	})
	for _, pri := range allRoutes {
		ari := ariByAgent[pri.AgentID]
		if ari == nil {
			continue
		}
		ari.Routes = append(ari.Routes, *pri)
	}
	// totalRoutes counts unique probes (not (probe, direction) tuples) so a
	// single bidirectional AGENT probe contributes 1, not 2.
	totalRoutes = len(seenProbeIDs)

	// 5. Build shared hops list
	sharedHops := make([]SharedHopInfo, 0)
	for hopIP, agentMetricsMap := range hopIndex {
		if len(agentMetricsMap) < 2 {
			continue
		}
		sh := SharedHopInfo{
			HopIP:    hopIP,
			HopCount: len(agentMetricsMap),
		}
		// Check if this shared hop IP matches any agent
		if aid, ok := agentIPToID[hopIP]; ok {
			if a, ok := agentByID[aid]; ok {
				sh.HopHostname = a.Name
			}
		}
		var totalLoss, totalLatency float64
		var metricsCount int
		for aid, metrics := range agentMetricsMap {
			sh.AgentIDs = append(sh.AgentIDs, aid)
			if a, ok := agentByID[aid]; ok {
				sh.AgentNames = append(sh.AgentNames, a.Name)
			}
			if metrics.Count > 0 {
				totalLoss += metrics.TotalLoss
				totalLatency += metrics.TotalLatency
				metricsCount += metrics.Count
			}
			if metrics.HasIssues {
				sh.HasIssues = true
			}
		}
		if metricsCount > 0 {
			sh.AvgLoss = totalLoss / float64(metricsCount)
			sh.AvgLatency = totalLatency / float64(metricsCount)
		}
		sharedHops = append(sharedHops, sh)
	}

	// 6. Build shared destinations — any target that 2+ agents are MTR-ing.
	// This is the most useful "common route" view because it surfaces
	// internet endpoints the deployment is collectively monitoring.
	sharedDestinations := make([]SharedDestinationInfo, 0)
	for _, ds := range destAgg {
		if len(ds.agents) < 2 {
			continue
		}
		sd := SharedDestinationInfo{
			Target:     ds.displayName,
			TargetIP:   ds.targetIP,
			AgentCount: len(ds.agents),
		}
		if ds.latSamples > 0 {
			sd.AvgEndLatencyMs = ds.totalLat / float64(ds.latSamples)
		}
		if ds.lossSamples > 0 {
			sd.AvgEndLossPct = ds.totalLoss / float64(ds.lossSamples)
		}
		sd.HasIssues = ds.hasIssues
		for aid := range ds.agents {
			sd.AgentIDs = append(sd.AgentIDs, aid)
			if a, ok := agentByID[aid]; ok {
				sd.AgentNames = append(sd.AgentNames, a.Name)
			}
		}
		sharedDestinations = append(sharedDestinations, sd)
	}

	// 7. Build common targets — every target probed by ≥1 agent, sorted by
	// agent count. Single-agent targets still get a row, so the UI can
	// answer "what is this agent MTR-ing?" without leaving the tab.
	commonTargets := make([]CommonTargetInfo, 0, len(destAgg))
	for _, ds := range destAgg {
		ct := CommonTargetInfo{
			Target:     ds.displayName,
			AgentCount: len(ds.agents),
			ProbeCount: ds.probeCount,
			HasIssues:  ds.hasIssues,
		}
		if ds.latSamples > 0 {
			ct.AvgEndLatencyMs = ds.totalLat / float64(ds.latSamples)
		}
		if ds.lossSamples > 0 {
			ct.AvgEndLossPct = ds.totalLoss / float64(ds.lossSamples)
		}
		for aid := range ds.agents {
			ct.AgentIDs = append(ct.AgentIDs, aid)
			if a, ok := agentByID[aid]; ok {
				ct.AgentNames = append(ct.AgentNames, a.Name)
			}
		}
		commonTargets = append(commonTargets, ct)
	}
	// Stable ordering: most-shared first, then alphabetical.
	sortCommonTargets(commonTargets)

	// 8. Build shared ASNs — group hop IPs by their ASN, only emit ASNs
	// that 2+ agents traverse. This is the "common upstream network"
	// view that survives last-mile ISP diversity.
	sharedASNs := buildSharedASNs(geoStore, hopIndex, agentByID)

	return &WorkspaceRouteAnalysis{
		WorkspaceID:        workspaceID,
		Agents:             agentRoutes,
		SharedHops:         sharedHops,
		SharedDestinations: sharedDestinations,
		SharedASNs:         sharedASNs,
		CommonTargets:      commonTargets,
		Incidents:          routeIncidents,
		TotalAgents:        len(agents),
		TotalRoutes:        totalRoutes,
		GeneratedAt:        time.Now().UTC(),
	}, nil
}

// sortCommonTargets orders: highest agent count first, then alpha by target.
func sortCommonTargets(cs []CommonTargetInfo) {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].AgentCount != cs[j].AgentCount {
			return cs[i].AgentCount > cs[j].AgentCount
		}
		return cs[i].Target < cs[j].Target
	})
}

// routeEcmpSimilarityThreshold is the minimum Jaccard similarity between the
// baseline and current hop-IP sets for a route to be considered "the same
// path" despite fingerprint differences. ECMP / load-balancing typically
// swaps 1-2 hops out of ~10-15, yielding similarity ~0.8-0.9; a real route
// change (different upstream, agent moved networks) lands well below this.
const routeEcmpSimilarityThreshold = 0.7

// routeBaselineStaleThreshold is the maximum age of a stored baseline before
// the MTR handler rewrites it to the current fingerprint. This way
// intentional long-term route changes (e.g. agent moved networks, ISP
// rerouted the path) are eventually picked up after a stabilization period
// and stop emitting route_change alerts indefinitely.
const routeBaselineStaleThreshold = 7 * 24 * time.Hour

// decideRouteChangeStatus reports whether the latest observed route
// differs from the stored baseline. Returns (hasChange, stabilityPct).
//
// "Route change" is defined as: the current path differs from a known
// prior state. The known prior state is the baseline. Without a baseline
// there is nothing to compare against, so no "change" can be concluded
// — the path is simply observed-as-is, and any signature diversity is
// shown via the stability percentage rather than flagged as a change.
//
// Previously this function fell through to a "no baseline + multiple
// signatures → change" fallback. That fallback fired on the reverse
// direction of every bidirectional AGENT probe (which intentionally
// has no baseline) and on freshly-observed standalone probes, surfacing
// a route-change incident that named the current path but had no
// baseline to diff against. The UI showed those incidents as
// "current-only" with the route-change banner lit, even though no
// actual change had been observed.
func decideRouteChangeStatus(latestHops, baselineHops string, sigs map[string]int, traceCount int) (bool, float64) {
	if baselineHops == "" {
		// No baseline → nothing to compare against. Surface stability
		// (how often the dominant signature appears) so the UI can
		// still show how consistent the path is, but don't claim a
		// "change" we cannot substantiate.
		if traceCount <= 0 {
			return false, 100
		}
		return false, dominantSignatureStabilityPct(sigs, traceCount)
	}
	if hopSetJaccard(parseHopPath(baselineHops), parseHopPath(latestHops)) >= routeEcmpSimilarityThreshold {
		return false, 100
	}
	return true, dominantSignatureStabilityPct(sigs, traceCount)
}

func dominantSignatureStabilityPct(sigs map[string]int, traceCount int) float64 {
	if traceCount <= 0 {
		return 100
	}
	maxCount := 0
	for _, c := range sigs {
		if c > maxCount {
			maxCount = c
		}
	}
	return math.Round(float64(maxCount)/float64(traceCount)*100*10) / 10
}

func parseHopPath(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '>' || r == ' '
	})
	out := parts[:0]
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func hopSetJaccard(a, b []string) float64 {
	aSet := make(map[string]struct{}, len(a))
	for _, h := range a {
		if h == "" || h == "*" {
			continue
		}
		aSet[h] = struct{}{}
	}
	bSet := make(map[string]struct{}, len(b))
	for _, h := range b {
		if h == "" || h == "*" {
			continue
		}
		bSet[h] = struct{}{}
	}
	if len(aSet) == 0 && len(bSet) == 0 {
		return 1
	}
	intersection := 0
	for h := range bSet {
		if _, ok := aSet[h]; ok {
			intersection++
		}
	}
	union := len(aSet) + len(bSet) - intersection
	if union == 0 {
		return 1
	}
	return float64(intersection) / float64(union)
}

// mtrPathKey uniquely identifies a (probe, agent, target) tuple for grouping
// MTR rows. The (probe_id, agent_id) pair identifies the trace, target_agent
// distinguishes the destination for AGENT (bidirectional) probes (0 for
// non-agent targets like 8.8.8.8).
//
// probeAgentID is the OWNER of the probe. For a bidirectional AGENT probe it
// is the same value across forward and reverse rows; for a standalone MTR
// probe it equals agentID. Callers that aggregate across agents (hopIndex,
// sharedHops, commonTargets, totalRoutes, incidents) must key on
// probeAgentID — never on agentID — so a single probe doesn't inflate
// counts via its reverse direction.
type mtrPathKey struct {
	probeID      uint
	agentID      uint
	targetAgent  uint
	probeAgentID uint
}

// ownerAgent returns the canonical source agent for this MTR row — the
// probe OWNER (probe_agent_id), not the reporter (agent_id). Every
// cross-probe aggregation in ComputeWorkspaceRouteAnalysis must key on
// this so a single bidirectional AGENT probe (forward A→B + reverse B→A)
// contributes one row of attribution, not two.
//
// Falls back to the reporter when probeAgentID is 0 (legacy rows where
// the column wasn't populated).
func (k mtrPathKey) ownerAgent() uint {
	if k.probeAgentID != 0 {
		return k.probeAgentID
	}
	return k.agentID
}

// isReverse reports whether this row is the reverse direction of a
// bidirectional AGENT probe — i.e. the reporter is NOT the probe owner.
// Used by tests and by any caller that needs to distinguish the two
// directions of a single probe.
func (k mtrPathKey) isReverse() bool {
	return k.probeAgentID != 0 && k.probeAgentID != k.agentID
}

// routeKey uniquely identifies a (probe, agent, target) tuple for grouping
// ProbeRouteInfo entries. Both forward and reverse directions of the same
// bidirectional AGENT probe share the probeID but differ on agentID, so
// each direction gets its own routeKey — the UI can render both, while
// cross-probe aggregates (hopIndex, incidents, totalRoutes, commonTargets)
// dedupe by probeID at the call site.
type routeKey struct {
	probeID     uint
	agentID     uint
	targetAgent uint // 0 for non-AGENT targets
}

// destStats is the per-target cross-agent aggregation bucket used to
// build CommonTargets / SharedDestinations. probeIDs dedupes
// per-probe contributions so a single bidirectional AGENT probe
// (forward A→B + reverse B→A stored as two (probe, agent, target)
// groups) counts as 1 in probeCount.
type destStats struct {
	agents      map[uint]bool
	probeCount  int
	probeIDs    map[uint]struct{}
	totalLat    float64
	totalLoss   float64
	latSamples  int
	lossSamples int
	hasIssues   bool
	targetIP    string
	displayName string
}

// mtrPathAggConfig wires the per-pathKey aggregation step to the shared
// state held by ComputeWorkspaceRouteAnalysis. Extracting this out of the
// main function lets the inflation-fix logic be unit-tested without
// standing up ClickHouse + Postgres.
type mtrPathAggConfig struct {
	ARIByAgent      map[uint]*AgentRouteInfo
	AgentByID       map[uint]agentInfo
	AgentIPToID     map[string]uint
	CommonTargetKey func(string) string
	RouteByKey      map[routeKey]*ProbeRouteInfo
	HopIndex        map[string]map[uint]HopMetrics
	DestAgg         map[string]*destStats
	SeenProbeIDs    map[uint]struct{}
	// IncidentProbeIDs dedupes route_change incidents per
	// (probe_id, attribution_id) so the forward and reverse directions
	// of a bidirectional AGENT probe can each surface their own
	// incident. Keyed on probeID<<32 | agentID.
	IncidentProbeIDs   map[uint]struct{}
	LoadBaselineForDir func(probeID, reportingAgentID uint) (routeBaseline, bool)
}

type mtrPathAgg struct {
	cfg mtrPathAggConfig
}

func newMTRPathAgg(cfg mtrPathAggConfig) *mtrPathAgg {
	return &mtrPathAgg{cfg: cfg}
}

// process is the body of the per-pathKey loop in ComputeWorkspaceRouteAnalysis.
//
// Attribution model:
//
//   - Forward direction (or standalone MTR): attribution = probe OWNER
//     (pathKey.ownerAgent()), which is also the reporter (agent_id ==
//     probe_agent_id).
//   - Reverse direction of a bidirectional AGENT probe: attribution = the
//     REPORTER (pathKey.agentID), which is the target agent running the
//     return-path MTR. The reverse is a separate test from that agent's
//     perspective and must NOT be folded into the owner's route / hop /
//     destination aggregates — doing so shows the owner a route they are
//     not actually traversing, and triggers false-positive route_change
//     incidents when the reverse path's ECMP signatures differ from the
//     forward's stable signature.
//
// Forward and reverse of the same probe share the same probe_id, so
// totalRoutes still counts one probe (not two). Incident dedupe is keyed
// on (probe_id, attribution_id) so each direction can surface its own
// incident when appropriate.
func (a *mtrPathAgg) process(pathKey mtrPathKey, rows []ProbeData, routeIncidents *[]RouteIncident) {
	cfg := &a.cfg

	ownerID := pathKey.ownerAgent()
	// Attribution: reverse direction is attributed to the reporter (the
	// agent running the return-path MTR), not the probe owner. Forward
	// direction and standalone MTR keep owner attribution (reporter ==
	// owner for those).
	attributionID := ownerID
	if pathKey.isReverse() {
		attributionID = pathKey.agentID
	}

	// Skip if the attribution agent isn't in this workspace. For reverse
	// rows this also covers the case where the probe owner is in the
	// workspace but the reverse reporter is not — the MTR query already
	// filters by reporter IN (workspace agents), so a missing reverse
	// reporter would mean the row shouldn't be in the result set at all.
	ari, ok := cfg.ARIByAgent[attributionID]
	if !ok {
		return
	}

	cfg.SeenProbeIDs[pathKey.probeID] = struct{}{}

	// Resolve target: for AGENT probes target_agent is set; otherwise
	// fall back to the resolved target IP / hostname from the payload.
	target, targetIP, targetAgentName := resolveMTRTarget(cfg.AgentByID, rows, pathKey.targetAgent)

	rk := routeKey{probeID: pathKey.probeID, agentID: pathKey.agentID, targetAgent: pathKey.targetAgent}
	pri, exists := cfg.RouteByKey[rk]
	if !exists {
		pri = &ProbeRouteInfo{
			ProbeID: pathKey.probeID,
			// AgentID is the attribution agent. For the forward
			// direction this is the owner; for the reverse direction
			// this is the reporter (target agent) so the return path
			// shows up under the agent actually running the test.
			AgentID:       attributionID,
			Target:        target,
			TargetAgentID: pathKey.targetAgent,
		}
		if targetAgentName != "" {
			pri.TargetAgentName = targetAgentName
		}
		if b, ok := cfg.LoadBaselineForDir(pathKey.probeID, pathKey.agentID); ok {
			pri.BaselineFingerprint = b.Fingerprint
			pri.BaselineHopCount = b.HopCount
			pri.BaselineRoutePath = b.RoutePath
		}
		cfg.RouteByKey[rk] = pri
	}

	// Process the MTR rows for this (probe, agent, target) tuple.
	sigs := make(map[string]int)
	var latestPayload *MtrPayload
	for i := range rows {
		var mp MtrPayload
		if err := json.Unmarshal(rows[i].Payload, &mp); err != nil {
			continue
		}
		sig := getMtrRouteSignature(mp.Report.Hops)
		sigs[sig]++
		if latestPayload == nil {
			latestPayload = &mp
			pri.LatestSignature = sig
			for _, hop := range mp.Report.Hops {
				if len(hop.Hosts) > 0 && hop.Hosts[0].IP != "" {
					pri.LatestHops = append(pri.LatestHops, hop.Hosts[0].IP)
				}
			}
			pri.LatestHopsDetail = buildHopDetails(latestPayload, cfg.AgentIPToID, cfg.AgentByID)
		}
	}
	if latestPayload != nil {
		pri.TraceCount = len(rows)
		pri.HasRouteChange, pri.RouteStabilityPct = decideRouteChangeStatus(pri.LatestSignature, pri.BaselineRoutePath, sigs, len(rows))
		// When the route has changed, find the first (newest) MTR row
		// whose signature differs from the baseline so the UI can show
		// how long the route has been changed. Rows are sorted
		// newest-first, so the first match gives us the upper bound
		// on the change duration.
		if pri.HasRouteChange && pri.BaselineRoutePath != "" {
			for i := range rows {
				var mp MtrPayload
				if err := json.Unmarshal(rows[i].Payload, &mp); err != nil {
					continue
				}
				hops := make([]string, 0, len(mp.Report.Hops))
				for _, h := range mp.Report.Hops {
					if len(h.Hosts) > 0 && h.Hosts[0].IP != "" {
						hops = append(hops, h.Hosts[0].IP)
					}
				}
				if hopSetJaccard(parseHopPath(pri.BaselineRoutePath), hops) < routeEcmpSimilarityThreshold {
					ts := rows[i].CreatedAt
					pri.RouteChangedAt = &ts
					break
				}
			}
		}
		if len(latestPayload.Report.Hops) > 0 {
			lastHop := latestPayload.Report.Hops[len(latestPayload.Report.Hops)-1]
			pri.AvgEndHopLatency = parseLatency(lastHop.Avg)
			pri.AvgEndHopLoss = parseLossPct(lastHop.LossPct)
		}
		hopCount := len(latestPayload.Report.Hops)
		if hopCount > 1 {
			for i := 0; i < hopCount-1; i++ {
				hop := latestPayload.Report.Hops[i]
				if len(hop.Hosts) == 0 || hop.Hosts[0].IP == "" || hop.Hosts[0].IP == "*" {
					continue
				}
				pri.IntermediateHops = append(pri.IntermediateHops, HopMetric{
					IP:       hop.Hosts[0].IP,
					Loss:     parseLossPct(hop.LossPct),
					Latency:  parseLatency(hop.Avg),
					HopIndex: i,
				})
			}
		}
	}

	// Index hops for shared-hop computation (now includes the final
	// hop so shared destinations surface in shared_hops too).
	//
	// Attribution: hop is attributed to the agent running the trace
	// (the REPORTER), which for forward direction is the owner and for
	// reverse direction is the target agent. Without this, a single
	// bidirectional AGENT probe would attach the reverse's hops to the
	// owner's profile and surface them as if the owner had reached
	// them — the owner never ran that trace.
	if len(pri.LatestHops) >= 1 {
		for idx, ip := range pri.LatestHops {
			if ip == "" || ip == "*" {
				continue
			}
			if cfg.HopIndex[ip] == nil {
				cfg.HopIndex[ip] = make(map[uint]HopMetrics)
			}
			metrics := HopMetrics{Count: 1}
			matched := false
			for _, ih := range pri.IntermediateHops {
				if ih.IP == ip {
					metrics.TotalLoss += ih.Loss
					metrics.TotalLatency += ih.Latency
					if ih.Loss > 0 || ih.Latency > 100 {
						metrics.HasIssues = true
					}
					matched = true
					break
				}
			}
			if !matched && idx == len(pri.LatestHops)-1 {
				metrics.TotalLoss += pri.AvgEndHopLoss
				metrics.TotalLatency += pri.AvgEndHopLatency
				if pri.AvgEndHopLoss > 0 || pri.AvgEndHopLatency > 100 {
					metrics.HasIssues = true
				}
			}
			cfg.HopIndex[ip][attributionID] = metrics
		}
	}

	// Aggregate per-target stats for cross-agent views. Attribute to
	// the agent running the trace (REPORTER). For the forward
	// direction this is the owner; for the reverse direction this is
	// the target agent. A single AGENT probe no longer "double counts"
	// the owner reaching the target via the reverse row, and the
	// reverse's destination (the owner, from B's perspective) is
	// correctly attributed to B.
	if target != "" {
		key2 := cfg.CommonTargetKey(target)
		ds, ok := cfg.DestAgg[key2]
		if !ok {
			ds = &destStats{
				agents:      make(map[uint]bool),
				displayName: target,
				probeIDs:    make(map[uint]struct{}),
			}
			cfg.DestAgg[key2] = ds
		}
		if !ds.agents[attributionID] {
			ds.agents[attributionID] = true
		}
		// probeCount is the number of distinct probes reaching this
		// target. Keying on probe_id still collapses forward/reverse of
		// the same AGENT probe into one (both directions share the
		// probe's view of the destination).
		if _, dup := ds.probeIDs[pathKey.probeID]; !dup {
			ds.probeIDs[pathKey.probeID] = struct{}{}
			ds.probeCount++
		}
		if pri.AvgEndHopLatency > 0 {
			ds.totalLat += pri.AvgEndHopLatency
			ds.latSamples++
		}
		if pri.AvgEndHopLoss > 0 {
			ds.totalLoss += pri.AvgEndHopLoss
			ds.lossSamples++
		}
		if pri.HasRouteChange {
			ds.hasIssues = true
		}
		if ds.targetIP == "" {
			if targetIP != "" {
				ds.targetIP = targetIP
			} else if len(pri.LatestHops) > 0 {
				ds.targetIP = pri.LatestHops[len(pri.LatestHops)-1]
			}
		}
	}

	// Route-change incident — emit at most one per (probe_id,
	// attribution_id). Forward and reverse of the same probe have
	// different attribution IDs (owner vs. reporter), so each
	// direction can surface its own incident. Within a single
	// direction the (probe_id, attribution_id) key prevents duplicate
	// incidents when the same probe has multiple targets.
	if !pri.HasRouteChange {
		return
	}
	incidentKey := pathKey.probeID<<32 | attributionID
	if _, alreadyEmitted := cfg.IncidentProbeIDs[incidentKey]; alreadyEmitted {
		return
	}
	cfg.IncidentProbeIDs[incidentKey] = struct{}{}

	// Build a structured diff so the UI can render a "before / after"
	// view of the actual IP paths, not just fingerprint hashes.
	// baselineHops preserves the order in which the baseline was first
	// observed; latestHops is the order of the most recent trace.
	baselineHops := parseHopPath(pri.BaselineRoutePath)
	latestHops := filterProbeRouteHops(pri.LatestHops)
	addedHops, removedHops := diffHopsOrdered(baselineHops, latestHops)
	jaccard := hopSetJaccard(baselineHops, latestHops)
	currentFingerprint := pri.LatestSignature

	evidence := []string{
		fmt.Sprintf("Current signature: %s", pri.LatestSignature),
	}
	if pri.BaselineFingerprint != "" {
		evidence = append(evidence, fmt.Sprintf("Baseline fingerprint: %s", pri.BaselineFingerprint))
	}
	evidence = append(evidence, fmt.Sprintf("Route stability: %.0f%% over %d traces", pri.RouteStabilityPct, pri.TraceCount))

	incident := RouteIncident{
		ID:                  fmt.Sprintf("route_change_%d_%d", attributionID, pathKey.probeID),
		Type:                "route_change",
		Severity:            "warning",
		AgentID:             attributionID,
		AgentName:           ari.AgentName,
		ProbeID:             pathKey.probeID,
		Target:              target,
		Message:             fmt.Sprintf("Route changed for %s → %s (stability %.0f%%)", ari.AgentName, target, pri.RouteStabilityPct),
		Evidence:            evidence,
		BaselineFingerprint: pri.BaselineFingerprint,
		CurrentFingerprint:  currentFingerprint,
		BaselinePath:        pri.BaselineRoutePath,
		CurrentPath:         strings.Join(latestHops, " -> "),
		BaselineHopCount:    pri.BaselineHopCount,
		CurrentHopCount:     len(latestHops),
		AddedHops:           addedHops,
		RemovedHops:         removedHops,
		Jaccard:             jaccard,
		StabilityPct:        pri.RouteStabilityPct,
		TraceCount:          pri.TraceCount,
	}
	if pri.RouteChangedAt != nil {
		incident.DetectedAt = pri.RouteChangedAt.UTC().Format(time.RFC3339)
	}
	*routeIncidents = append(*routeIncidents, incident)
}

// filterProbeRouteHops strips "no response" placeholders so the diff
// stays readable. ProbeRouteInfo.LatestHops already filters "*" in the
// writer, but the baseline path may contain "-> *" segments, so we also
// skip those here for symmetry.
func filterProbeRouteHops(hops []string) []string {
	out := make([]string, 0, len(hops))
	for _, h := range hops {
		if h == "" || h == "*" {
			continue
		}
		out = append(out, h)
	}
	return out
}

// diffHopsOrdered returns the IPs that appear only in current (added) and
// only in baseline (removed), preserving first-seen order so the UI can
// show "hop 4 swapped for a new one" rather than two unsorted lists.
// Empty strings and "*" placeholders are skipped on both sides.
func diffHopsOrdered(baseline, current []string) (added, removed []string) {
	baseSet := make(map[string]struct{}, len(baseline))
	for _, h := range baseline {
		if h == "" || h == "*" {
			continue
		}
		baseSet[h] = struct{}{}
	}
	currSet := make(map[string]struct{}, len(current))
	for _, h := range current {
		if h == "" || h == "*" {
			continue
		}
		currSet[h] = struct{}{}
	}
	for _, h := range current {
		if h == "" || h == "*" {
			continue
		}
		if _, ok := baseSet[h]; !ok {
			added = append(added, h)
		}
	}
	for _, h := range baseline {
		if h == "" || h == "*" {
			continue
		}
		if _, ok := currSet[h]; !ok {
			removed = append(removed, h)
		}
	}
	return added, removed
}

// getWorkspaceMTRByPath fetches all MTR rows for the given agents in a single
// batched ClickHouse query and groups them by (probe_id, agent_id,
// target_agent). Each group is sorted newest-first.
//
// The previous per-probe loop iterated ListByAgent and skipped probes whose
// Type wasn't "MTR" — which silently excluded AGENT (bidirectional) probes
// even though those probes are the primary source of MTR data in modern
// installations. Driving from probe_data type='MTR' rows instead means
// both standalone MTR and AGENT probes are handled uniformly.
func getWorkspaceMTRByPath(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time, perGroupLimit int) (map[mtrPathKey][]ProbeData, error) {
	out := make(map[mtrPathKey][]ProbeData)
	if len(agentIDs) == 0 {
		return out, nil
	}

	idStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		idStrs[i] = fmt.Sprintf("%d", id)
	}
	agentIDList := strings.Join(idStrs, ", ")

	// Pull enough rows per (probe, agent, target) group to compute stable
	// signatures. We OVER-fetch and dedupe in Go to avoid fragile
	// window-function queries. 200 rows per group is plenty for stability %.
	//
	// probe_agent_id is the OWNER of the probe (== agent_id for standalone
	// probes). It's read here so the downstream route analysis can attribute
	// hops / counts / incidents to the owner instead of the reporter, which
	// would otherwise inflate aggregates for bidirectional AGENT probes
	// (one probe → two reporters → false "shared" between them).
	q := fmt.Sprintf(`
SELECT
    created_at,
    probe_id,
    agent_id,
    probe_agent_id,
    target_agent,
    payload_raw
FROM probe_data
WHERE type = 'MTR'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 5000
`, agentIDList, chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("mtr query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var createdAt time.Time
		var probeID, agentID, probeAgentID, targetAgent uint64
		var payloadRaw string
		if err := rows.Scan(&createdAt, &probeID, &agentID, &probeAgentID, &targetAgent, &payloadRaw); err != nil {
			continue
		}
		// Legacy rows may have probe_agent_id == 0. Fall back to agent_id so
		// the owner is always non-zero in the key (callers use 0 to mean
		// "unknown", which would otherwise fall through to reporter-only
		// attribution and re-introduce the inflation bug).
		if probeAgentID == 0 {
			probeAgentID = agentID
		}
		key := mtrPathKey{
			probeID:      uint(probeID),
			agentID:      uint(agentID),
			targetAgent:  uint(targetAgent),
			probeAgentID: uint(probeAgentID),
		}
		if perGroupLimit > 0 && len(out[key]) >= perGroupLimit {
			continue
		}
		out[key] = append(out[key], ProbeData{
			CreatedAt:    createdAt,
			Type:         TypeMTR,
			ProbeID:      uint(probeID),
			ProbeAgentID: uint(probeAgentID),
			AgentID:      uint(agentID),
			TargetAgent:  uint(targetAgent),
			Payload:      []byte(payloadRaw),
		})
	}
	return out, rows.Err()
}

// resolveMTRTarget produces a display-friendly (target, targetIP, agentName)
// tuple for an MTR trace group. The priority is:
//
//  1. The target_agent's agent name (for AGENT / bidirectional probes) —
//     preferred because users recognise "Bob's laptop" over "10.0.0.5".
//  2. The MTR payload's resolved target.hostname (DNS hostname) — used when
//     the probe is targeting a literal hostname like "google.com".
//  3. The MTR payload's resolved target.ip — last resort; only used when
//     the trace was to an IP literal we can't otherwise name.
//
// targetIP is always populated from the final hop IP so the shared-destination
// card can show both "Bob's laptop" and "10.0.0.5" together.
func resolveMTRTarget(agentByID map[uint]agentInfo, rows []ProbeData, targetAgent uint) (target, targetIP, targetAgentName string) {
	if targetAgent != 0 {
		if a, ok := agentByID[targetAgent]; ok {
			targetAgentName = a.Name
		}
	}

	// Always derive the final-hop IP from the most recent MTR payload —
	// useful for both display and for cross-referencing with NETINFO / DNS.
	for i := range rows {
		var mp MtrPayload
		if err := json.Unmarshal(rows[i].Payload, &mp); err != nil {
			continue
		}
		if targetIP == "" {
			targetIP = mp.Report.HopFinalIP()
		}
		break // only need the most recent row for IP
	}

	// 1. AGENT probe → use the target agent's name.
	if targetAgentName != "" {
		return targetAgentName, targetIP, targetAgentName
	}

	// 2/3. Non-AGENT probe → use the payload's resolved hostname, or IP.
	for i := range rows {
		if t := extractMTRTargetHostname(rows[i].Payload); t != "" {
			return t, targetIP, ""
		}
	}
	return targetIP, targetIP, ""
}

// extractMTRTargetHostname pulls report.info.target.hostname out of a raw
// MTR payload without depending on the public MtrPayload struct (which
// intentionally omits Info). Falls back to .target.ip if no hostname.
func extractMTRTargetHostname(raw []byte) string {
	var probe struct {
		Report struct {
			Info struct {
				Target struct {
					Hostname string `json:"hostname"`
					IP       string `json:"ip"`
				} `json:"target"`
			} `json:"info"`
		} `json:"report"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	if probe.Report.Info.Target.Hostname != "" {
		return probe.Report.Info.Target.Hostname
	}
	return probe.Report.Info.Target.IP
}

// buildSharedASNs groups shared hop IPs by their autonomous system. Each
// emitted SharedASNInfo represents an ASN whose network is traversed by
// 2+ agents, with rollup latency/loss across the contributing hops.
//
// The geoStore parameter is an interface to keep this package import-free
// of the geoip package. Pass nil to skip ASN grouping (e.g. when no
// MaxMind DB is configured — HasASN() returns false in that case anyway).
func buildSharedASNs(geoStore GeoIPResolver, hopIndex map[string]map[uint]HopMetrics, agentByID map[uint]agentInfo) []SharedASNInfo {
	if geoStore == nil || !geoStore.HasASN() {
		return []SharedASNInfo{}
	}

	// ASN -> set of contributing hop IPs (for dedup in output)
	type asnBucket struct {
		asn      uint
		org      string
		hopIPs   map[string]struct{}
		agents   map[uint]bool
		hasIssue bool
		latSum   float64
		lossSum  float64
		latN     int
		lossN    int
	}
	buckets := make(map[uint]*asnBucket)

	for hopIP, agentMetricsMap := range hopIndex {
		if len(agentMetricsMap) < 2 {
			continue
		}
		asn, org, ok := geoStore.LookupASN(hopIP)
		if !ok || asn == 0 {
			continue
		}
		b, exists := buckets[asn]
		if !exists {
			b = &asnBucket{
				asn:    asn,
				org:    org,
				hopIPs: make(map[string]struct{}),
				agents: make(map[uint]bool),
			}
			buckets[asn] = b
		}
		b.hopIPs[hopIP] = struct{}{}
		var hopLat, hopLoss float64
		var hopLatN, hopLossN int
		for aid, metrics := range agentMetricsMap {
			b.agents[aid] = true
			if metrics.HasIssues {
				b.hasIssue = true
			}
			// Average per-agent latency/loss for this hop
			if metrics.Count > 0 {
				avgLat := metrics.TotalLatency / float64(metrics.Count)
				avgLoss := metrics.TotalLoss / float64(metrics.Count)
				hopLat += avgLat
				hopLatN++
				if avgLoss > 0 {
					hopLoss += avgLoss
					hopLossN++
				}
			}
		}
		b.latSum += hopLat
		b.latN += hopLatN
		b.lossSum += hopLoss
		b.lossN += hopLossN
	}

	out := make([]SharedASNInfo, 0, len(buckets))
	for _, b := range buckets {
		if len(b.agents) < 2 {
			continue
		}
		s := SharedASNInfo{
			ASN:        b.asn,
			ASNOrg:     b.org,
			AgentCount: len(b.agents),
			HasIssues:  b.hasIssue,
		}
		for ip := range b.hopIPs {
			s.HopIPs = append(s.HopIPs, ip)
		}
		for aid := range b.agents {
			s.AgentIDs = append(s.AgentIDs, aid)
			if a, ok := agentByID[aid]; ok {
				s.AgentNames = append(s.AgentNames, a.Name)
			}
		}
		if b.latN > 0 {
			s.AvgLatency = b.latSum / float64(b.latN)
		}
		if b.lossN > 0 {
			s.AvgLoss = b.lossSum / float64(b.lossN)
		}
		out = append(out, s)
	}

	// Sort: most agents first, then by ASN
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AgentCount != out[j].AgentCount {
			return out[i].AgentCount > out[j].AgentCount
		}
		return out[i].ASN < out[j].ASN
	})
	return out
}
