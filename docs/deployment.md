# Deployment Guide

This guide covers deploying NetWatcher OSS for both development and production environments.

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │              Caddy                   │
                    │     (TLS + reverse proxy)           │
                    │  api.example.com → controller:8080  │
                    │  app.example.com → panel:3000       │
                    └──────────────┬──────────────────────┘
                                   │
           ┌───────────────────────┴───────────────────────┐
           ▼                                               ▼
    ┌─────────────┐                                 ┌─────────────┐
    │ Controller  │                                 │   Panel     │
    │  (Go API)   │                                 │  (Vue SPA)  │
    │   :8080     │                                 │   :3000     │
    └──────┬──────┘                                 └─────────────┘
           │
    ┌──────┴──────┐
    ▼             ▼
┌────────┐  ┌───────────┐
│Postgres│  │ClickHouse │
│ :5432  │  │:8123/:9000│
└────────┘  └───────────┘
```

---

## Development Setup

### Prerequisites
- Docker & Docker Compose
- Go 1.23+
- Node.js 22+

### 1. Start Databases

```bash
# Start PostgreSQL and ClickHouse only
docker compose -f docker-compose.dev.yml up -d
```

### 2. Run Controller (with hot-reload)

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

# Run
go run .
```

Controller runs at: `http://localhost:8080`

### 3. Run Panel (with hot-reload)

```bash
cd panel
npm install
npm run dev
```

Panel runs at: `http://localhost:5173`

### 4. Access Application

- Panel: http://localhost:5173
- API: http://localhost:8080
- API Health: http://localhost:8080/healthz

---

## Production Deployment

### Prerequisites
- Docker & Docker Compose
- Domain names configured (e.g., `api.example.com`, `app.example.com`)
- (Optional) Cloudflare for CDN/DDoS protection

### 1. Configure Environment

```bash
# Copy sample environment
cp sample.env .env

# Edit with production values
nano .env
```

**Required variables:**

| Variable | Description |
|----------|-------------|
| `API_DOMAIN` | API domain (e.g., `api.netwatcher.io`) |
| `APP_DOMAIN` | Panel domain (e.g., `app.netwatcher.io`) |
| `POSTGRES_PASSWORD` | Strong database password |
| `CLICKHOUSE_PASSWORD` | Strong ClickHouse password |
| `JWT_SECRET` | 32+ character random string |
| `PIN_PEPPER` | Random string for PIN hashing |
| `CONTROLLER_ENDPOINT` | Public API URL for panel |

**Generate secrets:**
```bash
# JWT_SECRET
openssl rand -hex 32

# PIN_PEPPER  
openssl rand -hex 16
```

### 2. Build Images

```bash
docker build -t netwatcher-controller ./controller
docker build -t netwatcher-panel ./panel
```

### 3. Deploy

```bash
# Start all services
docker compose up -d

# View logs
docker compose logs -f

# Check status
docker compose ps
```

### 4. Verify

```bash
# Test API health
curl https://api.example.com/healthz

# Open panel
open https://app.example.com
```

### 5. Agent Setup

After creating an agent in the panel, a setup modal displays the bootstrap PIN and installation commands:

**Docker Installation:**
```bash
docker run -d --name netwatcher-agent \
  -e CONTROLLER_HOST="api.example.com" \
  -e CONTROLLER_SSL="true" \
  -e WORKSPACE_ID="1" \
  -e AGENT_ID="10" \
  -e AGENT_PIN="123456789" \
  --restart unless-stopped \
  netwatcher/agent:latest
```

**Binary Installation:**
```bash
CONTROLLER_HOST="api.example.com" \
CONTROLLER_SSL="true" \
WORKSPACE_ID="1" \
AGENT_ID="10" \
AGENT_PIN="123456789" \
./netwatcher-agent
```

The PIN is valid for 24 hours. After successful bootstrap, the agent receives a PSK for persistent authentication.

---

## Configuration Reference

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| **Domains** |||
| `API_DOMAIN` | - | Controller API domain |
| `APP_DOMAIN` | - | Panel frontend domain |
| `CONTROLLER_UPSTREAM` | `controller:8080` | Internal controller address |
| `CLIENT_UPSTREAM` | `panel:3000` | Internal panel address |
| **PostgreSQL** |||
| `POSTGRES_HOST` | `postgres` | Database host |
| `POSTGRES_PORT` | `5432` | Database port |
| `POSTGRES_USER` | - | Database user |
| `POSTGRES_PASSWORD` | - | Database password |
| `POSTGRES_DB` | - | Database name |
| **ClickHouse** |||
| `CLICKHOUSE_HOST` | `clickhouse` | ClickHouse host |
| `CLICKHOUSE_USER` | `default` | ClickHouse user |
| `CLICKHOUSE_PASSWORD` | - | ClickHouse password |
| **Security** |||
| `JWT_SECRET` | - | JWT signing key (32+ chars) |
| `PIN_PEPPER` | - | Agent PIN pepper |
| **GeoIP** |||
| `GEOIP_CITY_PATH` | - | Path to GeoLite2-City.mmdb |
| `GEOIP_COUNTRY_PATH` | - | Path to GeoLite2-Country.mmdb |
| `GEOIP_ASN_PATH` | - | Path to GeoLite2-ASN.mmdb |
| **OUI** |||
| `OUI_PATH` | - | Path to oui.txt (IEEE MAC vendor database) |
| **Panel** |||
| `CONTROLLER_ENDPOINT` | - | Public API URL |
| **Debug** |||
| `DEBUG` | `false` | Enable debug logging |
| `GORM_LOG_LEVEL` | `warn` | Database log level |

---

## File Structure

```
netwatcher-oss/
├── docker-compose.yml      # Production deployment
├── docker-compose.dev.yml  # Development (databases only)
├── Caddyfile               # Reverse proxy config
├── sample.env              # Environment template
├── controller/
│   └── Dockerfile          # Controller image
└── panel/
    └── Dockerfile          # Panel image
```

---

## Troubleshooting

### Database Connection Failed
```bash
# Check PostgreSQL
docker compose exec postgres pg_isready

# Check ClickHouse
docker compose exec clickhouse clickhouse-client --query "SELECT 1"
```

### Controller Won't Start
```bash
docker compose logs controller
# Common: missing env vars, database not ready
```

### Panel Blank Page
- Verify `CONTROLLER_ENDPOINT` is set correctly
- Check browser console for CORS errors
- Ensure API is accessible from browser

### WebSocket Issues
- Verify Caddy WebSocket headers in Caddyfile
- Check firewall allows WebSocket connections
