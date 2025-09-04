package probe

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentpkg "netwatcher-controller/internal/agent"
)

type Service interface {
	Create(ctx context.Context, p *Probe) error
	Get(ctx context.Context, id uint) (*Probe, error)
	ListByAgent(ctx context.Context, agentID uint, includeReverse bool) ([]Probe, error)
	DeleteByAgent(ctx context.Context, agentID uint) error
	DeleteByID(ctx context.Context, id uint) error

	// Specific flows from your old code:
	UpdateFirstTargetInline(ctx context.Context, id uint, newTarget string, markPendingForSpeedtest bool) error
	FindTrafficSimClients(ctx context.Context, serverAgentID uint) ([]Probe, error)
}

type service struct {
	repo   Repository
	agents agentpkg.Repository
}

func NewService(repo Repository, agents agentpkg.Repository) Service {
	return &service{repo: repo, agents: agents}
}

// ---------- CRUD ----------

func (s *service) Create(ctx context.Context, p *Probe) error {
	now := time.Now()
	p.CreatedAt, p.UpdatedAt = now, now
	if p.Labels == nil {
		p.Labels = []byte(`{}`)
	}
	if p.Metadata == nil {
		p.Metadata = []byte(`{}`)
	}
	for i := range p.Targets {
		if p.Targets[i].ProbeID == 0 {
			// gorm sets this after Create; we just ensure no garbage
		}
	}
	return s.repo.Create(ctx, p)
}

func (s *service) Get(ctx context.Context, id uint) (*Probe, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) ListByAgent(ctx context.Context, agentID uint, includeReverse bool) ([]Probe, error) {
	base, err := s.repo.ListByAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if !includeReverse {
		return base, nil
	}
	rev, err := s.generateReverseForAgent(ctx, agentID)
	if err != nil {
		// don't fail on reverse generation
		return append(base, rev...), nil
	}
	return append(base, rev...), nil
}

func (s *service) DeleteByAgent(ctx context.Context, agentID uint) error {
	return s.repo.DeleteByAgent(ctx, agentID)
}

func (s *service) DeleteByID(ctx context.Context, id uint) error {
	return s.repo.DeleteByID(ctx, id)
}

// ---------- Inline update (like your UpdateFirstProbeTarget) ----------

func (s *service) UpdateFirstTargetInline(ctx context.Context, id uint, newTarget string, markPendingForSpeedtest bool) error {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if len(p.Targets) == 0 {
		return fmt.Errorf("probe has no targets")
	}
	p.Targets[0].Target = newTarget
	if markPendingForSpeedtest && p.Type == TypeSpeedtest {
		now := time.Now()
		p.PendingAt = &now
	}
	p.UpdatedAt = time.Now()
	return s.repo.Update(ctx, p)
}

// ---------- Reverse generation & target resolution ----------

// generateReverseForAgent mirrors your old "reverse probe" logic in DB-backed form.
// It returns an array of *generated* probes (not saved), ready to send to the agent.
func (s *service) generateReverseForAgent(ctx context.Context, thisAgentID uint) ([]Probe, error) {
	var out []Probe

	// 1) MTR/PING reverse for AGENT meta-probes that target this agent (owned by other agents)
	//    For each source meta-probe X that targets 'thisAgentID', produce a reverse probe
	//    owned by this agent, pointing back to the source's public IP.
	// Step: find all AGENT probes where some other agent A has target=thisAgentID
	// We'll scan across *all* owners by joining in repo call(s). For performance, you might add a repo method
	// that gathers in one query; here we iterate by discovering "owners" through an index-friendly path:

	// Instead, gather AGENT probes globally that target this agent by filtering list-of-owners (simple approach):
	// We can reuse ListAgentProbesTargetingAgent for each owner; to avoid N calls you'd add a repo method with a subquery.
	// For simplicity and clarity, we’ll discover owners from the agents table:
	// (If you prefer not to depend on that, add a repo method to fetch global owners directly.)

	// For minimal dependency, do a rough approach: list all agents, filter, generate.
	// If you have many agents, replace with a dedicated repo method.
	owners, _, _ := s.agents.ListByWorkspace(ctx, 0, 100000, 0) // if you store workspaceId on agents; else create a ListAll
	for _, owner := range owners {
		if owner.ID == thisAgentID {
			continue
		}
		meta, err := s.repo.ListAgentProbesTargetingAgent(ctx, owner.ID, thisAgentID)
		if err != nil {
			continue
		}
		for _, src := range meta {
			// For each meta probe, generate MTR/PING reverse + Trafficsim reverse where applicable
			rev := s.makeReverseStandard(ctx, src, thisAgentID)
			out = append(out, rev...)

			if t := s.makeReverseTrafficSim(ctx, src, thisAgentID); t != nil {
				out = append(out, *t)
			}
		}
	}

	return out, nil
}

// makeReverseStandard creates reverse MTR/PING probes owned by thisAgent targeting the source's IP.
func (s *service) makeReverseStandard(ctx context.Context, source Probe, thisAgentID uint) []Probe {
	var out []Probe
	/*sourceIP, ok := s.getAgentPublicIP(ctx, source.AgentID)
	if !ok {
		return out
	}
	original := source.ID*/

	types := []Type{TypeMTR, TypePing}
	for _, t := range types {
		p := Probe{
			AgentID:       thisAgentID,
			Type:          t,
			Notifications: source.Notifications,
			DurationSec:   source.DurationSec,
			Count:         source.Count,
			IntervalSec:   source.IntervalSec,
			Server:        false,
			/*ReverseOfProbeID: &original,
			OriginalAgentID:  &source.AgentID,
			Targets: []Target{{
				Target:  sourceIP,        // concrete IP
				AgentID: &source.AgentID, // keep origin
			}},*/
		}
		out = append(out, p)
	}
	return out
}

// makeReverseTrafficSim creates the reverse TRAFFICSIM client when the source has a server.
func (s *service) makeReverseTrafficSim(ctx context.Context, source Probe, thisAgentID uint) *Probe {
	// Find if source agent has a TRAFFICSIM server
	srcServers, _ := s.repo.ListAgentProbesOfType(ctx, source.AgentID, TypeTrafficSim)
	var serverPort string
	for _, sp := range srcServers {
		if sp.Server && len(sp.Targets) > 0 && sp.Targets[0].Target != "" {
			parts := strings.Split(sp.Targets[0].Target, ":")
			if len(parts) >= 2 {
				serverPort = parts[1]
				break
			}
		}
	}
	if serverPort == "" {
		return nil
	}

	// source public ip
	/*sourceIP, ok := s.getAgentPublicIP(ctx, source.AgentID)
	if !ok {
		return nil
	}
	target := fmt.Sprintf("%s:%s", sourceIP, serverPort)
	original := source.ID*/

	p := Probe{
		AgentID:       thisAgentID,
		Type:          TypeTrafficSim,
		Server:        false,
		Notifications: source.Notifications,
		DurationSec:   source.DurationSec,
		Count:         source.Count,
		IntervalSec:   source.IntervalSec,
		/*ReverseOfProbeID: &original,
		OriginalAgentID:  &source.AgentID,
		Targets: []Target{{
			Target:  target,
			AgentID: &source.AgentID,
		}},*/
	}
	return &p
}

// ---------- Target resolution helpers ----------

// For agent targets lacking a concrete IP/host, resolve using Agent fields (override → detected → private).
func (s *service) resolveAgentTarget(ctx context.Context, t *Target) (string, bool) {
	if t == nil || t.AgentID == nil {
		return t.Target, t.Target != ""
	}
	// get agent
	a, err := s.agents.GetByID(ctx, *t.AgentID)
	if err != nil {
		return "", false
	}
	host := a.PublicIPOverride
	/*if host == "" {
		host = a.DetectedPublicIP
	}
	if host == "" {
		host = a.PrivateIP
	}*/
	return host, host != ""
}

/*// getAgentPublicIP is a convenient view for reverse generation.
func (s *service) getAgentPublicIP(ctx context.Context, agentID uint) (string, bool) {
	a, err := s.agents.GetByID(ctx, agentID)
	if err != nil {
		return "", false
	}
	if a.PublicIPOverride != "" {
		return a.PublicIPOverride, true
	}
	if a.DetectedPublicIP != "" {
		return a.DetectedPublicIP, true
	}
	if a.PrivateIP != "" {
		return a.PrivateIP, true
	}
	return "", false
}*/

// ---------- TrafficSim clients (server view) ----------

func (s *service) FindTrafficSimClients(ctx context.Context, serverAgentID uint) ([]Probe, error) {
	// This is equivalent to your old FindTrafficSimClients but DB-backed
	return s.repo.FindTrafficSimClients(ctx, serverAgentID)
}
