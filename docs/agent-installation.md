# NetWatcher Agent Installation Guide

This guide covers installing, configuring, and managing the NetWatcher Agent on Windows, Linux, and macOS.

## Prerequisites

| Platform | Requirements |
|----------|-------------|
| **Windows** | Windows 10+ or Windows Server 2016+, PowerShell 5.1+, Administrator access |
| **Linux** | systemd-based distribution (Ubuntu 18.04+, Debian 10+, RHEL 8+, etc.), root access |
| **macOS** | macOS 10.15+, root access |

## Quick Start

### Linux / macOS

```bash
# Download and run the installer
curl -fsSL https://raw.githubusercontent.com/netwatcherio/agent/main/install.sh | sudo bash -s -- \
  --id YOUR_AGENT_ID \
  --pin YOUR_AGENT_PIN
```

### Windows (PowerShell as Administrator)

```powershell
# Download the installer
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/netwatcherio/agent/main/install.ps1" -OutFile "install.ps1"

# Run the installer
.\install.ps1 -Id "YOUR_AGENT_ID" -Pin "YOUR_AGENT_PIN"
```

---

## Obtaining Agent Credentials

Before installation, you need to create an agent in the NetWatcher panel:

1. Log into your NetWatcher panel
2. Navigate to **Agents** → **Create Agent**
3. Give your agent a name and select the target workspace
4. Copy the **Agent ID** and **PIN** displayed after creation

> [!IMPORTANT]
> The PIN is only shown once during agent creation. Store it securely.

---

## Windows Installation

### Using PowerShell Script

The PowerShell script (`install.ps1`) handles the complete installation process.

#### Basic Installation

```powershell
# Run as Administrator
.\install.ps1 -Id "686c6d4298d36e8a13fb7ee6" -Pin "036977322"
```

#### All Installation Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `-Id` | Agent ID (required) | - |
| `-Pin` | Agent PIN (required) | - |
| `-Host` | API host URL | `https://api.netwatcher.io` |
| `-HostWs` | WebSocket host URL | `wss://api.netwatcher.io/agent_ws` |
| `-InstallDir` | Installation directory | `C:\Program Files\NetWatcher-Agent` |
| `-Version` | Specific version to install | latest |
| `-Force` | Force reinstallation | - |
| `-NoStart` | Don't start service after install | - |
| `-Uninstall` | Uninstall instead of install | - |

#### Custom Host Configuration

```powershell
.\install.ps1 -Id "YOUR_ID" -Pin "YOUR_PIN" `
  -Host "https://your-server.com" `
  -HostWs "wss://your-server.com/agent_ws"
```

### Service Management

```powershell
# Check service status
Get-Service -Name NetWatcherAgent

# Start/Stop/Restart
Start-Service -Name NetWatcherAgent
Stop-Service -Name NetWatcherAgent
Restart-Service -Name NetWatcherAgent

# View logs
Get-EventLog -LogName Application -Source NetWatcherAgent -Newest 50
```

---

## Linux Installation

### Using Bash Script

The bash script (`install.sh`) handles installation on Linux and macOS with systemd.

#### Basic Installation

```bash
sudo ./install.sh --id "686c6d4298d36e8a13fb7ee6" --pin "036977322"
```

#### All Installation Options

| Option | Description | Default |
|--------|-------------|---------|
| `--id`, `-i` | Agent ID (required) | - |
| `--pin`, `-p` | Agent PIN (required) | - |
| `--host` | API host URL | `https://api.netwatcher.io` |
| `--host-ws` | WebSocket host URL | `wss://api.netwatcher.io/agent_ws` |
| `--install-dir` | Installation directory | `/opt/netwatcher-agent` |
| `--version` | Specific version to install | latest |
| `--force` | Force reinstallation | - |
| `--no-service` | Skip systemd service creation | - |
| `--no-start` | Don't start service after install | - |
| `--uninstall` | Uninstall the agent | - |
| `--debug` | Enable debug output | - |

#### Custom Host Configuration

```bash
sudo ./install.sh --id "YOUR_ID" --pin "YOUR_PIN" \
  --host https://your-server.com \
  --host-ws wss://your-server.com/agent_ws
```

### Service Management

```bash
# Check service status
sudo systemctl status netwatcher-agent

# Start/Stop/Restart
sudo systemctl start netwatcher-agent
sudo systemctl stop netwatcher-agent
sudo systemctl restart netwatcher-agent

# View logs
sudo journalctl -u netwatcher-agent -f

# Enable/Disable auto-start
sudo systemctl enable netwatcher-agent
sudo systemctl disable netwatcher-agent
```

---

## Uninstallation

### Windows

```powershell
# Interactive uninstall (with confirmation)
.\install.ps1 -Uninstall

# Force uninstall (no confirmation)
.\install.ps1 -Uninstall -Force
```

### Linux / macOS

```bash
# Interactive uninstall (with confirmation)
sudo ./install.sh --uninstall

# Force uninstall (no confirmation)
sudo ./install.sh --uninstall --force
```

---

## Configuration Reference

The agent uses a configuration file (`config.conf`) with the following options:

```ini
# NetWatcher Agent Configuration

# Controller host (HTTP/HTTPS URL)
HOST=https://api.netwatcher.io

# WebSocket host (WS/WSS URL)
HOST_WS=wss://api.netwatcher.io/agent_ws

# Agent credentials
ID=YOUR_AGENT_ID
PIN=YOUR_AGENT_PIN

# PSK is saved here after initial authentication (auto-populated)
# AGENT_PSK=
```

### Configuration Locations

| Platform | Path |
|----------|------|
| Windows | `C:\Program Files\NetWatcher-Agent\config.conf` |
| Linux | `/opt/netwatcher-agent/config.conf` |
| macOS | `/opt/netwatcher-agent/config.conf` |

---

## Updating the Agent

The agent includes an auto-updater that checks for new versions every 6 hours. To manually update:

### Force Update

1. Stop the service
2. Run the installer with `--force` (or `-Force` on Windows)
3. The service will restart automatically

```bash
# Linux
sudo ./install.sh --id "YOUR_ID" --pin "YOUR_PIN" --force

# Windows
.\install.ps1 -Id "YOUR_ID" -Pin "YOUR_PIN" -Force
```

### Disable Auto-Updater

To run the agent without the auto-updater:

```bash
# Linux: Edit the service file
sudo systemctl edit netwatcher-agent
# Add: ExecStart=/opt/netwatcher-agent/netwatcher-agent --config /opt/netwatcher-agent/config.conf --no-update
```

---

## Troubleshooting

### Agent Won't Start

1. **Check logs** for error messages:
   ```bash
   # Linux
   sudo journalctl -u netwatcher-agent -n 100 --no-pager
   
   # Windows
   Get-EventLog -LogName Application -Source NetWatcherAgent -Newest 50
   ```

2. **Verify configuration** file has correct credentials
3. **Check network connectivity** to the controller host
4. **Ensure firewall** allows outbound HTTPS and WSS connections

### Authentication Failures

1. Delete the auth file to force re-authentication:
   ```bash
   # Linux
   sudo rm /opt/netwatcher-agent/agent_auth.json
   sudo systemctl restart netwatcher-agent
   
   # Windows (PowerShell as Admin)
   Remove-Item "C:\Program Files\NetWatcher-Agent\agent_auth.json"
   Restart-Service NetWatcherAgent
   ```

2. Verify your Agent ID and PIN are correct in the config file

### Service Not Found

If the service doesn't exist after installation:

```bash
# Linux: Recreate service
sudo systemctl daemon-reload
sudo systemctl start netwatcher-agent

# Windows: Reinstall with force
.\install.ps1 -Id "YOUR_ID" -Pin "YOUR_PIN" -Force
```

### Permission Issues

The agent requires elevated permissions for certain probes (ICMP, raw sockets):

- **Linux**: The service runs as root by default
- **Windows**: The service runs as LocalSystem

---

## File Locations

### Windows

| File | Path |
|------|------|
| Binary | `C:\Program Files\NetWatcher-Agent\netwatcher-agent.exe` |
| Config | `C:\Program Files\NetWatcher-Agent\config.conf` |
| Auth | `C:\Program Files\NetWatcher-Agent\agent_auth.json` |

### Linux

| File | Path |
|------|------|
| Binary | `/opt/netwatcher-agent/netwatcher-agent` |
| Config | `/opt/netwatcher-agent/config.conf` |
| Auth | `/opt/netwatcher-agent/agent_auth.json` |
| Service | `/etc/systemd/system/netwatcher-agent.service` |
