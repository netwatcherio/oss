# NetWatcher OSS Roadmap 2026

> Last Updated: March 2026

---

## Current State Summary

### ✅ Implemented Features (v1.0)

| Category | Features |
|----------|----------|
| **Agents** | Cross-platform (Windows, Linux), auto-update with SHA256 verification, watchdog timer |
| **Probe Types** | PING, MTR, TrafficSim, Speedtest, Sysinfo, Netinfo, DNS, HTTP/HTTPS |
| **Quality Metrics** | MOS Score (ITU-T G.107 E-Model) for VoIP quality |
| **Monitoring** | Bidirectional probe detection, 3-tier agent status (Online/Stale/Offline) |
| **Visualization** | D3 Workspace Network Map, Connectivity Matrix, real-time WebSocket dashboards |
| **Alerting** | Compound AND/OR conditions, MTR-specific (hop loss/latency/route changes), SysInfo (CPU/Memory), Agent Offline detection |
| **Controller APIs** | Centralized GeoIP, WHOIS, OUI lookup, `/whoami` for air-gapped networks |
| **Multi-Tenancy** | Workspace isolation, configurable resource limits, RBAC (Owner/Admin/User/Viewer) |
| **Sharing** | Sharable Agent Pages (P3.6) with token-based access, password protection, speedtest gating |
| **Storage** | ClickHouse time-series backend with lazy cache refresh |
| **Agent Lifecycle** | Deactivation on deletion, kick-and-prevent reconnection |

---

## Phase 1: Core Polish (Q1 2026)

### P1.1 Network Interface Binding 🔄
**Status: Partially Complete** | **Remaining: Medium Effort**

| Completed | Pending |
|-----------|---------|
| Interface discovery (IPs, MAC, type) | Probe-level interface binding |
| Cross-platform route discovery | Multi-WAN support |
| OUI/vendor lookup | Interface validation before probe execution |
| UI visualization | |

**Priority:** High — Critical for SD-WAN and multi-NIC environments

---

### P1.3 Dynamic Thresholds ⏳
**Status: Not Started** | **Effort: Medium**

- [ ] Rolling 7-day baseline calculation per probe
- [ ] Deviation-based alerting (2x standard deviation)
- [ ] Hybrid mode: static OR dynamic threshold per rule
- [ ] Baseline visualization in UI

**Priority:** High — Reduces alert fatigue, enables anomaly detection

---

### P1.4 Email & Registration Configuration ⏳
**Status: Partial (backend scaffolding exists)** | **Effort: Low**

**System-Wide SMTP (via .env):**
- [ ] Connect `sendEmailNotification` to email queue
- [ ] SMTP host, port, user, password env vars (mostly in place)
- [ ] HTML + plaintext email templates
- [ ] Digest mode option (batch alerts)

**Registration & Verification Controls:**
- [ ] `REQUIRE_EMAIL_VERIFICATION` env var (default: `false`)
- [ ] `ENABLE_REGISTRATION` env var (default: `true`)
- [ ] RBAC enforcement: verified email required for workspace creation when enabled
- [ ] Block unverified users from sensitive actions (invites, API key generation)

**Priority:** Medium — Essential for production deployments

---

### P1.5 Smart Notifications ⏳
**Status: Partial** | **Effort: Low**

| Completed | Pending |
|-----------|---------|
| Recovery notifications | Debouncing (suppress duplicates in time window) |
| | Grouping (combine related alerts) |
| | Escalation (Warning → Critical after N minutes) |

**Priority:** Medium — Reduces notification fatigue

---

## Phase 2: Probe Expansion (Q2 2026)

### P2.1 SNMP Probe 🎯
**Priority: Critical** | **Effort: High**

The #1 feature gap vs. established competitors:

- SNMP v2c/v3 support via gosnmp
- Device discovery and OID auto-detection
- Vendor profiles (Cisco, Juniper, Ubiquiti, Arista)
- Metrics: CPU, memory, interface bandwidth/errors/discards
- Ultra-fast polling (30-second intervals)
- Interface up/down/speed tracking

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

### P2.3 HTTP/HTTPS Probe 🔄
**Status: In Progress** | **Remaining: Medium Effort**

| Completed | Pending |
|-----------|---------|
| Response timing breakdown (DNS, TCP, TLS, TTFB) | Migrate `web.go.disabled` to current probe architecture |
| TLS certificate extraction | Register controller handler |
| Status code and header tracking | Dashboard visualization |
| | Shareable HTTP pages |

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
| **P3.2 Scheduled Reports** | Medium | PDF/CSV exports, email delivery, public URLs |
| **P3.3 RBAC Enhancements** | Medium | Operator role, per-role permissions, audit logging |
| **P3.4 SSO Integration** | Medium | SAML 2.0, OIDC/OAuth2, auto-provisioning |
| **P3.5 API Key Management** | Low | Scoped tokens, expiry/rotation, usage logging |
| **P3.6 Sharable Pages** | ✅ Complete | Token-based public links, password protection |
| **P3.7 Observability Integrations** | Medium | Prometheus `/metrics` endpoint, Loki log push, Grafana dashboards |

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
| 1 | P2.1 SNMP Probe | Critical competitor gap |
| 2 | P1.4 Email Notifications | Essential for production use |
| ~~3~~ | ~~P2.2 DNS Probe~~ | ✅ Shipped |
| 4 | P2.3 HTTP/HTTPS Probe | Agent code exists but needs migration |
| 5 | P1.3 Dynamic Thresholds | Differentiating intelligence |
| 6 | P1.1 Interface Binding | Finishes existing feature |
| 7 | P3.7 Prometheus/Loki API | Enterprise integration demand |

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
- [ ] SNMP polling shipped
- [x] DNS probe shipped
- [ ] HTTP/HTTPS probe shipped
- [ ] Email notifications complete
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
