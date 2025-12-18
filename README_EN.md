# Web Monitor

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-CC%20BY--NC%204.0-lightgrey.svg" alt="License">
  <img src="https://img.shields.io/badge/Platform-Linux-FCC624?logo=linux&logoColor=black" alt="Platform">
  <img src="https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white" alt="Docker">
</p>

<p align="center">
  <strong>🚀 High-Performance Real-Time System Monitoring Dashboard</strong>
</p>

<p align="center">
  A lightweight system monitoring tool built with Go, supporting both Docker and bare-metal deployments.<br/>
  Real-time updates via WebSocket for CPU, Memory, GPU, Network, Docker, Systemd, and more.
</p>

<p align="center">
  English | <a href="./README.md">简体中文</a>
</p>

---

## ✨ Features

### 📊 Real-Time Monitoring
- **CPU**: Usage, per-core load, frequency, temperature history
- **Memory**: Physical memory, Swap, cache, buffer analysis
- **Disk**: Partition info, usage, I/O stats, inode status
- **GPU**: NVIDIA GPU support (via nvml) - VRAM, temp, power, processes
- **Network**: Interface traffic, connection states, listening ports, socket stats
- **Processes**: Top processes by CPU/memory, I/O statistics

### 🔧 System Management
- **Docker**: Start/stop/restart/remove containers, image management
- **Systemd Services**: List, start/stop/restart/enable/disable services
- **Cron Jobs**: Create, list, delete cron jobs, view logs
- **Process Management**: Kill processes (admin only)

### 🔐 Security Features
- **JWT Authentication**: Secure token-based authentication
- **Role-Based Access**: Admin and regular user separation
- **Rate Limiting**: Brute-force protection for login
- **Security Headers**: CSP, X-Frame-Options, HSTS, etc.
- **Token Revocation**: Logout invalidates tokens

### 🌐 Modern Frontend
- **Real-Time Updates**: WebSocket bidirectional communication
- **PWA Support**: Installable as desktop/mobile app
- **Responsive Design**: Works on all screen sizes
- **Dark Theme**: Easy on the eyes
- **Chart Visualization**: Real-time charts powered by Chart.js

### ⚡ High-Performance Design
- **Parallel Collection**: 11 collectors running concurrently
- **Smart Caching**: TTL-based cache to reduce system load
- **Dynamic Intervals**: Auto-adjusts based on client demand
- **Graceful Shutdown**: Signal handling and smooth exit

---

## 🚀 Quick Start

### Docker Deployment (Recommended)

We provide a `Makefile` to simplify common operations.

```bash
# Clone the repository
git clone https://github.com/AnalyseDeCircuit/web-monitor.git
cd web-monitor

# Start service (Full Mode)
make up

# Start minimal mode (Core metrics only)
make up-minimal

# View logs
make logs
```

Access `http://localhost:38080` with default credentials:
- Username: `admin`
- Password: `admin123`

> ⚠️ **Change the default password immediately after first login!**

### Basic Configuration (.env)

The `.env` file in the root directory controls core security and network settings. Please check it before deployment:

```dotenv
# REQUIRED! Set a long, random string for JWT signing
JWT_SECRET=change-me-to-a-long-random-string

# If accessing via a domain or reverse proxy, set allowed WebSocket origins
# Comma-separated
WS_ALLOWED_ORIGINS=https://your-domain.com

# Service Port (Internal Docker port, change host port in docker-compose.yml)
PORT=38080
```

### Modular Configuration

You can control enabled modules via environment variables. Modify `docker-compose.yml` or specify them at startup:

| Variable | Default | Description |
| :--- | :--- | :--- |
| `ENABLE_DOCKER` | `true` | Enable Docker management |
| `ENABLE_GPU` | `true` | Enable NVIDIA GPU monitoring |
| `ENABLE_SSH` | `true` | Enable SSH session monitoring |
| `ENABLE_CRON` | `true` | Enable Cron job management |
| `ENABLE_SYSTEMD` | `true` | Enable Systemd service management |
| `ENABLE_SENSORS` | `true` | Enable hardware sensors (Temp/Fan) |
| `ENABLE_POWER` | `true` | Enable power management (Battery/Profile) |

**Example: Disable Docker and GPU only**
```bash
ENABLE_DOCKER=false ENABLE_GPU=false make up
```

### Common Commands

| Command | Description |
| :--- | :--- |
| `make up` | Start all services (Background) |
| `make up-minimal` | Start minimal mode (Core metrics only) |
| `make up-server` | Start server mode (No GPU/Power) |
| `make up-no-docker` | Start without Docker management |
| `make down` | Stop and remove containers |
| `make restart` | Restart services |
| `make logs` | View real-time logs |
| `make rebuild` | Rebuild images and restart |

### Bare-Metal Deployment

```bash
# Build
go build -mod=vendor -o server ./cmd/server

# Set environment variables
export PORT=8000
export DATA_DIR=/var/lib/web-monitor

# Run
./server
```

---

## 📁 Project Structure

```
web-monitor/
├── cmd/
│   ├── server/          # Main entry point
│   └── dockerproxy/     # Docker socket proxy
├── api/handlers/        # HTTP routes and handlers
├── internal/
│   ├── auth/            # Authentication & authorization
│   ├── cache/           # Metrics cache
│   ├── collectors/      # Data collectors (11 total)
│   ├── config/          # Configuration management
│   ├── cron/            # Cron job management
│   ├── docker/          # Docker API client
│   ├── middleware/      # HTTP middleware
│   ├── monitoring/      # Monitoring service & alerts
│   ├── systemd/         # Systemd service management
│   └── websocket/       # WebSocket hub
├── pkg/types/           # Shared type definitions
├── static/              # Frontend static assets
├── templates/           # HTML templates
└── vendor/              # Dependencies (offline build)
```

---

## ⚙️ Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8000` | HTTP server port |
| `DATA_DIR` | `/data` | Data storage directory |
| `JWT_SECRET` | Random | JWT signing key |
| `WS_ALLOWED_ORIGINS` | `*` | WebSocket allowed origins |
| `HOST_FS` | `/hostfs` | Host filesystem mount point |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker API endpoint |

### Container Mode vs Bare-Metal Mode

**Container Mode** (auto-detected when `HOST_FS` is set):
- Access host system via `/hostfs` mount
- Requires specific Linux capabilities

**Bare-Metal Mode** (`HOST_FS` is empty):
- Direct access to local `/proc`, `/sys`, etc.
- No additional permission configuration needed

### Docker Compose Reference

```yaml
services:
  web-monitor-go:
    image: web-monitor-go:latest
    cap_add:
      - SYS_PTRACE        # Read process info
      - DAC_READ_SEARCH   # Read log files
      - SYS_CHROOT        # Cron management
    network_mode: host
    pid: host
    volumes:
      - /:/hostfs:ro
      - /run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro
```

---

## 📡 API Overview

### Authentication
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/login` | POST | User login |
| `/api/logout` | POST | User logout |
| `/api/password` | POST | Change password |

### Monitoring Data
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/ws/stats` | WebSocket | Real-time monitoring stream |
| `/api/system/info` | GET | System info snapshot |
| `/api/info` | GET | Static system info |

### Management (Authenticated)
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/docker/containers` | GET | List Docker containers |
| `/api/docker/action` | POST | Container operations (admin) |
| `/api/systemd/services` | GET | List Systemd services |
| `/api/systemd/action` | POST | Service operations (admin) |
| `/api/cron/jobs` | GET | List cron jobs |
| `/api/users` | GET/POST | User management (admin) |

For detailed API documentation, see [API_DOCUMENTATION.md](./API_DOCUMENTATION.md).

---

## 🛡️ Security Recommendations

1. **Change Default Password**: Immediately change the admin password after first login
2. **Set JWT_SECRET**: Always set a strong random key in production
3. **Restrict Network Access**: Use a reverse proxy (Nginx) with HTTPS
4. **Docker Socket Proxy**: Use `docker-socket-proxy` to limit Docker API exposure
5. **Keep Updated**: Follow project updates for security patches

---

## 🔌 GPU Support

### NVIDIA GPU

Automatically detected and collected via nvml:
- GPU utilization
- VRAM usage
- Temperature/Power
- GPU processes

Enable NVIDIA Container Toolkit in Docker:

```yaml
environment:
  - NVIDIA_VISIBLE_DEVICES=all
  - NVIDIA_DRIVER_CAPABILITIES=all
```

---

## 📊 Architecture

For detailed architecture diagrams, see [ARCHITECTURE.md](./ARCHITECTURE.md).

```
┌─────────────┐     ┌──────────────────────────────────────┐
│   Browser   │────▶│            Go Server                 │
│  (WebSocket)│◀────│  ┌─────────┐  ┌──────────────────┐  │
└─────────────┘     │  │ Router  │──│ WebSocket Hub    │  │
                    │  └────┬────┘  └────────┬─────────┘  │
                    │       │                │            │
                    │  ┌────▼────┐  ┌────────▼─────────┐  │
                    │  │ Cache   │◀─│ Stats Aggregator │  │
                    │  └─────────┘  └────────┬─────────┘  │
                    │                        │            │
                    │         ┌──────────────┼──────────┐ │
                    │         ▼              ▼          ▼ │
                    │  ┌──────────┐ ┌──────────┐ ┌──────┐ │
                    │  │Collectors│ │  Docker  │ │Systemd││
                    │  │ (x11)    │ │  Client  │ │ D-Bus ││
                    │  └──────────┘ └──────────┘ └──────┘ │
                    └──────────────────────────────────────┘
```

---

## 🤝 Contributing

Contributions are welcome! Feel free to submit Issues and Pull Requests.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## 📄 License

This project is licensed under [CC BY-NC 4.0](./LICENSE) (Attribution-NonCommercial).

---

## 🙏 Acknowledgments

- [gopsutil](https://github.com/shirou/gopsutil) - Cross-platform system info
- [go-nvml](https://github.com/NVIDIA/go-nvml) - NVIDIA GPU monitoring
- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket implementation
- [Chart.js](https://www.chartjs.org/) - Frontend charting library
