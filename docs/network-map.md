# Workspace Network Map

The Workspace Network Map provides a visual "map of the internet" showing the network topology between your agents, intermediate hops, and destination targets. This feature aggregates data from MTR, PING, and TrafficSim probes to provide a unified view of network health across your workspace.

## Features

- **Topology Visualization**: D3-based force-directed or hierarchical network graph
- **Real-time Updates**: WebSocket-powered live updates when metrics change
- **Health Coloring**: Nodes and edges colored by latency and packet loss thresholds
- **Aggregated Data**: MTR hop data, PING latency, and TrafficSim metrics combined into a single view
- **Agent Context**: Reverse probes clearly show which agent owns the probe and which agent is the target
- **Interactive**: Click nodes for detailed metrics, zoom/pan, switch layouts

## Accessing the Network Map

1. Navigate to a Workspace (`/workspaces/{id}`)
2. Click the **"Network Map"** toggle button (next to "Agents Grid")
3. The map will load with aggregated topology data from the last 15 minutes

## Node Types

| Type | Description | Color |
|------|-------------|-------|
| **Agent** | Your deployed monitoring agents (starting points) | Blue (online) / Gray (offline) |
| **Hop** | Intermediate network hops from MTR traces | Green → Red (health gradient) |
| **Destination** | Probe targets (endpoints being monitored) | Purple |

## Health Color Scale

Nodes and edges are colored based on combined latency and packet loss:

- **Green** (Excellent): < 20ms latency, < 1% packet loss
- **Yellow-Green** (Good): 20-40ms latency, 1-5% packet loss  
- **Yellow** (Fair): 40-80ms latency, 5-10% packet loss
- **Orange** (Poor): 80-120ms latency, 10-20% packet loss
- **Red** (Critical): > 120ms latency, > 20% packet loss

## API Reference

### GET `/api/panel/workspaces/{wID}/network-map`

Returns aggregated network topology for the workspace.

**Query Parameters:**
- `lookback` (int, optional): Minutes of data to aggregate. Default: `15`

**Response:**
```json
{
  "nodes": [
    {
      "id": "agent:1",
      "type": "agent",
      "label": "NYC-Agent-01",
      "agent_id": 1,
      "ip": "203.0.113.10",
      "avg_latency": 0,
      "packet_loss": 0,
      "path_count": 5,
      "is_online": true
    },
    {
      "id": "192.168.1.1",
      "type": "hop",
      "label": "1",
      "ip": "192.168.1.1",
      "hostname": "router.local",
      "hop_number": 1,
      "avg_latency": 2.5,
      "packet_loss": 0,
      "path_count": 5
    }
  ],
  "edges": [
    {
      "id": "agent:1->192.168.1.1",
      "source": "agent:1",
      "target": "192.168.1.1",
      "avg_latency": 2.5,
      "packet_loss": 0,
      "path_count": 5
    }
  ],
  "destinations": [
    {
      "target": "google.com",
      "hostname": "google.com",
      "hop_count": 8,
      "avg_latency": 25.5,
      "packet_loss": 0,
      "status": "healthy",
      "agent_count": 3,
      "probe_types": ["MTR", "PING"],
      "endpoints": [
        {
          "ip": "142.250.80.46",
          "agent_id": 1,
          "agent_name": "NYC-Agent-01"
        }
      ],
      "last_updated": "2026-01-12T19:30:00Z"
    },
    {
      "target": "agent:2",
      "hostname": "LAX-Agent-02",
      "hop_count": 12,
      "avg_latency": 65.2,
      "packet_loss": 0.5,
      "status": "healthy",
      "agent_count": 1,
      "probe_types": ["MTR"],
      "endpoints": [
        {
          "ip": "198.51.100.25",
          "agent_id": 1,
          "agent_name": "NYC-Agent-01",
          "target_agent_id": 2,
          "target_agent_name": "LAX-Agent-02"
        }
      ]
    }
  ],
  "generated_at": "2026-01-12T19:30:00Z",
  "workspace_id": 1
}
```

### Destination Summary Fields

| Field | Type | Description |
|-------|------|-------------|
| `target` | string | Destination identifier (hostname, IP, or "agent:{id}") |
| `hostname` | string | Human-readable name |
| `hop_count` | int | Average number of hops from agents |
| `avg_latency` | float | Average latency in milliseconds |
| `packet_loss` | float | Average packet loss percentage |
| `status` | string | "healthy", "degraded", or "critical" |
| `agent_count` | int | Number of agents testing this target |
| `probe_types` | string[] | Types of probes: MTR, PING, TRAFFICSIM |
| `endpoints` | EndpointInfo[] | Endpoint IPs with agent context |
| `endpoint_ips` | string[] | **Deprecated**: Legacy array of IPs only |
| `last_updated` | string | ISO timestamp |

### EndpointInfo Fields

| Field | Type | Description |
|-------|------|-------------|
| `ip` | string | The endpoint IP address |
| `agent_id` | number | Probe owner agent ID (for reverse probes) |
| `agent_name` | string | Probe owner agent name |
| `target_agent_id` | number | Target agent ID (if agent-to-agent probe) |
| `target_agent_name` | string | Target agent name |

## Reverse Probe Identification

The network map now properly identifies **reverse probes** - probes owned by one agent but executed by another:

- **Forward Probe**: Agent A owns probe → Agent A runs probe → Targets Agent B
- **Reverse Probe**: Agent A owns probe → Agent B runs probe → Targets Agent A (or another target)

In the endpoints display, reverse probes show:
- `agent_name`: The agent that owns the probe
- `target_agent_name`: The agent being targeted (if applicable)

Example: If NYC-Agent-01 creates a bidirectional probe to LAX-Agent-02, the endpoint will show:
```json
{
  "ip": "198.51.100.25",
  "agent_id": 1,
  "agent_name": "NYC-Agent-01",
  "target_agent_id": 2,
  "target_agent_name": "LAX-Agent-02"
}
```

This makes it clear that NYC-Agent-01 initiated the probe, even though LAX-Agent-02 might be the one reporting the results.

## WebSocket Events

The panel subscribes to workspace-level updates via WebSocket. When network topology changes significantly, the server broadcasts a `network_map_update` event:

```json
{
  "event": "network_map_update",
  "data": {
    "workspace_id": 1,
    "nodes": [...],
    "edges": [...],
    "destinations": [...],
    "generated_at": "2026-01-12T19:30:30Z"
  }
}
```

Note: The `destinations` array contains the destination summaries with endpoint information, including agent context for reverse probes.

## Aggregation Logic

The backend aggregates data from multiple sources:

1. **MTR Data**: Extracts hop-by-hop routes, deduplicates nodes by IP, averages metrics across paths
2. **PING Data**: Overlays point-to-point latency and packet loss onto direct edges
3. **TrafficSim Data**: Adds RTT and packet loss from traffic simulation probes

### Agent Context Tracking

All probe types now capture `probe_agent_id` (probe owner) alongside `agent_id` (probe runner):

- **MTR**: Last hop IPs include probe owner information
- **PING**: Tracks all unique probe agents contributing to aggregated metrics
- **TrafficSim**: Tracks probe agents for return-path identification

This ensures accurate attribution even when:
- Agents have dynamic IPs
- Reverse probes are used for bidirectional monitoring
- Multiple agents test the same destination

Nodes that appear in multiple routes (same IP at same hop number) are merged with their metrics averaged.

## Layout Modes

- **Hierarchical** (default): Agents on left, destinations on right, hops positioned by hop number
- **Force**: D3 force-directed simulation for organic clustering
- **Concentric**: Agents and destinations on sides, hops in concentric rings

Use the layout toggle button in the map controls to switch between modes.
