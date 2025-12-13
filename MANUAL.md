# Web Monitor 使用手册

## 1. 简介

Web Monitor 是一个基于 Go 语言开发的轻量级 Linux 服务器监控与管理系统。它旨在提供一个简单、高效的 Web 界面，用于实时监控服务器状态并执行常见的管理任务。

### 核心架构

*   **后端**: Go (Golang) 1.21+
    *   使用 `gopsutil` 采集系统信息。
    *   使用 `gorilla/websocket` 推送实时数据。
    *   使用 `os/exec` 执行系统命令 (Docker, Systemd, Cron)。
*   **前端**: 纯 HTML5 / CSS3 / JavaScript (ES6+)
    *   无外部框架依赖 (如 React/Vue)，极度轻量。
    *   使用 WebSocket 接收实时数据。
*   **数据存储**: JSON 文件存储 (`/data/users.json`, `/data/operations.json`)。

---

## 2. 部署指南

### 2.1 Docker 部署 (推荐)

Docker 部署提供了隔离且一致的运行环境。由于监控系统需要访问宿主机的底层信息，因此配置较为特殊。

**docker-compose.yml 详解**:

```yaml
services:
  web-monitor-go:
    image: web-monitor-go:latest
    privileged: true        # 必须开启：允许访问硬件设备和执行特权指令
    network_mode: host      # 推荐开启：直接使用宿主机网络栈，监控更准确
    pid: host               # 必须开启：允许查看宿主机的所有进程
    volumes:
      - /:/hostfs:ro        # 关键：将宿主机根目录挂载到容器内的 /hostfs
                            # 程序通过 chroot /hostfs 来执行宿主机的 systemctl, crontab 等命令
      - /var/run/docker.sock:/var/run/docker.sock # 允许管理宿主机的 Docker
      - /sys:/sys:ro        # 读取硬件传感器信息
      - /proc:/proc         # 读取进程和系统信息
      - web-monitor-data:/data # 持久化存储用户数据
```

**启动**:
```bash
docker-compose up -d
```

### 2.2 二进制部署

适用于无法使用 Docker 的环境。

1.  **编译**:
    ```bash
    go build -ldflags="-s -w" -o web-monitor-go .
    ```
2.  **运行**:
    ```bash
    # 确保有 root 权限，否则部分监控数据无法获取
    sudo ./web-monitor-go
    ```

---

## 3. 功能详解

### 3.1 仪表盘 (Dashboard)
*   **CPU**: 显示总使用率、每个核心的使用率、频率、温度。
*   **内存**: 显示物理内存和 Swap 的使用情况。
*   **磁盘**: 显示各分区的空间使用率和 I/O 读写速度。
*   **网络**: 显示实时上传/下载速度、总流量、连接数统计。
*   **GPU**: 自动检测 NVIDIA, AMD, Intel 显卡，显示显存使用、温度、功耗。

### 3.2 进程管理 (Processes)
*   列出系统资源占用最高的进程。
*   支持按 CPU 或 内存排序。
*   显示进程的 PID, 用户, 线程数, 启动时间, 命令行参数。

### 3.3 Docker 管理
*   **容器列表**: 查看所有运行中和停止的容器。
*   **操作**: 支持 Start, Stop, Restart, Remove 操作。
*   **镜像列表**: 查看本地 Docker 镜像。

### 3.4 系统服务 (Services)
*   基于 `systemd`。
*   列出所有 Service 类型的单元。
*   支持 Start, Stop, Restart, Enable, Disable 操作。
*   **原理**: 容器内通过 `chroot /hostfs systemctl ...` 执行命令。

### 3.5 计划任务 (Cron)
*   读取和编辑 `root` 用户的 crontab。
*   **原理**: 容器内通过 `chroot /hostfs crontab ...` 执行命令。

### 3.6 网络诊断 (Network Diagnostics)
提供网页版的常用网络工具：
*   **Ping**: 测试连通性。
*   **Trace**: 路由追踪 (tracepath)。
*   **Dig**: DNS 查询。
*   **Curl**: HTTP 请求测试。

### 3.7 SSH 监控
*   **状态**: SSH 服务是否运行。
*   **连接**: 当前活跃的 SSH 连接数。
*   **会话**: 显示当前登录的用户 (基于 `who` 命令)。
*   **审计**: 统计公钥/密码登录次数，以及失败登录尝试 (读取 `/var/log/auth.log` 或 `/var/log/secure`)。

---

## 4. 安全与权限

### 4.1 用户管理
*   系统初始化时会自动创建默认管理员：
    *   用户: `admin`
    *   密码: `admin123`
*   **强烈建议**首次登录后在 "Profile" 页面修改密码。
*   管理员可以在 "Users" 页面创建新用户或删除旧用户。
*   **角色**:
    *   `admin`: 拥有所有权限 (包括 Docker, Systemd, Cron 操作)。
    *   `user`: 仅拥有查看权限 (只读模式)。

### 4.2 操作日志
*   系统会记录关键操作，如登录、修改密码、停止容器、重启服务等。
*   管理员可以在 "Logs" 页面查看审计日志。

### 4.3 安全建议
*   不要将服务直接暴露在公网，建议配合 Nginx 反向代理并配置 SSL。
*   如果必须暴露，请确保修改了默认密码。
*   Docker 挂载了 `/` 目录，这意味着获得 Web 面板的 Admin 权限等同于获得了服务器的 Root 权限。请务必保管好管理员账号。

---

## 5. 常见问题 (FAQ)

**Q: 为什么看不到 Systemd 服务或 Cron 任务？**
A: 请检查 Docker 挂载配置。必须将宿主机根目录挂载到容器的 `/hostfs` (`-v /:/hostfs`)。程序依赖此路径来访问宿主机的系统工具。

**Q: 为什么 Docker 管理页面为空？**
A: 请检查是否挂载了 Docker Socket (`-v /var/run/docker.sock:/var/run/docker.sock`)。

**Q: 为什么温度显示为 0 或不准确？**
A: 需要挂载 `/sys` 目录 (`-v /sys:/sys:ro`) 并且容器需要 `privileged: true` 权限来访问硬件传感器。

**Q: 如何重置管理员密码？**
A: 如果忘记密码，可以进入容器或服务器，删除 `/data/users.json` 文件，然后重启服务。系统会重新生成默认的 admin/admin123 账号。
```bash
rm /data/users.json
# 重启容器
docker restart web-monitor-go
```
