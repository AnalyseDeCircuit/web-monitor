# Web-Monitor - 系统监控仪表板

[English Version](./README_EN.md)

一个高性能、低开销的实时系统监控仪表板，使用 Python FastAPI 后端和原生 JavaScript 前端构建。支持 CPU、内存、磁盘、网络、进程等多维度系统监控，具有完整的温度趋势、连接状态分析等高级功能。

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

### ⚡ **性能优化**

- **智能缓存策略**
  - TCP 连接状态：10 秒缓存（减少 90% 系统调用）
  - 进程列表：5 秒缓存（优化遍历开销）
  - SSH 统计：10 秒缓存（降低连接查询频率）
  - GPU 信息：60 秒缓存（静态信息不需频繁读取）
  - 磁盘分区：60 秒缓存

- **低开销数据收集**
  - 温度历史：环形缓冲区（deque，maxlen=60）
  - 内存历史：环形缓冲区（deque，maxlen=60）
  - WebSocket 流式推送，无轮询开销
  - 直接读取 /proc 文件系统，减少系统调用

- **预期性能改进**
  - CPU 消耗降低 30-40%
  - 内存占用稳定（历史数据固定大小）
  - 响应延迟 < 1s

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

- Docker & Docker Compose 或 Python 3.9+

### 方式一：Docker Compose（推荐）

```bash
# 克隆或进入项目目录
cd web-monitor

# 启动服务
docker compose up -d

# 访问 http://localhost:8000
```

### 方式二：本地运行

```bash
# 安装依赖
pip install -r requirements.txt

# 启动服务
python main.py

# 或使用 uvicorn
uvicorn app.main:app --host 0.0.0.0 --port 8000
```

---

## 📊 API 文档

### REST API 端点

#### `GET /api/info`
获取系统基本信息

#### `GET /api/stats`
获取实时系统统计

#### `WebSocket /ws/stats`
实时流式推送系统统计

**查询参数：**
- `interval` (float): 更新间隔，秒为单位（默认 1.0，范围 0.1-60）

```javascript
const ws = new WebSocket('ws://localhost:8000/ws/stats?interval=1.0');
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log(data);
};
```

---

## 🔧 技术架构

### 后端技术栈

| 组件 | 说明 |
|------|------|
| **框架** | FastAPI + Uvicorn |
| **语言** | Python 3.9+ |
| **系统监控** | psutil 5.9+ |
| **模板引擎** | Jinja2 |

### 前端技术栈

| 组件 | 说明 |
|------|------|
| **语言** | Vanilla JavaScript (ES6+) |
| **样式** | CSS3（深色主题） |
| **通信** | WebSocket、REST API |
| **无依赖** | 无第三方库依赖 |

---

## 📦 部署指南

### Docker 单容器部署

```bash
docker run -d \
  --name web-monitor \
  --network host \
  --privileged \
  -v /:/hostfs:ro \
  -v /sys:/sys:ro \
  -v /proc:/proc:ro \
  -p 8000:8000 \
  web-monitor:latest
```

### Docker Compose 配置

```yaml
services:
  web-monitor:
    build: .
    container_name: web-monitor
    network_mode: host
    privileged: true
    volumes:
      - /:/hostfs:ro
      - /sys:/sys:ro
      - /proc:/proc
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/run/utmp:/var/run/utmp:ro
    restart: unless-stopped
    environment:
      - LANG=zh_CN.UTF-8
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

---

## 🐛 故障排除

### 无法读取温度信息

**解决方案：**
```bash
# 确保容器以特权模式运行
docker run --privileged ...

# 检查宿主机传感器
sensors

# 检查容器内传感器可见性
docker exec web-monitor sensors
```

### WebSocket 连接不稳定

**症状**：仪表板数据间歇性停止更新

**解决方案**：检查网络连接和服务器负载，浏览器自动重连机制已实现。

---

## 🤝 贡献指南

欢迎社区贡献！此项目采用 **CC BY-NC 4.0** 协议，不允许商用。

### 开发环境设置

```bash
# 创建虚拟环境
python -m venv venv
source venv/bin/activate

# 安装依赖
pip install -r requirements.txt

# 运行本地开发服务
uvicorn app.main:app --reload --host 0.0.0.0 --port 8000
```

---

## 📄 许可证

本项目采用 **Creative Commons Attribution-NonCommercial 4.0 International (CC BY-NC 4.0)** 协议。

**您可以：**
- ✅ 自由分发和使用（非商业用途）
- ✅ 修改和改进代码
- ✅ 在非商业项目中使用

**您不可以：**
- ❌ 用于商业目的
- ❌ 销售或提供付费服务
- ❌ 作为商业产品的一部分

详见 [LICENSE](./LICENSE) 文件。

---

**最后更新**: 2025-12-11  
**许可证**: CC BY-NC 4.0  
**Python 版本**: 3.9+
````
