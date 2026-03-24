# Reverse Probe Calculation & Bidirectional Testing

This document explains how NetWatcher calculates, creates, and manages reverse/return-path probes across all probe types. It covers the full lifecycle from AGENT probe creation through expansion, delivery, and execution.

---

## Table of Contents

- [Concepts](#concepts)
- [The AGENT Probe Type](#the-agent-probe-type)
- [Forward Expansion (Owner)](#forward-expansion-owner)
- [Reverse Expansion (Target)](#reverse-expansion-target)
- [Probe Delivery: ListForAgent](#probe-delivery-listforagent)
- [TrafficSim Bidirectional Mode](#trafficsim-bidirectional-mode)
- [Allowed Agents & Server Authentication](#allowed-agents--server-authentication)
- [Agent-Side Reconciliation](#agent-side-reconciliation)
- [End-to-End Scenario](#end-to-end-scenario)
- [Key Files Reference](#key-files-reference)

---

## Concepts

### Forward vs Reverse Probes

When Agent A monitors Agent B, two test directions exist:

```
  Agent A ──────► Agent B     (Forward: A measures path to B)
  Agent A ◄────── Agent B     (Reverse: B measures path to A)
```

For full bidirectional visibility, both agents need probes targeting each other. The system automates this through **AGENT probe expansion**.

### Probe Expansion

AGENT probes are meta-probes — they don't execute directly. The controller **expands** each AGENT probe into concrete test types (MTR, PING, TRAFFICSIM) at delivery time. This expansion is direction-aware:

| Function | Direction | Caller | Purpose |
|----------|-----------|--------|---------|
| `expandAgentProbeForOwner` | Forward | Owner's `ListForAgent` | Agent A's probe → expand for A to test B |
| `expandAgentProbe` | Reverse | Target's `ListForAgent` | Agent A's probe → expand for B to test A |

---

## The AGENT Probe Type

An AGENT probe is created in the panel targeting one or more agents:

```json
{
  "type": "AGENT",
  "agent_id": 100,
  "agent_targets": [200],
  "bidirectional": true
}
```

**Stored in the database** as:
- `probes` table: `{ id: 42, agent_id: 100, type: "AGENT" }`
- `probe_targets` table: `{ probe_id: 42, agent_id: 200 }`

The AGENT probe itself is **never sent to the agent** — it's expanded into concrete probes at delivery time.

---

## Forward Expansion (Owner)

When Agent 100 checks in, `ListForAgent(100)` finds its AGENT probe and calls `expandAgentProbeForOwner`:

```
expandAgentProbeForOwner(probe=42, targetAgentID=200)
```

This creates:

| Probe Type | Target | Condition |
|-----------|--------|-----------|
| **MTR** | Agent 200's public IP | Always |
| **PING** | Agent 200's public IP | Always |
| **TRAFFICSIM** (client) | Agent 200's IP:port | If Agent 200 has TrafficSim server enabled |
| **TRAFFICSIM** (`:bidir` marker) | Agent 200's IP + `:bidir` | If Agent 100 (owner) has TrafficSim server enabled |

### The `:bidir` Marker

When the **owner** agent has a TrafficSim server (but the target doesn't), a special `:bidir` probe is created. This probe:

- Is **NOT** executed as a client connection
- Serves as a **detection anchor** — when Agent 200 connects as a client to Agent 100's server, the server finds this probe via `GetClientProbeForAgent` and enables bidirectional measurement
- Has `Server: false` but target format `<ip>:bidir`

```go
// The agent worker explicitly skips :bidir probes
if strings.HasSuffix(target, ":bidir") {
    // Just wait for stop signal — used for server detection only
    <-stopChan
    return
}
```

---

## Reverse Expansion (Target)

When Agent 200 checks in, `ListForAgent(200)` needs to discover that Agent 100 has an AGENT probe targeting it. This is done by:

1. Calling `findReverseAgentProbes(agentID=200)` — queries for AGENT probes from **other** agents that have a target with `agent_id = 200`
2. For each reverse probe found, calling `expandAgentProbe` to create return-path probes

```
findReverseAgentProbes(200) → finds probe 42 (owned by agent 100, targeting 200)
expandAgentProbe(probe=42, targetAgentID=200)
```

This creates probes **on Agent 200's probe list** that target Agent 100:

| Probe Type | Target | Condition |
|-----------|--------|-----------|
| **MTR** | Agent 100's public IP | Always |
| **PING** | Agent 100's public IP | Always |
| **TRAFFICSIM** (client) | Agent 100's IP:port | If Agent 100 has TrafficSim server enabled |

### Key Difference: Forward vs Reverse Expansion

```
Forward (expandAgentProbeForOwner):
  - Resolves TARGET agent's IP
  - Checks if TARGET has server → creates client probe
  - Checks if OWNER has server → creates :bidir marker

Reverse (expandAgentProbe):
  - Resolves SOURCE agent's IP  
  - Checks if SOURCE has server → creates client probe
  - No :bidir markers (source already handles bidirectional via forward expansion)
```

---

## Probe Delivery: ListForAgent

`ListForAgent` is the core function that assembles the complete probe list for each agent. Here's the full pipeline:

```
ListForAgent(agentID) {

  1. OWNED PROBES
     ├── Query: WHERE agent_id = ? AND enabled = true
     ├── For each TypeAgent probe:
     │   └── expandAgentProbeForOwner → MTR + PING + TRAFFICSIM
     └── For each other type (MTR, PING, TRAFFICSIM, DNS):
         └── Resolve target IPs if AgentID is set

  2. REVERSE AGENT PROBES  ← NEW
     ├── findReverseAgentProbes(agentID)
     │   └── Query: JOIN probe_targets WHERE target.agent_id = ? AND probe.type = AGENT
     └── For each reverse probe:
         └── expandAgentProbe → MTR + PING + TRAFFICSIM (return path)

  3. VIRTUAL TRAFFICSIM SERVER PROBE
     ├── If agent.TrafficSimEnabled:
     │   ├── Create server probe with host:port as Target[0]
     │   └── Populate AllowedAgents as Target[1..N]:
     │       ├── From reverse AGENT probes (others targeting this agent)
     │       └── From owned AGENT probes (this agent targeting others)
     └── Agents in AllowedAgents can connect to this server

  4. VIRTUAL DEFAULT PROBES
     └── NETINFO, SYSINFO, SPEEDTEST_SERVERS, SPEEDTEST
}
```

---

## TrafficSim Bidirectional Mode

Bidirectional mode allows the server to measure return-path latency using an existing client connection, without the server needing to initiate a separate outbound connection.

### How It Works

```
┌──────────────────────────────────────────────────────────────┐
│ Agent 200 (Client)                Agent 100 (Server)         │
│                                                              │
│  1. Client connects:                                         │
│     HELLO ──────────────────────► handleServerMessage()       │
│                                   ├── isAgentAllowed(200)? ✓  │
│                                   ├── Create AgentConnection  │
│                                   └── GetClientProbeForAgent  │
│                                       (200) → finds :bidir   │
│                                       probe → enables bidir  │
│                                                              │
│  2. Forward data (Client → Server):                          │
│     DATA ───────────────────────► Receives, sends ACK        │
│     ◄────────────────────── ACK   Client measures RTT         │
│                                                              │
│  3. Reverse data (Server → Client):                          │
│     ◄──────────────────── DATA    Server sends test packet   │
│     ACK ───────────────────────►  Server measures RTT        │
│                                                              │
│  4. Reporting:                                               │
│     Forward: ProbeID = client's probe, AgentID = 200         │
│     Reverse: ProbeID = :bidir probe, AgentID = 100           │
└──────────────────────────────────────────────────────────────┘
```

### Detection Flow

When a new client connects to the server:

```go
func (ts *TrafficSim) GetClientProbeForAgent(targetAgentID uint) *Probe {
    // Search all probes for a non-server TRAFFICSIM probe targeting this agent
    for _, p := range probesToCheck {
        if p.Type == TRAFFICSIM && !p.Server {
            for _, t := range p.Targets {
                if t.AgentID != nil && *t.AgentID == targetAgentID {
                    return &p  // Found! Enable bidirectional mode
                }
            }
        }
    }
    return nil  // No bidir for this agent
}
```

The server uses `GetProbesFunc` (a callback to `getCurrentProbes()`) to dynamically fetch the latest probe list, ensuring newly added AGENT probes are detected without server restart.

---

## Allowed Agents & Server Authentication

### How AllowedAgents Are Populated

The virtual TRAFFICSIM server probe carries allowed agent IDs as additional targets:

```
Target[0]: "0.0.0.0:5000"  (server listen address)
Target[1]: { AgentID: 200 } (allowed agent from reverse AGENT probe)
Target[2]: { AgentID: 300 } (allowed agent from owned AGENT probe)
```

The agent's `NewTrafficSim` constructor reads these:

```go
// NewTrafficSim in trafficsim.go
if ts.IsServer && len(probe.Targets) > 1 {
    for i := 1; i < len(probe.Targets); i++ {
        if probe.Targets[i].AgentID != nil {
            ts.AllowedAgents = append(ts.AllowedAgents, *probe.Targets[i].AgentID)
        }
    }
}
```

### Authentication Logic

```go
func (ts *TrafficSim) isAgentAllowed(agentID uint) bool {
    // If no allowed list, allow all (open server)
    if len(ts.AllowedAgents) == 0 {
        return true
    }
    // Otherwise, check against the list
    for _, allowed := range ts.AllowedAgents {
        if allowed == agentID { return true }
    }
    return false  // Rejected
}
```

### Dynamic Updates

When the controller sends updated probe lists with changed allowed agents, the running server is updated **without restart** via:

```
Controller sends updated probes → Agent reconciliation loop →
  updateServerAllowedAgents(probe) → server.UpdateAllowedAgents(agents)
```

---

## Agent-Side Reconciliation

The agent's `FetchProbesWorker` handles the probe lifecycle:

```
┌─────────────────────────────────────────────────────────┐
│ Every 60s: Controller sends probe list                  │
│                                                         │
│  1. Store probes: setCurrentProbes(probes)               │
│     (Used by server's GetProbesFunc for bidir detection) │
│                                                         │
│  2. Reconcile workers:                                   │
│     ├── KEPT: Same Type+Target → update probe reference  │
│     │   └── If TRAFFICSIM server: updateServerAllowed()  │
│     ├── NEW: Start new worker goroutine                  │
│     └── REMOVED: Stop worker, cleanup                    │
│                                                         │
│  3. For TRAFFICSIM server probes specifically:           │
│     ├── Register as activeTrafficSimServer              │
│     ├── SetAllProbes() for bidir detection              │
│     └── Set GetProbesFunc = getCurrentProbes             │
└─────────────────────────────────────────────────────────┘
```

### Worker Continuity

The system uses a **continuity key** (`probeID_type_target`) to track workers. This means:

- Probe ID changes (e.g., after controller restart) → worker continues without restart
- Target IP changes (e.g., agent reconnects with new IP) → worker restarts
- AllowedAgents changes → server updated in-place, no restart

---

## End-to-End Scenario

### Setup
- **Agent A** (ID: 100) — Has TrafficSim server enabled on port 5000
- **Agent B** (ID: 200) — No TrafficSim server
- **Agent A** has AGENT probe targeting Agent B

### What Happens

**Agent A checks in** (`ListForAgent(100)`):

| # | Probe | Type | Target | Notes |
|---|-------|------|--------|-------|
| 1 | Forward MTR | MTR | B's IP | Expanded from AGENT probe |
| 2 | Forward PING | PING | B's IP | Expanded from AGENT probe |
| 3 | Bidir marker | TRAFFICSIM | B's IP + `:bidir` | A has server → bidir marker for when B connects |
| 4 | Virtual server | TRAFFICSIM (server) | 0.0.0.0:5000 | From agent settings, AllowedAgents: [200] |
| 5 | Default probes | NETINFO, SYSINFO, etc | — | Always present |

**Agent B checks in** (`ListForAgent(200)`):

| # | Probe | Type | Target | Notes |
|---|-------|------|--------|-------|
| 1 | Reverse MTR | MTR | A's IP | Expanded from A's reverse AGENT probe |
| 2 | Reverse PING | PING | A's IP | Expanded from A's reverse AGENT probe |
| 3 | Reverse TRAFFICSIM | TRAFFICSIM (client) | A's IP:5000 | A has server → client probe for B |
| 4 | Default probes | NETINFO, SYSINFO, etc | — | Always present |

**Data Flow**:

```
Agent B (client) connects to Agent A (server) on port 5000
  ├── Forward: B sends DATA → A sends ACK → B measures RTT (probe #3 on B)
  ├── Reverse: A sends DATA → B sends ACK → A measures RTT (probe #3 on A, :bidir)
  ├── Agent B reports forward stats with its probe ID
  └── Agent A reports reverse stats with its :bidir probe ID
```

### Two-Server Scenario

If **both** agents have TrafficSim servers:

**Agent A** (`ListForAgent(100)`):
- Forward MTR + PING to B
- TRAFFICSIM **client** probe to B (B has server)
- Virtual server probe (AllowedAgents: [200])

**Agent B** (`ListForAgent(200)`):
- Reverse MTR + PING to A  
- TRAFFICSIM **client** probe to A (A has server)
- Each acts as both client and server to the other

---

## Key Files Reference

### Controller

| File | Function | Purpose |
|------|----------|---------|
| `probe.go` | `ListForAgent` | Assembles probe list with forward + reverse expansion |
| `probe.go` | `expandAgentProbeForOwner` | Forward: owner's AGENT probe → concrete probes |
| `probe.go` | `expandAgentProbe` | Reverse: other's AGENT probe → return-path probes |
| `probe.go` | `findReverseAgentProbes` | Finds AGENT probes from others targeting this agent |
| `probe.go` | `hasTrafficSimServer` | Checks if agent has TrafficSim server enabled |
| `probe.go` | `createExpandedProbe` | Creates a concrete probe from an AGENT template |
| `trafficsim.go` | `initTrafficSim` | Registers the TrafficSim data handler for ClickHouse |

### Agent

| File | Function | Purpose |
|------|----------|---------|
| `trafficsim.go` | `NewTrafficSim` | Constructor — parses targets, populates AllowedAgents |
| `trafficsim.go` | `GetClientProbeForAgent` | Finds bidir probe for a connected client |
| `trafficsim.go` | `isAgentAllowed` | Checks if agent can connect to server |
| `trafficsim.go` | `UpdateAllowedAgents` | Thread-safe setter for live allowed agent updates |
| `trafficsim.go` | `sendReverseDataPacket` | Server sends test data back to client |
| `trafficsim.go` | `reportCycleStats` | Reports reverse direction stats with bidir probe ID |
| `probes.go` | `handleTrafficSimProbe` | Starts TrafficSim client/server, registers server instance |
| `probes.go` | `updateServerAllowedAgents` | Extracts agent IDs from targets, pushes to running server |
| `probes.go` | `FetchProbesWorker` | Probe reconciliation — keeps/adds/removes workers |
