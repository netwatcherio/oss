package probe

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"netwatcher-controller/internal/agent"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ── Compute Agent Voice Quality ─────────────────────────────────────────────

// ComputeAgentVoiceQuality computes comprehensive voice quality metrics for an agent
// including forward path probes and return path probes.
//
// Bug fix: previously this function picked a single "best" forward and return
// path by MIN MOS score (named `bestForward` / `bestReturn` but actually
// worst). It now exposes three artifacts per direction:
//   - `ForwardPath` / `ReturnPath`   → the WORST-offender probe (backward compat)
//   - `AggregateForward` / `AggregateReturn` → sample-weighted average across all probes
//   - `Probes` → full per-probe list
//
// Bug fix: previously the baseline was fetched for the first probe only
// (loop `break` after first hit), so multi-target agents got an
// incorrect baseline. The new code fetches a per-probe baseline map
// and passes the matching entry into each heuristic.
//
// Bug fix: the heuristic thresholds were hard-coded literals; they now
// come from `ResolveVoiceThresholds` (defaults → admin global → per-workspace).
func ComputeAgentVoiceQuality(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint, from, to time.Time) (*VoiceQualitySummary, error) {
	agentObj, err := agent.GetAgentByID(ctx, db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent %d: %w", agentID, err)
	}

	// Resolve effective thresholds for this workspace.
	thresholds, terr := ResolveVoiceThresholds(db, workspaceSettingsFor(ctx, db, agentID))
	if terr != nil {
		log.Warnf("[voice] threshold resolution failed for agent %d, using defaults: %v", agentID, terr)
		thresholds = VoiceDefaultThresholds
	}

	// Get baseline (7 days before the analysis window)
	baselineFrom := from.Add(-7 * 24 * time.Hour)

	// Get owned probes (forward path)
	probes, err := ListForAgent(ctx, db, ch, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list probes for agent %d: %w", agentID, err)
	}

	// Separate TRAFFICSIM and AGENT probes (both are voice-relevant).
	// AGENT probes include PING / MTR / TRAFFICSIM child expansions; when
	// TRAFFICSIM data is missing we fall back to fetching PING-derived
	// voice metrics so agents with no VoIP server still produce a
	// meaningful report (rather than the empty-data fallback that
	// rendered MOS=4.5 "excellent" with no pairs).
	var trafficSimProbes []Probe
	var agentProbes []Probe
	for _, p := range probes {
		if p.ID == 0 {
			continue // Skip virtual probes
		}
		switch p.Type {
		case TypeTrafficSim:
			trafficSimProbes = append(trafficSimProbes, p)
		case TypeAgent, TypePing, TypeMTR:
			// MTR matters here: AGENT probes always expand an MTR
			// child (PING is opt-in, TRAFFICSIM needs a server on the
			// target), so for server-less targets the MTR child is
			// often the only probe carrying per-cycle data.
			agentProbes = append(agentProbes, p)
		}
	}

	// Fetch TrafficSim metrics for forward path
	forwardMetrics := make(map[uint]*VoicePathMetrics)
	for _, p := range trafficSimProbes {
		metrics, err := fetchVoicePathMetrics(ctx, ch, []uint{agentID}, p.ID, from)
		if err != nil || metrics == nil {
			continue
		}
		targetName := ""
		if len(p.Targets) > 0 && p.Targets[0].AgentID != nil {
			if ta, err := agent.GetAgentByID(ctx, db, *p.Targets[0].AgentID); err == nil {
				targetName = ta.Name
			}
		}
		targetIP := ""
		if len(p.Targets) > 0 {
			targetIP = p.Targets[0].Target
		}
		metrics.TargetAgentID = uint(0)
		if len(p.Targets) > 0 && p.Targets[0].AgentID != nil {
			metrics.TargetAgentID = *p.Targets[0].AgentID
		}
		metrics.TargetAgentName = targetName
		metrics.SourceAgentID = agentID
		metrics.SourceAgentName = agentObj.Name
		metrics.ProbeID = p.ID
		metrics.ProbeType = string(p.Type)
		if targetIP != "" {
			metrics.TargetAgentName = targetIP
		}
		forwardMetrics[p.ID] = metrics
	}

	// Same-probe bidirectional reverse: for a bidirectional AGENT
	// probe the far-end server reports the return stream under the
	// client's probe ID with its own agent_id. Fetching those rows
	// gives the pair its true return direction (and lets asymmetry
	// detection compare both views of the same session).
	//
	// Iterate ALL agent-targeted probes, not just the TrafficSim
	// children: an AGENT probe whose target runs no TrafficSim server
	// only expands MTR/PING children, but a global target still
	// reports return-path rows under the same probe ID.
	reverseCandidates := make([]Probe, 0, len(trafficSimProbes)+len(agentProbes))
	reverseCandidates = append(reverseCandidates, trafficSimProbes...)
	reverseCandidates = append(reverseCandidates, agentProbes...)
	reverseMetrics := make(map[uint]*VoicePathMetrics)
	for _, p := range reverseCandidates {
		if _, done := reverseMetrics[p.ID]; done {
			continue // sibling expansions share the probe ID
		}
		if len(p.Targets) == 0 || p.Targets[0].AgentID == nil {
			continue // arbitrary SIP endpoint — nothing reports back
		}
		remoteID := *p.Targets[0].AgentID
		if remoteID == 0 || remoteID == agentID {
			continue
		}
		metrics, err := fetchVoicePathMetrics(ctx, ch, []uint{remoteID}, p.ID, from)
		if err != nil || metrics == nil || metrics.SampleCount == 0 {
			// Global targets run return-path MTR/PING probes under
			// the same probe ID — use those when there's no reverse
			// TrafficSim stream.
			metrics, err = fetchFallbackVoiceMetrics(ctx, ch, []uint{remoteID}, p.ID, from)
			if err != nil || metrics == nil || metrics.SampleCount == 0 {
				continue
			}
		}
		remoteName := ""
		if fwd, ok := forwardMetrics[p.ID]; ok && fwd != nil {
			remoteName = fwd.TargetAgentName
		}
		if remoteName == "" {
			if ra, err := agent.GetAgentByID(ctx, db, remoteID); err == nil {
				remoteName = ra.Name
			}
		}
		metrics.SourceAgentID = remoteID
		metrics.SourceAgentName = remoteName
		metrics.TargetAgentID = agentID
		metrics.TargetAgentName = agentObj.Name
		metrics.ProbeID = p.ID
		metrics.ProbeType = string(p.Type)
		metrics.Direction = VoicePathReturn
		reverseMetrics[p.ID] = metrics
	}

	// Derived-metrics fallback, per FORWARD direction: compute voice
	// quality from PING (preferred) or MTR samples for probes whose
	// forward direction has no TRAFFICSIM data. Coverage is judged
	// per direction — reverse-only TrafficSim data must NOT suppress
	// the forward fallback, or a pair whose far end reports fine
	// while our own sim stream is absent renders return-path-only.
	// AGENT/TrafficSim data still wins over derived metrics for the
	// same direction of the same probe / target.
	coveredTargets := make(map[uint]bool)
	for _, m := range forwardMetrics {
		if m.TargetAgentID != 0 {
			coveredTargets[m.TargetAgentID] = true
		}
	}
	for _, p := range agentProbes {
		if _, ok := forwardMetrics[p.ID]; ok {
			continue // TrafficSim data already covers this probe's forward path
		}
		if len(p.Targets) > 0 && p.Targets[0].AgentID != nil && coveredTargets[*p.Targets[0].AgentID] {
			continue // another probe covers this target's forward path with sim data
		}
		metrics, err := fetchFallbackVoiceMetrics(ctx, ch, []uint{agentID}, p.ID, from)
		if err != nil || metrics == nil || metrics.SampleCount == 0 {
			continue
		}
		targetName := ""
		if len(p.Targets) > 0 && p.Targets[0].AgentID != nil {
			if ta, err := agent.GetAgentByID(ctx, db, *p.Targets[0].AgentID); err == nil {
				targetName = ta.Name
			}
		}
		targetIP := ""
		if len(p.Targets) > 0 {
			targetIP = p.Targets[0].Target
		}
		metrics.TargetAgentID = uint(0)
		if len(p.Targets) > 0 && p.Targets[0].AgentID != nil {
			metrics.TargetAgentID = *p.Targets[0].AgentID
		}
		metrics.TargetAgentName = targetName
		metrics.SourceAgentID = agentID
		metrics.SourceAgentName = agentObj.Name
		metrics.ProbeID = p.ID
		metrics.ProbeType = string(p.Type)
		if targetIP != "" {
			metrics.TargetAgentName = targetIP
		}
		forwardMetrics[p.ID] = metrics
	}

	// Find return path probes (reverse AGENT probes from other agents targeting this agent)
	reverseAgentProbes, err := FindReverseAgentProbes(ctx, db, agentID)
	if err != nil {
		log.Warnf("[voice] failed to find reverse agent probes for agent %d: %v", agentID, err)
	}

	// Merge in metrics from reverse AGENT probes (owned by other
	// agents, targeting this one), keyed by their probe ID. These are
	// the remote agents' own measurements toward this agent — the only
	// voice data for target-only agents, and extra return-path signal
	// otherwise.
	for _, rap := range reverseAgentProbes {
		if rap.ID == 0 {
			continue
		}
		sourceAgentID := rap.AgentID
		sourceAgent, err := agent.GetAgentByID(ctx, db, sourceAgentID)
		if err != nil {
			continue
		}
		for _, t := range rap.Targets {
			if t.AgentID != nil && *t.AgentID == agentID {
				metrics, err := fetchVoicePathMetrics(ctx, ch, []uint{sourceAgentID}, rap.ID, from)
				if err != nil || metrics == nil || metrics.SampleCount == 0 {
					// AGENT probes whose target doesn't run a TrafficSim
					// server only produce PING/MTR samples (PING is
					// opt-in, so often MTR alone). Fall back to the
					// derived paths so target-only agents (e.g. a fax
					// server that never originates probes) still get
					// return-path voice data.
					metrics, err = fetchFallbackVoiceMetrics(ctx, ch, []uint{sourceAgentID}, rap.ID, from)
					if err != nil || metrics == nil || metrics.SampleCount == 0 {
						continue
					}
				}
				metrics.TargetAgentID = agentID
				metrics.TargetAgentName = agentObj.Name
				metrics.SourceAgentID = sourceAgentID
				metrics.SourceAgentName = sourceAgent.Name
				metrics.ProbeID = rap.ID
				metrics.ProbeType = string(rap.Type)
				metrics.Direction = VoicePathReturn
				reverseMetrics[rap.ID] = metrics
			}
		}
	}

	// One line per report computation — cheap, and the first thing to
	// check when a report comes back empty: which probe buckets the
	// engine saw and how many produced metrics per direction.
	log.Infof("[voice] agent %d (%s): probes trafficsim=%d agent/ping/mtr=%d reverse=%d → metrics forward=%d reverse=%d",
		agentID, agentObj.Name, len(trafficSimProbes), len(agentProbes), len(reverseAgentProbes), len(forwardMetrics), len(reverseMetrics))

	// Worst-offender (lowest MOS) for backward compat with the existing
	// `ForwardPath` / `ReturnPath` JSON field. Confusingly named in the
	// pre-fix code; the alias keeps API consumers from breaking.
	var worstForward, worstReturn *VoicePathMetrics
	for _, m := range forwardMetrics {
		m.Direction = VoicePathForward
		if worstForward == nil || m.MosScore < worstForward.MosScore {
			worstForward = m
		}
	}
	for _, m := range reverseMetrics {
		if worstReturn == nil || m.MosScore < worstReturn.MosScore {
			worstReturn = m
		}
	}

	// Per-probe baseline map. Old code used a single `baselineForward`
	// for every probe; that's wrong for agents with multiple targets
	// (each target has its own baseline). We fetch all of them and look
	// up by probe_id inside the detection helpers.
	baselineByProbeID := make(map[uint]*VoicePathMetrics)
	for _, p := range trafficSimProbes {
		bm, err := fetchVoicePathMetrics(ctx, ch, []uint{agentID}, p.ID, baselineFrom)
		if err == nil && bm != nil {
			baselineByProbeID[p.ID] = bm
		}
	}
	// Probes that fell back to PING-derived metrics (owned AGENT/PING
	// probes) aren't covered by the loop above — fetch their baselines
	// with the same TrafficSim-then-PING order used for the
	// current-window metrics.
	fillVoiceBaselines(ctx, ch, baselineByProbeID, forwardMetrics, baselineFrom)
	// Reverse baselines live in their own map: forward and reverse
	// share the probe ID on bidirectional probes, and comparing a
	// return path against the forward baseline would mask exactly the
	// asymmetric degradation the report exists to catch.
	// fillVoiceBaselines keys the fetch off each metric's
	// SourceAgentID, which for reverse metrics is the far-end reporter.
	reverseBaselineByProbeID := make(map[uint]*VoicePathMetrics)
	fillVoiceBaselines(ctx, ch, reverseBaselineByProbeID, reverseMetrics, baselineFrom)

	// Determine target agent name for issue detection
	targetName := ""
	if worstForward != nil {
		targetName = worstForward.TargetAgentName
	} else if worstReturn != nil {
		targetName = worstReturn.TargetAgentName
	}

	// Detect voice quality issues (now with per-probe baselines and
	// configurable thresholds).
	issues := detectVoiceQualityIssues(worstForward, worstReturn, baselineByProbeID, reverseBaselineByProbeID, targetName, &thresholds)

	// Per-probe / per-pair detection. Runs the same suite against every
	// probe in the agent's metric maps, so the multi-target voice report
	// can show issues per destination rather than collapsed onto the
	// worst-offender. Built off `forwardMetrics` (key = probe ID) and
	// `reverseMetrics` (also key = probe ID, post-refactor).
	//
	// buildTargetNameByProbeID batches the agent-name lookups into a
	// single SELECT (see helper) and pre-loads the result map so
	// detectVoiceQualityIssuesPerPair and buildVoicePairSummaries
	// can both reuse it without N+1 queries.
	// AGENT/PING probes participate too (the PING-fallback metrics are
	// keyed by their probe IDs); without them in the lookup tables the
	// fallback pairs silently drop out of buildVoicePairSummaries.
	voiceProbes := make([]Probe, 0, len(trafficSimProbes)+len(agentProbes))
	voiceProbes = append(voiceProbes, trafficSimProbes...)
	voiceProbes = append(voiceProbes, agentProbes...)
	targetByProbeID := buildTargetNameByProbeID(ctx, db, voiceProbes, agentObj.Name)
	// Shared agent-id→name map for buildVoicePairSummaries below
	// (resolveProbeTarget needs the same lookup table).
	pairNameByAgentID := buildVoicePairAgentNameMap(ctx, db, voiceProbes)
	perProbeIssues := detectVoiceQualityIssuesPerPair(forwardMetrics, reverseMetrics, baselineByProbeID, reverseBaselineByProbeID, targetByProbeID, &thresholds)

	// MTR hop correlation: fetch the MTR trace for the worst-offender
	// forward probe and tag issues with the most-degraded hop in the
	// same time window. Failure here is non-fatal — the report just
	// doesn't get the hop evidence line.
	mtrPath := worstForward
	mtrAgentID := agentID
	if mtrPath == nil && worstReturn != nil {
		// Target-only agents have no forward probe; correlate against
		// the remote agent's MTR trace toward us instead.
		mtrPath = worstReturn
		mtrAgentID = worstReturn.SourceAgentID
	}
	if mtrPath != nil {
		if hopSummaries := fetchMtrHopSummariesForVoice(ctx, ch, mtrAgentID, mtrPath, from); len(hopSummaries) > 0 {
			correlateWithRoute(issues, hopSummaries)
			for probeID, list := range perProbeIssues {
				correlateWithRoute(list, hopSummaries)
				perProbeIssues[probeID] = list
			}
		}
	}

	// Sample-weighted aggregate metrics (the "honest" average across
	// all probes, not just the worst one).
	allProbes := make([]*VoicePathMetrics, 0, len(forwardMetrics)+len(reverseMetrics))
	for _, m := range forwardMetrics {
		allProbes = append(allProbes, m)
	}
	for _, m := range reverseMetrics {
		allProbes = append(allProbes, m)
	}
	aggForward := aggregateVoicePathMetrics(forwardSlice(forwardMetrics), VoicePathForward)
	aggReturn := aggregateVoicePathMetrics(reverseSlice(reverseMetrics), VoicePathReturn)

	// Compute overall MOS as weighted average of the WORST-OFFENDER
	// paths (preserves existing behavior — the worst path is the
	// operator's signal of "is the worst case acceptable"). The
	// aggregate values are still surfaced separately for the report.
	var overallMos float64
	var totalWeight float64
	if worstForward != nil {
		overallMos += worstForward.MosScore * 1.0
		totalWeight += 1.0
	}
	if worstReturn != nil {
		overallMos += worstReturn.MosScore * 0.8 // Return path slightly less weight
		totalWeight += 0.8
	}
	if totalWeight > 0 {
		overallMos /= totalWeight
	} else {
		overallMos = 4.5 // Default to excellent if no data
	}

	// Compute scores from the worst-offender paths (kept consistent
	// with the existing overall MOS weighting so the report numbers
	// line up).
	var latencyScore, jitterScore, packetLossScore float64
	count := 0
	if worstForward != nil {
		latencyScore += scoreLatency(worstForward.AvgLatency, worstForward.P95Latency, worstForward.JitterAvg)
		jitterScore += jitterToScore(worstForward.JitterAvg)
		packetLossScore += scorePacketLoss(worstForward.PacketLoss)
		count++
	}
	if worstReturn != nil {
		latencyScore += scoreLatency(worstReturn.AvgLatency, worstReturn.P95Latency, worstReturn.JitterAvg)
		jitterScore += jitterToScore(worstReturn.JitterAvg)
		packetLossScore += scorePacketLoss(worstReturn.PacketLoss)
		count++
	}
	if count > 0 {
		latencyScore /= float64(count)
		jitterScore /= float64(count)
		packetLossScore /= float64(count)
	} else {
		latencyScore, jitterScore, packetLossScore = 100, 100, 100
	}

	// Time-of-day pattern from the per-bucket series (when available).
	// Falls back to the legacy pattern-from-incidents heuristic.
	timePattern := timePatternFromIncidents(issues)
	if patterns := fetchVoicePathSeriesPatterns(ctx, ch, agentID, trafficSimProbes, from, to); patterns != "" {
		timePattern = patterns
	}

	recommendation := buildVoiceQualityRecommendation(issues, worstForward, worstReturn)

	probeList := make([]VoicePathMetrics, 0, len(forwardMetrics))
	for _, m := range forwardMetrics {
		probeList = append(probeList, *m)
	}

	// Compute per-probe baseline deltas for the trend arrow.
	baselineComparison := computeBaselineComparison(allProbes, baselineByProbeID)

	// Build per-pair summaries (the multi-target report view). One
	// entry per forward probe, with the matching reverse probe (if
	// any) attached. Pair-level issues are taken from perProbeIssues;
	// for the typical single-pair case this is just one entry.
	pairs := buildVoicePairSummaries(forwardMetrics, reverseMetrics, perProbeIssues, baselineByProbeID, reverseBaselineByProbeID, voiceProbes, agentObj, agentID, thresholds, pairNameByAgentID)

	// Pull workspace-level incidents for context. We re-use the helper
	// from analysis.go that returns the full WorkspaceAnalysis and
	// filter down to incidents touching this agent.
	workspaceContext := buildWorkspaceIncidentContext(ctx, db, ch, agentID, agentObj.Name, from)

	// Time series for the MOS timeline chart. Reverse AGENT probes are
	// included so target-only agents get a Return series — the series
	// split classifies rows by reporting agent, so remote agents' rows
	// land in the Return direction either way.
	trendProbes := make([]Probe, 0, len(trafficSimProbes)+len(reverseAgentProbes))
	trendProbes = append(trendProbes, trafficSimProbes...)
	trendProbes = append(trendProbes, reverseAgentProbes...)
	trends := buildVoiceTrends(ctx, ch, agentID, trendProbes, from, to, thresholds)

	return &VoiceQualitySummary{
		AgentID:            agentID,
		AgentName:          agentObj.Name,
		OverallMos:         overallMos,
		OverallGrade:       voiceGradeFromMos(overallMos),
		LatencyScore:       latencyScore,
		JitterScore:        jitterScore,
		PacketLossScore:    packetLossScore,
		ForwardPath:        worstForward, // now correctly named: the worst-offender
		ReturnPath:         worstReturn,
		AggregateForward:   aggForward,
		AggregateReturn:    aggReturn,
		Probes:             probeList,
		Pairs:              pairs,
		Trends:             trends,
		BaselineComparison: baselineComparison,
		WorkspaceContext:   workspaceContext,
		Issues:             issues,
		TimePattern:        timePattern,
		Recommendation:     recommendation,
		ThresholdsUsed:     &thresholds,
		GeneratedAt:        time.Now().UTC(),
	}, nil
}

// buildTargetNameByProbeID resolves each forward probe's target to a
// human-readable name. Used by detectVoiceQualityIssuesPerPair to
// populate issue titles per destination.
//
// Implementation note: target agent name resolution is best-effort.
// When the target is an arbitrary SIP endpoint (no remote agent),
// the probe's own Target field (IP/host) is used. When both exist,
// the agent name wins for readability.
//
// One round-trip to the agents table per call (batched WHERE id IN
// query) rather than one per probe — agents with dozens of probes
// were hitting the legacy N+1 pattern in earlier revisions and
// saturating the GORM pool.
func buildTargetNameByProbeID(ctx context.Context, db *gorm.DB, probes []Probe, fallbackSourceName string) map[uint]string {
	out := make(map[uint]string, len(probes))
	if len(probes) == 0 {
		return out
	}

	// Collect the agent IDs we need to resolve. Skip the DB call
	// entirely when no probe points at a known agent (typical for
	// single-target or SIP-endpoint-only agents).
	want := make(map[uint]struct{}, len(probes))
	for _, p := range probes {
		if p.ID == 0 || p.Targets == nil || len(p.Targets) == 0 {
			continue
		}
		t := p.Targets[0]
		if t.AgentID != nil && *t.AgentID != 0 {
			want[*t.AgentID] = struct{}{}
		}
	}
	nameByAgentID := batchLoadAgentNames(ctx, db, want)

	for _, p := range probes {
		if p.ID == 0 {
			continue
		}
		if p.Targets == nil || len(p.Targets) == 0 {
			continue
		}
		t := p.Targets[0]
		name := ""
		if t.AgentID != nil && *t.AgentID != 0 {
			if n, ok := nameByAgentID[*t.AgentID]; ok {
				name = n
			}
		}
		if name == "" {
			name = t.Target
		}
		if name == "" {
			name = fallbackSourceName
		}
		out[p.ID] = name
	}
	return out
}

// batchLoadAgentNames fetches id→name for every agent ID in `want`
// in a single SELECT. Returns an empty map when `db` is nil or `want`
// is empty so callers don't have to nil-check.
//
// The function is intentionally tolerant of a nil DB — at the cost
// of returning no names. The voice report still renders, just with
// IP/host fallbacks in the target name field.
func batchLoadAgentNames(ctx context.Context, db *gorm.DB, want map[uint]struct{}) map[uint]string {
	out := make(map[uint]string, len(want))
	if db == nil || len(want) == 0 {
		return out
	}
	ids := make([]uint, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	// Stable order so the test suite (and any logs) see deterministic
	// IN-clause arguments.
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	type row struct {
		ID   uint
		Name string
	}
	var rows []row
	if err := db.WithContext(ctx).Table("agents").Select("id, name").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		log.Warnf("[voice] batchLoadAgentNames query failed: %v", err)
		return out
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out
}

// buildVoicePairAgentNameMap is a thin wrapper that walks `probes`,
// collects the agent IDs they reference, and batch-loads their
// names. Used to build the shared name-by-agent-id map that
// buildVoicePairSummaries + resolveProbeTarget share.
//
// Exposed as its own function (rather than inlined into
// buildVoicePairSummaries) so the test suite can hit the batched
// loader directly with a known probe set.
func buildVoicePairAgentNameMap(ctx context.Context, db *gorm.DB, probes []Probe) map[uint]string {
	want := make(map[uint]struct{}, len(probes))
	for _, p := range probes {
		if p.Targets == nil || len(p.Targets) == 0 {
			continue
		}
		t := p.Targets[0]
		if t.AgentID != nil && *t.AgentID != 0 {
			want[*t.AgentID] = struct{}{}
		}
	}
	return batchLoadAgentNames(ctx, db, want)
}

// buildVoicePairSummaries assembles the per-pair rollup for the
// multi-target voice report. One VoicePairSummary per forward probe;
// the matching reverse probe (same probe ID) is attached when present.
//
// Pair-level issues are the union of the per-probe detection runs
// (jitter/loss/latency/OOO + asymmetry). The overall MOS per pair is
// the worse of forward vs reverse, weighted slightly toward forward
// (consistent with the existing ComputeAgentVoiceQuality weighting).
func buildVoicePairSummaries(
	forwardMetrics map[uint]*VoicePathMetrics,
	reverseMetrics map[uint]*VoicePathMetrics,
	perProbeIssues map[uint][]VoiceQualityIssue,
	baselineByProbeID map[uint]*VoicePathMetrics,
	reverseBaselineByProbeID map[uint]*VoicePathMetrics,
	probes []Probe,
	sourceAgent *agent.Agent,
	sourceAgentID uint,
	thresholds VoiceThresholds,
	nameByAgentID map[uint]string,
) []VoicePairSummary {
	out := make([]VoicePairSummary, 0, len(forwardMetrics))

	// Index probes by ID for target lookup.
	probeByID := make(map[uint]Probe, len(probes))
	for _, p := range probes {
		if p.ID == 0 {
			continue
		}
		probeByID[p.ID] = p
	}

	for probeID, fwd := range forwardMetrics {
		if fwd == nil {
			continue
		}
		probe, ok := probeByID[probeID]
		if !ok {
			continue
		}

		pair := VoicePairSummary{
			ID:         fmt.Sprintf("pair-%d-%d", sourceAgentID, probeID),
			Forward:    fwd,
			Reverse:    reverseMetrics[probeID],
			Issues:     perProbeIssues[probeID],
			Thresholds: thresholds,
		}
		pair.Agent = AgentRef{
			ID:       sourceAgent.ID,
			Name:     sourceAgent.Name,
			IP:       sourceAgent.PublicIPOverride,
			Location: sourceAgent.Location,
		}
		// Resolve target
		pair.Target = resolveProbeTarget(probe, nameByAgentID, sourceAgent.Name)

		finalizeVoicePair(&pair, baselineByProbeID, reverseBaselineByProbeID)
		out = append(out, pair)
	}

	// Remote agents already represented by a forward pair — used to
	// avoid duplicating them as reverse-only pairs below (their return
	// direction is already attached to the forward pair via the
	// same-probe bidirectional rows, and the remote agent's own probe
	// shows up in that agent's report).
	coveredRemotes := make(map[uint]bool, len(forwardMetrics))
	for _, fwd := range forwardMetrics {
		if fwd != nil && fwd.TargetAgentID != 0 {
			coveredRemotes[fwd.TargetAgentID] = true
		}
	}

	// Reverse-only pairs: probes owned by a remote agent targeting this
	// agent, with no matching forward probe. For a target-only agent
	// (e.g. a fax server that never originates probes) these are the
	// only voice data — the path TO the agent is the agent's view.
	for probeID, rev := range reverseMetrics {
		if rev == nil {
			continue
		}
		if fwd, ok := forwardMetrics[probeID]; ok && fwd != nil {
			continue // already attached to a forward pair above
		}
		if rev.SourceAgentID != 0 && coveredRemotes[rev.SourceAgentID] {
			continue // remote agent already covered by a forward pair
		}
		pair := VoicePairSummary{
			ID:         fmt.Sprintf("pair-%d-%d", sourceAgentID, probeID),
			Reverse:    rev,
			Issues:     perProbeIssues[probeID],
			Thresholds: thresholds,
		}
		pair.Agent = AgentRef{
			ID:       sourceAgent.ID,
			Name:     sourceAgent.Name,
			IP:       sourceAgent.PublicIPOverride,
			Location: sourceAgent.Location,
		}
		// The remote end is the probe's owner — the agent measuring
		// the path toward us.
		pair.Target = TargetRef{
			Name:      rev.SourceAgentName,
			AgentID:   rev.SourceAgentID,
			AgentName: rev.SourceAgentName,
		}
		finalizeVoicePair(&pair, baselineByProbeID, reverseBaselineByProbeID)
		out = append(out, pair)
	}

	// Stable order: sort by descending MOS (worst first) so the report
	// shows the most degraded pair at the top.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].OverallMos < out[j].OverallMos
	})

	return out
}

// finalizeVoicePair computes the pair-level MOS/grade and the 7-day
// baseline delta. Pair-level MOS is the unweighted average of the
// directions that have samples (we keep the forward preference in the
// per-agent rollup; at the pair level it matters less). The baseline
// uses the forward path when available, falling back to reverse —
// each direction against its own baseline map, since forward and
// reverse share a probe ID on bidirectional probes.
func finalizeVoicePair(pair *VoicePairSummary, baselineByProbeID, reverseBaselineByProbeID map[uint]*VoicePathMetrics) {
	var mos, weight float64
	if pair.Forward != nil && pair.Forward.SampleCount > 0 {
		mos += pair.Forward.MosScore
		weight++
	}
	if pair.Reverse != nil && pair.Reverse.SampleCount > 0 {
		mos += pair.Reverse.MosScore
		weight++
	}
	if weight > 0 {
		pair.OverallMos = mos / weight
	}
	pair.OverallGrade = voiceGradeFromMos(pair.OverallMos)

	basePath := pair.Forward
	baselines := baselineByProbeID
	if basePath == nil {
		basePath = pair.Reverse
		baselines = reverseBaselineByProbeID
	}
	if basePath == nil {
		return
	}
	base, ok := baselines[basePath.ProbeID]
	if !ok || base == nil {
		return
	}
	pair.Baseline = &BaselineDelta{
		From:            time.Now().UTC().Add(-7 * 24 * time.Hour),
		To:              time.Now().UTC(),
		MosDelta:        basePath.MosScore - base.MosScore,
		LatencyDeltaMs:  basePath.AvgLatency - base.AvgLatency,
		JitterDeltaMs:   basePath.JitterAvg - base.JitterAvg,
		LossDeltaPct:    basePath.PacketLoss - base.PacketLoss,
		SampleCount:     basePath.SampleCount,
		BaselineSamples: base.SampleCount,
	}
	switch {
	case pair.Baseline.MosDelta > 0.1:
		pair.Baseline.Trend = "improving"
	case pair.Baseline.MosDelta < -0.1:
		pair.Baseline.Trend = "worsening"
	default:
		pair.Baseline.Trend = "stable"
	}
}

// resolveProbeTarget converts a probe's target into a TargetRef. When
// the target is a known netwatcher agent, both host and agent info
// are populated; otherwise only the SIP endpoint (host/IP/port).
//
// `nameByAgentID` is the pre-loaded agent ID → name map produced by
// buildTargetNameByProbeID's batched query, shared across all
// pairs in the same call to avoid N+1 lookups.
func resolveProbeTarget(p Probe, nameByAgentID map[uint]string, fallback string) TargetRef {
	out := TargetRef{}
	if p.Targets == nil || len(p.Targets) == 0 {
		out.Name = fallback
		return out
	}
	t := p.Targets[0]
	out.Host = t.Target
	if t.AgentID != nil && *t.AgentID != 0 {
		out.AgentID = *t.AgentID
		if n, ok := nameByAgentID[*t.AgentID]; ok {
			out.AgentName = n
			if out.Name == "" {
				out.Name = n
			}
		}
	}
	if out.Name == "" {
		if t.Target != "" {
			out.Name = t.Target
		} else {
			out.Name = fallback
		}
	}
	return out
}

// forwardSlice / reverseSlice are tiny helpers to drop the map keys
// when passing to aggregateVoicePathMetrics.
func forwardSlice(m map[uint]*VoicePathMetrics) []*VoicePathMetrics {
	out := make([]*VoicePathMetrics, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func reverseSlice(m map[uint]*VoicePathMetrics) []*VoicePathMetrics {
	return forwardSlice(m)
}

// aggregateVoicePathMetrics produces a sample-weighted aggregate
// VoicePathMetrics from a set of probes in a given direction. Latency,
// jitter, and loss are weighted by sample count; p95 metrics are the
// max (the worst P95 across probes represents the worst-case p95 the
// user can experience).
func aggregateVoicePathMetrics(probes []*VoicePathMetrics, dir VoicePathDirection) *VoicePathMetrics {
	if len(probes) == 0 {
		return nil
	}
	var (
		totalSamples                              int
		weightedMos, weightedLat                  float64
		weightedJitter, weightedLoss, weightedOOO float64
		weightedDup, weightedJitterP95            float64
		p95Lat, jitterP95                         float64
		mosContribs                               []string
	)
	for _, p := range probes {
		if p == nil || p.SampleCount == 0 {
			continue
		}
		w := float64(p.SampleCount)
		totalSamples += p.SampleCount
		weightedMos += p.MosScore * w
		weightedLat += p.AvgLatency * w
		weightedJitter += p.JitterAvg * w
		weightedJitterP95 += p.JitterP95 * w
		weightedLoss += p.PacketLoss * w
		weightedOOO += p.OutOfSequence * w
		weightedDup += p.Duplicates * w
		if p.P95Latency > p95Lat {
			p95Lat = p.P95Latency
		}
		if p.JitterP95 > jitterP95 {
			jitterP95 = p.JitterP95
		}
		mosContribs = append(mosContribs, p.MosContributors...)
	}
	if totalSamples == 0 {
		return nil
	}
	w := float64(totalSamples)
	out := &VoicePathMetrics{
		Direction:       dir,
		SampleCount:     totalSamples,
		MosScore:        weightedMos / w,
		AvgLatency:      weightedLat / w,
		P95Latency:      p95Lat,
		JitterAvg:       weightedJitter / w,
		JitterP95:       jitterP95,
		PacketLoss:      weightedLoss / w,
		OutOfSequence:   weightedOOO / w,
		Duplicates:      weightedDup / w,
		MosContributors: dedupeStrings(mosContribs),
	}
	out.MedianLatency = out.AvgLatency // close enough for an aggregate label
	out.CongestionLevel = congestionLevelFromMetrics(out.JitterAvg, out.PacketLoss, out.AvgLatency)
	return out
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// computeBaselineComparison rolls up per-probe baseline deltas into a
// single BaselineDelta for the executive summary. Returns nil when no
// baseline samples were found.
func computeBaselineComparison(probes []*VoicePathMetrics, baselineByProbeID map[uint]*VoicePathMetrics) *BaselineDelta {
	var (
		totalSamples, baselineSamples int
		mosDeltaSum, latDeltaSum      float64
		jitDeltaSum, lossDeltaSum     float64
		mosDeltaCount                 int
	)
	for _, p := range probes {
		if p == nil {
			continue
		}
		totalSamples += p.SampleCount
		bm := baselineByProbeID[p.ProbeID]
		if bm == nil {
			continue
		}
		baselineSamples += bm.SampleCount
		mosDeltaSum += p.MosScore - bm.MosScore
		latDeltaSum += p.AvgLatency - bm.AvgLatency
		jitDeltaSum += p.JitterAvg - bm.JitterAvg
		lossDeltaSum += p.PacketLoss - bm.PacketLoss
		mosDeltaCount++
	}
	if baselineSamples == 0 {
		return nil
	}
	d := &BaselineDelta{
		From:            time.Now().UTC().Add(-7 * 24 * time.Hour),
		To:              time.Now().UTC(),
		SampleCount:     totalSamples,
		BaselineSamples: baselineSamples,
	}
	if mosDeltaCount > 0 {
		d.MosDelta = mosDeltaSum / float64(mosDeltaCount)
		d.LatencyDeltaMs = latDeltaSum / float64(mosDeltaCount)
		d.JitterDeltaMs = jitDeltaSum / float64(mosDeltaCount)
		d.LossDeltaPct = lossDeltaSum / float64(mosDeltaCount)
	}
	switch {
	case d.MosDelta > 0.1:
		d.Trend = "improving"
	case d.MosDelta < -0.1:
		d.Trend = "worsening"
	default:
		d.Trend = "stable"
	}
	if d.MosDelta > 0 {
		d.PercentChange = d.MosDelta * 100
	} else {
		d.PercentChange = d.MosDelta * 100
	}
	return d
}
