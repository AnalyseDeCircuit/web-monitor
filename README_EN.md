# Web Monitor (Go Version)

A lightweight, high-performance Linux server monitoring and management dashboard. Built with a Go backend and pure HTML/JS frontend, it has a minimal footprint and is easy to deploy.

## ✨ Features

*   **Real-time Monitoring**: CPU, Memory, Disk I/O, Network Traffic, GPU (NVIDIA/AMD/Intel), Temperature Sensors.
*   **Process Management**: View top system processes, sortable by CPU, memory, IO usage, view process details.
*   **Docker Management**: List containers/images, start, stop, restart, remove containers, view container logs and statistics.
*   **System Management**:
    *   **Systemd Services**: View service status, start, stop, restart, enable, and disable services.
    *   **Cron Jobs**: View and edit scheduled tasks.
*   **SSH Monitoring**: Monitor SSH connections, active sessions, login history, and failed attempts.
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
    *   **Smart Caching**: Implemented caching for static process info (cmdline, start time) to avoid repetitive `/proc` filesystem reads.
    *   **Object Pooling**: Optimized network and process collection logic to reuse objects, reducing GC pressure and system calls.
*   **High-Performance Serialization**: Manually implemented `MarshalJSON` for hot paths (e.g., process lists) to bypass reflection overhead, boosting performance with large datasets.
*   **Static Asset Optimization**: Implemented file fingerprinting and aggressive caching strategies (`Cache-Control: immutable`) for faster frontend loading.

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
    *   `/:/hostfs`: **Core Configuration**. The app uses `chroot /hostfs` to manage the host's Systemd, Cron, and system information.
    *   `/var/run/docker.sock`: For Docker management features.
    *   `/proc`, `/sys`: For hardware statistics collection and GPU monitoring.
    *   GPU devices (e.g., `/dev/nvidia*`): If GPU monitoring is needed, mount the corresponding devices.

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

Web Monitor includes multiple layers of security mechanisms to ensure system safety:

### Authentication & Authorization
*   **JWT Token Authentication**: Uses standard JWT (JSON Web Token) for session management, tokens expire in 24 hours.
*   **Role-Based Access Control**: Two-level permissions: Administrator (admin) and Regular User (user).
*   **Password Policy**: Passwords must be at least 8 characters, containing three of: uppercase letters, lowercase letters, digits, and special characters.
*   **Account Lockout**: Accounts are locked for 15 minutes after 5 consecutive failed login attempts.
*   **Rate Limiting**: Login endpoints are rate-limited to prevent brute-force attacks.

### Network Security
*   **Security HTTP Headers**: Automatically sets CSP, X-Frame-Options, X-XSS-Protection, and other security headers.
*   **CSP Policy**: Strict Content Security Policy to prevent XSS attacks.
*   **Input Validation**: All user inputs are strictly validated to prevent command injection.
*   **HTTPS Ready**: Supports TLS certificate configuration to enable HTTPS.

### Operational Auditing
*   **Complete Operation Logs**: Records all critical operations (login, user management, service operations, etc.).
*   **IP Address Recording**: Logs source IP addresses of operations.
*   **Log Retention**: Retains the last 1000 operation logs.

## 📊 Monitoring Metrics

Web Monitor exposes rich system metrics through Prometheus, including:

*   `system_cpu_usage_percent`: CPU usage percentage
*   `system_memory_usage_percent`: Memory usage percentage
*   `system_memory_total_bytes`: Total memory size
*   `system_memory_used_bytes`: Used memory size
*   `system_disk_usage_percent`: Disk usage percentage (by mount point)
*   `system_network_sent_bytes_total`: Total network bytes sent
*   `system_network_recv_bytes_total`: Total network bytes received
*   `system_temperature_celsius`: Hardware temperature (by sensor)
*   `gpu_usage_percent`: GPU usage percentage (by device)
*   `gpu_memory_used_bytes`: GPU memory usage
*   `gpu_temperature_celsius`: GPU temperature

Data can be collected via the `/metrics` endpoint for integration with Prometheus + Grafana.

## ⚙️ Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8000` | Service listening port |
| `JWT_SECRET` | Auto-generated | JWT signing key, recommended to set in production |
| `WS_ALLOWED_ORIGINS` | Empty | Allowlist for WebSocket `/ws/stats` Origin (comma-separated); useful for Cloudflare / reverse proxy / custom domain |
| `SSL_CERT_FILE` | Empty | TLS certificate file path (for HTTPS) |
| `SSL_KEY_FILE` | Empty | TLS private key file path (for HTTPS) |

#### Recommended in production: local `.env` (not committed)

This repo ignores `.env` via `.gitignore`. Create a local `.env` on your server for secrets and site-specific settings.

- Template: `.env.example`
- Example (replace with your domain):

```bash
JWT_SECRET=change-me
WS_ALLOWED_ORIGINS=https://webmonitor.example.com,webmonitor.example.com
```

Docker Compose will automatically read `.env` in the same directory for variable injection.

### Cloudflare CDN / Reverse Proxy notes (WebSocket)

When accessing through Cloudflare on `https://<domain>/`, browsers send a WebSocket `Origin` header. For safety, the server allows same-origin WebSocket by default.

If your WebSocket connection fails (403 / Origin errors), set `WS_ALLOWED_ORIGINS` in `.env` and ensure your proxy forwards the correct `Host` / `X-Forwarded-Host`. Also ensure Cloudflare WebSockets is enabled.

### Data Persistence

All persistent data (user database, logs, alert configurations) is stored in the `/data` directory. In the Docker deployment, this is mapped to the `web-monitor-data` volume.

## 🐛 Troubleshooting

### Common Issues

1.  **Cannot view Systemd services or Cron jobs**
    *   Check if Docker has mounted the `/:/hostfs` directory.
    *   Ensure the container is running with `privileged: true` permission.

2.  **Docker management page is empty**
    *   Check if `/var/run/docker.sock` is mounted.

3.  **GPU monitoring shows as unavailable**
    *   Ensure the host has GPU hardware and drivers installed.
    *   For GPU monitoring inside container, mount GPU device files (e.g., `/dev/nvidia0`) and corresponding library files.
    *   Check if the container has permission to access GPU devices.

4.  **Temperature sensors show 0**
    *   Ensure the `/sys` directory is mounted and the container has privileged permissions.
    *   Some hardware may require additional kernel modules.

5.  **Forgot administrator password**
    ```bash
    # Enter the container
    docker exec -it web-monitor-go sh
    
    # Delete the user database
    rm /data/users.json
    
    # Restart the container
    docker restart web-monitor-go
    ```
    After restart, the system will recreate the default account (admin/admin123).

### Viewing Logs

```bash
# View container logs
docker logs web-monitor-go

# Follow real-time logs
docker logs -f web-monitor-go
```

## 🤝 Contributing

Issues and Pull Requests are welcome to improve Web Monitor.

### Development Environment Setup

1.  Clone the repository
2.  Install Go 1.21+ and Node.js
3.  Run `go mod download` to download dependencies
4.  Start the development server: `go run ./cmd/server/main.go`

### Code Standards

*   Go code follows standard formatting (use `go fmt`)
*   Frontend code uses ES6+ standards
*   Run tests before submitting

## 📄 License

CC BY-NC 4.0

## 📞 Support

*   [GitHub Issues](https://github.com/AnalyseDeCircuit/web-monitor/issues) - Report issues or request features
*   [User Manual](MANUAL.md) - Detailed feature descriptions and configuration guide
*   [Chinese Documentation](README.md) - 中文文档

---

**Important Notes**:
1.  Web Monitor requires high privileges to access system information. Deploy only in trusted network environments.
2.  In production environments, always change the default password and configure HTTPS.
3.  Regularly back up important data in the `/data` directory.
4.  GPU monitoring requires corresponding hardware and driver support, some features may be limited in container environments.
