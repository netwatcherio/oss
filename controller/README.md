# NetWatcher Controller

The NetWatcher Controller is a Go backend API server that manages workspaces, agents, probes, and authentication. It uses the Iris framework with PostgreSQL for metadata and ClickHouse for time-series probe data.

## Quick Start

```bash
# Set environment variables
cp .env.example .env
# Edit .env with your settings

# Development
go run .

# Production build
go build -ldflags="-s -w" -o netwatcher-controller .
```

## Environment Variables

| Variable | Required | Description |
|----------|:--------:|-------------|
| **App** |||
| `LISTEN` | ✅ | Bind address (default: `0.0.0.0:8080`) |
| `KEY` | ✅ | JWT signing key (32+ chars) |
| `PIN_PEPPER` | ✅ | Agent PIN hashing pepper |
| `DEBUG` | | Enable debug logging |
| **PostgreSQL** |||
| `POSTGRES_HOST` | ✅ | Database host |
| `POSTGRES_PORT` | | Port (default: 5432) |
| `POSTGRES_USER` | ✅ | Username |
| `POSTGRES_PASSWORD` | ✅ | Password |
| `POSTGRES_DB` | ✅ | Database name |
| **ClickHouse** |||
| `CLICKHOUSE_HOST` | ✅ | ClickHouse host |
| `CLICKHOUSE_USER` | | Username (default: `default`) |
| `CLICKHOUSE_PASSWORD` | | Password |
| `CLICKHOUSE_DB` | | Database (default: `netwatcher`) |

See [.env.example](./.env.example) for all options.

## Docker Build

```bash
# Build image
docker build -t netwatcher-controller .

# Run container
docker run -d \
  --name controller \
  -p 8080:8080 \
  --env-file .env \
  netwatcher-controller
```

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Health check |
| `POST /auth/register` | User registration |
| `POST /auth/login` | User login |
| `POST /agent` | Agent bootstrap/auth |
| `/ws/*` | WebSocket connections |
| `/workspaces/*` | Workspace CRUD |
| `/workspaces/{id}/agents/*` | Agent management |
| `/workspaces/{id}/members/*` | Member management |

See [API Reference](../docs/api-reference.md) for complete documentation.

## Permissions

All workspace-scoped endpoints enforce role-based access control:

| Role | Level | Capabilities |
|------|:-----:|--------------|
| OWNER | 4 | Full control, delete workspace |
| ADMIN | 3 | Manage members, delete agents/probes |
| USER | 2 | Create/edit agents and probes |
| VIEWER | 1 | Read-only access |

See [Permissions](../docs/permissions.md) for detailed matrix.

## Directory Structure

```
controller/
├── main.go              # Entry point
├── internal/
│   ├── agent/           # Agent management
│   ├── probe/           # Probe CRUD and data
│   ├── user/            # User auth and profiles
│   └── workspace/       # Workspaces and members
├── web/
│   ├── agents.go        # Agent routes
│   ├── permissions.go   # Permission middleware
│   ├── probes.go        # Probe routes
│   └── workspaces.go    # Workspace routes
└── Dockerfile           # Multi-stage build
```

## Development

```bash
# Run with hot reload (requires air)
air

# Run tests
go test ./...

# Build for production
CGO_ENABLED=0 go build -ldflags="-s -w" -o main .
```

## Database Migrations

The controller auto-migrates database schemas on startup via GORM. No manual migration is required, but ensure both PostgreSQL and ClickHouse are accessible before starting.
