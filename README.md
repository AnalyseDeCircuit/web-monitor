# Web Monitor (Go)

一个轻量级、高性能的 Linux 系统监控服务，内置现代化 Web 页面与 WebSocket 实时推送。专为小主机、NAS 及容器环境设计。

## 核心功能

### 全方位系统监控
- **CPU 监控**：实时 CPU 使用率、每核心使用率、频率、温度历史、负载均值、上下文切换和中断统计
- **GPU 支持**：支持 Intel 核显、NVIDIA 显卡、AMD 显卡的频率、负载、显存及温度监控，包括进程级的 GPU 显存使用统计
- **内存监控**：虚拟内存和交换分区详细统计、缓冲/缓存分析、内存使用历史数据
- **磁盘监控**：分区使用情况、磁盘 I/O 统计、Inode 使用率
- **网络监控**：接口状态、字节收发统计、错误和丢包监控、TCP 连接状态分类、监听端口列表、套接字统计
- **传感器监控**：硬件温度传感器、电源功率消耗
- **SSH 审计**：实时 SSH 连接数、活跃会话、登录失败次数、认证方式统计及 Known Hosts 变动

### 进程和容器监控
- 进程列表（按 CPU/内存排序）
- 进程树视图、内存 CPU 占用率、IO 统计
- 进程详情（命令行、工作目录、线程数、运行时长）
- OOM 风险进程预警

### Web 界面和 API
- 实时响应式仪表板
- WebSocket 实时数据推送（可配置更新间隔 2-60 秒）
- RESTful API 支持
- 现代化 HTML/CSS/JavaScript 前端

## 技术特性
- **低资源占用**：Go 语言编写，静态编译，无外部依赖
- **容器友好**：支持 Docker 部署，通过环境变量配置端口，支持 host 网络模式以获取完整指标
- **跨平台**：主要针对 Linux，支持多种架构

## 快速开始 (Docker Compose)

推荐使用 `host` 网络模式以获取最准确的网络和 SSH 状态：

```yaml
version: '3.8'
services:
  web-monitor-go:
    image: web-monitor-go:latest
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

启动：`docker-compose up -d`

访问：`http://<IP>:38080`

## 本地运行

```bash
# 编译
go build -o web-monitor main.go

# 默认监听 8000 端口
./web-monitor

# 自定义端口
PORT=9090 ./web-monitor
```

## 系统要求

- **操作系统**：Linux（推荐）
- **Go 版本**：1.21+（如从源码编译）
- **权限**：管理员/root 权限（某些功能需要）
- **依赖包**：gopsutil v3、Gorilla WebSocket

## API 端点

| 方法 | 端点 | 说明 |
|------|------|------|
| `GET` | `/` | Web 前端页面 |
| `WebSocket` | `/ws/stats?interval=2` | 实时系统统计数据流，interval 参数可设置 2-60 秒 |
| `GET` | `/api/info` | 获取系统基本信息（JSON 格式） |

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `8000` | Web 服务监听端口 |
| `SHELL` | `/bin/sh` | 系统 Shell 类型（用于显示） |
| `LANG` | `C` | 系统区域设置 |

## 数据格式示例

WebSocket 推送的实时数据包含 CPU、内存、GPU、网络、SSH、进程等完整系统信息，具体格式可参考源代码中的 `Response` 结构体定义。

## 故障排查

| 问题 | 解决方案 |
|------|--------|
| 无法访问 Web 界面 | 检查防火墙，确保端口已开放；验证应用是否运行 |
| GPU 信息显示 Unknown | 可能不支持该 GPU 或驱动不兼容，检查 `/sys/class/drm/` 目录 |
| SSH 连接统计为空 | 需要 root 权限读取 `/var/log/auth.log`，确认 SSH 服务运行 |
| 某些指标显示为 0 或 Unknown | 容器环境中某些指标可能不可用，需挂载相应系统目录 |

## 部署建议

### 安全部署（Cloudflare Tunnel）

由于本服务包含敏感系统信息，**强烈建议不要直接将端口暴露在公网**。推荐使用 Cloudflare Tunnel 配合 Access 进行安全访问。

详见 [使用手册 (MANUAL.md)](MANUAL.md) 中的安全部署部分。

## 许可证

[Attribution-NonCommercial 4.0 International (CC BY-NC 4.0)](LICENSE)

本项目采用知识共享署名-非商业性使用 4.0 国际许可证。您可以自由分享和改编本作品，但不可用于商业用途，且必须署名。

## 更多文档

详细的配置说明、功能详解及部署建议请参阅 [使用手册 (MANUAL.md)](MANUAL.md)。
