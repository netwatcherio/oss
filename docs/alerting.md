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

## Alert Types

### Threshold-Based Alerts

Static threshold evaluation using fixed values:

| Metric | Probe Types | Description |
|--------|-------------|-------------|
| `packet_loss` | PING, TRAFFICSIM | Triggered when avg/max loss exceeds threshold |
| `latency` | PING, MTR, TRAFFICSIM | Triggered when RTT exceeds threshold |
| `jitter` | PING, TRAFFICSIM | Monitored for VoIP/real-time traffic |
| `dns_query_time` | DNS | Triggered when DNS resolution time exceeds threshold |
| `offline` | HEARTBEAT | Triggered if agent fails to check in |

### Baseline-Based Alerts

Dynamic thresholds calculated from historical data using standard deviation:

| Metric | Probe Types | Description |
|--------|-------------|-------------|
| `latency_baseline` | PING, MTR | Threshold = baseline_avg + (baseline_stddev × multiplier) |
| `packet_loss_baseline` | PING, TRAFFICSIM | Baseline-aware packet loss detection |
| `dns_query_time_baseline` | DNS | Dynamic DNS response time thresholds |

**Configuration:**
- Baseline window: 7/14/30 days
- Multiplier: Configurable standard deviation multiplier per severity
- Percentile support: P50/P90/P95/P99

### DNS-Specific Alerts

| Metric | Description |
|--------|-------------|
| `dns_failure` | DNS query returned SERVFAIL or REFUSED |
| `dns_nxdomain` | Domain does not exist (NXDOMAIN response) |
| `dns_timeout` | DNS query timed out |
| `dns_mismatch` | DNS response mismatch between expected and actual |

### HTTP/HTTPS Alerts

| Metric | Description |
|--------|-------------|
| `http_status` | HTTP status code indicates error (>= 400) |
| `http_ttfb` | Time to first byte exceeds threshold |
| `http_total` | Total request time exceeds threshold |
| `http_cert_expiring` | TLS certificate expires within threshold days |
| `http_cert_expired` | TLS certificate has expired |

### MTR-Specific Alerts

| Metric | Description |
|--------|-------------|
| `hop_loss` | Packet loss at specific MTR hop |
| `route_change` | Route change detected (new hop, missing hop, changed ASN) |
| `mos_score` | VoIP quality MOS score below threshold (ITU-T G.107 E-Model) |

### SysInfo Alerts

| Metric | Description |
|--------|-------------|
| `cpu_percent` | CPU usage exceeds threshold |
| `memory_percent` | Memory usage exceeds threshold |

### Agent Health Alerts

| Metric | Description |
|--------|-------------|
| `agent_offline` | Agent fails to check in (no heartbeat within timeout) |
| `agent_stale` | Agent check-in is delayed but not yet offline |

---

## Alert Rules

Rules are configured per-workspace via **Workspace Settings → Alert Rules**.

### Rule Configuration

| Field | Description |
|-------|-------------|
| `name` | Human-readable rule name |
| `metric` | One of the metrics listed above |
| `operator` | `>`, `>=`, `<`, `<=`, `=` |
| `threshold` | Numeric threshold value |
| `severity` | `info`, `warning`, `critical` |
| `probe_id` | Optional: scope to specific probe |
| `agent_id` | Optional: scope to specific agent |
| `use_baseline` | Use dynamic baseline thresholds instead of static |
| `baseline_multiplier` | Standard deviation multiplier for baseline (default: 2.0) |

### Compound Conditions

Alert rules can use AND/OR logic for complex conditions:

```json
{
  "name": "High Latency + Packet Loss",
  "conditions": [
    { "metric": "latency", "operator": ">", "threshold": 100 },
    "AND",
    { "metric": "packet_loss", "operator": ">", "threshold": 5 }
  ]
}
```

### Notification Channels

| Channel | Description |
|---------|-------------|
| **Panel** | Always-on dashboard alerts with navbar badge |
| **Email** | Email workspace members via configured SMTP |
| **Webhook** | HTTP POST to configured URL with HMAC signature |

---

### Agent Offline Detection

The agent offline alert is automatically triggered when:
- No heartbeat received within 60 seconds of expected interval
- Agent has not reported any probe data
- Agent was previously online but stopped responding

**Auto-recovery:** Alert automatically resolves when agent reconnects and resumes heartbeat.

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
