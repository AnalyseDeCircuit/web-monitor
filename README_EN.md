# Web-Monitor - System Monitoring Dashboard

[中文版本](./README.md)

A high-performance, low-overhead real-time system monitoring dashboard built with Python FastAPI backend and vanilla JavaScript frontend. Supports multi-dimensional system monitoring including CPU, memory, disk, network, and processes, with advanced features like temperature trends and connection state analysis.

## 📋 Main Features

### 🖥️ **Core Monitoring Functions**

- **CPU Monitoring**
  - Real-time CPU overall usage and per-core usage rates
  - CPU temperature trend graph (60-second historical record)
  - Real-time per-core temperature display
  - CPU model, architecture, core count, thread count, frequency range
  - CPU time distribution (User, System, Idle, IO Wait, etc.)
  - Context switches, interrupts, soft interrupts, and syscall statistics
  - CPU power consumption monitoring (Intel RAPL and system power)

- **Memory Monitoring**
  - Memory usage rate and capacity display
  - Memory usage trend graph (60-second historical record)
  - Detailed memory breakdown (buffers, cache, shared memory, active/inactive, etc.)
  - Swap partition monitoring

- **Disk Monitoring**
  - Disk partition capacity and usage rate
  - Disk I/O statistics (read/write throughput, operation counts, latency)
  - Inode usage statistics
  - Multi-disk aggregated view

- **Network Monitoring**
  - Global network traffic statistics (upload/download speed, total traffic)
  - **Detailed TCP connection state classification** (ESTABLISHED, TIME_WAIT, LISTEN, etc.)
  - **Network error and packet loss statistics** (per-interface detailed stats)
  - Active connection counts (TCP/UDP/TIME_WAIT)
  - Network interface details (IP, speed, status)
  - SSH active connection monitoring

- **Process Monitoring**
  - Top processes sorting (by memory usage rate)
  - Process PID, name, user, thread count, memory, CPU usage rate

- **System Information**
  - System boot time and uptime
  - OS, kernel version, CPU model, GPU information
  - Memory, swap partition, disk capacity overview
  - Available shells, system language, IP address, hostname

- **Server Monitoring**
  - SSH service status and active connection count
  - Current SSH session details (user, IP, connection time)

### ⚡ **Performance Optimization**

- **Intelligent Caching Strategy**
  - TCP connection states: 10-second cache (90% reduction in system calls)
  - Process list: 5-second cache (optimized traversal overhead)
  - SSH statistics: 10-second cache (reduced connection query frequency)
  - GPU information: 60-second cache (static info doesn't need frequent updates)
  - Disk partitions: 60-second cache

- **Low-overhead Data Collection**
  - Temperature history: Ring buffer (deque, maxlen=60)
  - Memory history: Ring buffer (deque, maxlen=60)
  - WebSocket streaming push, no polling overhead
  - Direct /proc filesystem reads, minimized system calls

- **Expected Performance Improvements**
  - 30-40% reduction in CPU consumption
  - Stable memory usage (fixed-size historical data)
  - Response latency < 1s

### 🎨 **User Interface**

- **Responsive Design**
  - Support for desktop, tablet, and other devices
  - Dark theme for eye comfort

- **Multi-page View**
  - Dashboard (quick overview)
  - CPU Details (detailed CPU analysis)
  - Memory (memory details)
  - Network (network analysis)
  - Storage (disk analysis)
  - Processes (process monitoring)
  - Info (system information)

- **Real-time Updates**
  - WebSocket automatic push updates
  - Customizable update interval (0.1s - 60s)
  - Automatic reconnection on disconnection

---

## 🚀 Quick Start

### Prerequisites

- Docker & Docker Compose OR Python 3.9+

### Method 1: Docker Compose (Recommended)

```bash
# Navigate to project directory
cd web-monitor

# Start the service
docker compose up -d

# Access at http://localhost:8000
```

### Method 2: Local Development

```bash
# Install dependencies
pip install -r requirements.txt

# Start the service
python main.py

# Or using uvicorn
uvicorn app.main:app --host 0.0.0.0 --port 8000
```

---

## 📊 API Documentation

### REST API Endpoints

#### `GET /api/info`
Get basic system information

**Response example:**
```json
{
  "header": "root@hostname",
  "os": "Ubuntu 22.04 x86_64",
  "kernel": "6.1.0-1024-generic",
  "uptime": "10 days, 3:45:12",
  "shell": "bash, zsh",
  "cpu": "Intel Core i7-11700 (8) @ 4.90 GHz",
  "gpu": "Intel Graphics [Integrated]",
  "memory": "8.0 GiB / 16.0 GiB (50%)",
  "swap": "0 B / 2.0 GiB (0%)",
  "disk": "50.0 GiB / 100.0 GiB (50%)",
  "ip": "192.168.1.100",
  "locale": "en_US.UTF-8"
}
```

#### `GET /api/stats`
Get real-time system statistics

**Response contains:**
- `cpu`: usage rate, core frequencies, time distribution, temperature history, CPU info
- `memory`: usage rate, detailed breakdown, historical records
- `disk`: partition list, I/O statistics, inode usage
- `network`: traffic, interface details, TCP connection states, error statistics
- `processes`: top process list
- `ssh`: SSH service status and session information
- `boot_time`: system boot time

#### `WebSocket /ws/stats`
Real-time streaming system statistics push

**Query parameters:**
- `interval` (float): Update interval in seconds (default 1.0, range 0.1-60)

**Example connection:**
```javascript
const ws = new WebSocket('ws://localhost:8000/ws/stats?interval=1.0');
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log(data);
};
```

---

## 🔧 Technical Architecture

### Backend Tech Stack

| Component | Description |
|-----------|-------------|
| **Framework** | FastAPI + Uvicorn |
| **Language** | Python 3.9+ |
| **System Monitoring** | psutil 5.9+ |
| **Template Engine** | Jinja2 |
| **Async Support** | asyncio, WebSocket |

### Frontend Tech Stack

| Component | Description |
|-----------|-------------|
| **Language** | Vanilla JavaScript (ES6+) |
| **Styling** | CSS3 (dark theme) |
| **Communication** | WebSocket, REST API |
| **Charting** | CSS gradient bar charts |
| **Dependencies** | Zero third-party libraries |

### Core Dependencies

```
fastapi==0.104.1
uvicorn==0.24.0
psutil==5.9.6
python-distro==1.8.0
jinja2==3.1.2
```

---

## 📦 Deployment Guide

### Docker Single Container Deployment

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

### Docker Compose Configuration

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
      - LANG=en_US.UTF-8
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-monitor
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web-monitor
  template:
    metadata:
      labels:
        app: web-monitor
    spec:
      hostNetwork: true
      containers:
      - name: web-monitor
        image: web-monitor:latest
        ports:
        - containerPort: 8000
        volumeMounts:
        - name: hostfs
          mountPath: /hostfs
          readOnly: true
        - name: sys
          mountPath: /sys
          readOnly: true
        - name: proc
          mountPath: /proc
          readOnly: true
      volumes:
      - name: hostfs
        hostPath:
          path: /
      - name: sys
        hostPath:
          path: /sys
      - name: proc
        hostPath:
          path: /proc
```

### Systemd Service

```ini
[Unit]
Description=Web Monitor System Dashboard
After=network.target

[Service]
Type=simple
User=monitor
WorkingDirectory=/opt/web-monitor
ExecStart=/usr/bin/python3 -m uvicorn app.main:app --host 0.0.0.0 --port 8000
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

---

## 🔍 Monitoring Metrics Details

### CPU Temperature

- **Data Source**:
  - Prioritizes `psutil.sensors_temperatures()`
  - Reads coretemp / CPU sensor data
  - Supports both per-core and average temperature display

- **Historical Record**:
  - Retains last 60 seconds of data (1-second sampling interval)
  - Ring buffer implementation with fixed memory footprint

- **Update Frequency**:
  - WebSocket: 1 second (configurable)
  - REST API: unlimited

### Memory Usage

- **Breakdown Items**:
  - `Total`: Total memory
  - `Used`: Memory in use
  - `Available`: Available memory
  - `Buffers`: Buffer memory
  - `Cached`: Cache memory
  - `Shared`: Shared memory
  - `Active`: Active memory
  - `Inactive`: Inactive memory
  - `Slab`: Kernel Slab memory

- **Historical Record**:
  - Retains last 60 seconds of data
  - Tracks percentage, used, and available memory

### TCP Connection States

- **Tracked State Types** (9 types):
  - `ESTABLISHED`: Connection established
  - `TIME_WAIT`: Waiting for timeout closure
  - `CLOSE_WAIT`: Waiting for application closure
  - `LISTEN`: Listening for new connections
  - `SYN_SENT`: Synchronization sent
  - `SYN_RECV`: Synchronization received
  - `FIN_WAIT1/2`: Closure handshake in progress
  - `LAST_ACK`: Final acknowledgment

- **Caching Strategy**:
  - Updates every 10 seconds (reduces system calls)
  - Ring buffer records history
  - Supports high-connection scenarios

### Network Error Statistics

- **Global Statistics**:
  - `errors_in`: Receive errors
  - `errors_out`: Send errors
  - `drops_in`: Receive packet drops
  - `drops_out`: Send packet drops

- **Per-Interface Statistics**:
  - Same as above, but per network interface
  - Only displays interfaces with errors

---

## ⚙️ Performance Optimization Details

### Caching Strategy Comparison

| Metric | Cache Time | Benefit |
|--------|------------|---------|
| TCP connection states | 10s | ↓ 90% system calls |
| Process list | 5s | ↓ 40% CPU traversal |
| SSH statistics | 10s | ↓ 50% connection queries |
| GPU information | 60s | ↓ 99% file reads |
| Disk partitions | 60s | ↓ I/O overhead |

### Memory Usage Estimation

- **Base usage**: ~40-60 MB (Python runtime)
- **Historical data**: ~2 MB (60 CPU/memory history entries)
- **Cache data**: ~5-10 MB (connection states, process list, etc.)
- **Total**: ~50-80 MB (stable, non-increasing)

### CPU Consumption

- **Idle**: < 1% (mostly waiting)
- **WebSocket connected**: ~0.5-2% (depends on update frequency)
- **Heavy queries**: ~3-5% (with concurrent requests)

---

## 🐛 Troubleshooting

### Issue 1: Cannot read temperature information

**Symptoms**:
- CPU temperature shows 0°C
- Temperature trend graph is empty

**Causes**:
1. System has no temperature sensor drivers
2. Container insufficient privileges (needs privileged mode)
3. Temperature information path differs

**Solutions**:
```bash
# Ensure container runs in privileged mode
docker run --privileged ...

# Check host sensors
sensors  # requires lm-sensors package

# Check sensor visibility in container
docker exec web-monitor sensors
```

### Issue 2: WebSocket connection unstable

**Symptoms**:
- Dashboard data updates intermittently stop
- Browser console shows connection errors

**Causes**:
1. Network instability
2. Server overload
3. Browser auto-closes idle connections

**Solutions**:
```javascript
// Add reconnection logic on frontend
const reconnect = () => {
  setTimeout(() => connectWebSocket(), 3000);
};
```

### Issue 3: Memory leak

**Symptoms**:
- Container memory usage continuously increases
- Performance degradation over time

**Causes**:
1. Historical data not properly cleared (fixed with deque)
2. Cache fragmentation

**Diagnosis**:
```bash
# Monitor container memory
docker stats web-monitor

# View detailed information
docker exec web-monitor ps aux | grep python
```

---

## 📝 Logging and Debugging

### Enable Debug Mode

```bash
# Modify docker-compose.yml
environment:
  - DEBUG=true

# Or run directly
PYTHONUNBUFFERED=1 python main.py
```

### View Real-time Logs

```bash
# Docker container logs
docker logs -f web-monitor

# Log locations
/app/logs/  # if logging is configured
```

---

## 🤝 Contributing Guide

We welcome community contributions! Please note this project uses **CC BY-NC 4.0** license and does not allow commercial use.

### Contribution Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

### Development Environment Setup

```bash
# Clone repository
git clone <your-fork-url>
cd web-monitor

# Create virtual environment
python -m venv venv
source venv/bin/activate  # Linux/Mac
venv\Scripts\activate  # Windows

# Install dependencies (including dev tools)
pip install -r requirements.txt
pip install pytest black flake8

# Run tests
pytest tests/

# Code formatting
black app/

# Code checking
flake8 app/
```

---

## 📄 License

This project is licensed under the **Creative Commons Attribution-NonCommercial 4.0 International (CC BY-NC 4.0)** license.

**You can:**
- ✅ Freely distribute and use (for non-commercial purposes)
- ✅ Modify and improve code
- ✅ Use in non-commercial projects

**You cannot:**
- ❌ Use for commercial purposes
- ❌ Sell or provide paid services
- ❌ Use as part of a commercial product

See [LICENSE](./LICENSE) file for details.

---

## 📞 Contact and Support

- **Bug Reports**: Submit an Issue
- **Feature Requests**: Discuss in Discussions
- **Security Issues**: Contact maintainers privately

---

## 🙏 Acknowledgments

Thank you to all contributors and users for their support!

---

## 📚 Changelog

### v1.0.0 (2025-12-11)

- ✨ Initial release
- ✨ Complete system monitoring functionality
- ✨ 5 high-priority features
- ✨ Performance optimization (caching strategy)
- 📖 Detailed documentation in Chinese and English

---

**Last Updated**: 2025-12-11  
**License**: CC BY-NC 4.0  
**Python Version**: 3.9+
