package probe

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"netwatcher-controller/internal/agent"
)

// Offline integration tests for probe creation and the dynamic per-agent
// expansion (ListForAgent). ClickHouse is never touched: every test agent has
// PublicIPOverride set, which getPublicIP uses before falling back to NETINFO.

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	// A single connection keeps the in-memory database alive and shared.
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&agent.Agent{}, &Probe{}, &Target{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedAgent(t *testing.T, db *gorm.DB, id uint, ip string, tsEnabled bool, tsPort int) {
	t.Helper()
	a := agent.Agent{
		ID:                id,
		WorkspaceID:       1,
		Name:              "test-agent",
		PublicIPOverride:  ip,
		TrafficSimEnabled: tsEnabled,
		TrafficSimPort:    tsPort,
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("seed agent %d: %v", id, err)
	}
}

func bidirAgentMetadata(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"trafficsim": map[string]any{
			"voip_mode":     true,
			"interval_ms":   float64(20),
			"dscp":          float64(46),
			"bidirectional": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func countAgentProbes(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&Probe{}).Where("type = ?", TypeAgent).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// NEW single-probe mode: bidirectional in metadata → no persistent reverse probe.
func TestCreateNewFormatSkipsReverseProbe(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "10.0.0.1", false, 0)
	seedAgent(t, db, 2, "10.0.0.2", true, 5005)

	p, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:   1,
		AgentID:       1,
		Type:          TypeAgent,
		Enabled:       true,
		AgentTargets:  []uint{2},
		Bidirectional: true,
		Metadata:      bidirAgentMetadata(t),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.AgentID != 1 {
		t.Errorf("probe owner = %d, want 1", p.AgentID)
	}
	if got := countAgentProbes(t, db); got != 1 {
		t.Errorf("AGENT probes in DB = %d, want 1 (new format must not create a reverse probe)", got)
	}
}

// AGENT probes never create a persistent reverse probe anymore: a caller using
// only the legacy top-level flag gets it normalized into metadata so the return
// path is generated dynamically instead.
func TestCreateAgentLegacyFlagNormalizedToMetadata(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "10.0.0.1", false, 0)
	seedAgent(t, db, 2, "10.0.0.2", true, 5005)

	p, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:   1,
		AgentID:       1,
		Type:          TypeAgent,
		Enabled:       true,
		AgentTargets:  []uint{2},
		Bidirectional: true, // legacy DTO flag only, no metadata
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got := countAgentProbes(t, db); got != 1 {
		t.Fatalf("AGENT probes in DB = %d, want 1 (AGENT type must never create a reverse probe)", got)
	}
	if !agentProbeHasBidirectional(p) {
		t.Error("legacy bidirectional flag was not normalized into metadata")
	}
}

// Non-AGENT types keep the legacy dual-probe behavior (no dynamic reverse
// generation exists for them).
func TestCreateNonAgentLegacyStillCreatesReverseProbe(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "10.0.0.1", false, 0)
	seedAgent(t, db, 2, "10.0.0.2", true, 5005)

	if _, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:   1,
		AgentID:       1,
		Type:          TypePing,
		Enabled:       true,
		AgentTargets:  []uint{2},
		Bidirectional: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var n int64
	if err := db.Model(&Probe{}).Where("type = ?", TypePing).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("PING probes in DB = %d, want 2 (legacy dual-probe for non-AGENT types)", n)
	}
	var reverse Probe
	if err := db.Preload("Targets").Where("type = ? AND agent_id = ?", TypePing, 2).First(&reverse).Error; err != nil {
		t.Fatalf("reverse probe not found: %v", err)
	}
	if len(reverse.Targets) != 1 || reverse.Targets[0].AgentID == nil || *reverse.Targets[0].AgentID != 1 {
		t.Errorf("reverse probe must target agent 1, got %+v", reverse.Targets)
	}
}

// filterByType collects expanded probes of one type, skipping virtual defaults
// (NETINFO, SYSINFO, SPEEDTEST...) that ListForAgent always appends.
func filterByType(list []Probe, typ Type) []Probe {
	var out []Probe
	for _, p := range list {
		if p.Type == typ {
			out = append(out, p)
		}
	}
	return out
}

// Full new-format expansion: agent A (client) has one AGENT probe targeting
// agent B (TrafficSim server). Validates everything both agents receive.
func TestListForAgentNewFormatBidirectional(t *testing.T) {
	t.Setenv("AGENT_EXPANSION_INCLUDE_PING", "true")
	db := newTestDB(t)
	const clientID, serverID = uint(1), uint(2)
	seedAgent(t, db, clientID, "10.0.0.1", false, 0)
	seedAgent(t, db, serverID, "10.0.0.2", true, 5005)

	created, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:   1,
		AgentID:       clientID,
		Type:          TypeAgent,
		Enabled:       true,
		AgentTargets:  []uint{serverID},
		Bidirectional: true,
		Metadata:      bidirAgentMetadata(t),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ctx := context.Background()

	// ---- Agent A (client side) ----
	clientList, err := ListForAgent(ctx, db, nil, clientID)
	if err != nil {
		t.Fatalf("ListForAgent(client): %v", err)
	}

	for _, typ := range []Type{TypeMTR, TypePing} {
		probes := filterByType(clientList, typ)
		if len(probes) != 1 {
			t.Fatalf("client got %d %s probes, want exactly 1 (no pair/self probes)", len(probes), typ)
		}
		p := probes[0]
		if p.ID != created.ID || p.AgentID != clientID {
			t.Errorf("%s probe ID/owner = %d/%d, want %d/%d", typ, p.ID, p.AgentID, created.ID, clientID)
		}
		if p.Targets[0].Target != "10.0.0.2" {
			t.Errorf("%s target = %q, want server IP (never own IP)", typ, p.Targets[0].Target)
		}
	}

	tsProbes := filterByType(clientList, TypeTrafficSim)
	if len(tsProbes) != 1 {
		t.Fatalf("client got %d TRAFFICSIM probes, want 1", len(tsProbes))
	}
	ts := tsProbes[0]
	if ts.Server {
		t.Error("client TRAFFICSIM probe must not be a server probe")
	}
	if ts.ID != created.ID {
		t.Errorf("TRAFFICSIM probe ID = %d, want %d (attribution)", ts.ID, created.ID)
	}
	if ts.Targets[0].Target != "10.0.0.2:5005" {
		t.Errorf("TRAFFICSIM target = %q, want 10.0.0.2:5005", ts.Targets[0].Target)
	}
	// Regression: Target.AgentID = owner made the agent's self-target guard skip
	// the probe entirely and misattributed reported data.
	if ts.Targets[0].AgentID == nil || *ts.Targets[0].AgentID != serverID {
		t.Errorf("TRAFFICSIM Target.AgentID = %v, want %d (the target, never the owner)", ts.Targets[0].AgentID, serverID)
	}
	if !agentProbeHasBidirectional(&ts) {
		t.Error("client TRAFFICSIM probe missing bidirectional metadata flag")
	}

	// Nothing in A's list may belong to another agent (pair-probe leakage).
	for _, p := range clientList {
		if p.AgentID != clientID {
			t.Errorf("client list contains probe owned by agent %d: %s %d", p.AgentID, p.Type, p.ID)
		}
	}

	// ---- Agent B (server side) ----
	serverList, err := ListForAgent(ctx, db, nil, serverID)
	if err != nil {
		t.Fatalf("ListForAgent(server): %v", err)
	}

	// Return-path MTR/PING: same probe ID, owned by B, targeting A's IP.
	for _, typ := range []Type{TypeMTR, TypePing} {
		probes := filterByType(serverList, typ)
		if len(probes) != 1 {
			t.Fatalf("server got %d %s return probes, want exactly 1 (no duplicates)", len(probes), typ)
		}
		p := probes[0]
		if p.ID != created.ID || p.AgentID != serverID {
			t.Errorf("return %s probe ID/owner = %d/%d, want %d/%d", typ, p.ID, p.AgentID, created.ID, serverID)
		}
		if p.Targets[0].Target != "10.0.0.1" {
			t.Errorf("return %s target = %q, want client IP 10.0.0.1", typ, p.Targets[0].Target)
		}
	}

	tsServer := filterByType(serverList, TypeTrafficSim)
	var bidirServer, genericServer *Probe
	for i := range tsServer {
		p := &tsServer[i]
		if !p.Server {
			t.Errorf("server received a TRAFFICSIM client probe (A has no server): %+v", p)
			continue
		}
		var md map[string]any
		_ = json.Unmarshal(p.Metadata, &md)
		if tsmd, ok := md["trafficsim"].(map[string]any); ok && tsmd["bidirectional_server"] == true {
			bidirServer = p
		} else {
			genericServer = p
		}
	}
	if genericServer == nil {
		t.Fatal("generic TRAFFICSIM server probe missing")
	}
	if bidirServer == nil {
		t.Fatal("per-client bidirectional TRAFFICSIM server probe missing")
	}

	// The bidir server probe must reference the client's probe and agent and
	// inherit the client's VoIP settings for reverse traffic.
	var md map[string]any
	if err := json.Unmarshal(bidirServer.Metadata, &md); err != nil {
		t.Fatalf("bidir server metadata: %v", err)
	}
	tsmd := md["trafficsim"].(map[string]any)
	if tsmd["client_probe_id"] != float64(created.ID) {
		t.Errorf("client_probe_id = %v, want %d", tsmd["client_probe_id"], created.ID)
	}
	if tsmd["client_agent_id"] != float64(clientID) {
		t.Errorf("client_agent_id = %v, want %d", tsmd["client_agent_id"], clientID)
	}
	if tsmd["voip_mode"] != true || tsmd["interval_ms"] != float64(20) {
		t.Errorf("client VoIP settings not inherited by bidir server probe: %v", tsmd)
	}
	if md["bidirectional"] != true {
		t.Error("bidir server probe missing top-level bidirectional flag")
	}

	// Generic server probe carries the allowed-agents list (targets 1+).
	foundAllowed := false
	for _, tgt := range genericServer.Targets[1:] {
		if tgt.AgentID != nil && *tgt.AgentID == clientID {
			foundAllowed = true
		}
	}
	if !foundAllowed {
		t.Errorf("client agent %d missing from generic server probe allowed list: %+v", clientID, genericServer.Targets)
	}
}

// Expansion of PRE-EXISTING legacy dual AGENT probes (created by older
// controller versions, no metadata flag anywhere) must keep working. The rows
// are seeded directly since Create() no longer produces dual AGENT probes.
func TestListForAgentLegacyDualProbes(t *testing.T) {
	t.Setenv("AGENT_EXPANSION_INCLUDE_PING", "true")
	db := newTestDB(t)
	const aID, bID = uint(1), uint(2)
	seedAgent(t, db, aID, "10.0.0.1", false, 0)
	seedAgent(t, db, bID, "10.0.0.2", true, 5005)

	mkLegacyAgentProbe := func(owner, target uint) *Probe {
		p := &Probe{
			WorkspaceID: 1,
			AgentID:     owner,
			Type:        TypeAgent,
			Enabled:     true,
			IntervalSec: 60,
			TimeoutSec:  10,
			Labels:      datatypes.JSON([]byte(`{}`)),
			Metadata:    datatypes.JSON([]byte(`{}`)),
		}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("seed legacy probe: %v", err)
		}
		tgt := Target{ProbeID: p.ID, AgentID: &target}
		if err := db.Create(&tgt).Error; err != nil {
			t.Fatalf("seed legacy target: %v", err)
		}
		return p
	}

	created := mkLegacyAgentProbe(aID, bID) // forward A→B
	mkLegacyAgentProbe(bID, aID)            // legacy reverse B→A

	ctx := context.Background()

	aList, err := ListForAgent(ctx, db, nil, aID)
	if err != nil {
		t.Fatalf("ListForAgent(A): %v", err)
	}
	aTS := filterByType(aList, TypeTrafficSim)
	if len(aTS) != 1 {
		t.Fatalf("A got %d TRAFFICSIM probes, want 1 (forward only; A has no server so no :bidir marker)", len(aTS))
	}
	if aTS[0].ID != created.ID || aTS[0].Targets[0].Target != "10.0.0.2:5005" {
		t.Errorf("legacy forward TS probe wrong: ID=%d target=%q", aTS[0].ID, aTS[0].Targets[0].Target)
	}

	bList, err := ListForAgent(ctx, db, nil, bID)
	if err != nil {
		t.Fatalf("ListForAgent(B): %v", err)
	}
	// B owns its reverse AGENT probe → return MTR/PING from its own expansion.
	if got := len(filterByType(bList, TypeMTR)); got != 1 {
		t.Errorf("B got %d MTR probes, want 1", got)
	}
	if got := len(filterByType(bList, TypePing)); got != 1 {
		t.Errorf("B got %d PING probes, want 1", got)
	}
	// No bidirectional metadata anywhere → only the generic server probe.
	for _, p := range filterByType(bList, TypeTrafficSim) {
		if !p.Server {
			t.Errorf("B got a TS client probe but A has no server: %+v", p)
			continue
		}
		var md map[string]any
		_ = json.Unmarshal(p.Metadata, &md)
		if tsmd, ok := md["trafficsim"].(map[string]any); ok && tsmd["bidirectional_server"] == true {
			t.Error("legacy mode must not generate per-client bidirectional server probes")
		}
	}
}

// PING is opt-in. With both the env and the metadata unset, the default is
// off and no PING probe should be produced from the AGENT expansion. MTR and
// TRAFFICSIM (when the target has a server) must still be produced.
func TestListForAgentPingDefaultOff(t *testing.T) {
	t.Setenv("AGENT_EXPANSION_INCLUDE_PING", "")
	db := newTestDB(t)
	const aID, bID = uint(1), uint(2)
	seedAgent(t, db, aID, "10.0.0.1", false, 0)
	seedAgent(t, db, bID, "10.0.0.2", true, 5005)

	if _, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      aID,
		Type:         TypeAgent,
		Enabled:      true,
		AgentTargets: []uint{bID},
		// No Metadata — defaults to no PING expansion.
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := ListForAgent(context.Background(), db, nil, aID)
	if err != nil {
		t.Fatalf("ListForAgent: %v", err)
	}

	if got := len(filterByType(list, TypeMTR)); got != 1 {
		t.Errorf("MTR probes = %d, want 1 (MTR is always produced)", got)
	}
	if got := len(filterByType(list, TypePing)); got != 0 {
		t.Errorf("PING probes = %d, want 0 (default off, no env, no metadata)", got)
	}
	if got := len(filterByType(list, TypeTrafficSim)); got != 1 {
		t.Errorf("TRAFFICSIM probes = %d, want 1 (target has a server)", got)
	}
}

// Per-probe metadata override beats the env default: env off, metadata true
// must produce PING.
func TestListForAgentPingEnabledByMetadata(t *testing.T) {
	t.Setenv("AGENT_EXPANSION_INCLUDE_PING", "")
	db := newTestDB(t)
	const aID, bID = uint(1), uint(2)
	seedAgent(t, db, aID, "10.0.0.1", false, 0)
	seedAgent(t, db, bID, "10.0.0.2", true, 5005)

	md, err := json.Marshal(map[string]any{
		"expansion": map[string]any{"include_ping": true},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      aID,
		Type:         TypeAgent,
		Enabled:      true,
		AgentTargets: []uint{bID},
		Metadata:     datatypes.JSON(md),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := ListForAgent(context.Background(), db, nil, aID)
	if err != nil {
		t.Fatalf("ListForAgent: %v", err)
	}

	if got := len(filterByType(list, TypePing)); got != 1 {
		t.Errorf("PING probes = %d, want 1 (metadata override)", got)
	}
	if got := len(filterByType(list, TypeMTR)); got != 1 {
		t.Errorf("MTR probes = %d, want 1", got)
	}
}

// Per-probe metadata override beats the env default: env on, metadata false
// must NOT produce PING.
func TestListForAgentPingDisabledByMetadata(t *testing.T) {
	t.Setenv("AGENT_EXPANSION_INCLUDE_PING", "true")
	db := newTestDB(t)
	const aID, bID = uint(1), uint(2)
	seedAgent(t, db, aID, "10.0.0.1", false, 0)
	seedAgent(t, db, bID, "10.0.0.2", true, 5005)

	md, err := json.Marshal(map[string]any{
		"expansion": map[string]any{"include_ping": false},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      aID,
		Type:         TypeAgent,
		Enabled:      true,
		AgentTargets: []uint{bID},
		Metadata:     datatypes.JSON(md),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := ListForAgent(context.Background(), db, nil, aID)
	if err != nil {
		t.Fatalf("ListForAgent: %v", err)
	}

	if got := len(filterByType(list, TypePing)); got != 0 {
		t.Errorf("PING probes = %d, want 0 (metadata override says no, even with env=true)", got)
	}
	if got := len(filterByType(list, TypeMTR)); got != 1 {
		t.Errorf("MTR probes = %d, want 1", got)
	}
}

// Env-on alone (no metadata) must produce PING — covers the operator-friendly
// case where PING is enabled site-wide via .env.
func TestListForAgentPingEnabledByEnv(t *testing.T) {
	t.Setenv("AGENT_EXPANSION_INCLUDE_PING", "true")
	db := newTestDB(t)
	const aID, bID = uint(1), uint(2)
	seedAgent(t, db, aID, "10.0.0.1", false, 0)
	seedAgent(t, db, bID, "10.0.0.2", true, 5005)

	if _, err := Create(context.Background(), db, CreateInput{
		WorkspaceID:  1,
		AgentID:      aID,
		Type:         TypeAgent,
		Enabled:      true,
		AgentTargets: []uint{bID},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := ListForAgent(context.Background(), db, nil, aID)
	if err != nil {
		t.Fatalf("ListForAgent: %v", err)
	}

	if got := len(filterByType(list, TypePing)); got != 1 {
		t.Errorf("PING probes = %d, want 1 (env true, no metadata)", got)
	}
	if got := len(filterByType(list, TypeMTR)); got != 1 {
		t.Errorf("MTR probes = %d, want 1", got)
	}
}
