# Web Monitor (Go) 使用手册

本文档详细介绍了 Web Monitor (Go) 的安装、配置、功能特性及安全部署建议。

## 目录

1. [安装与部署](#1-安装与部署)
2. [功能特性详解](#2-功能特性详解)
3. [Web 界面说明](#3-web-界面说明)
4. [API 数据格式](#4-api-数据格式)
5. [故障排查](#5-故障排查)
6. [安全部署建议](#6-安全部署建议cloudflare-tunnel)
7. [性能优化](#7-性能优化)

---

## 1. 安装与部署

### 1.1 Docker Compose (推荐)

为了获得最完整的系统监控能力（特别是 SSH 状态和宿主机网络连接），建议使用 `network_mode: host` 模式。

**docker-compose.yml**
```yaml
version: '3.8'
services:
  web-monitor-go:
    build: .  # 或者使用 image: web-monitor-go:latest
    container_name: web-monitor-go
    privileged: true  # 需要特权以读取部分硬件信息
    network_mode: host # 共享宿主机网络栈
    environment:
      - PORT=38080    # 服务监听端口
    volumes:
      - /:/hostfs:ro                   # 挂载宿主根目录（只读），用于读取磁盘、日志等
      - /sys:/sys:ro                   # 读取硬件信息（GPU、电源等）
      - /proc:/proc                    # 读取进程和系统信息
      - /var/run/docker.sock:/var/run/docker.sock:ro # (可选) 监控容器
      - /var/run/utmp:/var/run/utmp:ro # 读取 SSH 登录会话
      - /etc/passwd:/etc/passwd:ro     # 解析用户名称
      - /etc/group:/etc/group:ro       # 解析组名称
    restart: unless-stopped
```

启动服务：
```bash
docker-compose up -d --build
```

查看日志：
```bash
docker-compose logs -f web-monitor-go
```

### 1.2 本地编译安装

**编译要求**：Go 1.21+

```bash
# 克隆仓库
git clone https://github.com/AnalyseDeCircuit/web-monitor.git
cd web-monitor

# 下载依赖
go mod download

# 编译
go build -o web-monitor main.go

# 运行
./web-monitor
```

### 1.3 环境变量配置

| 变量名 | 默认值 | 说明 | 示例 |
| :--- | :--- | :--- | :--- |
| `PORT` | `8000` | 服务监听的 HTTP 端口 | `PORT=8080` |
| `SHELL` | `/bin/sh` | 系统 Shell 类型（显示用） | `SHELL=/bin/bash` |
| `LANG` | `C` | 系统区域设置 | `LANG=zh_CN.UTF-8` |

在 Docker 中：
```yaml
environment:
  - PORT=38080
  - LANG=zh_CN.UTF-8
```

在 systemd 中，编辑 `/etc/systemd/system/web-monitor-go.service`：
```ini
[Service]
Environment="PORT=38080"
Environment="LANG=zh_CN.UTF-8"
```

---

## 2. 功能特性详解

### 2.1 CPU 监控

**指标说明**：
- **使用率 (CPU %)**：总体 CPU 使用百分比，0-100%
- **每核使用率 (Per Core)**：各 CPU 核心的使用百分比
- **频率 (Frequency)**：当前 CPU 运行频率（MHz）
  - 平均频率：所有核心的平均值
  - 每核频率：各核心的实时频率（支持动态频率调整）
- **负载 (Load Avg)**：
  - 1 分钟平均负载
  - 5 分钟平均负载
  - 15 分钟平均负载
- **温度历史 (Temp History)**：最近 300 个采样点的温度趋势

**硬件信息**：
- **型号 (Model)**：CPU 型号名称
- **核心数 (Cores)**：物理核心数
- **线程数 (Threads)**：逻辑线程数
- **最高频率 (Max Freq)**：CPU 最高工作频率

### 2.2 GPU 监控

**支持的 GPU**：
- Intel 核显（Alder Lake-N、Tiger Lake 等）
- NVIDIA 独立显卡（通过 `/sys/class/drm/` 接口）
- AMD 显卡（通过 `/sys/class/drm/` 接口）

**指标说明**：
- **名称 (Name)**：GPU 名称和型号
- **显存使用 (VRAM)**：
  - 总容量：如 10.0 GiB
  - 已用容量：如 4.5 GiB
  - 使用率 (%)：百分比形式
  - 注意：集成显卡使用共享系统内存，某些驱动可能无法直接读取
- **频率 (Frequency)**：GPU 当前运行频率（MHz）
  - 空闲时可能显示 0 MHz，这是正常的省电状态（RC6）
- **温度 (Temperature)**：GPU 当前温度（℃）
- **功率 (Power)**：GPU 当前功耗（瓦）
- **负载 (Load)**：GPU 核心使用率（%）
- **进程列表**：占用 GPU 显存的进程列表
  - 进程 ID
  - 进程名称
  - 显存占用

### 2.3 内存监控

**指标说明**：
- **总内存 (Total)**：系统总物理内存
- **已用 (Used)**：当前使用的内存
- **可用 (Available)**：可供分配的可用内存
- **缓冲 (Buffers)**：内核缓冲区内存
- **缓存 (Cached)**：页面缓存内存
- **使用率 (%)**：已用内存占总内存的百分比
- **历史数据 (History)**：最近 300 个采样点的使用率趋势

**交换分区 (Swap)**：
- **总容量 (Total)**
- **已用 (Used)**
- **可用 (Free)**
- **使用率 (%)**

### 2.4 磁盘监控

**分区信息**：
- **设备名 (Device)**：如 `/dev/sda1`
- **挂载点 (Mountpoint)**：如 `/`
- **文件系统 (Fstype)**：如 `ext4`、`btrfs`
- **总大小 (Total)**：分区总容量
- **已用 (Used)**：已使用容量
- **可用 (Free)**：剩余可用容量
- **使用率 (%)**：已用占总容量的百分比

**磁盘 I/O 统计**：
- **读字节数 (Read Bytes)**
- **写字节数 (Write Bytes)**
- **读操作数 (Read Count)**
- **写操作数 (Write Count)**

**Inode 统计**：
- 挂载点
- 总 Inode 数
- 已用 Inode 数
- 可用 Inode 数
- 使用率

### 2.5 网络监控

**总体统计**：
- **发送字节数 (Bytes Sent)**：累计发送流量
- **接收字节数 (Bytes Received)**：累计接收流量

**网络接口 (Interfaces)**：
- **接口名称**：如 `eth0`, `wlan0`
- **IP 地址**
- **状态 (Is Up)**：接口是否启用
- **流量统计**：该接口的发送/接收字节数
- **错误和丢包**：
  - 入站错误 (Errors In)
  - 出站错误 (Errors Out)
  - 入站丢包 (Drops In)
  - 出站丢包 (Drops Out)
- **速率 (Speed)**：接口速率（Mbps），如 1000（1 Gbps）

**连接状态分类**：
- **ESTABLISHED**：已建立连接
- **TIME_WAIT**：等待关闭的连接
- **LISTEN**：监听状态
- 其他 TCP 状态

**监听端口**：
- 列表中的端口号表示服务正在监听

**套接字统计**：
- TCP、UDP 等协议的套接字数量统计

### 2.6 SSH 审计监控

**连接统计**：
- **状态 (Status)**：SSH 服务是否运行（running/stopped）
- **当前连接数 (Connections)**：TCP 22 端口的已建立连接数
- **活跃会话 (Active Sessions)**：当前登录的用户会话
  - 用户名
  - 来源 IP
  - 登录时间

**认证统计**：
- **公钥认证 (Public Key)**：使用公钥认证成功的次数
- **密码认证 (Password)**：使用密码认证成功的次数
- **其他方式 (Other)**：其他认证方式成功次数
- **失败次数 (Failed Logins)**：认证失败的次数

**主机密钥 (Host Key)**：
- 显示 RSA 主机公钥的指纹
- 用于客户端核对服务器身份

**已知主机 (Known Hosts)**：
- 统计 `/root/.ssh/known_hosts` 和 `/home/*/.ssh/known_hosts` 中的条目数
- 表示该服务器登录过的主机数量

**SSH 进程内存 (SSH Process Memory)**：
- 所有 sshd 进程占用的总内存百分比

### 2.7 进程监控

**进程列表**：
- **进程 ID (PID)**
- **进程名 (Name)**
- **用户 (Username)**：运行进程的用户
- **CPU 使用率 (%)**
- **内存使用率 (%)**
- **线程数 (Num Threads)**
- **运行时长 (Uptime)**：进程启动到现在的运行时间
- **命令行 (Cmdline)**：启动进程的完整命令
- **工作目录 (Cwd)**
- **IO 统计**：
  - 读取字节数 (IO Read)
  - 写入字节数 (IO Write)

**顶级进程**：
- 默认按 CPU 或内存占用率排序，显示占用最多资源的进程

**OOM 风险进程**：
- 内存占用率高的进程，可能面临 OOM 杀死风险

---

## 3. Web 界面说明

### 3.1 仪表板布局

Web 界面采用卡片式设计，主要分为以下几个部分：

**顶部概览**：
- 系统信息：操作系统、内核版本、主机名、运行时间
- 快速指标：CPU、内存、磁盘、IP 地址

**实时数据区**：
- **CPU 模块**：总体使用率、每核详情、频率、负载
- **GPU 模块**：GPU 名称、显存、频率、温度、占用进程
- **内存模块**：物理内存、交换分区、使用率历史图表
- **磁盘模块**：各分区使用情况、I/O 统计
- **网络模块**：接口列表、连接统计、监听端口
- **SSH 模块**：连接数、活跃会话、认证统计
- **进程模块**：占用资源最多的进程列表

### 3.2 数据更新机制

- **WebSocket 连接**：浏览器自动与服务器建立 WebSocket 连接
- **更新间隔**：可通过 URL 参数 `interval` 配置，范围 2-60 秒（默认 2 秒）
- **实时推送**：服务器按设定间隔推送完整系统数据

---

## 4. API 数据格式

### 4.1 WebSocket `/ws/stats` 接口

**连接方式**：
```javascript
const ws = new WebSocket('ws://localhost:8000/ws/stats?interval=2');
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  // 处理数据
};
```

**响应数据结构**（简化版）：
包含 CPU、内存、GPU、网络、SSH、进程等完整系统信息的 JSON 对象。

### 4.2 GET `/api/info` 接口

**请求**：
```bash
curl http://localhost:8000/api/info
```

**响应示例**：
```json
{
  "header": "root@server-name",
  "os": "Ubuntu 22.04 LTS (5.15.0-89-generic)",
  "kernel": "5.15.0-89-generic",
  "uptime": "45d 12h 30m 20s",
  "shell": "/bin/bash",
  "cpu": "Intel(R) Core(TM) i7-10700 CPU @ 2.90GHz (8) @ 2.90 GHz",
  "gpu": "Intel Iris Xe Graphics + NVIDIA RTX 3080",
  "memory": "8.2 GiB / 16.0 GiB (51.2%)",
  "swap": "0.5 GiB / 2.0 GiB (25.0%)",
  "disk": "250.5 GiB / 500.0 GiB (50.1%)",
  "ip": "192.168.1.100",
  "locale": "en_US.UTF-8"
}
```

---

## 5. 故障排查

### 5.1 常见问题

#### Q1: 无法访问 Web 界面
**症状**：浏览器无法连接到 `http://localhost:8000` 或 `http://IP:8000`

**原因和解决**：
1. 应用未运行
   ```bash
   ps aux | grep web-monitor
   docker-compose restart web-monitor-go
   sudo systemctl restart web-monitor-go
   ```

2. 防火墙阻止
   ```bash
   sudo ufw allow 8000
   sudo ufw allow 38080
   ```

3. 端口被占用
   ```bash
   sudo lsof -i :8000
   PORT=9000 ./web-monitor
   ```

#### Q2: GPU 信息显示 "Unknown" 或 "No GPUs detected"
**症状**：GPU 模块显示"No GPUs detected"或"Unknown GPU"

**原因和解决**：
1. Docker 未挂载 `/sys` 目录
   ```yaml
   volumes:
     - /sys:/sys:ro
   ```

2. GPU 驱动问题
   ```bash
   ls /sys/class/drm/
   lspci | grep -i gpu
   ```

3. 权限不足
   ```yaml
   privileged: true
   ```

#### Q3: SSH 连接统计为空或不准确
**症状**：SSH 模块显示 0 连接或会话为空

**原因和解决**：
1. 权限不足（需要 root）
   ```bash
   sudo ./web-monitor
   ```

2. SSH 日志文件不可访问
   ```bash
   ls -l /var/log/auth.log
   ls -l /var/log/secure
   ```

3. SSH 服务未运行
   ```bash
   sudo systemctl status ssh
   sudo systemctl status sshd
   ```

#### Q4: 某些指标显示为 0 或 "Unknown"
**症状**：温度、功率、磁盘 I/O 等指标无法读取

**原因和解决**：
1. 容器化环境中某些系统文件不可用
   ```yaml
   volumes:
     - /proc:/proc
     - /sys:/sys:ro
     - /:/hostfs:ro
   ```

2. 驱动或硬件不支持
   - 某些虚拟机和云环境不支持某些指标
   - 这是正常现象，应用会优雅地处理

### 5.2 日志查看

**Docker**：
```bash
docker-compose logs -f web-monitor-go
docker logs -f web-monitor-go
```

**Systemd**：
```bash
sudo journalctl -u web-monitor-go -f
sudo journalctl -u web-monitor-go -n 50
```

**直接运行**：
```bash
./web-monitor 2>&1 | tee app.log
```

---

## 6. 安全部署建议（Cloudflare Tunnel）

本服务包含敏感系统信息，**强烈建议不要直接将端口暴露在公网**。推荐使用 Cloudflare Tunnel 配合 Access 进行安全访问。

### 6.1 为什么更安全？

- **无需公网端口**：不需要在路由器做端口映射，减少被扫描风险
- **身份认证**：Cloudflare Access 提供强制登录（支持 Google/GitHub 等），只有认证通过才能访问面板
- **隐藏源站 IP**：攻击者无法直接获取你的真实 IP 地址
- **DDOS 防护**：Cloudflare 的全球网络提供 DDOS 保护

### 6.2 配置步骤

#### 步骤 1：本地监听

确保 `web-monitor-go` 监听在本地端口，例如 `127.0.0.1:38080`。

#### 步骤 2：安装 cloudflared

```bash
wget -q https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb
sudo dpkg -i cloudflared-linux-amd64.deb
cloudflared --version
```

#### 步骤 3：创建隧道

```bash
cloudflared tunnel login
cloudflared tunnel create my-monitor
```

#### 步骤 4：配置 Ingress 规则

创建 `~/.cloudflared/config.yml`：
```yaml
tunnel: <YOUR-TUNNEL-UUID>
credentials-file: /root/.cloudflared/<YOUR-TUNNEL-UUID>.json
ingress:
  - hostname: monitor.yourdomain.com
    service: http://127.0.0.1:38080
  - service: http_status:404
```

#### 步骤 5：启动隧道

```bash
sudo systemctl start cloudflared
sudo systemctl enable cloudflared
sudo journalctl -u cloudflared -f
```

---

## 7. 性能优化

### 7.1 减少数据推送频率

如果觉得更新间隔太短（默认 2 秒），可以配置：
```
http://localhost:8000/\?interval\=5
```

### 7.2 内存优化（Docker）

```yaml
deploy:
  resources:
    limits:
      memory: 256M
```

---

**最后更新**：2025年12月

## 许可证

本项目采用 [知识共享署名-非商业性使用 4.0 国际许可证 (CC BY-NC 4.0)](LICENSE)。

您可以自由分享和改编本作品，但不可用于商业用途，且必须署名。
