# Web Monitor (Go)

A lightweight, high-performance Linux system monitoring service with a modern Web UI and real-time WebSocket streaming. Designed for small hosts, NAS, and container environments.

## Key Features

### Comprehensive System Monitoring
- **CPU Monitoring**: Real-time CPU usage, per-core usage, frequency, temperature history, load averages, context switches and interrupt statistics
- **GPU Support**: Support for Intel iGPUs, NVIDIA GPUs, and AMD GPUs with frequency, load, VRAM, and temperature monitoring, including process-level GPU VRAM usage
- **Memory Monitoring**: Virtual memory and swap space detailed statistics, buffer/cache analysis, memory usage history
- **Disk Monitoring**: Partition usage, disk I/O statistics, inode usage rate
- **Network Monitoring**: Interface status, bytes sent/received, error and drop monitoring, TCP connection state classification, listening ports list, socket statistics
- **Sensor Monitoring**: Hardware temperature sensors, power consumption monitoring
- **SSH Auditing**: Real-time SSH connections, active sessions, failed login tracking, authentication method statistics, and known hosts changes

### Process and Container Monitoring
- Process list (sorted by CPU/memory usage)
- Process tree view, memory/CPU percentage, IO statistics
- Process details (command line, working directory, thread count, uptime)
- OOM risk process warnings

### Web Interface and API
- Real-time responsive dashboard
- WebSocket real-time data streaming (configurable update interval 2-60 seconds)
- RESTful API support
- Modern HTML/CSS/JavaScript frontend

## Technical Features

- **Resource Efficient**: Written in Go, statically compiled, zero external dependencies
- **Container Ready**: Docker-friendly with configurable ports via environment variables and host network support for full metrics visibility
- **Cross-Platform**: Primarily for Linux, supports multiple architectures

## Quick Start (Docker Compose)

Using `host` network mode is recommended for the most accurate network and SSH metrics:

```yaml
version: '3.8'
services:
  web-monitor-go:
    image: web-monitor-go:latest
    container_name: web-monitor-go
    privileged: true
    network_mode: host
    environment:
      - PORT=38080  # Custom listening port
    volumes:
      - /:/hostfs:ro
      - /sys:/sys:ro
      - /proc:/proc
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/run/utmp:/var/run/utmp:ro
      - /etc/passwd:/etc/passwd:ro
      - /etc/group:/etc/group:ro
    restart: unless-stopped
```

Start: `docker-compose up -d`

Access: `http://<IP>:38080`

## Local Compilation and Run

```bash
# Build
go build -o web-monitor main.go

# Defaults to port 8000
./web-monitor

# Custom port
PORT=9090 ./web-monitor
```

## System Requirements

- **Operating System**: Linux (recommended)
- **Go Version**: 1.21+ (if compiling from source)
- **Permissions**: Administrator/root privileges (required for certain features)
- **Dependencies**: gopsutil v3, Gorilla WebSocket

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/` | Web frontend page |
| `WebSocket` | `/ws/stats?interval=2` | Real-time system statistics stream, interval parameter sets 2-60 seconds |
| `GET` | `/api/info` | Get system basic information (JSON format) |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8000` | Web service listening port |
| `SHELL` | `/bin/sh` | System shell type (for display) |
| `LANG` | `C` | System locale |

## Data Format

WebSocket streaming provides complete real-time system information including CPU, memory, GPU, network, SSH, and processes. Refer to the `Response` struct definition in the source code for detailed format specification.

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Cannot access web UI | Check firewall settings and ensure port is open; verify application is running |
| GPU shows Unknown | Device may not be supported or driver incompatible; check `/sys/class/drm/` directory |
| SSH connection stats empty | Requires root privileges to read `/var/log/auth.log`; confirm SSH service is running |
| Some metrics show 0 or Unknown | Certain metrics may unavailable in container environments; mount appropriate system directories |

## Deployment Recommendations

### Secure Deployment (Cloudflare Tunnel)

Since this service contains sensitive system information, **it is strongly recommended not to expose the port directly to the public internet**. It's recommended to use Cloudflare Tunnel with Access for secure access.

See the security deployment section in [User Manual (MANUAL.md)](MANUAL.md) for details.

## License

[Attribution-NonCommercial 4.0 International (CC BY-NC 4.0)](LICENSE)

This project is licensed under Creative Commons Attribution-NonCommercial 4.0 International. You are free to share and adapt this work, but not for commercial purposes, and you must give appropriate credit.

## Documentation

For detailed configuration instructions, feature guides, and deployment recommendations, please refer to [User Manual (MANUAL.md)](MANUAL.md).
