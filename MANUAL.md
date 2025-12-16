# Web Monitor 使用手册

## 1. 简介

Web Monitor 是一个基于 Go 语言开发的轻量级 Linux 服务器监控与管理系统。它旨在提供一个简单、高效的 Web 界面，用于实时监控服务器状态并执行常见的管理任务。

### 核心架构

*   **后端**: Go (Golang) 1.21+
    *   使用 `gopsutil` 采集系统信息。
    *   使用 `gorilla/websocket` 推送实时数据。
  *   系统管理：
    *   Docker：通过 Docker Engine API（`DOCKER_HOST` / `unix:///var/run/docker.sock`）。
    *   Systemd：通过 systemd D-Bus（挂载 `/run/dbus/system_bus_socket`）。
    *   Cron：通过 `chroot /hostfs` 调用宿主机 `crontab`。
    *   使用 `github.com/golang-jwt/jwt/v5` 进行 JWT 认证。
*   **前端**: 纯 HTML5 / CSS3 / JavaScript (ES6+)
    *   无外部框架依赖 (如 React/Vue)，极度轻量。
    *   使用 WebSocket 接收实时数据。
*   **数据存储**: JSON 文件存储 (`/data/users.json`, `/data/operations.json`, `/data/alerts.json`)。

---

## 2. 部署指南

### 2.1 Docker 部署 (推荐)

Docker 部署提供了隔离且一致的运行环境。由于监控系统需要访问宿主机的底层信息，因此配置较为特殊。

**docker-compose.yml 详解**:

```yaml
version: '3.8'
services:
  web-monitor-go:
    build:
      context: .
      dockerfile: Dockerfile
    image: web-monitor-go:latest
    container_name: web-monitor-go
    # 采用最小能力集替代 privileged（具体能力见仓库内 docker-compose.yml）
    cap_add:
      - SYS_PTRACE
      - DAC_READ_SEARCH
      - SYS_CHROOT
    security_opt:
      - apparmor=unconfined
    network_mode: host      # 推荐开启：直接使用宿主机网络栈，监控更准确
    pid: host               # 必须开启：允许查看宿主机的所有进程
    environment:
      - PORT=38080          # 服务端口
      - JWT_SECRET=${JWT_SECRET:-} # JWT密钥，建议在生产环境设置
      - SSL_CERT_FILE=      # TLS证书文件路径（可选，启用HTTPS）
      - SSL_KEY_FILE=       # TLS私钥文件路径（可选，启用HTTPS）
      # Docker API：默认仅通过本地 proxy 访问（本容器不挂载 docker.sock）
      - DOCKER_HOST=${DOCKER_HOST:-tcp://127.0.0.1:2375}
      # Host Filesystem Configuration
      - HOST_FS=/hostfs
      - HOST_PROC=/hostfs/proc
      - HOST_SYS=/hostfs/sys
      - HOST_ETC=/hostfs/etc
      - HOST_VAR=/hostfs/var
      - HOST_RUN=/hostfs/run
      # Systemd D-Bus Connection
      - DBUS_SYSTEM_BUS_ADDRESS=unix:path=/run/dbus/system_bus_socket
    volumes:
      - /:/hostfs           # 关键：将宿主机根目录挂载到容器内的 /hostfs（Cron 等功能需要）
      - /sys:/sys:ro        # 读取硬件传感器信息
      - /proc:/proc:ro      # 读取进程和系统信息
      - /var/run/utmp:/var/run/utmp:ro # SSH会话信息
      - /run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro
      - /etc/passwd:/etc/passwd:ro
      - /etc/group:/etc/group:ro
      - web-monitor-data:/data # 持久化存储用户数据、日志和配置
    restart: unless-stopped

  # Docker API allowlist proxy（仅监听 127.0.0.1:2375，降低暴露面）
  docker-socket-proxy:
    build:
      context: .
      dockerfile: Dockerfile.docker-proxy
    container_name: docker-socket-proxy
    restart: unless-stopped
    mem_limit: 16m
    ports:
      - "127.0.0.1:2375:2375"
    environment:
      - DOCKER_SOCK=/var/run/docker.sock
    volumes:
      # 支持 rootless：通过 DOCKER_SOCK 覆盖默认 socket 路径
      - ${DOCKER_SOCK:-/var/run/docker.sock}:/var/run/docker.sock

volumes:
  web-monitor-data:
    driver: local
```

**启动**:
```bash
# 设置JWT密钥（生产环境强烈建议）
export JWT_SECRET="your-secure-jwt-secret-key-here"

# 启动服务
docker compose up -d
```

如需调整环境变量（例如 `DOCKER_HOST` / `DOCKER_READ_ONLY` 或性能相关可调项），可使用示例文件：

```bash
cp .env.example .env
# 按需编辑 .env
docker compose --env-file .env up -d
```

### 2.2 Docker Socket 安全防护 ⚠️

**重要**: 直接访问 `/var/run/docker.sock` 等同于获得宿主机 root 权限。任何应用程序的 RCE（远程代码执行）漏洞都会直接导致宿主机被完全控制。

#### 默认策略（本仓库 Docker Compose 默认）

默认不把 `docker.sock` 挂到 `web-monitor-go` 容器里，而是通过本仓库自带的 `docker-socket-proxy`（超轻量 allowlist proxy）进行访问：

*   `docker-socket-proxy`：挂载宿主机 `${DOCKER_SOCK:-/var/run/docker.sock}`，并仅暴露 `127.0.0.1:2375`。
*   `web-monitor-go`：设置 `DOCKER_HOST=tcp://127.0.0.1:2375`。

这样可以在保留 Docker 管理能力的同时，避免把高危的 docker.sock 直接暴露给主服务容器。

#### 推荐的安全配置方案

**方案 1：最小化（生产环境推荐）**
- 启用环境变量 `DOCKER_READ_ONLY=true`
- 此时后端只能查看容器/镜像，**无法执行启动、停止、删除等写操作**
- 推荐继续使用默认的 proxy（不要把 docker.sock 挂到 `web-monitor-go`）

**方案 2：使用 docker 组权限（中等安全）**
```bash
# 在宿主机上配置（容器运行前）
sudo usermod -aG docker web-monitor-user  # 假设以 web-monitor-user 身份运行
sudo chown :docker /var/run/docker.sock
sudo chmod 660 /var/run/docker.sock
```
- 在 docker-compose.yml 中：
  ```yaml
  user: "web-monitor-user"  # 非 root 身份运行
  ```

**方案 3：使用 Sidecar 代理（最安全）**
部署一个受控的代理容器，限制 API 访问范围。

本仓库已内置一个超轻量 allowlist proxy（见上面 2.1 的 `docker-socket-proxy` 服务）；如果你更倾向第三方实现，也可以使用 `tecnativa/docker-socket-proxy` 等镜像。
```yaml
version: '3.8'
services:
  docker-socket-proxy:
    image: ghcr.io/tecnativa/docker-socket-proxy:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - CONTAINERS=1
      - IMAGES=1
      # 只读示例：禁止写操作（如需保留 Start/Stop/Remove 等能力，请开启 POST/DELETE 并做好网络隔离）
      - POST=0
      - DELETE=0
    expose:
      - 2375
    networks:
      - internal

  web-monitor-go:
    # ... 其他配置
    environment:
      - DOCKER_HOST=http://docker-socket-proxy:2375
    networks:
      - internal

networks:
  internal:
    driver: bridge
```

#### 环境变量控制

本应用支持以下环境变量强制只读模式：

| 环境变量 | 值 | 说明 |
|---------|-----|------|
| `DOCKER_READ_ONLY` | `true` | 启用只读模式，所有写操作（start/stop/restart/remove）被拒绝 |
| `DOCKER_HOST` | `tcp://127.0.0.1:2375`（Compose 默认） | Docker Engine API 地址（推荐走本地 proxy） |
| `DOCKER_SOCK` | 为空 | 仅供 `docker-socket-proxy` 使用：宿主机 docker.sock 的路径（rootless 场景覆盖默认） |

#### 性能相关环境变量（可选）

这些参数用于在“进程多/系统繁忙”时降低采集开销、减少 `web-monitor-go` 的 CPU 尖峰。

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `PROCESS_IO_REFRESH` | `30s` | 进程列表里 `io_read/io_write` 的刷新周期（支持写 `60s` 或 `60` 表示 60 秒） |
| `PROCESS_CWD_REFRESH` | `60s` | 进程列表里 `cwd` 的刷新周期（支持写 `60s` 或 `60`） |
| `WS_PROCESSES_INTERVAL` | `15s` | WebSocket 可选主题 `processes` 的服务端采集周期（越大越省 CPU） |
| `WS_PROCESSES_TIMEOUT` | `3s` | WebSocket 采集 `processes` 的超时上限（避免偶发卡顿拉长 CPU 尖峰） |
| `WS_NET_DETAIL_INTERVAL` | `15s` | WebSocket 可选主题 `net_detail` 的服务端采集周期 |
| `WS_NET_DETAIL_TIMEOUT` | `3s` | WebSocket 采集 `net_detail` 的超时上限 |
| `WS_SSH_TIMEOUT` | `3s` | WebSocket 采集 SSH 统计的超时上限（在 `net_detail` 快照里使用） |

当启用只读模式时，容器 REST API 的写操作（`/api/docker/action`, `/api/docker/image/remove`）将返回 403 错误：
```json
{
  "error": "Docker read-only mode is enabled; action 'start' is not allowed"
}
```

#### 部署清单

- [ ] 评估是否真正需要容器/镜像管理功能
- [ ] 如果需要，选择方案 1（仅读）或方案 3（Sidecar）
- [ ] 如果使用方案 2，确保以非 root 用户身份运行
- [ ] 定期审计操作日志（Web Monitor 记录所有 Docker 操作）
- [ ] 通过 WAF/负载均衡器限制访问范围（仅允许信任的 IP）

### 2.3 二进制部署

适用于无法使用 Docker 的环境。

1.  **编译**:
    ```bash
    # 开启静态编译以兼容不同 Linux 发行版
    export CGO_ENABLED=0
    go build -ldflags="-s -w" -trimpath -o web-monitor-go .
    
    # 可选：使用 upx 压缩体积
    upx --lzma --best web-monitor-go
    ```

2.  **运行**:
    ```bash
    # 确保有 root 权限，否则部分监控数据无法获取
    sudo ./web-monitor-go
    
    # 或指定端口运行（同时启用只读模式）
    PORT=8080 JWT_SECRET="your-secret" DOCKER_READ_ONLY=true sudo ./web-monitor-go
    ```

---

## 3. 功能详解

### 3.1 仪表盘 (Dashboard)
*   **CPU**: 显示总使用率、每个核心的使用率、频率、温度历史曲线。
*   **内存**: 显示物理内存和 Swap 的使用情况，包含使用率历史曲线。
*   **磁盘**: 显示各分区的空间使用率和 I/O 读写速度。
*   **网络**: 显示实时上传/下载速度、总流量、连接数统计、监听端口。
*   **GPU**: 自动检测 NVIDIA, AMD, Intel 显卡，显示显存使用、温度、功耗、负载百分比。
*   **电源**: 显示系统功耗（如果硬件支持 RAPL 或电池传感器）。

### 3.2 进程管理 (Processes)
*   列出系统资源占用最高的进程（默认按内存排序）。
*   支持按 CPU 使用率或内存使用率排序。
*   显示进程的 PID, 用户, 线程数, 启动时间, 命令行参数。
*   自动缓存进程列表，减少系统负载。

### 3.3 Docker 管理
*   **容器列表**: 查看所有运行中和停止的容器，显示状态、镜像、端口映射。
*   **操作**: 支持 Start, Stop, Restart, Remove 操作（需要管理员权限）。
*   **镜像列表**: 查看本地 Docker 镜像，显示大小、创建时间。

### 3.4 系统服务 (Services)
*   基于 `systemd` 的服务管理。
*   列出所有 Service 类型的单元，显示加载状态、活动状态、描述。
*   支持 Start, Stop, Restart, Enable, Disable 操作（需要管理员权限）。
*   **原理**: 容器内通过 systemd D-Bus（挂载 `/run/dbus/system_bus_socket`）控制宿主机 systemd。

### 3.5 计划任务 (Cron)
*   读取和编辑 `root` 用户的 crontab。
*   支持添加、编辑、删除计划任务。
*   **原理**: 容器内通过 `chroot /hostfs crontab ...` 执行命令。

### 3.6 网络诊断 (Network Diagnostics)
提供网页版的常用网络工具，所有命令均经过严格输入验证，防止命令注入攻击：

*   **Ping**: 测试网络连通性（限制4个包，2秒超时）。
*   **Trace**: 路由追踪 (tracepath，限制15跳)。
*   **Dig**: DNS 查询（限制3秒超时，2次尝试）。
*   **Curl**: HTTP 请求测试（限制5秒超时，10KB最大文件大小）。

### 3.7 SSH 监控
*   **状态**: SSH 服务是否运行（检查22端口监听状态）。
*   **连接**: 当前活跃的 SSH 连接数。
*   **会话**: 显示当前登录的用户、IP地址、登录时间（基于 `who` 命令）。
*   **审计**: 统计公钥/密码登录次数，以及失败登录尝试（读取 `/var/log/auth.log` 或 `/var/log/secure`）。
*   **主机密钥**: 显示 SSH 主机密钥指纹。

### 3.8 告警配置 (Alerts)
*   **阈值告警**: 配置 CPU、内存、磁盘使用率阈值。
*   **Webhook 通知**: 支持配置 Webhook URL，触发告警时发送通知。
*   **防抖动**: 5分钟内只发送一次相同告警，避免告警风暴。
*   **开关控制**: 可全局启用/禁用告警功能。

### 3.9 电源管理 (Power Management)
*   **性能模式**: 查看当前系统电源性能模式（性能/平衡/省电）。
*   **模式切换**: 支持切换电源模式（需要管理员权限，硬件支持）。
*   **兼容性**: 支持 `powerprofilesctl` 和 `/sys/firmware/acpi/platform_profile` 两种接口。

---

## 4. 安全与权限

### 4.1 用户认证系统

#### JWT 令牌认证
*   使用标准 JWT (JSON Web Token) v5 进行会话管理。
*   令牌有效期24小时，过期后需要重新登录。
*   支持三种令牌传递方式：
    1.  Authorization 头: `Bearer <token>`
    2.  Cookie: `auth_token=<token>`
    3.  查询参数: `?token=<token>`（主要用于 WebSocket）

#### 密码策略
*   **最小长度**: 8个字符
*   **复杂度要求**: 必须包含以下四类字符中的至少三类：
    *   大写字母 (A-Z)
    *   小写字母 (a-z)
    *   数字 (0-9)
    *   特殊字符 (!@#$%^&*等)
*   **账户锁定**: 连续5次登录失败后，账户锁定15分钟。

#### 角色权限
*   **管理员 (admin)**:
    *   所有监控数据的查看权限
    *   Docker 容器/镜像管理
    *   Systemd 服务管理
    *   Cron 任务管理
    *   用户管理（创建、删除、修改）
    *   告警配置
    *   电源管理
    *   操作日志查看
*   **普通用户 (user)**:
    *   所有监控数据的查看权限（只读）
    *   修改自己的密码

### 4.2 网络安全

#### 安全HTTP头
*   **Content-Security-Policy (CSP)**: 严格限制资源加载，防止 XSS 攻击
*   **X-Frame-Options**: DENY，防止点击劫持
*   **X-XSS-Protection**: 1; mode=block，启用XSS过滤
*   **X-Content-Type-Options**: nosniff，防止MIME类型混淆
*   **Referrer-Policy**: strict-origin-when-cross-origin
*   **Strict-Transport-Security**: 启用HTTPS时自动设置HSTS

#### 输入验证
*   所有用户输入均经过严格验证，使用正则表达式白名单机制
*   网络诊断工具的目标地址验证：仅允许合法IPv4、IPv6、域名格式
*   防止命令注入：使用参数化命令执行，不拼接字符串

#### HTTPS 支持
*   支持配置 TLS 证书启用 HTTPS
*   环境变量：`SSL_CERT_FILE`, `SSL_KEY_FILE`
*   启用后自动设置 HSTS 头

### 4.3 操作审计
*   **完整日志记录**: 记录所有关键操作：
    *   用户登录/登出
    *   密码修改
    *   用户创建/删除
    *   Docker 容器操作
    *   Systemd 服务操作
    *   Cron 任务修改
    *   告警配置修改
    *   电源模式切换
*   **日志字段**: 时间、用户名、操作类型、详细信息、IP地址
*   **日志保留**: 保留最近1000条操作日志，自动保存到 `/data/operations.json`
*   **日志查看**: 仅管理员可查看操作日志

### 4.4 默认账户
*   系统初始化时会自动创建默认管理员：
    *   用户: `admin`
    *   密码: `admin123` (bcrypt hash)
*   **强烈建议**首次登录后在 "Profile" 页面修改密码。

### 4.5 安全建议
1.  **生产环境部署**:
    *   设置 `JWT_SECRET` 环境变量，使用强密钥
    *   配置 HTTPS，使用有效 TLS 证书
    *   修改默认管理员密码
    *   定期备份 `/data` 目录

2.  **网络访问控制**:
    *   不要将服务直接暴露在公网
    *   使用 Nginx 反向代理，配置访问限制
    *   配置防火墙，限制访问来源IP

3.  **权限最小化**:
    *   为不同用户创建对应角色的账户
    *   日常监控使用普通用户账户
    *   仅管理员执行管理操作

4.  **定期维护**:
    *   定期检查操作日志
    *   定期更新系统和服务
    *   定期备份重要数据

---

## 5. 常见问题 (FAQ)

### 部署问题

**Q: 为什么看不到 Systemd 服务或 Cron 任务？**
A: 请检查 Docker 挂载配置。必须将宿主机根目录挂载到容器的 `/hostfs` (`-v /:/hostfs`)。程序依赖此路径来访问宿主机的系统工具。

**Q: 为什么 Docker 管理页面为空？**
A: 默认走 `docker-socket-proxy`，请检查：

1. `docker-socket-proxy` 容器是否在运行
2. `web-monitor-go` 的 `DOCKER_HOST` 是否为 `tcp://127.0.0.1:2375`
3. Rootless Docker：是否正确设置了 `DOCKER_SOCK` 指向实际 socket（例如 `$XDG_RUNTIME_DIR/docker.sock`）

**Q: 为什么温度显示为 0 或不准确？**
A: 需要挂载 `/sys` 目录 (`-v /sys:/sys:ro`) 并确保容器具备读取传感器所需的权限（本仓库默认 Compose 使用 `cap_add` 最小能力集）。部分硬件/驱动可能仍需要额外权限或内核模块支持。

**Q: 为什么网络监控不显示流量？**
A: 建议使用 `network_mode: host` 以便准确监控宿主机网络流量。

**Q: GPU 监控显示不可用或数据为空？**
A: GPU 监控需要宿主机有相应的 GPU 硬件和驱动支持。对于容器部署，需要额外挂载 GPU 设备文件和相应的库文件。例如，对于 NVIDIA GPU，需要挂载 `/dev/nvidia*` 设备。同时确保容器有权限访问这些设备。

### 认证与权限问题

**Q: 忘记管理员密码怎么办？**
A: 可以通过以下步骤重置：
```bash
# 进入容器
docker exec -it web-monitor-go sh

# 删除用户数据库
rm /data/users.json

# 重启容器
docker restart web-monitor-go
```
重启后系统将重新创建默认账户 (admin/admin123)。

**Q: 如何创建新用户？**
A: 使用管理员账户登录后，进入 "Users" 页面，点击 "Create User" 按钮。

**Q: 普通用户能执行哪些操作？**
A: 普通用户只能查看监控数据，不能执行任何管理操作。可以修改自己的密码。

### 功能问题

**Q: 告警功能如何配置？**
A: 使用管理员账户登录，进入 "Alerts" 页面，启用告警并配置阈值和 Webhook URL。

**Q: 电源管理功能需要什么条件？**
A: 需要硬件支持（Intel/AMD CPU 的 RAPL 或 ACPI 平台配置文件）。大多数现代服务器和台式机支持此功能。

**Q: SSH 监控为什么显示无会话？**
A: 需要挂载 `/var/run/utmp` 文件 (`-v /var/run/utmp:/var/run/utmp:ro`) 来读取登录会话信息。

**Q: Prometheus 指标如何采集？**
A: 访问 `http://<server>:<port>/metrics` 端点即可获取 Prometheus 格式的指标。

**Q: 进程管理页面显示的进程信息不完整？**
A: 进程信息通过缓存机制减少系统负载，默认缓存15秒。如果需要实时数据，可以手动刷新页面。

### 性能与资源

**Q: 系统资源占用如何？**
A: 典型情况下：
*   内存: 50-100 MB
*   CPU: < 1% (空闲时)
*   磁盘: < 50 MB (不含日志)

**Q: 如何调整资源限制？**
A: 在 `docker-compose.yml` 的 `deploy.resources` 部分调整 CPU 和内存限制。

**Q: 日志文件会无限增长吗？**
A: 不会。操作日志最多保留1000条，自动清理旧记录。

### 故障排除

**Q: 服务启动失败怎么办？**
```bash
# 查看容器日志
docker logs web-monitor-go

# 查看详细日志
docker logs -f web-monitor-go
```

**Q: WebSocket 连接断开怎么办？**
A: 这是正常现象，前端会自动重连。检查网络连接和防火墙设置。

**Q: 如何备份数据？**
A: 备份 `/data` 目录下的所有文件：
```bash
# 备份到本地
tar -czf web-monitor-backup-$(date +%Y%m%d).tar.gz /data/*

# 或直接从容器复制
docker cp web-monitor-go:/data ./backup/
```

**Q: 如何升级到新版本？**
```bash
# 停止并删除旧容器
docker compose down

# 拉取新镜像或重新构建
docker compose build --pull

# 启动新容器
docker compose up -d
```

---

## 6. 技术细节

### 6.1 数据采集
*   **CPU**: 使用 `gopsutil/cpu` 采集使用率、频率、时间统计
*   **内存**: 使用 `gopsutil/mem` 采集物理内存和交换分区信息
*   **磁盘**: 使用 `gopsutil/disk` 采集分区、使用率、IO统计
*   **网络**: 使用 `gopsutil/net` 采集接口统计、连接状态
*   **GPU**: 通过 `/sys/class/drm` 和 PCI ID 数据库识别显卡信息，支持 NVIDIA、AMD、Intel 显卡
*   **温度**: 通过 `gopsutil/host` 或 `/sys/class/hwmon` 读取传感器
*   **进程**: 使用 `gopsutil/process` 采集进程信息，按内存排序
*   **系统信息**: 通过新的 `system` 模块采集主机名、操作系统、内核版本、正常运行时间等信息

### 6.2 缓存机制
*   **进程缓存**: 15秒缓存，减少频繁的进程枚举
*   **GPU信息缓存**: 60秒缓存，减少文件系统访问
*   **连接状态缓存**: 10秒缓存，优化性能
*   **SSH统计缓存**: 120秒缓存，减少日志解析开销

### 6.3 错误处理
*   **优雅降级**: 某个数据源失败时不影响其他功能
*   **详细日志**: 所有错误都记录到日志，便于排查
*   **用户友好**: 前端显示友好的错误信息，不暴露技术细节

### 6.4 性能优化
*   **WebSocket 推送**: 实时数据通过 WebSocket 推送，减少 HTTP 请求。
*   **极低资源占用**: 经过 pprof 深度分析与优化，大幅降低了 CPU 和内存消耗。
*   **高效采集**:
    *   **智能缓存**: 引入进程静态信息缓存（如命令行、启动时间），避免重复读取 `/proc` 文件系统。
    *   **对象复用**: 优化网络和进程采集逻辑，复用对象以减少 GC 压力和系统调用。
*   **高性能序列化**: 针对高频数据（如进程列表）手动实现 `MarshalJSON` 接口，避开反射开销，显著提升大数据量下的序列化性能。
*   **静态资源优化**:
    *   **本地化**: 所有静态资源（Font Awesome, Chart.js, JetBrains Mono）均已本地化，无外部 CDN 依赖。
    *   **强缓存**: 实现静态资源指纹化 (Fingerprinting) 和 `Cache-Control: immutable` 策略，加速前端加载。
*   **增量更新**: SSH 认证日志使用增量读取，避免重复处理。
*   **资源限制**: Docker 部署默认限制 CPU 和内存使用。
*   **静态编译**: 二进制文件静态编译，无外部依赖。

---

## 7. 更新日志

### 最新版本 (v1.5)
*   **性能飞跃**: 深度优化核心采集逻辑，大幅降低 CPU 占用。
*   **内网友好**: 彻底移除所有外部 CDN 依赖，静态资源完全本地化。
*   **底层优化**: 引入对象池和自定义 JSON 序列化，减少 GC 压力。
*   **安全增强**: 移除 pprof 调试接口，减少攻击面。

### 版本历史
*   **v1.4**: 架构重构，模块化改进，功能增强
*   **v1.3**: 安全增强和性能优化
*   **v1.2**: 添加 SSH 监控、GPU 支持
*   **v1.1**: 添加 Docker、Systemd、Cron 管理
*   **v1.0**: 初始版本，基础监控功能

---

## 8. 获取帮助

*   **GitHub Issues**: [https://github.com/AnalyseDeCircuit/web-monitor/issues](https://github.com/AnalyseDeCircuit/web-monitor/issues)
*   **文档**:
    *   [README.md](README.md) - 项目概述和快速开始
    *   [README_EN.md](README_EN.md) - English documentation
    *   [MANUAL.md](MANUAL.md) - 详细使用手册（本文档）

---

**最后更新**: 2025年12月16日  
**版本**: 1.5
