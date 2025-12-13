# Web Monitor (Go Version)

A lightweight, high-performance Linux server monitoring and management dashboard. Built with a Go backend and pure HTML/JS frontend, it has a minimal footprint and is easy to deploy.

## ✨ Features

*   **Real-time Monitoring**: CPU, Memory, Disk I/O, Network Traffic, GPU (NVIDIA/AMD/Intel), Temperature Sensors.
*   **Process Management**: View top system processes, sortable by resource usage.
*   **Docker Management**: List containers/images, start, stop, restart, and remove containers.
*   **System Management**:
    *   **Systemd Services**: View service status, start, stop, restart, enable, and disable services.
    *   **Cron Jobs**: View and edit scheduled tasks.
*   **Network Tools**: Built-in Ping, Traceroute, Dig, Curl diagnostics.
*   **SSH Monitoring**: Monitor SSH connections, active sessions, login history, and failed attempts.
*   **Security Auditing**: Built-in Role-Based Access Control (Admin/User), logging critical operations.
*   **Prometheus Integration**: Exposes `/metrics` endpoint for Prometheus/Grafana integration.

## 🚀 Quick Start (Docker Compose)

This is the recommended deployment method, pre-configured for full functionality.

1.  Ensure Docker and Docker Compose are installed.
2.  Run the following command in the project root:

```bash
docker-compose up -d
```

3.  Open your browser: `http://<Server-IP>:38080`
4.  **Default User**: `admin`
5.  **Default Password**: `admin123` **(Change immediately after login)**

### ⚠️ Critical Configuration

To enable full monitoring and management capabilities, the container requires elevated privileges and specific mounts:

*   `privileged: true`: Required to access hardware sensors and execute privileged commands.
*   `network_mode: host`: Recommended for accurate host network monitoring.
*   `pid: host`: Required to view host processes.
*   `volumes`:
    *   `/:/hostfs`: **Core Configuration**. The app uses `chroot /hostfs` to manage the host's Systemd and Cron.
    *   `/var/run/docker.sock`: For Docker management features.
    *   `/proc`, `/sys`: For hardware statistics collection.

## 🛠️ Manual Build & Run

If you prefer not to use Docker, you can compile and run the binary directly.

### Build

```bash
# Enable static compilation for compatibility
export CGO_ENABLED=0
go build -ldflags="-s -w" -trimpath -o web-monitor-go .

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

## 📝 Data Persistence

All persistent data (user database, logs) is stored in the `/data` directory. In the Docker deployment, this is mapped to the `web-monitor-data` volume.

## 🖥️ Compatibility

*   **OS**: Linux (Ubuntu, Debian, CentOS, Alpine recommended)
*   **Arch**: AMD64, ARM64
*   **Browser**: Chrome, Firefox, Edge, Safari (Modern browsers)

## 📄 License

CC BY-NC 4.0
