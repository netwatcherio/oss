# Sentry Integration Guide

This document outlines how to implement Sentry error tracking and performance monitoring for the NetWatcher platform, covering both the **Vue.js frontend (panel)** and the **Go backend (controller)**.

---

## Overview

[Sentry](https://sentry.io/) provides real-time error tracking and performance monitoring. Integrating Sentry will help identify and diagnose issues in production before users report them.

| Component   | Technology        | Sentry SDK                  |
| ----------- | ----------------- | --------------------------- |
| Panel       | Vue 3 + Vite      | `@sentry/vue`               |
| Controller  | Go (Iris v12)     | `github.com/getsentry/sentry-go` |

---

## Prerequisites

1. **Create a Sentry Account** at [sentry.io](https://sentry.io/)
2. **Create Two Projects** in Sentry:
   - One for **Frontend** (Vue)
   - One for **Backend** (Go)
3. **Obtain DSN keys** for each project

---

## Frontend Integration (Vue 3 + Vite)

### 1. Install Dependencies

```bash
cd panel
npm install @sentry/vue
```

### 2. Configure Sentry in `main.ts`

Update `panel/src/main.ts` to initialize Sentry:

```typescript
import { createApp } from 'vue'
import * as Sentry from '@sentry/vue'
import App from './App.vue'
import router from './router'

const app = createApp(App)

// Initialize Sentry (only in production)
if (import.meta.env.PROD) {
  Sentry.init({
    app,
    dsn: import.meta.env.VITE_SENTRY_DSN,
    environment: import.meta.env.VITE_SENTRY_ENVIRONMENT || 'production',
    
    // Performance Monitoring
    integrations: [
      Sentry.browserTracingIntegration({ router }),
      Sentry.replayIntegration(),
    ],
    
    // Capture 20% of transactions for performance monitoring
    tracesSampleRate: 0.2,
    
    // Session Replay - capture 10% of sessions, 100% on error
    replaysSessionSampleRate: 0.1,
    replaysOnErrorSampleRate: 1.0,
    
    // Filter out noisy errors
    ignoreErrors: [
      'ResizeObserver loop limit exceeded',
      'Network Error',
      'Request aborted',
    ],
    
    // Add release version for tracking deployments
    release: import.meta.env.VITE_SENTRY_RELEASE || 'netwatcher-panel@unknown',
  })
}

app.use(router)
app.mount('#app')
```

### 3. Add Environment Variables

Add to `.env` (and `.env.example`):

```bash
# Sentry Configuration
VITE_SENTRY_DSN=https://your-key@sentry.io/project-id
VITE_SENTRY_ENVIRONMENT=production
VITE_SENTRY_RELEASE=netwatcher-panel@1.0.0
```

### 4. Source Maps for Better Stack Traces (Optional but Recommended)

Install the Vite plugin for uploading source maps:

```bash
npm install @sentry/vite-plugin --save-dev
```

Update `vite.config.ts`:

```typescript
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { sentryVitePlugin } from '@sentry/vite-plugin'

export default defineConfig({
  plugins: [
    vue(),
    // Upload source maps in production builds
    sentryVitePlugin({
      org: 'your-sentry-org',
      project: 'netwatcher-panel',
      authToken: process.env.SENTRY_AUTH_TOKEN,
    }),
  ],
  build: {
    sourcemap: true, // Required for source map uploads
  },
})
```

### 5. Manual Error Capturing

Capture custom errors or add context:

```typescript
import * as Sentry from '@sentry/vue'

// Capture an error with context
try {
  await riskyOperation()
} catch (error) {
  Sentry.captureException(error, {
    tags: { feature: 'probe-management' },
    extra: { probeId: probe.id, agentId: agent.id },
  })
}

// Set user context after login
Sentry.setUser({
  id: user.id,
  email: user.email,
  username: user.name,
})

// Clear user on logout
Sentry.setUser(null)
```

---

## Backend Integration (Go + Iris)

### 1. Install the Sentry SDK

```bash
cd controller
go get github.com/getsentry/sentry-go
go get github.com/getsentry/sentry-go/iris
```

### 2. Initialize Sentry in `main.go`

```go
package main

import (
    "log"
    "os"
    "time"

    "github.com/getsentry/sentry-go"
    sentryiris "github.com/getsentry/sentry-go/iris"
    "github.com/kataras/iris/v12"
)

func main() {
    // Initialize Sentry
    err := sentry.Init(sentry.ClientOptions{
        Dsn:              os.Getenv("SENTRY_DSN"),
        Environment:      os.Getenv("SENTRY_ENVIRONMENT"),
        Release:          os.Getenv("SENTRY_RELEASE"),
        TracesSampleRate: 0.2, // 20% of transactions
        Debug:            os.Getenv("SENTRY_DEBUG") == "true",
        
        // Attach stack traces to all messages
        AttachStacktrace: true,
        
        // Filter sensitive data
        BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
            // Remove sensitive headers
            if event.Request != nil {
                delete(event.Request.Headers, "Authorization")
                delete(event.Request.Headers, "Cookie")
            }
            return event
        },
    })
    if err != nil {
        log.Fatalf("sentry.Init: %s", err)
    }
    defer sentry.Flush(2 * time.Second)

    app := iris.New()
    
    // Add Sentry middleware
    app.Use(sentryiris.New(sentryiris.Options{
        Repanic: true, // Re-panic after capturing
    }))

    // ... rest of your routes and setup
    
    app.Listen(":8080")
}
```

### 3. Add Environment Variables

Add to `.env`:

```bash
# Sentry Configuration
SENTRY_DSN=https://your-key@sentry.io/backend-project-id
SENTRY_ENVIRONMENT=production
SENTRY_RELEASE=netwatcher-controller@1.0.0
SENTRY_DEBUG=false
```

### 4. Capturing Errors in Handlers

```go
import (
    "github.com/getsentry/sentry-go"
    "github.com/kataras/iris/v12"
)

func someHandler(ctx iris.Context) {
    // Get hub from context (set by middleware)
    hub := sentryiris.GetHubFromContext(ctx)
    
    // Add breadcrumbs for debugging
    hub.AddBreadcrumb(&sentry.Breadcrumb{
        Category: "handler",
        Message:  "Processing probe creation",
        Level:    sentry.LevelInfo,
    }, nil)
    
    // Capture error with context
    if err := processProbe(); err != nil {
        hub.WithScope(func(scope *sentry.Scope) {
            scope.SetTag("probe.type", "PING")
            scope.SetExtra("agent_id", agentID)
            scope.SetUser(sentry.User{
                ID:    userID,
                Email: userEmail,
            })
            hub.CaptureException(err)
        })
        
        ctx.StatusCode(500)
        ctx.JSON(iris.Map{"error": "Internal server error"})
        return
    }
    
    ctx.JSON(iris.Map{"status": "ok"})
}
```

### 5. Panic Recovery with Sentry

The Iris middleware handles panics automatically, but for goroutines:

```go
func backgroundTask() {
    defer sentry.Recover()
    
    // ... task that might panic
}
```

---

## Agent Integration (Go - Distributed)

The NetWatcher Agent runs on customer infrastructure and reports telemetry back to the controller. Sentry integration here helps diagnose issues in remote deployments.

### 1. Install the Sentry SDK

```bash
cd agent
go get github.com/getsentry/sentry-go
```

### 2. Initialize Sentry in `main.go`

Add Sentry initialization early in `runAgent()`:

```go
package main

import (
    "github.com/getsentry/sentry-go"
    "time"
)

func runAgent(ctx context.Context) error {
    // Initialize Sentry early
    err := sentry.Init(sentry.ClientOptions{
        Dsn:              os.Getenv("SENTRY_DSN"),
        Environment:      os.Getenv("SENTRY_ENVIRONMENT"),
        Release:          "netwatcher-agent@" + VERSION,
        TracesSampleRate: 0.1, // Lower sample rate for distributed agents
        
        // Attach stack traces
        AttachStacktrace: true,
        
        // Tag all events with agent context
        BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
            event.Tags["agent_id"] = os.Getenv("NWIO_AGENT_ID")
            event.Tags["workspace_id"] = os.Getenv("NWIO_WORKSPACE_ID")
            return event
        },
    })
    if err != nil {
        log.Warnf("Sentry init failed: %s (continuing without Sentry)", err)
        // Don't fail agent startup if Sentry init fails
    } else {
        defer sentry.Flush(2 * time.Second)
    }

    loadConfig(configPath)
    // ... rest of runAgent
}
```

### 3. Capture Errors in Workers

Update workers to report errors to Sentry:

```go
// workers/data.go
import "github.com/getsentry/sentry-go"

func ProbeDataWorker(wsClient *web.WSClient, probeDataCh chan probes.ProbeData) {
    defer sentry.Recover() // Catch panics in this goroutine
    
    for data := range probeDataCh {
        if err := sendProbeData(wsClient, data); err != nil {
            sentry.WithScope(func(scope *sentry.Scope) {
                scope.SetTag("probe.type", string(data.Type))
                scope.SetExtra("probe_id", data.ProbeID)
                sentry.CaptureException(err)
            })
            log.Errorf("Failed to send probe data: %v", err)
        }
    }
}
```

### 4. Capture WebSocket Connection Issues

```go
// web/ws_client.go
func (c *WSClient) ConnectWithRetry(ctx context.Context) {
    for {
        if err := c.connect(); err != nil {
            sentry.WithScope(func(scope *sentry.Scope) {
                scope.SetTag("ws.url", c.URL)
                scope.SetLevel(sentry.LevelWarning)
                sentry.CaptureException(err)
            })
            // ... retry logic
        }
    }
}
```

### 5. Agent Environment Variables

Add to agent configuration (can be set via install script or systemd unit):

```bash
# Optional - agent runs fine without Sentry
SENTRY_DSN=https://your-key@sentry.io/agent-project-id
SENTRY_ENVIRONMENT=production
```

> [!TIP]
> Consider making Sentry optional for the agent since it runs on customer infrastructure.
> Customers who don't want telemetry can simply omit the `SENTRY_DSN` variable.

### 6. Windows Service Considerations

For Windows service deployments, ensure Sentry flushes properly on service stop:

```go
// In platform/windows_service.go
func (s *agentService) Stop(req *svc.Request) error {
    sentry.Flush(2 * time.Second) // Flush before shutdown
    // ... existing shutdown logic
}
```

---

## Environment-Specific Configuration

| Environment | `tracesSampleRate` | `replaysSessionSampleRate` |
| ----------- | ------------------ | -------------------------- |
| Development | `0.0` (disabled)   | `0.0`                      |
| Staging     | `1.0` (100%)       | `0.5`                      |
| Production  | `0.1-0.2` (10-20%) | `0.1`                      |

> [!TIP]
> Start with higher sample rates and reduce them as traffic grows to manage costs.

---

## Docker / Deployment Configuration

Update your Docker Compose or deployment configs to include Sentry environment variables:

```yaml
# docker-compose.yml
services:
  controller:
    environment:
      - SENTRY_DSN=${SENTRY_DSN}
      - SENTRY_ENVIRONMENT=${SENTRY_ENVIRONMENT:-production}
      - SENTRY_RELEASE=${SENTRY_RELEASE:-unknown}
      
  panel:
    build:
      args:
        - VITE_SENTRY_DSN=${VITE_SENTRY_DSN}
        - VITE_SENTRY_ENVIRONMENT=${VITE_SENTRY_ENVIRONMENT:-production}
```

---

## CI/CD Integration

### GitHub Actions Example

```yaml
# .github/workflows/deploy.yml
jobs:
  deploy:
    steps:
      - name: Build Panel with Sentry
        run: npm run build
        env:
          VITE_SENTRY_DSN: ${{ secrets.SENTRY_FRONTEND_DSN }}
          VITE_SENTRY_RELEASE: netwatcher-panel@${{ github.sha }}
          SENTRY_AUTH_TOKEN: ${{ secrets.SENTRY_AUTH_TOKEN }}
          
      - name: Create Sentry Release
        uses: getsentry/action-release@v1
        env:
          SENTRY_AUTH_TOKEN: ${{ secrets.SENTRY_AUTH_TOKEN }}
          SENTRY_ORG: your-org
        with:
          projects: netwatcher-panel netwatcher-controller
          version: ${{ github.sha }}
          environment: production
```

---

## Alerting Configuration (in Sentry Dashboard)

Set up alerts for:

1. **New Issues** - Get notified when new error types appear
2. **Issue Frequency** - Alert when an issue spikes (e.g., >10 occurrences/hour)
3. **Performance Regression** - P95 latency exceeds threshold
4. **Crash Rate** - Frontend session crash rate > 1%

---

## Testing the Integration

### Frontend Test

```typescript
// Trigger a test error in browser console
Sentry.captureMessage('Test message from NetWatcher Panel')
throw new Error('Test error from NetWatcher Panel')
```

### Backend Test

```go
// Add a test endpoint
app.Get("/sentry-test", func(ctx iris.Context) {
    hub := sentryiris.GetHubFromContext(ctx)
    hub.CaptureMessage("Test message from NetWatcher Controller")
    
    // Or trigger a panic
    panic("Test panic from NetWatcher Controller")
})
```

---

## Security Considerations

> [!IMPORTANT]
> - Never commit DSN keys to version control
> - Use environment variables for all Sentry configuration
> - Filter sensitive data (auth tokens, passwords) using `beforeSend` hooks
> - Consider using Sentry's data scrubbing features for PII compliance

---

## Cost Management Tips

1. **Sample Rate Tuning** - Lower `tracesSampleRate` as traffic grows
2. **Error Filtering** - Use `ignoreErrors` to filter noisy/known issues
3. **Inbound Filters** - Configure in Sentry dashboard to drop browser extension errors
4. **Rate Limits** - Set project rate limits to prevent runaway costs

---

## Useful Dashboard Queries

```
# Find all errors for a specific user
user.email:user@example.com

# Find errors related to probes
tags[feature]:probe-management

# Find slow API calls
transaction.duration:>2s
```

---

## References

- [Sentry Vue SDK Documentation](https://docs.sentry.io/platforms/javascript/guides/vue/)
- [Sentry Go SDK Documentation](https://docs.sentry.io/platforms/go/)
- [Sentry Iris Integration](https://docs.sentry.io/platforms/go/guides/iris/)
- [Performance Monitoring](https://docs.sentry.io/product/performance/)
