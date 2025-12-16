# Web Monitor (Go Version)

A lightweight, high-performance Linux server monitoring and management dashboard. Built with a Go backend and pure HTML/JS frontend, it has a minimal footprint and is easy to deploy.

## ✨ Features

*   **Real-time Monitoring**: CPU, Memory, Disk I/O, Network Traffic, GPU (NVIDIA/AMD/Intel), Temperature Sensors.
*   **Process Management**: View top system processes, sortable by CPU, memory, IO usage.
    *   **Lazy Loading**: Process I/O details are loaded on-demand via REST API, significantly reducing overhead during regular collection.
*   **Docker Management**: List containers/images, start, stop, restart, remove containers, view container logs and statistics.
*   **System Management**:
    *   **Systemd Services**: View service status, start, stop, restart, enable, and disable services.
    *   **Cron Jobs**: View and edit scheduled tasks.
*   **SSH Monitoring**: Monitor SSH connections, active sessions, login history, and failed attempts.
    *   **Multi-level Caching**: Implements TTL caching (Connections 60s / Logs 5m / HostKey 1h) with manual refresh support.
    *   **Memory Optimization**: Uses VmRSS for accurate sshd memory usage statistics.
*   **Security Auditing**: Built-in Role-Based Access Control (Admin/User), logging critical operations.
*   **Prometheus Integration**: Exposes `/metrics` endpoint for Prometheus/Grafana integration.
*   **Alert Configuration**: Supports CPU, memory, disk usage threshold alerts with configurable webhooks.
*   **Power Management**: View and adjust system power performance modes (requires hardware support).
*   **GPU Monitoring**: Supports NVIDIA, AMD, Intel GPU temperature, usage, and memory monitoring.

## ⚡ Performance & Optimization

This project has undergone deep performance tuning to ensure minimal resource usage while providing rich features:

*   **Ultra-low Resource Usage**: Deeply optimized via pprof profiling to significantly reduce CPU and memory footprint.
*   **Zero External Dependencies**: All static assets (Font Awesome, Chart.js, JetBrains Mono) are **fully localized**. Works perfectly in intranet/offline environments without CDN issues.
*   **Efficient Collection**:
    *   **Native Linux Parsing**: Network details are parsed directly from `/proc/net/{tcp,udp}`, replacing generic library calls for better performance.
    *   **Smart Caching**: Implemented caching for static process info (cmdline, start time) to avoid repetitive `/proc` filesystem reads.
    *   **Object Pooling**: Optimized network and process collection logic to reuse objects, reducing GC pressure and system calls.
*   **On-Demand Loading**:
    *   **Dynamic Subscription**: WebSocket supports page-based dynamic subscription (e.g., only Top 10 processes on Dashboard, full list on Processes page), reducing data transfer.
    *   **Lazy I/O Stats**: Process I/O statistics are fetched via REST API only when viewing details, avoiding I/O overhead in every collection cycle.
*   **High-Performance Serialization**: Manually implemented `MarshalJSON` for hot paths (e.g., process lists) to bypass reflection overhead.
*   **Static Asset Optimization**: Implemented file fingerprinting and aggressive caching strategies (`Cache-Control: immutable`) for faster frontend loading.

## 🚀 Quick Start (Docker Compose)

This is the recommended deployment method, pre-configured for full functionality.

1.  Ensure Docker and Docker Compose are installed.
2.  Run the following command in the project root:

```bash
docker compose up -d
```

3.  Open your browser: `http://<Server-IP>:38080`
4.  **Default User**: `admin`
5.  **Default Password**: `admin123` **(Change immediately after login)**

### ⚠️ Critical Configuration

To enable full monitoring and management capabilities, the container requires elevated privileges and specific mounts:

*   `cap_add`: Uses a minimal capability set (instead of `privileged: true`) for reading host process/log data and running required system operations (see `docker-compose.yml`).
    *   `SYS_PTRACE`: Read process info from `/proc`.
    *   `DAC_READ_SEARCH`: Read some restricted files (e.g., auth/audit logs).
    *   `SYS_CHROOT`: Execute `chroot` (used for Cron management, etc.).
*   `security_opt: apparmor=unconfined`: Enabled by default in the current Compose (mainly to keep systemd D-Bus control working on some distros/policies).
*   `network_mode: host`: Recommended for accurate host network monitoring.
*   `pid: host`: Required to view host processes.
*   `volumes`:
    *   `/:/hostfs`: **Core Configuration**. Used to access the host filesystem (process/log/hardware info, Cron management, etc.).
    *   `/run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro`: Required for Systemd management (via D-Bus).
    *   `/proc`, `/sys`: For hardware statistics collection and GPU monitoring.
    *   GPU devices (e.g., `/dev/nvidia*`): If GPU monitoring is needed, mount the corresponding devices.

#### Docker Management (via Local Proxy by Default)

To reduce risk, this repo’s default setup **does not mount** `docker.sock` into the `web-monitor-go` container. Instead, it talks to Docker through `docker-socket-proxy` (listening on `127.0.0.1:2375`) which forwards a limited allowlist of Docker Engine API endpoints:

*   `web-monitor-go` uses `DOCKER_HOST=tcp://127.0.0.1:2375` (proxy only).
*   Only `docker-socket-proxy` mounts the host `${DOCKER_SOCK:-/var/run/docker.sock}`.
    *   Rootless Docker: set `DOCKER_SOCK` to your actual socket path (e.g. `$XDG_RUNTIME_DIR/docker.sock`).

## 🛠️ Manual Build & Run

If you prefer not to use Docker, you can compile and run the binary directly.

### Build

```bash
# Enable static compilation for compatibility
export CGO_ENABLED=0
go build -ldflags="-s -w" -trimpath -o web-monitor-go ./cmd/server/main.go

# Optional: Compress binary with upx
upx --lzma --best web-monitor-go
```

### Run

```bash
# Defaults to port 8000
./web-monitor-go

# Specify a custom port
PORT=8080 ./web-monitor-go
```

Note: When running directly on the host, the app executes commands directly and does not require the `/hostfs` mechanism.

## 🔒 Security Features

*   **HttpOnly Cookie Auth**: Removed frontend localStorage token storage. Uses HttpOnly Cookies for authentication to prevent token theft via XSS.
*   **Cloudflare/Proxy Support**: Supports `CF-Connecting-IP` and other proxy headers (requires firewall configuration to restrict source IP).
*   **Docker Socket Isolation**: Accesses Docker API via a read-only/allowlist proxy to prevent container escape risks.
*   **Least Privilege**: Docker container uses fine-grained Linux Capabilities instead of `privileged` mode.
*   **Security Headers**: Built-in CSP (Content Security Policy), HSTS, and other security headers.
*   **CSRF Protection**: Cookie-based SameSite policy and Origin validation.

## 📝 License


CC BY-NC 4.0
