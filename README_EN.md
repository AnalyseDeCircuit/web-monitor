# Web Monitor (Go)

A lightweight, high-performance Linux system monitoring service with a modern Web UI and real-time WebSocket streaming. Designed for small hosts, NAS, and container environments.

## Key Features
- **Comprehensive Monitoring**: CPU (freq/load/power), Memory, Disk I/O, Network traffic/connections, Processes.
- **GPU Support**: Monitoring for Intel iGPUs (e.g., Alder Lake-N) including frequency, load, and VRAM.
- **SSH Auditing**: Real-time tracking of SSH connections, active sessions, failed logins, and known hosts.
- **Efficient**: Written in Go, statically compiled, zero external dependencies.
- **Container Ready**: Docker-friendly with configurable ports via environment variables and host network support for full metrics visibility.

## Quick Start (Docker Compose)

Using `host` network mode is recommended for the most accurate network and SSH metrics:

```yaml
services:
  web-monitor-go:
    image: web-monitor-go:offline
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

Access: `http://<IP>:38080`

## Local Run
```bash
# Defaults to port 8000
go run .

# Custom port
PORT=9090 go run .
```

## Documentation
For detailed configuration, feature guides, and security recommendations (e.g., Cloudflare Tunnel), please refer to the [User Manual (MANUAL.md)](MANUAL.md).
