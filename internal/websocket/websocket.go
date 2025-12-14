// Package websocket 提供WebSocket连接处理功能
package websocket

import (
	"log"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
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

	// Disk
	parts, _ := disk.Partitions(false)
	for _, part := range parts {
		if strings.Contains(part.Device, "loop") || part.Fstype == "squashfs" {
			continue
		}

		u, err := disk.Usage(part.Mountpoint)
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
		}
	}

	// Network
	netIO, _ := net.IOCounters(false)
	if len(netIO) > 0 {
		resp.Network.BytesSent = utils.GetSize(netIO[0].BytesSent)
		resp.Network.BytesRecv = utils.GetSize(netIO[0].BytesRecv)
		resp.Network.RawSent = netIO[0].BytesSent
		resp.Network.RawRecv = netIO[0].BytesRecv
	}

	// Processes
	resp.Processes = getTopProcesses()

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

	return info
}

// getTopProcesses 获取顶级进程
func getTopProcesses() []types.ProcessInfo {
	procs, err := process.Processes()
	if err != nil {
		return []types.ProcessInfo{}
	}

	var result []types.ProcessInfo
	for _, p := range procs {
		name, _ := p.Name()
		username, _ := p.Username()
		memPercent, _ := p.MemoryPercent()
		cpuPercent, _ := p.CPUPercent()

		result = append(result, types.ProcessInfo{
			PID:           p.Pid,
			Name:          name,
			Username:      username,
			MemoryPercent: utils.Round(float64(memPercent)),
			CPUPercent:    utils.Round(cpuPercent),
		})
	}

	// Sort by memory percent desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].MemoryPercent > result[j].MemoryPercent
	})

	if len(result) > 20 {
		return result[:20]
	}
	return result
}
