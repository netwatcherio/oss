# NetWatcher Permissions System

Role-based access control (RBAC) for workspace-scoped operations. Permissions are enforced on both controller (API) and panel (UI) sides.

---

## Role Hierarchy

```
OWNER (4) > ADMIN (3) > USER (2) > VIEWER (1)
```

| Role | Code Value | Description |
|------|------------|-------------|
| **OWNER** | `OWNER` | Full control, workspace deletion, ownership transfer |
| **ADMIN** | `ADMIN` | Manage agents, probes, and members |
| **USER** | `READ_WRITE` | Create and edit agents/probes |
| **VIEWER** | `READ_ONLY` | View-only access |

---

## Permission Matrix

### Workspace Operations
| Action | VIEWER | USER | ADMIN | OWNER |
|--------|:------:|:----:|:-----:|:-----:|
| View workspace | ✅ | ✅ | ✅ | ✅ |
| Edit name/description | ❌ | ❌ | ✅ | ✅ |
| Delete workspace | ❌ | ❌ | ❌ | ✅ |
| Transfer ownership | ❌ | ❌ | ❌ | ✅ |

### Member Operations
| Action | VIEWER | USER | ADMIN | OWNER |
|--------|:------:|:----:|:-----:|:-----:|
| View members | ✅ | ✅ | ✅ | ✅ |
| Invite members | ❌ | ❌ | ✅ | ✅ |
| Change role | ❌ | ❌ | ✅* | ✅ |
| Remove members | ❌ | ❌ | ✅* | ✅ |

*ADMIN cannot modify OWNER or other ADMINs

### Agent Operations
| Action | VIEWER | USER | ADMIN | OWNER |
|--------|:------:|:----:|:-----:|:-----:|
| View agents | ✅ | ✅ | ✅ | ✅ |
| Create agents | ❌ | ✅ | ✅ | ✅ |
| Edit agents | ❌ | ✅ | ✅ | ✅ |
| Delete agents | ❌ | ❌ | ✅ | ✅ |
| Issue bootstrap PIN | ❌ | ✅ | ✅ | ✅ |

### Probe Operations
| Action | VIEWER | USER | ADMIN | OWNER |
|--------|:------:|:----:|:-----:|:-----:|
| View probes/data | ✅ | ✅ | ✅ | ✅ |
| Create probes | ❌ | ✅ | ✅ | ✅ |
| Edit probes | ❌ | ✅ | ✅ | ✅ |
| Delete probes | ❌ | ❌ | ✅ | ✅ |

---

## API Endpoints & Required Roles

### Workspaces
| Endpoint | Method | Min Role |
|----------|--------|----------|
| `/workspaces/{id}` | GET | VIEWER |
| `/workspaces/{id}` | PATCH | ADMIN |
| `/workspaces/{id}` | DELETE | OWNER |

### Members
| Endpoint | Method | Min Role |
|----------|--------|----------|
| `/workspaces/{id}/members` | GET | VIEWER |
| `/workspaces/{id}/members` | POST | ADMIN |
| `/workspaces/{id}/members/{mid}` | PATCH | ADMIN |
| `/workspaces/{id}/members/{mid}` | DELETE | ADMIN |

### Agents
| Endpoint | Method | Min Role |
|----------|--------|----------|
| `/workspaces/{id}/agents` | GET | VIEWER |
| `/workspaces/{id}/agents` | POST | USER |
| `/workspaces/{id}/agents/{aid}` | GET | VIEWER |
| `/workspaces/{id}/agents/{aid}` | PATCH | USER |
| `/workspaces/{id}/agents/{aid}` | DELETE | ADMIN |
| `/workspaces/{id}/agents/{aid}/issue-pin` | POST | USER |

### Probes
| Endpoint | Method | Min Role |
|----------|--------|----------|
| `/.../probes` | GET | VIEWER |
| `/.../probes` | POST | USER |
| `/.../probes/{pid}` | PATCH | USER |
| `/.../probes/{pid}` | DELETE | ADMIN |

---

## Implementation Details

### Controller Middleware

**File:** `controller/web/permissions.go`

```go
// Permission level constants
const (
    CanView   = workspace.RoleReadOnly   // VIEWER
    CanEdit   = workspace.RoleReadWrite  // USER
    CanManage = workspace.RoleAdmin      // ADMIN
    CanOwn    = workspace.RoleOwner      // OWNER
)

// Middleware functions
RequireRole(store, minRole)        // Check minimum role
RequireWorkspaceAccess(store)      // Check any membership
```

### API Response Enhancement

`GET /workspaces/{id}` returns user's role:

```json
{
  "id": 1,
  "name": "Production Network",
  "my_role": "ADMIN"
}
```

### Panel Composable

**File:** `panel/src/composables/usePermissions.ts`

```typescript
const { canView, canEdit, canManage, canOwn } = usePermissions(role);

// Granular permissions
canCreateAgent, canDeleteAgent
canCreateProbe, canDeleteProbe
canInviteMembers, canEditMembers
canDeleteWorkspace, canTransferOwnership

// Helper function
hasMinimumRole(userRole, requiredRole)
```

### Panel Route Guards

**File:** `panel/src/router/index.ts`

Protected routes use `meta.requiresRole`:

| Route Pattern | Required Role |
|---------------|---------------|
| `/workspaces/:wID/edit` | ADMIN |
| `/workspaces/:wID/members/invite` | ADMIN |
| `/workspaces/:wID/members/edit/:id` | ADMIN |
| `/workspaces/:wID/members/remove/:id` | ADMIN |
| `/workspaces/:wID/agents/new` | USER |
| `/workspaces/:wID/agents/:aID/edit` | USER |
| `/workspaces/:wID/agents/:aID/delete` | ADMIN |
| `/workspaces/:wID/agents/:aID/probes/new` | USER |
| `/workspaces/:wID/agents/:aID/probes/edit` | USER |
| `/workspaces/:wID/agents/:aID/probes/:pID/delete` | ADMIN |

Unauthorized access redirects to `/403` (Forbidden page).

### UI Permission Guards

Views use `usePermissions` composable:

```vue
<script setup>
const permissions = computed(() => usePermissions(state.workspace.my_role));
</script>

<template>
  <button v-if="permissions.canEdit.value">Edit</button>
  <button v-if="permissions.canManage.value">Delete</button>
  <span v-if="!permissions.canEdit.value">View only</span>
</template>
```

**Guarded Views:**
- `Workspace.vue` - Edit button (ADMIN+), Create Agent (USER+)
- `Agent.vue` - Edit Probes (USER+), Add Probe (USER+)
- `Members.vue` - Invite/Edit/Remove (ADMIN+)

---

## Database Schema

```sql
CREATE TABLE workspace_members (
    id SERIAL PRIMARY KEY,
    workspace_id INT NOT NULL REFERENCES workspaces(id),
    user_id INT DEFAULT 0,
    email VARCHAR(320) DEFAULT '',
    role VARCHAR(20) NOT NULL,  -- READ_ONLY, READ_WRITE, ADMIN, OWNER
    meta JSONB DEFAULT '{}',
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    deleted_at TIMESTAMP,
    invited_at TIMESTAMP,
    accepted_at TIMESTAMP,
    revoked_at TIMESTAMP
);
```

---

## Error Responses

### 403 Forbidden (API)
```json
{
  "error": "insufficient permissions",
  "required_role": "ADMIN"
}
```

### 403 Forbidden (Panel)
Redirects to `/403` page with "Access Denied" message.

---

## See Also

- [API Reference](./api-reference.md)
- [Panel Architecture](./panel-architecture.md)
- [Data Models](./data-models.md)
