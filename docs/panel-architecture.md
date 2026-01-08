# Panel Architecture

The NetWatcher panel is a Vue 3 SPA that visualizes probe data from agents.

---

## Data Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Panel     │────▶│ Controller  │────▶│ ClickHouse  │
│   (Vue 3)   │◀────│    (Go)     │◀────│  /Postgres  │
└─────────────┘     └─────────────┘     └─────────────┘
     │                     │
     │  REST/WebSocket     │  SQL
     ▼                     ▼
 apiService.ts        probe.go
```

---

## Target Types

Probes can target three kinds of endpoints:

| Kind | Detection | Example | Panel Key |
|------|-----------|---------|-----------|
| **host** | `target` has value, no `agent_id` | `"8.8.8.8"` | `host|8.8.8.8` |
| **agent** | `agent_id` has value, no `target` | `agent_id: 101` | `agent|101` |
| **local** | No targets array | Self-monitoring | `local|42` |

### Controller IP Resolution

For agent-type targets, the controller fills the IP at fetch time:

```go
// controller/internal/probe/probe.go - ListForAgent()
if t.Target == "" && t.AgentID != nil {
    ip, _ := getPublicIP(ctx, db, ch, *t.AgentID)
    t.Target = ip  // Filled dynamically, not stored
}
```

Resolution priority:
1. `agent.PublicIPOverride` (manual override)
2. Latest NETINFO from ClickHouse

---

## Key Views

### Workspace.vue

Displays agent grid with:
- Online/offline status (based on `updated_at` < 1 min)
- Net info (ISP, public IP) from `ProbeService.netInfo()`
- Quick stats (total, online, offline counts)

### Probe.vue

Displays probe data with:
- Time range selector (default: last 3 hours)
- Type-specific graphs (Ping, MTR, TrafficSim)
- Agent-to-agent bidirectional view when `isAgentProbe = true`

**Load sequence:**
```typescript
1. Load workspace, agent metadata
2. Load probe config: ProbeService.get()
3. Find related probes: findProbesByInitialTarget()
4. Load time-series: ProbeDataService.byProbe()
5. Bucket by type: pingData[], mtrData[], etc.
```

---

## Probe Grouping

`panel/src/utils/probeGrouping.ts` provides functions for organizing probes:

| Function | Purpose |
|----------|---------|
| `groupProbesByTarget()` | Groups all probes by target (host/agent/local) |
| `findMatchingProbesByProbeId()` | Find probes sharing same target |
| `findProbesByInitialTarget()` | Find probes with matching first target |
| `canonicalTargetKey()` | Creates stable key like `"agent:101"` |

### Canonical Key Format

```typescript
function canonicalTargetKey(probe: Probe): string | null {
    const t = probe.targets?.[0];
    if (t?.agent_id != null) return `agent:${t.agent_id}`;
    if (t?.target) return `host:${t.target.toLowerCase()}`;
    return null;
}
```

---

## API Services

`panel/src/services/apiService.ts` wraps all controller endpoints:

| Service | Key Methods |
|---------|-------------|
| `AuthService` | `login()`, `register()`, `health()` |
| `WorkspaceService` | `get()`, `list()`, `create()`, `update()`, `remove()`, `listMembers()`, `addMember()`, `removeMember()` |
| `AgentService` | `get()`, `list()`, `create()`, `update()`, `remove()`, `heartbeat()`, `issuePin()` |
| `ProbeService` | `get()`, `list()`, `create()`, `update()`, `remove()`, `netInfo()`, `sysInfo()` |
| `ProbeDataService` | `find()`, `byProbe()`, `byAgent()`, `latest()`, `similar()` |
| `AgentBootstrap` | `authenticate()` (PSK/PIN auth for agent binary) |

All list endpoints return `{ data: [...] }` wrapper.

---

## Bidirectional View

When a probe targets another agent (`state.isAgentProbe = true`):

```
┌─────────────────────────────────────────────────────┐
│  Agent-to-Agent Monitoring Comparison               │
├─────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐           │
│  │ Agent A → B     │  │ Agent C → B     │   Tabs    │
│  └─────────────────┘  └─────────────────┘           │
├─────────────────────────────────────────────────────┤
│  Latency Graph (source → target)                    │
│  TrafficSim Graph                                   │
│  MTR Accordion with NetworkMap                      │
└─────────────────────────────────────────────────────┘
```

Each tab shows data from one agent's perspective to the target.

---

## Components

| Component | File | Purpose |
|-----------|------|---------|
| `LatencyGraph` | `PingGraph.vue` | Ping RTT visualization with P95 stats |
| `TrafficSimGraph` | `TrafficSimGraph.vue` | Traffic simulation metrics |
| `NetworkMap` | `NetworkMap.vue` | MTR hop visualization |
| `AgentCard` | `AgentCard.vue` | Agent status card |

### Graph Component Props

Both `PingGraph.vue` and `TrafficSimGraph.vue` accept:

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `pingResults` / `trafficResults` | Array | required | Time-series data from probe |
| `intervalSec` | Number | 60 | Probe's scheduling interval for gap detection |

**Gap Detection:** Graphs calculate gap threshold as `1.5 × intervalSec + 30s` to avoid false "Data Gap" annotations when probe interval is longer than the default 90s threshold.

---

## State Management

Views use Vue 3 `reactive()` for local state. No global store (Vuex/Pinia) currently.

```typescript
const state = reactive({
    workspace: {} as Workspace,
    agent: {} as Agent,
    probe: {} as Probe,
    pingData: [] as ProbeData[],
    mtrData: [] as ProbeData[],
    // ...
});
```

---

## Types

Key types in `panel/src/types.ts`:

```typescript
interface Probe {
    id: number;
    agent_id: number;
    type: ProbeType;  // PING, MTR, TRAFFICSIM, etc.
    targets: Target[];
    enabled: boolean;
    interval_sec: number;
}

interface Target {
    id: number;
    target?: string;      // Host/IP (filled by controller)
    agent_id?: number;    // Target agent ID
}

interface ProbeData {
    id: number;
    probe_id: number;
    agent_id: number;
    type: string;
    payload: any;  // Type-specific data
    created_at: string;
}
```

---

## View Inventory

All views in `panel/src/views/`:

### Auth Views
| View | Purpose |
|------|---------|
| `Login.vue` | User authentication with session management |
| `Register.vue` | New user registration with validation |
| `Reset.vue` | Password reset flow |

### Workspace Views
| View | Purpose |
|------|---------|
| `Workspace.vue` | Agent grid with online/offline status |
| `NewWorkspace.vue` | Create new workspace |
| `EditWorkspace.vue` | Update workspace name/description |
| `Members.vue` | List workspace members |
| `InviteMember.vue` | Invite new member |
| `EditMember.vue` | Change member role |
| `RemoveMember.vue` | Remove member from workspace |

### Agent Views
| View | Purpose |
|------|---------|
| `Agent.vue` | Comprehensive agent dashboard (1588 lines) |
| `NewAgent.vue` | Create new agent |
| `EditAgent.vue` | Update agent name/location/IP override |
| `DeleteAgent.vue` | Delete agent with confirmation |
| `DeactivateAgent.vue` | Temporarily disable agent |
| `ProbesEdit.vue` | Manage agent probes |
| `Speedtests.vue` | Speedtest history |
| `NewSpeedtest.vue` | Trigger manual speedtest |

### Probe Views
| View | Purpose |
|------|---------|
| `Probe.vue` | Probe data visualization (733 lines) |
| `NewProbe.vue` | Create new probe (1349 lines) |
| `DeleteProbe.vue` | Delete probe with confirmation |

### Root Views
| View | Purpose |
|------|---------|
| `Root.vue` | Layout wrapper with navigation |
| `HomeView.vue` | Landing/dashboard |
| `Workspaces.vue` | Workspace list |
| `BasicView.vue` | Minimal template |

---

## Code Patterns

### Async Data Loading
Views follow a consistent async/await pattern:

```typescript
onMounted(async () => {
  const id = router.currentRoute.value.params["wID"] as string;
  if (!id) {
    state.error = "Missing workspace ID";
    return;
  }

  try {
    state.loading = true;
    const data = await WorkspaceService.get(id);
    state.workspace = data as Workspace;
    state.ready = true;
  } catch (err) {
    console.error("Failed to load:", err);
    state.error = "Failed to load data";
  } finally {
    state.loading = false;
  }
});
```

### Error Handling
Views display errors consistently:

```html
<div v-if="state.error" class="alert alert-danger">
  <i class="bi bi-exclamation-circle me-2"></i>{{ state.error }}
</div>
```

### Delete Confirmations
Delete views use danger zone pattern:

```html
<div class="card border-danger">
  <div class="card-header bg-danger text-white">
    <h5><i class="bi bi-exclamation-triangle me-2"></i>Danger Zone</h5>
  </div>
  <div class="card-body">
    <div class="alert alert-warning">Warning: This cannot be undone.</div>
    <!-- Confirmation content -->
  </div>
</div>
```

---

## Permissions

The panel enforces role-based access control at multiple levels.

### User Role (from API)

`GET /workspaces/{id}` includes `my_role` field:

```json
{
  "id": 1,
  "name": "Production",
  "my_role": "ADMIN"
}
```

### Permission Composable

`panel/src/composables/usePermissions.ts` provides reactive permission flags:

```typescript
const permissions = computed(() => usePermissions(state.workspace.my_role));

// Usage in template
permissions.canView.value   // Any member
permissions.canEdit.value   // USER+
permissions.canManage.value // ADMIN+
permissions.canOwn.value    // OWNER

// Helper function
hasMinimumRole(userRole, requiredRole)
```

### UI Guards

Views conditionally render action buttons:

```vue
<button v-if="permissions.canEdit.value">Edit</button>
<button v-if="permissions.canManage.value">Delete</button>
<span v-else class="text-muted">View only</span>
```

**Guarded Views:**
| View | Guarded Elements | Required Role |
|------|------------------|---------------|
| `Workspace.vue` | Edit button, Create Agent | ADMIN, USER |
| `Agent.vue` | Edit Probes, Add Probe | USER |
| `Members.vue` | Invite, Edit, Remove | ADMIN |

### Route Guards

`router/index.ts` uses `meta.requiresRole` for navigation protection:

```typescript
{
  path: 'agents/new',
  name: 'agentNew',
  component: NewAgent,
  meta: { requiresRole: 'USER' }
}
```

The `beforeEach` guard checks roles and redirects to `/403` if unauthorized.

### Protected Routes

| Route | Min Role |
|-------|----------|
| Workspace Edit | ADMIN |
| Member Invite/Edit/Remove | ADMIN |
| Agent New/Edit | USER |
| Agent Delete | ADMIN |
| Probe New/Edit | USER |
| Probe Delete | ADMIN |

See [Permissions](./permissions.md) for complete documentation.

