# OpsKernel

单机 Linux 服务器监控内核。通过 WebSocket 推送系统指标，提供本机监控与有限管理能力，支持通过 Docker 插件扩展功能。

简体中文 | [English](README_EN.md)

---

## 简介

OpsKernel 是一个面向单机 Linux 的监控与运维内核，用来在**一台服务器上**集中查看系统状态，并在授权的前提下执行少量管理操作（如 Docker 容器、systemd、Cron、电源等）。

项目设计目标：

- 只关注**单节点**，不做集群、控制平面或“统一平台”
- 提供可裁剪的 **Core**：只启用所需采集与管理模块，可缩减到“仅监控”
- 插件 = 通过 Docker 运行的**隔离容器 HTTP 服务**，不在核心进程内加载第三方代码
- 所有高风险能力（杀进程、关机、Docker 管理等）默认只面向管理员，并建议仅在内网或 VPN 后使用

不提供的能力：

- 不做“智能运维”“自动修复”或任何形式的自愈
- 不做 AIOps / 机器学习 / 异常检测
- 不做多租户、集中控制平面或远程主机自动发现

---

## 架构设计

整体架构按“采集 / 聚合 / 展示 / 管理”拆分：

- **Collectors**：CPU、内存、磁盘、网络、GPU、进程、systemd、电源等采集器，周期性从 `/proc`、`/sys`、NVML、systemd D-Bus 等来源读取数据
- **Streaming Aggregator**：将各采集器的最新数据做原子聚合，作为 WebSocket 推送的单一数据源
- **HTTP API + WebSocket Hub**：提供 REST API 与实时监控流；前端通过 WebSocket 订阅所需数据
- **Managers**：Docker / Systemd / Cron / Power / 进程管理，仅管理员可见
- **Alerts**：基于静态规则的告警评估与通知
- **Plugins**：通过 Docker 运行的独立容器，由插件管理器统一发现、启动、停止与代理

核心可裁剪：

- 通过环境变量 `ENABLE_*` 可以关闭 Docker、Systemd、Cron、Power、GPU、SSH、Sensors 等模块
- 将所有 `ENABLE_*` 管理相关项关闭后，可将 OpsKernel 作为“只读监控内核”使用

---

## 功能模块

### 1. 数据采集（Collectors）

所有采集器独立运行，通过环境变量开关控制：

| 模块 | 环境变量 | 默认 | 内容 |
|------|----------|------|------|
| CPU | `ENABLE_CPU` | true | 总使用率、每核负载、频率、温度趋势（来自 Sensors 汇总） |
| Memory | `ENABLE_MEMORY` | true | 物理内存、Swap、Buffers/Cached/Slab 等 |
| Disk | `ENABLE_DISK` | true | 分区使用率、IO、Inode |
| Network | `ENABLE_NETWORK` | true | 接口流量、连接、监听端口 |
| GPU | `ENABLE_GPU` | true | 优先通过 NVML 获取 NVIDIA 详细信息，并通过 DRM 提供 Intel/AMD 等 GPU 的基础信息 |
| Sensors | `ENABLE_SENSORS` | true | 温度、风扇等硬件传感器 |
| Power | `ENABLE_POWER` | true | 电池、电源模式 |
| SSH | `ENABLE_SSH` | true | SSH 会话统计 |

（如果未设置对应采集器，相关页面会自然退化为无数据或隐藏。）

### 2. 管理与控制（Managers）

以下能力均为**高风险能力**，仅管理员可见，且建议不要直接暴露在公网：

| 模块 | 环境变量 | 功能 | 作用范围 |
|------|----------|------|----------|
| Docker | `ENABLE_DOCKER` | 列表、启动、停止、重启、删除容器；镜像列表与删除；日志查看；Prune | 当前 Docker 守护进程 |
| Systemd | `ENABLE_SYSTEMD` | 查看服务、start/stop/restart/reload、enable/disable | 当前主机 systemd |
| Cron | `ENABLE_CRON` | 管理系统 crontab 中带特定标记的任务（增删改查、日志查看） | 当前主机 cron |
| Power | `ENABLE_POWER` | 关机、重启、休眠、取消计划关机；查看 Uptime 与电源状态 | 当前主机 |
| Process | 内置 | 列出进程、按 PID 终止进程 | 当前主机 |

关闭对应 `ENABLE_*` 开关后，相关 HTTP 路由不会被注册，前端页面也不会显示相应管理入口。

### 3. 告警系统

- 基于静态规则的告警引擎
  - 规则包含：指标名、比较运算符、阈值、持续时间、严重级别（warning / critical）、是否启用
  - 支持启用/禁用规则、重置为内置预设规则
  - 告警状态包括触发与恢复，事件保存在内存与持久化存储中
- 通知方式
  - Webhook：向配置的 URL 发送 JSON 负载
  - Dashboard：前端显示当前活跃告警与历史记录
  - 其他渠道（如邮件）由配置驱动
- 仅做**检测与通知**，不执行任何自动修复或自愈动作

### 4. 认证与会话

- 本地用户数据库（JSON 文件），内置 `admin` 账号，角色为 `admin` / `user`
- 密码使用 bcrypt 存储
- 认证使用 JWT（HttpOnly Cookie 或 Authorization 头）
- 登录限流与账户锁定，防暴力破解
- 维护活跃会话列表与登录历史，用户可查看和撤销自身会话（撤销会话不等同于撤销已签发 JWT）

### 5. 前端与 API

- 内置 HTML 模板与静态资源，提供浏览器端监控面板
- WebSocket 推送实时监控数据；REST API 提供系统信息快照与管理操作
- 提供 `/api/metrics` 的 Prometheus 文本输出（当前实现为最小占位指标，主要用于连通性/集成测试场景；不是完整的系统指标导出）

---

## 插件系统

插件系统的设计边界：

- **插件 = 通过 Docker 运行的隔离容器**，通常是一个 HTTP 服务
- 核心进程只负责：
  - 从插件目录读取清单（manifest）
  - 使用本机 Docker 守护进程启动/停止/卸载插件容器（容器自动创建目前未实现，通常需要用户先通过 docker compose 创建容器）
  - 将部分 URL 反向代理到插件容器
  - 记录插件运行状态
- 核心进程**不会**在自身进程内加载插件代码，也没有动态链接或脚本执行

内置插件示例（仅作为机制说明，具体实现不随核心本体一起发布）：

- WebShell：通过 SSH 建立终端会话
- FileManager：通过 SFTP 访问远程文件系统
- DB Explorer：以只读方式浏览数据库
- Perf Report：读取主系统监控数据，生成报表

安全边界说明：

- 插件隔离级别等同于 Docker 容器隔离，不是专用沙箱
- 插件所使用的 SSH/数据库等凭证由用户或配置提供，核心不会自动管理这些凭证
- 对于 `privileged` 类型插件，仅管理员可见和操作

当前仓库默认不会在版本控制中提交具体插件代码（plugins/ 目录被忽略），示例插件通常以独立仓库或镜像形式提供。本节描述的是插件机制和典型形态，而不是固定的内置插件集合。

插件实现后续会以独立仓库发布，并遵循与核心本体相同的许可协议（CC BY-NC 4.0）。

---

## 安全模型

### 角色与授权

- 角色只有两类：
  - `admin`：拥有管理能力（Docker/Systemd/Cron/Power/进程、用户管理、插件管理等）
  - `user`：只读访问监控数据与告警
- 授权在 handler 层做显式检查，不提供细粒度 RBAC 政策编辑界面

### 认证与防护

- JWT 认证；登出会将当前 JWT 加入进程内 revoke 列表（内存存储，重启后不保留）
- 登录速率限制（按 IP 和用户名），多次失败后可触发锁定
- 设置多种 HTTP 安全头（CSP、X-Frame-Options、X-Content-Type-Options 等）

补充说明：WebSocket Origin 校验目前默认较为宽松以兼容反向代理；如需严格限制，可通过 `WS_ALLOWED_ORIGINS` 配置允许列表。

### 高风险能力（建议不上公网）

以下能力在设计上假设运行在**受控内网环境**，不建议直接暴露在公网：

- Docker 容器与镜像管理
- Systemd 服务管理
- Cron 任务增删改
- 进程终止
- 电源操作（关机、重启、休眠）
- 所有 `privileged` 类型插件（如 webshell、filemanager、db-explorer 等）

在公网部署场景下，可通过关闭对应 `ENABLE_*` 开关，将实例缩减为只读监控内核。

---

## 适用场景

适合：

- 单机或少量服务器的监控与日常运维
- 需要本地 Web 面板，而不需要集中控制平面
- 开发/测试环境中快速查看宿主机状态
- 希望通过少量插件扩展（WebShell/FileManager 等）的团队

不适合：

- 大规模集群、机房级或多租户环境
- 需要集中式 agent 管理与统一调度的平台
- 需要自动扩缩容、自动修复、自愈等能力的场景
- 需要长期指标存储与复杂分析的场景（本项目只保留短期内存历史）

---

## 约束与限制

- **单节点架构**：不支持跨节点聚合或集中管理
- **Linux 专用**：依赖 gopsutil、/proc、/sys、systemd D-Bus 等，仅面向 Linux
- **短期指标历史**：指标历史主要保存在内存，侧重于“当前状态”和短窗口趋势，而非长期存储
- **无自动修复**：告警只产生通知，不执行自动操作
- **GPU 支持受限**：NVIDIA 通过 NVML 提供较完整信息，其他品牌通过 DRM 提供较为有限的基础信息
- **插件并非内嵌 SDK**：插件通过 HTTP/Docker 集成，没有统一的编程 SDK 或事件总线
- **RBAC 简单**：只有 admin/user 两级，无多租户、项目或空间概念

上述“不提供/不适合”的能力均指 **OpsKernel 核心本体**；更复杂能力可以通过独立插件或外部系统在其之上实现，但不属于本项目核心职责范围。

---

## 配置与部署概览

### 使用管理脚本快速启动/停止

在仓库根目录使用 `./opskernel.sh` 即可便捷管理核心与插件（依赖 docker compose）：

#### 交互模式（TUI 菜单）

直接运行脚本会启动基于 whiptail 的交互式菜单界面：

```bash
./opskernel.sh
```

> 需要安装 whiptail：`sudo apt install whiptail`

交互菜单顶部会显示简要状态（Docker / Core / Plugins），并提供 `View Status` 查看更详细的 running/stopped/是否崩溃等信息。

#### 命令行模式

也可以直接传入命令参数，无需交互：

```bash
# 状态（推荐先看一眼，避免重复 Start/Stop）
./opskernel.sh status

# 核心服务
./opskernel.sh up              # 启动核心
./opskernel.sh down            # 停止并移除所有容器
./opskernel.sh restart         # 重启核心
./opskernel.sh logs            # 持续查看核心日志
./opskernel.sh stats           # 查看容器资源占用

# 预设模式
./opskernel.sh up-minimal      # 最小模式（仅 CPU/Mem/Disk/Net）
./opskernel.sh up-server       # 服务器模式（无 GPU/Power）
./opskernel.sh up-no-docker    # 禁用 Docker 管理

# 所有插件
./opskernel.sh plugins-build   # 构建所有插件镜像
./opskernel.sh plugins-create  # 创建但不启动插件容器
./opskernel.sh plugins-up      # 启动所有插件
./opskernel.sh plugins-down    # 停止所有插件

# 单个插件（以 webshell 为例）
./opskernel.sh plugin-build webshell
./opskernel.sh plugin-create webshell
./opskernel.sh plugin-up webshell
./opskernel.sh plugin-down webshell
./opskernel.sh plugin-logs webshell

# 一键启动核心 + 所有插件
./opskernel.sh all

# 查看帮助
./opskernel.sh help
```

### 关键环境变量

```bash
# 基础
PORT=8000                    # HTTP 端口
DATA_DIR=/var/lib/opskernel   # 数据目录
JWT_SECRET=<random>          # JWT 签名密钥（生产必须设置）

# 宿主机挂载（容器模式）
HOST_FS=/hostfs
HOST_PROC=/hostfs/proc
HOST_SYS=/hostfs/sys

# Docker 连接
DOCKER_HOST=unix:///var/run/docker.sock
DOCKER_READ_ONLY=false       # 只读模式
```

### 最小化部署示例

仅启用监控，关闭所有管理与高风险模块：

```bash
ENABLE_DOCKER=false \
ENABLE_SYSTEMD=false \
ENABLE_CRON=false \
ENABLE_POWER=false \
ENABLE_SSH=false \
./opskernel
```

### Docker 运行示例

```yaml
services:
  opskernel:
    image: opskernel:latest
    network_mode: host
    pid: host
    cap_add:
      - SYS_PTRACE
      - DAC_READ_SEARCH
    volumes:
      - /:/hostfs:ro
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/data
    environment:
      - HOST_FS=/hostfs
      - JWT_SECRET=${JWT_SECRET}
```

---

## API 概览

这里只给出大致分组，详细接口以代码路由与内置 Swagger 文档为准。

### 公开端点

- `/api/login`：用户登录
- `/api/health`：健康检查
- `/api/metrics`：Prometheus 指标导出

### 认证后端点（所有用户）

- `/ws/stats`：WebSocket 监控流
- `/api/system/info`：系统信息快照
- `/api/alerts/history`：告警历史
- `/api/profile/*`：用户配置、登录历史、会话列表

### 管理端点（仅 admin）

- `/api/docker/*`：Docker 容器与镜像管理
- `/api/systemd/*`：Systemd 服务管理
- `/api/cron/*`：Cron 任务管理
- `/api/power/*`：电源状态与动作
- `/api/process/io`：进程 IO（按 PID 懒加载）
- `/api/process/kill`：终止进程（POST，仅管理员）
- `/api/users/*`：用户管理
- `/api/plugins/list`：插件列表（按角色过滤）
- `/api/plugins/action`：启用/禁用插件（POST，仅管理员）
- `/api/plugins/install`：执行安装钩子（POST，仅管理员，主要用于 privileged 插件）
- `/api/plugins/uninstall`：执行卸载钩子（POST，仅管理员）
- `/api/plugins/<plugin_name>/...`：代理到插件容器

## 许可证

本项目采用 **Creative Commons Attribution-NonCommercial 4.0 International (CC BY-NC 4.0)** 许可证，详细条款见仓库中的 LICENSE 文件。
