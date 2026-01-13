# Workspace Network Map

The Workspace Network Map provides a visual "map of the internet" showing the network topology between your agents, intermediate hops, and destination targets. This feature aggregates data from MTR, PING, and TrafficSim probes to provide a unified view of network health across your workspace.

## Features

- **Topology Visualization**: D3-based force-directed or hierarchical network graph
- **Real-time Updates**: WebSocket-powered live updates when metrics change
- **Health Coloring**: Nodes and edges colored by latency and packet loss thresholds
- **Aggregated Data**: MTR hop data, PING latency, and TrafficSim metrics combined into a single view
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
  "generated_at": "2026-01-12T19:30:00Z",
  "workspace_id": 1
}
```

## WebSocket Events

The panel subscribes to workspace-level updates via WebSocket. When network topology changes significantly, the server broadcasts a `network_map_update` event:

```json
{
  "event": "network_map_update",
  "data": {
    "workspace_id": 1,
    "nodes": [...],
    "edges": [...],
    "generated_at": "2026-01-12T19:30:30Z"
  }
}
```

## Aggregation Logic

The backend aggregates data from multiple sources:

1. **MTR Data**: Extracts hop-by-hop routes, deduplicates nodes by IP, averages metrics across paths
2. **PING Data**: Overlays point-to-point latency and packet loss onto direct edges
3. **TrafficSim Data**: Adds RTT and packet loss from traffic simulation probes

Nodes that appear in multiple routes (same IP at same hop number) are merged with their metrics averaged.

## Layout Modes

- **Hierarchical** (default): Agents on left, destinations on right, hops positioned by hop number
- **Force**: D3 force-directed simulation for organic clustering

Use the layout toggle button in the map controls to switch between modes.
