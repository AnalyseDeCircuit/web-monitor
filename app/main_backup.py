from fastapi import FastAPI, Request
from fastapi.templating import Jinja2Templates
from fastapi.responses import HTMLResponse
import psutil
import os
import time
import distro
import platform
import socket
import datetime

app = FastAPI()

templates = Jinja2Templates(directory="app/templates")

class RaplReader:
    def __init__(self):
        self.base_path = "/sys/class/powercap"
        self.last_readings = {}  # path -> (timestamp, energy_uj)

    def get_power_stats(self):
        stats = {}
        if not os.path.exists(self.base_path):
            return stats

        try:
            # Find all intel-rapl directories
            rapl_dirs = []
            if os.path.exists(self.base_path):
                for d in os.listdir(self.base_path):
                    if "intel-rapl" in d and ":" in d:
                        rapl_dirs.append(os.path.join(self.base_path, d))
            
            for path in rapl_dirs:
                name_file = os.path.join(path, "name")
                energy_file = os.path.join(path, "energy_uj")
                
                if os.path.exists(name_file) and os.path.exists(energy_file):
                    try:
                        with open(name_file, "r") as f:
                            name = f.read().strip()
                        # Make name unique if multiple packages
                        if "package" in name:
                            idx = path.split(":")[-1]
                            name = f"Package-{idx}"
                        
                        with open(energy_file, "r") as f:
                            energy_uj = int(f.read().strip())
                            
                        now = time.time()
                        
                        if path in self.last_readings:
                            last_time, last_energy = self.last_readings[path]
                            dt = now - last_time
                            de = energy_uj - last_energy
                            
                            # Handle wrap around
                            if de < 0:
                                max_file = os.path.join(path, "max_energy_range_uj")
                                if os.path.exists(max_file):
                                    with open(max_file, "r") as f:
                                        max_range = int(f.read().strip())
                                    de += max_range
                            
                            if dt > 0 and de >= 0:
                                power_watts = (de / 1000000.0) / dt
                                stats[name] = round(power_watts, 2)
                        
                        self.last_readings[path] = (now, energy_uj)
                    except Exception:
                        continue
        except Exception:
            pass
            
        return stats

rapl_reader = RaplReader()

import locale
import getpass
import subprocess

app = FastAPI()

templates = Jinja2Templates(directory="app/templates")

def get_size(bytes, suffix="B"):
    """
    Scale bytes to its proper format
    e.g:
        1253656 => '1.20MB'
        1253656678 => '1.17GB'
    """
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
    # Try to get GPU info from lspci if available, or /sys
    # Since we are in a container, lspci might not be installed.
    # We can try to look at /sys/class/drm/card0/device/uevent
    try:
        # Simple check for Intel GPU in /sys
        if os.path.exists("/sys/class/drm/card0/device/vendor"):
            with open("/sys/class/drm/card0/device/vendor", "r") as f:
                vendor = f.read().strip()
            if vendor == "0x8086":
                return "Intel Graphics [Integrated]"
    except:
        pass
    return "Unknown GPU"

@app.get("/", response_class=HTMLResponse)
async def read_root(request: Request):
    return templates.TemplateResponse("index.html", {"request": request})

@app.get("/api/info")
async def get_info():
    boot_time_timestamp = psutil.boot_time()
    uptime_seconds = time.time() - boot_time_timestamp
    uptime_string = str(datetime.timedelta(seconds=int(uptime_seconds)))
    
    # Memory
    mem = psutil.virtual_memory()
    mem_str = f"{get_size(mem.used)} / {get_size(mem.total)} ({mem.percent}%)"
    
    # Swap
    swap = psutil.swap_memory()
    swap_str = f"{get_size(swap.used)} / {get_size(swap.total)} ({swap.percent}%)"
    
    # Disk /
    try:
        if os.path.exists("/hostfs"):
            disk = psutil.disk_usage('/hostfs')
        else:
            disk = psutil.disk_usage('/')
        disk_str = f"{get_size(disk.used)} / {get_size(disk.total)} ({disk.percent}%)"
    except:
        disk_str = "Unknown"
    
    # IP
    ip = "Unknown"
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(('10.255.255.255', 1))
        ip = s.getsockname()[0]
        s.close()
    except:
        pass

    # Locale
    loc = os.environ.get('LANG', 'Unknown')

    # User/Host
    user = "root" # In container it's root
    hostname = socket.gethostname()

    return {
        "header": f"{user}@{hostname}",
        "os": f"{distro.name(pretty=True)} {platform.machine()}",
        "kernel": platform.release(),
        "uptime": uptime_string,
        "shell": os.environ.get("SHELL", "/bin/sh"),
        "cpu": f"{get_cpu_model()} ({psutil.cpu_count(logical=False)}) @ {psutil.cpu_freq().max/1000:.2f} GHz" if psutil.cpu_freq() else get_cpu_model(),
        "gpu": get_gpu_info(),
        "memory": mem_str,
        "swap": swap_str,
        "disk": disk_str,
        "ip": ip,
        "locale": loc
    }

@app.get("/api/stats")

def get_cpu_model():
    try:
        with open("/proc/cpuinfo", "r") as f:
            for line in f:
                if "model name" in line:
                    return line.split(":")[1].strip()
    except:
        return platform.processor()
    return "Unknown CPU"

@app.get("/", response_class=HTMLResponse)
async def read_root(request: Request):
    return templates.TemplateResponse("index.html", {"request": request})

@app.get("/api/info")
async def get_info():
    boot_time_timestamp = psutil.boot_time()
    uptime_seconds = time.time() - boot_time_timestamp
    uptime_string = str(datetime.timedelta(seconds=int(uptime_seconds)))
    
    return {
        "os": f"{distro.name(pretty=True)} {platform.machine()}",
        "kernel": platform.release(),
        "hostname": socket.gethostname(),
        "uptime": uptime_string,
        "cpu_model": get_cpu_model(),
        "cpu_count": psutil.cpu_count(logical=True),
        "memory_total": get_size(psutil.virtual_memory().total)
    }

@app.get("/api/stats")
async def get_stats():
    # CPU
    cpu_percent = psutil.cpu_percent(interval=None)

    cpu_per_core = psutil.cpu_percent(interval=None, percpu=True)
    
    # CPU Freq
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

    # Fans
    fans = []
    try:
        fans_dict = psutil.sensors_fans()
        for name, entries in fans_dict.items():
            for entry in entries:
                # Ignore unreasonable fan speeds (s-tui logic)
                if entry.current > 10000:
                    continue
                fans.append({
                    "label": entry.label or name,
                    "current": entry.current
                })
    except:
        pass

    # Sensors (Temperature)
    sensors = {}
    try:
        temps = psutil.sensors_temperatures()
        if temps:
            for name, entries in temps.items():
                sensors[name] = []
                for entry in entries:
                    sensors[name].append({
                        "label": entry.label or name,
                        "current": entry.current,
                        "high": entry.high,
                        "critical": entry.critical
                    })
    except:
        pass

    # Battery / Power
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

    # RAPL Power
    rapl_stats = rapl_reader.get_power_stats()
    if rapl_stats:
        power_status["rapl"] = rapl_stats
        # Calculate total package power for summary
        total_watts = sum(v for k, v in rapl_stats.items() if "Package" in k)
        if total_watts > 0:
            power_status["consumption_watts"] = total_watts

    # Try to get power consumption (Watts) from /sys/class/power_supply if RAPL failed
    if "consumption_watts" not in power_status:
        power_watts = 0.0
        try:
            base_path = "/sys/class/power_supply"
            if os.path.exists(base_path):
                for supply in os.listdir(base_path):
                    supply_path = os.path.join(base_path, supply)
                    
                    # Try power_now (microWatts)
                    try:
                        with open(os.path.join(supply_path, "power_now"), "r") as f:
                            p_now = int(f.read().strip())
                            power_watts += p_now / 1000000.0
                            continue
                    except:
                        pass
                    
                    # Try voltage_now * current_now
                    try:
                        with open(os.path.join(supply_path, "voltage_now"), "r") as f:
                            v_now = int(f.read().strip())
                        with open(os.path.join(supply_path, "current_now"), "r") as f:
                            c_now = int(f.read().strip())
                        
                        # microvolts * microamps = picowatts. 1W = 10^12 pW
                        power_watts += (v_now * c_now) / 1e12
                    except:
                        pass
        except:
            pass

        if power_watts > 0:
            power_status["consumption_watts"] = round(power_watts, 2)

    # Memory
    svmem = psutil.virtual_memory()
    memory_stats = {
        "total": get_size(svmem.total),
        "available": get_size(svmem.available),
        "used": get_size(svmem.used),
        "percent": svmem.percent
    }

    # Disk
    partitions = psutil.disk_partitions()
    disk_stats = []
    for partition in partitions:
        if 'loop' in partition.device or partition.fstype == 'squashfs':
            continue
        try:
            # If we are in a container with hostfs mounted, we need to adjust the path
            mountpoint = partition.mountpoint
            if os.path.exists("/hostfs"):
                # Remove leading / from mountpoint to join correctly
                # e.g. / -> /hostfs/
                # /home -> /hostfs/home
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

    # Network
    net_io = psutil.net_io_counters()
    network_stats = {
        "bytes_sent": get_size(net_io.bytes_sent),
        "bytes_recv": get_size(net_io.bytes_recv)
    }
    
    # Boot Time
    boot_time_timestamp = psutil.boot_time()
    bt = time.localtime(boot_time_timestamp)
    boot_time = f"{bt.tm_year}/{bt.tm_mon}/{bt.tm_mday} {bt.tm_hour}:{bt.tm_min}:{bt.tm_sec}"

    return {
        "cpu": {
            "percent": cpu_percent,
            "per_core": cpu_per_core,
            "freq": {
                "avg": cpu_freq_avg,
                "per_core": cpu_freq_per_core
            }
        },
        "fans": fans,
        "sensors": sensors,
        "power": power_status,
        "memory": memory_stats,
        "disk": disk_stats,
        "network": network_stats,
        "boot_time": boot_time
    }
