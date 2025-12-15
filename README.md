# Web Monitor (Go Version)

一个轻量级、高性能的 Linux 服务器监控与管理面板。采用 Go 语言开发后端，纯 HTML/JS 前端，资源占用极低，部署简单。

## ✨ 功能特性

*   **实时监控**：CPU、内存、磁盘 I/O、网络流量、GPU (NVIDIA/AMD/Intel)、温度传感器。
*   **进程管理**：查看系统 Top 进程，支持按 CPU、内存、IO 排序，查看进程详情。
*   **Docker 管理**：查看容器/镜像列表，支持启动、停止、重启、删除容器，查看容器日志和统计信息。
*   **系统管理**：
    *   **Systemd 服务**：查看服务状态，支持启动、停止、重启、启用、禁用服务。
    *   **Cron 任务**：查看和编辑计划任务。
*   **SSH 监控**：监控 SSH 连接数、活跃会话、登录历史及失败记录。
*   **安全审计**：内置用户角色系统 (Admin/User)，记录关键操作日志。
*   **Prometheus 集成**：暴露 `/metrics` 接口，支持 Prometheus/Grafana 采集。
*   **告警配置**：支持 CPU、内存、磁盘使用率阈值告警，可配置 Webhook。
*   **电源管理**：查看和调整系统电源性能模式（需硬件支持）。
*   **GPU 监控**：支持 NVIDIA、AMD、Intel GPU 的温度、使用率、显存监控。

## 🚀 快速部署 (Docker Compose)

这是推荐的部署方式，已针对功能完整性进行了预配置。

1.  确保已安装 Docker 和 Docker Compose。
2.  在项目根目录下运行：

```bash
docker-compose up -d
```

3.  访问浏览器：`http://<服务器IP>:38080`
4.  **默认账号**：`admin`
5.  **默认密码**：`admin123` **(请登录后立即修改)**

### ⚠️ 关键配置说明

为了使监控和管理功能正常工作，容器需要较高的权限和特定的挂载：

*   `privileged: true`: 必须开启，用于访问硬件传感器和执行特权命令。
*   `network_mode: host`: 推荐开启，以便准确监控宿主机网络流量。
*   `pid: host`: 必须开启，以便获取宿主机进程列表。
*   `volumes`:
    *   `/:/hostfs`: **核心配置**。程序通过 `chroot /hostfs` 来管理宿主机的 Systemd、Cron 和系统信息。
    *   `/var/run/docker.sock`: 用于 Docker 管理功能。
    *   `/proc`, `/sys`: 用于采集硬件信息和 GPU 监控。
    *   GPU 设备（如 `/dev/nvidia*`）: 如需 GPU 监控，需挂载相应设备。

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

Web Monitor 内置多层安全机制，确保系统安全：

### 认证与授权
*   **JWT 令牌认证**：使用标准 JWT (JSON Web Token) 进行会话管理，令牌24小时过期。
*   **角色权限控制**：管理员 (admin) 和普通用户 (user) 两级权限。
*   **密码策略**：密码必须至少8位，包含大小写字母、数字和特殊字符中的三种。
*   **账户锁定**：连续5次登录失败后账户锁定15分钟。
*   **速率限制**：登录接口限流，防止暴力破解。

### 网络安全
*   **安全HTTP头**：自动设置 CSP, X-Frame-Options, X-XSS-Protection 等安全头。
*   **CSP策略**：严格的内容安全策略，防止 XSS 攻击。
*   **输入验证**：所有用户输入均经过严格验证，防止命令注入。
*   **HTTPS就绪**：支持配置 TLS 证书，启用 HTTPS。

### 操作审计
*   **完整操作日志**：记录所有关键操作（登录、用户管理、服务操作等）。
*   **IP地址记录**：记录操作来源 IP。
*   **日志保留**：保留最近1000条操作日志。

## 📊 监控指标

Web Monitor 通过 Prometheus 暴露丰富的系统指标，包括：

*   `system_cpu_usage_percent`: CPU 使用率
*   `system_memory_usage_percent`: 内存使用率
*   `system_memory_total_bytes`: 总内存大小
*   `system_memory_used_bytes`: 已用内存大小
*   `system_disk_usage_percent`: 磁盘使用率（按挂载点）
*   `system_network_sent_bytes_total`: 网络发送总字节数
*   `system_network_recv_bytes_total`: 网络接收总字节数
*   `system_temperature_celsius`: 硬件温度（按传感器）
*   `gpu_usage_percent`: GPU 使用率（按设备）
*   `gpu_memory_used_bytes`: GPU 显存使用量
*   `gpu_temperature_celsius`: GPU 温度

可通过 `/metrics` 端点采集数据，与 Prometheus + Grafana 集成。

## ⚙️ 配置说明

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `PORT` | `8000` | 服务监听端口 |
| `JWT_SECRET` | 自动生成 | JWT 签名密钥，建议在生产环境设置 |
| `WS_ALLOWED_ORIGINS` | 空 | WebSocket `/ws/stats` 的 Origin 允许列表（逗号分隔）；用于 Cloudflare/反代/自定义域名场景 |
| `SSL_CERT_FILE` | 空 | TLS 证书文件路径（启用 HTTPS） |
| `SSL_KEY_FILE` | 空 | TLS 私钥文件路径（启用 HTTPS） |

#### 生产环境推荐：使用本地 `.env`（不提交到 Git）

本项目已在 `.gitignore` 中忽略 `.env`，你可以在服务器上创建一个本地 `.env` 来放置敏感配置。

- 参考模板：`.env.example`
- 示例（把域名替换成你自己的）：

```bash
JWT_SECRET=change-me
WS_ALLOWED_ORIGINS=https://webmonitor.example.com,webmonitor.example.com
```

Docker Compose 会自动读取同目录的 `.env` 用于变量注入（无需把域名/密钥写进仓库）。

### Cloudflare CDN / 反向代理注意事项（WebSocket）

你通过 Cloudflare 以 `https://<domain>/` 访问时，浏览器的 WebSocket 会携带 `Origin`。为了避免 CSWSH 风险，服务端默认只允许同源 WebSocket。

如果你看到 WebSocket 连接失败（控制台 403 / Origin 错误），请：

1. 在 `.env` 中设置 `WS_ALLOWED_ORIGINS`（如上示例）
2. 确保你的反代在转发 WebSocket 时保留 Host/转发 Host（例如设置 `Host`/`X-Forwarded-Host`）
3. Cloudflare 控制台里确保 WebSockets 功能开启（Network/WebSockets）

### 数据持久化

所有持久化数据（用户数据库、日志、告警配置）存储在 `/data` 目录下。在 Docker 部署中，这被映射为 `web-monitor-data` 卷。

## 🐛 故障排除

### 常见问题

1.  **无法查看 Systemd 服务或 Cron 任务**
    *   检查 Docker 是否挂载了 `/:/hostfs` 目录。
    *   确保容器以 `privileged: true` 权限运行。

2.  **Docker 管理页面为空**
    *   检查是否挂载了 `/var/run/docker.sock`。

3.  **GPU 监控显示为不可用**
    *   确保宿主机有 GPU 硬件且驱动已安装。
    *   如需在容器内监控 GPU，需挂载 GPU 设备文件（如 `/dev/nvidia0`）和相应库文件。
    *   检查容器是否有权限访问 GPU 设备。

4.  **温度传感器显示为0**
    *   确保挂载了 `/sys` 目录且容器有特权权限。
    *   某些硬件可能需要额外内核模块。

5.  **忘记管理员密码**
    ```bash
    # 进入容器
    docker exec -it web-monitor-go sh
    
    # 删除用户数据库
    rm /data/users.json
    
    # 重启容器
    docker restart web-monitor-go
    ```
    重启后系统将重新创建默认账户 (admin/admin123)。

### 日志查看

```bash
# 查看容器日志
docker logs web-monitor-go

# 跟踪实时日志
docker logs -f web-monitor-go
```

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request 来改进 Web Monitor。

### 开发环境搭建

1. 克隆仓库
2. 安装 Go 1.21+ 和 Node.js
3. 运行 `go mod download` 下载依赖
4. 启动开发服务器：`go run ./cmd/server/main.go`

### 代码规范

*   Go 代码遵循标准格式（使用 `go fmt`）
*   前端代码使用 ES6+ 标准
*   提交前请运行测试

## 📄 许可证

CC BY-NC 4.0

## 📞 支持

*   [GitHub Issues](https://github.com/AnalyseDeCircuit/web-monitor/issues) - 报告问题或提出功能请求
*   [使用手册](MANUAL.md) - 详细功能说明和配置指南
*   [英文文档](README_EN.md) - English documentation

---

**注意事项**：
1.  Web Monitor 需要较高权限访问系统信息，请仅在受信任的网络环境中部署。
2.  生产环境务必修改默认密码并配置 HTTPS。
3.  定期备份 `/data` 目录中的重要数据。
4.  GPU 监控功能需要相应硬件和驱动支持，部分功能可能受限于容器环境。
