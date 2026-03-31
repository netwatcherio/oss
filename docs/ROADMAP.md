# NetWatcher OSS Roadmap 2026

> Last Updated: March 2026

---

## Current State Summary

### ✅ Implemented Features (v1.0)

| Category | Features |
|----------|----------|
| **Agents** | Cross-platform (Windows, Linux), auto-update with SHA256 verification, watchdog timer |
| **Probe Types** | PING, MTR, TrafficSim, Speedtest, Sysinfo, Netinfo, DNS, HTTP/HTTPS, TLS Certificate, SNMP |
| **Quality Metrics** | MOS Score (ITU-T G.107 E-Model) for VoIP quality |
| **Monitoring** | Bidirectional probe detection, 3-tier agent status (Online/Stale/Offline) |
| **Visualization** | D3 Workspace Network Map, Connectivity Matrix, real-time WebSocket dashboards |
| **Alerting** | Compound AND/OR conditions, MTR-specific (hop loss/latency/route changes), DNS (query time/failure/NXDOMAIN), SysInfo (CPU/Memory), Agent Offline detection, webhook notifications (HMAC-signed), auto-resolve on recovery |
| **Controller APIs** | Centralized GeoIP, WHOIS, OUI lookup, `/whoami` for air-gapped networks |
| **Multi-Tenancy** | Workspace isolation, configurable resource limits, RBAC (Owner/Admin/User/Viewer) |
| **Sharing** | Sharable Agent Pages (P3.6) with token-based access, password protection, speedtest gating |
| **Storage** | ClickHouse time-series backend with lazy cache refresh |
| **Agent Lifecycle** | Deactivation on deletion, kick-and-prevent reconnection |
| **Observability** | Prometheus `/metrics` endpoint (agent/workspace/alert counters), Loki log shipping via HTTP push |

---

## Phase 1: Core Polish (Q1 2026)

### P1.1 Network Interface Binding 🔄
**Status: Mostly Complete** | **Remaining: Low Effort**

| Completed | Pending |
|-----------|---------|
| Interface discovery (IPs, MAC, type) | Multi-WAN support (failover/load-balancing) |
| Cross-platform route discovery | Full UI visualization (status indicator) |
| OUI/vendor lookup | |
| Probe-level interface binding (PING, MTR, DNS, TrafficSim) | |
| Interface validation before probe execution (fail-fast) | |
| UI: dropdown in NewProbe.vue and ProbesEdit.vue | |

**Priority:** High — Critical for SD-WAN and multi-NIC environments

---

### P1.3 Dynamic Thresholds ✅
**Status: Partially Complete** | **Priority: High** | **Effort: Medium**

| Completed | Pending |
|-----------|---------|
| Rolling baseline calculation per probe (ClickHouse aggregation) | Baseline visualization in UI |
| Standard deviation-based thresholds (configurable multiplier per severity) | Hybrid mode toggle in alert rule UI |
| Percentile-based thresholds (P50/P90/P95/P99) | |
| Per-metric baselines: latency, packet_loss, DNS query time, HTTP TTFB/total, TLS expiry, SNMP response | |
| Baseline stats API endpoint (`GET /workspaces/:id/probes/:probeID/baseline`) | |
| Dynamic threshold evaluation in alert pipeline | |
| Baseline window configuration (7/14/30 days) | |

---

### P1.4 Email & Registration Configuration ✅
**Status: Complete**

**System-Wide SMTP (via .env):**
- [x] SMTP host, port, user, password, TLS, skip-verify env vars
- [x] Background email worker with queue, retry logic, batch processing
- [x] HTML + plaintext branded email templates (invite, registration, password reset, email verification)
- [x] Webhook delivery alternative (`EMAIL_WEBHOOK_URL`)

**Registration & Verification Controls:**
- [x] `REQUIRE_EMAIL_VERIFICATION` env var (default: `false`)
- [x] `REGISTRATION_ENABLED` env var (default: `true`)
- [x] Email verification flow (send, resend, verify endpoints + panel UI)
- [x] Password reset flow (forgot password, reset with token)
- [x] Workspace invite emails with branded templates

> **Future Enhancement:** Alert email digest mode (batch alerts) — `sendEmailNotification` in alerting pipeline is scaffolded but not yet connected to the email queue.

**Priority:** ✅ Shipped

---

### P1.5 Smart Notifications 🔄
**Status: Partially Complete** | **Remaining: Low Effort**

| Completed | Pending |
|-----------|---------|
| Auto-resolve on recovery (probes + agent offline) | Debouncing (suppress duplicates in time window) |
| Webhook notifications with HMAC signing | Grouping (combine related alerts) |
| Panel in-app alert notifications | Escalation (Warning → Critical after N minutes) |
| Duplicate suppression (skip if active alert exists) | |

**Priority:** Medium — Reduces notification fatigue

---

## Phase 2: Probe Expansion (Q2 2026)

### P2.1 SNMP Probe ✅
**Status: Partially Complete** | **Priority: Critical** | **Effort: High**

The #1 feature gap vs. established competitors. Core probe implemented, full device discovery and vendor profiles remaining.

| Completed | Pending |
|-----------|---------|
| SNMP v1, v2c, v3 support (NoAuthNoPriv/AuthNoPriv/AuthPriv) | Device discovery and OID auto-detection |
| Auth protocols: MD5, SHA, SHA224/256/384/512 | Vendor profiles (Cisco, Juniper, Ubiquiti, Arista) |
| Privacy protocols: DES, AES-128/192/256 | Interface up/down/speed tracking |
| Built-in OID profiles: system, interface, cpu, memory | |
| Custom OID support | |
| SNMP probe creation UI (NewProbe.vue) | |
| Ultra-fast polling (30-second intervals) | |

---

### P2.2 DNS Probe ✅
**Status: Complete**

- [x] Query types: A, AAAA, MX, TXT, CNAME, NS
- [x] Resolution time metrics
- [x] Multiple resolver support
- [x] Configurable intervals
- [x] Dashboard visualization
- [x] Shareable DNS pages

---

### P2.3 HTTP/HTTPS Probe ✅
**Status: Complete**

| Completed | Pending |
|-----------|---------|
| Response timing breakdown (DNS, TCP, TLS, TTFB) | Dashboard visualization |
| TLS certificate extraction | Shareable HTTP pages |
| Status code and header tracking | |
| Content match (contains/regex) | |
| Configurable headers, follow redirects, insecure TLS | |
| HTTP probe creation UI (NewProbe.vue) | |

---

### P2.3b TLS Certificate Probe ✅
**Status: Complete**

- [x] Connects to `host:443`, extracts full certificate chain
- [x] Days until expiry, `is_expired`, `is_expiring_soon` (≤30 days)
- [x] Issuer org, SHA256 fingerprint, cert type (leaf/intermediate/root/self-signed)
- [x] TLS probe creation UI (NewProbe.vue)

---

### P2.4 AS Path Resolution
**Priority: Medium** | **Effort: Low**

- IP-to-ASN lookup
- ASN display in MTR hop table
- AS path change alerting
- AS-level grouping in Network Map

---

### P2.5 RIR REST API Integration
**Priority: Medium** | **Effort: Medium**

Replace command-line WHOIS with structured RIR APIs:
- ARIN, RIPE, APNIC, LACNIC, AFRINIC
- Auto-detect RIR by IP range
- Cache responses in ClickHouse

---

## Phase 3: Enterprise Features (Q3-Q4 2026)

| Feature | Priority | Description |
|---------|----------|-------------|
| **P3.1 Custom Dashboards** | Medium | Drag-and-drop widget library, per-workspace layouts |
| **P3.2 Scheduled Reports** | ✅ Complete | PDF exports, cron scheduling, email delivery, on-demand preview (see [API Reference](./api-reference.md#reports-api-endpoints)) |
| **P3.3 RBAC Enhancements** | Medium | Operator role, per-role permissions, audit logging |
| **P3.4 SSO Integration** | Medium | SAML 2.0, OIDC/OAuth2, auto-provisioning |
| **P3.5 API Key Management** | Low | Scoped tokens, expiry/rotation, usage logging |
| **P3.6 Sharable Pages** | ✅ Complete | Token-based public links, password protection |
| **P3.7 Observability Integrations** | ✅ Backend Complete | Prometheus `/metrics` endpoint, Loki log shipping, Grafana dashboards |

---

## Phase 4: Scale & Polish (2027+)

| Feature | Description |
|---------|-------------|
| **Horizontal Scaling** | ClickHouse cluster, controller load balancing |
| **Cloud Templates** | Terraform (AWS/Azure/GCP), Helm charts, K8s DaemonSet |
| **Advanced Speed Tests** | Improved bandwidth testing |
| **Geographic Visualization** | GeoIP-based agent/hop locations with latency gradients |

---

## Proposed Priority Order for Q1-Q2 2026

Based on user value and competitive positioning:

| Priority | Feature | Rationale |
|----------|---------|-----------|
| ~~1~~ | ~~P2.1 SNMP Probe~~ | ✅ Core shipped (partial: device discovery + vendor profiles pending) |
| ~~2~~ | ~~P1.4 Email Notifications~~ | ✅ Shipped |
| ~~3~~ | ~~P2.2 DNS Probe~~ | ✅ Shipped |
| ~~4~~ | ~~P2.3 HTTP/HTTPS Probe~~ | ✅ Shipped |
| ~~4b~~ | ~~P2.3b TLS Certificate Probe~~ | ✅ Shipped |
| ~~5~~ | ~~P3.2 Scheduled Reports~~ | ✅ Shipped |
| ~~6~~ | ~~P1.3 Dynamic Thresholds~~ | ✅ Backend complete (UI pending) |
| ~~7~~ | ~~P1.1 Interface Binding~~ | ✅ Validation + binding done (multi-WAN + viz remaining) |
| ~~8~~ | ~~P3.7 Prometheus/Loki API~~ | ✅ Prometheus /metrics + Loki log shipping done |
| 9 | P2.4 AS Path Resolution | IP-to-ASN lookup, MTR hop ASN display |

---

## Not Planned

| Feature | Reason |
|---------|--------|
| AI/NLP Queries | High cost, low differentiation |
| Full NetFlow Ingestion | Separate product vertical |
| DDoS Mitigation | Requires carrier-grade infrastructure |
| CDN/OTT Tracking | Needs DPI or flow analysis |
| Mobile Native Apps | Consider PWA instead |

---

## Success Metrics

### 6-Month Goals (Mid-2026)
- [x] SNMP polling shipped (core — partial: discovery + vendor profiles pending)
- [x] DNS probe shipped
- [x] HTTP/HTTPS probe shipped (including TLS cert probe)
- [x] Email notifications complete
- [x] Scheduled reports shipped
- [ ] 500+ GitHub stars

### 12-Month Goals (End of 2026)
- [ ] Custom dashboards shipped
- [ ] SSO integration shipped
- [ ] 2,000+ GitHub stars
- [ ] 5+ documented production deployments

---

## Contributing

We welcome community contributions! Priority areas:

1. **SNMP device profiles** — Vendor-specific OID mappings
2. **Probe types** — New monitoring capabilities
3. **Integrations** — Notification channels, export formats
4. **Documentation** — Deployment guides, tutorials

See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
