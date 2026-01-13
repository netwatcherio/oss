#!/bin/bash
# scripts/update-geoip.sh
# Fetches GeoLite2 database files from P3TERX/GeoLite.mmdb GitHub repo
# Run this script via cron on the host machine to keep databases updated
#
# Example cron entry (weekly update, Sundays at 3 AM):
#   0 3 * * 0 /path/to/scripts/update-geoip.sh >> /var/log/geoip-update.log 2>&1
#
# The databases are mounted into the Docker container via volume mount.

set -e

# Configuration
GEOIP_DIR="${GEOIP_DIR:-/opt/oss/geoip}"
GITHUB_BASE="https://github.com/P3TERX/GeoLite.mmdb/raw/download"

# Database files to download
declare -A DATABASES=(
    ["GeoLite2-City.mmdb"]="GeoLite2-City.mmdb"
    ["GeoLite2-Country.mmdb"]="GeoLite2-Country.mmdb"
    ["GeoLite2-ASN.mmdb"]="GeoLite2-ASN.mmdb"
)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

# Create directory if it doesn't exist
mkdir -p "$GEOIP_DIR"

log_info "Starting GeoIP database update..."
log_info "Target directory: $GEOIP_DIR"

# Download each database
success_count=0
for filename in "${!DATABASES[@]}"; do
    target="${DATABASES[$filename]}"
    url="${GITHUB_BASE}/${filename}"
    target_path="${GEOIP_DIR}/${target}"
    tmp_path="${target_path}.tmp"
    
    log_info "Downloading ${filename}..."
    
    if curl -sL --fail -o "$tmp_path" "$url"; then
        # Validate the file is a valid MMDB (check magic bytes)
        if head -c 16 "$tmp_path" | grep -q ""; then
            # Move temp file to final location
            mv "$tmp_path" "$target_path"
            log_info "Successfully updated ${target}"
            ((success_count++))
        else
            log_error "Downloaded file ${filename} appears invalid"
            rm -f "$tmp_path"
        fi
    else
        log_error "Failed to download ${filename}"
        rm -f "$tmp_path"
    fi
done

log_info "Update complete. ${success_count}/${#DATABASES[@]} databases updated."

# Set proper permissions
chmod 644 "${GEOIP_DIR}"/*.mmdb 2>/dev/null || true

# Print file info
log_info "Database files:"
ls -la "${GEOIP_DIR}"/*.mmdb 2>/dev/null || log_warn "No database files found"

exit 0
