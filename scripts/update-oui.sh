#!/bin/bash
# scripts/update-oui.sh
# Fetches IEEE OUI database for MAC vendor lookup
# Run this script via cron on the host machine to keep database updated
#
# Example cron entry (monthly update, 1st of month at 4 AM):
#   0 4 1 * * /opt/oss/scripts/update-oui.sh >> /var/log/oui-update.log 2>&1
#
# The database file is mounted into the Docker container via volume mount.

# Configuration - defaults to ./oui relative to the oss directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OSS_DIR="$(dirname "$SCRIPT_DIR")"
OUI_DIR="${OUI_DIR:-${OSS_DIR}/oui}"
IEEE_URL="https://standards-oui.ieee.org/oui/oui.txt"

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
mkdir -p "$OUI_DIR"

TARGET_PATH="${OUI_DIR}/oui.txt"
TMP_PATH="${TARGET_PATH}.tmp"

log_info "Starting OUI database update..."
log_info "Target file: $TARGET_PATH"

# Download with retry
max_attempts=3
attempt=1

while [ $attempt -le $max_attempts ]; do
    log_info "Download attempt ${attempt}/${max_attempts}..."
    
    if curl -sL --fail --connect-timeout 30 --max-time 120 -o "$TMP_PATH" "$IEEE_URL"; then
        # Verify file has content (should be ~4MB+)
        file_size=$(stat -f%z "$TMP_PATH" 2>/dev/null || stat -c%s "$TMP_PATH" 2>/dev/null || echo 0)
        
        if [ "$file_size" -gt 1000000 ]; then
            mv "$TMP_PATH" "$TARGET_PATH"
            log_info "Successfully updated OUI database (${file_size} bytes)"
            chmod 644 "$TARGET_PATH"
            
            # Count entries
            entry_count=$(grep -c "(hex)" "$TARGET_PATH" 2>/dev/null || echo "unknown")
            log_info "Database contains ${entry_count} vendor entries"
            
            log_info "Update complete!"
            exit 0
        else
            log_warn "Downloaded file too small (${file_size} bytes), retrying..."
            rm -f "$TMP_PATH"
        fi
    else
        log_warn "Download failed, retrying..."
        rm -f "$TMP_PATH"
    fi
    
    attempt=$((attempt + 1))
    [ $attempt -le $max_attempts ] && sleep 5
done

log_error "Failed to download OUI database after ${max_attempts} attempts"
exit 1
