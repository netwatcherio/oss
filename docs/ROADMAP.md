# NetWatcher OSS Roadmap

> Last Updated: January 2026

This document outlines the planned features, improvements, and priorities for NetWatcher OSS development.

---

## Current State (v1.0)

### ✅ Implemented Features
- **Distributed Agents** - Cross-platform (Windows, Linux, macOS)
- **Probe Types** - PING, MTR, TrafficSim (synthetic UDP)
- **Bidirectional Monitoring** - Forward + return path analysis
- **Workspace Network Map** - D3 topology with multiple layouts
- **Connectivity Matrix** - High-density mesh health view
- **Alerting System** - Threshold-based with webhook notifications
- **Multi-Tenant** - Workspace isolation with member management
- **ClickHouse Backend** - Scalable time-series telemetry storage
- **Real-time Updates** - WebSocket-based live dashboards

---

## Phase 1: Core Polish (Q1 2026)

Focus: Production-readiness and UX improvements

### P1.1 Network Interface Selection
**Priority: High** | **Effort: Medium** | **Status: Complete** ✅

Enhanced network interface detection and probe-level binding:

- [x] **Agent Interface Discovery** - Enumerate all available network interfaces with metadata:
  - Name, IP addresses (v4/v6), MAC address, gateway, type (ethernet, wifi, loopback)
  - Routing table entries per interface
- [x] **Cross-Platform Route Discovery** - Windows (PowerShell/netsh), Linux (/proc/net/route, ip route), macOS (netstat)
- [x] **Backward Compatibility** - `NormalizeFromLegacy()` converts old agent data to new format
- [x] **OUI Lookup** - Backend API for MAC vendor identification using IEEE database
- [x] **UI Display** - Panel shows interfaces, routes, and vendor names on agent detail page
- [ ] **Probe Interface Binding** - New `interface` field on probes allowing explicit interface selection
- [ ] **Multi-WAN Support** - Enable monitoring over specific ISP links
- [ ] **Validation** - Verify interface exists on agent before probe execution

**Use Cases:**
- Servers with multiple NICs (management vs production)
- SD-WAN deployments with multiple ISP uplinks
- VPN vs direct internet path comparison
- Dual-stack (IPv4/IPv6) interface selection

### P1.2 MOS Score Calculation ✅
**Priority: High** | **Effort: Low** | **Status: Complete**

VoIP quality metric from existing telemetry:

- [x] Calculate MOS from latency/jitter/loss using ITU-T G.107 E-Model
- [x] Add MOS field to PING and TrafficSim payloads
- [x] Display MOS in probe dashboards with quality color coding
- [x] Aggregated MOS graph combining ICMP and TrafficSim data sources
- [ ] Add MOS threshold support in alerting

### P1.3 Dynamic Thresholds
**Priority: High** | **Effort: Medium**

Baseline-relative alerting:

- [ ] Calculate rolling baseline per probe (7-day average)
- [ ] Alert on deviation from baseline (e.g., 2x standard deviation)
- [ ] Hybrid mode: static OR dynamic threshold per rule
- [ ] UI for baseline visualization

### P1.4 Email Notifications
**Priority: Medium** | **Effort: Low**

Complete the partial implementation:

- [ ] Connect `sendEmailNotification` to email queue
- [ ] SMTP configuration in workspace settings
- [ ] Email templates for alerts (HTML + plaintext)
- [ ] Digest mode option (batch alerts)

### P1.5 Smart Notifications
**Priority: Medium** | **Effort: Low**

Reduce alert fatigue:

- [ ] Debounce: Suppress duplicate alerts within time window
- [ ] Grouping: Combine related alerts (same target, same timeframe)
- [ ] Escalation: Warning → Critical after N minutes
- [ ] Recovery notifications

### P1.6 Controller API Services ✅
**Priority: High** | **Effort: Medium** | **Status: Complete**

Centralized APIs to eliminate external service dependencies:

**Public IP Discovery:**
- [x] Controller endpoint: `GET /agent/api/whoami` returns agent's public IP as seen by controller
- [x] Agents call controller on startup instead of external APIs (ifconfig.me, speedtest, etc.)
- [x] Removes dependency on third-party services
- [x] Works in air-gapped/restricted networks

**GeoIP & Location Services:**
- [x] Controller-hosted MaxMind GeoLite2 database (existing)
- [x] Endpoint: `GET /geoip/lookup?ip={ip}` returns city, region, country, coordinates
- [x] Used by agents for self-location and by panel for hop enrichment
- [x] ASN lookup included in GeoIP responses
- [x] Lazy cache refresh (entries >30 days refreshed on access, stale fallback if refresh fails)

**IP Intelligence API:**
- [x] Unified endpoint: `GET /lookup/ip/{ip}` returns combined data:
  - GeoIP (city, region, country, lat/lon)
  - ASN (number, name, organization)
  - Reverse DNS (PTR record)
  - Optional: threat score integration
- [x] Caching layer for frequently queried IPs (ClickHouse cache)
- [x] Bulk lookup support for MTR hop enrichment (`POST /geoip/lookup`)

**Benefits:**
- No external API rate limits or costs
- Works offline/air-gapped after initial DB download
- Consistent data source across all components
- Privacy: No IP data sent to external services

---

## Phase 2: Probe Expansion (Q2 2026)

Focus: New probe types for broader monitoring coverage

### P2.1 SNMP Probe Type
**Priority: Critical** | **Effort: High**

#1 feature gap vs competitors (Obkio, Kentik):

- [ ] SNMP v2c/v3 support in agent (gosnmp library)
- [ ] Device discovery and OID auto-detection
- [ ] Pre-built profiles for common vendors (Cisco, Juniper, Ubiquiti, etc.)
- [ ] Metrics: CPU, memory, interface bandwidth, errors, discards
- [ ] Ultra-fast polling option (30s intervals)
- [ ] Device inventory management
- [ ] Interface status tracking (up/down/speed)

### P2.2 DNS Probe Type
**Priority: High** | **Effort: Medium**

DNS monitoring:

- [ ] Query types: A, AAAA, MX, TXT, CNAME, NS
- [ ] Metrics: Resolution time, TTL, response code
- [ ] Record validation (expected vs actual)
- [ ] Multiple resolver support per probe
- [ ] Propagation delay detection

### P2.3 HTTP/HTTPS Probe Type
**Priority: High** | **Effort: Low**

Endpoint health checks:

- [ ] GET/POST/HEAD methods
- [ ] Status code validation
- [ ] Response time measurement
- [ ] Response body pattern matching
- [ ] TLS certificate expiry monitoring
- [ ] Custom headers support
- [ ] Basic/Bearer auth options

### P2.4 AS Path Resolution
**Priority: Medium** | **Effort: Low**

Enhanced path intelligence:

- [ ] IP-to-ASN lookup (Team Cymru / local database)
- [ ] Display ASN in MTR hop table
- [ ] Alert on AS path changes
- [ ] AS-level grouping in Network Map

### P2.5 RIR REST API Integration
**Priority: Medium** | **Effort: Medium**

Replace/augment command-line WHOIS with RIR REST APIs for faster, structured data:

**ARIN (North America):**
- [ ] REST API: `https://whois.arin.net/rest/ip/{ip}`
- [ ] Extract: Network name, handle, CIDR, organization, customer ref
- [ ] Faster than whois command with structured XML/JSON response

**Other RIRs:**
- [ ] RIPE (Europe/Middle East): `https://rest.db.ripe.net/search`
- [ ] APNIC (Asia-Pacific): `https://wq.apnic.net/query`
- [ ] LACNIC (Latin America): RDAP API
- [ ] AFRINIC (Africa): RDAP API

**Features:**
- [ ] Auto-detect RIR based on IP range
- [ ] Fallback chain: RIR REST API → command-line whois
- [ ] Cache responses in ClickHouse
- [ ] Display logical network name in hop tables and agent info

---

## Phase 3: Enterprise Features (Q3-Q4 2026)

Focus: Mid-market and enterprise adoption

### P3.1 Custom Dashboards
**Priority: Medium** | **Effort: Medium**

User-configurable layouts:

- [ ] Widget library (charts, maps, matrices, stat cards)
- [ ] Drag-and-drop layout editor
- [ ] Per-workspace dashboard configurations
- [ ] Time range synchronization across widgets
- [ ] Dashboard templates

### P3.2 Scheduled Reports
**Priority: Medium** | **Effort: Medium**

Automated reporting:

- [ ] Report builder with data source selection
- [ ] Schedule options: daily, weekly, monthly
- [ ] Output formats: PDF, CSV, JSON
- [ ] Email delivery with attachment
- [ ] Public shareable report URLs

### P3.3 RBAC Enhancements
**Priority: Medium** | **Effort: Medium**

Granular permissions:

- [ ] Workspace roles: Owner, Admin, Operator, Viewer
- [ ] Per-role permissions matrix
- [ ] Read-only dashboard access
- [ ] Audit logging for admin actions

### P3.4 SSO Integration
**Priority: Medium** | **Effort: Medium**

Enterprise authentication:

- [ ] SAML 2.0 support
- [ ] OIDC/OAuth2 support
- [ ] Auto-provisioning from IdP
- [ ] Group-to-role mapping

### P3.5 API Key Management
**Priority: Low** | **Effort: Low**

Programmatic access:

- [ ] Per-user API tokens
- [ ] Token scopes (read-only, full access)
- [ ] Token expiry and rotation
- [ ] Usage logging

### P3.6 Sharable Agent Pages
**Priority: Medium** | **Effort: Medium**

Time-limited public access to agent views:

- [ ] Generate shareable link for any agent page
- [ ] Configurable expiration (1 hour, 24 hours, 7 days, custom)
- [ ] Optional password protection
- [ ] Read-only access (no configuration changes)
- [ ] Link revocation capability
- [ ] Access logging (views, IPs)
- [ ] Customizable data scope (all metrics vs specific probes)

**Use Cases:**
- NOC handoffs during incidents
- Share agent status with vendors/ISPs for troubleshooting
- Temporary client access without workspace invitation
- Public status pages for specific endpoints

---

## Phase 4: Scale & Polish (2027+)

### P4.1 Horizontal Scaling
- [ ] ClickHouse cluster deployment guide
- [ ] Controller load balancing patterns
- [ ] Agent connection pooling

### P4.2 Cloud Deployment Templates
- [ ] Terraform modules (AWS, Azure, GCP)
- [ ] Kubernetes DaemonSet for agents
- [ ] Helm chart for controller stack

### P4.3 Speed Tests
- [ ] Upload/download bandwidth testing
- [ ] Scheduled and on-demand modes
- [ ] Integration with TrafficSim

### P4.4 Geographic Visualization
- [ ] GeoIP-based agent/hop locations
- [ ] Map overlay with latency gradients
- [ ] Optional enhancement (topology view preferred)

---

## Not Planned

Features explicitly out of scope:

| Feature | Reason |
|---------|--------|
| AI/NLP Queries | High cost, low differentiation vs good UX |
| Full NetFlow Ingestion | Separate product vertical |
| DDoS Mitigation | Requires carrier-grade infrastructure |
| CDN/OTT Tracking | Needs DPI or flow analysis |
| Mobile Native Apps | Consider PWA instead |

---

## Success Metrics

### 6-Month (Phase 2 Complete)
- [ ] SNMP polling shipped
- [ ] DNS + HTTP probes shipped
- [ ] MOS score implemented
- [ ] 500+ GitHub stars

### 12-Month (Phase 3 Complete)
- [ ] Custom dashboards shipped
- [ ] SSO integration shipped
- [ ] 2,000+ GitHub stars
- [ ] 5+ documented production deployments

---

## Contributing

We welcome community contributions! Priority areas:

1. **SNMP device profiles** - Vendor-specific OID mappings
2. **Probe types** - New monitoring capabilities
3. **Integrations** - Notification channels, export formats
4. **Documentation** - Deployment guides, tutorials

See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
