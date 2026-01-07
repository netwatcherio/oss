# NetWatcher API Reference

## Base URL

```
https://api.netwatcher.io
```

## Authentication

### User Authentication (JWT)

All protected routes require a valid JWT token in the `Authorization` header:

```
Authorization: Bearer <jwt_token>
```

### Agent Authentication (PSK/PIN)

Agents authenticate via:
- **PIN** – One-time bootstrap credential
- **PSK** – Persistent pre-shared key (returned after PIN bootstrap)

---

## Response Format

### List Endpoints

All list endpoints return data wrapped in a consistent format:

```json
{
  "data": [ /* array of items */ ],
  "total": 100,    // optional: total count (for paginated endpoints)
  "limit": 50,     // optional: requested limit
  "offset": 0      // optional: requested offset
}
```

### Single Item Endpoints

Single item endpoints return the object directly:

```json
{
  "id": 1,
  "name": "Example",
  // ... other fields
}
```

### Error Responses

All errors return:

```json
{
  "error": "error message here"
}

---

## Auth Endpoints

### `POST /auth/register`

Register a new user account.

**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "securepassword",
  "name": "John Doe",
  "role": "USER",
  "labels": {},
  "metadata": {}
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "data": {
    "id": 1,
    "email": "user@example.com",
    "name": "John Doe",
    "role": "USER",
    "verified": false,
    "createdAt": "2024-01-01T00:00:00Z",
    "updatedAt": "2024-01-01T00:00:00Z"
  }
}
```

---

### `POST /auth/login`

Authenticate an existing user.

**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "securepassword"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "data": { /* User object */ }
}
```

---

## Agent Endpoints

### `POST /agent`

Agent login/bootstrap endpoint. Supports both PIN bootstrap and PSK authentication.

**Request Body (PIN Bootstrap):**
```json
{
  "workspace_id": 1,
  "agent_id": 10,
  "pin": "123456789"
}
```

**Response (Bootstrap Success):**
```json
{
  "status": "success",
  "psk": "generated-psk-token",
  "agent": { /* Agent object */ }
}
```

**Request Body (PSK Auth):**
```json
{
  "workspace_id": 1,
  "agent_id": 10,
  "psk": "existing-psk-token"
}
```

**Response (PSK Success):**
```json
{
  "status": "ok",
  "agent": { /* Agent object */ }
}
```

---

## Workspace Endpoints

### `GET /workspaces`

List all workspaces for the authenticated user.

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `q` | string | - | Search query |
| `limit` | int | 50 | Max results (1-200) |
| `offset` | int | 0 | Pagination offset |

**Response:**
```json
{
  "data": [
    {
      "id": 1,
      "name": "Production",
      "ownerId": 1,
      "description": "Production network monitoring",
      "settings": {},
      "createdAt": "2024-01-01T00:00:00Z",
      "updatedAt": "2024-01-01T00:00:00Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

---

### `POST /workspaces`

Create a new workspace.

**Request Body:**
```json
{
  "name": "my-workspace",
  "displayName": "My Workspace",
  "settings": {}
}
```

---

### `GET /workspaces/{id}`

Get a specific workspace by ID.

**Required Role:** Any workspace member

**Response includes user's role:**
```json
{
  "id": 1,
  "name": "Production",
  "description": "...",
  "my_role": "ADMIN"
}
```

---

### `PATCH /workspaces/{id}`

Update workspace properties.

**Required Role:** `ADMIN`

**Request Body:**
```json
{
  "name": "new-name",
  "description": "Updated description",
  "settings": { "key": "value" }
}
```

---

### `DELETE /workspaces/{id}`

Delete a workspace (soft delete).

**Required Role:** `OWNER`

---

## Workspace Member Endpoints

### `GET /workspaces/{id}/members`

List all members of a workspace.

**Required Role:** Any workspace member

---

### `POST /workspaces/{id}/members`

Invite a new member to the workspace.

**Required Role:** `ADMIN`

**Request Body:**
```json
{
  "userId": 0,
  "email": "newuser@example.com",
  "role": "USER",
  "meta": {}
}
```

**Roles:** `USER`, `ADMIN`, `OWNER`

---

### `PATCH /workspaces/{id}/members/{memberId}`

Update a member's role.

**Required Role:** `ADMIN`

**Request Body:**
```json
{
  "role": "ADMIN"
}
```

---

### `DELETE /workspaces/{id}/members/{memberId}`

Remove a member from the workspace.

**Required Role:** `ADMIN`

---

### `POST /workspaces/{id}/accept-invite`

Accept a pending workspace invitation.

**Request Body:**
```json
{
  "email": "user@example.com"
}
```

---

### `POST /workspaces/{id}/transfer-ownership`

Transfer workspace ownership to another user.

**Required Role:** `OWNER`

**Request Body:**
```json
{
  "newOwnerUserId": 5
}
```

---

## Agent Management Endpoints

### `GET /workspaces/{id}/agents`

List all agents in a workspace.

**Required Role:** Any workspace member

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `limit` | int | 50 | Max results (1-200) |
| `offset` | int | 0 | Pagination offset |

**Response:**
```json
{
  "data": [
    {
      "id": 10,
      "workspace_id": 1,
      "name": "Office Router",
      "description": "Main office network monitor",
      "location": "Vancouver, BC",
      "public_ip_override": "",
      "version": "1.2.0",
      "last_seen_at": "2024-01-01T12:00:00Z",
      "initialized": true,
      "labels": {},
      "metadata": {}
    }
  ],
  "total": 1
}
```

---

### `POST /workspaces/{id}/agents`

Create a new agent with bootstrap PIN.

**Required Role:** `USER`

**Request Body:**
```json
{
  "name": "New Agent",
  "description": "Description here",
  "location": "Seattle, WA",
  "public_ip_override": "",
  "version": "",
  "pinLength": 9,
  "pinTTLSeconds": 3600,
  "labels": {},
  "metadata": {}
}
```

**Response:**
```json
{
  "agent": { /* Agent object */ },
  "pin": "123456789"
}
```

---

### `GET /workspaces/{id}/agents/{agentID}`

Get a specific agent.

**Required Role:** Any workspace member

---

### `PATCH /workspaces/{id}/agents/{agentID}`

Update agent properties.

**Required Role:** `USER`

**Request Body:**
```json
{
  "name": "Updated Name",
  "description": "Updated description",
  "location": "New Location",
  "labels": { "env": "production" }
}
```

---

### `DELETE /workspaces/{id}/agents/{agentID}`

Delete an agent.

**Required Role:** `ADMIN`

---

### `POST /workspaces/{id}/agents/{agentID}/heartbeat`

Manual heartbeat update.

**Response:**
```json
{
  "ok": true,
  "ts": "2024-01-01T12:00:00Z"
}
```

---

### `POST /workspaces/{id}/agents/{agentID}/issue-pin`

Issue a new bootstrap PIN for an agent.

**Request Body:**
```json
{
  "pinLength": 9,
  "ttlSeconds": 3600
}
```

**Response:**
```json
{
  "pin": "987654321"
}
```

---

### `GET /workspaces/{id}/agents/{agentID}/netinfo`

Get the latest network info for an agent.

---

### `GET /workspaces/{id}/agents/{agentID}/sysinfo`

Get the latest system info for an agent.

---

## Probe Endpoints

### `GET /workspaces/{id}/agents/{agentID}/probes`

List all probes for an agent.

---

### `POST /workspaces/{id}/agents/{agentID}/probes`

Create a new probe.

**Request Body:**
```json
{
  "workspace_id": 1,
  "agent_id": 10,
  "type": "PING",
  "enabled": true,
  "interval_sec": 60,
  "timeout_sec": 10,
  "count": 5,
  "duration_sec": 0,
  "server": false,
  "targets": ["8.8.8.8", "1.1.1.1"],
  "agent_targets": [],
  "labels": {},
  "metadata": {}
}
```

**Probe Types:**
| Type | Description |
|------|-------------|
| `MTR` | Traceroute with per-hop stats |
| `PING` | ICMP ping |
| `SPEEDTEST` | Speed test |
| `SYSINFO` | System information |
| `NETINFO` | Network information |
| `TRAFFICSIM` | Traffic simulation |

---

### `GET /workspaces/{id}/agents/{agentID}/probes/{probeID}`

Get a specific probe.

---

### `PATCH /workspaces/{id}/agents/{agentID}/probes/{probeID}`

Update probe settings.

**Request Body:**
```json
{
  "enabled": false,
  "intervalSec": 120,
  "timeoutSec": 15,
  "labels": { "priority": "high" },
  "replaceTargets": ["8.8.4.4"]
}
```

---

### `DELETE /workspaces/{id}/agents/{agentID}/probes/{probeID}`

Delete a probe.

---

## Probe Data Endpoints

### `GET /workspaces/{id}/probe-data/find`

Flexible query across all probe data.

**Query Parameters:**
| Param | Type | Description |
|-------|------|-------------|
| `type` | string | Filter by probe type (PING, MTR, etc.) |
| `probeId` | uint | Filter by specific probe |
| `agentId` | uint | Filter by reporting agent |
| `probeAgentId` | uint | Filter by probe-owning agent |
| `targetAgent` | uint | Filter by target agent |
| `targetPrefix` | string | Filter by target prefix |
| `triggered` | bool | Filter by triggered status |
| `from` | time | Start timestamp (RFC3339 or Unix) |
| `to` | time | End timestamp |
| `limit` | int | Max results |
| `asc` | bool | Sort ascending (default: false) |

---

### `GET /workspaces/{id}/probe-data/probes/{probeID}/data`

Get time-series data for a specific probe.

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `from` | time | - | Start timestamp |
| `to` | time | - | End timestamp |
| `limit` | int | 0 | Max results |
| `asc` | bool | false | Sort ascending |

---

### `GET /workspaces/{id}/probe-data/latest`

Get the latest probe data by type and agent.

**Query Parameters:**
| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Probe type |
| `agentId` | uint | Yes | Agent ID |
| `probeId` | uint | No | Specific probe ID |

---

### `GET /workspaces/{id}/probe-data/by-target/data`

Get probe data for a specific target.

**Query Parameters:**
| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `target` | string | Yes | Target host/IP |
| `type` | string | No | Filter by probe type |
| `from` | time | No | Start timestamp |
| `to` | time | No | End timestamp |
| `limit` | int | No | Max results |
| `latestOnly` | bool | No | Return only latest |

---

### `GET /workspaces/{id}/probe-data/probes/{probeID}/similar`

Find similar probes (same targets or target agents).

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `sameType` | bool | true | Restrict to same probe type |
| `includeSelf` | bool | false | Include the reference probe |
| `latest` | bool | false | Include latest data points |

---

## WebSocket API

### Connection

```
ws://api.netwatcher.io/ws
```

**Required Headers:**
```
X-Workspace-ID: <workspace_id>
X-Agent-ID: <agent_id>
X-Agent-PSK: <psk_token>
```

### Events (Agent Namespace)

| Event | Direction | Description |
|-------|-----------|-------------|
| `probe_get` | Agent → Controller | Request probe configurations |
| `probe_get` | Controller → Agent | Probe config response |
| `probe_post` | Agent → Controller | Submit probe results |
| `probe_post_ok` | Controller → Agent | Acknowledgment |
| `version` | Agent → Controller | Report agent version |
| `version` | Controller → Agent | Acknowledgment |

### probe_post Payload

```json
{
  "probe_id": 123,
  "triggered": false,
  "triggered_reason": "",
  "created_at": "2024-01-01T12:00:00Z",
  "type": "PING",
  "payload": { /* type-specific data */ },
  "target": "8.8.8.8",
  "target_agent": 0
}
```

---

## Health Check

### `GET /healthz`

Returns service health status.

**Response:**
```json
{
  "ok": true
}
```
