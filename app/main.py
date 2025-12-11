from fastapi import FastAPI, Request, WebSocket, WebSocketDisconnect
from fastapi.templating import Jinja2Templates
from fastapi.responses import HTMLResponse
import psutil
import os
import time
import distro
import platform
import socket
import datetime
import asyncio
import json
from collections import deque

app = FastAPI()
templates = Jinja2Templates(directory="app/templates")

# Cache for disk partitions
_disk_partitions_cache = []
_last_partitions_time = 0

# Cache for processes
_process_cache = []
_last_process_time = 0

# Cache for TCP connection states (高优先级优化)
_connection_states_cache = {}
_last_connection_states_time = 0

# Cache for SSH stats
_ssh_stats_cache = {"status": "Stopped", "connections": 0, "sessions": []}
_last_ssh_time = 0

# Cache for GPU info
_gpu_info_cache = None
_last_gpu_info_time = 0

# History data for trends (keep last 60 data points, ~1 minute at 1s interval)
_cpu_temp_history = deque(maxlen=60)
_memory_history = deque(maxlen=60)

class RaplReader:
    def __init__(self):
        self.base_path = "/sys/class/powercap"
        self.last_readings = {}
        self.domains = []
        self._init_domains()
        self.cleanup_counter = 0

    def _init_domains(self):
        self.domains = []
        if not os.path.exists(self.base_path):
            return

        try:
            rapl_dirs = []
            for d in os.listdir(self.base_path):
                if "intel-rapl" in d and ":" in d:
                    rapl_dirs.append(os.path.join(self.base_path, d))
            
            for path in rapl_dirs:
                name_file = os.path.join(path, "name")
                if os.path.exists(name_file):
                    try:
                        with open(name_file, "r") as f:
                            name = f.read().strip()
                        if "package" in name:
                            idx = path.split(":")[-1]
                            name = f"Package-{idx}"
                        
                        max_range = 0
                        max_file = os.path.join(path, "max_energy_range_uj")
                        if os.path.exists(max_file):
                            with open(max_file, "r") as f:
                                max_range = int(f.read().strip())
                                
                        self.domains.append({
                            "path": path,
                            "name": name,
                            "energy_file": os.path.join(path, "energy_uj"),
                            "max_range": max_range
                        })
                    except:
                        continue
        except:
            pass

    def get_power_stats(self):
        stats = {}
        now = time.time()
        
        # Periodic cleanup of history to prevent memory leaks
        self.cleanup_counter += 1
        if self.cleanup_counter > 300:  # Check every ~300 calls
            self.cleanup_counter = 0
            keys_to_remove = []
            for path, (timestamp, _) in self.last_readings.items():
                if now - timestamp > 60:  # Remove if older than 60s
                    keys_to_remove.append(path)
            for k in keys_to_remove:
                del self.last_readings[k]
        
        for domain in self.domains:
            try:
                if os.path.exists(domain["energy_file"]):
                    with open(domain["energy_file"], "r") as f:
                        energy_uj = int(f.read().strip())
                    
                    path = domain["path"]
                    if path in self.last_readings:
                        last_time, last_energy = self.last_readings[path]
                        dt = now - last_time
                        de = energy_uj - last_energy
                        
                        if de < 0:
                            de += domain["max_range"]
                        
                        if dt > 0 and de >= 0:
                            power_watts = (de / 1000000.0) / dt
                            stats[domain["name"]] = round(power_watts, 2)
                    
                    self.last_readings[path] = (now, energy_uj)
            except:
                continue
            
        return stats

rapl_reader = RaplReader()

def get_cpu_info():
    """Get detailed CPU information"""
    info = {
        "model": "Unknown",
        "architecture": platform.machine(),
        "cores": psutil.cpu_count(logical=False),
        "threads": psutil.cpu_count(logical=True),
        "max_freq": 0,
        "min_freq": 0
    }
    
    try:
        # Try to read CPU model from /proc/cpuinfo
        if os.path.exists("/proc/cpuinfo"):
            with open("/proc/cpuinfo", "r") as f:
                for line in f:
                    if "model name" in line:
                        info["model"] = line.split(":")[1].strip()
                        break
    except:
        pass
    
    # Get frequency info
    try:
        freq = psutil.cpu_freq()
        if freq:
            info["max_freq"] = round(freq.max, 2) if freq.max else 0
            info["min_freq"] = round(freq.min, 2) if freq.min else 0
    except:
        pass
    
    return info

def get_size(bytes, suffix="B"):
    factor = 1024
    for unit in ["", "K", "M", "G", "T", "P"]:
        if bytes < factor:
            return f"{bytes:.2f} {unit}i{suffix}"
        bytes /= factor

def get_cpu_model():
    try:
        with open("/proc/cpuinfo", "r") as f:
            for line in f:
                if "model name" in line:
                    return line.split(":")[1].strip()
    except:
        return platform.processor()
    return "Unknown CPU"

def get_gpu_info():
    global _gpu_info_cache, _last_gpu_info_time
    # GPU信息基本不变，60秒缓存
    if time.time() - _last_gpu_info_time > 60 or _gpu_info_cache is None:
        try:
            import glob
            # Intel GPU device IDs mapping (partial list for common ones)
            intel_gpu_names = {
                "0x46d4": "Intel Alder Lake-N [UHD Graphics]",  # N150
                "0x46d1": "Intel Alder Lake-N [UHD Graphics]",  # N100
                "0x46d0": "Intel Alder Lake-N [UHD Graphics]",
            }
            
            # Search all card devices
            for card_path in glob.glob("/sys/class/drm/card?"):
                vendor_file = os.path.join(card_path, "device/vendor")
                device_file = os.path.join(card_path, "device/device")
                
                if os.path.exists(vendor_file) and os.path.exists(device_file):
                    with open(vendor_file, "r") as f:
                        vendor = f.read().strip()
                    with open(device_file, "r") as f:
                        device = f.read().strip()
                    
                    if vendor == "0x8086":  # Intel
                        gpu_name = intel_gpu_names.get(device, f"Intel Graphics [{device}]")
                        _gpu_info_cache = f"{gpu_name} [Integrated]"
                        _last_gpu_info_time = time.time()
                        return _gpu_info_cache
        except Exception as e:
            pass
        _gpu_info_cache = "Unknown GPU"
        _last_gpu_info_time = time.time()
    return _gpu_info_cache

def get_top_processes():
    global _process_cache, _last_process_time
    if time.time() - _last_process_time > 5:
        processes = []
        try:
            # 获取所有进程信息（包括父进程、启动时间、I/O 统计、工作目录等）
            for p in psutil.process_iter(['pid', 'name', 'username', 'num_threads', 'memory_percent', 'cpu_percent']):
                try:
                    p_info = p.info
                    if p_info['memory_percent'] is None: p_info['memory_percent'] = 0
                    if p_info['cpu_percent'] is None: p_info['cpu_percent'] = 0
                    if p_info['username'] is None: p_info['username'] = "unknown"
                    
                    # 附加信息（低开销获取）
                    try:
                        p_obj = psutil.Process(p_info['pid'])
                        
                        # 父进程 ID
                        p_info['ppid'] = p_obj.ppid()
                        
                        # 启动时间（转换为可读格式）
                        try:
                            create_time = p_obj.create_time()
                            uptime_sec = time.time() - create_time
                            if uptime_sec < 60:
                                p_info['uptime'] = f"{int(uptime_sec)}s"
                            elif uptime_sec < 3600:
                                p_info['uptime'] = f"{int(uptime_sec // 60)}m"
                            elif uptime_sec < 86400:
                                p_info['uptime'] = f"{int(uptime_sec // 3600)}h"
                            else:
                                p_info['uptime'] = f"{int(uptime_sec // 86400)}d"
                        except:
                            p_info['uptime'] = "-"
                        
                        # 工作目录
                        try:
                            p_info['cwd'] = p_obj.cwd()
                        except:
                            p_info['cwd'] = "-"
                        
                        # 完整命令行
                        try:
                            cmdline = p_obj.cmdline()
                            p_info['cmdline'] = ' '.join(cmdline) if cmdline else "-"
                        except:
                            p_info['cmdline'] = "-"
                        
                        # I/O 统计（可能不支持所有系统）
                        try:
                            io = p_obj.io_counters()
                            p_info['io_read'] = get_size(io.read_bytes)
                            p_info['io_write'] = get_size(io.write_bytes)
                        except:
                            p_info['io_read'] = "-"
                            p_info['io_write'] = "-"
                        
                    except:
                        p_info['ppid'] = 0
                        p_info['uptime'] = "-"
                        p_info['cwd'] = "-"
                        p_info['cmdline'] = "-"
                        p_info['io_read'] = "-"
                        p_info['io_write'] = "-"
                    
                    processes.append(p_info)
                except:
                    pass
        except:
            pass
            
        # Sort by memory_percent desc
        processes.sort(key=lambda x: x['memory_percent'], reverse=True)
        _process_cache = processes
        _last_process_time = time.time()
    return _process_cache

def build_process_tree(processes):
    """构建进程树结构，用于前端树形展示"""
    # 创建 PID 到进程的映射
    pid_map = {p['pid']: p for p in processes}
    
    # 标记每个进程的子进程
    for p in processes:
        p['children'] = []
    
    # 构建父子关系
    root_processes = []
    for p in processes:
        ppid = p.get('ppid', 0)
        if ppid in pid_map:
            pid_map[ppid]['children'].append(p)
        else:
            # 没有父进程或父进程不在列表中，作为根进程
            root_processes.append(p)
    
    return root_processes

def get_listening_ports():
    """获取监听的端口信息"""
    try:
        listening_ports = {}
        for conn in psutil.net_connections():
            try:
                if conn.status == 'LISTEN' and conn.laddr:
                    port = conn.laddr[1]
                    proto = conn.type
                    protocol = 'TCP' if proto == 1 else 'UDP' if proto == 2 else 'OTHER'
                    
                    # 获取使用该端口的进程名称
                    try:
                        proc = psutil.Process(conn.pid)
                        proc_name = proc.name()
                    except:
                        proc_name = f"PID {conn.pid}"
                    
                    if port not in listening_ports:
                        listening_ports[port] = {'tcp': None, 'udp': None}
                    
                    proto_key = protocol.lower()
                    listening_ports[port][proto_key] = proc_name
            except:
                continue
        
        # 返回格式化的端口列表（最多显示前 20 个）
        result = []
        for port in sorted(listening_ports.keys())[:20]:
            port_info = listening_ports[port]
            proto_str = ""
            if port_info['tcp']:
                proto_str += f"TCP:{port_info['tcp']}"
            if port_info['udp']:
                if proto_str:
                    proto_str += ", "
                proto_str += f"UDP:{port_info['udp']}"
            result.append({"port": port, "protocol": proto_str})
        
        return result
    except:
        return []

@app.get("/", response_class=HTMLResponse)
async def read_root(request: Request):
    return templates.TemplateResponse("index.html", {"request": request})

def get_available_shells():
    shells = set()
    shell_path = "/etc/shells"
    # If running in container with hostfs mounted, use host's shells file
    if os.path.exists("/hostfs/etc/shells"):
        shell_path = "/hostfs/etc/shells"

    try:
        if os.path.exists(shell_path):
            with open(shell_path, "r") as f:
                for line in f:
                    line = line.strip()
                    if not line or line.startswith("#"):
                        continue
                    shell_name = os.path.basename(line)
                    if shell_name in ["nologin", "false", "git-shell", "sync", "shutdown", "halt", "usr", "bin"]:
                        continue
                    shells.add(shell_name.capitalize())
    except:
        pass
    
    if not shells:
        common_shells = ["bash", "zsh", "fish", "sh"]
        import shutil
        for s in common_shells:
            if shutil.which(s):
                shells.add(s.capitalize())

    if not shells:
        return "Unknown"
        
    return ", ".join(sorted(list(shells)))

@app.get("/api/info")
async def get_info():
    boot_time_timestamp = psutil.boot_time()
    uptime_seconds = time.time() - boot_time_timestamp
    uptime_string = str(datetime.timedelta(seconds=int(uptime_seconds)))
    
    mem = psutil.virtual_memory()
    mem_str = f"{get_size(mem.used)} / {get_size(mem.total)} ({mem.percent}%)"
    
    swap = psutil.swap_memory()
    swap_str = f"{get_size(swap.used)} / {get_size(swap.total)} ({swap.percent}%)"
    
    try:
        if os.path.exists("/hostfs"):
            disk = psutil.disk_usage('/hostfs')
        else:
            disk = psutil.disk_usage('/')
        disk_str = f"{get_size(disk.used)} / {get_size(disk.total)} ({disk.percent}%)"
    except:
        disk_str = "Unknown"
    
    ip = "Unknown"
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(('10.255.255.255', 1))
        ip = s.getsockname()[0]
        s.close()
    except:
        pass

    loc = os.environ.get('LANG', 'Unknown')
    user = "root"
    hostname = socket.gethostname()
    
    os_name = f"{distro.name(pretty=True)} {platform.machine()}"
    # Try to read host os-release if available
    try:
        if os.path.exists("/hostfs/etc/os-release"):
            with open("/hostfs/etc/os-release", "r") as f:
                info = {}
                for line in f:
                    if "=" in line:
                        k, v = line.strip().split("=", 1)
                        info[k] = v.strip('"')
                if "PRETTY_NAME" in info:
                    os_name = f"{info['PRETTY_NAME']} {platform.machine()}"
    except:
        pass

    return {
        "header": f"{user}@{hostname}",
        "os": os_name,
        "kernel": platform.release(),
        "uptime": uptime_string,
        "shell": get_available_shells(),
        "cpu": f"{get_cpu_model()} ({psutil.cpu_count(logical=False)}) @ {psutil.cpu_freq().max/1000:.2f} GHz" if psutil.cpu_freq() else get_cpu_model(),
        "gpu": get_gpu_info(),
        "memory": mem_str,
        "swap": swap_str,
        "disk": disk_str,
        "ip": ip,
        "locale": loc
    }

def get_connection_states():
    """获取TCP连接状态统计，带10秒缓存"""
    global _connection_states_cache, _last_connection_states_time
    
    if time.time() - _last_connection_states_time > 10:
        states = {
            "ESTABLISHED": 0,
            "SYN_SENT": 0,
            "SYN_RECV": 0,
            "FIN_WAIT1": 0,
            "FIN_WAIT2": 0,
            "TIME_WAIT": 0,
            "CLOSE": 0,
            "CLOSE_WAIT": 0,
            "LAST_ACK": 0,
            "LISTEN": 0,
            "CLOSING": 0
        }
        try:
            connections = psutil.net_connections(kind='tcp')
            for conn in connections:
                state = conn.status
                if state in states:
                    states[state] += 1
        except:
            pass
        _connection_states_cache = states
        _last_connection_states_time = time.time()
    
    return _connection_states_cache

def get_ssh_stats():
    global _ssh_stats_cache, _last_ssh_time
    # SSH连接变化较慢，改为10秒缓存
    if time.time() - _last_ssh_time > 10:
        ssh_stats = {
            "status": "Stopped",
            "connections": 0,
            "port": 22,
            "sessions": [],
            "auth_methods": {"password": 0, "publickey": 0, "other": 0},
            "failed_logins": 0,
            "hostkey_fingerprint": "-",
            "oom_risk_processes": [],
            "ssh_process_memory": 0,
            "history_size": 0
        }
        try:
            # 1. Check SSH port (22) status and connections
            for conn in psutil.net_connections(kind='tcp'):
                if conn.laddr.port == 22:
                    if conn.status == psutil.CONN_LISTEN:
                        ssh_stats["status"] = "Running"
                    elif conn.status == psutil.CONN_ESTABLISHED:
                        ssh_stats["connections"] += 1
                        
                        remote_ip = conn.raddr.ip if conn.raddr else "Unknown"
                        user_name = "Unknown"
                        started = "-"
                        
                        try:
                            if conn.pid:
                                p = psutil.Process(conn.pid)
                                started = datetime.datetime.fromtimestamp(p.create_time()).strftime("%H:%M:%S")
                                user_name = p.username()
                                try:
                                    cmdline = p.cmdline()
                                    if cmdline:
                                        cmd_str = " ".join(cmdline)
                                        if "sshd:" in cmd_str:
                                            parts = cmd_str.split()
                                            if len(parts) > 1:
                                                potential_user = parts[1]
                                                if "@" in potential_user:
                                                    user_name = potential_user.split('@')[0]
                                                elif potential_user != "[priv]" and potential_user != "[net]":
                                                    user_name = potential_user
                                except:
                                    pass
                        except:
                            pass

                        ssh_stats["sessions"].append({
                            "user": user_name,
                            "ip": remote_ip,
                            "started": started
                        })

            # 2. Get SSH process memory usage
            try:
                for proc in psutil.process_iter(['pid', 'name', 'memory_percent']):
                    if proc.info['name'] in ['sshd', 'ssh']:
                        ssh_stats["ssh_process_memory"] = max(
                            ssh_stats["ssh_process_memory"],
                            proc.info['memory_percent'] or 0
                        )
            except:
                pass

            # 3. Check for OOM risk processes (high memory consumers)
            try:
                procs = psutil.process_iter(['pid', 'name', 'memory_percent'])
                oom_risk = []
                for proc in procs:
                    try:
                        if proc.info['memory_percent'] and proc.info['memory_percent'] > 5:
                            oom_risk.append({
                                "pid": proc.info['pid'],
                                "name": proc.info['name'],
                                "memory": round(proc.info['memory_percent'], 1)
                            })
                    except:
                        pass
                ssh_stats["oom_risk_processes"] = sorted(oom_risk, key=lambda x: x['memory'], reverse=True)[:5]
            except:
                pass

            # 4. Get host key fingerprint (低开销读文件)
            try:
                hostkey_path = "/etc/ssh/ssh_host_rsa_key.pub"
                if os.path.exists(hostkey_path):
                    with open(hostkey_path, 'r') as f:
                        content = f.read().strip()
                        if content:
                            parts = content.split()
                            if len(parts) >= 2:
                                # 提取密钥的前24个字符作为指纹示意
                                ssh_stats["hostkey_fingerprint"] = parts[1][:24] + "..."
            except:
                pass

            # 5. Get SSH auth methods and failed logins (采样方式，低开销)
            try:
                # 采样最近 100 行 auth.log（而不是全文扫描）
                auth_log = "/var/log/auth.log"
                if not os.path.exists(auth_log):
                    auth_log = "/var/log/secure"
                
                if os.path.exists(auth_log):
                    with open(auth_log, 'r') as f:
                        lines = f.readlines()
                        # 只检查最后100行
                        sample_lines = lines[-100:] if len(lines) > 100 else lines
                        
                        for line in sample_lines:
                            if 'sshd' in line and 'Failed password' in line:
                                ssh_stats["failed_logins"] += 1
                            elif 'sshd' in line and 'publickey' in line:
                                if 'Accepted' in line:
                                    ssh_stats["auth_methods"]["publickey"] += 1
                            elif 'sshd' in line and 'password' in line:
                                if 'Accepted' in line:
                                    ssh_stats["auth_methods"]["password"] += 1
            except:
                pass

            # 6. Get SSH session history size (采样，低开销)
            try:
                ssh_history = os.path.expanduser("~/.ssh/")
                if os.path.exists(ssh_history):
                    known_hosts = os.path.join(ssh_history, "known_hosts")
                    if os.path.exists(known_hosts):
                        ssh_stats["history_size"] = len(open(known_hosts).readlines())
            except:
                pass

            _ssh_stats_cache = ssh_stats
        except:
            pass
        _last_ssh_time = time.time()
    return _ssh_stats_cache

def collect_stats():
    cpu_percent = psutil.cpu_percent(interval=None)
    cpu_per_core = psutil.cpu_percent(interval=None, percpu=True)
    
    # CPU Times
    cpu_times = psutil.cpu_times_percent(interval=None)
    cpu_times_dict = {
        "user": cpu_times.user,
        "system": cpu_times.system,
        "idle": cpu_times.idle,
        "nice": getattr(cpu_times, 'nice', 0.0),
        "iowait": getattr(cpu_times, 'iowait', 0.0),
        "irq": getattr(cpu_times, 'irq', 0.0),
        "softirq": getattr(cpu_times, 'softirq', 0.0),
        "steal": getattr(cpu_times, 'steal', 0.0),
        "guest": getattr(cpu_times, 'guest', 0.0),
        "guest_nice": getattr(cpu_times, 'guest_nice', 0.0)
    }

    # Load Average
    try:
        load_avg = psutil.getloadavg()
    except:
        load_avg = (0, 0, 0)

    # CPU Stats (Ctx switches, interrupts)
    try:
        c_stats = psutil.cpu_stats()
        cpu_stats_dict = {
            "ctx_switches": c_stats.ctx_switches,
            "interrupts": c_stats.interrupts,
            "soft_interrupts": c_stats.soft_interrupts,
            "syscalls": c_stats.syscalls
        }
    except:
        cpu_stats_dict = {}

    cpu_freq_avg = 0
    cpu_freq_per_core = []
    try:
        cpu_freq = psutil.cpu_freq()
        cpu_freq_avg = cpu_freq.current if cpu_freq else 0
        
        freqs = psutil.cpu_freq(percpu=True)
        if freqs:
            cpu_freq_per_core = [f.current for f in freqs]
    except:
        pass

    fans = []
    try:
        fans_dict = psutil.sensors_fans()
        if fans_dict:
            for name, entries in fans_dict.items():
                for entry in entries:
                    if entry.current > 10000:
                        continue
                    fans.append({
                        "label": entry.label or name,
                        "current": entry.current
                    })
    except:
        pass

    sensors = {}
    cpu_temp_avg = 0
    cpu_temp_per_core = []
    try:
        temps = psutil.sensors_temperatures()
        if temps:
            # Collect CPU temperatures for history
            core_temps = []
            for name, entries in temps.items():
                sensors[name] = []
                for entry in entries:
                    sensors[name].append({
                        "label": entry.label or name,
                        "current": entry.current,
                        "high": entry.high,
                        "critical": entry.critical
                    })
                    # Track CPU core temperatures
                    if 'coretemp' in name.lower() or 'cpu' in name.lower():
                        if 'core' in (entry.label or '').lower():
                            core_temps.append(entry.current)
            
            if core_temps:
                cpu_temp_avg = round(sum(core_temps) / len(core_temps), 1)
                cpu_temp_per_core = core_temps
                # Add to history
                _cpu_temp_history.append({
                    "time": time.time(),
                    "avg": cpu_temp_avg,
                    "cores": cpu_temp_per_core.copy()
                })
    except:
        pass

    power_status = {}
    try:
        battery = psutil.sensors_battery()
        if battery:
            power_status = {
                "percent": battery.percent,
                "power_plugged": battery.power_plugged,
                "secsleft": battery.secsleft
            }
    except:
        pass

    rapl_stats = rapl_reader.get_power_stats()
    if rapl_stats:
        power_status["rapl"] = rapl_stats
        total_watts = sum(v for k, v in rapl_stats.items() if "Package" in k)
        if total_watts > 0:
            power_status["consumption_watts"] = total_watts

    if "consumption_watts" not in power_status:
        power_watts = 0.0
        try:
            base_path = "/sys/class/power_supply"
            if os.path.exists(base_path):
                for supply in os.listdir(base_path):
                    supply_path = os.path.join(base_path, supply)
                    
                    try:
                        with open(os.path.join(supply_path, "power_now"), "r") as f:
                            p_now = int(f.read().strip())
                            power_watts += p_now / 1000000.0
                            continue
                    except:
                        pass
                    
                    try:
                        with open(os.path.join(supply_path, "voltage_now"), "r") as f:
                            v_now = int(f.read().strip())
                        with open(os.path.join(supply_path, "current_now"), "r") as f:
                            c_now = int(f.read().strip())
                        power_watts += (v_now * c_now) / 1e12
                    except:
                        pass
        except:
            pass

        if power_watts > 0:
            power_status["consumption_watts"] = round(power_watts, 4)

    svmem = psutil.virtual_memory()
    memory_stats = {
        "total": get_size(svmem.total),
        "available": get_size(svmem.available),
        "used": get_size(svmem.used),
        "percent": svmem.percent,
        "buffers": get_size(getattr(svmem, 'buffers', 0)),
        "cached": get_size(getattr(svmem, 'cached', 0)),
        "shared": get_size(getattr(svmem, 'shared', 0)),
        "slab": get_size(getattr(svmem, 'slab', 0)),
        "active": get_size(getattr(svmem, 'active', 0)),
        "inactive": get_size(getattr(svmem, 'inactive', 0))
    }
    
    # Add memory usage to history
    _memory_history.append({
        "time": time.time(),
        "percent": svmem.percent,
        "used": svmem.used,
        "available": svmem.available
    })

    global _disk_partitions_cache, _last_partitions_time
    if time.time() - _last_partitions_time > 60 or not _disk_partitions_cache:
        _disk_partitions_cache = psutil.disk_partitions()
        _last_partitions_time = time.time()
    
    partitions = _disk_partitions_cache
    disk_stats = []
    for partition in partitions:
        if 'loop' in partition.device or partition.fstype == 'squashfs':
            continue
        try:
            mountpoint = partition.mountpoint
            if os.path.exists("/hostfs"):
                check_path = os.path.join("/hostfs", mountpoint.lstrip('/'))
            else:
                check_path = mountpoint

            partition_usage = psutil.disk_usage(check_path)
            disk_stats.append({
                "device": partition.device,
                "mountpoint": partition.mountpoint,
                "total": get_size(partition_usage.total),
                "used": get_size(partition_usage.used),
                "free": get_size(partition_usage.free),
                "percent": partition_usage.percent
            })
        except Exception:
            continue

    # Disk I/O Stats
    disk_io_stats = {}
    try:
        disk_io = psutil.disk_io_counters(perdisk=True)
        if disk_io:
            for disk_name, io in disk_io.items():
                disk_io_stats[disk_name] = {
                    "read_count": io.read_count,
                    "write_count": io.write_count,
                    "read_bytes": get_size(io.read_bytes),
                    "write_bytes": get_size(io.write_bytes),
                    "read_time": io.read_time,
                    "write_time": io.write_time
                }
    except:
        pass

    # Inode stats (Unix-like systems only)
    inode_stats = []
    for partition in partitions:
        if 'loop' in partition.device or partition.fstype == 'squashfs':
            continue
        try:
            mountpoint = partition.mountpoint
            if os.path.exists("/hostfs"):
                check_path = os.path.join("/hostfs", mountpoint.lstrip('/'))
            else:
                check_path = mountpoint
            
            # Get inode info via statvfs
            st = os.statvfs(check_path)
            total_inodes = st.f_files
            free_inodes = st.f_ffree
            used_inodes = total_inodes - free_inodes
            if total_inodes > 0:
                inode_percent = (used_inodes / total_inodes) * 100
                inode_stats.append({
                    "mountpoint": partition.mountpoint,
                    "total": total_inodes,
                    "used": used_inodes,
                    "free": free_inodes,
                    "percent": round(inode_percent, 1)
                })
        except:
            continue

    net_io = psutil.net_io_counters()
    
    # Detailed Interface Stats
    interfaces = {}
    try:
        net_io_per_nic = psutil.net_io_counters(pernic=True)
        net_addrs = psutil.net_if_addrs()
        net_stats = psutil.net_if_stats()
        
        for nic, addrs in net_addrs.items():
            ip = "N/A"
            for addr in addrs:
                if addr.family == socket.AF_INET:
                    ip = addr.address
                    break
            
            io = net_io_per_nic.get(nic)
            stats = net_stats.get(nic)
            
            interfaces[nic] = {
                "ip": ip,
                "bytes_sent": get_size(io.bytes_sent) if io else "0B",
                "bytes_recv": get_size(io.bytes_recv) if io else "0B",
                "speed": stats.speed if stats else 0,
                "is_up": stats.isup if stats else False,
                "errors_in": io.errin if io else 0,
                "errors_out": io.errout if io else 0,
                "drops_in": io.dropin if io else 0,
                "drops_out": io.dropout if io else 0
            }
    except:
        pass

    # TCP Connection States (使用缓存函数，每10秒查询一次)
    connection_states = get_connection_states()

    network_stats = {
        "bytes_sent": get_size(net_io.bytes_sent),
        "bytes_recv": get_size(net_io.bytes_recv),
        "raw_sent": net_io.bytes_sent,
        "raw_recv": net_io.bytes_recv,
        "interfaces": interfaces,
        "sockets": {"tcp": 0, "udp": 0, "tcp_tw": 0},
        "connection_states": connection_states,
        "errors": {
            "total_errors_in": net_io.errin,
            "total_errors_out": net_io.errout,
            "total_drops_in": net_io.dropin,
            "total_drops_out": net_io.dropout
        },
        "listening_ports": get_listening_ports()
    }
    
    # Socket Stats (Low overhead via /proc/net/sockstat)
    try:
        if os.path.exists("/proc/net/sockstat"):
            with open("/proc/net/sockstat", "r") as f:
                for line in f:
                    if line.startswith("TCP:"):
                        parts = line.split()
                        for i, part in enumerate(parts):
                            if part == "inuse":
                                network_stats["sockets"]["tcp"] = int(parts[i+1])
                            elif part == "tw":
                                network_stats["sockets"]["tcp_tw"] = int(parts[i+1])
                    elif line.startswith("UDP:"):
                        parts = line.split()
                        for i, part in enumerate(parts):
                            if part == "inuse":
                                network_stats["sockets"]["udp"] = int(parts[i+1])
    except:
        pass

    # Swap Stats
    try:
        swap = psutil.swap_memory()
        swap_stats = {
            "total": get_size(swap.total),
            "used": get_size(swap.used),
            "free": get_size(swap.free),
            "percent": swap.percent,
            "sin": get_size(swap.sin),
            "sout": get_size(swap.sout)
        }
    except:
        swap_stats = {"percent": 0, "total": "0B", "used": "0B"}

    boot_time_timestamp = psutil.boot_time()
    bt = time.localtime(boot_time_timestamp)
    boot_time = f"{bt.tm_year}/{bt.tm_mon}/{bt.tm_mday} {bt.tm_hour}:{bt.tm_min}:{bt.tm_sec}"

    return {
        "cpu": {
            "percent": cpu_percent,
            "per_core": cpu_per_core,
            "times": cpu_times_dict,
            "load_avg": load_avg,
            "stats": cpu_stats_dict,
            "freq": {
                "avg": cpu_freq_avg,
                "per_core": cpu_freq_per_core
            },
            "info": get_cpu_info(),
            "temp_history": list(_cpu_temp_history)
        },
        "fans": fans,
        "sensors": sensors,
        "power": power_status,
        "memory": {
            **memory_stats,
            "history": list(_memory_history)
        },
        "swap": swap_stats,
        "disk": disk_stats,
        "disk_io": disk_io_stats,
        "inodes": inode_stats,
        "network": network_stats,
        "ssh_stats": get_ssh_stats(),
        "boot_time": boot_time,
        "processes": get_top_processes()
    }

@app.get("/api/stats")
async def get_stats():
    return collect_stats()

@app.websocket("/ws/stats")
async def websocket_endpoint(websocket: WebSocket, interval: float = 1.0):
    await websocket.accept()
    
    # Clamp interval to reasonable bounds
    if interval < 0.1: interval = 0.1
    if interval > 60: interval = 60
    
    try:
        while True:
            start_time = time.time()
            try:
                data = collect_stats()
                await websocket.send_json(data)
            except Exception as e:
                print(f"Error collecting/sending stats: {e}")
                import traceback
                traceback.print_exc()
                await asyncio.sleep(1)
                continue
            
            elapsed = time.time() - start_time
            sleep_time = max(0, interval - elapsed)
            await asyncio.sleep(sleep_time)
    except WebSocketDisconnect:
        pass

