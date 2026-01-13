# GeoIP & WHOIS Lookup

NetWatcher includes GeoIP and WHOIS lookup functionality to provide geolocation and network ownership information for IP addresses in traceroutes and probes.

## Features

- **GeoIP Lookups**: City, country, ASN (Autonomous System Number), and coordinates
- **WHOIS Lookups**: Network name, organization, registrar, and abuse contacts
- **ClickHouse Caching**: Lookup results are cached with automatic TTL expiration
- **Combined Lookup**: Single API call returns both GeoIP and WHOIS data
- **Lookup History**: View past lookups from the cache

## Setup

### 1. Download GeoIP Databases

NetWatcher uses MaxMind GeoLite2 databases. The easiest way to get them is from the P3TERX mirror on GitHub:

```bash
# Create directory for databases
mkdir -p ./data/geoip

# Download databases (one-time or scheduled)
./scripts/update-geoip.sh
```

### 2. Configure Automatic Updates

Add a cron job to keep databases updated (they are refreshed weekly):

```bash
# Edit crontab
crontab -e

# Add weekly update (Sundays at 3 AM)
0 3 * * 0 /path/to/netwatcher/scripts/update-geoip.sh >> /var/log/geoip-update.log 2>&1
```

### 3. Environment Configuration

The following environment variables configure GeoIP paths (already set in `.env.example`):

```env
GEOIP_CITY_PATH=/data/geoip/GeoLite2-City.mmdb
GEOIP_COUNTRY_PATH=/data/geoip/GeoLite2-Country.mmdb
GEOIP_ASN_PATH=/data/geoip/GeoLite2-ASN.mmdb
```

### 4. Docker Volume Mount

The docker-compose.yml already includes the volume mount:

```yaml
controller:
  volumes:
    - ./data/geoip:/data/geoip:ro
```

## API Endpoints

### GeoIP

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/geoip/lookup?ip={ip}` | GET | Single IP GeoIP lookup |
| `/geoip/lookup` | POST | Bulk IP lookup (body: `{"ips": [...]}`) |
| `/geoip/history?ip={ip}` | GET | Past lookups for an IP |
| `/geoip/status` | GET | Check database availability |

### WHOIS

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/whois/lookup?query={ip}` | GET | WHOIS lookup |
| `/whois/history?query={ip}` | GET | Past lookups |

### Combined

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/lookup/combined?ip={ip}` | GET | GeoIP + WHOIS in one call |

## Frontend Usage

### Standalone Lookup Page

Navigate to `/lookup` in the panel to search for IP information.

### API Example

```javascript
// Combined lookup
const response = await fetch('/lookup/combined?ip=8.8.8.8', {
  headers: { Authorization: `Bearer ${token}` }
});
const data = await response.json();
// {
//   ip: "8.8.8.8",
//   geoip: { country: { code: "US", name: "United States" }, ... },
//   whois: { parsed: { organization: "Google LLC", ... }, ... },
//   cached: true,
//   cache_time: "2026-01-13T08:00:00Z"
// }
```

## Cache Behavior

- **GeoIP Cache TTL**: 30 days
- **WHOIS Cache TTL**: 7 days
- Results are cached automatically after lookup
- Cached results include `cached: true` and `cache_time` fields

## WHOIS Requirements

WHOIS lookups require the `whois` command-line tool installed on the controller host/container:

```bash
# Debian/Ubuntu
apt-get install whois

# Alpine (Docker)
apk add whois
```
