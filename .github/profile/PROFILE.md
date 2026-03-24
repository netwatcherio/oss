<div align="center">

# 🌐 NetWatcher

**Simple network monitoring, reimagined.**

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![Vue.js](https://img.shields.io/badge/Vue.js-4FC08D?logo=vue.js&logoColor=white)](https://vuejs.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-316192?logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![ClickHouse](https://img.shields.io/badge/ClickHouse-FFCC01?logo=clickhouse&logoColor=black)](https://clickhouse.com)

</div>

---

## 🚀 What is NetWatcher?

NetWatcher is an **open-source distributed network monitoring platform** built for developers, sysadmins, and MSPs who need powerful, real-time visibility into their network infrastructure.

Deploy lightweight agents across your infrastructure and get instant insights through beautiful dashboards — all self-hosted and under your control.

---

## 📦 Our Repositories

| Repository | Description |
|------------|-------------|
| [**oss**](https://github.com/netwatcherio/oss) | 🏠 **Full Platform** — Controller (Go backend), Panel (Vue 3 frontend), and Agent in one monorepo. Start here for self-hosting! |
| [**agent**](https://github.com/netwatcherio/agent) | 🤖 **Standalone Agent** — Lightweight Go daemon for running network probes. Auto-updates from GitHub releases. |

---

## ⚡ Features

- **MTR Checks** — Multi-hop traceroute with per-hop latency and packet loss analysis
- **Ping Monitoring** — ICMP latency monitoring with detailed RTT statistics
- **Traffic Simulation** — Synthetic traffic generation between agents using rPerf
- **System Metrics** — CPU, memory, disk, and OS information collection
- **Network Info** — Public IP, gateway, ISP, and interface data
- **Real-time Dashboards** — Live WebSocket updates with interactive visualizations
- **Role-Based Access** — Owner, Admin, User, and Viewer permission levels
- **Auto-Updates** — Agents automatically update from GitHub releases

---

## 🛠️ Tech Stack

| Component | Technology |
|-----------|------------|
| **Backend** | Go (Iris framework), REST API, WebSocket |
| **Frontend** | Vue 3, Vite, TailwindCSS |
| **Databases** | PostgreSQL (metadata), ClickHouse (time-series) |
| **Deployment** | Docker, Docker Compose, Caddy |

---

## 🏁 Quick Start

```bash
# Clone the OSS repository
git clone https://github.com/netwatcherio/oss.git
cd oss

# Start with Docker Compose
docker-compose up -d
```

📖 See our [documentation](https://github.com/netwatcherio/oss/tree/master/docs) for detailed setup guides.

---

## 🤝 Get Involved

We welcome contributions! Whether it's bug fixes, new features, or documentation improvements:

1. Check out our [open issues](https://github.com/netwatcherio/oss/issues)
2. Fork the repo and create a feature branch
3. Submit a pull request

---

## 📬 Contact

- 📧 **Email**: [contact@netwatcher.io](mailto:contact@netwatcher.io)
- 🐛 **Issues**: [GitHub Issues](https://github.com/netwatcherio/oss/issues)

---

<div align="center">

**Built with ❤️ by the NetWatcher Team**

*Shaun Agostinho & Contributors*

</div>
