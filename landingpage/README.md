# NetWatcher OSS Landing Page

A static landing page for NetWatcher OSS, served via Caddy with automatic HTTPS.

## Quick Start

### Development (localhost)

```bash
docker-compose up -d
# Access at http://localhost
```

### Production (with SSL)

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` and set your domain:
   ```bash
   DOMAIN=netwatcher.io
   ```

3. Ensure your domain's DNS points to this server

4. Deploy:
   ```bash
   docker-compose up -d
   ```

Caddy will automatically obtain and renew Let's Encrypt certificates.

## Files

| File | Description |
|------|-------------|
| `index.html` | Main landing page |
| `demo.html` | Screenshots & demo page |
| `style.css` | Styling |
| `script.js` | Interactions |
| `Dockerfile` | Container build |
| `docker-compose.yml` | Deployment config |
| `Caddyfile` | Web server config |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOMAIN` | `localhost` | Domain for SSL certificate |

## Features

- **Automatic HTTPS** via Let's Encrypt (production)
- **Self-signed certs** for localhost (development)
- **Security headers** (HSTS, X-Frame-Options, etc.)
- **Gzip/Zstd compression**
- **Static asset caching**
- **Health checks**

## Adding Screenshots

1. Place images in `assets/` directory
2. Update `demo.html` to reference them:
   ```html
   <img src="assets/screenshot-dashboard.png" alt="Dashboard">
   ```
3. Rebuild:
   ```bash
   docker-compose up -d --build
   ```

## Customization

Edit `Caddyfile` to:
- Add redirects
- Configure custom headers
- Set up reverse proxy to other services
- Add IP restrictions
