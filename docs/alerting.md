# Alerting System

NetWatcher OSS includes a configurable alerting system that monitors network telemetry and agent health, providing real-time notifications via the dashboard and external channels.

## Overview

The alerting system follows a three-tier lifecycle:

```
Alert Definition → Alert Rule → Alert Instance
```

- **Alert Definition**: Templates defining logic for specific metrics
- **Alert Rule**: User-configured thresholds applied to workspaces, agents, or probes
- **Alert Instance**: A triggered event record tracking state and severity

---

## Supported Metrics

| Metric | Probe Types | Description |
|--------|-------------|-------------|
| `packet_loss` | PING, TRAFFICSIM | Triggered when avg/max loss exceeds threshold |
| `latency` | PING, MTR, TRAFFICSIM | Triggered when RTT exceeds threshold |
| `jitter` | PING, TRAFFICSIM | Monitored for VoIP/real-time traffic |
| `offline` | HEARTBEAT | Triggered if agent fails to check in |

---

## Alert Rules

Rules are configured per-workspace via **Workspace Settings → Alert Rules**.

### Rule Configuration

| Field | Description |
|-------|-------------|
| `name` | Human-readable rule name |
| `metric` | One of: `packet_loss`, `latency`, `jitter`, `offline` |
| `operator` | `>`, `>=`, `<`, `<=`, `=` |
| `threshold` | Numeric threshold value |
| `severity` | `info`, `warning`, `critical` |
| `probe_id` | Optional: scope to specific probe |
| `agent_id` | Optional: scope to specific agent |

### Notification Channels

| Channel | Description |
|---------|-------------|
| **Panel** | Always-on dashboard alerts with navbar badge |
| **Email** | Email workspace members (planned) |
| **Webhook** | HTTP POST to configured URL |

---

## Webhook Notifications

When an alert triggers, the system sends an HTTP POST request to the configured webhook URL.

### Headers

```
Content-Type: application/json
User-Agent: NetWatcher-Alert/1.0
X-NetWatcher-Signature: sha256=<hmac_signature>
```

### Payload

```json
{
  "alert_id": 123,
  "workspace_id": 1,
  "probe_id": 45,
  "agent_id": 10,
  "probe_type": "PING",
  "probe_name": "Core Router Ping",
  "probe_target": "10.0.0.1",
  "agent_name": "Edge-Node-01",
  "panel_url": "/workspaces/1/agents/10?probe=45",
  "metric": "packet_loss",
  "value": 5.2,
  "threshold": 1.0,
  "severity": "critical",
  "message": "packet_loss exceeded threshold: 5.20 (threshold: 1.00)",
  "triggered_at": "2026-01-12T20:30:00Z"
}
```

### HMAC Verification

If a webhook secret is configured, verify the signature:

```go
mac := hmac.New(sha256.New, []byte(secret))
mac.Write(payloadJSON)
expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
// Compare with X-NetWatcher-Signature header
```

---

## Alert States

| State | Description |
|-------|-------------|
| `active` | Alert triggered and requires attention |
| `acknowledged` | User acknowledged, still monitoring |
| `resolved` | Condition cleared or manually resolved |

---

## Global Alerts View

Access `/workspaces/alerts` to view alerts across all workspaces:

- **Stats Cards**: Active, Acknowledged, Resolved counts
- **Status Filtering**: Tab-based navigation
- **Inline Actions**: Acknowledge or Resolve directly from list
- **Deep Links**: Click agent/probe names to navigate

---

## API Endpoints

See [API Reference](./api-reference.md#alert-endpoints) for complete endpoint documentation.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/workspaces/alerts` | GET | List alerts across workspaces |
| `/workspaces/alerts/count` | GET | Get active alert count |
| `/alerts/{id}` | PATCH | Update alert status |
| `/workspaces/{id}/alert-rules` | GET | List workspace rules |
| `/workspaces/{id}/alert-rules` | POST | Create alert rule |
| `/workspaces/{id}/alert-rules/{ruleId}` | PATCH | Update rule |
| `/workspaces/{id}/alert-rules/{ruleId}` | DELETE | Delete rule |
