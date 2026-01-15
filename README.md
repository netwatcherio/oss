# NetWatcher

## Overview

NetWatcher is an open-source distributed network monitoring platform consisting of three components:

- **Controller** – Go backend (Iris framework) with REST API and WebSocket server
- **Panel** – Vue 3 frontend for dashboards, agent management, and visualization
- **Agent** – Go daemon that executes monitoring probes and reports data

The system uses **PostgreSQL** for metadata storage and **ClickHouse** for time-series probe data.

## Quick Start

```bash
# Clone the repository
git clone https://github.com/netwatcherio/netwatcher-oss.git
cd netwatcher-oss

# Start infrastructure with Docker
docker-compose up -d

# Configure and run controller
cd controller
cp .env.example .env
# Edit .env with your settings
go run .

# In another terminal, run the panel
cd panel
npm install
npm run dev
```

See [docs/development.md](./docs/development.md) for detailed setup instructions.

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](./docs/architecture.md) | System design and data flow |
| [API Reference](./docs/api-reference.md) | REST API endpoints with required roles |
| [Data Models](./docs/data-models.md) | Database schemas and types |
| [Deployment](./docs/deployment.md) | Production deployment guide |
| [Development](./docs/development.md) | Setup, building, and testing |
| [Permissions](./docs/permissions.md) | Role-based access control |
| [Panel Architecture](./docs/panel-architecture.md) | Frontend design and patterns |
| [Agent Probes](./docs/agent-probes.md) | Probe types and configuration |
| [TrafficSim](./docs/trafficsim-architecture.md) | Inter-agent traffic simulation |
| [Alerting](./docs/alerting.md) | Alert rules and notifications |

## Environment Variables

### Controller

```bash
# App settings
LISTEN=0.0.0.0:8080
KEY=<jwt-signing-key>       # Required: 32+ chars
PIN_PEPPER=<pin-pepper>     # Required: random string

# PostgreSQL
DB_DRIVER=postgres
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=netwatcher
POSTGRES_PASSWORD=<password>
POSTGRES_DB=netwatcher

# ClickHouse
CLICKHOUSE_HOST=localhost
CLICKHOUSE_PORT=9000
CLICKHOUSE_USER=default
CLICKHOUSE_PASSWORD=
CLICKHOUSE_DB=netwatcher
```

See [controller/.env.example](./controller/.env.example) for all options.

### Panel

```bash
CONTROLLER_ENDPOINT=https://api.netwatcher.io
```

## Docker Compose

```yaml
version: '3.8'
services:
  controller:
    build: ./controller
    environment:
      LISTEN: "0.0.0.0:8080"
      DB_DRIVER: "postgres"
      POSTGRES_HOST: "postgres"
      KEY: "${JWT_SECRET}"
    depends_on:
      - postgres
      - clickhouse

  panel:
    build: ./panel
    ports:
      - "3000:3000"

  postgres:
    image: postgres:14
    environment:
      POSTGRES_PASSWORD: ${PG_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data

  clickhouse:
    image: clickhouse/clickhouse-server:latest
    volumes:
      - chdata:/var/lib/clickhouse

  caddy:
    image: caddy:2-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile

volumes:
  pgdata:
  chdata:
```

## Permissions

NetWatcher uses role-based access control (RBAC) for workspace-scoped operations:

| Role | Level | Capabilities |
|------|:-----:|--------------|
| **OWNER** | 4 | Full control, delete workspace, transfer ownership |
| **ADMIN** | 3 | Manage members, delete agents & probes |
| **USER** | 2 | Create/edit agents and probes |
| **VIEWER** | 1 | Read-only access |

See [docs/permissions.md](./docs/permissions.md) for the complete permission matrix.

## Probe Types

| Type | Description |
|------|-------------|
| MTR | Multi-hop traceroute with per-hop latency and loss |
| PING | ICMP ping with RTT statistics |
| SPEEDTEST | Download/upload speed measurements |
| SYSINFO | System information (CPU, memory, OS) |
| NETINFO | Network information (public IP, gateway, ISP) |
| TRAFFICSIM | Inter-agent traffic simulation |

## Project Structure

```
netwatcher-oss/
├── agent/              # Monitoring agent daemon
├── controller/         # Backend API server
│   ├── internal/       # Business logic
│   └── web/            # HTTP routes & middleware
├── panel/              # Vue.js frontend
│   ├── src/
│   │   ├── composables/  # Vue composables
│   │   ├── router/       # Route guards
│   │   ├── services/     # API services
│   │   └── views/        # Page components
├── docs/               # Documentation
├── docker-compose.yml  # Production deployment
├── docker-compose.dev.yml  # Development (DB only)
└── Caddyfile           # Reverse proxy config
```

## License

[GNU Affero General Public License v3.0](./LICENSE.md)
