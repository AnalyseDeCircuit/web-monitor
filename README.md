# Web Monitor (Go)

一个轻量级、高性能的 Linux 系统监控服务，内置现代化 Web 页面与 WebSocket 实时推送。专为小主机、NAS 及容器环境设计。

## 核心功能
- **全方位监控**：CPU（频率/负载/功耗）、内存、磁盘 I/O、网络流量/连接数、进程列表。
- **GPU 支持**：支持 Intel 核显（Alder Lake-N 等）的频率、负载及显存监控。
- **SSH 审计**：实时监控 SSH 连接数、活跃会话、登录失败次数及 Known Hosts 变动。
- **低资源占用**：Go 语言编写，静态编译，无外部依赖。
- **容器友好**：支持 Docker 部署，通过环境变量配置端口，支持 host 网络模式以获取完整指标。

## 快速开始 (Docker Compose)

推荐使用 `host` 网络模式以获取最准确的网络和 SSH 状态：

```yaml
services:
  web-monitor-go:
    image: web-monitor-go:offline
    container_name: web-monitor-go
    privileged: true
    network_mode: host
    environment:
      - PORT=38080  # 自定义监听端口
    volumes:
      - /:/hostfs:ro
      - /sys:/sys:ro
      - /proc:/proc
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/run/utmp:/var/run/utmp:ro
      - /etc/passwd:/etc/passwd:ro
      - /etc/group:/etc/group:ro
    restart: unless-stopped
```

访问：`http://<IP>:38080`

## 本地运行
```bash
# 默认监听 8000 端口
go run .

# 自定义端口
PORT=9090 go run .
```

## 详细文档
更多配置、功能说明及安全部署建议（如 Cloudflare Tunnel），请参阅 [使用手册 (MANUAL.md)](MANUAL.md)。
