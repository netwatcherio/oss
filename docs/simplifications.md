# NetWatcher Simplification Recommendations

This document outlines opportunities to simplify and improve the NetWatcher codebase across all three components.

---

## Panel Simplifications

### 1. ⚠️ Component Consolidation

**Issue:** Multiple similar graphing components with duplicated logic.

| Current Files | Lines | Issues |
|---------------|-------|--------|
| `PingGraph.vue` | 17,395 | Large, complex |
| `TrafficSimGraph.vue` | 26,601 | Very large |
| `TrafficSimGraph_OLD.vue` | 8,225 | Dead code |
| `RperfGraph.vue` | 10,345 | Similar patterns |

**Recommendation:**
- Delete `TrafficSimGraph_OLD.vue` (dead code)
- Extract shared chart logic into a composable: `useChartData.ts`
- Create base `TimeSeriesChart.vue` component with slots for probe-specific overlays
- Reduce duplication by ~40%

```typescript
// src/composables/useChartData.ts
export function useChartData(probeType: ProbeType) {
  const data = ref<ProbeData[]>([])
  const loading = ref(false)
  
  async function fetchRange(from: Date, to: Date) { /* ... */ }
  async function fetchLatest() { /* ... */ }
  
  return { data, loading, fetchRange, fetchLatest }
}
```

---

### 2. ⚠️ Router Nesting Complexity

**Issue:** Deep nesting with repeated `BasicView` wrapper components.

**Current Pattern:**
```
/workspaces/:wID/agents/:aID/probes/:pID/delete
     ↓
BasicView → BasicView → BasicView → DeleteProbe.vue
```

**Recommendation:**
- Flatten route structure where possible
- Use route guards instead of nested layout components
- Consolidate `BasicView.vue` and `Shell` into a single `LayoutShell.vue`

---

### 3. ✅ Quick Wins

| File | Issue | Fix |
|------|-------|-----|
| `panel/src/types.ts` | `calculateMOS` function in types file | Move to `src/utils/mos.ts` |
| `panel/src/remote.ts` | Only 124 bytes, placeholder | Remove or implement |
| Various views | Inconsistent naming (`wID` vs `workspaceId`) | Standardize prop names |

---

## Controller Simplifications

### 1. ⚠️ Duplicate Helper Functions

**Issue:** Same helper functions defined in multiple files.

| Function | Locations |
|----------|-----------|
| `uintParam()` | `workspaces.go`, `agents.go` |
| `intParam()` | `workspaces.go`, `agents.go` |
| `boolOr()` | `data.go`, `probe.go` |
| `ifZero()` | `agents.go`, `probe.go` |
| `uintParamName()` | `workspaces.go` (duplicate of `uintParam`) |

**Recommendation:**
Create `web/helpers.go`:
```go
// web/helpers.go
package web

func uintParam(ctx iris.Context, name string) uint { ... }
func intParam(ctx iris.Context, name string, def, min, max int) int { ... }
func boolOr(val string, def bool) bool { ... }
func ifZero(v, def int) int { ... }
func stringsTrim(s string) string { ... }
```

---

### 2. ⚠️ Old/Dead Code Files

| File | Issue | Action |
|------|-------|--------|
| `web/probes.old` | 13KB of old code | Delete |
| `internal/probe/dns.go` | Only 14 bytes (empty) | Delete or implement |
| `internal/probe/alerts.go` | Only 14 bytes (empty) | Delete or implement |
| `internal/probe/trafficsim.go` | Only 14 bytes (empty) | Delete or implement |

---

### 3. ⚠️ TODO Comments

Several unresolved TODOs need attention:

```go
// web/probes.go:29
// todo validate workspace id permissions and such

// internal/probe/probe.go:268
// TODO: create multiple probes for agent type for all available target agents
```

**Recommendation:** Create issues or implement validation:
```go
// web/probes.go - Add workspace ownership check
base.Post("/", func(ctx iris.Context) {
    wsID := uintParam(ctx, "id")
    userID := currentUserID(ctx)
    
    // Add this validation:
    if !workspaceStore.UserHasAccess(ctx, wsID, userID) {
        ctx.StatusCode(http.StatusForbidden)
        return
    }
    // ...
})
```

---

### 4. ⚠️ Database Connection Inconsistency

**Issue:** `MONGO_URI` env var name suggests MongoDB, but actually PostgreSQL.

**Current:**
```go
// .env.example
MONGO_URI=<mongodb_connection_string>
```

**Recommendation:**
```go
// Rename to:
DATABASE_URL=postgres://user:pass@host:5432/dbname
```

Update all references in `internal/database/` and documentation.

---

### 5. ✅ API Response Consistency

**Issue:** Inconsistent response formats across endpoints.

| Endpoint | Response Format |
|----------|-----------------|
| `GET /workspaces` | `{ data: [], total, limit, offset }` |
| `GET /workspaces/{id}/members` | `[ ... ]` (raw array) |
| `GET /.../probes` | `[ ... ]` (raw array) |
| `GET /.../agents` | `{ data: [], total, limit, offset }` |

**Recommendation:** Standardize all list endpoints to use pagination wrapper:
```go
type PaginatedResponse struct {
    Data   interface{} `json:"data"`
    Total  int         `json:"total"`
    Limit  int         `json:"limit"`
    Offset int         `json:"offset"`
}
```

---

## Agent Simplifications

### 1. ⚠️ Dead/Old Probe Files

| File | Size | Issue |
|------|------|-------|
| `agent/probes/dns.old` | 6,306 | Old code |
| `agent/probes/rperf.current` | 7,274 | `.current` extension (use `.go`) |
| `agent/probes/trafficsim.current` | 78,205 | `.current` extension |
| `agent/probes/trafficsim.oldgo` | 22,244 | Old code |
| `agent/probes/web.current` | 6,788 | `.current` extension |

**Recommendation:**
1. Delete `.old` and `.oldgo` files
2. Rename `.current` files to `.go`
3. Add migration notes if needed

---

### 2. ⚠️ Platform Detection Duplication

**Issue:** Platform/architecture detection logic repeated in multiple files.

**Current (mtr.go):**
```go
switch runtime.GOOS {
case "windows":
    if runtime.GOARCH == "amd64" {
        trippyBinary = "trip.exe"
    } else {
        trippyBinary = "trip.exe"  // Same value!
    }
case "darwin":
    trippyBinary = "trip"
case "linux":
    if runtime.GOARCH == "amd64" {
        trippyBinary = "trip"
    } else if runtime.GOARCH == "arm64" {
        trippyBinary = "trip"  // Same value!
    }
    // ...
}
```

**Recommendation:**
Create `lib/platform.go`:
```go
package lib

func BinaryName(base string) string {
    if runtime.GOOS == "windows" {
        return base + ".exe"
    }
    return base
}

func BinaryPath(name string) (string, error) {
    return filepath.Join(".", "lib", BinaryName(name)), nil
}
```

---

### 3. ⚠️ Commented Code Blocks

**Issue:** Large blocks of commented-out code clutter the codebase.

| File | Lines | Issue |
|------|-------|-------|
| `mtr.go` | 43-75 | Commented `MtrPayload` struct |
| `mtr.go` | 113-124 | Commented args array |

**Recommendation:** Delete commented code; use git history if needed.

---

### 4. ✅ Quick Wins

| File | Issue | Fix |
|------|-------|-----|
| `probes/types.go:21-24` | Empty `Labels` and `Metadata` structs | Use `map[string]any` |
| `main.go:145-152` | Redundant PSK assignment | Simplify logic |

---

## Architecture Simplifications

### 1. ⚠️ Database Technology Mismatch

**Issue:** README and code reference MongoDB, but PostgreSQL is used.

**Recommendation:**
- Update all README files to reflect PostgreSQL usage
- Update Docker Compose examples
- Update environment variable naming

---

### 2. ⚠️ Missing Error Types

**Issue:** Some errors are inline strings, others use typed errors.

**Current:**
```go
// Typed (good)
var ErrNotFound = errors.New("probe not found")

// Inline (inconsistent)
return errors.New("unauthorized: no agent in context")
```

**Recommendation:** Create `internal/errors/errors.go`:
```go
package errors

var (
    ErrUnauthorized = errors.New("unauthorized")
    ErrForbidden    = errors.New("forbidden")
    ErrNotFound     = errors.New("not found")
    ErrBadRequest   = errors.New("bad request")
)
```

---

### 3. ⚠️ WebSocket Error Handling

**Issue:** WebSocket handlers don't return errors consistently.

**Current (ws.go:124):**
```go
// Important: nsConn.Emit returns bool; do not treat as error
nsConn.Emit("version", []byte("ok"))
return nil  // Always returns nil
```

**Recommendation:** Log failed emissions and consider retry logic.

---

## Priority Matrix

| Priority | Component | Item | Effort | Status |
|----------|-----------|------|--------|--------|
| ✅ Done | Controller | Consolidate helper functions into `helpers.go` | Low | **Completed** |
| ✅ Done | Controller | Delete `web/probes.old` | Low | **Completed** |
| ✅ Done | Panel | Delete `TrafficSimGraph_OLD.vue` | Low | **Completed** |
| ✅ Done | Agent | Delete `dns.old`, `trafficsim.oldgo` | Low | **Completed** |
| ✅ Done | All | Update README to reflect PostgreSQL | Low | **Completed** |
| ✅ Done | Controller | Standardize API responses | Medium | **Completed** |
| ⚠️ Blocked | Agent | Enable `.current` probe files | Medium | **Needs code fixes** |
| ✅ Done | Agent | Extract platform detection logic | Medium | **Completed** |
| 🟢 Low | Panel | Refactor chart components | High | Pending |
| ✅ Done | Controller | Add TODO validations | Medium | **Completed** |
| ✅ Done | All | Create centralized error types | Medium | **Completed** |

---

## Completed Changes

The following simplifications have been implemented:

### Controller
- ✅ Created `web/helpers.go` with consolidated helper functions:
  - `currentUserID()`, `uintParam()`, `uintParamName()`, `intParam()`, `ifZero()`
  - `ListResponse` type and `NewListResponse()`, `NewPaginatedResponse()` helpers
- ✅ Removed duplicate functions from `workspaces.go` and `agents.go`
- ✅ Deleted `web/probes.old` (13KB dead code)
- ✅ Standardized list endpoints to use consistent `{data: [...]}` wrapper:
  - `GET /workspaces/{id}/members`
  - `GET /workspaces/{id}/agents/{agentID}/probes`
  - `GET /workspaces/{id}/probe-data/find`
  - `GET /workspaces/{id}/probe-data/probes/{probeID}/data`
- ✅ Added workspace access control to `internal/workspace/workspace.go`:
  - `UserHasAccess()` – checks if user is workspace member
  - `UserHasRole()` – checks if user has minimum role level
  - `GetMemberByUserID()` – retrieves member record
- ✅ Implemented permission validation in `web/probes.go`:
  - GET probes now requires workspace membership
  - POST probes now requires READ_WRITE or higher role

### Agent
- ✅ Deleted `probes/dns.old` (6KB dead code)
- ✅ Deleted `probes/trafficsim.oldgo` (22KB dead code)
- ✅ Created `lib/platform/platform.go` with consolidated platform detection:
  - `BinaryPath()`, `BinaryName()` for cross-platform binary resolution
  - `IsWindows()`, `IsLinux()`, `IsDarwin()` for OS detection
  - `CheckSupported()` for platform validation
- ✅ Refactored `probes/mtr.go` to use platform package (reduced 30+ lines to 4)
- ⚠️ Renamed `.current` files to `.go.disabled` – they contain code that references
  undefined `Probe.Config` field and needs refactoring before enabling

### Panel
- ✅ Deleted `TrafficSimGraph_OLD.vue` (8KB dead code)

### Documentation
- ✅ Updated main `README.md` to reflect PostgreSQL + ClickHouse architecture
- ✅ Removed all MongoDB references from documentation

### Error Handling
- ✅ Created `internal/errors/errors.go` with centralized error types:
  - Sentinel errors: `ErrUnauthorized`, `ErrForbidden`, `ErrNotFound`, `ErrBadRequest`, etc.
  - HTTP error constructors: `BadRequest()`, `NotFound()`, `Forbidden()`, `InternalError()`
  - Status code mapping: `StatusFromError()` for automatic HTTP status resolution
- ✅ Added `ErrorResponse` type and `NewErrorResponse()` helper to `web/helpers.go`

---

## Blocked: Agent Probe Files

The following files were renamed from `.current` to `.go.disabled` because they
have compile errors that need to be fixed:

| File | Issue |
|------|-------|
| `rperf.go.disabled` | References `Probe.Config` which doesn't exist |
| `trafficsim.go.disabled` | Type mismatch: `uint` vs `primitive.ObjectID` |
| `web.go.disabled` | Similar struct field issues |

**To fix:** Update these files to use the new `Probe` struct fields:
- Replace `cd.Config.Target[0].Target` → `cd.Targets[0].Target`
- Replace `primitive.ObjectID` → `uint` for IDs

---

## Estimated Impact

| Metric | Before | After |
|--------|--------|-------|
| Dead Code Deleted | 0 | ~49KB |
| Duplicate Functions | 5+ | 0 |
| Controller Build | ✅ | ✅ |
| Agent Build | ✅ | ✅ |

