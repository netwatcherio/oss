package probe

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"netwatcher-controller/internal/agent"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ComputePerAgentAnalysis builds the full agent detail: a real
// per-probe analysis (bidirectional, via ComputeProbeAnalysis) for
// every voice/path probe the agent owns, plus return-path analyses
// for other agents' AGENT probes targeting it, plus the voice
// quality summary. The agent health score is the average of the
// per-probe combined (forward+reverse) health across BOTH owned and
// return-path probes — the same all-probes-both-directions policy
// the voice report uses.
func ComputePerAgentAnalysis(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint, lookbackMinutes int) (*AgentAnalysis, error) {
	agentObj, err := agent.GetAgentByID(ctx, db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent %d: %w", agentID, err)
	}
	if lookbackMinutes <= 0 {
		lookbackMinutes = 60
	}
	from := time.Now().UTC().Add(-time.Duration(lookbackMinutes) * time.Minute)

	// Compute voice quality (all probes + reverse paths).
	vq, err := ComputeAgentVoiceQuality(ctx, db, ch, agentID, from, time.Now().UTC())
	if err != nil {
		log.Warnf("[analysis] failed to compute voice quality for agent %d: %v", agentID, err)
	}

	// Owned probes — raw DB rows, not the expanded runtime list:
	// expansions share the AGENT probe's ID and ComputeProbeAnalysis
	// already merges siblings and both directions per probe.
	var ownedProbes []Probe
	if err := db.WithContext(ctx).
		Preload("Targets", "deleted_at IS NULL").
		Where("agent_id = ? AND enabled = ? AND deleted_at IS NULL", agentID, true).
		Order("id").
		Find(&ownedProbes).Error; err != nil {
		return nil, fmt.Errorf("failed to list probes for agent %d: %w", agentID, err)
	}

	analyzable := map[Type]bool{TypeAgent: true, TypePing: true, TypeMTR: true, TypeTrafficSim: true}
	ownedTargets := make(map[uint]bool)
	var probeAnalyses []ProbeAnalysis
	var healthScores []float64

	collectHealth := func(pa *ProbeAnalysis) {
		h := pa.Health
		if pa.CombinedHealth != nil {
			h = *pa.CombinedHealth
		}
		if pa.Metrics.SampleCount > 0 || pa.CombinedHealth != nil {
			healthScores = append(healthScores, h.OverallHealth)
		}
	}

	for _, p := range ownedProbes {
		for _, t := range p.Targets {
			if t.AgentID != nil {
				ownedTargets[*t.AgentID] = true
			}
		}
		if !analyzable[p.Type] {
			continue
		}
		pa, err := ComputeProbeAnalysis(ctx, ch, db, p.WorkspaceID, p.ID, lookbackMinutes)
		if err != nil || pa == nil {
			continue
		}
		probeAnalyses = append(probeAnalyses, *pa)
		collectHealth(pa)
	}

	// Return-path probes: other agents' AGENT probes targeting this
	// agent — the inbound half of the agent's health, and the only
	// half for target-only agents. Skip remotes already covered by an
	// owned probe: ComputeProbeAnalysis pulled their rows in as that
	// probe's reverse direction, and counting them again would
	// double-weight the pair.
	var returnAnalyses []ProbeAnalysis
	if reverseProbes, rerr := FindReverseAgentProbes(ctx, db, agentID); rerr == nil {
		for _, rp := range reverseProbes {
			if ownedTargets[rp.AgentID] {
				continue
			}
			pa, err := ComputeProbeAnalysis(ctx, ch, db, rp.WorkspaceID, rp.ID, lookbackMinutes)
			if err != nil || pa == nil {
				continue
			}
			returnAnalyses = append(returnAnalyses, *pa)
			collectHealth(pa)
		}
	} else {
		log.Warnf("[analysis] failed to find reverse probes for agent %d: %v", agentID, rerr)
	}

	// Check if online
	isOnline := agentObj.LastSeenAt.After(time.Now().UTC().Add(-5 * time.Minute))

	// Agent health: per-probe combined health first; voice scores
	// enrich the vector rather than define it. Falls back to the
	// voice-derived score when probes produced no analyzable samples,
	// and to "unknown" when there's no data at all.
	var health HealthVector
	switch {
	case len(healthScores) > 0:
		overall := clampScore(avg(healthScores))
		health = HealthVector{
			OverallHealth:  overall,
			Grade:          gradeFromScore(overall),
			RouteStability: 100,
			MosScore:       1.0,
		}
		if vq != nil {
			health.LatencyScore = vq.LatencyScore
			health.PacketLossScore = vq.PacketLossScore
			health.MosScore = vq.OverallMos
		}
	case vq != nil && len(vq.Pairs) > 0:
		health = HealthVector{
			OverallHealth:   (vq.LatencyScore + vq.JitterScore + vq.PacketLossScore) / 3,
			LatencyScore:    vq.LatencyScore,
			PacketLossScore: vq.PacketLossScore,
			MosScore:        vq.OverallMos,
			Grade:           vq.OverallGrade,
		}
	default:
		health = HealthVector{Grade: "unknown", RouteStability: 100, MosScore: 1.0}
	}

	log.Infof("[analysis] agent %d (%s): %d owned probes analyzed, %d return-path, %d health samples → %.1f (%s)",
		agentID, agentObj.Name, len(probeAnalyses), len(returnAnalyses), len(healthScores), health.OverallHealth, health.Grade)

	return &AgentAnalysis{
		AgentID:          agentID,
		AgentName:        agentObj.Name,
		IsOnline:         isOnline,
		Health:           health,
		VoiceQuality:     vq,
		Probes:           probeAnalyses,
		ReturnPathProbes: returnAnalyses,
		Incidents:        nil,
		GeneratedAt:      time.Now().UTC(),
	}, nil
}
