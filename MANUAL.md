# Web Monitor (Go) 使用手册

本文档详细介绍了 Web Monitor (Go) 的安装、配置、功能特性及安全部署建议。

## 1. 安装与部署

### 1.1 Docker Compose (推荐)

为了获得最完整的系统监控能力（特别是 SSH 状态和宿主机网络连接），建议使用 `network_mode: host` 模式。

**docker-compose.yml**
```yaml
version: '3.8'
services:
  web-monitor-go:
    build: .  # 或者使用 image: web-monitor-go:offline
    container_name: web-monitor-go
    privileged: true  # 需要特权以读取部分硬件信息
    network_mode: host # 共享宿主机网络栈
    environment:
      - PORT=38080    # 服务监听端口
    volumes:
      - /:/hostfs:ro                   # 挂载宿主根目录（只读），用于读取磁盘、日志等
      - /sys:/sys:ro                   # 读取硬件信息（GPU、电源等）
      - /proc:/proc                    # 读取进程和系统信息
      - /var/run/docker.sock:/var/run/docker.sock:ro # (可选) 如果需要监控容器
      - /var/run/utmp:/var/run/utmp:ro # 读取 SSH 登录会话
      - /etc/passwd:/etc/passwd:ro     # 解析用户名称
      - /etc/group:/etc/group:ro       # 解析组名称
    restart: unless-stopped
```

启动服务：
```bash
docker compose up -d --build
```

### 1.2 环境变量

| 变量名 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `PORT` | `8000` | 服务监听的 HTTP 端口。在 `host` 模式下，这就是宿主机对外暴露的端口。 |

## 2. 功能特性详解

### 2.1 概览 (General)
- **系统信息**：显示内核版本、运行时间、CPU/内存/磁盘摘要。
- **SSH Status**：
    - **Service**: SSH 服务运行状态。
    - **Connections**: 当前 TCP 连接到 22 端口的数量。
    - **Active Sessions**: 当前登录的用户会话（基于 `who` 命令和 TCP 连接推断）。
- **GPU Summary**: 简要显示第一块 GPU 的名称、温度、负载和频率。

### 2.2 GPU Monitor
- **支持设备**：目前主要优化了 Intel 核显 (iGPU)，如 Alder Lake-N 系列。
- **指标说明**：
    - **Frequency (频率)**：GPU 当前的运行频率。空闲时可能显示 `0 MHz` 或 `--`，这是正常的省电状态 (RC6)。
    - **VRAM (显存)**：集成显卡使用共享内存，部分驱动不直接暴露显存用量，可能显示 `0 B / 0 B`。
    - **Load (负载)**：GPU 核心的使用率。
- **故障排查**：如果显示 "No GPUs detected"，请检查容器是否添加了 `privileged: true` 或挂载了 `/sys`。

### 2.3 SSH Monitor
- **Known Hosts**: 统计 `/root/.ssh/known_hosts` 及 `/home/*/.ssh/known_hosts` 中的条目数量。
- **Auth Methods**: 分析 `/var/log/auth.log` (或 `secure`)，统计公钥、密码等认证方式的使用次数。
- **Host Key**: 显示 RSA Host Key 指纹，用于核对服务器身份。

### 2.4 Network
- **Host 模式优势**：使用 `network_mode: host` 后，面板能看到宿主机所有物理网卡（如 `eth0`, `wlan0`）的真实流量和连接状态。
- **Bridge 模式限制**：如果使用默认 Bridge 网络，只能看到容器内部的 `eth0`，无法监控宿主机的整体网络状况。

## 3. 安全部署建议 (Cloudflare Tunnel)

本服务包含敏感系统信息，**强烈建议不要直接将端口暴露在公网**。推荐使用 Cloudflare Tunnel 配合 Access 进行安全访问。

### 3.1 为什么更安全？
- **无需公网端口**：不需要在路由器做端口映射，减少被扫描风险。
- **身份认证**：Cloudflare Access 提供强制登录（支持 Google/GitHub 等），只有认证通过才能访问面板。
- **隐藏源站 IP**：攻击者无法直接获取你家里的真实 IP。

### 3.2 配置步骤简述

1.  **本地监听**：确保 `web-monitor-go` 监听在本地端口，例如 `38080`。
2.  **安装 cloudflared**：在宿主机安装 Cloudflare Tunnel 客户端。
3.  **创建隧道**：
    ```bash
    cloudflared tunnel create my-monitor
    ```
4.  **配置 Ingress** (`config.yml`)：
    ```yaml
    tunnel: <Tunnel-UUID>
    credentials-file: /root/.cloudflared/<Tunnel-UUID>.json
    ingress:
      - hostname: monitor.yourdomain.com
        service: http://127.0.0.1:38080
      - service: http_status:404
    ```
5.  **启动隧道**：`systemctl start cloudflared`
6.  **设置 Access 策略**：
    - 登录 Cloudflare Dashboard -> Zero Trust -> Access -> Applications。
    - 添加应用 `monitor.yourdomain.com`。
    - 设置 Policy：`Include Emails = your-email@example.com`。

现在，你可以在任何地方通过 `https://monitor.yourdomain.com` 安全访问你的监控面板，且必须先通过邮箱验证码登录。
