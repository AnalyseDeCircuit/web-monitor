# Web Monitor

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-CC%20BY--NC%204.0-lightgrey.svg" alt="License">
  <img src="https://img.shields.io/badge/Platform-Linux-FCC624?logo=linux&logoColor=black" alt="Platform">
  <img src="https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white" alt="Docker">
</p>

<p align="center">
  <strong>🚀 高性能实时系统监控面板</strong>
</p>

<p align="center">
  基于 Go 构建的轻量级系统监控工具，支持 Docker 容器部署和裸机运行。<br/>
  通过 WebSocket 实时推送，提供 CPU、内存、GPU、网络、Docker、Systemd 等全方位监控。
</p>

<p align="center">
  <a href="./README_EN.md">English</a> | 简体中文
</p>

---

## ✨ 特性

### 📊 实时监控
- **CPU**：使用率、每核心负载、频率、温度历史趋势
- **内存**：物理内存、Swap、缓存、Buffer 详细分析
- **磁盘**：分区信息、使用率、IO 读写、Inode 状态
- **GPU**：NVIDIA GPU 支持（via nvml）- 显存、温度、功耗、进程
- **网络**：接口流量、连接状态、监听端口、Socket 统计
- **进程**：Top 进程列表、CPU/内存占用、IO 统计

### 🔧 系统管理
- **Docker 管理**：容器启停/重启/删除、镜像管理
- **Systemd 服务**：服务列表、启停/重启/启用/禁用
- **Cron 任务**：定时任务增删改查、日志查看
- **进程管理**：进程终止（仅管理员）

### 🧩 插件系统
- **WebShell**：SSH 终端 (24-bit 真彩色、xterm.js)
- **FileManager**：SFTP 文件管理器
- **DB Explorer**：数据库连接浏览器
- **Perf Report**：性能报告生成器
- **插件隔离**：每个插件运行在独立容器中
- **热加载**：无需重启主程序即可安装插件

### 🔐 安全特性
- **JWT 认证**：安全的 Token 认证机制
- **角色权限**：管理员/普通用户分离
- **速率限制**：登录防暴力破解
- **安全头**：CSP、X-Frame-Options、HSTS 等
- **Token 撤销**：支持登出后 Token 失效

### 🌐 现代化前端
- **实时更新**：WebSocket 双向通信
- **PWA 支持**：可安装为桌面/移动应用
- **响应式设计**：适配各种屏幕尺寸
- **深色主题**：护眼暗色界面
- **图表可视化**：Chart.js 驱动的实时图表

### ⚡ 高性能设计
- **流式数据采集**：各模块独立采集周期，无阻塞式数据流
- **智能缓存**：TTL 缓存减少系统负载
- **Canvas 图表**：HiDPI 支持的 Canvas 渲染，性能更优
- **DOM 复用**：智能列表更新，避免重复创建 DOM 元素
- **渲染节流**：WebSocket 数据 10 FPS 最大渲染频率
- **优雅关闭**：支持信号处理和平滑退出
- **模块化架构**：通过环境变量按需启用/禁用功能（Docker, GPU, SSH 等）

```mermaid
graph LR
    Browser[Web Browser]
    
    subgraph Server["Go Server"]
        Entry[Entry Point]
        
        subgraph ConfigLayer["Configuration"]
            Config[Config Manager]
            ModuleFlags["Module Flags<br/>ENABLE_*"]
        end
        
        subgraph Middleware["Middleware"]
            SecHeaders[Security Headers]
            AuthMW[Auth]
            RateLimit[Rate Limiter]
        end
        
        Router["Router<br/>(Conditional Routes)"]
        WSHandler[WebSocket]
        
        subgraph Services["Core Services"]
            Auth[Auth Service]
            Monitor[Monitoring]
            WSHub[WebSocket Hub]
            Alerts[Alert Manager]
        end
        
        subgraph Cache["Cache"]
            MetricsCache[Metrics Cache]
        end
        
        subgraph Collection["Streaming Aggregator"]
            direction TB
            FastCollectors["Fast Collectors<br/>(CPU/Mem/Net)"]
            SlowCollectors["Slow Collectors<br/>(Disk/GPU/SSH)"]
            AtomicStorage["Atomic Storage"]
        end
        
        subgraph Management["Management (Optional)"]
            DockerMgmt[Docker]
            SystemdMgmt[Systemd]
            CronMgmt[Cron]
        end
    end
    
    subgraph Sources["Data Sources"]
        Host["Host System"]
        Docker["Docker"]
        SystemD[Systemd]
        ProcFS["/proc /sys"]
    end
    
    Browser -->|HTTP/WS| SecHeaders
    SecHeaders --> AuthMW --> RateLimit
    RateLimit --> Router
    RateLimit --> WSHandler
    
    Entry --> ConfigLayer
    ConfigLayer -->|Enable Flags| Router
    ConfigLayer -->|Enable Flags| FastCollectors
    ConfigLayer -->|Enable Flags| SlowCollectors
    
    Router -->|Conditional| Management
    Router --> Monitor
    WSHandler --> WSHub
    
    Monitor <--> MetricsCache
    WSHub --> AtomicStorage
    
    FastCollectors -->|2-60s| AtomicStorage
    SlowCollectors -->|2-10s| AtomicStorage
    FastCollectors --> Host
    FastCollectors --> ProcFS
    SlowCollectors --> Host
    Management --> Docker
    Management --> SystemD
```

---

## 🚀 快速开始

### Docker 部署（推荐）

我们提供了 `Makefile` 来简化常用操作。

```bash
# 克隆仓库
git clone https://github.com/AnalyseDeCircuit/web-monitor.git
cd web-monitor

# 启动服务 (全量模式)
make up

# 启动极简模式 (仅 CPU/内存/磁盘/网络)
make up-minimal

# 查看日志
make logs
```

访问 `http://localhost:38080`，默认账户：
- 用户名：`admin`
- 密码：`admin123`

> ⚠️ **首次登录后请立即修改密码！**

### 基础配置 (.env)

项目根目录下的 `.env` 文件用于配置核心安全和网络选项。首次部署前请务必检查：

```dotenv
# 必须修改！设置一个长且随机的字符串作为 JWT 签名密钥
JWT_SECRET=change-me-to-a-long-random-string

# 如果通过域名或反向代理访问，请设置允许的 WebSocket 来源
# 多个来源用逗号分隔
WS_ALLOWED_ORIGINS=https://your-domain.com

# 服务端口 (仅影响 Docker 内部映射，宿主机端口在 docker-compose.yml 中修改)
PORT=38080
```

### 模块化配置

你可以通过环境变量控制启用的模块。在 `docker-compose.yml` 中修改或在启动时指定：

| 变量名 | 默认值 | 描述 |
| :--- | :--- | :--- |
| `ENABLE_DOCKER` | `true` | 启用 Docker 管理 |
| `ENABLE_GPU` | `true` | 启用 NVIDIA GPU 监控 |
| `ENABLE_SSH` | `true` | 启用 SSH 会话监控 |
| `ENABLE_CRON` | `true` | 启用 Cron 任务管理 |
| `ENABLE_SYSTEMD` | `true` | 启用 Systemd 服务管理 |
| `ENABLE_SENSORS` | `true` | 启用硬件传感器 (温度/风扇) |
| `ENABLE_POWER` | `true` | 启用电源管理 (电池/性能模式) |

**示例：仅禁用 Docker 和 GPU**
```bash
ENABLE_DOCKER=false ENABLE_GPU=false make up
```

### 常用命令

| 命令 | 作用 |
| :--- | :--- |
| `make up` | 启动所有服务 (后台运行) |
| `make up-minimal` | 启动极简模式 (仅核心指标) |
| `make up-server` | 启动服务器模式 (无 GPU/电源) |
| `make up-no-docker` | 启动无 Docker 模式 |
| `make all` | 启动核心服务 + 所有插件 |
| `make down` | 停止并移除容器 |
| `make restart` | 重启服务 |
| `make logs` | 查看实时日志 |
| `make rebuild` | 重新构建镜像并重启 |
| `make stats` | 显示服务资源使用统计 |

### 插件命令

| 命令 | 作用 |
| :--- | :--- |
| `make plugin-build P=webshell` | 构建单个插件 |
| `make plugin-up P=webshell` | 启动单个插件 |
| `make plugin-down P=webshell` | 停止单个插件 |
| `make plugin-logs P=webshell` | 查看插件日志 |
| `make plugins-build` | 构建所有插件 |
| `make plugins-up` | 启动所有插件 |
| `make plugins-down` | 停止所有插件 |

可用插件：`webshell`, `filemanager`, `db-explorer`, `perf-report`

### 流式聚合器架构

本项目采用流式数据采集架构，各采集器独立运行，不互相阻塞：

| 采集器 | 更新间隔 | 说明 |
| :--- | :--- | :--- |
| CPU, 内存, 网络 | 用户设定 (2-60s) | 快速指标，响应用户选择 |
| 磁盘, 传感器, GPU | 2s | 中速指标 |
| 电源 | 3s | 电池/功耗 |
| SSH | 5s | 会话监控 |
| 系统信息 | 10s | 进程列表等 |

**优势**：
- 快速指标（CPU/内存）不受慢指标（SSH）影响
- 各模块原子存储，WebSocket 推送时瞬时合并
- 无 goroutine 阻塞，内存占用稳定

### 裸机部署

```bash
# 构建
go build -mod=vendor -o server ./cmd/server

# 设置环境变量
export PORT=8000
export DATA_DIR=/var/lib/web-monitor

# 运行
./server
```

---

## 📁 项目结构

```
web-monitor/
├── cmd/
│   ├── server/          # 主程序入口
│   └── dockerproxy/     # Docker Socket 代理
├── api/handlers/        # HTTP 路由和处理器
├── internal/
│   ├── auth/            # 认证和授权
│   ├── cache/           # 指标缓存
│   ├── collectors/      # 数据采集器（11个）
│   ├── config/          # 配置管理
│   ├── cron/            # Cron 任务管理
│   ├── docker/          # Docker API 客户端
│   ├── middleware/      # 中间件
│   ├── monitoring/      # 监控服务和告警
│   ├── plugin/          # 插件管理器
│   ├── systemd/         # Systemd 服务管理
│   └── websocket/       # WebSocket Hub
├── plugins/                 # 插件目录
│   ├── webshell/        # SSH 终端插件
│   ├── filemanager/     # 文件管理器插件
│   ├── db-explorer/     # 数据库浏览器插件
│   └── perf-report/     # 性能报告插件
├── pkg/types/           # 公共类型定义
├── static/              # 前端静态资源
├── templates/           # HTML 模板
└── vendor/              # 依赖（离线构建）
```

---

## ⚙️ 配置说明

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `PORT` | `8000` | HTTP 服务端口 |
| `DATA_DIR` | `/data` | 数据存储目录 |
| `JWT_SECRET` | 随机生成 | JWT 签名密钥 |
| `WS_ALLOWED_ORIGINS` | `*` | WebSocket 允许的源 |
| `HOST_FS` | `/hostfs` | 宿主机文件系统挂载点 |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker API 地址 |

### 容器模式 vs 裸机模式

**容器模式**（自动检测 `HOST_FS`）：
- 通过 `/hostfs` 挂载访问宿主机系统
- 需要特定的 Linux Capabilities

**裸机模式**（`HOST_FS` 为空）：
- 直接访问本机 `/proc`、`/sys` 等
- 无需额外权限配置

### Docker Compose 配置参考

```yaml
services:
  web-monitor-go:
    image: web-monitor-go:latest
    cap_add:
      - SYS_PTRACE        # 读取进程信息
      - DAC_READ_SEARCH   # 读取日志文件
      - SYS_CHROOT        # Cron 管理
    network_mode: host
    pid: host
    volumes:
      - /:/hostfs:ro
      - /run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro
```

---

## 📡 API 概览

### 认证
| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/login` | POST | 用户登录 |
| `/api/logout` | POST | 用户登出 |
| `/api/password` | POST | 修改密码 |

### 监控数据
| 端点 | 方法 | 说明 |
|------|------|------|
| `/ws/stats` | WebSocket | 实时监控数据流 |
| `/api/system/info` | GET | 系统信息快照 |
| `/api/info` | GET | 静态系统信息 |

### 管理功能（需认证）
| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/docker/containers` | GET | Docker 容器列表 |
| `/api/docker/action` | POST | 容器操作（管理员） |
| `/api/systemd/services` | GET | Systemd 服务列表 |
| `/api/systemd/action` | POST | 服务操作（管理员） |
| `/api/cron/jobs` | GET | Cron 任务列表 |
| `/api/users` | GET/POST | 用户管理（管理员） |

**📖 完整 API 文档**:
- **Swagger UI** (推荐): `http://localhost:8000/swagger/index.html` - 交互式API测试
- **Markdown**: [API_DOCUMENTATION.md](./API_DOCUMENTATION.md) - 详细文档
- **生成文档**: `make docs` - 自动同步最新代码

---

## 🛡️ 安全建议

1. **修改默认密码**：首次登录后立即修改 admin 密码
2. **设置 JWT_SECRET**：生产环境务必设置强随机密钥
3. **限制网络访问**：建议通过反向代理（Nginx）并启用 HTTPS
4. **Docker Socket 代理**：使用 `docker-socket-proxy` 限制 Docker API 暴露面
5. **定期更新**：关注项目更新以获取安全补丁

---

## 🧩 插件系统

Web Monitor 支持通过插件扩展功能。插件运行在独立容器中，通过主程序代理访问。

### 内置插件

| 插件 | 描述 | 端口 |
|------|------|------|
| **WebShell** | SSH 终端 (24-bit 真彩色、链接点击) | 38101 |
| **FileManager** | SFTP 文件管理器 | 38102 |
| **DB Explorer** | MySQL/PostgreSQL/SQLite 连接器 | 38104 |
| **Perf Report** | 性能报告生成 (Chart.js 可视化) | 38105 |

### 插件管理

```bash
# 构建并启动所有插件
make plugins-build
make plugins-up

# 或使用一键启动 (核心 + 插件)
make all

# 管理单个插件
make plugin-up P=webshell
make plugin-logs P=webshell
```

### 插件开发

详见 [plugins/DEVELOPMENT.md](./plugins/DEVELOPMENT.md) 插件开发指南。

---

## 🔌 GPU 支持

### NVIDIA GPU

自动检测并通过 nvml 库采集：
- GPU 使用率
- 显存使用
- 温度/功耗
- GPU 进程

需要在 Docker 中启用 NVIDIA Container Toolkit：

```yaml
environment:
  - NVIDIA_VISIBLE_DEVICES=all
  - NVIDIA_DRIVER_CAPABILITIES=all
```

---

## 📊 架构设计

详细架构图请参阅 [ARCHITECTURE.md](./ARCHITECTURE.md)。

```
┌─────────────┐     ┌──────────────────────────────────────┐
│   Browser   │────▶│            Go Server                 │
│  (WebSocket)│◀────│  ┌─────────┐  ┌──────────────────┐  │
└─────────────┘     │  │ Router  │──│ WebSocket Hub    │  │
                    │  └────┬────┘  └────────┬─────────┘  │
                    │       │                │            │
                    │  ┌────▼────┐  ┌────────▼─────────┐  │
                    │  │ Cache   │◀─│ Stats Aggregator │  │
                    │  └─────────┘  └────────┬─────────┘  │
                    │                        │            │
                    │         ┌──────────────┼──────────┐ │
                    │         ▼              ▼          ▼ │
                    │  ┌──────────┐ ┌──────────┐ ┌──────┐ │
                    │  │Collectors│ │  Docker  │ │Systemd││
                    │  │ (x11)    │ │  Client  │ │ D-Bus ││
                    │  └──────────┘ └──────────┘ └──────┘ │
                    └──────────────────────────────────────┘
```

---

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 提交 Pull Request

---

## 📄 许可证

本项目采用 [CC BY-NC 4.0 许可证](./LICENSE)（署名-非商业性使用）。

---

## 🙏 致谢

- [gopsutil](https://github.com/shirou/gopsutil) - 跨平台系统信息采集
- [go-nvml](https://github.com/NVIDIA/go-nvml) - NVIDIA GPU 监控
- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket 实现
- [Chart.js](https://www.chartjs.org/) - 前端图表库
