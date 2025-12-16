# Web Monitor (Go Version)

一个轻量级、高性能的 Linux 服务器监控与管理面板。采用 Go 语言开发后端，纯 HTML/JS 前端，资源占用极低，部署简单。

## ✨ 功能特性

*   **实时监控**：CPU、内存、磁盘 I/O、网络流量、GPU (NVIDIA/AMD/Intel)、温度传感器。
*   **进程管理**：查看系统 Top 进程，支持按 CPU、内存、IO 排序。
    *   **懒加载优化**：进程 I/O 详情仅在点击时加载，大幅降低常规采集开销。
*   **Docker 管理**：查看容器/镜像列表，支持启动、停止、重启、删除容器，查看容器日志和统计信息。
*   **系统管理**：
    *   **Systemd 服务**：查看服务状态，支持启动、停止、重启、启用、禁用服务。
    *   **Cron 任务**：查看和编辑计划任务。
*   **SSH 监控**：监控 SSH 连接数、活跃会话、登录历史及失败记录。
    *   **多级缓存**：采用多级 TTL 缓存策略（连接数 60s / 登录记录 5m / HostKey 1h），支持手动强制刷新。
    *   **内存优化**：使用 VmRSS 统计 sshd 内存占用，更准确反映真实开销。
*   **安全审计**：内置用户角色系统 (Admin/User)，记录关键操作日志。
*   **Prometheus 集成**：暴露 `/metrics` 接口，支持 Prometheus/Grafana 采集。
*   **告警配置**：支持 CPU、内存、磁盘使用率阈值告警，可配置 Webhook。
*   **电源管理**：查看和调整系统电源性能模式（需硬件支持）。
*   **GPU 监控**：支持 NVIDIA、AMD、Intel GPU 的温度、使用率、显存监控。

## ⚡ 性能与优化

本项目经过深度性能调优，致力于在提供丰富功能的同时保持极低的资源占用：

*   **极低资源占用**：经过 pprof 深度分析与优化，大幅降低了 CPU 和内存消耗。
*   **零外部依赖**：所有静态资源（Font Awesome, Chart.js, JetBrains Mono）均已本地化，**内网环境完美运行**，彻底解决 CDN 加载慢或被墙的问题。
*   **高效采集**：
    *   **Linux 原生解析**：网络详情采集直接解析 `/proc/net/{tcp,udp}`，替代通用库调用，性能提升显著。
    *   **智能缓存**：引入进程静态信息缓存（如命令行、启动时间），避免重复读取 `/proc` 文件系统。
    *   **对象复用**：优化网络和进程采集逻辑，复用对象以减少 GC 压力和系统调用。
*   **按需加载**：
    *   **动态订阅**：WebSocket 支持按页面动态订阅（如仅在概览页订阅 Top10 进程，详情页才订阅全量），大幅减少数据传输。
    *   **IO 懒加载**：进程 I/O 统计仅在查看详情时通过 REST API 拉取，避免每轮采集的 I/O 开销。
*   **高性能序列化**：针对高频数据（如进程列表）手动实现 `MarshalJSON` 接口，避开反射开销。
*   **静态资源优化**：实现静态资源指纹化 (Fingerprinting) 和强缓存策略 (`Cache-Control: immutable`)，加速前端加载。

## 🚀 快速部署 (Docker Compose)

这是推荐的部署方式，已针对功能完整性进行了预配置。

1.  确保已安装 Docker 和 Docker Compose。
2.  在项目根目录下运行：

```bash
docker compose up -d
```

3.  访问浏览器：`http://<服务器IP>:38080`
4.  **默认账号**：`admin`
5.  **默认密码**：`admin123` **(请登录后立即修改)**

### ⚠️ 关键配置说明

为了使监控和管理功能正常工作，容器需要较高的权限和特定的挂载：

*   `cap_add`: 采用最小能力集（替代 `privileged: true`），用于读取进程/日志与执行必要的系统操作（见 `docker-compose.yml`）。
    *   `SYS_PTRACE`: 读取 `/proc` 的进程信息。
    *   `DAC_READ_SEARCH`: 读取部分受限文件（如认证/审计日志）。
    *   `SYS_CHROOT`: 执行 `chroot`（用于 Cron 管理等场景）。
*   `security_opt: apparmor=unconfined`: 当前默认启用（主要用于 systemd D-Bus 控制在部分发行版/内核策略下可用）。
*   `network_mode: host`: 推荐开启，以便准确监控宿主机网络流量。
*   `pid: host`: 必须开启，以便获取宿主机进程列表。
*   `volumes`:
    *   `/:/hostfs`: **核心配置**。用于访问宿主机文件系统（进程/日志/硬件信息、Cron 管理等）。
    *   `/run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro`: 用于 Systemd 管理（通过 D-Bus）。
    *   `/proc`, `/sys`: 用于采集硬件信息和 GPU 监控。
    *   GPU 设备（如 `/dev/nvidia*`）: 如需 GPU 监控，需挂载相应设备。

#### Docker 管理（默认通过本地 Proxy）

为降低风险，默认 **不在 `web-monitor-go` 容器内挂载** `docker.sock`，而是通过同编排内的 `docker-socket-proxy`（仅监听 `127.0.0.1:2375`）转发有限的 Docker Engine API：

*   `web-monitor-go` 通过 `DOCKER_HOST=tcp://127.0.0.1:2375` 访问 Docker（仅走 proxy）。
*   只有 `docker-socket-proxy` 挂载宿主机的 `${DOCKER_SOCK:-/var/run/docker.sock}`。
    *   Rootless Docker 场景：把 `DOCKER_SOCK` 指向实际的 socket 路径即可（例如 `$XDG_RUNTIME_DIR/docker.sock`）。

## 🛠️ 手动编译与运行

如果你不想使用 Docker，可以直接编译二进制文件运行。

### 编译

```bash
# 开启静态编译以兼容不同 Linux 发行版
export CGO_ENABLED=0
go build -ldflags="-s -w" -trimpath -o web-monitor-go ./cmd/server/main.go

# 可选：使用 upx 压缩体积
upx --lzma --best web-monitor-go
```

### 运行

```bash
# 默认监听 8000 端口
./web-monitor-go

# 指定端口
PORT=8080 ./web-monitor-go
```

注意：直接运行时，程序会直接调用宿主机命令，不需要 `/hostfs` 机制。

## 🔒 安全特性

*   **HttpOnly Cookie 鉴权**：全面移除前端 localStorage Token 存储，采用 HttpOnly Cookie 进行鉴权，有效防御 XSS 攻击下的 Token 窃取。
*   **Cloudflare/Proxy 支持**：支持识别 `CF-Connecting-IP` 等代理头（需配合防火墙限制源站仅允许代理 IP 访问）。
*   **Docker Socket 隔离**：通过只读/白名单 Proxy 访问 Docker API，避免容器逃逸风险。
*   **最小权限原则**：Docker 容器不再使用 `privileged` 模式，而是精细化配置 Linux Capabilities。
*   **安全头配置**：内置 CSP (Content Security Policy)、HSTS 等安全响应头。
*   **CSRF 防护**：基于 Cookie 的 SameSite 策略和 Origin 校验。

## 📝 许可证

CC BY-NC 4.0
