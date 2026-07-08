package probe

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ComputeWorkspaceAnalysis aggregates health vectors across all agents in a workspace
func ComputeWorkspaceAnalysis(ctx context.Context, ch *sql.DB, pg *gorm.DB, workspaceID uint, lookbackMinutes int) (*WorkspaceAnalysis, error) {
	if lookbackMinutes <= 0 {
		lookbackMinutes = 60
	}
	from := time.Now().UTC().Add(-time.Duration(lookbackMinutes) * time.Minute)

	// Get agents
	agents, err := getWorkspaceAgents(ctx, pg, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get agents: %w", err)
	}

	if len(agents) == 0 {
		return &WorkspaceAnalysis{
			WorkspaceID:   workspaceID,
			OverallHealth: HealthVector{Grade: "unknown", RouteStability: 100, MosScore: 1.0},
			Agents:        []AgentHealthSummary{},
			GeneratedAt:   time.Now().UTC(),
		}, nil
	}

	agentIDs := make([]uint, len(agents))
	agentByID := make(map[uint]agentInfo)
	for i, a := range agents {
		agentIDs[i] = a.ID
		agentByID[a.ID] = a
	}

	// Fetch metrics for all agents
	pingMetrics, _ := getWorkspacePingMetrics(ctx, ch, agentIDs, from)
	mtrMetrics, _ := getWorkspaceMTRMetrics(ctx, ch, pg, agentIDs, from)
	trafficMetrics, _ := getWorkspaceTrafficSimMetrics(ctx, ch, agentIDs, from)
	sysInfoMetrics, _ := getWorkspaceSysInfoMetrics(ctx, ch, agentIDs, from)
	netInfoChanges, _ := getWorkspaceNetInfoChanges(ctx, ch, agentIDs, from)

	// Fetch baseline metrics (7-day rolling average) for change detection
	baselineFrom := time.Now().UTC().Add(-7 * 24 * time.Hour)
	baselinePing, _ := getWorkspacePingMetrics(ctx, ch, agentIDs, baselineFrom)
	baselineTraffic, _ := getWorkspaceTrafficSimMetrics(ctx, ch, agentIDs, baselineFrom)

	// Build per-agent summaries
	var agentSummaries []AgentHealthSummary
	var allHealthScores []float64
	totalProbes := 0

	for _, agent := range agents {
		isOnline := time.Since(agent.UpdatedAt) < time.Minute

		// Collect metrics for probes FROM this agent
		var agentLatencies []float64
		var agentLoss []float64
		var agentJitterAvg []float64
		var probeEntries []ProbeHealthEntry

		prefix := fmt.Sprintf("%d:", agent.ID)

		// PING metrics
		for key, stats := range pingMetrics {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			target := key[len(prefix):]
			m := ProbeMetrics{
				AvgLatency:  stats.AvgLatency,
				PacketLoss:  stats.PacketLoss,
				SampleCount: stats.Count,
			}
			h := computeHealthVector(m, 100)
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    stripPort(target),
				ProbeType: "PING",
				Health:    h,
				Metrics:   m,
			})
			agentLatencies = append(agentLatencies, stats.AvgLatency)
			agentLoss = append(agentLoss, stats.PacketLoss)
		}

		// MTR metrics
		for key, stats := range mtrMetrics {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			target := key[len(prefix):]
			m := ProbeMetrics{
				AvgLatency:  stats.AvgLatency,
				PacketLoss:  stats.PacketLoss,
				JitterAvg:   stats.Jitter,
				SampleCount: stats.Count,
			}
			h := computeHealthVector(m, 100)
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    stripPort(target),
				ProbeType: "MTR",
				Health:    h,
				Metrics:   m,
			})
			agentLatencies = append(agentLatencies, stats.AvgLatency)
			agentLoss = append(agentLoss, stats.PacketLoss)
			agentJitterAvg = append(agentJitterAvg, stats.Jitter)
		}

		// TrafficSim metrics
		for key, stats := range trafficMetrics {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			target := key[len(prefix):]
			m := ProbeMetrics{
				AvgLatency:  stats.AvgRTT,
				PacketLoss:  stats.PacketLoss,
				SampleCount: stats.Count,
			}
			h := computeHealthVector(m, 100)
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    stripPort(target),
				ProbeType: "TRAFFICSIM",
				Health:    h,
				Metrics:   m,
			})
			agentLatencies = append(agentLatencies, stats.AvgRTT)
			agentLoss = append(agentLoss, stats.PacketLoss)
		}

		// Inbound paths: probes owned by OTHER agents that target this
		// agent. The path toward an agent is as much a part of its
		// health as the paths it originates — without these entries a
		// target-only agent (e.g. a fax server that never runs probes)
		// always graded "unknown" no matter how degraded the routes to
		// it were.
		inboundSrc := func(key string) string {
			// Metric keys are "<srcAgentID>:<target>".
			if i := strings.IndexByte(key, ':'); i > 0 {
				var srcID uint64
				fmt.Sscanf(key[:i], "%d", &srcID)
				if a, ok := agentByID[uint(srcID)]; ok {
					return a.Name
				}
			}
			return "remote agent"
		}
		for key, stats := range pingMetrics {
			if strings.HasPrefix(key, prefix) || stats.TargetAgent != agent.ID {
				continue
			}
			m := ProbeMetrics{
				AvgLatency:  stats.AvgLatency,
				PacketLoss:  stats.PacketLoss,
				SampleCount: stats.Count,
			}
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    "from " + inboundSrc(key),
				ProbeType: "PING (inbound)",
				Health:    computeHealthVector(m, 100),
				Metrics:   m,
			})
			agentLatencies = append(agentLatencies, stats.AvgLatency)
			agentLoss = append(agentLoss, stats.PacketLoss)
		}
		for key, stats := range mtrMetrics {
			if strings.HasPrefix(key, prefix) || stats.TargetAgent != agent.ID {
				continue
			}
			m := ProbeMetrics{
				AvgLatency:  stats.AvgLatency,
				PacketLoss:  stats.PacketLoss,
				JitterAvg:   stats.Jitter,
				SampleCount: stats.Count,
			}
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    "from " + inboundSrc(key),
				ProbeType: "MTR (inbound)",
				Health:    computeHealthVector(m, 100),
				Metrics:   m,
			})
			agentLatencies = append(agentLatencies, stats.AvgLatency)
			agentLoss = append(agentLoss, stats.PacketLoss)
			agentJitterAvg = append(agentJitterAvg, stats.Jitter)
		}
		for key, stats := range trafficMetrics {
			if strings.HasPrefix(key, prefix) || stats.TargetAgent != agent.ID {
				continue
			}
			m := ProbeMetrics{
				AvgLatency:  stats.AvgRTT,
				PacketLoss:  stats.PacketLoss,
				SampleCount: stats.Count,
			}
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    "from " + inboundSrc(key),
				ProbeType: "TRAFFICSIM (inbound)",
				Health:    computeHealthVector(m, 100),
				Metrics:   m,
			})
			agentLatencies = append(agentLatencies, stats.AvgRTT)
			agentLoss = append(agentLoss, stats.PacketLoss)
		}

		// SysInfo metrics (host health)
		if si, ok := sysInfoMetrics[fmt.Sprintf("%d", agent.ID)]; ok {
			sysScore := sysInfoHealthScore(si)
			probeEntries = append(probeEntries, ProbeHealthEntry{
				Target:    "host-resources",
				ProbeType: "SYSINFO",
				Health: HealthVector{
					OverallHealth:  clampScore(sysScore),
					Grade:          gradeFromScore(sysScore),
					RouteStability: 100,
					MosScore:       1.0,
				},
				Metrics: ProbeMetrics{SampleCount: 1},
			})
		}

		totalProbes += len(probeEntries)

		// Compute agent-level health
		var agentHealth HealthVector
		var dataGap bool
		if len(probeEntries) > 0 {
			avgLat := avg(agentLatencies)
			avgLossVal := avg(agentLoss)
			avgJitterAvgVal := avg(agentJitterAvg)

			agentMetrics := ProbeMetrics{
				AvgLatency: avgLat,
				PacketLoss: avgLossVal,
				JitterAvg:  avgJitterAvgVal,
			}
			agentHealth = computeHealthVector(agentMetrics, 100)
		} else {
			dataGap = true
			agentHealth = HealthVector{
				Grade:          "unknown",
				RouteStability: 100,
				MosScore:       1.0,
			}
		}

		if !isOnline {
			agentHealth.OverallHealth = 0
			agentHealth.Grade = gradeFromScore(0)
		} else if isOnline && dataGap {
			agentHealth.OverallHealth = math.Max(0, agentHealth.OverallHealth-10)
			agentHealth.Grade = gradeFromScore(agentHealth.OverallHealth)
		}

		allHealthScores = append(allHealthScores, agentHealth.OverallHealth)

		// Sort worst probes (by lowest overall health)
		sortProbesByHealth(probeEntries)
		worstCount := 3
		if len(probeEntries) < worstCount {
			worstCount = len(probeEntries)
		}

		agentSummaries = append(agentSummaries, AgentHealthSummary{
			AgentID:     agent.ID,
			AgentName:   agent.Name,
			IsOnline:    isOnline,
			Health:      agentHealth,
			ProbeCount:  len(probeEntries),
			WorstProbes: probeEntries[:worstCount],
		})
	}

	// Compute overall workspace health
	var overallHealth HealthVector
	if len(allHealthScores) > 0 {
		overall := avg(allHealthScores)
		overallHealth = HealthVector{
			OverallHealth: clampScore(overall),
			Grade:         gradeFromScore(overall),
			MosScore:      computeMos(avg(extractField(agentSummaries, "latency")), avg(extractField(agentSummaries, "loss")), avg(extractField(agentSummaries, "jitter"))),
		}
		// Compute sub-scores from agent averages
		overallHealth.LatencyScore = clampScore(avg(extractHealthField(agentSummaries, "latency_score")))
		overallHealth.PacketLossScore = clampScore(avg(extractHealthField(agentSummaries, "loss_score")))
		overallHealth.RouteStability = clampScore(avg(extractHealthField(agentSummaries, "route_stability")))
	} else {
		overallHealth = HealthVector{Grade: "unknown", RouteStability: 100, MosScore: 1.0}
	}

	// ── Cross-Agent Correlation & Incident Detection ──
	// Pull latest NETINFO for each agent so IP→agent resolution in
	// "Shared degradation" titles can map the agent's real public IP back
	// to its name when PublicIPOverride is unset.
	netInfoByAgent := getLatestNetInfoForAgents(ctx, ch, agentIDs, from)
	agentIPToID := buildAgentIPToIDMap(agentSummaries, agentByID, netInfoByAgent)
	incidents := detectIncidents(agentSummaries, pingMetrics, mtrMetrics, trafficMetrics, agentByID, lookbackMinutes, agentIPToID)

	// ── Temporal Change Detection ──
	changeIncidents := detectTemporalChanges(pingMetrics, baselinePing, trafficMetrics, baselineTraffic, netInfoChanges, sysInfoMetrics, agentByID)
	incidents = append(incidents, changeIncidents...)

	// ── Speedtest Bandwidth Regression Detection ──
	speedtestIncidents := detectSpeedtestIncidents(ctx, ch, agentIDs, from, baselineFrom, agentByID)
	incidents = append(incidents, speedtestIncidents...)

	// ── DNS Pattern Detection ──
	dnsIncidents := detectDNSIncidents(ctx, ch, agentIDs, from, agentByID)
	incidents = append(incidents, dnsIncidents...)

	// Build status summary
	status := buildStatusSummary(overallHealth, agentSummaries, incidents)

	// ── Optional LLM Enrichment ──
	// Trigger on incidents OR healthy state (periodic "all clear" summaries)
	if llmProvider != nil && llmProvider.Available() && (len(incidents) > 0 || status.Status == "healthy") {
		enriched := enrichWithLLM(ctx, status, incidents, agentSummaries, overallHealth, totalProbes)
		if enriched != "" {
			status.Message = enriched
		}
	}

	return &WorkspaceAnalysis{
		WorkspaceID:   workspaceID,
		OverallHealth: overallHealth,
		Status:        status,
		Incidents:     incidents,
		Agents:        agentSummaries,
		TotalProbes:   totalProbes,
		TotalAgents:   len(agents),
		GeneratedAt:   time.Now().UTC(),
	}, nil
}

// ── Helpers ──

func buildFindings(health HealthVector, metrics ProbeMetrics, path *MtrPathAnalysis, signals []AnalysisSignal) []AnalysisFinding {
	var findings []AnalysisFinding

	// Grade-based overall finding
	switch health.Grade {
	case "critical":
		findings = append(findings, AnalysisFinding{
			ID:       "overall_critical",
			Title:    "Critical Path Degradation",
			Severity: "critical",
			Category: "performance",
			Summary:  fmt.Sprintf("Overall health score is %.0f/100 (grade: critical). Immediate attention recommended.", health.OverallHealth),
			Evidence: []string{
				fmt.Sprintf("Avg Latency: %.1fms", metrics.AvgLatency),
				fmt.Sprintf("Packet Loss: %.2f%%", metrics.PacketLoss),
				fmt.Sprintf("MOS: %.2f", health.MosScore),
			},
			Steps: []string{
				"Check for ISP outages or congestion at peering points",
				"Review recent MTR traces for route changes",
				"Contact upstream provider if issues persist",
			},
		})
	case "poor":
		findings = append(findings, AnalysisFinding{
			ID:       "overall_poor",
			Title:    "Degraded Path Performance",
			Severity: "warning",
			Category: "performance",
			Summary:  fmt.Sprintf("Overall health score is %.0f/100 (grade: poor). Performance is significantly below optimal.", health.OverallHealth),
			Evidence: []string{
				fmt.Sprintf("Avg Latency: %.1fms", metrics.AvgLatency),
				fmt.Sprintf("Packet Loss: %.2f%%", metrics.PacketLoss),
			},
			Steps: []string{
				"Monitor for further degradation",
				"Check for traffic congestion during peak hours",
			},
		})
	case "excellent", "good":
		findings = append(findings, AnalysisFinding{
			ID:       "overall_healthy",
			Title:    "Path Health Normal",
			Severity: "info",
			Category: "performance",
			Summary:  fmt.Sprintf("Overall health score is %.0f/100 (grade: %s). Path is performing within acceptable parameters.", health.OverallHealth, health.Grade),
		})
	}

	// Path-specific findings
	if path != nil {
		if len(path.RateLimitedHops) > 0 {
			findings = append(findings, AnalysisFinding{
				ID:       "icmp_rate_limit",
				Title:    "ICMP Rate Limiting Detected (Measurement Artifact)",
				Severity: "info",
				Category: "measurement_artifact",
				Summary:  "Some intermediate routers appear to rate-limit ICMP TTL-exceeded responses. The reported loss at these hops is NOT affecting end-to-end traffic.",
				Evidence: []string{
					fmt.Sprintf("Affected hops: %v", path.RateLimitedHops),
					fmt.Sprintf("End-to-end loss: %.1f%%", path.AvgEndHopLoss),
				},
			})
		}
		if path.UniqueRoutes > 2 {
			findings = append(findings, AnalysisFinding{
				ID:       "route_instability",
				Title:    "Route Path Instability",
				Severity: "warning",
				Category: "routing",
				Summary:  fmt.Sprintf("Multiple route paths detected (%d unique routes, %.0f%% stability). This may indicate ECMP load balancing or flapping.", path.UniqueRoutes, path.RouteStabilityPct),
				Steps: []string{
					"Run MTR with TCP mode (mtr -T) to test for ECMP effects",
					"Compare routes at different times of day",
				},
			})
		}
	}

	return findings
}

// avg and minF/maxF are defined in clickhouse.go (same package)

func percentile(vals []float64, pct int) float64 {
	if len(vals) == 0 {
		return 0
	}
	// Simple percentile by sorting
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	// Insertion sort (good enough for our sizes)
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	idx := int(float64(len(sorted)-1) * float64(pct) / 100.0)
	return sorted[idx]
}

func sortProbesByHealth(entries []ProbeHealthEntry) {
	// Insertion sort by overall health ascending (worst first)
	for i := 1; i < len(entries); i++ {
		key := entries[i]
		j := i - 1
		for j >= 0 && entries[j].Health.OverallHealth > key.Health.OverallHealth {
			entries[j+1] = entries[j]
			j--
		}
		entries[j+1] = key
	}
}

func extractField(summaries []AgentHealthSummary, field string) []float64 {
	var out []float64
	for _, s := range summaries {
		if s.ProbeCount == 0 {
			continue
		}
		switch field {
		case "latency":
			if len(s.WorstProbes) > 0 {
				var total float64
				for _, p := range s.WorstProbes {
					total += p.Metrics.AvgLatency
				}
				out = append(out, total/float64(len(s.WorstProbes)))
			}
		case "loss":
			if len(s.WorstProbes) > 0 {
				var total float64
				for _, p := range s.WorstProbes {
					total += p.Metrics.PacketLoss
				}
				out = append(out, total/float64(len(s.WorstProbes)))
			}
		case "jitter":
			if len(s.WorstProbes) > 0 {
				var total float64
				for _, p := range s.WorstProbes {
					total += p.Metrics.JitterAvg
				}
				out = append(out, total/float64(len(s.WorstProbes)))
			}
		}
	}
	return out
}

func extractHealthField(summaries []AgentHealthSummary, field string) []float64 {
	var out []float64
	for _, s := range summaries {
		if s.ProbeCount == 0 {
			continue
		}
		switch field {
		case "latency_score":
			out = append(out, s.Health.LatencyScore)
		case "loss_score":
			out = append(out, s.Health.PacketLossScore)
		case "route_stability":
			out = append(out, s.Health.RouteStability)
		}
	}
	return out
}
