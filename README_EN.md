# OpsKernel

Single-node Linux monitoring kernel. Provides local system observability and limited management capabilities via HTTP APIs, WebSocket streaming and Docker-based plugins.

[简体中文](README.md) | English

---

## Overview

OpsKernel is designed for **one Linux host at a time**. It collects system metrics, exposes them via WebSocket and APIs, and—when enabled—allows a small set of administrative operations (Docker, systemd, cron, power, processes).

Design goals:

- Focus on **single-node** monitoring and control, not clusters or a centralized control plane
- Core is **shrinkable**: monitoring-only mode is possible by disabling management modules
- Plugins are **isolated containers** (Docker-based HTTP services), not in-process extensions
- High-risk capabilities (kill process, shutdown, Docker/systemd/cron operations, privileged plugins) are admin-only and intended for trusted networks (LAN/VPN)

This project intentionally does **not** provide:

- "Intelligent operations", auto-remediation, or self-healing
- AIOps, machine learning, anomaly detection, or predictive features
- Multi-tenant control plane, agent fleet management, or automatic host discovery

---

## Architecture

High-level components:

- **Collectors**: periodic samplers for CPU, memory, disk, network, GPU, processes, SSH, sensors, power, etc.
- **Streaming Aggregator**: aggregates the latest values from all collectors into a single snapshot structure
- **HTTP API & WebSocket Hub**: exposes REST endpoints and a WebSocket stream for real-time metrics
- **Managers**: optional modules providing operational actions (Docker, systemd, cron, power, process control)
- **Alerts**: rule-based evaluation over metrics and alert history storage
- **Plugins**: external HTTP services run as Docker containers, managed and proxied by OpsKernel

Core can be trimmed down by turning off modules using `ENABLE_*` environment variables. With all management-related flags disabled, OpsKernel behaves as a read-only monitoring kernel.

---

## Functional Modules

### 1. Data Collection (Collectors)

Each collector runs independently and can be toggled via environment flags:

| Module | Env Flag | Default | Description |
|--------|----------|---------|-------------|
| CPU | `ENABLE_CPU` | true | Overall usage, per-core load, frequency, temperature trend (derived from Sensors aggregation) |
| Memory | `ENABLE_MEMORY` | true | Physical memory, swap, buffers/cached/slab, etc. |
| Disk | `ENABLE_DISK` | true | Per-partition usage, I/O, inodes |
| Network | `ENABLE_NETWORK` | true | Interface traffic, connections, listening ports |
| GPU | `ENABLE_GPU` | true | Detailed NVIDIA metrics via NVML, basic info for other vendors via DRM |
| Sensors | `ENABLE_SENSORS` | true | Temperatures, fans and other hardware sensors |
| Power | `ENABLE_POWER` | true | Battery and power profile |
| SSH | `ENABLE_SSH` | true | SSH session statistics |

If a collector is disabled, related UI sections naturally degrade to empty or hidden.

### 2. Management & Control (Managers)

All management capabilities below are **high-risk** by design. They are admin-only and recommended for LAN/VPN environments, not public Internet exposure:

| Module | Env Flag | Capabilities | Scope |
|--------|----------|-------------|-------|
| Docker | `ENABLE_DOCKER` | List/start/stop/restart/remove containers; list/remove images; view logs; prune | Current Docker daemon only |
| Systemd | `ENABLE_SYSTEMD` | List units; start/stop/restart/reload; enable/disable | Local host systemd |
| Cron | `ENABLE_CRON` | Manage crontab entries marked as managed by OpsKernel; list/create/update/delete; view logs | Local host cron |
| Power | `ENABLE_POWER` | Shutdown, reboot, suspend, cancel scheduled shutdown; view uptime and power state | Local host |
| Process | built-in | List processes; terminate by PID | Local host |

If the corresponding `ENABLE_*` flag is off, the related HTTP routes are not registered and the UI does not expose these controls.

### 3. Alerting

- Static rule-based alert engine
  - Rules: metric name, comparison operator, threshold, duration, severity (`warning` / `critical`), enabled flag
  - Supports enabling/disabling rules, restoring built-in presets
  - Tracks firing and resolved events, stored both in memory and persistence
- Notification channels
  - Webhook: JSON payload to arbitrary HTTP endpoints
  - Dashboard: in-UI listing of active and historical alerts
  - Other channels (e.g. email) are configuration-driven
- The engine only **detects and notifies**; it does **not** execute any automated remediation actions.

### 4. Authentication & Sessions

- Local user database (JSON), with built-in `admin` account and two roles: `admin` and `user`
- Passwords stored via bcrypt
- JWT-based authentication (HttpOnly cookie or Authorization header)
- Login rate limiting and optional account lockout
- Active sessions and login history tracked per user; users can view and revoke their own sessions
  - Note: revoking a session entry is not the same as revoking an already-issued JWT.

### 5. Frontend & APIs

- Built-in HTML templates and static assets implement a web dashboard
- WebSocket stream for real-time metrics; REST APIs for snapshots and management actions
- `/api/metrics` returns Prometheus text output (current implementation is a minimal stub, mainly for connectivity/integration; it is not a full system-metrics exporter)

---

## Plugin System

Design boundaries of the plugin system:

- **Plugins are Docker containers**, typically exposing an HTTP service
- The core process is responsible only for:
  - Discovering plugin manifests from a directory
  - Starting/stopping/uninstalling plugin containers via the local Docker daemon (container auto-creation is not implemented yet; you usually pre-create containers via docker compose)
  - Reverse-proxying certain URL paths to the plugin container
  - Tracking plugin runtime state and errors
- The core process does **not** dynamically load plugin code into its own address space and does not execute third-party scripts.

Examples of built-in plugins (for illustration only; reference implementations are not shipped with the core by default):

- WebShell: SSH terminal over the browser
- FileManager: SFTP-based file browser
- DB Explorer: read-oriented database exploration
- Perf Report: report generation based on monitoring data

Security boundary notes:

- Isolation level is that of Docker containers; there is no specialized sandbox beyond Docker itself
- Credentials used by plugins (SSH, databases, etc.) are provided by users or configuration and are not auto-managed by the core
- `privileged` plugins are visible and operable to admins only

The `plugins/` directory is ignored in version control by default, and plugin implementations/images are typically maintained and released separately. This section documents the plugin mechanism and typical plugin types, not a guaranteed built-in plugin set.

Plugin implementations will be released as separate repositories and follow the same license as the core (CC BY-NC 4.0).

---

## Security Model

### Roles & Authorization

- Two roles:
  - `admin`: full management capabilities (Docker/Systemd/Cron/Power/processes, user management, plugin management, etc.)
  - `user`: read-only access to monitoring data and alerts
- Authorization is implemented explicitly in handlers; there is no editable fine-grained RBAC policy.

### Authentication & Protection

- JWT authentication; logout adds the current JWT into an in-memory revoke list (not persisted across restarts)
- Login rate limiting (per IP and username) and optional lockout on repeated failures
- Multiple HTTP security headers (CSP, X-Frame-Options, X-Content-Type-Options, etc.)

Note: WebSocket Origin checks are currently permissive by default to avoid breaking reverse-proxy setups; use `WS_ALLOWED_ORIGINS` to enforce an allowlist.

### High-Risk Capabilities (Not for Public Internet)

The following are intended for trusted networks and should normally **not** be exposed directly on the public Internet:

- Docker container and image management
- Systemd service management
- Cron job creation/modification/deletion
- Process termination
- Power actions (shutdown, reboot, suspend)
- All `privileged` plugins (e.g. webshell, filemanager, db-explorer)

For Internet-facing deployments, you can disable these modules via `ENABLE_*` flags and run OpsKernel in a monitoring-only profile.

---

## Use Cases

Suitable for:

- Monitoring and day-to-day operations on single servers or small server sets
- Teams that want a local web console without a central control plane
- Development/test environments where quick host introspection is useful
- Scenarios that benefit from a few carefully chosen plugins (WebShell, FileManager, etc.)

Not suitable for:

- Large-scale clusters, data centers, or multi-tenant environments
- Centralized platform-style management of agents or nodes
- Environments expecting automatic scaling, self-healing, or runbook automation
- Long-term metrics storage and advanced analytics (OpsKernel keeps only short-term in-memory history)

---

## Constraints & Limitations

- **Single-node architecture**: no cross-node aggregation or central management
- **Linux-only**: relies on gopsutil, `/proc`, `/sys`, systemd D-Bus, etc.
- **Short metrics history**: focuses on current state and short time windows, not historical TSDB
- **No auto-remediation**: alerts do not invoke automatic actions
- **GPU support is limited**: NVIDIA gets richer data via NVML; other vendors get basic info via DRM
- **Plugins are not an in-process SDK**: integration is via HTTP/Docker, not a shared API/runtime
- **Simple RBAC**: only `admin` and `user`; no tenants, projects, or namespaces

---

## Configuration & Deployment (Overview)

### Key Environment Variables

```bash
# Core
PORT=8000                        # HTTP port
DATA_DIR=/var/lib/opskernel      # Data directory
JWT_SECRET=<random>              # JWT signing key (required in production)

# Host mounting (container mode)
HOST_FS=/hostfs
HOST_PROC=/hostfs/proc
HOST_SYS=/hostfs/sys

# Docker
DOCKER_HOST=unix:///var/run/docker.sock
DOCKER_READ_ONLY=false
```

### Minimal Profile Example

Monitoring-only profile with all management modules disabled:

```bash
ENABLE_DOCKER=false \
ENABLE_SYSTEMD=false \
ENABLE_CRON=false \
ENABLE_POWER=false \
ENABLE_SSH=false \
./opskernel
```

### Docker Example

```yaml
services:
  opskernel:
    image: opskernel:latest
    network_mode: host
    pid: host
    cap_add:
      - SYS_PTRACE
      - DAC_READ_SEARCH
    volumes:
      - /:/hostfs:ro
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/data
    environment:
      - HOST_FS=/hostfs
      - JWT_SECRET=${JWT_SECRET}
```

---

## API Overview

This section outlines categories only. Treat the router implementation and the built-in Swagger UI as the source of truth.

### Public Endpoints

- `/api/login`: user login
- `/api/health`: health check
- `/api/metrics`: Prometheus metrics export

### Authenticated (All Users)

- `/ws/stats`: WebSocket monitoring stream
- `/api/system/info`: system information snapshot
- `/api/alerts/history`: alert history
- `/api/profile/*`: user profile, preferences, login history, active sessions

### Administrative (Admins Only)

- `/api/docker/*`: Docker containers and images
- `/api/systemd/*`: systemd units
- `/api/cron/*`: cron jobs
- `/api/power/*`: power status and actions
- `/api/process/io`: process I/O (lazy-loaded by PID)
- `/api/process/kill`: terminate process (POST, admin-only)
- `/api/users/*`: user management
- `/api/plugins/list`: list plugins (filtered by role)
- `/api/plugins/action`: enable/disable (POST, admin-only)
- `/api/plugins/install`: run install hooks (POST, admin-only; mainly for privileged plugins)
- `/api/plugins/uninstall`: run uninstall hooks (POST, admin-only)
- `/api/plugins/<plugin_name>/...`: reverse proxy to the plugin container

## License

This project is licensed under the **Creative Commons Attribution-NonCommercial 4.0 International (CC BY-NC 4.0)** license. See the LICENSE file in this repository for the full text.
