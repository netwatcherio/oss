# NetWatcher Development Guide

## Prerequisites

- Go 1.23+
- Node.js 22+
- PostgreSQL 17+
- ClickHouse 24+
- Docker & Docker Compose

---

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/netwatcherio/netwatcher-oss.git
cd netwatcher-oss
```

### 2. Start Databases

```bash
# Development mode - databases only
docker compose -f docker-compose.dev.yml up -d
```

This starts:
- PostgreSQL on port 5432
- ClickHouse on ports 8123/9000

### 3. Run Controller

```bash
cd controller

# Set environment variables
export POSTGRES_HOST=localhost
export POSTGRES_USER=netwatcher
export POSTGRES_PASSWORD=devpassword
export POSTGRES_DB=netwatcher
export CLICKHOUSE_HOST=localhost
export CLICKHOUSE_USER=default
export CLICKHOUSE_PASSWORD=devpassword
export JWT_SECRET=dev-secret-change-in-prod
export PIN_PEPPER=dev-pepper-change-in-prod
export DEBUG=true

go run .
```

Controller runs at: http://localhost:8080

### 4. Run Panel

```bash
cd panel
npm install
npm run dev
```

Panel runs at: http://localhost:5173

---

## Environment Variables

### Controller

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `LISTEN` | No | `0.0.0.0:8080` | HTTP listen address |
| `POSTGRES_HOST` | Yes | - | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_USER` | Yes | - | PostgreSQL user |
| `POSTGRES_PASSWORD` | Yes | - | PostgreSQL password |
| `POSTGRES_DB` | Yes | - | PostgreSQL database |
| `POSTGRES_DSN` | No | - | Full DSN (alternative to individual vars) |
| `CLICKHOUSE_HOST` | Yes | - | ClickHouse host |
| `CLICKHOUSE_USER` | No | `default` | ClickHouse user |
| `CLICKHOUSE_PASSWORD` | Yes | - | ClickHouse password |
| `JWT_SECRET` | Yes | - | JWT signing key (32+ chars) |
| `PIN_PEPPER` | Yes | - | Agent PIN hashing salt |
| `DEBUG` | No | `false` | Enable debug logging |
| `GORM_LOG_LEVEL` | No | `warn` | Database log level |

### Panel

| Variable | Description |
|----------|-------------|
| `CONTROLLER_ENDPOINT` | API URL for production (set in container) |

---

## Project Structure

```
netwatcher-oss/
├── agent/                 # Monitoring daemon
│   ├── main.go           # Entrypoint
│   ├── lib/platform/     # Platform detection utilities
│   ├── probes/           # Probe implementations
│   └── workers/          # Background workers
├── controller/           # Backend API
│   ├── main.go          # Entrypoint
│   ├── internal/        # Business logic
│   │   ├── admin/       # Site admin bootstrap, stats
│   │   ├── agent/       # Agent management
│   │   ├── alert/       # Alert rules and incidents
│   │   ├── database/    # PostgreSQL connection
│   │   ├── email/       # Email queue and SMTP
│   │   ├── errors/      # Centralized error types
│   │   ├── geoip/       # MaxMind GeoIP lookups
│   │   ├── probe/       # Probes + ClickHouse
│   │   ├── scheduler/   # Data retention cleanup
│   │   ├── speedtest/   # Speedtest queue
│   │   ├── users/       # User authentication
│   │   ├── whois/       # WHOIS lookups
│   │   └── workspace/   # Workspace management
│   └── web/             # HTTP handlers + WebSocket
├── panel/               # Vue.js frontend
│   ├── src/
│   │   ├── components/  # Reusable UI components
│   │   ├── composables/ # Vue composables (hooks)
│   │   ├── lib/         # Utility libraries
│   │   ├── router/      # Vue Router configuration
│   │   ├── services/    # API service layer
│   │   ├── utils/       # Helper utilities
│   │   ├── views/       # Page components
│   │   └── types.ts     # TypeScript definitions
│   └── vite.config.ts
├── docs/                # Documentation
├── docker-compose.yml   # Production deployment
├── docker-compose.dev.yml # Development (databases only)
├── Caddyfile           # Reverse proxy config
└── .env.example        # Environment template
```

---

## Development Workflow

### Controller Development

```bash
cd controller

# Run with hot reload (install air first)
go install github.com/cosmtrek/air@latest
air

# Or manually
go run .

# Run tests
go test ./...

# Build for production
go build -ldflags="-s -w" -o dist/netwatcher-controller
```

### Panel Development

```bash
cd panel

# Development server with HMR
npm run dev

# Type checking
npm run typecheck

# Build for production
npm run build

# Preview production build
npm run preview
```

### Agent Development

```bash
cd agent

# Run locally
go run .

# Build for current platform
go build -o netwatcher-agent

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o netwatcher-agent-linux
```

---

## Testing

### API Testing with cURL

```bash
# Health check
curl http://localhost:8080/healthz

# Register user
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123","name":"Test User"}'

# Login
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123"}' | jq -r '.token')

# Create workspace
curl -X POST http://localhost:8080/workspaces \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-workspace"}'

# List workspaces
curl http://localhost:8080/workspaces \
  -H "Authorization: Bearer $TOKEN"
```

### Database Queries

```bash
# PostgreSQL
docker compose exec postgres psql -U netwatcher -d netwatcher

# ClickHouse
docker compose exec clickhouse clickhouse-client

# Query probe data
SELECT * FROM probe_data ORDER BY created_at DESC LIMIT 10;
```

---

## Debugging

### Controller Logs

```bash
# Run with debug logging
DEBUG=true go run .

# View database queries
GORM_LOG_LEVEL=info go run .
```

### Common Issues

**Database Connection Failed:**
```bash
# Check PostgreSQL
docker compose exec postgres pg_isready

# Check ClickHouse
curl http://localhost:8123/ping
```

**Agent Can't Connect:**
1. Check controller is running
2. Verify WebSocket URL uses `ws://` or `wss://`
3. Confirm PSK is valid

**Panel Blank Page:**
- Check browser console for errors
- Verify API endpoint is accessible
- Check CORS configuration

---

## IDE Setup

### VS Code Extensions

- Go (by Go team)
- Vue - Official
- TypeScript Vue Plugin (Volar)
- Prettier
- ESLint

### Recommended Settings

```json
// .vscode/settings.json
{
  "go.lintTool": "golangci-lint",
  "editor.formatOnSave": true,
  "[go]": {
    "editor.defaultFormatter": "golang.go"
  },
  "[vue]": {
    "editor.defaultFormatter": "Vue.volar"
  }
}
```

---

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Run tests: `go test ./...`
5. Build panel: `cd panel && npm run build`
6. Submit a pull request

### Code Style

- **Go**: Follow [Effective Go](https://go.dev/doc/effective_go)
- **TypeScript**: Prettier + ESLint
- **Vue**: Composition API with `<script setup>`
- **Commits**: Conventional commits recommended
