# NetWatcher Documentation

Welcome to the NetWatcher documentation. This folder contains comprehensive technical documentation for the NetWatcher network monitoring platform.

## Quick Links

| Document | Description |
|----------|-------------|
| [Architecture](./architecture.md) | System components, data flow, and high-level design |
| [API Reference](./api-reference.md) | Complete REST API and WebSocket documentation |
| [Permissions](./permissions.md) | Role-based access control and permission matrix |
| [Site Administration](./site-admin.md) | Site-wide admin panel and user management |
| [Agent Probes](./agent-probes.md) | Probe system, types, and implementation guide |
| [Agent Installation](./agent-installation.md) | Agent deployment and configuration |
| [TrafficSim Architecture](./trafficsim-architecture.md) | Agent-to-agent traffic simulation (disabled) |
| [Panel Architecture](./panel-architecture.md) | Vue 3 panel structure, views, and data flow |
| [Data Models](./data-models.md) | Database schemas and TypeScript interfaces |
| [Deployment](./deployment.md) | Production and development deployment guides |
| [Development Guide](./development.md) | Setup, building, debugging, and contributing |
| [Alerting](./alerting.md) | Alert rules, notifications, and webhook configuration |
| [GeoIP & WHOIS](./geoip-whois.md) | IP geolocation and WHOIS lookup services |
| [Speedtest](./speedtest.md) | Speedtest probe configuration and server selection |
| [Network Map](./network-map.md) | Network topology visualization |
| [Simplifications](./simplifications.md) | Recommendations for code cleanup and improvements |

---

## System Overview

NetWatcher is a distributed network monitoring system with three main components:

```
┌─────────────────────────────────────────────────────────────┐
│                        PANEL                                 │
│                    (Vue 3 Frontend)                         │
│    ┌──────────────────────────────────────────────────┐    │
│    │  Dashboards • Charts • Agent Management • Probes │    │
│    └──────────────────────────────────────────────────┘    │
└─────────────────────────┬───────────────────────────────────┘
                          │ HTTPS / WSS
┌─────────────────────────▼───────────────────────────────────┐
│                      CONTROLLER                              │
│                    (Go + Iris Backend)                      │
│    ┌──────────────────────────────────────────────────┐    │
│    │ REST API • WebSocket Server • Authentication     │    │
│    └──────────────────────────────────────────────────┘    │
│              │                          │                    │
│              ▼                          ▼                    │
│    ┌─────────────────┐        ┌─────────────────┐          │
│    │   PostgreSQL    │        │   ClickHouse    │          │
│    │   (Metadata)    │        │  (Time Series)  │          │
│    └─────────────────┘        └─────────────────┘          │
└─────────────────────────▲───────────────────────────────────┘
                          │ WebSocket
┌─────────────────────────┴───────────────────────────────────┐
│                        AGENTS                                │
│                     (Go Daemons)                            │
│    ┌──────────────────────────────────────────────────┐    │
│    │  MTR • Ping • Speedtest • SysInfo • TrafficSim  │    │
│    └──────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

---

## Documentation Map

### For Developers

1. Start with [Development Guide](./development.md) for setup instructions
2. Review [Architecture](./architecture.md) to understand the system
3. Use [API Reference](./api-reference.md) when building integrations
4. Check [Data Models](./data-models.md) for database schemas

### For Contributors

1. Read [Simplifications](./simplifications.md) for cleanup opportunities
2. Follow the coding standards in [Development Guide](./development.md)
3. Review open TODOs in the codebase

### For Operators

1. See [Deployment Guide](./deployment.md) for production and development setup
2. Review [Architecture](./architecture.md) for system design
3. Check [sample.env](../sample.env) for all configuration options

---

## Key Concepts

### Workspaces

Organizational containers that group agents, probes, and team members. Each user can own multiple workspaces and invite members with different roles.

### Agents

Network monitoring daemons deployed on remote hosts. Agents connect to the controller via WebSocket and execute probes based on their configuration.

### Probes

Monitoring checks configured in the panel. Each probe targets one or more hosts (or other agents) and runs at a configurable interval.

### Probe Types

| Type | Description |
|------|-------------|
| **MTR** | Multi-hop traceroute with latency and loss per hop |
| **PING** | ICMP ping with RTT statistics |
| **SPEEDTEST** | Download/upload speed measurements |
| **SYSINFO** | System information (CPU, memory, OS) |
| **NETINFO** | Network information (public IP, gateway, ISP) |
| **TRAFFICSIM** | Inter-agent traffic simulation |

---

## License

NetWatcher OSS is licensed under the [GNU Affero General Public License v3.0](../LICENSE.md).
