# Web Monitor (Go Version)

一个轻量级、高性能的 Linux 服务器监控与管理面板。采用 Go 语言开发后端，纯 HTML/JS 前端，资源占用极低，部署简单。

## ✨ 功能特性

*   **实时监控**：CPU、内存、磁盘 I/O、网络流量、GPU (NVIDIA/AMD/Intel)、温度传感器。
*   **进程管理**：查看系统 Top 进程，支持按资源占用排序。
*   **Docker 管理**：查看容器/镜像列表，支持启动、停止、重启、删除容器。
*   **系统管理**：
    *   **Systemd 服务**：查看服务状态，支持启动、停止、重启、启用、禁用服务。
    *   **Cron 任务**：查看和编辑计划任务。
*   **网络工具**：内置 Ping, Traceroute, Dig, Curl 等网络诊断工具。
*   **SSH 监控**：监控 SSH 连接数、活跃会话、登录历史及失败记录。
*   **安全审计**：内置用户角色系统 (Admin/User)，记录关键操作日志。
*   **Prometheus 集成**：暴露 `/metrics` 接口，支持 Prometheus/Grafana 采集。

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
    *   `/:/hostfs`: **核心配置**。程序通过 `chroot /hostfs` 来管理宿主机的 Systemd 和 Cron。
    *   `/var/run/docker.sock`: 用于 Docker 管理功能。
    *   `/proc`, `/sys`: 用于采集硬件信息。

## 🛠️ 手动编译与运行

如果你不想使用 Docker，可以直接编译二进制文件运行。

### 编译

```bash
# 开启静态编译以兼容不同 Linux 发行版
export CGO_ENABLED=0
go build -ldflags="-s -w" -trimpath -o web-monitor-go .

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

## 📝 配置文件与数据

所有持久化数据（用户数据库、日志）存储在 `/data` 目录下。在 Docker 部署中，这被映射为 `web-monitor-data` 卷。

## 🖥️ 兼容性

*   **OS**: Linux (推荐 Ubuntu, Debian, CentOS, Alpine)
*   **架构**: AMD64, ARM64
*   **浏览器**: Chrome, Firefox, Edge, Safari (现代浏览器)

## 📄 许可证

CC BY-NC 4.0