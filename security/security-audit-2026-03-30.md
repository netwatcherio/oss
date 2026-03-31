# NetWatcher Security Audit Report

**Date:** Mon Mar 30 2026  
**Auditor:** Security Agent  
**Status:** Partial Fix Complete - Remaining Issues Documented Below

---

## Executive Summary

This security audit identified **16 security vulnerabilities** across the NetWatcher codebase, including distributed network monitoring platform components (Controller, Agent, Panel). Of these, **2 critical/high issues have been fixed**, and **14 remain pending remediation**.

### Issues Fixed in This Session

| # | Severity | Issue | File | Status |
|---|----------|-------|------|--------|
| 8 | HIGH | IDOR in Workspace Member Management | `controller/web/workspaces.go` | ✅ FIXED |
| 9 | HIGH | ClickHouse Query String Formatting | `controller/internal/probe/clickhouse.go` | ✅ FIXED |

---

## Remaining Security Findings

### Critical Severity

#### 1. Hardcoded JWT Fallback Secret
**File:** `controller/internal/users/auth.go:53-58`
```go
func signingKey() []byte {
    if s := os.Getenv("JWT_SECRET"); s != "" {
        return []byte(s)
    }
    return []byte("dev-secret-change-me")  // ← DANGEROUS
}
```
**Risk:** If `JWT_SECRET` env var is not set, all JWT tokens can be forged. Complete authentication bypass.  
**Recommendation:** Fail startup if `JWT_SECRET` is not set:
```go
if s := os.Getenv("JWT_SECRET"); s != "" {
    return []byte(s)
}
return nil, errors.New("JWT_SECRET environment variable is required")
```

---

#### 2. CORS Allows All Origins ("*")
**File:** `controller/main.go:125-130`
```go
AllowOrigins: "*"
```
**Risk:** Any website can make API requests on behalf of users. Enables CSRF attacks and unauthorized cross-origin access.  
**Recommendation:** Restrict to known frontend origins:
```go
AllowOrigins: "https://app.netwatcher.io,https://www.netwatcher.io"
```

---

#### 3. WebSocket Origin Validation Disabled
**Files:** 
- `controller/web/ws.go:46`
- `controller/web/ws_raw_panel.go:36`
```go
CheckOrigin: func(r *http.Request) bool {
    return true  // Allow all origins
}
```
**Risk:** Cross-site WebSocket hijacking possible for both agent and panel connections.  
**Recommendation:** Implement proper origin validation based on deployment configuration.

---

### High Severity

#### 4. Agent TrafficSim UDP DDoS Vector
**File:** `agent/probes/trafficsim.go`

**Risk:** 
- Agents communicate via UDP with each other using arbitrary IP:port based on probe configuration
- No rate limiting on UDP sends
- Could be exploited as UDP reflector/amplifier for DDoS attacks

**Recommendation:** 
- Add rate limiting to traffic simulation probes
- Validate target IPs are within expected ranges
- Implement egress filtering at the agent level

---

#### 5. JWT Tokens Stored in localStorage
**File:** `panel/src/session.ts:14-34`
```typescript
const STORAGE_KEY = "netwatcher.session";
export function setSession(s: Session) {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
}
```
**Risk:** Vulnerable to XSS attacks - any injected script can exfiltrate tokens.  
**Recommendation:** Use httpOnly cookies for token storage.

---

#### 6. Token Passed in WebSocket URL Query String
**File:** `panel/src/services/websocketService.ts:79`
```typescript
return `${baseUrl}/ws/panel/raw?token=${encodeURIComponent(token)}`;
```
**Risk:** Token leaks in server logs, browser history, and Referer headers.  
**Recommendation:** Pass token via WebSocket subprotocol or first message after connection.

---

#### 7. WebSocket URL Redirection via window.CONTROLLER_ENDPOINT
**File:** `panel/src/services/websocketService.ts:57-76`
```typescript
let baseUrl = anyWindow?.CONTROLLER_ENDPOINT
    || import.meta.env.CONTROLLER_ENDPOINT
    || '';
```
**Risk:** Attacker could set `window.CONTROLLER_ENDPOINT` to a malicious server to steal tokens.  
**Recommendation:** Remove runtime override capability; only use build-time environment variables.

---

### Medium Severity

#### 8. bcrypt DefaultCost (10) Could Be Higher
**Files:** 
- `controller/internal/users/users.go:87`
- `controller/internal/agent/agent.go:174,376,636`
- `controller/internal/share/share.go:110`

**Risk:** Default bcrypt cost of 10 is acceptable but could be stronger for high-security scenarios.  
**Recommendation:** Use cost 12 or higher for PSK and sensitive credential hashing.

---

#### 9. PIN Stored Plaintext Until Consumed
**File:** `controller/internal/agent/agent.go:182`
```go
PinPlaintext string `gorm:"-" json:"-"`
```
**Risk:** PIN stored unencrypted in memory/database during PIN validity period.  
**Recommendation:** Hash PIN immediately upon receipt; only store hash.

---

#### 10. Agent PSK Stored in Plaintext
**File:** `agent/web/client.go` (agent_auth.json)

**Risk:** PSK saved to file with 0600 permissions but unencrypted. If filesystem is compromised, PSK is revealed.  
**Recommendation:** Consider OS keychain storage (Keychain on macOS, Credential Manager on Windows).

---

#### 11. Auto-Updater Configurable UpdateURL
**File:** `agent/auto_updater.go:151-152`
```go
if u.config.UpdateURL != "" {
    url = u.config.UpdateURL
}
```
**Risk:** If attacker can configure `UpdateURL`, they can serve malicious updates (only SHA256 hash verification, no signature).  
**Recommendation:** 
- Enforce signature verification (not just hash)
- Only allow updates from official GitHub releases by default
- Require code signing for custom update URLs

---

#### 12. Dependency Download Hash-Only Verification
**File:** `agent/dependency_download.go`

**Risk:** Downloads external binaries (trippy) with only SHA256 verification. If hash is compromised, malicious binary could be delivered.  
**Recommendation:** Implement GPG signature verification for downloaded dependencies.

---

#### 13. Debug Endpoint Exposes Connection Metadata
**File:** `controller/web/admin.go:56`
```go
adminAPI.Get("/debug/connections", adminDebugConnectionsHandler(db))
```
**Risk:** Returns agent IDs, workspace IDs, connection IDs, client IPs - information disclosure.  
**Recommendation:** Ensure debug endpoints are disabled in production or require additional authentication.

---

#### 14. Log Injection Possible
**Multiple locations** - User-controlled data logged without sanitization.

**Risk:** If log aggregation systems process these logs, special characters could cause log formatting issues or hide entries.  
**Recommendation:** Sanitize log input or use structured logging that escapes control characters.

---

## Summary of All Findings

| # | Severity | Issue | Component | Status |
|---|----------|-------|-----------|--------|
| 1 | CRITICAL | Hardcoded JWT fallback secret | Controller | PENDING |
| 2 | CRITICAL | CORS AllowOrigins: "*" | Controller | PENDING |
| 3 | CRITICAL | WebSocket origin validation disabled | Controller | PENDING |
| 4 | HIGH | TrafficSim UDP DDoS vector | Agent | PENDING |
| 5 | HIGH | JWT tokens in localStorage | Panel | PENDING |
| 6 | HIGH | Token in WebSocket URL query string | Panel | PENDING |
| 7 | HIGH | WebSocket URL via window.CONTROLLER_ENDPOINT | Panel | PENDING |
| 8 | HIGH | IDOR in member management | Controller | ✅ FIXED |
| 9 | HIGH | ClickHouse string formatting injection | Controller | ✅ FIXED |
| 10 | MEDIUM | bcrypt cost 10 | Controller | PENDING |
| 11 | MEDIUM | PIN stored plaintext | Controller | PENDING |
| 12 | MEDIUM | PSK stored plaintext (agent) | Agent | PENDING |
| 13 | MEDIUM | Configurable UpdateURL | Agent | PENDING |
| 14 | MEDIUM | Dependency hash-only verification | Agent | PENDING |
| 15 | MEDIUM | Debug endpoint exposure | Controller | PENDING |
| 16 | MEDIUM | Log injection possible | Multiple | PENDING |

---

## Recommended Priority Fix Order

1. **Immediately:** Fix JWT secret handling (fail if not set)
2. **Immediately:** Restrict CORS origins
3. **Immediately:** Add WebSocket origin validation
4. **High:** Move JWT from localStorage to httpOnly cookies
5. **High:** Remove window.CONTROLLER_ENDPOINT override
6. **High:** Add rate limiting to TrafficSim
7. **Medium:** Increase bcrypt cost for sensitive operations
8. **Medium:** Add GPG signature verification for agent updates

---

## Files Modified in This Session

- `controller/internal/workspace/workspace.go` - IDOR fix
- `controller/web/workspaces.go` - IDOR fix
- `controller/internal/probe/probe.go` - Type validation
- `controller/internal/probe/clickhouse.go` - Query injection fix
- `controller/web/data.go` - Type validation
- `controller/web/admin.go` - Updated for new store signatures
