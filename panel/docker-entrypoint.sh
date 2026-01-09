#!/bin/sh
# Runtime configuration injection for NetWatcher Panel
# This script runs at container startup and injects environment variables
# into the built static files.

CONFIG_FILE="/app/dist/index.html"

# Create the config injection script if CONTROLLER_ENDPOINT is set
if [ -n "$CONTROLLER_ENDPOINT" ]; then
    echo "Injecting CONTROLLER_ENDPOINT: $CONTROLLER_ENDPOINT"
    
    # Create a config script to inject into index.html
    CONFIG_SCRIPT="<script>window.CONTROLLER_ENDPOINT=\"$CONTROLLER_ENDPOINT\";</script>"
    
    # Inject the config script right after the opening <head> tag
    # Using sed to insert the script
    sed -i "s|<head>|<head>$CONFIG_SCRIPT|" "$CONFIG_FILE"
fi

# Start the server
exec serve -s /app/dist -l 3000
