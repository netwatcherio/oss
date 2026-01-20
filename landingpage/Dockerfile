# NetWatcher OSS Landing Page
# Multi-stage build with Caddy for automatic HTTPS

FROM caddy:2-alpine

# Copy static files
COPY index.html /srv/
COPY demo.html /srv/
COPY style.css /srv/
COPY script.js /srv/
COPY assets/ /srv/assets/

# Copy Caddyfile
COPY Caddyfile /etc/caddy/Caddyfile

# Expose ports
EXPOSE 80
EXPOSE 443

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:80/ || exit 1
