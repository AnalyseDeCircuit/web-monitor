// Package websocket 提供WebSocket连接处理功能
package websocket

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/gpu"
	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
	"github.com/AnalyseDeCircuit/web-monitor/internal/network"
	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	gopsutilnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

var (
	cpuTempHistory = make([]float64, 0, 300)
	memHistory     = make([]float64, 0, 300)
	historyMutex   sync.Mutex
)

// Upgrader WebSocket升级器
var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// HandleWebSocket 处理WebSocket连接
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	c, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	intervalStr := r.URL.Query().Get("interval")
	interval, err := strconv.ParseFloat(intervalStr, 64)
	if err != nil || interval < 2.0 {
		interval = 2.0
	}
	if interval > 60 {
		interval = 60
	}

	ticker := time.NewTicker(time.Duration(interval * float64(time.Second)))
	defer ticker.Stop()

	for {
		stats := collectStats()
		err := c.WriteJSON(stats)
		if err != nil {
			log.Println("write:", err)
			break
		}
		<-ticker.C
	}
}

// collectStats 收集系统统计信息
func collectStats() types.Response {
	var resp types.Response

	// CPU
	cpuPercent, _ := cpu.Percent(0, false)
	if len(cpuPercent) > 0 {
		resp.CPU.Percent = utils.Round(cpuPercent[0])
	}

	perCore, _ := cpu.Percent(0, true)
	resp.CPU.PerCore = make([]float64, len(perCore))
	for i, v := range perCore {
		resp.CPU.PerCore[i] = utils.Round(v)
	}

	resp.CPU.Info = getCPUInfo()

	// Load Avg
	if avg, err := load.Avg(); err == nil {
		resp.CPU.LoadAvg = []float64{utils.Round(avg.Load1), utils.Round(avg.Load5), utils.Round(avg.Load15)}
	}

	// CPU Stats
	stats := getCPUStats()
	resp.CPU.Stats = map[string]uint64{
		"ctx_switches":    stats["ctx_switches"],
		"interrupts":      stats["interrupts"],
		"soft_interrupts": stats["soft_interrupts"],
		"syscalls":        stats["syscalls"],
	}

	// CPU Times
	if times, err := cpu.Times(false); err == nil && len(times) > 0 {
		t := times[0]
		total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice
		if total <= 0 {
			total = 1
		}
		resp.CPU.Times = map[string]float64{
			"user":    utils.Round((t.User / total) * 100),
			"system":  utils.Round((t.System / total) * 100),
			"idle":    utils.Round((t.Idle / total) * 100),
			"iowait":  utils.Round((t.Iowait / total) * 100),
			"irq":     utils.Round((t.Irq / total) * 100),
			"softirq": utils.Round((t.Softirq / total) * 100),
		}
	}

	// CPU Freq
	resp.CPU.Freq = getCPUFreq()

	// Sensors
	resp.Sensors = getSensors()

	// Power
	resp.Power = getPower()

	// Update temp history
	historyMutex.Lock()
	currentTemp := 0.0
	if sensors, ok := resp.Sensors.(map[string][]interface{}); ok {
		count := 0.0
		sum := 0.0
		for _, list := range sensors {
			for _, item := range list {
				if m, ok := item.(map[string]interface{}); ok {
					if t, ok := m["current"].(float64); ok && t > 0 {
						sum += t
						count++
					}
				}
			}
		}
		if count > 0 {
			currentTemp = utils.Round(sum / count)
		}
	}

	if len(cpuTempHistory) >= 300 {
		copy(cpuTempHistory, cpuTempHistory[1:])
		cpuTempHistory = cpuTempHistory[:len(cpuTempHistory)-1]
	}
	cpuTempHistory = append(cpuTempHistory, currentTemp)
	resp.CPU.TempHistory = make([]float64, len(cpuTempHistory))
	copy(resp.CPU.TempHistory, cpuTempHistory)
	historyMutex.Unlock()

	// Memory
	v, _ := mem.VirtualMemory()
	resp.Memory = types.MemInfo{
		Total:     utils.GetSize(v.Total),
		Used:      utils.GetSize(v.Used),
		Free:      utils.GetSize(v.Free),
		Percent:   utils.Round(v.UsedPercent),
		Available: utils.GetSize(v.Available),
		Buffers:   utils.GetSize(v.Buffers),
		Cached:    utils.GetSize(v.Cached),
		Shared:    utils.GetSize(v.Shared),
		Active:    utils.GetSize(v.Active),
		Inactive:  utils.GetSize(v.Inactive),
		Slab:      utils.GetSize(v.Slab),
	}

	// Update memory history
	historyMutex.Lock()
	if len(memHistory) >= 300 {
		copy(memHistory, memHistory[1:])
		memHistory = memHistory[:len(memHistory)-1]
	}
	memHistory = append(memHistory, v.UsedPercent)
	resp.Memory.History = make([]float64, len(memHistory))
	copy(resp.Memory.History, memHistory)
	historyMutex.Unlock()

	// Swap
	s, _ := mem.SwapMemory()
	resp.Swap = types.SwapInfo{
		Total:   utils.GetSize(s.Total),
		Used:    utils.GetSize(s.Used),
		Free:    utils.GetSize(s.Free),
		Percent: utils.Round(s.UsedPercent),
		Sin:     utils.GetSize(s.Sin),
		Sout:    utils.GetSize(s.Sout),
	}

	// Disk - use hostfs if available
	useHostfs := false
	if _, err := os.Stat("/hostfs"); err == nil {
		useHostfs = true
	}

	parts, _ := disk.Partitions(false)
	for _, part := range parts {
		if strings.Contains(part.Device, "loop") || part.Fstype == "squashfs" {
			continue
		}

		checkPath := part.Mountpoint
		if useHostfs {
			checkPath = "/hostfs" + part.Mountpoint
		}

		u, err := disk.Usage(checkPath)
		if err == nil {
			resp.Disk = append(resp.Disk, types.DiskInfo{
				Device:     part.Device,
				Mountpoint: part.Mountpoint,
				Fstype:     part.Fstype,
				Total:      utils.GetSize(u.Total),
				Used:       utils.GetSize(u.Used),
				Free:       utils.GetSize(u.Free),
				Percent:    utils.Round(u.UsedPercent),
			})

			// Add inode info
			if u.InodesTotal > 0 {
				resp.Inodes = append(resp.Inodes, types.InodeInfo{
					Mountpoint: part.Mountpoint,
					Total:      u.InodesTotal,
					Used:       u.InodesUsed,
					Free:       u.InodesFree,
					Percent:    utils.Round(u.InodesUsedPercent),
				})
			}
		}
	}

	// Disk IO
	ioCounters, _ := disk.IOCounters()
	if ioCounters != nil {
		resp.DiskIO = make(map[string]types.DiskIOInfo)
		for name, io := range ioCounters {
			resp.DiskIO[name] = types.DiskIOInfo{
				ReadBytes:  utils.GetSize(io.ReadBytes),
				WriteBytes: utils.GetSize(io.WriteBytes),
				ReadCount:  io.ReadCount,
				WriteCount: io.WriteCount,
				ReadTime:   io.ReadTime,
				WriteTime:  io.WriteTime,
			}
		}
	}

	// Network
	netIO, _ := gopsutilnet.IOCounters(false)
	if len(netIO) > 0 {
		resp.Network.BytesSent = utils.GetSize(netIO[0].BytesSent)
		resp.Network.BytesRecv = utils.GetSize(netIO[0].BytesRecv)
		resp.Network.RawSent = netIO[0].BytesSent
		resp.Network.RawRecv = netIO[0].BytesRecv
	}

	// Network detailed info
	if netInfo, err := network.GetNetworkInfo(); err == nil {
		resp.Network.ConnectionStates = netInfo.ConnectionStates
		resp.Network.Sockets = netInfo.Sockets
		resp.Network.Interfaces = netInfo.Interfaces
		resp.Network.Errors = netInfo.Errors
		resp.Network.ListeningPorts = netInfo.ListeningPorts
	}

	// SSH Stats
	resp.SSHStats = network.GetSSHStats()

	// GPU
	resp.GPU = gpu.GetGPUInfo()

	// Processes
	resp.Processes = getAllProcesses()

	// Boot Time
	bootTime, _ := host.BootTime()
	bt := time.Unix(int64(bootTime), 0)
	resp.BootTime = bt.Format("2006/01/02 15:04:05")

	// Check alerts
	monitoring.CheckAlerts(resp.CPU.Percent, resp.Memory.Percent, 0)

	return resp
}

// getCPUInfo 获取CPU信息
func getCPUInfo() types.CPUDetail {
	info := types.CPUDetail{
		Model:        "Unknown",
		Architecture: runtime.GOARCH,
		Cores:        0,
		Threads:      0,
		MaxFreq:      0,
		MinFreq:      0,
	}

	info.Cores, _ = cpu.Counts(false)
	info.Threads, _ = cpu.Counts(true)

	// Read CPU model from /proc/cpuinfo
	paths := []string{"/hostfs/proc/cpuinfo", "/proc/cpuinfo"}
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "model name") {
					parts := strings.Split(line, ":")
					if len(parts) > 1 {
						info.Model = strings.TrimSpace(parts[1])
						file.Close()
						return info
					}
				}
			}
			file.Close()
			break
		}
	}

	return info
}

// getAllProcesses 获取所有进程
func getAllProcesses() []types.ProcessInfo {
	procs, err := process.Processes()
	if err != nil {
		return []types.ProcessInfo{}
	}

	var result []types.ProcessInfo
	for _, p := range procs {
		name, _ := p.Name()
		username, _ := p.Username()
		if username == "" {
			if uids, err := p.Uids(); err == nil && len(uids) > 0 {
				username = fmt.Sprintf("uid:%d", uids[0])
			} else {
				username = "unknown"
			}
		}
		numThreads, _ := p.NumThreads()
		memPercent, _ := p.MemoryPercent()
		cpuPercent, _ := p.CPUPercent()
		ppid, _ := p.Ppid()
		createTime, _ := p.CreateTime() // ms

		uptimeSec := time.Now().Unix() - (createTime / 1000)
		uptimeStr := "-"
		if uptimeSec < 60 {
			uptimeStr = fmt.Sprintf("%ds", uptimeSec)
		} else if uptimeSec < 3600 {
			uptimeStr = fmt.Sprintf("%dm", uptimeSec/60)
		} else if uptimeSec < 86400 {
			uptimeStr = fmt.Sprintf("%dh", uptimeSec/3600)
		} else {
			uptimeStr = fmt.Sprintf("%dd", uptimeSec/86400)
		}

		cmdline, _ := p.Cmdline()
		cwd, _ := p.Cwd()
		if cwd == "" {
			cwd = "-"
		}

		ioRead := "-"
		ioWrite := "-"
		if ioCounters, err := p.IOCounters(); err == nil {
			ioRead = utils.GetSize(ioCounters.ReadBytes)
			ioWrite = utils.GetSize(ioCounters.WriteBytes)
		}

		result = append(result, types.ProcessInfo{
			PID:           p.Pid,
			Name:          name,
			Username:      username,
			NumThreads:    numThreads,
			MemoryPercent: utils.Round(float64(memPercent)),
			CPUPercent:    utils.Round(cpuPercent),
			PPID:          ppid,
			Uptime:        uptimeStr,
			Cmdline:       cmdline,
			Cwd:           cwd,
			IORead:        ioRead,
			IOWrite:       ioWrite,
		})
	}

	// Sort by memory percent desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].MemoryPercent > result[j].MemoryPercent
	})

	// Return all processes
	return result
}

func getCPUStats() map[string]uint64 {
	stats := make(map[string]uint64)
	paths := []string{"/hostfs/proc/stat", "/proc/stat"}

	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				parts := strings.Fields(line)
				if len(parts) < 2 {
					continue
				}

				key := parts[0]
				val, _ := strconv.ParseUint(parts[1], 10, 64)

				switch key {
				case "ctxt":
					stats["ctx_switches"] = val
				case "intr":
					stats["interrupts"] = val
				case "softirq":
					stats["soft_interrupts"] = val
				}
			}
			break
		}
	}
	stats["syscalls"] = 0
	return stats
}

func getCPUFreq() types.CPUFreq {
	freq := types.CPUFreq{
		Avg:     0,
		PerCore: []float64{},
	}

	var realFreqs []float64
	paths := []string{"/hostfs/proc/cpuinfo", "/proc/cpuinfo"}

	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "cpu MHz") {
					parts := strings.Split(line, ":")
					if len(parts) > 1 {
						valStr := strings.TrimSpace(parts[1])
						val, err := strconv.ParseFloat(valStr, 64)
						if err == nil {
							realFreqs = append(realFreqs, utils.Round(val))
						}
					}
				}
			}
			file.Close()
			break
		}
	}

	if len(realFreqs) > 0 {
		freq.PerCore = realFreqs
		sum := 0.0
		for _, f := range realFreqs {
			sum += f
		}
		freq.Avg = utils.Round(sum / float64(len(realFreqs)))
	}

	return freq
}

func getSensors() interface{} {
	sensors := make(map[string][]interface{})
	if temps, err := host.SensorsTemperatures(); err == nil {
		for _, t := range temps {
			sensors[t.SensorKey] = append(sensors[t.SensorKey], map[string]interface{}{
				"label":    t.SensorKey,
				"current":  t.Temperature,
				"high":     t.High,
				"critical": t.Critical,
			})
		}
	}
	return sensors
}

func getPower() interface{} {
	powerStatus := make(map[string]interface{})

	// Battery
	basePath := "/sys/class/power_supply"
	if _, err := os.Stat(basePath); err == nil {
		files, _ := os.ReadDir(basePath)
		for _, f := range files {
			supplyPath := filepath.Join(basePath, f.Name())

			// Try power_now
			if content, err := os.ReadFile(filepath.Join(supplyPath, "power_now")); err == nil {
				if pNow, err := strconv.ParseFloat(strings.TrimSpace(string(content)), 64); err == nil {
					powerStatus["power_watts"] = utils.Round(pNow / 1000000)
				}
			}

			// Try voltage_now
			if content, err := os.ReadFile(filepath.Join(supplyPath, "voltage_now")); err == nil {
				if vNow, err := strconv.ParseFloat(strings.TrimSpace(string(content)), 64); err == nil {
					powerStatus["voltage_volts"] = utils.Round(vNow / 1000000)
				}
			}

			// Try current_now
			if content, err := os.ReadFile(filepath.Join(supplyPath, "current_now")); err == nil {
				if cNow, err := strconv.ParseFloat(strings.TrimSpace(string(content)), 64); err == nil {
					powerStatus["current_amps"] = utils.Round(cNow / 1000000)
				}
			}
		}
	}

	return powerStatus
}
