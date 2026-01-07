# TrafficSim Architecture

TrafficSim is NetWatcher's agent-to-agent traffic simulation probe. It provides continuous latency and packet loss monitoring between distributed agents using UDP.

> **Status:** Currently disabled (`trafficsim.go.disabled`) due to MongoDB dependencies and incomplete testing. This document captures the architecture for future re-implementation.

---

## Overview

```
┌─────────────────────┐                    ┌─────────────────────┐
│     Agent A         │                    │     Agent B         │
│   (Client Mode)     │                    │   (Server Mode)     │
│                     │                    │                     │
│  ┌───────────────┐  │     UDP Port       │  ┌───────────────┐  │
│  │  TrafficSim   │──┼────────────────────┼──│  TrafficSim   │  │
│  │   Client      │  │                    │  │   Server      │  │
│  └───────────────┘  │                    │  └───────────────┘  │
│         │           │                    │         │           │
│         ▼           │                    │         ▼           │
│  ┌───────────────┐  │                    │  ┌───────────────┐  │
│  │ Flow Stats    │  │                    │  │ Connection    │  │
│  │ Cycle Tracker │  │                    │  │ Manager       │  │
│  └───────────────┘  │                    │  └───────────────┘  │
└─────────────────────┘                    └─────────────────────┘
```

---

## Protocol

### Message Types

| Type | Direction | Purpose |
|------|-----------|---------|
| `HELLO` | Client→Server | Connection handshake |
| `ACK` | Server→Client | Acknowledge packet receipt |
| `DATA` | Client→Server | Regular traffic packet |
| `PING` | Bidirectional | Keepalive |
| `PONG` | Bidirectional | Keepalive response |
| `REPORT` | Server→Client | Server-side statistics |

### Message Structure

```go
type TrafficSimMsg struct {
    Type      TrafficSimMsgType  `json:"type"`
    Data      TrafficSimData     `json:"data"`
    Src       primitive.ObjectID `json:"src"`      // TODO: Change to uint
    Dst       primitive.ObjectID `json:"dst"`      // TODO: Change to uint
    Timestamp int64              `json:"timestamp"`
    Size      int                `json:"size"`
}

type TrafficSimData struct {
    Sent     int64                  `json:"sent"`     // Unix ms
    Received int64                  `json:"received"` // Unix ms
    Seq      int                    `json:"seq"`      // Sequence number
    Report   map[string]interface{} `json:"report"`   // Stats (REPORT only)
}
```

---

## Cycle-Based Reporting

TrafficSim uses a **cycle-based model** where statistics are calculated over fixed packet windows:

```
┌────────────────────── Cycle 1 ──────────────────────┐
│ Packet 1 → Packet 2 → ... → Packet 60 → REPORT     │
└──────────────────────────────────────────────────────┘
                                                  │
                                                  ▼
┌────────────────────── Cycle 2 ──────────────────────┐
│ Packet 1 → Packet 2 → ... → Packet 60 → REPORT     │
└──────────────────────────────────────────────────────┘
```

### Constants

```go
const (
    TrafficSim_ReportSeq    = 60          // Packets per cycle
    TrafficSim_DataInterval = 1           // Seconds between packets
    PacketTimeout           = 2 * time.Second
    RetryInterval           = 5 * time.Second
    GracefulShutdownTimeout = 70 * time.Second
)
```

### Cycle Tracker

```go
type CycleTracker struct {
    StartSeq    int
    EndSeq      int
    PacketSeqs  []int              // All packets in cycle
    StartTime   time.Time
    PacketTimes map[int]PacketTime // Per-packet timing
    mu          sync.RWMutex
}
```

---

## Flow Statistics

Comprehensive per-flow metrics are tracked:

```go
type FlowStats struct {
    Direction     string    // "client-server" or "server-client"
    StartTime     time.Time
    EndTime       time.Time
    PacketsSent   int
    PacketsRecv   int
    PacketsLost   int
    BytesSent     int64
    BytesRecv     int64
    RTTStats      RTTStatistics
    JitterStats   JitterStatistics
    PacketDetails map[int]*PacketDetail
}

type RTTStatistics struct {
    Min    time.Duration
    Max    time.Duration
    Avg    time.Duration
    StdDev time.Duration
    P50    time.Duration  // 50th percentile
    P95    time.Duration  // 95th percentile
    P99    time.Duration  // 99th percentile
}
```

### Reported Metrics

| Metric | Description |
|--------|-------------|
| `lostPackets` | Packets without ACK within timeout |
| `lossPercentage` | Packet loss as percentage |
| `outOfSequence` | Packets received out of order |
| `duplicatePackets` | Duplicate ACKs received |
| `averageRTT` | Mean round-trip time (ms) |
| `minRTT` | Minimum RTT |
| `maxRTT` | Maximum RTT |
| `stdDevRTT` | RTT standard deviation |
| `flows` | Per-flow breakdown |

---

## Network Interface Selection

TrafficSim has a sophisticated interface selection algorithm:

```go
func (ts *TrafficSim) scoreInterface(iface net.Interface, ip net.IP) int {
    score := 0
    
    // Prefer running interfaces (+20)
    if iface.Flags&net.FlagRunning != 0 { score += 20 }
    
    // Prefer standard interface names
    if strings.HasPrefix(name, "en") || strings.HasPrefix(name, "eth") {
        score += 15  // Ethernet
    } else if strings.HasPrefix(name, "wlan") || strings.HasPrefix(name, "wi") {
        score += 10  // WiFi
    }
    
    // Prefer previously used interface (+25)
    if ts.preferredInterface != "" && iface.Name == ts.preferredInterface {
        score += 25
    }
    
    // Prefer private networks
    if ip.IsPrivate() { score += 10 }
    if ip[0] == 192 && ip[1] == 168 { score += 5 }  // Home networks
    
    // Penalize virtual interfaces
    if strings.Contains(name, "docker") { score -= 20 }
    if strings.Contains(name, "veth") { score -= 20 }
    if strings.Contains(name, "vmnet") { score -= 20 }
    
    // Penalize VPN interfaces
    if strings.Contains(name, "tun") { score -= 15 }
    if strings.Contains(name, "tap") { score -= 15 }
    
    return score
}
```

### Interface Monitoring

A background goroutine monitors interface validity:

```go
func (ts *TrafficSim) runInterfaceMonitor(ctx context.Context) {
    ticker := time.NewTicker(ts.InterfaceCheckInterval)
    for {
        select {
        case <-ticker.C:
            if !ts.isIPStillValid(ts.localIP) {
                // Force interface reselection
                ts.setConnectionValid(false)
            }
        }
    }
}
```

---

## Connection Management

### Client Mode

1. **Establish Connection** → Select best interface, dial UDP
2. **Handshake** → Send HELLO, wait for ACK
3. **Data Loop** → Send packets at interval
4. **Receive Loop** → Listen for ACKs
5. **Cycle Complete** → Report statistics
6. **Repeat**

### Server Mode

1. **Listen** → Bind to UDP port
2. **Accept Connections** → Per-client state
3. **Process Packets** → Update stats, send ACKs
4. **Report** → Periodic server-side stats

### Reconnection Logic

```go
func (ts *TrafficSim) reestablishConnection() bool {
    ts.closeConnection()
    return ts.establishUDPConnection()
}

func (ts *TrafficSim) continuousHandshakeAttempts(ctx context.Context, ...) bool {
    maxHandshakeTime := 10 * time.Second
    for time.Since(start) < maxHandshakeTime {
        if ts.establishUDPConnection() {
            if ts.attemptSingleHandshake(flow, cycle) {
                return true
            }
        }
        time.Sleep(2 * time.Second)
    }
    return false
}
```

---

## Triggered MTR

TrafficSim includes **automatic network diagnostics** that trigger MTR probes when anomalies are detected.

### Trigger Conditions

| Condition | Threshold | Action |
|-----------|-----------|--------|
| **Packet Loss** | >5% in cycle | Trigger MTR to target |
| **High Latency** | >2x baseline RTT | Trigger MTR to target |
| **Jitter Spike** | >3x std dev | Trigger MTR to target |

### Implementation

```go
// After cycle stats calculation
func (ts *TrafficSim) checkAndTriggerMTR(stats map[string]interface{}, mtrProbe *Probe) {
    lossPercentage := stats["lossPercentage"].(float64)
    avgRTT := stats["averageRTT"].(float64)
    
    // Trigger on packet loss
    if lossPercentage > 5.0 && ts.isRunning() && !ts.isStopping() {
        ts.triggerMTR(mtrProbe, fmt.Sprintf("packet_loss:%.2f%%", lossPercentage))
        return
    }
    
    // Trigger on high latency (compare to baseline)
    if ts.baselineRTT > 0 && avgRTT > ts.baselineRTT*2 {
        ts.triggerMTR(mtrProbe, fmt.Sprintf("high_latency:%.0fms", avgRTT))
        return
    }
    
    // Trigger on jitter spike
    stdDevRTT := stats["stdDevRTT"].(float64)
    if ts.baselineJitter > 0 && stdDevRTT > ts.baselineJitter*3 {
        ts.triggerMTR(mtrProbe, fmt.Sprintf("jitter_spike:%.0fms", stdDevRTT))
    }
}

func (ts *TrafficSim) triggerMTR(mtrProbe *Probe, reason string) {
    log.Printf("TrafficSim: Triggering MTR due to %s", reason)
    
    // Run MTR with triggered=true and increased cycle count
    mtrResult, err := Mtr(mtrProbe, true) // triggered=true → 15 cycles
    if err != nil {
        log.Errorf("TrafficSim: Failed to run triggered MTR: %v", err)
        return
    }
    
    // Report MTR result with triggered flag
    payload, _ := json.Marshal(mtrResult)
    ts.DataChan <- probes.ProbeData{
        ProbeID:         mtrProbe.ID,
        Type:            probes.ProbeType_MTR,
        Triggered:       true,
        TriggeredReason: reason,
        Payload:         payload,
        Target:          ts.IPAddress,
    }
}
```

### Baseline Calculation

```go
// Calculate baseline from first N successful cycles
const BaselineCycles = 5

func (ts *TrafficSim) updateBaseline(stats map[string]interface{}) {
    if ts.cycleCount < BaselineCycles {
        ts.rttSamples = append(ts.rttSamples, stats["averageRTT"].(float64))
        ts.jitterSamples = append(ts.jitterSamples, stats["stdDevRTT"].(float64))
        ts.cycleCount++
        
        if ts.cycleCount == BaselineCycles {
            ts.baselineRTT = average(ts.rttSamples)
            ts.baselineJitter = average(ts.jitterSamples)
            log.Printf("TrafficSim: Baseline established - RTT: %.0fms, Jitter: %.0fms",
                ts.baselineRTT, ts.baselineJitter)
        }
    }
}
```

---

## Bidirectional Metrics

Each agent reports metrics **from its own perspective**, enabling full path analysis:

### Reporting Model

```
┌─────────────────────────────────────────────────────────────────────┐
│                        CLIENT AGENT (101)                            │
│                                                                      │
│   Reports for probe targeting Server Agent (100):                   │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │ ProbeData {                                                  │   │
│   │   probe_id: 42,                                              │   │
│   │   agent_id: 101,         // Who is reporting                 │   │
│   │   target_agent: 100,     // Who we're testing against        │   │
│   │   payload: {                                                 │   │
│   │     direction: "client-to-server",                           │   │
│   │     rtt_stats: { min, max, avg, p95, p99 },                  │   │
│   │     loss_percentage: 2.5,                                    │   │
│   │     jitter_stats: { min, max, avg },                         │   │
│   │   }                                                          │   │
│   │ }                                                            │   │
│   └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                        SERVER AGENT (100)                            │
│                                                                      │
│   Reports for probe targeting Client Agent (101):                   │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │ ProbeData {                                                  │   │
│   │   probe_id: 43,          // Server's perspective probe       │   │
│   │   agent_id: 100,         // Who is reporting                 │   │
│   │   target_agent: 101,     // Who we're measuring              │   │
│   │   payload: {                                                 │   │
│   │     direction: "server-to-client",                           │   │
│   │     packets_received: 58,                                    │   │
│   │     packets_sent: 58,    // ACKs sent                        │   │
│   │     connection_info: { first_seen, last_seen },              │   │
│   │   }                                                          │   │
│   │ }                                                            │   │
│   └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### Combined View in ClickHouse

```sql
-- Query bidirectional metrics between two agents
WITH 
    client_view AS (
        SELECT 
            created_at,
            JSONExtractFloat(payload_raw, 'loss_percentage') as client_loss,
            JSONExtractFloat(payload_raw, 'rtt_stats', 'avg') as client_rtt
        FROM probe_data 
        WHERE type = 'TRAFFICSIM' 
          AND agent_id = 101 
          AND target_agent = 100
    ),
    server_view AS (
        SELECT 
            created_at,
            JSONExtractInt(payload_raw, 'packets_received') as server_recv
        FROM probe_data 
        WHERE type = 'TRAFFICSIM' 
          AND agent_id = 100 
          AND target_agent = 101
    )
SELECT 
    c.created_at,
    c.client_loss,
    c.client_rtt,
    s.server_recv
FROM client_view c
JOIN server_view s ON abs(dateDiff('second', c.created_at, s.created_at)) < 5
ORDER BY c.created_at DESC;
```

### Flow Direction Tracking

```go
type FlowDirection string

const (
    FlowClientToServer FlowDirection = "client-to-server"
    FlowServerToClient FlowDirection = "server-to-client"
)

type BidirectionalStats struct {
    // Client perspective (what client sees)
    ClientToServer FlowStats `json:"client_to_server"`
    
    // Server perspective (what server sees from this client)
    ServerToClient FlowStats `json:"server_to_client"`
    
    // Asymmetry metrics
    LatencyAsymmetry float64 `json:"latency_asymmetry_ms"`  // Difference in RTT
    LossAsymmetry    float64 `json:"loss_asymmetry_pct"`    // Difference in loss rates
}
```

### Triggered Probe Tagging

All triggered probes are tagged for correlation:

```go
type ProbeData struct {
    // ... existing fields
    Triggered       bool   `json:"triggered"`
    TriggeredReason string `json:"triggered_reason"`
    // Examples:
    // "packet_loss:12.5%"
    // "high_latency:250ms" 
    // "jitter_spike:45ms"
}
```

This allows querying all diagnostics triggered by TrafficSim:

```sql
SELECT 
    created_at,
    type,
    triggered_reason,
    agent_id,
    target
FROM probe_data
WHERE triggered = true
  AND type = 'MTR'
ORDER BY created_at DESC;
```

---

## Data Structures

### TrafficSim Struct

```go
type TrafficSim struct {
    // State
    Running       int32  // atomic: 0=stopped, 1=running
    stopping      int32  // atomic: 0=no, 1=yes
    
    // Identity
    ThisAgent     primitive.ObjectID  // TODO: uint
    OtherAgent    primitive.ObjectID  // TODO: uint
    
    // Connection
    Conn          *net.UDPConn
    IPAddress     string
    Port          int64
    IsServer      bool
    
    // Authorization (server only)
    AllowedAgents []primitive.ObjectID
    Connections   map[primitive.ObjectID]*Connection
    
    // Statistics
    ClientStats   *ClientStats
    flowStats     map[string]*FlowStats
    serverStats   *ServerStats
    
    // Cycle management
    currentCycle   *CycleTracker
    packetsInCycle int
    
    // Interface selection
    localIP            string
    preferredInterface string
}
```

---

## Issues for Re-implementation

### 1. MongoDB → PostgreSQL/ClickHouse Migration

The original implementation used MongoDB ObjectIDs throughout:

```go
// OLD (MongoDB)
import "go.mongodb.org/mongo-driver/bson/primitive"

type TrafficSimMsg struct {
    Src primitive.ObjectID `json:"src"`
    Dst primitive.ObjectID `json:"dst"`
}

// NEW (PostgreSQL)
type TrafficSimMsg struct {
    SrcAgentID uint `json:"src_agent_id"`
    DstAgentID uint `json:"dst_agent_id"`
}
```

**Affected areas:**
- `TrafficSim.ThisAgent` / `OtherAgent`
- `TrafficSimMsg.Src` / `Dst`
- `Connection.AgentID`
- `AllowedAgents []primitive.ObjectID`
- All flow tracking keys

### 2. ProbeData Structure Alignment

Current controller expects:
```go
type ProbeData struct {
    ProbeID     uint            `json:"probe_id"`
    AgentID     uint            `json:"agent_id"`     // Reporting agent
    Type        Type            `json:"type"`
    Payload     json.RawMessage `json:"payload"`
    Target      string          `json:"target"`
    TargetAgent uint            `json:"target_agent"`
}
```

TrafficSim needs to emit this format, not the old MongoDB-based structure.

### 3. Typo Fix

```go
// Line ~1080
reportingAgent, err := primitive.ObjectIDFromHex(os.getEnv("AGENT_ID"))
//                                                    ^ should be os.Getenv
```

---

## Bidirectional Architecture

TrafficSim supports **bidirectional latency measurement** where one agent acts as server and others connect as clients:

```
                    ┌──────────────────────────────────────┐
                    │           SERVER AGENT               │
                    │    (Port-forwarded UDP endpoint)     │
                    │                                      │
                    │   AllowedAgents: [101, 102, 103]     │
                    │   Listening: 0.0.0.0:5000            │
                    └──────────────┬───────────────────────┘
                                   │
                     ┌─────────────┼─────────────┐
                     │             │             │
                     ▼             ▼             ▼
              ┌──────────┐  ┌──────────┐  ┌──────────┐
              │ Agent 101│  │ Agent 102│  │ Agent 103│
              │ (Client) │  │ (Client) │  │ (Client) │
              └──────────┘  └──────────┘  └──────────┘
```

### Measurement Directions

| Direction | What's Measured | Who Reports |
|-----------|-----------------|-------------|
| **Client → Server** | Upload latency from client perspective | Client agent |
| **Server → Client** | Download latency from client perspective | Client agent |
| **Aggregate** | Bidirectional flow stats | Both |

### Server-Side Authentication

The server agent maintains a list of allowed client agents:

```go
type TrafficSim struct {
    AllowedAgents []uint          // Agents permitted to connect
    Connections   map[uint]*Connection  // Active client connections
}
```

When a client sends a `HELLO`:
1. Server validates `SrcAgentID` is in `AllowedAgents`
2. If valid: create/update `Connection`, send `ACK`
3. If invalid: ignore packet (no response)

---

## Controller IP Resolution

When a probe targets another **agent** (instead of a custom IP), the **controller resolves the IP** before sending the probe config to the agent. This logic already exists for MTR/PING probes.

### Probe Target Types

```go
type ProbeTarget struct {
    ID      int    `json:"id"`
    ProbeId int    `json:"probe_id"`
    Target  string `json:"target"`   // IP/hostname (filled by controller)
    AgentID *uint  `json:"agent_id"` // Target agent (set in panel)
    GroupID *uint  `json:"group_id"` // Optional grouping
}
```

### IP Resolution Flow

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│      Panel      │     │   Controller    │     │      Agent      │
│                 │     │                 │     │                 │
│ Select target   │     │                 │     │                 │
│ agent (ID: 101) │────▶│                 │     │                 │
│                 │     │                 │     │                 │
│ agent_targets:  │     │ ListForAgent()  │     │                 │
│   [101]         │     │       │         │     │                 │
│                 │     │       ▼         │     │                 │
│                 │     │ getPublicIP(101)│     │                 │
│                 │     │       │         │     │                 │
│                 │     │       ▼         │     │                 │
│                 │     │ 1. Check        │     │                 │
│                 │     │    PublicIP     │     │                 │
│                 │     │    Override     │     │                 │
│                 │     │       │         │     │                 │
│                 │     │       ▼         │     │                 │
│                 │     │ 2. Get latest   │     │                 │
│                 │     │    NETINFO from │     │                 │
│                 │     │    ClickHouse   │     │                 │
│                 │     │       │         │     │                 │
│                 │     │       ▼         │     │                 │
│                 │     │ Target.Target = │────▶│ Receives        │
│                 │     │ "203.0.113.50"  │     │ probe with IP   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Existing Controller Logic

```go
// controller/internal/probe/probe.go

func ListForAgent(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint) ([]Probe, error) {
    // ...fetch probes...
    
    for i := range out {
        p := &out[i]
        for j := range p.Targets {
            t := &p.Targets[j]
            
            switch p.Type {
            case TypeMTR, TypePing:
                // Fill target when empty and we have a target agent
                if t.Target == "" && t.AgentID != nil {
                    aid := *t.AgentID
                    ip, err := getPublicIP(ctx, db, ch, aid)
                    if err != nil {
                        continue
                    }
                    t.Target = ip  // Controller fills the IP!
                }
            }
        }
    }
    return out, nil
}

func getPublicIP(ctx context.Context, db *gorm.DB, ch *sql.DB, agentID uint) (string, error) {
    // 1. Check for manual override
    agent, _ := agent.GetAgentByID(ctx, db, agentID)
    if agent.PublicIPOverride != "" {
        return agent.PublicIPOverride, nil
    }
    
    // 2. Get from latest NETINFO in ClickHouse
    netInfoPayload, _ := GetLatestNetInfoForAgent(ctx, ch, uint64(agentID), nil)
    var netInfo struct {
        PublicAddress string `json:"public_address"`
    }
    json.Unmarshal(netInfoPayload.Payload, &netInfo)
    return netInfo.PublicAddress, nil
}
```

### TrafficSim Extension

For TrafficSim, the controller needs to also append the **port** to the resolved IP:

```go
case TypeTrafficSim:
    if t.Target == "" && t.AgentID != nil {
        aid := *t.AgentID
        ip, _ := getPublicIP(ctx, db, ch, aid)
        
        // Get port from server probe's metadata or targets
        serverProbe, _ := GetServerProbeForAgent(ctx, db, aid)
        port := extractPortFromProbe(serverProbe)  // e.g., 5000
        
        t.Target = fmt.Sprintf("%s:%d", ip, port)
    }
```

---

## Panel Probe Creation

The panel's `NewProbe.vue` already supports agent-type targets:

### Agent Target Selection

```typescript
// panel/src/views/probes/NewProbe.vue

interface ProbeState {
    targetAgent: boolean;              // true = agent target, false = custom
    targetAgentSelected: Agent | null; // Selected agent
    hostInput: string;                 // Custom IP/hostname
    portInput: string;                 // Port (default "5000")
}

// Probe types that support agent targets
const showTargetAgentOption = computed(() => {
    const validTypes = ['MTR', 'PING', 'RPERF', 'TRAFFICSIM', 'AGENT'];
    return validTypes.includes(state.selected.value || '');
});
```

### Submission Logic

```typescript
async function submit() {
    // If targeting an agent
    if (state.targetAgent && state.targetAgentSelected) {
        newProbe.agent_targets = [state.targetAgentSelected.id];
        newProbe.targets = [];  // Controller will fill this
    } else {
        // Custom IP/hostname
        newProbe.targets = [state.hostInput];
        newProbe.agent_targets = [];
    }
}
```

### TrafficSim Server Mode

```typescript
if (state.selected.value === 'TRAFFICSIM' && state.probe.server) {
    newProbe.server = true;
    const listenHost = state.hostInput.trim() 
        ? state.hostInput.trim() + ":" + state.portInput 
        : '0.0.0.0:' + state.portInput;
    newProbe.targets = [listenHost];
}
```

---

## Probe Configuration

```yaml
# Server Agent (ID: 100)
probe:
  type: TRAFFICSIM
  agent_id: 100
  server: true
  targets:
    - target: "0.0.0.0:5000"  # Listen address
    - agent_id: 101           # Allowed client
    - agent_id: 102           # Allowed client
    - agent_id: 103           # Allowed client

# Client Agent (ID: 101)
probe:
  type: TRAFFICSIM
  agent_id: 101
  server: false
  targets:
    - target: "server-agent.example.com:5000"
    - agent_id: 100           # Server agent (for reference)
```

---

## Re-implementation Plan

### Phase 1: Data Model Migration (1-2 days)

**Goal:** Replace all MongoDB types with PostgreSQL-compatible types.

| Change | Files Affected |
|--------|----------------|
| Replace `primitive.ObjectID` → `uint` | `trafficsim.go` |
| Update `TrafficSimMsg` struct | `trafficsim.go` |
| Update `Connection` struct | `trafficsim.go` |
| Update flow key format | `trafficsim.go` |
| Fix `os.getEnv` typo | `trafficsim.go` |

```go
// Before
type TrafficSimMsg struct {
    Src primitive.ObjectID `json:"src"`
    Dst primitive.ObjectID `json:"dst"`
}

// After
type TrafficSimMsg struct {
    SrcAgentID uint `json:"src_agent_id"`
    DstAgentID uint `json:"dst_agent_id"`
}
```

### Phase 2: ProbeData Alignment (1 day)

**Goal:** Align with current controller `probe.Dispatch` expectations.

```go
// Create payload structure
type TrafficSimPayload struct {
    Direction      string             `json:"direction"`
    CycleRange     CycleRange         `json:"cycle_range"`
    LostPackets    int                `json:"lost_packets"`
    LossPercentage float64            `json:"loss_percentage"`
    OutOfSequence  int                `json:"out_of_sequence"`
    RTTStats       RTTStatistics      `json:"rtt_stats"`
    JitterStats    JitterStatistics   `json:"jitter_stats"`
    Flows          map[string]FlowStats `json:"flows"`
    Timestamp      time.Time          `json:"timestamp"`
}

// Emit to data channel
payload, _ := json.Marshal(stats)
ts.DataChan <- probes.ProbeData{
    ProbeID:     ts.Probe.ID,
    Type:        probes.ProbeType_TRAFFICSIM,
    Payload:     payload,
    Target:      fmt.Sprintf("%s:%d", ts.IPAddress, ts.Port),
    TargetAgent: ts.OtherAgentID,  // uint now
}
```

### Phase 3: Controller Handler (0.5 days)

**Goal:** Add ClickHouse storage for TrafficSim data.

```go
// controller/internal/probe/trafficsim.go
func initTrafficSim(db *sql.DB) {
    Register(NewHandler[TrafficSimPayload](
        TypeTrafficSim,
        nil, // validation
        func(ctx context.Context, data ProbeData, p TrafficSimPayload) error {
            return SaveRecordCH(ctx, db, data, string(TypeTrafficSim), p)
        },
    ))
}
```

Add to `InitWorkers`:
```go
func InitWorkers(ch *sql.DB) {
    // ...existing handlers
    initTrafficSim(ch)
}
```

### Phase 4: Server Mode Polish (1-2 days)

**Goal:** Ensure server mode is production-ready.

- [ ] Validate `AllowedAgents` from agent config
- [ ] Add connection timeout/cleanup
- [ ] Implement proper REPORT messages
- [ ] Test multi-client scenarios
- [ ] Add server-side metrics to ClickHouse

### Phase 5: Testing (2-3 days)

| Test | Description |
|------|-------------|
| **Unit tests** | Message parsing, stats calculation |
| **2-agent integration** | Client↔Server basic flow |
| **Multi-client** | 3+ clients to 1 server |
| **Reconnection** | Network interruption recovery |
| **Interface failover** | WiFi→Ethernet transition |
| **High packet loss** | Simulated 50%+ loss |

### Phase 6: Enable & Document (0.5 days)

1. Rename `trafficsim.go.disabled` → `trafficsim.go`
2. Add to worker switch in `workers/probes.go`
3. Update `docs/agent-probes.md`
4. Add panel UI components if needed

---

## Simplified Alternative

If full re-implementation is too complex, consider a **simplified version**:

```go
type SimpleTrafficSim struct {
    ServerAddr  string
    Interval    time.Duration
    PacketSize  int
}

// Just UDP ping-pong with timestamps
func (s *SimpleTrafficSim) Run(ctx context.Context) {
    for {
        sent := time.Now()
        conn.Write(timestampPacket)
        conn.Read(response)  
        rtt := time.Since(sent)
        // Report RTT
    }
}
```

This would provide:
- ✅ Basic latency measurement
- ✅ Packet loss detection
- ❌ No flow statistics
- ❌ No percentile calculations
- ❌ No bidirectional metrics

---

## ClickHouse Schema

```sql
-- Add TrafficSim-specific columns or use generic probe_data
-- The payload_raw column already stores JSON, which works for TrafficSim

-- Example query for TrafficSim data
SELECT 
    created_at,
    agent_id,
    target_agent,
    JSONExtractFloat(payload_raw, 'loss_percentage') as loss_pct,
    JSONExtractFloat(payload_raw, 'rtt_stats', 'avg') as avg_rtt_ms
FROM probe_data
WHERE type = 'TRAFFICSIM'
ORDER BY created_at DESC
LIMIT 100;
```

---

## Configuration

### Probe Config

```go
type Probe struct {
    ID          uint
    AgentID     uint      // Agent running the probe
    Type        "TRAFFICSIM"
    Server      bool      // true = server mode
    Targets []ProbeTarget{
        {Target: "192.168.1.100:5000"},  // For client: server address
        {AgentID: 456},                  // For auth: allowed agents
    }
}
```

### Environment

| Variable | Description |
|----------|-------------|
| `AGENT_ID` | This agent's ID (for reporting) |

---

## Key Files

| File | Purpose |
|------|---------|
| `agent/probes/trafficsim.go.disabled` | Main implementation (2958 lines) |
| `agent/workers/probes.go` | Worker integration |
| `agent/TRAFFIC_SIMULATION.md` | Original design notes |
