# Web Monitor - User Manual

**Version**: 2.0
**Last Updated**: December 17, 2025
**License**: CC BY-NC 4.0 (Attribution-NonCommercial)

[English Version](#web-monitor-user-manual) | [中文版本](#web-monitor-用户手册)

---

# Web Monitor - User Manual

## Table of Contents

1. [Overview](#11-overview)
2. [Quick Start](#12-quick-start)
3. [Installation](#2-installation)
4. [Features Guide](#3-features-guide)
5. [Security](#4-security)
6. [Troubleshooting](#5-troubleshooting)
7. [Technical Details](#6-technical-details)
8. [Appendix](#7-appendix)

---

## 1. Overview

### 1.1 What is Web Monitor?

Web Monitor is a **lightweight, high-performance Linux server monitoring and management panel** that provides real-time system metrics, container management, service control, and security auditing through a web interface.

### 1.2 System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Frontend                            │
│                  (HTML5/CSS3/Vanilla JS)                    │
├─────────────────────────────────────────────────────────────┤
│                         API Layer                          │
│                    REST API + WebSocket                    │
├─────────────────────────────────────────────────────────────┤
│                      Business Logic                        │
│  Collectors  →  Aggregation  →  Processing  →  Response  │
├─────────────────────────────────────────────────────────────┤
│                      Data Sources                          │
│  /proc  /sys  Docker API  Systemd D-Bus  Log Files       │
└─────────────────────────────────────────────────────────────┘
```

### 1.3 Supported Platforms

- **OS**: Linux (kernel 3.10+)
- **Architecture**: amd64, arm64
- **Deployment**: Docker (recommended), Binary
- **License**: CC BY-NC 4.0 (Attribution-NonCommercial)

---

## 2. Quick Start

### 2.1 Docker Deployment (Recommended)

```bash
# 1. Set JWT secret (required for production)
export JWT_SECRET=$(openssl rand -base64 64)

# 2. Start services
docker compose up -d

# 3. Access Web Monitor
open http://localhost:38080

# 4. Login with default credentials
# Username: admin
# Password: admin123
```

### 2.2 Manual Installation

```bash
# 1. Build from source
cd cmd/server
go build -o web-monitor main.go

# 2. Set JWT secret
export JWT_SECRET=$(openssl rand -base64 64)

# 3. Run with root privileges
sudo ./web-monitor
```

---

## 3. Installation

### 3.1 Prerequisites

**System Requirements:**
- CPU: 1 core minimum
- RAM: 100 MB available
- Disk: 100 MB free space
- Network: TCP port access (default: 38080)

**Software:**
- Docker 20.10+ (for Docker deployment)
- Go 1.23+ (for manual build)

### 3.2 Docker Installation

#### Standard Deployment

```bash
# Clone repository
git clone <repository-url>
cd web-monitor

# Configure environment
cp .env.example .env
# Edit .env with your JWT_SECRET

# Deploy
docker compose up -d
```

#### Security-Enhanced Deployment

```yaml
# docker-compose.yml - Production Configuration
services:
  web-monitor-go:
    environment:
      - JWT_SECRET=${JWT_SECRET}
      - DOCKER_READ_ONLY=true  # Enable read-only mode
      - WS_ALLOWED_ORIGINS=https://your-domain.com
    cap_drop:
      - ALL  # Drop all capabilities
    cap_add:
      - SYS_PTRACE
      - DAC_READ_SEARCH
      - SYS_CHROOT
```

### 3.3 Binary Installation

```bash
# Build static binary
go build -ldflags="-s -w -extldflags '-static'" -o web-monitor ./cmd/server

# Run with systemd
sudo tee /etc/systemd/system/web-monitor.service > /dev/null << EOF
[Unit]
Description=Web Monitor
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/web-monitor
Restart=always
Environment="JWT_SECRET=your-secret-here"
Environment="PORT=38080"

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now web-monitor
```

---

## 4. Features Guide

### 4.1 Dashboard

The dashboard displays **real-time system metrics** with 2-60 second refresh intervals.

**Key Metrics:**
- **CPU**: Usage per core, frequency, temperature history
- **Memory**: RAM and Swap usage with history graphs
- **Disk**: Space usage per partition, I/O statistics
- **Network**: Real-time bandwidth, connections, listening ports
- **GPU**: Temperature, utilization, memory (NVIDIA/AMD/Intel)

### 4.2 Process Management

**Viewing Processes:**
1. Navigate to **Processes** tab
2. Sort by CPU, Memory, or I/O usage
3. I/O statistics load on-demand (reduce overhead)

**Killing Processes (Admin only):**
1. Select process and click **Kill**
2. Confirmation dialog appears
3. Process is terminated with SIGTERM → SIGKILL

**Security Features:**
- Cannot kill system processes (PID < 1000)
- Cannot kill init/systemd (PID 1)
- Permission checks prevent unauthorized termination

### 4.3 Docker Management

**Container Operations:**
- **View**: List all containers with status, resources
- **Control**: Start, stop, restart, remove containers
- **Logs**: Real-time container logs
- **Stats**: CPU, memory, network usage per container

**Image Management:**
- List all local images
- Remove unused images
- View image details (size, layers, creation)

**Security Notes:**
- Docker socket accessed through local proxy
- Read-only mode available for safer deployments
- All operations logged for audit trail

### 4.4 System Services (systemd)

**Service Control:**
1. **List Services**: All systemd units with status
2. **Start/Stop**: Control service states
3. **Enable/Disable**: Manage auto-start on boot
4. **View Logs**: Journal logs for troubleshooting

### 4.5 Scheduled Tasks (Cron)

**Managing Cron Jobs:**
1. View all cron jobs for root user
2. Add new scheduled tasks
3. Edit existing jobs
4. Enable/disable individual jobs

**Interface:**
- Visual cron expression builder
- Next execution preview
- Easy enable/disable toggle

### 4.6 Network Diagnostics

**Built-in Tools:**
- **Ping**: Test connectivity with packet loss stats
- **Traceroute**: 15-hop limit for path discovery
- **DNS Lookup**: Dig interface with timeout controls
- **HTTP Test**: Curl with size/time limits

**Security:** All commands sanitized to prevent injection

### 4.7 SSH Monitoring

**Session Tracking:**
- Active SSH sessions with user/IP
- Login history with authentication method
- Failed attempt detection
- Brute force pattern recognition

**Host Key Display:**
- SSH host key fingerprints
- Algorithm verification

### 4.8 Alert Configuration

**Threshold Alerts:**
- CPU usage percentage
- Memory usage percentage
- Disk usage percentage

**Webhook Notifications:**
- Configurable webhook URL
- Rate limiting (5 min per alert type)
- JSON payload format compatible with Discord/Slack

### 4.9 Power Management

**Performance Profiles:**
- View current power mode
- Switch between Performance/Balanced/Power Save
- Hardware compatibility detection
- RAPL/ACPI interface support

---

## 5. Security

### 5.1 Authentication

**JWT Implementation:**
- 24-hour token expiration
- HttpOnly cookie storage
- Token revocation on logout
- Automatic rotation

**Password Policy:**
- Minimum 8 characters
- Requires 3 of 4 character types
- Account lockout after 5 failed attempts
- 15-minute lockout duration

### 5.2 Role-Based Access Control

**Admin Role:**
- Full system access
- User management
- Service/container control
- Alert configuration
- Audit log access

**User Role:**
- Read-only monitoring
- Personal password change
- Cannot perform management actions

### 5.3 Security Best Practices

**Production Checklist:**
- [ ] Set strong JWT_SECRET (min 32 bytes, 64+ recommended)
- [ ] Change default admin password
- [ ] Enable HTTPS with valid certificate
- [ ] Configure firewall rules
- [ ] Enable Docker read-only mode if write operations not needed
- [ ] Restrict Docker proxy to localhost
- [ ] Regular security updates

**Network Security:**
- WebSocket origin validation
- Proxy IP detection support
- Built-in CSP and security headers
- Rate limiting on all endpoints

### 5.4 Audit Logging

**Logged Operations:**
- User login/logout
- Password changes
- Container/service actions
- User management
- Alert configuration changes

**Log Format:**
```json
{
  "timestamp": "2025-01-01T12:00:00Z",
  "username": "admin",
  "action": "stop_container",
  "details": "Stopped container nginx (id: abc123)",
  "ip": "192.168.1.100"
}
```

**Retention:** 1000 entries maximum, stored in `/data/operations.json`

---

## 6. Troubleshooting

### 6.1 Common Issues

**Issue: Dashboard shows no data**
- Solution: Check WebSocket connection in browser dev tools
- Verify no firewall blocking WebSocket port

**Issue: Cannot see systemd services**
- Solution: Verify volume mount `-v /:/hostfs`
- Check dbus socket mount `-v /run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro`

**Issue: Docker management shows nothing**
- Solution: Check `docker-socket-proxy` container is running
- Verify `DOCKER_HOST=tcp://127.0.0.1:2375` environment variable

**Issue: GPU monitoring not working**
- Solution: Ensure GPU drivers installed on host
- Mount GPU devices (e.g., `/dev/nvidia*`) in docker-compose.yml

### 6.2 Performance Issues

**High CPU Usage:**
1. Reduce WebSocket client interval from UI
2. Check for excessive process count
3. Monitor Docker API response times

**Memory Growth:**
1. Check process cache cleanup logs
2. Monitor WebSocket client connections
3. Restart container if memory leak suspected

**Network Slowness:**
1. Ensure `network_mode: host` for accurate metrics
2. Check for network-intensive containers

### 6.3 Debug Mode

Enable debug logging:
```bash
docker exec -it web-monitor-go sh
echo 'VERBOSE=1' >> /app/config/debug.conf
```

View runtime metrics:
```bash
# For binary installation
curl http://localhost:38080/api/metrics

# For Docker
docker exec web-monitor-go curl http://localhost:38080/api/metrics
```

---

## 7. Technical Details

### 7.1 Architecture

```
Client → HTTPS → Reverse Proxy → Web Monitor
                ↓
         ┌──────┴──────┐
         ↓             ↓
      REST API    WebSocket
         ↓             ↓
         └───┬─────────┘
             ↓
      Business Logic
             ↓
      ┌──────┴──────┐
      ↓      ↓      ↓
 Collectors  Auth   Logger
      ↓      ↓      ↓
   System   DB    Files
```

### 7.2 Data Collection

**CPU Stats:**
- Source: `/proc/stat`, `/proc/cpuinfo`
- Metrics: Usage per core, frequency, load average
- Interval: Configurable (2-60s)

**Process Stats:**
- Source: `/proc/[pid]/status`, `stat`, `cmdline`
- Caching: 15-second cache to reduce overhead
- Sorting: By memory usage (configurable)

**Network Stats:**
- Source: `/proc/net/dev`, `/proc/net/tcp`, `/proc/net/udp`
- Features: Direct parsing for better performance
- Connection tracking: IPv4/IPv6 support

### 7.3 API Endpoints

**Authentication:**
```
POST /api/login          # Login
POST /api/logout         # Logout
POST /api/password       # Change password
GET  /api/validate-password  # Check password strength
```

**Monitoring:**
```
GET  /api/system/info    # System metrics
GET  /api/process/io     # Process I/O (lazy loaded)
POST /api/process/kill   # Kill process (Admin only)
GET  /api/docker/containers  # Container list
POST /api/docker/action  # Container control
GET  /api/systemd/services # Service list
POST /api/systemd/action # Service control
GET  /api/ssh/stats      # SSH statistics
```

**Configuration:**
```
GET /api/alerts          # Get alert config
PUT /api/alerts          # Update alert config
GET /api/power/profile   # Power management
```

### 7.4 Performance Tuning

**For High Load Systems:**
```yaml
# Increase collection timeout
environment:
  - COLLECTION_TIMEOUT=15s  # Default: 8s

# Reduce collection frequency
environment:
  - MIN_COLLECTION_INTERVAL=5s  # Default: 2s
  - MAX_COLLECTION_INTERVAL=30s  # Default: 60s
```

**Resource Limits:**
```yaml
deploy:
  resources:
    limits:
      memory: 200M
      cpus: '0.5'
```

---

## 8. Appendix

### 8.1 Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 38080 | Server port |
| `JWT_SECRET` | - | **Required** - JWT signing key |
| `DATA_DIR` | /data | User data directory |
| `DOCKER_READ_ONLY` | false | Disable Docker write operations |
| `WS_ALLOWED_ORIGINS` | - | Restrict WebSocket origins |
| `COLLECTION_TIMEOUT` | 8s | Timeout for collectors |
| `LOG_LEVEL` | info | Set to `debug` for verbose logging |

### 8.2 File Structure

```
/data/
├── users.json          # User database
├── operations.json     # Audit log
├── alerts.json         # Alert configuration
└── ssl/                # SSL certificates (optional)
```

### 8.3 Backup and Restore

**Backup:**
```bash
# With Docker
docker exec web-monitor-go tar -czf /tmp/backup.tar.gz /data
docker cp web-monitor-go:/tmp/backup.tar.gz ./web-monitor-backup- $(date +%Y%m%d).tar.gz

# With binary
tar -czf web-monitor-backup-$(date +%Y%m%d).tar.gz /data
```

**Restore:**
```bash
# Stop service first
tar -xzf web-monitor-backup-20250101.tar.gz -C /
docker compose up -d
```

### 8.4 Monitoring Statistics

**Collection Performance:**
- Average: 50-200ms for full collection
- CPU: ~5-15ms per core collection
- Memory: ~8-20ms including sorting
- Network: ~10-30ms (direct /proc parsing)
- Processes: ~50-150ms for 1000+ processes

**WebSocket Performance:**
- Connection overhead: <50ms
- Message latency: <100ms average
- Concurrent clients: 100+ tested

---

**Need help?** See [Troubleshooting](#6-troubleshooting) or [GitHub Issues](https://github.com/AnalyseDeCircuit/web-monitor/issues)

---

## License

This project is licensed under **CC BY-NC 4.0 (Attribution-NonCommercial 4.0 International)**.

- ✅ Allowed: Copy, distribute, modify, create derivative works
- ✅ Required: Attribution, indicate changes
- ❌ Prohibited: Commercial use

See [LICENSE](./LICENSE) for details.

---

# Web Monitor - 用户手册

**版本**: 2.0
**最后更新**: 2025年12月17日

---

## 目录

1. [概述](#11-概述)
2. [快速开始](#12-快速开始)
3. [安装部署](#2-安装部署)
4. [功能指南](#3-功能指南)
5. [安全设置](#4-安全设置)
6. [故障排除](#5-故障排除)
7. [技术细节](#6-技术细节)
8. [附录](#7-附录)

---

## 1. 概述

### 1.1 什么是 Web Monitor？

Web Monitor 是一个**轻量级、高性能的 Linux 服务器监控与管理面板**，通过 Web 界面提供实时系统指标、容器管理、服务控制和安全审计功能。

### 1.2 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                         前端层                             │
│                  (HTML5/CSS3/原生 JS)                       │
├─────────────────────────────────────────────────────────────┤
│                         API 层                             │
│                   REST API + WebSocket                     │
├─────────────────────────────────────────────────────────────┤
│                       业务逻辑层                           │
│  采集器 → 聚合器 → 处理器 → 响应器                        │
├─────────────────────────────────────────────────────────────┤
│                       数据源层                             │
│  /proc /sys Docker API Systemd D-Bus 日志文件              │
└─────────────────────────────────────────────────────────────┘
```

### 1.3 支持平台

- **操作系统**: Linux (内核 3.10+)
- **架构**: amd64, arm64
- **部署方式**: Docker (推荐)、二进制文件

---

## 2. 快速开始

### 2.1 Docker 部署（推荐）

```bash
# 1. 设置 JWT 密钥（生产环境必需）
export JWT_SECRET=$(openssl rand -base64 64)

# 2. 启动服务
docker compose up -d

# 3. 访问 Web Monitor
open http://localhost:38080

# 4. 使用默认凭据登录
# 用户名: admin
# 密码: admin123
```

### 2.2 手动安装

```bash
# 1. 从源码构建
cd cmd/server
go build -o web-monitor main.go

# 2. 设置 JWT 密钥
export JWT_SECRET=$(openssl rand -base64 64)

# 3. 使用 root 权限运行
sudo ./web-monitor
```

---

## 3. 功能指南

### 3.1 仪表盘

仪表盘显示**实时系统指标**，刷新间隔 2-60 秒可配置。

**关键指标:**
- **CPU**: 单核使用率、频率、温度历史
- **内存**: RAM 和 Swap 使用情况及历史图表
- **磁盘**: 各分区空间使用率和 I/O 统计
- **网络**: 实时带宽、连接数、监听端口
- **GPU**: 温度、使用率、显存（NVIDIA/AMD/Intel）

### 3.2 进程管理

**查看进程:**
1. 导航到 **进程** 标签页
2. 按 CPU、内存或 I/O 使用率排序
3. I/O 统计按需加载（降低开销）

**终止进程（仅管理员）:**
1. 选择进程并点击 **终止**
2. 出现确认对话框
3. 使用 SIGTERM → SIGKILL 终止进程

**安全特性:**
- 无法终止系统进程（PID < 1000）
- 无法终止 init/systemd（PID 1）
- 权限检查防止未授权终止

### 3.3 Docker 管理

**容器操作:**
- **查看**: 列出所有容器，显示状态、资源使用情况
- **控制**: 启动、停止、重启、删除容器
- **日志**: 实时容器日志
- **统计**: 每个容器的 CPU、内存、网络使用情况

**镜像管理:**
- 列出所有本地镜像
- 删除未使用的镜像
- 查看镜像详情（大小、层、创建时间）

**安全说明:**
- Docker socket 通过本地代理访问
- 更安全的部署支持只读模式
- 所有操作记录用于审计追踪

### 3.4 系统服务 (systemd)

**服务控制:**
1. **列出服务**: 显示所有 systemd 单元及其状态
2. **启动/停止**: 控制服务状态
3. **启用/禁用**: 管理开机自启
4. **查看日志**: Journal 日志用于故障排查

### 3.5 计划任务 (Cron)

**管理 Cron 任务:**
1. 查看 root 用户的所有 cron 作业
2. 添加新的计划任务
3. 编辑现有作业
4. 启用/禁用单个作业

**界面:**
- 可视化 cron 表达式生成器
- 下次执行预览
- 简单的启用/禁用开关

### 3.6 网络诊断

**内置工具:**
- **Ping**: 测试连通性并显示丢包统计
- **Traceroute**: 15 跳限制的路径发现
- **DNS 查询**: Dig 接口带超时控制
- **HTTP 测试**: Curl 带大小/时间限制

**安全:** 所有命令经过清理以防止注入攻击

### 3.7 SSH 监控

**会话跟踪:**
- 带用户/IP 的活跃 SSH 会话
- 带认证方式的登录历史
- 失败尝试检测
- 暴力破解模式识别

**主机密钥显示:**
- SSH 主机密钥指纹
- 算法验证

### 3.8 告警配置

**阈值告警:**
- CPU 使用率百分比
- 内存使用率百分比
- 磁盘使用率百分比

**Webhook 通知:**
- 可配置的 webhook URL
- 速率限制（每类告警 5 分钟一次）
- 与 Discord/Slack 兼容的 JSON 负载格式

### 3.9 电源管理

**性能配置文件:**
- 查看当前电源模式
- 在性能/平衡/省电之间切换
- 硬件兼容性检测
- 支持 RAPL/ACPI 接口

---

## 4. 安全设置

### 4.1 认证

**JWT 实现:**
- 24 小时令牌有效期
- HttpOnly cookie 存储
- 登出时令牌撤销
- 自动轮换

**密码策略:**
- 最少 8 个字符
- 需要 4 种字符类型中的 3 种
- 5 次失败尝试后账号锁定
- 15 分钟锁定时长

### 4.2 基于角色的访问控制

**管理员角色:**
- 完整的系统访问权限
- 用户管理
- 服务/容器控制
- 告警配置
- 审计日志访问

**普通用户角色:**
- 只读监控
- 修改个人密码
- 无法执行管理操作

### 4.3 安全最佳实践

**生产环境清单:**
- [ ] 设置强 JWT_SECRET（最少 32 字节，推荐 64+
- [ ] 修改默认管理员密码
- [ ] 启用 HTTPS 和有效证书
- [ ] 配置防火墙规则
- [ ] 启用 Docker 只读模式（如果不需要写操作）
- [ ] 将 Docker 代理限制为本地访问
- [ ] 定期安全更新

### 4.4 审计日志

**记录的操作:**
- 用户登录/登出
- 密码修改
- 容器/服务操作
- 用户管理
- 告警配置更改

**日志格式:**
```json
{
  "timestamp": "2025-01-01T12:00:00Z",
  "username": "admin",
  "action": "stop_container",
  "details": "停止容器 nginx (id: abc123)",
  "ip": "192.168.1.100"
}
```

**保留:** 最多 1000 条条目，存储在 `/data/operations.json`

---

## 5. 故障排除

### 5.1 常见问题

**问题: 仪表盘不显示数据**
- 解决方案: 在浏览器开发工具中检查 WebSocket 连接
- 验证防火墙没有阻止 WebSocket 端口

**问题: 无法查看 systemd 服务**
- 解决方案: 验证卷挂载 `-v /:/hostfs`
- 检查 dbus socket 挂载 `-v /run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro`

**问题: Docker 管理页面为空**
- 解决方案: 检查 `docker-socket-proxy` 容器是否运行
- 验证 `DOCKER_HOST=tcp://127.0.0.1:2375` 环境变量

### 5.2 性能问题

**CPU 使用率高:**
1. 从 UI 减少 WebSocket 客户端间隔
2. 检查进程数量是否过多
3. 监控 Docker API 响应时间

**内存增长:**
1. 检查进程缓存清理日志
2. 监控 WebSocket 客户端连接
3. 如果怀疑内存泄漏，重启容器

### 5.3 调试模式

启用调试日志:
```bash
docker exec -it web-monitor-go sh
echo 'VERBOSE=1' >> /app/config/debug.conf
```

查看运行时指标:
```bash# 对于二进制安装
curl http://localhost:38080/api/metrics

# 对于 Docker
docker exec web-monitor-go curl http://localhost:38080/api/metrics
```

---

## 6. 技术细节

### 6.1 API 端点

**认证:**
```
POST /api/login          # 登录
POST /api/logout         # 登出
POST /api/password       # 修改密码
GET  /api/validate-password  # 检查密码强度
```

**监控:**
```
GET  /api/system/info    # 系统指标
GET  /api/process/io     # 进程 I/O（懒加载）
POST /api/process/kill   # 终止进程（仅管理员）
GET  /api/docker/containers  # 容器列表
POST /api/docker/action  # 容器控制
GET  /api/systemd/services # 服务列表
POST /api/systemd/action # 服务控制
GET  /api/ssh/stats      # SSH 统计
```

### 6.2 性能调优

**高负载系统:**
```yaml
# 增加采集超时
environment:
  - COLLECTION_TIMEOUT=15s  # 默认: 8s

# 减少采集频率
environment:
  - MIN_COLLECTION_INTERVAL=5s  # 默认: 2s
  - MAX_COLLECTION_INTERVAL=30s  # 默认: 60s
```

---

## 7. 附录

### 7.1 环境变量

| 变量 | 默认值 | 说明 |
|----------|---------|-------------|
| `PORT` | 38080 | 服务端口 |
| `JWT_SECRET` | - | **必需** - JWT 签名密钥 |
| `DATA_DIR` | /data | 用户数据目录 |
| `DOCKER_READ_ONLY` | false | 禁用 Docker 写操作 |
| `WS_ALLOWED_ORIGINS` | - | 限制 WebSocket 源 |

### 7.2 备份与恢复

**备份:**
```bash
# Docker 部署
docker exec web-monitor-go tar -czf /tmp/backup.tar.gz /data
docker cp web-monitor-go:/tmp/backup.tar.gz ./web-monitor-backup-$(date +%Y%m%d).tar.gz

# 二进制部署
tar -czf web-monitor-backup-$(date +%Y%m%d).tar.gz /data
```

**恢复:**
```bash
# 先停止服务
tar -xzf web-monitor-backup-20250101.tar.gz -C /
docker compose up -d
```

### 7.3 监控统计

**采集性能:**
- 平均值: 50-200ms 完整采集
- CPU: ~5-15ms 每核采集
- 内存: ~8-20ms 包含排序
- 网络: ~10-30ms（直接解析 /proc）
- 进程: ~50-150ms 处理 1000+ 进程

**WebSocket 性能:**
- 连接开销: <50ms
- 消息延迟: <100ms 平均
- 并发客户端: 100+ 已测试

---

**需要帮助？** 查看 [故障排除](#5-故障排除) 或 [GitHub Issues](https://github.com/AnalyseDeCircuit/web-monitor/issues)

---

## 许可证

本项目采用 **CC BY-NC 4.0（署名-非商业性使用 4.0 国际）** 许可证。

- ✅ 允许：复制、分发、修改、创作衍生作品
- ✅ 必须：署名、注明修改
- ❌ 禁止：商业用途

详见 [LICENSE](./LICENSE) 文件。
