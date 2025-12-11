# Web-Monitor - 系统监控仪表板

[English Version](./README_EN.md)

一个高性能、低开销的实时系统监控仪表板，采用 Python FastAPI 后端和原生 JavaScript 前端构建。支持 CPU、内存、磁盘、网络、进程、SSH 等多维度系统监控，具有完整的温度趋势、连接状态分析、智能缓存等高级功能。

**核心亮点**：采用增量日志读取、智能多层缓存、WebSocket 流式传输等优化技术，在保证实时性的同时使 **CPU 占用率从 82% 优化至 47%（43% 性能提升）**。

## 📋 主要特性

### 🖥️ **核心监控功能**

- **CPU 监控**
  - 实时 CPU 整体使用率与每核心使用率
  - CPU 温度趋势图（60秒历史记录）
  - 每核心温度实时显示
  - CPU 型号、架构、核心数、线程数、频率范围
  - CPU 时间分布（User、System、Idle、IO Wait 等）
  - 上下文切换、中断、软中断、系统调用统计
  - CPU 功耗监控（Intel RAPL 和系统功耗）

- **内存监控**
  - 内存使用率与容量展示
  - 内存使用趋势图（60秒历史记录）
  - 详细内存分项（缓冲区、缓存、共享内存、活跃/非活跃等）
  - 交换分区监控

- **磁盘监控**
  - 磁盘分区容量与使用率
  - 磁盘 I/O 统计（读写吞吐量、操作计数、延迟）
  - Inode 使用统计
  - 多磁盘聚合视图

- **网络监控**
  - 全局网络流量统计（上传/下载速度、总流量）
  - **TCP 连接状态详细分类**（ESTABLISHED、TIME_WAIT、LISTEN 等）
  - **网络错误与丢包统计**（按接口详细统计）
  - 活跃连接数（TCP/UDP/TIME_WAIT）
  - 网络接口详情（IP、速率、状态）
  - SSH 活跃连接监控

- **进程监控**
  - TOP 进程排序（按内存使用率）
  - 进程 PID、名称、用户、线程数、内存、CPU 占用率

- **系统信息**
  - 系统启动时间与运行时长
  - 操作系统、内核版本、CPU 型号、GPU 信息
  - 内存、交换分区、磁盘容量总览
  - 可用 Shell、系统语言、IP 地址、主机名

- **服务器监控**
  - SSH 服务状态与活跃连接数
  - 当前 SSH 会话详情（用户、IP、连接时间）

### ⚡ **性能优化架构**

#### 智能多层缓存策略
| 指标 | 缓存时间 | 优化收益 | 说明 |
|------|--------|--------|------|
| TCP 连接状态 | 10 秒 | ↓ 90% 系统调用 | 减少 psutil.net_connections() 调用频率 |
| 进程列表 | 5 秒 | ↓ 40% CPU 遍历 | 使用 process_iter() 单次迭代 |
| SSH 统计 | 10 秒 | ↓ 50% 连接查询 | 增量日志读取 + 累积计数 |
| GPU 信息 | 60 秒 | ↓ 99% 文件读取 | 静态信息无需频繁更新 |
| 磁盘分区 | 60 秒 | ↓ I/O 开销 | 分区列表基本不变 |

#### 增量日志读取系统（SSH 认证）
采用 **file.seek()** 技术实现增量读取，避免重复处理日志行：
- 跟踪文件偏移量，只读取新增行
- 支持日志轮转自动检测与恢复
- 累积计数器持续记录认证统计
- 初次读取时读取最后 10000 字节，获取足够历史上下文

#### 内存占用优化
- **历史数据**：环形缓冲区（deque maxlen=60），恒定 ~2 MB
- **缓存数据**：TCP 状态 ~5-10 MB、进程列表 ~3-5 MB、其他 ~2 MB
- **总体**：稳定在 50-80 MB，不会持续增长

#### CPU 性能改进案例
- **前**：82% CPU 占用（频繁系统调用、高频推送）
- **后**：47% CPU 占用（智能缓存、合并迭代、增加最小间隔）
- **提升**：43% 的性能改进

### 🎨 **用户界面**

- **响应式设计**
  - 支持桌面和平板等多种设备
  - 深色主题，护眼舒适

- **多页面视图**
  - 仪表板（快速概览）
  - CPU Details（详细 CPU 分析）
  - Memory（内存详情）
  - Network（网络分析）
  - Storage（磁盘分析）
  - Processes（进程监控）
  - Info（系统信息）

- **实时更新**
  - WebSocket 连接自动推送
  - 可自定义更新间隔（0.1s - 60s）
  - 断线自动重连

---

## 🚀 快速开始

### 前置条件
- **Docker & Docker Compose**（推荐），或
- **Python 3.9+**（本地开发）

### 方式一：Docker Compose（推荐）

```bash
# 进入项目目录
cd web-monitor

# 启动服务
docker compose up -d

# 查看日志
docker compose logs -f web-monitor

# 停止服务
docker compose down

# 访问仪表板：http://localhost:8000
```

### 方式二：本地运行（开发/测试）

```bash
# 1. 创建虚拟环境
python3 -m venv venv
source venv/bin/activate

# 2. 安装依赖
pip install -r requirements.txt

# 3. 启动服务
python -m uvicorn app.main:app --host 0.0.0.0 --port 8000

# 4. 访问仪表板：http://localhost:8000
```

**本地运行注意事项：**
- 需要 Linux/macOS 系统（Windows 10+ WSL2 可行）
- 部分功能需要 root 权限（温度读取、网络连接详情）
- Docker 环境性能监控更准确

---

## 📊 API 文档

### REST API 端点

#### `GET /api/info`
获取系统基本信息（启动时加载一次）

**示例响应：**
```json
{
  "system": "Linux",
  "release": "5.15.0-112-generic",
  "hostname": "my-server",
  "cpu_model": "Intel(R) Core(TM) i7-9700K",
  "memory_total_gb": 16.0,
  "boot_time": "2025/10/18 9:54:0",
  "gpu_model": "Intel UHD Graphics 630"
}
```

#### `GET /api/stats`
获取实时系统统计数据（主要用于初始加载）

#### `WebSocket /ws/stats`
实时流式推送系统统计数据（推荐方式）

**连接方式：**
```javascript
// 基本连接
const ws = new WebSocket('ws://localhost:8000/ws/stats?interval=5.0');

// 事件处理
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('CPU:', data.cpu.percent + '%');
};
```

**查询参数：**
- `interval` (float): 数据推送间隔，秒为单位
  - 范围：2 ~ 60 秒（优化后的推荐范围）
  - 默认：5 秒
  - 示例：`ws://localhost:8000/ws/stats?interval=2.0`

**浏览器特性：**
- 失焦自动降低更新频率（节省带宽）
- 网络中断自动重连（指数退避）

---

## 🔧 技术架构

### 后端技术栈

| 组件 | 版本 | 用途 |
|------|------|------|
| **Python** | 3.9+ | 核心运行时 |
| **FastAPI** | 0.100+ | Web 框架，支持异步 I/O |
| **Uvicorn** | 0.20+ | ASGI 服务器，高性能 |
| **psutil** | 5.9+ | 系统监控库 |
| **Jinja2** | 3.0+ | 模板引擎 |

### 前端技术栈

| 组件 | 说明 |
|------|------|
| **HTML5** | 语义化标记，响应式布局 |
| **CSS3** | Grid 布局、深色主题、媒体查询 |
| **JavaScript (ES6+)** | Vanilla JS，无第三方库依赖 |
| **WebSocket API** | 原生浏览器 API，双向实时通信 |

**前端亮点：**
- 零依赖（不依赖 jQuery、React、Vue 等）
- 轻量级（主要 JS 文件 < 50 KB）
- 高兼容性（Chrome 60+、Firefox 55+、Safari 11+）

---

## 📦 部署指南

### Docker Compose 配置详解

```yaml
version: '3.8'

services:
  web-monitor:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: web-monitor
    
    # 网络配置 - 使用主机网络，直接访问系统资源
    network_mode: host
    
    # 权限配置 - 特权模式，访问硬件传感器
    privileged: true
    
    # 卷挂载
    volumes:
      - /:/hostfs:ro              # 主机根文件系统（只读）
      - /sys:/sys:ro              # sysfs（只读）
      - /proc:/proc               # procfs（读写）
      - /var/run/docker.sock:/var/run/docker.sock:ro  # Docker socket
      - /var/run/utmp:/var/run/utmp:ro                # 用户会话信息
    
    # 自动重启
    restart: unless-stopped
    
    # 环境变量
    environment:
      - LANG=zh_CN.UTF-8
      - TZ=Asia/Shanghai
```

### Docker 单容器部署

```bash
# 构建镜像
docker build -t web-monitor:latest .

# 运行容器
docker run -d \
  --name web-monitor \
  --network host \
  --privileged \
  -v /:/hostfs:ro \
  -v /sys:/sys:ro \
  -v /proc:/proc \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /var/run/utmp:/var/run/utmp:ro \
  -e TZ=Asia/Shanghai \
  -p 8000:8000 \
  web-monitor:latest
```

### Nginx 反向代理配置

```nginx
server {
    listen 80;
    server_name monitor.example.com;

    location / {
        proxy_pass http://localhost:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # WebSocket 支持
    location /ws/ {
        proxy_pass http://localhost:8000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
    }
}
```

---

## ⚙️ 性能优化细节

### 缓存策略

| 指标 | 缓存时间 | 收益 |
|------|--------|------|
| TCP 连接状态 | 10 秒 | ↓ 90% 系统调用 |
| 进程列表 | 5 秒 | ↓ 40% CPU 遍历 |
| SSH 统计 | 10 秒 | ↓ 50% 连接查询 |
| GPU 信息 | 60 秒 | ↓ 99% 文件读取 |
| 磁盘分区 | 60 秒 | ↓ I/O 开销 |

### 内存占用估算

- **基础占用**：~40-60 MB（Python 运行时）
- **历史数据**：~2 MB（60 条 CPU/内存历史）
- **缓存数据**：~5-10 MB（连接状态、进程列表等）
- **总计**：~50-80 MB（稳定不增长）

### CPU 优化案例

**优化前：82% CPU**
- 频繁调用 psutil.process_iter()
- 为每个连接读取 cwd 和 io_counters（高开销）
- WebSocket 最小间隔 0.1s

**优化步骤：**
1. 合并进程迭代（-15% CPU）
2. 移除高开销字段 cwd、io_counters（-10% CPU）
3. 增加最小推送间隔从 0.1s 到 2s（-18% CPU）

**优化后：47% CPU（43% 性能提升）**

---

## 🐛 故障排除

### 问题 1：无法读取 CPU 温度

**症状**：CPU 温度显示为 "-" 或 "Unknown"

**原因：**
- 系统无温度传感器驱动
- 容器非特权模式
- `/sys/class/thermal/` 无访问权限

**解决方案：**
```bash
# 检查宿主机温度
sensors

# Docker 容器必须启用特权模式
docker run --privileged ...

# 检查容器内传感器可见性
docker exec web-monitor ls /sys/class/thermal/
```

### 问题 2：WebSocket 连接间歇断连

**症状**：仪表板数据每隔几秒停止更新，然后恢复

**原因：**
- 网络不稳定
- 服务器负载过高
- 防火墙/代理超时

**解决方案**：浏览器自动重连机制已实现，检查网络和服务器负载

### 问题 3：SSH 认证计数不增长

**症状**：登录成功后 SSH 页面的 Auth Methods 仍显示 0

**原因：**
- auth.log 未被正确读取
- 日志格式不匹配

**解决方案：**
```bash
# 检查 auth.log 中的日志格式
tail -20 /var/log/auth.log | grep -i "sshd.*accepted\|failed"

# Docker 容器需要挂载 /hostfs
docker exec web-monitor cat /hostfs/var/log/auth.log | wc -l
```

### 问题 4：内存占用持续增长

**症状**：容器内存占用从 60MB 增长至 200MB+

**原因：**
- 缓存使用了普通 List 而非 Deque
- 内存泄漏

**诊断：**
```bash
# 检查内存占用
docker stats web-monitor

# 查看进程内存映射
docker exec web-monitor ps aux | grep python
```

---

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

### 开发环境设置

```bash
# 1. 克隆仓库
git clone <repo-url>
cd web-monitor

# 2. 创建虚拟环境
python -m venv venv
source venv/bin/activate

# 3. 安装开发依赖
pip install -r requirements.txt

# 4. 运行开发服务（自动热重载）
uvicorn app.main:app --reload --host 0.0.0.0 --port 8000
```

### 代码结构

```
web-monitor/
├── app/
│   ├── main.py              # FastAPI 主程序、所有路由和缓存逻辑
│   ├── templates/
│   │   └── index.html       # 前端完整页面（HTML + CSS + JS）
│   └── __init__.py
├── docker-compose.yml       # Docker Compose 配置
├── Dockerfile               # Docker 镜像定义
├── requirements.txt         # Python 依赖
├── README.md               # 中文文档
├── README_EN.md            # 英文文档
└── LICENSE                 # CC BY-NC 4.0 许可证
```

### 提交规范

**格式：**
```
<type>: <subject>

<body>

<footer>
```

**类型：**
- `feat`: 新功能
- `fix`: 问题修复
- `docs`: 文档更新
- `perf`: 性能优化
- `refactor`: 代码重构

**示例：**
```
perf: 优化 TCP 连接状态读取，增加 10 秒缓存

- 使用 psutil.net_connections() 缓存结果
- 减少系统调用频率
- CPU 占用率降低 15%

Closes #123
```

---

## 📄 许可证

本项目采用 **Creative Commons Attribution-NonCommercial 4.0 International (CC BY-NC 4.0)** 协议。

### 您可以：
- ✅ 自由使用、修改、分发（非商业用途）
- ✅ 在技术博客/文章中引用
- ✅ 在公司内部使用（非对外商业产品）
- ✅ 作为学生项目或课程作业

### 您不可以：
- ❌ 用于任何商业目的（销售、SaaS、付费服务）
- ❌ 声称拥有该项目或其变体
- ❌ 移除许可证和版权声明
- ❌ 将其作为商业产品的核心组件

详见 [LICENSE](./LICENSE) 文件或 [CC BY-NC 4.0 官方说明](https://creativecommons.org/licenses/by-nc/4.0/)

---

**最后更新**: 2025-12-12  
**许可证**: CC BY-NC 4.0  
**Python 版本**: 3.9+  
**维护者**: Web-Monitor 社区
````
