package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// ── Cross-Agent Correlation & Incident Detection ──

// detectIncidents correlates metrics across agents to find infrastructure-wide vs agent-specific issues
func detectIncidents(
	agents []AgentHealthSummary,
	pingMetrics map[string]pingStats,
	mtrMetrics map[string]mtrStats,
	trafficMetrics map[string]trafficStats,
	agentByID map[uint]agentInfo,
	lookbackMinutes int,
	agentIPToID map[string]uint,
) []DetectedIncident {
	var incidents []DetectedIncident

	// Confidence scaling: number of affected agents / total agents in workspace
	totalAgents := len(agents)
	confScale := func(affected int) float64 {
		if totalAgents == 0 {
			return 0.3
		}
		return math.Min(1.0, float64(affected)/float64(totalAgents)*1.5+0.2)
	}

	// 1. Shared-target correlation: find targets seen by multiple agents with degradation
	type targetIssue struct {
		target        string
		agentNames    []string
		probeTypes    map[string]bool
		latencyValues []float64
		lossValues    []float64
	}
	targetMap := make(map[string]*targetIssue)

	// Analyze PING metrics across agents
	for key, stats := range pingMetrics {
		target := extractTarget(key)
		if stats.PacketLoss > 1 || stats.AvgLatency > 100 {
			agentName := resolveAgentName(key, agentByID)
			if targetMap[target] == nil {
				targetMap[target] = &targetIssue{target: target, probeTypes: map[string]bool{}}
			}
			ti := targetMap[target]
			ti.agentNames = append(ti.agentNames, agentName)
			ti.probeTypes["PING"] = true
			ti.latencyValues = append(ti.latencyValues, stats.AvgLatency)
			ti.lossValues = append(ti.lossValues, stats.PacketLoss)
		}
	}

	// Analyze MTR metrics across agents
	for key, stats := range mtrMetrics {
		target := extractTarget(key)
		if stats.PacketLoss > 1 || stats.AvgLatency > 100 {
			agentName := resolveAgentName(key, agentByID)
			if targetMap[target] == nil {
				targetMap[target] = &targetIssue{target: target, probeTypes: map[string]bool{}}
			}
			ti := targetMap[target]
			ti.agentNames = append(ti.agentNames, agentName)
			ti.probeTypes["MTR"] = true
			ti.latencyValues = append(ti.latencyValues, stats.AvgLatency)
			ti.lossValues = append(ti.lossValues, stats.PacketLoss)
		}
	}

	// Analyze TrafficSim metrics across agents
	for key, stats := range trafficMetrics {
		target := extractTarget(key)
		if stats.PacketLoss > 1 || stats.AvgRTT > 100 {
			agentName := resolveAgentName(key, agentByID)
			if targetMap[target] == nil {
				targetMap[target] = &targetIssue{target: target, probeTypes: map[string]bool{}}
			}
			ti := targetMap[target]
			ti.agentNames = append(ti.agentNames, agentName)
			ti.probeTypes["TRAFFICSIM"] = true
			ti.latencyValues = append(ti.latencyValues, stats.AvgRTT)
			ti.lossValues = append(ti.lossValues, stats.PacketLoss)
		}
	}

	// Generate incidents from shared-target analysis
	for target, ti := range targetMap {
		uniqueAgents := uniqueStrings(ti.agentNames)
		avgLat := avg(ti.latencyValues)
		avgLoss := avg(ti.lossValues)

		if len(uniqueAgents) >= 2 {
			// Multiple agents see the same target degraded → infrastructure issue
			severity := "warning"
			if avgLoss > 5 || avgLat > 200 {
				severity = "critical"
			}

			var probeTypeList []string
			for pt := range ti.probeTypes {
				probeTypeList = append(probeTypeList, pt)
			}

			cause := suggestCause(avgLat, avgLoss, len(uniqueAgents), len(agents), ti.probeTypes)
			resolvedTarget := resolveTargetToName(stripPort(target), agentByID, agentIPToID)
			matchedCriteria := fmt.Sprintf("packet_loss > 1%% OR latency > 100ms (avg_loss: %.1f%%, avg_lat: %.1fms)", avgLoss, avgLat)
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("shared_target_%s", sanitizeKey(target)),
				Title:           fmt.Sprintf("Shared degradation to %s", resolvedTarget),
				Severity:        severity,
				Scope:           "infrastructure",
				SuggestedCause:  cause,
				AffectedAgents:  uniqueAgents,
				AffectedTargets: []string{resolvedTarget},
				Evidence: []string{
					fmt.Sprintf("%d agents affected: %s", len(uniqueAgents), strings.Join(uniqueAgents, ", ")),
					fmt.Sprintf("Avg latency: %.1fms, Avg loss: %.1f%%", avgLat, avgLoss),
					fmt.Sprintf("Detected via: %s", strings.Join(probeTypeList, ", ")),
				},
				Recommendations: suggestRemediation(cause, severity),
				Confidence:      confScale(len(uniqueAgents)),
				LookbackMinutes: lookbackMinutes,
				MatchedCriteria: matchedCriteria,
			})
		} else if len(uniqueAgents) == 1 && (avgLoss > 3 || avgLat > 200) {
			// Only one agent sees degradation to this target → agent-specific or local ISP
			severity := "warning"
			if avgLoss > 10 || avgLat > 400 {
				severity = "critical"
			}

			resolvedTarget := resolveTargetToName(stripPort(target), agentByID, agentIPToID)
			matchedCriteria := fmt.Sprintf("packet_loss > 3%% OR latency > 200ms (avg_loss: %.1f%%, avg_lat: %.1fms)", avgLoss, avgLat)
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("agent_target_%s_%s", sanitizeKey(uniqueAgents[0]), sanitizeKey(target)),
				Title:           fmt.Sprintf("Degradation from %s to %s", uniqueAgents[0], resolvedTarget),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("Likely local to %s — possible local ISP issue, network congestion, or routing problem specific to this path", uniqueAgents[0]),
				AffectedAgents:  uniqueAgents,
				AffectedTargets: []string{resolvedTarget},
				Evidence: []string{
					fmt.Sprintf("Only %s sees this issue (other agents to the same target are unaffected)", uniqueAgents[0]),
					fmt.Sprintf("Avg latency: %.1fms, Avg loss: %.1f%%", avgLat, avgLoss),
				},
				Recommendations: []string{
					fmt.Sprintf("Check the local network at %s for interface errors or congestion", uniqueAgents[0]),
					"Review MTR traces for the specific degraded hops",
					"Compare with other probe destinations from this agent",
				},
				Confidence:      0.4,
				LookbackMinutes: lookbackMinutes,
				MatchedCriteria: matchedCriteria,
			})
		}
	}

	// 2. Agent-level correlation: detect agents offline or fully degraded
	for _, agent := range agents {
		if !agent.IsOnline {
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("agent_offline_%d", agent.AgentID),
				Title:           fmt.Sprintf("%s is offline", agent.AgentName),
				Severity:        "critical",
				Scope:           "agent-specific",
				SuggestedCause:  "Agent has not reported in — possible host outage, network partition, or agent service failure",
				AffectedAgents:  []string{agent.AgentName},
				AffectedTargets: []string{},
				Evidence:        []string{"Agent has not sent a heartbeat within the expected interval"},
				Recommendations: []string{
					fmt.Sprintf("Check if the host running %s is reachable", agent.AgentName),
					"Verify the agent service is running (systemctl status netwatcher-agent)",
					"Check host resources (disk, memory, CPU)",
				},
				Confidence: 0.95,
			})
		} else if agent.Health.Grade == "critical" || agent.Health.Grade == "poor" {
			var worstTargets []string
			for _, p := range agent.WorstProbes {
				worstTargets = append(worstTargets, p.Target)
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("agent_degraded_%d", agent.AgentID),
				Title:           fmt.Sprintf("%s health degraded (grade: %s)", agent.AgentName, agent.Health.Grade),
				Severity:        agent.Health.Grade,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("All %d probes from %s show degradation — likely a local network issue or upstream provider problem at this location", agent.ProbeCount, agent.AgentName),
				AffectedAgents:  []string{agent.AgentName},
				AffectedTargets: worstTargets,
				Evidence: []string{
					fmt.Sprintf("Overall health: %.0f/100 (%s)", agent.Health.OverallHealth, agent.Health.Grade),
					fmt.Sprintf("MOS: %.2f", agent.Health.MosScore),
					fmt.Sprintf("%d probes monitored", agent.ProbeCount),
				},
				Recommendations: []string{
					"Check local network connectivity at this agent's location",
					"Review ISP status/outage pages for the agent's provider",
					"Compare latency trends to identify when degradation started",
				},
				Confidence: 0.75,
			})
		}
	}

	// 3. Infrastructure-wide detection: majority of agents degraded
	degradedCount := 0
	for _, agent := range agents {
		if !agent.IsOnline || agent.Health.Grade == "critical" || agent.Health.Grade == "poor" {
			degradedCount++
		}
	}
	if len(agents) > 1 && degradedCount >= len(agents)/2+1 {
		incidents = append(incidents, DetectedIncident{
			ID:              "infrastructure_wide",
			Title:           "Majority of agents reporting issues",
			Severity:        "critical",
			Scope:           "infrastructure",
			SuggestedCause:  fmt.Sprintf("%d of %d agents showing degradation or offline — possible upstream provider issue, DNS resolution problem, or widespread network event", degradedCount, len(agents)),
			AffectedAgents:  []string{},
			AffectedTargets: []string{},
			Evidence:        []string{fmt.Sprintf("%d/%d agents degraded or offline", degradedCount, len(agents))},
			Recommendations: []string{
				"Check shared infrastructure (DNS, upstream ISP, core routing)",
				"Review if a recent change (firewall rule, route update) could explain this",
				"Check external status pages (cloudflare, aws, etc.) for regional issues",
			},
			Confidence: confScale(degradedCount),
		})
	}

	return incidents
}

// suggestCause generates a human-readable root cause hypothesis
func suggestCause(avgLatency, avgLoss float64, affectedAgents, totalAgents int, probeTypes map[string]bool) string {
	parts := []string{}

	if affectedAgents >= totalAgents && totalAgents > 1 {
		parts = append(parts, "All agents are affected — likely an issue with the target destination or a shared upstream transit provider")
	} else if affectedAgents > 1 {
		parts = append(parts, fmt.Sprintf("%d of %d agents affected — possible shared peering point, transit provider, or regional network issue", affectedAgents, totalAgents))
	}

	// Detect ICMP rate limiting patterns (loss with low latency often indicates ICMP limiting)
	if probeTypes["PING"] || probeTypes["MTR"] {
		if avgLoss > 1 && avgLatency < 50 {
			parts = append(parts, "ICMP rate limiting detected (ping/MTR loss with low latency) — firewall or ISP may be throttling ICMP")
		}
	}

	if avgLoss > 10 {
		parts = append(parts, "High packet loss suggests network congestion, overloaded links, or an active outage along the path")
	} else if avgLoss > 3 {
		parts = append(parts, "Moderate packet loss may indicate congestion during peak hours or a degraded link")
	}

	if avgLatency > 300 {
		parts = append(parts, "Very high latency suggests route changes, satellite links, or severe congestion")
	} else if avgLatency > 150 {
		parts = append(parts, "Elevated latency may indicate suboptimal routing or congestion at peering points")
	}

	if len(parts) == 0 {
		return "Degradation detected — further investigation of MTR traces recommended to identify the specific hop or segment"
	}
	return strings.Join(parts, ". ")
}

// suggestRemediation returns actionable steps based on the cause
func suggestRemediation(cause, severity string) []string {
	steps := []string{
		"Review MTR traceroutes from affected agents to identify the degraded hop",
	}
	if strings.Contains(cause, "transit provider") || strings.Contains(cause, "peering") {
		steps = append(steps, "Contact the upstream provider if the degraded hop is in their network")
		steps = append(steps, "Check looking glass tools (e.g., bgp.tools, stat.ripe.net) for route changes")
	}
	if strings.Contains(cause, "congestion") {
		steps = append(steps, "Check if the issue correlates with time-of-day traffic patterns")
	}
	if severity == "critical" {
		steps = append(steps, "Escalate if the issue persists beyond 15 minutes and impacts production services")
	}
	return steps
}

// buildStatusSummary generates the high-level workspace status
func buildStatusSummary(health HealthVector, agents []AgentHealthSummary, incidents []DetectedIncident) StatusSummary {
	offlineCount := 0
	degradedCount := 0
	for _, a := range agents {
		if !a.IsOnline {
			offlineCount++
		} else if a.Health.Grade == "critical" || a.Health.Grade == "poor" {
			degradedCount++
		}
	}

	criticalIncidents := 0
	for _, inc := range incidents {
		if inc.Severity == "critical" {
			criticalIncidents++
		}
	}

	total := len(agents)
	activeIssues := len(incidents)

	switch {
	case total == 0:
		return StatusSummary{Status: "unknown", Message: "No agents configured", ActiveIssues: 0}
	case offlineCount == total:
		return StatusSummary{Status: "outage", Message: "All agents are offline — no monitoring data available", ActiveIssues: activeIssues}
	case criticalIncidents > 0:
		return StatusSummary{
			Status:       "degraded",
			Message:      fmt.Sprintf("%d critical issue(s) detected across your infrastructure", criticalIncidents),
			ActiveIssues: activeIssues,
		}
	case degradedCount > 0 || offlineCount > 0:
		msg := ""
		if offlineCount > 0 && degradedCount > 0 {
			msg = fmt.Sprintf("%d agent(s) offline, %d showing degraded performance", offlineCount, degradedCount)
		} else if offlineCount > 0 {
			msg = fmt.Sprintf("%d agent(s) offline", offlineCount)
		} else {
			msg = fmt.Sprintf("%d agent(s) showing degraded performance", degradedCount)
		}
		return StatusSummary{Status: "degraded", Message: msg, ActiveIssues: activeIssues}
	case health.Grade == "excellent" || health.Grade == "good":
		return StatusSummary{
			Status:       "healthy",
			Message:      fmt.Sprintf("All %d agents healthy — no issues detected", total),
			ActiveIssues: activeIssues,
		}
	default:
		return StatusSummary{
			Status:       "healthy",
			Message:      fmt.Sprintf("%d agents online, overall health: %s", total-offlineCount, health.Grade),
			ActiveIssues: activeIssues,
		}
	}
}

// ── Incident Detection Helpers ──

func extractTarget(key string) string {
	if idx := strings.Index(key, ":"); idx >= 0 {
		return key[idx+1:]
	}
	return key
}

func resolveAgentName(key string, agentByID map[uint]agentInfo) string {
	if idx := strings.Index(key, ":"); idx >= 0 {
		idStr := key[:idx]
		var id uint
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
			if a, ok := agentByID[id]; ok {
				return a.Name
			}
		}
		return idStr
	}
	return key
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// resolveTargetToName checks if the target IP matches any agent and returns the agent name.
// If no agent matches, the target string (which may already be a hostname after stripPort)
// is returned unchanged — this preserves the original probe host for non-agent targets.
func resolveTargetToName(target string, agentByID map[uint]agentInfo, agentIPToID map[string]uint) string {
	if agentID, ok := agentIPToID[target]; ok {
		if agent, ok := agentByID[agentID]; ok {
			return agent.Name
		}
	}
	return target
}

// buildAgentIPToIDMap builds a map from agent IP addresses to agent IDs.
// It prefers the agent's manual PublicIPOverride (admin-supplied) and falls
// back to the latest NETINFO-discovered PublicAddress so that targets
// recorded in ClickHouse — which are usually the NAT'd public IP observed
// during the probe, not the override — still resolve back to the agent
// name in "Shared degradation" incident titles.
func buildAgentIPToIDMap(agents []AgentHealthSummary, agentByID map[uint]agentInfo, netInfoByAgent map[uint]*netInfoPayload) map[string]uint {
	agentIPToID := make(map[string]uint)
	for _, agent := range agents {
		if a, ok := agentByID[agent.AgentID]; ok {
			if a.PublicIPOverride != "" {
				agentIPToID[a.PublicIPOverride] = agent.AgentID
			}
		}
		// Augment with the actual public IP observed in NETINFO. Don't
		// overwrite an entry already populated by PublicIPOverride so the
		// manual value stays authoritative when both are present.
		if ni, ok := netInfoByAgent[agent.AgentID]; ok && ni != nil && ni.PublicAddress != "" {
			if _, exists := agentIPToID[ni.PublicAddress]; !exists {
				agentIPToID[ni.PublicAddress] = agent.AgentID
			}
		}
	}
	return agentIPToID
}

func sanitizeKey(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, s)
}

// sanitizeFloat is defined in network_map.go (same package)

// ── Temporal Change Detection ──

func detectTemporalChanges(
	currentPing map[string]pingStats, baselinePing map[string]pingStats,
	currentTraffic map[string]trafficStats, baselineTraffic map[string]trafficStats,
	netInfoChanges []netInfoChange,
	sysInfoMetrics map[string]sysInfoStats,
	agentByID map[uint]agentInfo,
) []DetectedIncident {
	var incidents []DetectedIncident

	// 1. Latency/loss regression detection (PING)
	for key, current := range currentPing {
		baseline, exists := baselinePing[key]
		if !exists || baseline.Count < 3 {
			continue
		}
		agentName := resolveAgentName(key, agentByID)
		target := extractTarget(key)

		// Latency increased by >2x baseline
		if baseline.AvgLatency > 5 && current.AvgLatency > baseline.AvgLatency*2 {
			severity := "warning"
			if current.AvgLatency > baseline.AvgLatency*3 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("latency_regression_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("Latency regression to %s from %s", stripPort(target), agentName),
				Severity:        severity,
				Scope:           "target-specific",
				SuggestedCause:  fmt.Sprintf("Latency increased from %.1fms (baseline) to %.1fms (now) — possible route change or congestion", baseline.AvgLatency, current.AvgLatency),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("Baseline (7-day avg): %.1fms", baseline.AvgLatency),
					fmt.Sprintf("Current: %.1fms (%.0f%% increase)", current.AvgLatency, ((current.AvgLatency-baseline.AvgLatency)/baseline.AvgLatency)*100),
				},
				Recommendations: []string{
					"Compare MTR traces to identify if the routing path has changed",
					"Check if the increase correlates with specific times of day",
				},
			})
		}

		// Loss increased significantly from baseline
		if current.PacketLoss > 1 && baseline.PacketLoss < 0.5 {
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("loss_regression_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("New packet loss to %s from %s", stripPort(target), agentName),
				Severity:        "warning",
				Scope:           "target-specific",
				SuggestedCause:  fmt.Sprintf("Packet loss appeared: %.1f%% now vs %.1f%% baseline — possible link degradation", current.PacketLoss, baseline.PacketLoss),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("Baseline (7-day avg): %.1f%% loss", baseline.PacketLoss),
					fmt.Sprintf("Current: %.1f%% loss", current.PacketLoss),
				},
				Recommendations: []string{
					"Review MTR for the degraded hops",
					"Check if the target or intermediate network is under maintenance",
				},
			})
		}
	}

	// 2. SysInfo capacity warnings
	for agentKey, si := range sysInfoMetrics {
		var id uint
		if _, err := fmt.Sscanf(agentKey, "%d", &id); err != nil {
			continue
		}
		agentName := agentKey
		if a, ok := agentByID[id]; ok {
			agentName = a.Name
		}

		if si.MemUsagePct > 90 {
			severity := "warning"
			if si.MemUsagePct > 95 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("memory_high_%s", agentKey),
				Title:           fmt.Sprintf("High memory usage on %s", agentName),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("Memory at %.1f%% — the host may be running low on resources, which can affect probe accuracy", si.MemUsagePct),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{"host-resources"},
				Evidence: []string{
					fmt.Sprintf("Memory: %.1f%% used (%s / %s)",
						si.MemUsagePct,
						formatBytes(si.MemUsedBytes),
						formatBytes(si.MemTotalBytes)),
				},
				Recommendations: []string{
					"Check for runaway processes consuming memory",
					"Consider increasing host memory allocation",
				},
			})
		}

		if si.CPUUsagePct > 85 {
			severity := "warning"
			if si.CPUUsagePct > 95 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("cpu_high_%s", agentKey),
				Title:           fmt.Sprintf("High CPU usage on %s", agentName),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("CPU at %.1f%% — high CPU can cause probe timing inaccuracies", si.CPUUsagePct),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{"host-resources"},
				Evidence:        []string{fmt.Sprintf("CPU usage: %.1f%%", si.CPUUsagePct)},
				Recommendations: []string{
					"Check for CPU-intensive processes",
					"Verify probe scheduling isn't overlapping",
				},
			})
		}
	}

	// 4. NetInfo changes (IP/ISP changes)
	for _, change := range netInfoChanges {
		agentName := fmt.Sprintf("Agent %d", change.AgentID)
		if a, ok := agentByID[change.AgentID]; ok {
			agentName = a.Name
		}

		switch change.Field {
		case "public_ip":
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("ip_change_%d", change.AgentID),
				Title:           fmt.Sprintf("Public IP changed on %s", agentName),
				Severity:        "info",
				Scope:           "agent-specific",
				SuggestedCause:  "Public IP address changed — this may indicate a DHCP renewal, failover event, or ISP change",
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{},
				Evidence: []string{
					fmt.Sprintf("Previous: %s", change.OldValue),
					fmt.Sprintf("Current: %s", change.NewValue),
				},
				Recommendations: []string{
					"Verify if this was an expected change (DHCP, failover)",
					"Check if monitoring targets are still reachable from the new IP",
				},
			})
		case "isp":
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("isp_change_%d", change.AgentID),
				Title:           fmt.Sprintf("ISP changed on %s", agentName),
				Severity:        "warning",
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("ISP changed from %s to %s — this may indicate a WAN failover or circuit switch", change.OldValue, change.NewValue),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{},
				Evidence: []string{
					fmt.Sprintf("Previous ISP: %s", change.OldValue),
					fmt.Sprintf("Current ISP: %s", change.NewValue),
				},
				Recommendations: []string{
					"Verify if this was a planned failover",
					"Check if latency/loss metrics changed with the ISP switch",
					"Review SD-WAN or dual-WAN configuration if applicable",
				},
			})
		}
	}

	return incidents
}

// ── Speedtest Bandwidth Regression Detection ──

func detectSpeedtestIncidents(ctx context.Context, ch *sql.DB, agentIDs []uint, from, baselineFrom time.Time, agentByID map[uint]agentInfo) []DetectedIncident {
	if len(agentIDs) == 0 {
		return nil
	}

	current, err := getWorkspaceSpeedtestMetrics(ctx, ch, agentIDs, from)
	if err != nil || len(current) == 0 {
		return nil
	}

	baseline, _ := getWorkspaceSpeedtestMetrics(ctx, ch, agentIDs, baselineFrom)
	if len(baseline) == 0 {
		return nil
	}

	var incidents []DetectedIncident
	for key, curr := range current {
		base, exists := baseline[key]
		if !exists || base.Count < 3 || curr.Count < 3 {
			continue
		}

		agentName := resolveAgentName(key, agentByID)
		target := extractTarget(key)

		// Download regression: >50% drop when baseline was >10 Mbps
		if base.AvgDownload > 10 && curr.AvgDownload < base.AvgDownload*0.5 {
			severity := "warning"
			if curr.AvgDownload < base.AvgDownload*0.25 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("speedtest_dl_regression_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("Bandwidth regression detected for %s (%s)", agentName, stripPort(target)),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("Download speed dropped from %.1f Mbps to %.1f Mbps — possible ISP throttling, link degradation, or network congestion", base.AvgDownload, curr.AvgDownload),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("Baseline download: %.1f Mbps (from %d tests)", base.AvgDownload, base.Count),
					fmt.Sprintf("Current download: %.1f Mbps (from %d tests)", curr.AvgDownload, curr.Count),
					fmt.Sprintf("Latency: %.1fms, JitterAvg: %.1fms", curr.AvgLatency, curr.AvgJitterAvg),
				},
				Recommendations: []string{
					"Run a manual speed test to confirm results",
					"Check for ISP SLA violations or data caps",
					"Review interface error counts on the agent's host",
				},
				Confidence: 0.75,
			})
		}

		// Upload regression
		if base.AvgUpload > 10 && curr.AvgUpload < base.AvgUpload*0.5 {
			severity := "warning"
			if curr.AvgUpload < base.AvgUpload*0.25 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("speedtest_ul_regression_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("Upload bandwidth regression for %s (%s)", agentName, stripPort(target)),
				Severity:        severity,
				Scope:           "agent-specific",
				SuggestedCause:  fmt.Sprintf("Upload speed dropped from %.1f Mbps to %.1f Mbps — possible upstream congestion or ISP shaping", base.AvgUpload, curr.AvgUpload),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("Baseline upload: %.1f Mbps", base.AvgUpload),
					fmt.Sprintf("Current upload: %.1f Mbps", curr.AvgUpload),
				},
				Recommendations: []string{
					"Check for upstream ISP issues or contention ratio",
					"Verify QoS settings haven't changed",
				},
				Confidence: 0.70,
			})
		}
	}

	return incidents
}

// ── DNS Pattern Detection ──

func detectDNSIncidents(ctx context.Context, ch *sql.DB, agentIDs []uint, from time.Time, agentByID map[uint]agentInfo) []DetectedIncident {
	if len(agentIDs) == 0 {
		return nil
	}

	agentIDStrs := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		agentIDStrs[i] = fmt.Sprintf("%d", id)
	}

	q := fmt.Sprintf(`
SELECT agent_id, target, payload_raw
FROM probe_data
WHERE type = 'DNS'
  AND agent_id IN (%s)
  AND created_at >= %s
ORDER BY created_at DESC
LIMIT 1000
`, strings.Join(agentIDStrs, ", "), chQuoteTime(from))

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type dnsAccum struct {
		queryTimes []float64
		nxdomain   int
		total      int
		servfail   int
		respondIPs map[string]int
	}
	acc := make(map[string]*dnsAccum)

	for rows.Next() {
		var agentID uint64
		var target, payloadRaw string
		if err := rows.Scan(&agentID, &target, &payloadRaw); err != nil || payloadRaw == "" {
			continue
		}
		var p DNSPayload
		if err := json.Unmarshal([]byte(payloadRaw), &p); err != nil {
			continue
		}
		key := fmt.Sprintf("%d:%s", agentID, target)
		if acc[key] == nil {
			acc[key] = &dnsAccum{respondIPs: make(map[string]int)}
		}
		a := acc[key]
		a.total++
		a.queryTimes = append(a.queryTimes, p.QueryTimeMs)
		switch p.ResponseCode {
		case "NXDOMAIN":
			a.nxdomain++
		case "SERVFAIL":
			a.servfail++
		}
		if len(p.Answers) > 0 {
			a.respondIPs[p.Answers[0].Value]++
		}
	}

	var incidents []DetectedIncident
	for key, a := range acc {
		if a.total < 5 {
			continue
		}

		agentName := resolveAgentName(key, agentByID)
		target := extractTarget(key)
		nxPct := float64(a.nxdomain) / float64(a.total) * 100
		sfPct := float64(a.servfail) / float64(a.total) * 100

		// NXDOMAIN storm detection: >30% NXDOMAIN rate
		if nxPct > 30 {
			severity := "warning"
			if nxPct > 60 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("dns_nxdomain_storm_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("DNS NXDOMAIN storm from %s to %s", agentName, stripPort(target)),
				Severity:        severity,
				SuggestedCause:  fmt.Sprintf("%.1f%% of queries returned NXDOMAIN — possible domain expiry, misconfiguration, or DNS cache poisoning attack", nxPct),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("%d/%d queries NXDOMAIN (%.1f%%)", a.nxdomain, a.total, nxPct),
					fmt.Sprintf("Target: %s", target),
				},
				Recommendations: []string{
					"Verify the domain is still registered and DNS records are correct",
					"Check if the target DNS server is experiencing issues",
					"Review firewall logs for anomalous DNS query patterns",
				},
				Confidence: math.Min(0.95, 0.3+nxPct/100),
			})
		}

		// SERVFAIL storm: >20% SERVFAIL rate
		if sfPct > 20 && nxPct < 30 {
			severity := "warning"
			if sfPct > 50 {
				severity = "critical"
			}
			incidents = append(incidents, DetectedIncident{
				ID:              fmt.Sprintf("dns_servfail_%s", sanitizeKey(key)),
				Title:           fmt.Sprintf("DNS SERVFAIL errors from %s to %s", agentName, stripPort(target)),
				Severity:        severity,
				SuggestedCause:  fmt.Sprintf("%.1f%% of queries returned SERVFAIL — possible DNS server overload or recursive resolver failure", sfPct),
				AffectedAgents:  []string{agentName},
				AffectedTargets: []string{stripPort(target)},
				Evidence: []string{
					fmt.Sprintf("%d/%d queries SERVFAIL (%.1f%%)", a.servfail, a.total, sfPct),
					fmt.Sprintf("Target: %s", target),
				},
				Recommendations: []string{
					"Check DNS server status and resource usage",
					"Verify upstream DNS server is reachable",
					"Review DNSSEC validation failures if applicable",
				},
				Confidence: 0.75,
			})
		}

		// High query time (possible DNS amplification)
		if len(a.queryTimes) > 5 {
			avgQT := avg(a.queryTimes)
			if avgQT > 500 {
				severity := "warning"
				if avgQT > 2000 {
					severity = "critical"
				}
				incidents = append(incidents, DetectedIncident{
					ID:              fmt.Sprintf("dns_high_latency_%s", sanitizeKey(key)),
					Title:           fmt.Sprintf("High DNS latency from %s to %s", agentName, stripPort(target)),
					Severity:        severity,
					SuggestedCause:  fmt.Sprintf("Average DNS query time: %.1fms — possible DNS server overload, network path issue, or amplification attack pattern", avgQT),
					AffectedAgents:  []string{agentName},
					AffectedTargets: []string{stripPort(target)},
					Evidence: []string{
						fmt.Sprintf("Average query time: %.1fms across %d queries", avgQT, len(a.queryTimes)),
					},
					Recommendations: []string{
						"Check if the DNS server is under load or experiencing DoS",
						"Review upstream DNS provider status",
						"Consider switching to a faster DNS resolver (e.g., 1.1.1.1, 8.8.8.8)",
					},
					Confidence: 0.65,
				})
			}
		}
	}

	return incidents
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
