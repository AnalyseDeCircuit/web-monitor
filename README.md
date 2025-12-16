# Web Monitor - Lightweight Linux Server Management Panel

Web Monitor is a **high-performance, lightweight Linux server monitoring and management panel** built with Go and vanilla JavaScript. Designed for resource-constrained environments and secure deployments.

[English Version](#web-monitor) | [中文版本](#web-monitor-中文版)

---

## 🌟 Key Features

### Real-Time Monitoring
- **System Metrics**: CPU (per-core), memory, disk I/O, network traffic, temperature sensors
- **GPU Monitoring**: NVIDIA, AMD, and Intel GPU support (temperature, utilization, memory)
- **Process Management**: Top processes by CPU/memory/IO with on-demand I/O statistics
- **SSH Monitoring**: Active sessions, authentication logs, brute-force detection

### Container & Service Management
- **Docker**: Container/image management with safe Docker socket proxy
- **Systemd**: Full service control (start, stop, restart, enable, disable)
- **Cron Jobs**: View and manage scheduled tasks

### Security & Access Control
- **Role-Based Access**: Admin and User roles with granular permissions
- **Audit Logging**: All operations logged with IP and timestamp
- **JWT Authentication**: HttpOnly cookies, secure token management
- **SSH Security**: Monitor failed attempts, detect suspicious activity

### Observability
- **Prometheus Integration**: Built-in `/metrics` endpoint
- **Alerting**: Configurable thresholds for CPU, memory, disk
- **WebSocket**: Real-time data push with dynamic subscription

---

## 🏗️ Architecture Highlights

### Backend (Go 1.23)
- **No Web Framework**: Pure `net/http` for minimal overhead
- **Vendor Mode**: All dependencies bundled for offline deployment
- **Parallel Collection**: 9 collectors run concurrently with 8s timeout
- **Lazy Loading**: I/O stats fetched only when needed

### Frontend (Vanilla JS)
- **Zero Dependencies**: Chart.js, Font Awesome vendored locally
- **PWA Support**: Offline-capable with service worker
- **Efficient Updates**: WebSocket with selective data subscription

### Deployment Options
- **Docker-First**: Optimized multi-stage build
- **HostFS Support**: Containerized monitoring of host system
- **Socket Proxy**: Secure Docker access without mounting docker.sock

---

## ⚡ Performance Characteristics

### Resource Usage (Typical)
- **Memory**: 20-50 MB RSS (container)
- **CPU**: <1% on idle, spikes during collection
- **Collection Interval**: 2-60s (configurable per client)

### Optimizations
- **Linux Native**: Direct `/proc` parsing instead of library calls
- **Caching**: Process static info cached across collections
- **Object Pool**: Minimized allocations in hot paths
- **Immutable Assets**: Fingerprinted with 1-year cache headers

---

## 🚀 Quick Start (Docker Compose)

**Recommended for production deployments**

```bash
# 1. Clone repository
git clone <repository-url>
cd web-monitor

# 2. Configure environment
cp .env.example .env
# Edit .env and set JWT_SECRET (min 32 bytes)

# 3. Deploy
docker compose up -d

# 4. Access
# http://<server-ip>:38080
# Default: admin / admin123 (change immediately!)
```

### Docker Security Configuration

The container requires specific privileges for system monitoring:

```yaml
cap_add:
  - SYS_PTRACE      # Read /proc for process info
  - DAC_READ_SEARCH # Read logs and restricted files
  - SYS_CHROOT      # Cron management

security_opt:
  - apparmor=unconfined

network_mode: host  # Accurate network monitoring
pid: host          # Access host process tree

volumes:
  - /:/hostfs       # Core: host filesystem access
  - /run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro  # Systemd
  - /proc:/proc     # Hardware/process info
  - /sys:/sys       # GPU/temperature data
```

### Docker Socket Security

**Default (Recommended)**: Uses proxy container
- `web-monitor-go` accesses Docker via `tcp://127.0.0.1:2375`
- Only `docker-socket-proxy` mounts docker.sock
- Limited API surface exposed

**Alternative**: Direct socket mount (not recommended)
```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

---

## 🔧 Manual Build

```bash
# 1. Build static binary
cd cmd/server
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o web-monitor main.go

# 2. Run
./web-monitor

# Optional: compress with upx
upx --lzma --best web-monitor
```

**Binary Size**: ~15 MB (uncompressed), ~5 MB (with upx)

---

## 🔒 Security Features

### Authentication & Authorization
- **HttpOnly Cookies**: JWT token not accessible to JavaScript
- **BCrypt Passwords**: Password hashing with automatic salt
- **Rate Limiting**: Login attempts throttled per IP/username
- **Account Lockout**: Auto-lock after 5 failed attempts (15 min)
- **JWT Management**: Token revocation on logout

### Production Security Checklist
- [ ] Set strong `JWT_SECRET` (≥64 bytes recommended)
- [ ] Change default admin password
- [ ] Configure firewall to limit access
- [ ] Enable HTTPS (reverse proxy)
- [ ] Restrict Docker proxy to localhost
- [ ] Review capability requirements

### Network Security
- **Origin Validation**: WebSocket origins configurable via `WS_ALLOWED_ORIGINS`
- **Proxy Support**: Correctly identifies client IP behind Cloudflare/proxies
- **Security Headers**: CSP, HSTS, X-Frame-Options configured

---

## 📊 Metrics & Monitoring

### Prometheus Integration
```yaml
# Add to prometheus.yml
scrape_configs:
  - job_name: 'web-monitor'
    static_configs:
      - targets: ['your-server:38080']
```

### Available Metrics
- Go runtime: Memory, GC, goroutines
- System: CPU, memory, disk, network (via API)
- Custom: Request counts, error rates

---

## ⚙️ Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `JWT_SECRET` | **Yes** | - | Min 32 bytes, 64+ recommended |
| `PORT` | No | 8000 | HTTP server port |
| `DATA_DIR` | No | ./data | User database location |
| `WS_ALLOWED_ORIGINS` | No | - | Comma-separated origins |
| `DOCKER_HOST` | No | - | Docker API endpoint |

### Development Mode
```bash
ENV=development  # Allows auto-generated JWT keys (not for production!)
```

---

## 📈 Benchmarks

### Collection Performance
- **Full collection**: ~50-200ms (9 parallel collectors)
- **Process list**: <100ms for 1000+ processes
- **Network details**: ~10-30ms (direct /proc parsing)

### WebSocket Throughput
- Concurrent clients: 100+ tested
- Message rate: Up to 10 Hz per client
- Typical bandwidth: 10-50 KB/s per client

---

## 🐛 Troubleshooting

### High CPU Usage
1. Check collection interval (WebSocket clients)
2. Review process count (affects collection time)
3. Monitor Docker API response time

### Memory Growing
1. Check for WebSocket client leaks
2. Monitor process cache size
3. Review log for cleanup messages

### Docker Not Working
1. Verify docker-socket-proxy is running
2. Check DOCKER_HOST environment variable
3. Test with `docker exec web-monitor-go curl http://docker-proxy:2375/version`

---

## 📝 License

CC BY-NC 4.0 - Non-commercial use only

---

## 🤝 Contributing

Contributions welcome! Please ensure:
1. Code follows Go best practices
2. Security implications considered
3. Performance impact measured
4. Documentation updated

---

# Web Monitor - 轻量级 Linux 服务器监控面板

Web Monitor 是一个**高性能、轻量级**的 Linux 服务器监控与管理面板，采用 Go 语言开发后端，原生 JavaScript 前端，专为资源受限环境和安全部署而设计。

---

## 🌟 核心功能

### 实时监控
- **系统指标**: CPU（单核）、内存、磁盘 I/O、网络流量、温度传感器
- **GPU 监控**: 支持 NVIDIA、AMD、Intel GPU（温度、使用率、显存）
- **进程管理**: Top 进程查看，支持按 CPU/内存/IO 排序
- **SSH 监控**: 活跃会话、认证日志、暴力破解检测

### 容器与服务管理
- **Docker**: 容器/镜像管理，安全 Docker Socket 代理
- **Systemd**: 完整的系统服务控制（启动、停止、重启、启用、禁用）
- **Cron 任务**: 查看和管理计划任务

### 安全与访问控制
- **基于角色的访问**: Admin 和 User 角色，细粒度权限控制
- **审计日志**: 所有操作记录 IP 和时间戳
- **JWT 认证**: HttpOnly Cookie，安全的令牌管理

### 可观测性
- **Prometheus 集成**: 内置 `/metrics` 端点
- **告警配置**: CPU、内存、磁盘使用率阈值告警
- **WebSocket**: 实时数据推送，动态订阅

---

## 🏗️ 架构亮点

### 后端 (Go 1.23)
- **无 Web 框架**: 纯 `net/http` 实现，最小化开销
- **Vendor 模式**: 所有依赖打包，支持离线部署
- **并行采集**: 9 个采集器并发运行，8 秒超时控制
- **懒加载**: I/O 统计仅在需要时获取

### 前端 (原生 JS)
- **零依赖**: Chart.js、Font Awesome 本地化处理
- **PWA 支持**: 支持离线访问
- **高效更新**: WebSocket 选择性数据订阅

### 部署选项
- **Docker 优先**: 优化的多阶段构建
- **HostFS 支持**: 容器内监控宿主机系统
- **Socket 代理**: 无需挂载 docker.sock 的安全 Docker 访问

---

## ⚡ 性能特性

### 资源占用（典型值）
- **内存**: 20-50 MB RSS（容器环境）
- **CPU**: <1% 空闲时，采集期间峰值
- **采集间隔**: 2-60 秒（每个客户端可配置）

### 优化措施
- **Linux 原生**: 直接解析 `/proc` 而非库调用
- **缓存机制**: 进程静态信息跨采集周期缓存
- **对象复用**: 减少 GC 压力和系统调用
- **不可变资源**: 资源文件指纹化，1 年缓存

---

## 🚀 快速部署（Docker Compose）

**生产环境推荐方式**

```bash
# 1. 克隆仓库
git clone <repository-url>
cd web-monitor

# 2. 配置环境
cp .env.example .env
# 编辑 .env，设置 JWT_SECRET（至少 32 字节）

# 3. 部署
docker compose up -d

# 4. 访问
# http://<服务器IP>:38080
# 默认账号: admin / admin123（立即修改！）
```

### Docker 安全配置

容器需要特定权限进行系统监控：

```yaml
cap_add:
  - SYS_PTRACE      # 读取 /proc 进程信息
  - DAC_READ_SEARCH # 读取日志和受限文件
  - SYS_CHROOT      # Cron 管理

security_opt:
  - apparmor=unconfined

network_mode: host  # 准确监控网络
pid: host          # 访问宿主机进程树

volumes:
  - /:/hostfs       # 核心：宿主机文件系统访问
  - /run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro  # Systemd
  - /proc:/proc     # 硬件/进程信息
  - /sys:/sys       # GPU/温度数据
```

### Docker Socket 安全

**默认（推荐）**: 使用代理容器
- `web-monitor-go` 通过 `tcp://127.0.0.1:2375` 访问 Docker
- 仅 `docker-socket-proxy` 挂载 docker.sock
- 暴露有限的 API 接口

---

## 🔧 手动编译

```bash
# 1. 编译静态二进制文件
cd cmd/server
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o web-monitor main.go

# 2. 运行
./web-monitor

# 可选：使用 upx 压缩
upx --lzma --best web-monitor
```

**二进制大小**: ~15 MB（未压缩），~5 MB（upx 压缩后）

---

## 🔒 安全特性

### 认证与授权
- **HttpOnly Cookie**: JWT 令牌不可被 JavaScript 访问
- **BCrypt 密码**: 密码哈希加自动盐值
- **速率限制**: 按 IP/用户名限制登录尝试
- **账号锁定**: 5 次失败尝试后锁定 15 分钟
- **JWT 管理**: 登出时令牌撤销

### 生产环境安全清单
- [ ] 设置强 `JWT_SECRET`（推荐 ≥64 字节）
- [ ] 修改默认管理员密码
- [ ] 配置防火墙限制访问
- [ ] 启用 HTTPS（反向代理）
- [ ] 限制 Docker 代理为本地访问
- [ ] 审查能力集需求

---

## 📊 指标与监控

### Prometheus 集成
```yaml
# 添加到 prometheus.yml
scrape_configs:
  - job_name: 'web-monitor'
    static_configs:
      - targets: ['your-server:38080']
```

### 可用指标
- Go 运行时: 内存、GC、goroutine
- 系统: CPU、内存、磁盘、网络（通过 API）
- 自定义: 请求计数、错误率

---

## ⚙️ 配置

### 环境变量

| 变量 | 必需 | 默认值 | 说明 |
|----------|----------|---------|-------------|
| `JWT_SECRET` | **是** | - | 至少 32 字节，推荐 64+ 字节 |
| `PORT` | 否 | 8000 | HTTP 服务端口 |
| `DATA_DIR` | 否 | ./data | 用户数据目录 |
| `WS_ALLOWED_ORIGINS` | 否 | - | 逗号分隔的源列表 |

### 开发模式
```bash
ENV=development  # 允许自动生成 JWT 密钥（仅限开发！）
```

---

## 📈 基准测试

### 采集性能
- **完整采集**: ~50-200ms（9 个并行采集器）
- **进程列表**: <100ms（1000+ 进程）
- **网络详情**: ~10-30ms（直接解析 /proc）

### WebSocket 吞吐量
- 并发客户端: 100+ 已测试
- 消息频率: 每个客户端最高 10 Hz
- 典型带宽: 每个客户端 10-50 KB/s

---

## 🐛 故障排查

### CPU 使用率高
1. 检查采集间隔（WebSocket 客户端）
2. 查看进程数量（影响采集时间）
3. 监控 Docker API 响应时间

### 内存增长
1. 检查 WebSocket 客户端泄漏
2. 监控进程缓存大小
3. 查看清理日志

### Docker 无法工作
1. 确认 docker-socket-proxy 运行正常
2. 检查 DOCKER_HOST 环境变量
3. 测试: `docker exec web-monitor-go curl http://docker-proxy:2375/version`

---

## 📝 许可证

CC BY-NC 4.0 - 仅限非商业用途

---

## 🤝 贡献

欢迎贡献！请确保：
1. 代码遵循 Go 最佳实践
2. 考虑安全影响
3. 评估性能影响
4. 更新文档
