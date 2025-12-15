# Web Monitor API 文档（以源码为准）

本文档根据当前项目源码生成（以 `api/handlers/router.go` 注册的路由为准）。示例与字段名尽量严格匹配当前 handler / 公共类型定义。

## 约定

- **Base URL**：`http://<host>:<port>`
- **认证（JWT）**：部分接口会校验 JWT（Header 或 Cookie）。
  - Header：`Authorization: Bearer <token>`
  - Cookie：`auth_token=<token>`
- **权限**：
  - `admin-only`：必须是管理员；会返回 `401 Unauthorized`（未登录/Token 无效）或 `403 Forbidden`（非管理员）
  - 其它接口：当前实现已对大多数 `/api/*` 端点强制校验 JWT（少数公共端点除外，如 `/api/login`、`/api/health`、`/api/metrics`）。
- **返回格式**：
  - JSON：`application/json`
  - 文本：`text/plain`
- **明确的非 JSON 端点**（请使用 `response.text()`）：
  - `GET /api/metrics`（Prometheus 指标输出）
  - `GET /api/cron/logs`（Cron 日志文本）
- **错误返回**：
  - 部分 handler 使用 `http.Error(...)`，会返回 `text/plain` 的错误字符串
  - admin-only 的认证失败通常返回 JSON：`{"error":"..."}`

## 运行配置（环境变量）

- `WS_ALLOWED_ORIGINS`：WebSocket `/ws/stats` 的 Origin 允许列表（逗号分隔）。
  - 默认不设置时：仅允许同源浏览器连接（无 `Origin` 的非浏览器客户端仍允许）。
  - 可填写完整 Origin（如 `https://example.com`）或仅 hostname（如 `example.com`）。

## 认证相关

### POST /api/login

登录并获取 JWT（同时设置 `auth_token` 的 HttpOnly Cookie）。

请求 Body（JSON）：

```json
{
  "username": "admin",
  "password": "admin123"
}
```

成功响应（200，JSON）：

```json
{
  "token": "<jwt>",
  "message": "Login successful",
  "username": "admin",
  "role": "admin"
}
```

常见错误：

- 401（JSON）：

```json
{ "error": "Invalid credentials" }
```

- 429（JSON）：

```json
{ "error": "Too many login attempts. Please try again later." }
```

- 400（text/plain）：`Invalid request`

### POST /api/logout

登出（清理 Cookie；前端也会清理 localStorage token）。

成功响应（200，JSON）：

```json
{ "message": "Logged out" }
```

### POST /api/password

修改密码（需要登录）。

- 普通用户：仅可修改自己的密码，必须提供 `old_password`。
- 管理员：可修改任意用户密码；当修改他人密码时，`old_password` 不要求。

请求 Body（JSON）：

```json
{
  "username": "admin",
  "old_password": "old",
  "new_password": "new"
}
```

成功响应（200，JSON）：

```json
{ "status": "success" }
```

常见错误：

- 400（JSON）：`{"error":"old_password is required"}` / `{"error":"new_password is required"}`
- 401（JSON）：`{"error":"Unauthorized"}` 或 `{"error":"Invalid old password"}`
- 403（JSON）：`{"error":"Forbidden: Admin access required"}`
- 404（JSON）：`{"error":"User not found"}`

### POST /api/validate-password

校验密码复杂度（使用 `internal/utils.ValidatePasswordPolicy`）。

请求 Body（JSON）：

```json
{ "password": "P@ssw0rd123" }
```

成功响应（200，JSON）：

```json
{ "valid": true }
```

常见错误：

- 400（text/plain）：`Invalid request`

## 用户管理（admin-only）

认证失败/权限不足的通用响应：

- 401（JSON）：`{ "error": "Unauthorized" }`
- 403（JSON）：`{ "error": "Forbidden: Admin access required" }`

### GET /api/users

成功响应（200，JSON）：

```json
{
  "users": [
    {
      "id": "admin",
      "username": "admin",
      "role": "admin",
      "created_at": "2025-12-15T00:00:00Z",
      "last_login": null
    }
  ]
}
```

### POST /api/users

创建用户。

请求 Body（JSON）：

```json
{ "username": "alice", "password": "StrongPass#123", "role": "user" }
```

成功响应（201，JSON）：

```json
{ "message": "User created successfully", "username": "alice" }
```

常见错误：

- 400（JSON）：

```json
{ "error": "Username and password are required" }
```

- 400（JSON，密码不符合策略）：

```json
{ "error": "Password does not meet complexity requirements. Must be at least 8 characters long and contain at least three of: uppercase letters, lowercase letters, digits, and special characters." }
```

- 409（JSON，用户已存在）：

```json
{ "error": "user already exists" }
```

### DELETE /api/users?username=alice

成功响应（200，JSON）：

```json
{ "message": "User deleted successfully" }
```

常见错误：

- 400（JSON）：`{ "error": "Username required" }`
- 403（JSON）：`{ "error": "Cannot delete admin user" }`
- 404（JSON）：`{ "error": "user not found" }`

## 操作日志

### GET /api/logs

返回操作日志（admin-only）。

成功响应（200，JSON）：

```json
{
  "logs": [
    {
      "time": "2025-12-15T00:00:00Z",
      "username": "admin",
      "action": "login",
      "details": "User logged in",
      "ip_address": "127.0.0.1:12345"
    }
  ]
}
```

## 系统与监控

### GET /api/info

静态系统信息（`internal/system.StaticInfo`）。

需要登录（任意有效 JWT；admin/user 均可）。

成功响应（200，JSON）：

```json
{
  "header": "my-hostname",
  "os": "Ubuntu 22.04.3 LTS",
  "kernel": "6.2.0-39-generic",
  "uptime": "2 days, 3 hours, 45 minutes",
  "shell": "/bin/bash",
  "cpu": "Intel(R) ...",
  "gpu": "NVIDIA ...",
  "memory": "31.3 GiB / 31.4 GiB (99.7%)",
  "swap": "0 B / 2.0 GiB (0.0%)",
  "disk": "245 GiB / 1.0 TiB (24.0%)",
  "ip": "192.168.1.100",
  "locale": "en_US.UTF-8"
}
```

常见错误：

- 405（text/plain）：`Method not allowed`

### GET /api/system/info

系统综合信息（聚合接口）。该接口会尽量返回所有模块的数据；如果某个模块获取失败，会同时提供 `*_error` 字段并给对应数据一个空值。

需要登录（任意有效 JWT；admin/user 均可）。

成功响应（200，JSON，示例）：

```json
{
  "system_metrics": {
    "cpu_percent": 12.3,
    "memory_percent": 45.6,
    "memory_total": 34359738368,
    "memory_used": 15600000000,
    "memory_free": 18759738368,
    "disk_percent": 24.5,
    "disk_total": 1000000000000,
    "disk_used": 245000000000,
    "disk_free": 755000000000,
    "disk": [
      {
        "device": "/dev/sda1",
        "mountpoint": "/",
        "fstype": "ext4",
        "total": "1.0 TiB",
        "used": "245 GiB",
        "free": "755 GiB",
        "percent": 24.5
      }
    ],
    "disk_io": {
      "sda": {
        "read_bytes": "15.2 GiB",
        "write_bytes": "8.7 GiB",
        "read_count": 123,
        "write_count": 456,
        "read_time": 1234,
        "write_time": 5678
      }
    },
    "timestamp": "2025-12-15T00:00:00Z"
  },
  "docker": {
    "containers": [
      {
        "Id": "a1b2c3",
        "Names": ["/nginx"],
        "Image": "nginx:latest",
        "State": "running",
        "Status": "Up 2 hours",
        "Ports": [
          { "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp" }
        ]
      }
    ]
  },
  "systemd": {
    "services": [
      {
        "unit": "ssh.service",
        "load": "loaded",
        "active": "active",
        "sub": "running",
        "description": "OpenSSH server"
      }
    ]
  },
  "ssh_stats": {
    "status": "Running",
    "connections": 1,
    "sessions": [
      { "user": "root", "ip": "1.2.3.4", "started": "2025-12-15T00:00:00Z" }
    ],
    "auth_methods": { "publickey": 1 },
    "hostkey_fingerprint": "SHA256:...",
    "history_size": 0,
    "oom_risk_processes": [],
    "failed_logins": 0,
    "ssh_process_memory": 0
  },
  "network": {
    "bytes_sent": "1.2 GiB",
    "bytes_recv": "3.4 GiB",
    "raw_sent": 123,
    "raw_recv": 456,
    "interfaces": {
      "eth0": {
        "ip": "192.168.1.100",
        "bytes_sent": "1.2 GiB",
        "bytes_recv": "3.4 GiB",
        "speed": 1000,
        "is_up": true,
        "errors_in": 0,
        "errors_out": 0,
        "drops_in": 0,
        "drops_out": 0
      }
    },
    "sockets": { "tcp": 10 },
    "connection_states": { "ESTABLISHED": 1 },
    "errors": { "total_errors_in": 0 },
    "listening_ports": [ { "port": 22, "protocol": "tcp" } ]
  },
  "power": {
    "profile": "unknown",
    "ac_status": "unknown",
    "timestamp": "2025-12-15T00:00:00Z",
    "ac_power": true,
    "uptime": "2d",
    "shutdown_scheduled": false,
    "scheduled_time": ""
  },
  "cache": { "size": 10 }
}
```

常见错误：

- 405（text/plain）：`Method not allowed`

## Docker

### GET /api/docker/containers

成功响应（200，JSON）：

```json
{
  "containers": [
    {
      "Id": "a1b2c3d4e5f6",
      "Names": ["/nginx"],
      "Image": "nginx:latest",
      "State": "running",
      "Status": "Up 2 days",
      "Ports": [
        { "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp" }
      ]
    }
  ]
}
```

常见错误：

- 500（text/plain）：`Failed to get Docker containers: ...`

### GET /api/docker/images

成功响应（200，JSON）：

```json
{
  "images": [
    {
      "Id": "sha256:abcdef",
      "RepoTags": ["nginx:latest"],
      "Size": 142000000,
      "Created": 1672531200
    }
  ]
}
```

常见错误：

- 500（text/plain）：`Failed to get Docker images: ...`

### POST /api/docker/action（admin-only）

支持的 `action`：`start|stop|restart|remove`。

请求 Body（JSON）：

```json
{ "id": "a1b2c3d4e5f6", "action": "restart" }
```

成功响应（200，JSON）：

```json
{ "status": "success", "message": "Docker action completed" }
```

常见错误：

- 400（JSON）：

```json
{ "error": "Invalid request body: <decode error>" }
```

- 500（JSON）：

```json
{ "error": "Docker action failed: <reason>" }
```

## Systemd

### GET /api/systemd/services

返回 `types.ServiceInfo[]`（注意：不是 `{services: [...]}` 包装）。

成功响应（200，JSON）：

```json
[
  {
    "unit": "ssh.service",
    "load": "loaded",
    "active": "active",
    "sub": "running",
    "description": "OpenSSH server"
  }
]
```

常见错误：

- 500（text/plain）：`Failed to get Systemd services: ...`

### POST /api/systemd/action（admin-only）

支持的 `action`：`start|stop|restart|reload|enable|disable`。

请求 Body（JSON）：

```json
{ "unit": "ssh.service", "action": "restart" }
```

成功响应（200，JSON）：

```json
{ "status": "success" }
```

常见错误：

- 400（JSON）：`{ "error": "Invalid request body" }`
- 400（JSON）：`{ "error": "Invalid action" }`
- 500（JSON）：`{ "error": "<systemd error>" }`

## 网络

### GET /api/network/info

成功响应（200，JSON）：

```json
{
  "interfaces": [
    {
      "name": "eth0",
      "ip": "192.168.1.100",
      "mac": "aa:bb:cc:dd:ee:ff",
      "speed": 1000,
      "is_up": true,
      "mtu": 1500,
      "addresses": ["192.168.1.100/24"],
      "flags": ["up", "broadcast"],
      "hardware_addr": "aa:bb:cc:dd:ee:ff"
    }
  ],
  "timestamp": "2025-12-15T00:00:00Z"
}
```

常见错误：

- 500（text/plain）：`Failed to get network info: ...`

## 电源

### GET /api/power/info

成功响应（200，JSON，示例）：

```json
{
  "profile": "unknown",
  "battery": {
    "present": true,
    "percentage": 85.5,
    "status": "Charging",
    "time_remaining": "1h23m",
    "capacity": 95.0
  },
  "ac_status": "online",
  "timestamp": "2025-12-15T00:00:00Z",
  "ac_power": true,
  "uptime": "2d",
  "shutdown_scheduled": false,
  "scheduled_time": ""
}
```

常见错误：

- 500（text/plain）：`Failed to get power info: ...`

### POST /api/power/action

admin-only。

支持的 `action`：`shutdown|reboot|cancel|suspend|hibernate`。

请求 Body（JSON）：

```json
{ "action": "shutdown", "delay": 0, "reason": "maintenance" }
```

成功响应（200，JSON，`types.PowerActionResult`）：

```json
{
  "action": "shutdown",
  "success": true,
  "message": "Shutdown scheduled",
  "timestamp": "2025-12-15T00:00:00Z",
  "delay": "0s",
  "reason": "maintenance",
  "output": "..."
}
```

常见错误（text/plain）：

- 400（JSON）：`{ "error": "Invalid request body: ..." }`
- 400（JSON）：`{ "error": "Invalid power action: <action>" }`
- 500（JSON）：`{ "error": "Power action failed: ..." }`

### GET /api/power/shutdown-status

成功响应（200，JSON，`types.ShutdownStatus`）：

```json
{
  "scheduled": true,
  "time": "2025-12-15T01:00:00Z",
  "message": "Shutdown scheduled",
  "timestamp": "2025-12-15T00:00:00Z",
  "scheduled_time": "2025-12-15T01:00:00Z",
  "uptime": "2d"
}
```

常见错误：

- 500（text/plain）：`Failed to get shutdown status: ...`

## Cache

### GET /api/cache/info

成功响应（200，JSON）：

```json
{
  "size": 10,
  "keys": ["system_metrics", "docker_containers"],
  "stats": "Cache system is operational"
}
```

## Cron

### GET /api/cron

兼容旧版 cron 列表接口。

成功响应（200，JSON，`types.CronJob[]`）：

```json
[
  { "id": "1", "schedule": "*/5 * * * *", "command": "/usr/bin/true" }
]
```

常见错误：

- 500（text/plain）：`Failed to get cron jobs: ...`

### POST /api/cron（admin-only）

用请求体数组覆盖保存 cron jobs。

请求 Body（JSON，`types.CronJob[]`）：

```json
[
  { "id": "1", "schedule": "*/5 * * * *", "command": "/usr/bin/true" }
]
```

成功响应（200，JSON）：

```json
{ "status": "success" }
```

常见错误：

- 400（JSON）：`{ "error": "Invalid request body" }`
- 500（JSON）：`{ "error": "<save error>" }`

### GET /api/cron/jobs

返回 cron jobs 列表（与 `/api/cron` GET 结构一致）。

成功响应（200，JSON）：

```json
[
  { "id": "1", "schedule": "*/5 * * * *", "command": "/usr/bin/true" }
]
```

### POST /api/cron/action

admin-only。

请求 Body（JSON）：

```json
{
  "action": "add",
  "user": "root",
  "schedule": "* * * * *",
  "command": "/usr/bin/true"
}
```

可用的 `action`：`add|remove|enable|disable`。

成功响应（200，JSON）：

```json
{ "status": "success", "message": "Cron action completed" }
```

常见错误（JSON）：

- 400：`{ "error": "Invalid request body: ..." }`
- 400：`{ "error": "Invalid cron action: <action>" }`
- 500：`{ "error": "Cron action failed: ..." }`

### GET /api/cron/logs?lines=50

返回 cron 日志文本。

成功响应（200，text/plain）：

```text
...log lines...
```

常见错误（JSON）：

- 401：`{ "error": "Unauthorized" }`
- 403：`{ "error": "Forbidden: Admin access required" }`
- 500：`{ "error": "Failed to get cron logs: ..." }`

## 进程管理（admin-only）

### POST /api/process/kill（admin-only）

终止指定 PID（SIGKILL）。服务端会拒绝 `pid <= 1`，并拒绝杀死当前 web-monitor 进程本身。

请求 Body（JSON）：

```json
{ "pid": 1234 }
```

成功响应（200，JSON）：

```json
{ "status": "success" }
```

常见错误（JSON）：

- 400：`{ "error": "Invalid request body" }`
- 400：`{ "error": "Invalid pid" }`
- 400：`{ "error": "Refusing to kill current server process" }`
- 500：`{ "error": "<syscall error>" }`

## 健康检查与指标

### GET /api/health

健康检查（当前 handler 未限制方法）。

成功响应（200，JSON）：

```json
{ "status": "healthy", "version": "1.0.0", "message": "Web Monitor is running" }
```

### GET /api/metrics

Prometheus 指标（Prometheus exposition format）。

成功响应（200，text/plain，示例片段）：

```text
# HELP ...
# TYPE ...
...
```

## WebSocket

### WS /ws/stats?interval=2

实时监控推送（需要 token）。

- Query 参数：
  - `interval`：秒，范围 `2` 到 `60`（小于 2 会被提升到 2，大于 60 会被限制为 60）
  - `token`：JWT（兼容支持；更推荐使用 Cookie 或 `Sec-WebSocket-Protocol` 传递 Token）

- Token 传递方式（优先级）：
  1. Cookie：`auth_token=<token>`
  2. `Sec-WebSocket-Protocol`：客户端可使用 `new WebSocket(url, ['jwt', token])`
  3. Query：`?token=<token>`（仅为兼容保留）

服务端推送消息为 `types.Response`（示例字段结构）：

```json
{
  "cpu": {
    "percent": 12.3,
    "per_core": [10.1, 20.2],
    "times": { "user": 1, "system": 2, "idle": 97, "iowait": 0, "irq": 0, "softirq": 0 },
    "load_avg": [0.12, 0.34, 0.56],
    "stats": { "ctx_switches": 1, "interrupts": 2, "soft_interrupts": 3, "syscalls": 4 },
    "freq": { "avg": 0, "per_core": [] },
    "info": { "model": "...", "architecture": "x86_64", "cores": 8, "threads": 16, "max_freq": 0, "min_freq": 0 },
    "temp_history": [45.2]
  },
  "fans": [],
  "sensors": {},
  "power": {},
  "memory": {
    "total": "31.4 GiB",
    "used": "15.7 GiB",
    "free": "15.7 GiB",
    "percent": 50,
    "available": "20.0 GiB",
    "buffers": "0 B",
    "cached": "0 B",
    "shared": "0 B",
    "active": "0 B",
    "inactive": "0 B",
    "slab": "0 B",
    "history": [50]
  },
  "swap": { "total": "0 B", "used": "0 B", "free": "0 B", "percent": 0, "sin": "0 B", "sout": "0 B" },
  "disk": [],
  "disk_io": {},
  "inodes": [],
  "network": {
    "bytes_sent": "0 B",
    "bytes_recv": "0 B",
    "raw_sent": 0,
    "raw_recv": 0,
    "interfaces": {},
    "sockets": {},
    "connection_states": {},
    "errors": {},
    "listening_ports": []
  },
  "ssh_stats": {
    "status": "Stopped",
    "connections": 0,
    "sessions": [],
    "auth_methods": {},
    "hostkey_fingerprint": "",
    "history_size": 0,
    "oom_risk_processes": [],
    "failed_logins": 0,
    "ssh_process_memory": 0
  },
  "boot_time": "",
  "processes": [],
  "gpu": []
}
```
