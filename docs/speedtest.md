# Speedtest System

The speedtest system allows users to run on-demand internet speed tests from any agent. Results are stored in ClickHouse and displayed in the panel.

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Panel     │────▶│  Controller │◀───▶│   Agent     │
│  (Vue 3)    │ API │   (Go)      │ WS  │   (Go)      │
└─────────────┘     └─────────────┘     └─────────────┘
                           │
           ┌───────────────┼───────────────┐
           ▼               ▼               ▼
     ┌──────────┐   ┌──────────────┐  ┌───────────┐
     │PostgreSQL│   │  ClickHouse  │  │speedtest- │
     │(queue)   │   │  (results)   │  │go library │
     └──────────┘   └──────────────┘  └───────────┘
```

## Database Tables

### `speedtest_queue` (PostgreSQL)

Stores pending and completed speedtest requests.

| Column | Type | Description |
|--------|------|-------------|
| `id` | SERIAL | Primary key |
| `workspace_id` | uint | FK to workspaces |
| `agent_id` | uint | FK to agents |
| `server_id` | string | speedtest.net server ID |
| `server_name` | string | Display name |
| `status` | enum | `pending`, `running`, `completed`, `failed`, `cancelled`, `expired` |
| `requested_by` | uint | User who requested |
| `requested_at` | timestamp | When queued |
| `expires_at` | timestamp | When item expires if not picked up |
| `started_at` | timestamp | When agent started |
| `completed_at` | timestamp | When finished |
| `error` | text | Error message if failed |

### `agent_speedtest_servers` (PostgreSQL)

Caches available speedtest.net servers per agent (populated on agent connect).

| Column | Type | Description |
|--------|------|-------------|
| `agent_id` | uint | FK to agents |
| `server_id` | string | speedtest.net server ID |
| `name` | string | Server name |
| `sponsor` | string | ISP sponsor |
| `host` | string | Server hostname |
| `country` | string | Country |
| `distance` | float | km from agent |
| `last_seen_at` | timestamp | When agent last reported |

## REST API

### Queue Management

```http
# List queue items for an agent
GET /workspaces/{wID}/agents/{aID}/speedtest-queue
GET /workspaces/{wID}/agents/{aID}/speedtest-queue?status=pending

# Queue a new speedtest
POST /workspaces/{wID}/agents/{aID}/speedtest-queue
Content-Type: application/json
{
  "server_id": "12345",      // optional, empty = auto-select
  "server_name": "Example"   // optional, for display
}

# Cancel a pending item
DELETE /workspaces/{wID}/agents/{aID}/speedtest-queue/{queueID}
```

### Server Cache

```http
# Get cached servers for an agent
GET /workspaces/{wID}/agents/{aID}/speedtest-servers
```

## WebSocket Events

| Event | Direction | Description |
|-------|-----------|-------------|
| `speedtest_servers` | Agent → Controller | Agent sends available servers on connect |
| `speedtest_servers_ok` | Controller → Agent | Acknowledgement |
| `speedtest_queue_get` | Agent → Controller | Agent requests pending queue items |
| `speedtest_queue` | Controller → Agent | Controller returns pending items |
| `speedtest_result` | Agent → Controller | Agent submits completed results |
| `speedtest_result_ok` | Controller → Agent | Acknowledgement |

### `speedtest_servers` Payload

```json
[
  {
    "id": "12345",
    "name": "Server Name",
    "sponsor": "ISP Name",
    "host": "speedtest.example.com",
    "url": "http://speedtest.example.com/speedtest/upload.php",
    "country": "US",
    "lat": "40.7128",
    "lon": "-74.0060",
    "distance": 15.5
  }
]
```

### `speedtest_result` Payload

```json
{
  "queue_id": 123,
  "success": true,
  "error": "",
  "data": { /* speedtest results */ }
}
```

## Agent Flow

1. **On Connect**: Agent fetches speedtest.net servers and sends via `speedtest_servers` event
2. **Every 30 seconds**: Agent emits `speedtest_queue_get` to poll for pending tests
3. **When queue received**: Agent processes items in order:
   - Runs speedtest using specified server (or auto-select)
   - Sends `speedtest_result` with success/failure
4. **Controller**: Marks queue item complete and stores results in ClickHouse

## Expiration & Online Delivery

Queue items have a **15-minute expiration window** to ensure stale requests don't execute unexpectedly.

### Behavior

- **Expiration**: Items not picked up within 15 minutes are marked `expired`
- **Online-Only**: The controller only returns pending items if the agent was seen within the last 2 minutes
- **Cleanup**: Expired items are cleaned up when any agent polls the queue

### Status Transitions

```
pending → running → completed
        → expired (after 15 min if not picked up)
        → failed (if speedtest fails)
        → cancelled (user cancellation)
```

## Usage Example

### Queue a Speedtest (Panel)

```typescript
import { SpeedtestService } from '@/services/apiService';

// Get available servers
const { data: servers } = await SpeedtestService.listServers(workspaceId, agentId);

// Queue a test
await SpeedtestService.queueTest(workspaceId, agentId, {
  server_id: servers[0].server_id,
  server_name: servers[0].name,
});

// Check status
const { data: queue } = await SpeedtestService.listQueue(workspaceId, agentId);
```

## Files

| Component | File |
|-----------|------|
| Queue Model | `controller/internal/speedtest/queue.go` |
| Server Model | `controller/internal/speedtest/servers.go` |
| WebSocket Events | `controller/web/ws.go` |
| REST API | `controller/web/speedtest.go` |
| Agent Queue Runner | `agent/probes/speedtest_queue.go` |
| Agent WS Client | `agent/web/client.go` |
| Panel Service | `panel/src/services/apiService.ts` |
