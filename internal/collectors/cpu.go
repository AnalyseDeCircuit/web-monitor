package collectors

import (
	"bufio"
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/AnalyseDeCircuit/web-monitor/internal/config"
	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/load"
)

// CPUCollector 采集 CPU 相关指标
type CPUCollector struct {
	tempHistory []float64
	historyMu   sync.Mutex
}

// CPUData 包含 CPU 采集结果
type CPUData struct {
	Percent     float64
	PerCore     []float64
	Info        types.CPUDetail
	LoadAvg     []float64
	Stats       map[string]uint64
	Times       map[string]float64
	Freq        types.CPUFreq
	TempHistory []float64
}

// NewCPUCollector 创建 CPU 采集器
func NewCPUCollector() *CPUCollector {
	return &CPUCollector{
		tempHistory: make([]float64, 0, 300),
	}
}

func (c *CPUCollector) Name() string {
	return "cpu"
}

func (c *CPUCollector) Collect(ctx context.Context) interface{} {
	data := CPUData{
		Stats: make(map[string]uint64),
		Times: make(map[string]float64),
	}

	// CPU Percent (overall)
	cpuPercent, _ := cpu.Percent(0, false)
	if len(cpuPercent) > 0 {
		data.Percent = utils.Round(cpuPercent[0])
	}

	// Per-core percent
	perCore, _ := cpu.Percent(0, true)
	data.PerCore = make([]float64, len(perCore))
	for i, v := range perCore {
		data.PerCore[i] = utils.Round(v)
	}

	// CPU Info
	data.Info = c.getCPUInfo()

	// Load Average
	if avg, err := load.Avg(); err == nil {
		data.LoadAvg = []float64{
			utils.Round(avg.Load1),
			utils.Round(avg.Load5),
			utils.Round(avg.Load15),
		}
	}

	// CPU Stats
	data.Stats = c.getCPUStats()

	// CPU Times
	if times, err := cpu.Times(false); err == nil && len(times) > 0 {
		t := times[0]
		total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice
		if total <= 0 {
			total = 1
		}
		data.Times = map[string]float64{
			"user":    utils.Round((t.User / total) * 100),
			"system":  utils.Round((t.System / total) * 100),
			"idle":    utils.Round((t.Idle / total) * 100),
			"iowait":  utils.Round((t.Iowait / total) * 100),
			"irq":     utils.Round((t.Irq / total) * 100),
			"softirq": utils.Round((t.Softirq / total) * 100),
		}
	}

	// CPU Frequency
	data.Freq = c.getCPUFreq()

	return data
}

// UpdateTempHistory 更新温度历史记录
func (c *CPUCollector) UpdateTempHistory(currentTemp float64) []float64 {
	c.historyMu.Lock()
	defer c.historyMu.Unlock()

	if len(c.tempHistory) >= 300 {
		copy(c.tempHistory, c.tempHistory[1:])
		c.tempHistory = c.tempHistory[:len(c.tempHistory)-1]
	}
	c.tempHistory = append(c.tempHistory, currentTemp)

	result := make([]float64, len(c.tempHistory))
	copy(result, c.tempHistory)
	return result
}

func (c *CPUCollector) getCPUInfo() types.CPUDetail {
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

	paths := []string{config.HostPath("/proc/cpuinfo"), "/proc/cpuinfo"}
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

func (c *CPUCollector) getCPUStats() map[string]uint64 {
	stats := make(map[string]uint64)
	paths := []string{config.HostPath("/proc/stat"), "/proc/stat"}

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

func (c *CPUCollector) getCPUFreq() types.CPUFreq {
	freq := types.CPUFreq{
		Avg:     0,
		PerCore: []float64{},
	}

	var realFreqs []float64
	paths := []string{config.HostPath("/proc/cpuinfo"), "/proc/cpuinfo"}

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
