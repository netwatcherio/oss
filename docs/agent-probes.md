# Agent Probe System

This document describes how the NetWatcher Agent probe system works, including the data flow, probe types, and implementation details.

## Overview

The agent executes network monitoring probes based on configurations received from the controller. Results are sent back via WebSocket and stored in ClickHouse for time-series analysis.

```
┌──────────────────────────────────────────────────────────────┐
│                        CONTROLLER                             │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────┐   │
│  │  WebSocket  │───▶│  Registry   │───▶│   ClickHouse    │   │
│  │   Server    │    │  Dispatch   │    │   SaveRecordCH  │   │
│  └──────▲──────┘    └─────────────┘    └─────────────────┘   │
│         │                                                     │
└─────────┼─────────────────────────────────────────────────────┘
          │ probe_get / probe_post
┌─────────┴─────────────────────────────────────────────────────┐
│                          AGENT                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────┐   │
│  │  WSClient   │───▶│   Workers   │───▶│     Probes      │   │
│  │  (neffos)   │    │             │    │  (PING/MTR/etc) │   │
│  └─────────────┘    └─────────────┘    └─────────────────┘   │
└───────────────────────────────────────────────────────────────┘
```

---

## Data Flow

### 1. Fetch Probe Configurations

```
Agent                                    Controller
  |                                           |
  |── emit("probe_get", "hello") ───────────▶|
  |                                           |
  |◀─── emit("probe_get", [probes...]) ──────|
  |                                           |
```

The agent requests its probe configurations when:
- First connecting to the WebSocket
- Every 60 seconds (polling interval)

### 2. Execute Probes

The `FetchProbesWorker` receives probe configs and spawns workers:

```go
// workers/probes.go
for _, probe := range probes {
    switch probe.Type {
    case probes.ProbeType_PING:
        go handlePingProbe(probe, dataChan)
    case probes.ProbeType_MTR:
        go handleMTRProbe(probe, dataChan)
    case probes.ProbeType_SPEEDTEST:
        go handleSpeedTestProbe(probe, dataChan)
    // ... etc
    }
}
```

### 3. Submit Results

```
Agent                                    Controller
  |                                           |
  |── emit("probe_post", ProbeData) ────────▶|
  |                                           |
  |◀─── emit("probe_post_ok", {ok:true}) ────|
  |                                           |
```

---

## Probe Types

### PING

**Purpose:** ICMP ping with comprehensive RTT statistics

**Library:** `github.com/prometheus-community/pro-bing`

**Payload:**
```go
type PingPayload struct {
    StartTimestamp        time.Time     
    StopTimestamp         time.Time     
    PacketsRecv           int           
    PacketsSent           int           
    PacketsRecvDuplicates int           
    PacketLoss            float64       
    Addr                  string        
    MinRtt                time.Duration 
    MaxRtt                time.Duration 
    AvgRtt                time.Duration 
    StdDevRtt             time.Duration 
}
```

**Execution:**
- Default: 60 packets at 1-second intervals
- Uses privileged mode (raw ICMP sockets)
- Platform-aware (Windows payload size: 548 bytes)

---

### MTR (Multi-hop Traceroute)

**Purpose:** Traceroute with per-hop latency and loss statistics

**Tool:** External `trippy` binary (Rust-based MTR alternative)

**Payload:**
```go
type MtrPayload struct {
    StartTimestamp time.Time
    StopTimestamp  time.Time
    Report struct {
        Info struct {
            Target struct {
                IP       string 
                Hostname string 
            }
        }
        Hops []struct {
            TTL      int
            Hosts    []struct { IP, Hostname string }
            LossPct  string
            Sent     int
            Recv     int
            Avg      string
            Best     string
            Worst    string
            StdDev   string
        }
    }
}
```

**Execution:**
- Uses `trippy --mode json` for structured output
- Default: 5 cycles (15 when triggered)
- DNS resolution via Cloudflare

---

### SPEEDTEST

**Purpose:** Download/upload speed measurement

**Library:** `github.com/showwin/speedtest-go`

**Payload:**
```go
type SpeedTestPayload struct {
    TestData  []SpeedTestServer 
    Timestamp time.Time         
}

type SpeedTestServer struct {
    URL, Name, Country, Sponsor string
    Distance  float64
    Latency   time.Duration
    DLSpeed   float64  // bytes/sec
    ULSpeed   float64  // bytes/sec
    Jitter    time.Duration
}
```

**Execution:**
- Auto-selects nearest server if no target specified
- Can target specific server by ID
- Runs ping, download, then upload tests

---

### SYSINFO

**Purpose:** System resource information

**Library:** `github.com/shirou/gopsutil`

**Payload:**
```go
type SysInfoPayload struct {
    Hostname     string
    OS           string
    Platform     string
    KernelVer    string
    Uptime       uint64
    CPUModel     string
    CPUCores     int
    CPUUsage     float64
    MemTotal     uint64
    MemUsed      uint64
    MemPercent   float64
    DiskTotal    uint64
    DiskUsed     uint64
    DiskPercent  float64
}
```

---

### NETINFO

**Purpose:** Network environment information

**Payload:**
```go
type NetInfoPayload struct {
    PublicIP    string
    LocalIP     string
    Gateway     string
    ISP         string
    ASN         string
    Country     string
    City        string
}
```

**Execution:**
- Queries external APIs for public IP info
- Detects local gateway and interface

---

## Disabled Probes

### TRAFFICSIM (trafficsim.go.disabled)

**Purpose:** Continuous UDP traffic between agents for latency/loss monitoring

**Features:**
- Client/Server UDP communication
- Flow statistics tracking
- Automatic reconnection with backoff
- Cycle-based reporting (every N packets)
- Interface selection with scoring algorithm

**Why Disabled:** Complex 2900+ line implementation, requires testing

---

### WEB (web.go.disabled)

**Purpose:** HTTP endpoint monitoring

**Potential Features:**
- Response time measurement
- Status code validation
- Content matching
- TLS certificate checking

**Why Disabled:** Incomplete implementation

---

### RPERF (rperf.go.disabled)

**Purpose:** Bandwidth/throughput testing (iPerf-like)

**Why Disabled:** Compilation issues

---

## Probe Configuration

Probes are configured in the controller and sent to agents:

```go
type Probe struct {
    ID          uint      
    AgentID     uint      
    Type        ProbeType 
    Enabled     bool      
    IntervalSec int       
    TimeoutSec  int       
    Count       int       
    DurationSec int       
    Server      bool      
    Targets     []ProbeTarget 
}

type ProbeTarget struct {
    Target  string // IP/hostname
    AgentID *uint  // For agent-to-agent probes
}
```

---

## Adding a New Probe Type

### 1. Agent Side (probes/)

```go
// probes/myprobe.go
package probes

type MyProbePayload struct {
    // Fields...
}

func MyProbe(probe *Probe) (MyProbePayload, error) {
    // Implementation
    return result, nil
}
```

### 2. Agent Worker (workers/probes.go)

```go
func handleMyProbe(probe probes.Probe, dataChan chan probes.ProbeData) {
    result, err := probes.MyProbe(&probe)
    if err != nil {
        return
    }
    
    payload, _ := json.Marshal(result)
    dataChan <- probes.ProbeData{
        ProbeID: probe.ID,
        Type:    probes.ProbeType_MYPROBE,
        Payload: payload,
        Target:  probe.Targets[0].Target,
    }
}
```

### 3. Add to Worker Switch (workers/probes.go)

```go
case probes.ProbeType_MYPROBE:
    go handleMyProbe(probe, dataChan)
```

### 4. Controller Handler (internal/probe/)

```go
// myprobe.go
func initMyProbe(db *sql.DB) {
    Register(NewHandler[MyProbePayload](
        TypeMyProbe,
        nil, // validation func
        func(ctx context.Context, data ProbeData, p MyProbePayload) error {
            return SaveRecordCH(ctx, db, data, string(TypeMyProbe), p)
        },
    ))
}
```

### 5. Register in InitWorkers (registry.go)

```go
func InitWorkers(ch *sql.DB) {
    // ...existing handlers
    initMyProbe(ch)
}
```

---

## Environment Variables

### Agent

| Variable | Description |
|----------|-------------|
| `API_URL` | Controller API URL for login |
| `WS_URL` | Controller WebSocket URL |
| `WORKSPACE_ID` | Workspace ID |
| `AGENT_ID` | Agent ID |
| `AGENT_PIN` | Bootstrap PIN (first run) |
| `AGENT_PSK` | Pre-shared key (subsequent runs) |

### Controller

| Variable | Description |
|----------|-------------|
| `CLICKHOUSE_HOST` | ClickHouse server |
| `CLICKHOUSE_PORT` | ClickHouse port (9000) |
| `CLICKHOUSE_USER` | ClickHouse user |
| `CLICKHOUSE_PASSWORD` | ClickHouse password |

---

## Troubleshooting

### Agent Won't Connect

```bash
# Check WebSocket URL format
WS_URL=ws://controller:8080/ws  # Not http://

# Verify PSK is valid
cat agent_auth.json
```

### Probes Not Running

```bash
# Check agent logs for probe fetch
DEBUG=true ./netwatcher-agent

# Verify probes are assigned to this agent in panel
```

### Missing Probe Data in ClickHouse

```sql
SELECT * FROM probe_data 
WHERE agent_id = 123 
ORDER BY created_at DESC 
LIMIT 10;
```
