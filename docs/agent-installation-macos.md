# NetWatcher Agent macOS Installation Guide

A lightweight network monitoring agent that reports metrics to the NetWatcher platform.

## Requirements

- **macOS**: macOS 10.15 (Catalina) or later
- **Architecture**: Intel (amd64) or Apple Silicon (arm64)
- **Permissions**: Root privileges (required for ICMP and raw sockets)
- **Controller**: Running instance of [NetWatcher Controller](https://github.com/α2io/oss)

## Installation

### Quick Start (User-Level Service)

User-level installation runs the agent in your user session. It starts automatically when you log in and stops when you log out.

```bash
curl -fsSL https://raw.githubusercontent.com/α2io/agent/master/install-macos.sh | bash -s -- \
  --workspace YOUR_WORKSPACE_ID \
  --id YOUR_AGENT_ID \
  --pin YOUR_AGENT_PIN
```

### System-Level Installation (Requires sudo)

System-level installation runs the agent as a background service that starts at boot and runs regardless of user login.

```bash
curl -fsSL https://raw.githubusercontent.com/α2io/agent/master/install-macos.sh | sudo bash -s -- \
  --workspace YOUR_WORKSPACE_ID \
  --id YOUR_AGENT_ID \
  --pin YOUR_AGENT_PIN \
  --system
```

### Self-Hosted Controller

For self-hosted NetWatcher instances:

```bash
# User-level
curl -fsSL https://raw.githubusercontent.com/α2io/agent/master/install-macos.sh | bash -s -- \
  --host your-controller.example.com \
  --ssl true \
  --workspace 1 \
  --id 42 \
  --pin 123456789

# System-level
curl -fsSL https://raw.githubusercontent.com/α2io/agent/master/install-macos.sh | sudo bash -s -- \
  --host your-controller.example.com \
  --ssl true \
  --workspace 1 \
  --id 42 \
  --pin 123456789 \
  --system
```

## Service Management

### User-Level Service

| Action | Command |
|--------|---------|
| Check status | `launchctl list \| grep com.netwatcher.agent` |
| View logs | `tail -f ~/netwatcher-agent/agent.log` |
| Restart | `launchctl stop com.netwatcher.agent && launchctl start com.netwatcher.agent` |
| Stop | `launchctl stop com.netwatcher.agent` |
| Unload service | `launchctl unload ~/Library/LaunchAgents/com.netwatcher.agent.plist` |

### System-Level Service

| Action | Command |
|--------|---------|
| Check status | `sudo launchctl list \| grep com.netwatcher.agent` |
| View logs | `tail -f /var/root/netwatcher-agent/agent.log` |
| Restart | `sudo launchctl stop com.netwatcher.agent && sudo launchctl start com.netwatcher.agent` |
| Stop | `sudo launchctl stop com.netwatcher.agent` |
| Unload service | `sudo launchctl unload /Library/LaunchDaemons/com.netwatcher.agent.plist` |

## Configuration

Configuration is stored in `config.conf` within the installation directory:

| Path | Description |
|------|-------------|
| `~/netwatcher-agent/config.conf` | User-level installation |
| `/var/root/netwatcher-agent/config.conf` | System-level installation (root user) |

| Parameter | Description |
|-----------|-------------|
| `CONTROLLER_HOST` | Controller hostname (e.g., `api.α2.io`) |
| `CONTROLLER_SSL` | Use HTTPS/WSS (`true` or `false`) |
| `WORKSPACE_ID` | Workspace ID |
| `AGENT_ID` | Agent ID |
| `AGENT_PIN` | Initial authentication PIN |

## Installer Options

| Flag | Description |
|------|-------------|
| `--workspace`, `-w` | Workspace ID (required for install) |
| `--id`, `-i` | Agent ID (required for install) |
| `--pin`, `-p` | Agent PIN (required for install) |
| `--host` | Controller host (default: `api.α2.io`) |
| `--ssl` | Use SSL/HTTPS — `true` or `false` (default: `true`) |
| `--install-dir` | Installation directory (default: `~/netwatcher-agent`) |
| `--system` | Install as system-level service (requires sudo) |
| `--user` | Install as user-level service (default, no sudo) |
| `--force` | Force reinstallation or skip uninstall confirmation |
| `--no-service` | Skip launchd service creation |
| `--no-start` | Don't start the service after installation |
| `--version` | Install a specific version tag |
| `--update` | Update only the binary (preserves config and service) |
| `--uninstall` | Uninstall the agent |
| `--debug` | Enable debug output |

## Updating

### Update to Latest Version

```bash
# User-level
~/netwatcher-agent/install-macos.sh --update

# System-level
sudo /var/root/netwatcher-agent/install-macos.sh --update
```

### Update to Specific Version

```bash
# User-level
~/netwatcher-agent/install-macos.sh --update --version v20260219-5c692b8

# System-level
sudo /var/root/netwatcher-agent/install-macos.sh --update --version v20260219-5c692b8
```

## Uninstallation

### Standard Uninstall

```bash
# User-level
~/netwatcher-agent/install-macos.sh --uninstall

# System-level
sudo /var/root/netwatcher-agent/install-macos.sh --uninstall
```

### Force Uninstall (Skip Confirmation)

```bash
# User-level
~/netwatcher-agent/install-macos.sh --uninstall --force

# System-level
sudo /var/root/netwatcher-agent/install-macos.sh --uninstall --force
```

## Troubleshooting

### Viewing Logs

```bash
# User-level
tail -f ~/netwatcher-agent/agent.log

# System-level
tail -f /var/root/netwatcher-agent/agent.log
```

### Failed Auto-Updates

If the agent's auto-update fails, use the install script to manually update:

```bash
# User-level
~/netwatcher-agent/install-macos.sh --update

# System-level
sudo /var/root/netwatcher-agent/install-macos.sh --update
```

### Manual Binary Replacement

If the install script isn't available, manually replace the binary:

```bash
# 1. Stop the service
launchctl stop com.netwatcher.agent

# 2. Download the latest release
# Visit: https://github.com/α2io/agent/releases/latest
# Download the darwin-amd64 or darwin-arm64 zip for your Mac

# 3. Extract and replace
cd ~/netwatcher-agent
unzip ~/Downloads/netwatcher-*-darwin-*.zip
cp netwatcher-darwin-*/* ./α2-agent
chmod +x ./α2-agent

# 4. Verify and restart
./α2-agent --version
launchctl start com.netwatcher.agent
```

### Service Won't Start

1. Check if the plist is loaded:
   ```bash
   launchctl list | grep com.netwatcher.agent
   ```

2. Verify the configuration file exists and has correct permissions:
   ```bash
   cat ~/netwatcher-agent/config.conf
   chmod 600 ~/netwatcher-agent/config.conf
   ```

3. Check the log file for errors:
   ```bash
   tail -50 ~//netwatcher-agent/agent.log
   ```

4. Try running the agent manually to see error output:
   ```bash
   ~/netwatcher-agent/α2-agent --config ~/netwatcher-agent/config.conf
   ```

### Common Issues

| Issue | Solution |
|-------|----------|
| "Cannot open raw socket" | Run with sudo: `sudo launchctl start com.netwatcher.agent` |
| Service not starting on login | Verify plist is in `~/Library/LaunchAgents/` and `RunAtLoad` is `true` |
| Agent not connecting | Check `config.conf` settings and firewall rules |
| "Unauthorized" errors | Re-bootstrap with correct PIN or generate new agent credentials |
| Binary blocked by Gatekeeper | The agent requires root privileges; Gatekeeper should prompt on first run |

## File Locations

| Component | User-Level Path | System-Level Path |
|-----------|----------------|-------------------|
| Binary | `~/netwatcher-agent/α2-agent` | `/var/root/netwatcher-agent/α2-agent` |
| Config | `~/netwatcher-agent/config.conf` | `/var/root/netwatcher-agent/config.conf` |
| Logs | `~/netwatcher-agent/agent.log` | `/var/root/netwatcher-agent/agent.log` |
| Plist | `~/Library/LaunchAgents/com.netwatcher.agent.plist` | `/Library/LaunchDaemons/com.netwatcher.agent.plist` |

## Building from Source

```bash
git clone https://github.com/α2io/agent
cd agent

# Build for darwin
./build.sh

# The binary will be in bin/netwatcher-darwin-*
```

## License

[GNU Affero General Public License v3.0](LICENSE.md)
