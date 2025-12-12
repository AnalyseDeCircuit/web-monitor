package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	stdnet "net"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// --- Structs matching JSON response ---

type Response struct {
	CPU       CPUInfo               `json:"cpu"`
	Fans      []interface{}         `json:"fans"`
	Sensors   interface{}           `json:"sensors"`
	Power     interface{}           `json:"power"`
	Memory    MemInfo               `json:"memory"`
	Swap      SwapInfo              `json:"swap"`
	Disk      []DiskInfo            `json:"disk"`
	DiskIO    map[string]DiskIOInfo `json:"disk_io"`
	Inodes    []InodeInfo           `json:"inodes"`
	Network   NetInfo               `json:"network"`
	SSHStats  SSHStats              `json:"ssh_stats"`
	BootTime  string                `json:"boot_time"`
	Processes []ProcessInfo         `json:"processes"`
	GPU       []GPUDetail           `json:"gpu"`
}

type GPUDetail struct {
	Index       int          `json:"index"`
	Name        string       `json:"name"`
	Vendor      string       `json:"vendor"`
	PCIAddress  string       `json:"pci_address"`
	DRMCard     string       `json:"drm_card"`
	VRAMTotal   string       `json:"vram_total"`
	VRAMUsed    string       `json:"vram_used"`
	VRAMPercent float64      `json:"vram_percent"`
	FreqMHz     float64      `json:"freq_mhz"`
	TempC       float64      `json:"temp_c"`
	PowerW      float64      `json:"power_w"`
	LoadPercent float64      `json:"load_percent"`
	Processes   []GPUProcess `json:"processes"`
}

type GPUProcess struct {
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	VRAMUsed string `json:"vram_used"`
}

type CPUInfo struct {
	Percent     float64     `json:"percent"`
	PerCore     []float64   `json:"per_core"`
	Times       interface{} `json:"times"`
	LoadAvg     []float64   `json:"load_avg"`
	Stats       interface{} `json:"stats"`
	Freq        CPUFreq     `json:"freq"`
	Info        CPUDetail   `json:"info"`
	TempHistory []float64   `json:"temp_history"`
}

type CPUFreq struct {
	Avg     float64   `json:"avg"`
	PerCore []float64 `json:"per_core"`
}

type CPUDetail struct {
	Model        string  `json:"model"`
	Architecture string  `json:"architecture"`
	Cores        int     `json:"cores"`
	Threads      int     `json:"threads"`
	MaxFreq      float64 `json:"max_freq"`
	MinFreq      float64 `json:"min_freq"`
}

type MemInfo struct {
	Total     string    `json:"total"`
	Used      string    `json:"used"`
	Free      string    `json:"free"`
	Percent   float64   `json:"percent"`
	Available string    `json:"available"`
	Buffers   string    `json:"buffers"`
	Cached    string    `json:"cached"`
	Shared    string    `json:"shared"`
	Active    string    `json:"active"`
	Inactive  string    `json:"inactive"`
	Slab      string    `json:"slab"`
	History   []float64 `json:"history"`
}

type SwapInfo struct {
	Total   string  `json:"total"`
	Used    string  `json:"used"`
	Free    string  `json:"free"`
	Percent float64 `json:"percent"`
	Sin     string  `json:"sin"`
	Sout    string  `json:"sout"`
}

type DiskInfo struct {
	Device     string  `json:"device"`
	Mountpoint string  `json:"mountpoint"`
	Fstype     string  `json:"fstype"`
	Total      string  `json:"total"`
	Used       string  `json:"used"`
	Free       string  `json:"free"`
	Percent    float64 `json:"percent"`
}

type DiskIOInfo struct {
	ReadBytes  string `json:"read_bytes"`
	WriteBytes string `json:"write_bytes"`
	ReadCount  uint64 `json:"read_count"`
	WriteCount uint64 `json:"write_count"`
	ReadTime   uint64 `json:"read_time"`
	WriteTime  uint64 `json:"write_time"`
}

type InodeInfo struct {
	Mountpoint string  `json:"mountpoint"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Free       uint64  `json:"free"`
	Percent    float64 `json:"percent"`
}

type NetInfo struct {
	BytesSent        string               `json:"bytes_sent"`
	BytesRecv        string               `json:"bytes_recv"`
	RawSent          uint64               `json:"raw_sent"`
	RawRecv          uint64               `json:"raw_recv"`
	Interfaces       map[string]Interface `json:"interfaces"`
	Sockets          map[string]int       `json:"sockets"`
	ConnectionStates map[string]int       `json:"connection_states"`
	Errors           map[string]uint64    `json:"errors"`
	ListeningPorts   []ListeningPort      `json:"listening_ports"`
}

type Interface struct {
	IP        string  `json:"ip"`
	BytesSent string  `json:"bytes_sent"`
	BytesRecv string  `json:"bytes_recv"`
	Speed     float64 `json:"speed"`
	IsUp      bool    `json:"is_up"`
	ErrorsIn  uint64  `json:"errors_in"`
	ErrorsOut uint64  `json:"errors_out"`
	DropsIn   uint64  `json:"drops_in"`
	DropsOut  uint64  `json:"drops_out"`
}

type ListeningPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type SSHStats struct {
	Status           string         `json:"status"`
	Connections      int            `json:"connections"`
	Sessions         []interface{}  `json:"sessions"`
	AuthMethods      map[string]int `json:"auth_methods"`
	HostKey          string         `json:"hostkey_fingerprint"`
	HistorySize      int            `json:"history_size"`
	OOMRiskProcesses []ProcessInfo  `json:"oom_risk_processes"`
	FailedLogins     int            `json:"failed_logins"`
	SSHProcessMemory float64        `json:"ssh_process_memory"`
}

type ProcessInfo struct {
	PID           int32         `json:"pid"`
	Name          string        `json:"name"`
	Username      string        `json:"username"`
	NumThreads    int32         `json:"num_threads"`
	MemoryPercent float64       `json:"memory_percent"`
	CPUPercent    float64       `json:"cpu_percent"`
	PPID          int32         `json:"ppid"`
	Uptime        string        `json:"uptime"`
	Cmdline       string        `json:"cmdline"`
	Cwd           string        `json:"cwd"`
	IORead        string        `json:"io_read"`
	IOWrite       string        `json:"io_write"`
	Children      []ProcessInfo `json:"children,omitempty"`
}

// --- Global Caches ---

var (
	cpuTempHistory = make([]float64, 0, 300)
	memHistory     = make([]float64, 0, 300)
	historyMutex   sync.Mutex

	processCache     []ProcessInfo
	lastProcessTime  time.Time
	processCacheLock sync.Mutex

	gpuInfoCache    string
	lastGPUInfoTime time.Time
	gpuInfoLock     sync.Mutex

	connStatesCache    map[string]int
	lastConnStatesTime time.Time
	connStatesLock     sync.Mutex

	sshStatsCache    SSHStats
	lastSSHTime      time.Time
	sshAuthLogOffset int64
	sshAuthCounters  = map[string]int{"publickey": 0, "password": 0, "other": 0, "failed": 0}
	sshStatsLock     sync.Mutex

	// RAPL Cache
	raplReadings = make(map[string]uint64)
	raplTime     time.Time
	raplLock     sync.Mutex
)

// --- Helper Functions ---

func round(val float64) float64 {
	return math.Round(val*100) / 100
}

func getSize(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatUptime(sec uint64) string {
	days := sec / 86400
	sec %= 86400
	hours := sec / 3600
	sec %= 3600
	mins := sec / 60
	secs := sec % 60
	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 || len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 || len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	parts = append(parts, fmt.Sprintf("%ds", secs))
	return strings.Join(parts, " ")
}

func detectOSName() string {
	paths := []string{"/hostfs/etc/os-release", "/etc/os-release"}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		m := make(map[string]string)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if idx := strings.Index(line, "="); idx > 0 {
				key := line[:idx]
				val := strings.Trim(line[idx+1:], "\"")
				m[key] = val
			}
		}
		if v, ok := m["PRETTY_NAME"]; ok && v != "" {
			return v
		}
		name := m["NAME"]
		ver := m["VERSION"]
		if name != "" && ver != "" {
			return fmt.Sprintf("%s %s", name, ver)
		}
		if name != "" {
			return name
		}
	}
	return ""
}

func getCPUInfo() CPUDetail {
	info := CPUDetail{
		Model:        "Unknown",
		Architecture: runtime.GOARCH,
		Cores:        0,
		Threads:      0,
		MaxFreq:      0,
		MinFreq:      0,
	}

	// Try to read model from /proc/cpuinfo
	if file, err := os.Open("/proc/cpuinfo"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "model name") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					info.Model = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	info.Cores, _ = cpu.Counts(false)
	info.Threads, _ = cpu.Counts(true)

	// Get frequency info
	if freqs, err := cpu.Info(); err == nil && len(freqs) > 0 {
		info.MaxFreq = freqs[0].Mhz
	}

	return info
}

func lookupPCIName(vendorID, deviceID string) string {
	// Strip 0x prefix
	vendorID = strings.TrimPrefix(vendorID, "0x")
	deviceID = strings.TrimPrefix(deviceID, "0x")

	// Common locations for pci.ids
	paths := []string{
		"/usr/share/hwdata/pci.ids",
		"/usr/share/pci.ids",
		"/usr/share/misc/pci.ids",
	}

	var file *os.File
	var err error
	for _, path := range paths {
		file, err = os.Open(path)
		if err == nil {
			break
		}
	}
	if file == nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inVendor := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		if !strings.HasPrefix(line, "\t") {
			// Vendor line: "1234  Vendor Name"
			if strings.HasPrefix(line, vendorID) {
				inVendor = true
			} else {
				inVendor = false
			}
		} else if inVendor && strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t") {
			// Device line: "\t5678  Device Name"
			trimmed := strings.TrimPrefix(line, "\t")
			if strings.HasPrefix(trimmed, deviceID) {
				// Found it. Extract name.
				// Usually "ID  Name" (two spaces)
				if len(trimmed) > len(deviceID) {
					return strings.TrimSpace(trimmed[len(deviceID):])
				}
			}
		}
	}
	return ""
}

func parseBytes(s string) uint64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func getGPUDetails() []GPUDetail {
	var details []GPUDetail
	matches, _ := filepath.Glob("/sys/class/drm/card*")
	sort.Strings(matches)

	for i, cardPath := range matches {
		// Skip virtual devices if they don't have a physical device link
		// But for some iGPUs, the link might be different.
		// Let's try to read vendor/device directly first.

		vendorFile := filepath.Join(cardPath, "device/vendor")
		deviceFile := filepath.Join(cardPath, "device/device")

		// If device/vendor doesn't exist, try direct vendor (some drivers expose it differently)
		if _, err := os.Stat(vendorFile); os.IsNotExist(err) {
			vendorFile = filepath.Join(cardPath, "vendor")
			deviceFile = filepath.Join(cardPath, "device")
		}

		vendorBytes, err1 := ioutil.ReadFile(vendorFile)
		deviceBytes, err2 := ioutil.ReadFile(deviceFile)

		if err1 == nil && err2 == nil {
			vendor := strings.ToLower(strings.TrimSpace(string(vendorBytes)))
			device := strings.ToLower(strings.TrimSpace(string(deviceBytes)))

			// Ensure we have valid hex IDs
			if !strings.HasPrefix(vendor, "0x") {
				vendor = "0x" + vendor
			}
			if !strings.HasPrefix(device, "0x") {
				device = "0x" + device
			}

			realName := lookupPCIName(vendor, device)
			var gpuName string
			if realName != "" {
				lowerName := strings.ToLower(realName)
				if vendor == "0x8086" && !strings.Contains(lowerName, "intel") {
					gpuName = "Intel " + realName
				} else if vendor == "0x10de" && !strings.Contains(lowerName, "nvidia") {
					gpuName = "NVIDIA " + realName
				} else if vendor == "0x1002" && !strings.Contains(lowerName, "amd") {
					gpuName = "AMD " + realName
				} else {
					gpuName = realName
				}
			} else {
				gpuName = fmt.Sprintf("Unknown [%s:%s]", vendor, device)
			}

			pciAddr := ""
			if link, err := os.Readlink(filepath.Join(cardPath, "device")); err == nil {
				pciAddr = filepath.Base(link)
			}

			var vramTotal, vramUsed uint64
			// Intel i915 specific paths
			if content, err := ioutil.ReadFile(filepath.Join(cardPath, "gt/gt0/mem_info_vram_total")); err == nil {
				vramTotal = parseBytes(string(content))
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/mem_info_vram_total")); err == nil {
				vramTotal = parseBytes(string(content))
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/drm/card0/gt/gt0/mem_info_vram_total")); err == nil {
				// Try deeper path for some setups
				vramTotal = parseBytes(string(content))
			}

			if content, err := ioutil.ReadFile(filepath.Join(cardPath, "gt/gt0/mem_info_vram_used")); err == nil {
				vramUsed = parseBytes(string(content))
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/mem_info_vram_used")); err == nil {
				vramUsed = parseBytes(string(content))
			}

			var freq float64
			if content, err := ioutil.ReadFile(filepath.Join(cardPath, "gt_act_freq_mhz")); err == nil {
				freq = parseFloat(string(content))
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/pp_dpm_sclk")); err == nil {
				lines := strings.Split(string(content), "\n")
				for _, line := range lines {
					if strings.Contains(line, "*") {
						parts := strings.Fields(line)
						if len(parts) >= 2 {
							valStr := strings.TrimSuffix(parts[1], "Mhz")
							freq = parseFloat(valStr)
						}
						break
					}
				}
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "gt_cur_freq_mhz")); err == nil {
				freq = parseFloat(string(content))
			}

			var temp float64
			hwmonGlob, _ := filepath.Glob(filepath.Join(cardPath, "device/hwmon/hwmon*"))
			for _, hwmon := range hwmonGlob {
				if content, err := ioutil.ReadFile(filepath.Join(hwmon, "temp1_input")); err == nil {
					temp = parseFloat(string(content)) / 1000
					break
				}
			}

			var power float64
			for _, hwmon := range hwmonGlob {
				if content, err := ioutil.ReadFile(filepath.Join(hwmon, "power1_average")); err == nil {
					power = parseFloat(string(content)) / 1000000
					break
				}
			}

			var loadVal float64
			if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/gpu_busy_percent")); err == nil {
				loadVal = parseFloat(string(content))
			}

			detail := GPUDetail{
				Index:       i,
				Name:        gpuName,
				Vendor:      vendor,
				PCIAddress:  pciAddr,
				DRMCard:     filepath.Base(cardPath),
				VRAMTotal:   getSize(vramTotal),
				VRAMUsed:    getSize(vramUsed),
				FreqMHz:     freq,
				TempC:       temp,
				PowerW:      power,
				LoadPercent: loadVal,
			}
			if vramTotal > 0 {
				detail.VRAMPercent = round(float64(vramUsed) / float64(vramTotal) * 100)
			}

			details = append(details, detail)
		} else {
			fmt.Printf("Failed to read vendor/device for %s: %v, %v\n", cardPath, err1, err2)
		}
	}
	return details
}

func getGPUInfo() string {
	gpuInfoLock.Lock()
	defer gpuInfoLock.Unlock()

	if time.Since(lastGPUInfoTime) < 60*time.Second && gpuInfoCache != "" {
		return gpuInfoCache
	}

	var gpus []string
	seen := make(map[string]bool)

	matches, _ := filepath.Glob("/sys/class/drm/card*")
	for _, cardPath := range matches {
		vendorFile := filepath.Join(cardPath, "device/vendor")
		deviceFile := filepath.Join(cardPath, "device/device")

		vendorBytes, err1 := ioutil.ReadFile(vendorFile)
		deviceBytes, err2 := ioutil.ReadFile(deviceFile)

		if err1 == nil && err2 == nil {
			vendor := strings.ToLower(strings.TrimSpace(string(vendorBytes)))
			device := strings.ToLower(strings.TrimSpace(string(deviceBytes)))

			// Deduplicate based on vendor+device ID
			key := vendor + ":" + device
			if seen[key] {
				continue
			}
			seen[key] = true

			// Try to lookup real name
			realName := lookupPCIName(vendor, device)
			var gpuName string

			if realName != "" {
				// Ensure vendor name is present for clarity
				lowerName := strings.ToLower(realName)
				if vendor == "0x8086" && !strings.Contains(lowerName, "intel") {
					gpuName = "Intel " + realName
				} else if vendor == "0x10de" && !strings.Contains(lowerName, "nvidia") {
					gpuName = "NVIDIA " + realName
				} else if vendor == "0x1002" && !strings.Contains(lowerName, "amd") {
					gpuName = "AMD " + realName
				} else {
					gpuName = realName
				}
			} else {
				// Fallback
				switch vendor {
				case "0x8086":
					gpuName = fmt.Sprintf("Intel [%s]", device)
				case "0x10de":
					gpuName = fmt.Sprintf("NVIDIA [%s]", device)
				case "0x1002":
					gpuName = fmt.Sprintf("AMD [%s]", device)
				default:
					gpuName = fmt.Sprintf("Generic [%s:%s]", vendor, device)
				}
			}
			gpus = append(gpus, gpuName)
		}
	}

	if len(gpus) == 0 {
		gpuInfoCache = "Unknown GPU"
	} else {
		gpuInfoCache = strings.Join(gpus, " + ")
	}

	lastGPUInfoTime = time.Now()
	return gpuInfoCache
}

func getCPUStats() map[string]uint64 {
	stats := make(map[string]uint64)
	if file, err := os.Open("/proc/stat"); err == nil {
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
	}
	// syscalls is not typically in /proc/stat on Linux, usually 0 or requires other source
	stats["syscalls"] = 0
	return stats
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
		files, _ := ioutil.ReadDir(basePath)
		for _, f := range files {
			supplyPath := filepath.Join(basePath, f.Name())

			// Try power_now
			if content, err := ioutil.ReadFile(filepath.Join(supplyPath, "power_now")); err == nil {
				if pNow, err := strconv.ParseFloat(strings.TrimSpace(string(content)), 64); err == nil {
					powerStatus["consumption_watts"] = round(pNow / 1000000.0)
					break
				}
			}

			// Try voltage_now * current_now
			vBytes, err1 := ioutil.ReadFile(filepath.Join(supplyPath, "voltage_now"))
			cBytes, err2 := ioutil.ReadFile(filepath.Join(supplyPath, "current_now"))
			if err1 == nil && err2 == nil {
				vNow, _ := strconv.ParseFloat(strings.TrimSpace(string(vBytes)), 64)
				cNow, _ := strconv.ParseFloat(strings.TrimSpace(string(cBytes)), 64)
				powerStatus["consumption_watts"] = round((vNow * cNow) / 1e12)
				break
			}
		}
	}

	// RAPL (Intel Power)
	raplLock.Lock()
	defer raplLock.Unlock()

	raplBasePath := "/sys/class/powercap"
	now := time.Now()

	// Find RAPL domains
	if matches, err := filepath.Glob(filepath.Join(raplBasePath, "intel-rapl:*")); err == nil {
		totalWatts := 0.0
		hasNewReading := false

		for _, domainPath := range matches {
			// Check if it's a package domain
			nameFile := filepath.Join(domainPath, "name")
			if nameBytes, err := ioutil.ReadFile(nameFile); err == nil {
				name := strings.TrimSpace(string(nameBytes))
				// We typically care about "package-X" for total CPU power
				// But let's just sum up everything that looks like a package or dram if we want detailed
				// The Python code sums up "Package" domains for total.

				energyFile := filepath.Join(domainPath, "energy_uj")
				maxEnergyFile := filepath.Join(domainPath, "max_energy_range_uj")

				if energyBytes, err := ioutil.ReadFile(energyFile); err == nil {
					energyUj, _ := strconv.ParseUint(strings.TrimSpace(string(energyBytes)), 10, 64)

					// Calculate watts if we have previous reading
					if lastEnergy, ok := raplReadings[domainPath]; ok && !raplTime.IsZero() {
						dt := now.Sub(raplTime).Seconds()
						if dt > 0 {
							// Handle overflow/reset
							var de uint64
							if energyUj >= lastEnergy {
								de = energyUj - lastEnergy
							} else {
								// Wrapped around
								maxRange := uint64(0)
								if maxBytes, err := ioutil.ReadFile(maxEnergyFile); err == nil {
									maxRange, _ = strconv.ParseUint(strings.TrimSpace(string(maxBytes)), 10, 64)
								}
								if maxRange > 0 {
									de = (maxRange - lastEnergy) + energyUj
								} else {
									de = 0 // Can't calculate
								}
							}

							watts := (float64(de) / 1000000.0) / dt

							if strings.HasPrefix(name, "package") || strings.HasPrefix(name, "dram") {
								// Add to detailed map if we want to return it
								// But for now, let's just update total consumption if not already set by battery
								if strings.HasPrefix(name, "package") {
									totalWatts += watts
								}
							}
						}
					}

					raplReadings[domainPath] = energyUj
					hasNewReading = true
				}
			}
		}

		if hasNewReading {
			raplTime = now
			if totalWatts > 0 {
				// Prefer RAPL over battery if available (or sum them? usually one or other)
				// If we already have battery, maybe this is desktop and battery is UPS?
				// Usually RAPL is more accurate for CPU power.
				// Let's just overwrite or add? Python code:
				// if total_watts > 0: power_status["consumption_watts"] = total_watts
				powerStatus["consumption_watts"] = round(totalWatts)
			}
		}
	}

	return powerStatus
}

func getTopProcesses() []ProcessInfo {
	processCacheLock.Lock()
	defer processCacheLock.Unlock()

	if time.Since(lastProcessTime) < 15*time.Second && processCache != nil {
		return processCache
	}

	procs, err := process.Processes()
	if err != nil {
		return []ProcessInfo{}
	}

	var result []ProcessInfo
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

		result = append(result, ProcessInfo{
			PID:           p.Pid,
			Name:          name,
			Username:      username,
			NumThreads:    numThreads,
			MemoryPercent: round(float64(memPercent)),
			CPUPercent:    round(cpuPercent),
			PPID:          ppid,
			Uptime:        uptimeStr,
			Cmdline:       cmdline,
			Cwd:           "-",
			IORead:        "-",
			IOWrite:       "-",
		})
	}

	// Sort by memory percent desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].MemoryPercent > result[j].MemoryPercent
	})

	processCache = result
	lastProcessTime = time.Now()
	return result
}

func getConnectionStates() map[string]int {
	connStatesLock.Lock()
	defer connStatesLock.Unlock()

	if time.Since(lastConnStatesTime) < 10*time.Second && connStatesCache != nil {
		return connStatesCache
	}

	states := map[string]int{
		"ESTABLISHED": 0, "SYN_SENT": 0, "SYN_RECV": 0, "FIN_WAIT1": 0,
		"FIN_WAIT2": 0, "TIME_WAIT": 0, "CLOSE": 0, "CLOSE_WAIT": 0,
		"LAST_ACK": 0, "LISTEN": 0, "CLOSING": 0,
	}

	conns, err := net.Connections("tcp")
	if err == nil {
		for _, c := range conns {
			if _, ok := states[c.Status]; ok {
				states[c.Status]++
			}
		}
	}

	connStatesCache = states
	lastConnStatesTime = time.Now()
	return states
}

func getListeningPorts() []ListeningPort {
	portsMap := make(map[int]map[string]string) // port -> protocol -> process

	conns, err := net.Connections("all")
	if err == nil {
		for _, c := range conns {
			if c.Status == "LISTEN" {
				port := int(c.Laddr.Port)
				proto := "TCP"
				if c.Type == 2 {
					proto = "UDP"
				}

				if _, ok := portsMap[port]; !ok {
					portsMap[port] = make(map[string]string)
				}

				procName := fmt.Sprintf("PID %d", c.Pid)
				if p, err := process.NewProcess(c.Pid); err == nil {
					if name, err := p.Name(); err == nil {
						procName = name
					}
				}
				portsMap[port][strings.ToLower(proto)] = procName
			}
		}
	}

	var result []ListeningPort
	for port, protos := range portsMap {
		var protoStrs []string
		if name, ok := protos["tcp"]; ok {
			protoStrs = append(protoStrs, fmt.Sprintf("TCP:%s", name))
		}
		if name, ok := protos["udp"]; ok {
			protoStrs = append(protoStrs, fmt.Sprintf("UDP:%s", name))
		}

		result = append(result, ListeningPort{
			Port:     port,
			Protocol: strings.Join(protoStrs, ", "),
		})
	}

	// Sort by port
	sort.Slice(result, func(i, j int) bool {
		return result[i].Port < result[j].Port
	})

	if len(result) > 20 {
		return result[:20]
	}
	return result
}

func getSSHStats() SSHStats {
	sshStatsLock.Lock()
	defer sshStatsLock.Unlock()

	if time.Since(lastSSHTime) < 120*time.Second && sshStatsCache.Status != "" {
		return sshStatsCache
	}

	stats := SSHStats{
		Status:       "Stopped",
		Connections:  0,
		Sessions:     []interface{}{},
		AuthMethods:  map[string]int{"password": 0, "publickey": 0, "other": 0},
		FailedLogins: 0,
		HostKey:      "-",
		HistorySize:  0,
	}

	// 1. Check SSH port status
	conns, _ := net.Connections("tcp")
	for _, c := range conns {
		if c.Laddr.Port == 22 {
			if c.Status == "LISTEN" {
				stats.Status = "Running"
			} else if c.Status == "ESTABLISHED" {
				stats.Connections++
			}
		}
	}

	// Use `who` for sessions
	if out, err := exec.Command("who").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) >= 5 {
				user := parts[0]
				ip := parts[len(parts)-1]
				if strings.HasPrefix(ip, "(") && strings.HasSuffix(ip, ")") {
					ip = ip[1 : len(ip)-1]
					// Filter out local X11 or tmux if they don't look like IPs
					if stdnet.ParseIP(ip) != nil {
						startedStr := parts[2] + " " + parts[3]
						// Try to parse time and convert to ISO 8601 (UTC)
						// who output format: YYYY-MM-DD HH:MM
						if t, err := time.ParseInLocation("2006-01-02 15:04", startedStr, time.Local); err == nil {
							startedStr = t.UTC().Format(time.RFC3339)
						}

						stats.Sessions = append(stats.Sessions, map[string]string{
							"user":    user,
							"ip":      ip,
							"started": startedStr,
						})
					}
				}
			}
		}
	}

	// 2. Host Key
	keyPaths := []string{"/etc/ssh/ssh_host_rsa_key.pub", "/hostfs/etc/ssh/ssh_host_rsa_key.pub"}
	for _, path := range keyPaths {
		if content, err := ioutil.ReadFile(path); err == nil {
			parts := strings.Fields(string(content))
			if len(parts) >= 2 {
				stats.HostKey = parts[1]
				break
			}
		}
	}

	// 3. Auth Logs (Incremental)
	logPaths := []string{"/var/log/auth.log", "/hostfs/var/log/auth.log", "/var/log/secure", "/hostfs/var/log/secure"}
	var authLogPath string
	for _, path := range logPaths {
		if _, err := os.Stat(path); err == nil {
			authLogPath = path
			break
		}
	}

	if authLogPath != "" {
		file, err := os.Open(authLogPath)
		if err == nil {
			stat, _ := file.Stat()
			fileSize := stat.Size()

			if sshAuthLogOffset > 0 && sshAuthLogOffset <= fileSize {
				file.Seek(sshAuthLogOffset, 0)
			} else {
				start := fileSize - 10000
				if start < 0 {
					start = 0
				}
				file.Seek(start, 0)
			}

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if !strings.Contains(line, "sshd") {
					continue
				}

				if strings.Contains(line, "Failed password") || strings.Contains(line, "Invalid user") || strings.Contains(line, "authentication failure") {
					sshAuthCounters["failed"]++
				} else if strings.Contains(line, "Accepted publickey") {
					sshAuthCounters["publickey"]++
				} else if strings.Contains(line, "Accepted password") {
					sshAuthCounters["password"]++
				} else if strings.Contains(line, "Accepted") {
					sshAuthCounters["other"]++
				}
			}
			sshAuthLogOffset, _ = file.Seek(0, 1) // Current position
			file.Close()
		}
	}

	stats.AuthMethods["publickey"] = sshAuthCounters["publickey"]
	stats.AuthMethods["password"] = sshAuthCounters["password"]
	stats.AuthMethods["other"] = sshAuthCounters["other"]
	stats.FailedLogins = sshAuthCounters["failed"]

	// 4. History Size (known_hosts)
	knownHostsPaths := []string{
		"/root/.ssh/known_hosts",
		"/hostfs/root/.ssh/known_hosts",
		os.ExpandEnv("$HOME/.ssh/known_hosts"),
	}
	// Also try common user home locations on host
	globPatterns := []string{
		"/home/*/.ssh/known_hosts",
		"/hostfs/home/*/.ssh/known_hosts",
	}
	for _, pattern := range globPatterns {
		if matches, err := filepath.Glob(pattern); err == nil {
			knownHostsPaths = append(knownHostsPaths, matches...)
		}
	}
	for _, path := range knownHostsPaths {
		if content, err := ioutil.ReadFile(path); err == nil {
			stats.HistorySize = len(strings.Split(strings.TrimSpace(string(content)), "\n"))
			if stats.HistorySize > 0 {
				break
			}
		}
	}

	// 5. SSH Process Memory
	// Calculate total memory usage of all sshd processes
	procs := getTopProcesses()
	var totalMem float64
	for _, p := range procs {
		if p.Name == "sshd" || strings.Contains(p.Cmdline, "sshd") {
			totalMem += p.MemoryPercent
		}
	}
	stats.SSHProcessMemory = round(totalMem)

	// Fallback: if no sessions captured but connections exist, infer from TCP 22
	// Also try to find sshd processes to get real users
	if len(stats.Sessions) == 0 {
		// Get active SSH connections first to try to match IPs
		activeIPs := []string{}
		if conns, err := net.Connections("tcp"); err == nil {
			for _, c := range conns {
				if c.Laddr.Port == 22 && c.Status == "ESTABLISHED" {
					activeIPs = append(activeIPs, c.Raddr.IP)
				}
			}
		}

		// Use getTopProcesses to leverage cache and avoid double scanning
		procs := getTopProcesses()
		for _, pInfo := range procs {
			if pInfo.Name == "sshd" {
				// Pattern: sshd: user@pts/0 OR sshd: user@notty
				if strings.Contains(pInfo.Cmdline, "@") {
					parts := strings.Split(pInfo.Cmdline, "@")
					if len(parts) > 0 {
						userPart := parts[0]
						// sshd: user
						userParts := strings.Fields(userPart)
						if len(userParts) > 1 {
							user := userParts[len(userParts)-1]

							// Find IP from connection
							ip := "Unknown"
							if len(activeIPs) == 1 {
								ip = activeIPs[0]
							} else if len(activeIPs) > 0 {
								ip = strings.Join(activeIPs, ", ")
							}

							var started int64 = 0
							// Need CreateTime from actual process
							if p, err := process.NewProcess(pInfo.PID); err == nil {
								if createTime, err := p.CreateTime(); err == nil {
									started = createTime
								}
							}

							stats.Sessions = append(stats.Sessions, map[string]interface{}{
								"user":    user,
								"ip":      ip,
								"started": started,
							})
						}
					}
				}
			}
		}
	}

	// If still empty, use the TCP fallback
	if len(stats.Sessions) == 0 && stats.Connections > 0 {
		conns, _ := net.Connections("tcp")
		for _, c := range conns {
			if c.Laddr.Port == 22 && c.Status == "ESTABLISHED" {
				ip := c.Raddr.IP
				stats.Sessions = append(stats.Sessions, map[string]interface{}{
					"user":    "ssh (est)",
					"ip":      ip,
					"started": 0,
				})
			}
		}
	}

	sshStatsCache = stats
	lastSSHTime = time.Now()
	return stats
}

func collectStats() Response {
	var resp Response

	// CPU
	cpuPercent, _ := cpu.Percent(0, false)
	if len(cpuPercent) > 0 {
		resp.CPU.Percent = round(cpuPercent[0])
	}

	perCore, _ := cpu.Percent(0, true)
	resp.CPU.PerCore = make([]float64, len(perCore))
	for i, v := range perCore {
		resp.CPU.PerCore[i] = round(v)
	}

	resp.CPU.Info = getCPUInfo()

	// Load Avg
	if avg, err := load.Avg(); err == nil {
		resp.CPU.LoadAvg = []float64{round(avg.Load1), round(avg.Load5), round(avg.Load15)}
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
			"user":    round((t.User / total) * 100),
			"system":  round((t.System / total) * 100),
			"idle":    round((t.Idle / total) * 100),
			"iowait":  round((t.Iowait / total) * 100),
			"irq":     round((t.Irq / total) * 100),
			"softirq": round((t.Softirq / total) * 100),
		}
	}

	// CPU Freq
	if freqs, err := cpu.Info(); err == nil && len(freqs) > 0 {
		// Try to get real-time freq
		var perCoreFreq []float64

		// Manual parsing of /proc/cpuinfo for real-time frequency
		// because gopsutil's Info() returns max freq, and Freq() is missing in this version
		var realFreqs []float64
		if file, err := os.Open("/proc/cpuinfo"); err == nil {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "cpu MHz") {
					parts := strings.Split(line, ":")
					if len(parts) > 1 {
						valStr := strings.TrimSpace(parts[1])
						val, err := strconv.ParseFloat(valStr, 64)
						if err == nil {
							realFreqs = append(realFreqs, val)
						}
					}
				}
			}
			file.Close()
		}

		if len(realFreqs) > 0 {
			for _, f := range realFreqs {
				perCoreFreq = append(perCoreFreq, round(f))
			}
		} else {
			// Fallback to Info() Mhz if manual parsing fails
			for _, f := range freqs {
				perCoreFreq = append(perCoreFreq, round(f.Mhz))
			}
		}

		avgFreq := 0.0
		if len(perCoreFreq) > 0 {
			sum := 0.0
			for _, f := range perCoreFreq {
				sum += f
			}
			avgFreq = round(sum / float64(len(perCoreFreq)))
		}

		resp.CPU.Freq = CPUFreq{
			Avg:     avgFreq,
			PerCore: perCoreFreq,
		}
	}

	// Sensors
	resp.Sensors = getSensors()

	// Power
	resp.Power = getPower()

	// Update history
	historyMutex.Lock()
	// Mock temp for now
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
			currentTemp = round(sum / count)
		}
	}

	if len(cpuTempHistory) >= 300 {
		cpuTempHistory = cpuTempHistory[1:]
	}
	cpuTempHistory = append(cpuTempHistory, currentTemp)
	resp.CPU.TempHistory = make([]float64, len(cpuTempHistory))
	copy(resp.CPU.TempHistory, cpuTempHistory)
	historyMutex.Unlock()

	// Memory
	v, _ := mem.VirtualMemory()
	resp.Memory = MemInfo{
		Total:     getSize(v.Total),
		Used:      getSize(v.Used),
		Free:      getSize(v.Free),
		Percent:   round(v.UsedPercent),
		Available: getSize(v.Available),
		Buffers:   getSize(v.Buffers),
		Cached:    getSize(v.Cached),
		Shared:    getSize(v.Shared),
		Active:    getSize(v.Active),
		Inactive:  getSize(v.Inactive),
		Slab:      getSize(v.Slab),
	}

	historyMutex.Lock()
	if len(memHistory) >= 300 {
		memHistory = memHistory[1:]
	}
	memHistory = append(memHistory, v.UsedPercent)
	resp.Memory.History = make([]float64, len(memHistory))
	copy(resp.Memory.History, memHistory)
	historyMutex.Unlock()

	// Swap
	s, _ := mem.SwapMemory()
	resp.Swap = SwapInfo{
		Total:   getSize(s.Total),
		Used:    getSize(s.Used),
		Free:    getSize(s.Free),
		Percent: round(s.UsedPercent),
		Sin:     getSize(s.Sin),
		Sout:    getSize(s.Sout),
	}

	// Disk
	parts, _ := disk.Partitions(false)
	for _, part := range parts {
		if strings.Contains(part.Device, "loop") || part.Fstype == "squashfs" {
			continue
		}

		mountpoint := part.Mountpoint
		checkPath := mountpoint
		if _, err := os.Stat("/hostfs"); err == nil {
			checkPath = filepath.Join("/hostfs", strings.TrimPrefix(mountpoint, "/"))
		}

		u, err := disk.Usage(checkPath)
		if err == nil {
			resp.Disk = append(resp.Disk, DiskInfo{
				Device:     part.Device,
				Mountpoint: part.Mountpoint,
				Fstype:     part.Fstype,
				Total:      getSize(u.Total),
				Used:       getSize(u.Used),
				Free:       getSize(u.Free),
				Percent:    round(u.UsedPercent),
			})

			// Inodes
			resp.Inodes = append(resp.Inodes, InodeInfo{
				Mountpoint: part.Mountpoint,
				Total:      u.InodesTotal,
				Used:       u.InodesUsed,
				Free:       u.InodesFree,
				Percent:    round(u.InodesUsedPercent),
			})
		}
	}

	// Disk IO
	resp.DiskIO = make(map[string]DiskIOInfo)
	if ioCounters, err := disk.IOCounters(); err == nil {
		for name, io := range ioCounters {
			resp.DiskIO[name] = DiskIOInfo{
				ReadBytes:  getSize(io.ReadBytes),
				WriteBytes: getSize(io.WriteBytes),
				ReadCount:  io.ReadCount,
				WriteCount: io.WriteCount,
				ReadTime:   io.ReadTime,
				WriteTime:  io.WriteTime,
			}
		}
	}

	// Network
	netIO, _ := net.IOCounters(false)
	if len(netIO) > 0 {
		resp.Network.BytesSent = getSize(netIO[0].BytesSent)
		resp.Network.BytesRecv = getSize(netIO[0].BytesRecv)
		resp.Network.RawSent = netIO[0].BytesSent
		resp.Network.RawRecv = netIO[0].BytesRecv
		resp.Network.Errors = map[string]uint64{
			"total_errors_in":  netIO[0].Errin,
			"total_errors_out": netIO[0].Errout,
			"total_drops_in":   netIO[0].Dropin,
			"total_drops_out":  netIO[0].Dropout,
		}
	}

	// Interfaces
	resp.Network.Interfaces = make(map[string]Interface)
	netIfs, _ := net.Interfaces()
	netIOPerNic, _ := net.IOCounters(true)
	ioMap := make(map[string]net.IOCountersStat)
	for _, io := range netIOPerNic {
		ioMap[io.Name] = io
	}

	for _, nic := range netIfs {
		var ip string
		for _, addr := range nic.Addrs {
			if strings.Contains(addr.Addr, ".") { // IPv4
				ip = addr.Addr
				break
			}
		}

		io := ioMap[nic.Name]
		resp.Network.Interfaces[nic.Name] = Interface{
			IP:        ip,
			BytesSent: getSize(io.BytesSent),
			BytesRecv: getSize(io.BytesRecv),
			IsUp:      strings.Contains(strings.Join(nic.Flags, ","), "up"),
			ErrorsIn:  io.Errin,
			ErrorsOut: io.Errout,
			DropsIn:   io.Dropin,
			DropsOut:  io.Dropout,
		}
	}

	// Network Extras
	resp.Network.ConnectionStates = getConnectionStates()
	resp.Network.ListeningPorts = getListeningPorts()

	// Socket Stats (from /proc/net/sockstat)
	resp.Network.Sockets = map[string]int{"tcp": 0, "udp": 0, "tcp_tw": 0}
	if file, err := os.Open("/proc/net/sockstat"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.Fields(line)
			if strings.HasPrefix(line, "TCP:") {
				for i, p := range parts {
					if p == "inuse" && i+1 < len(parts) {
						v, _ := strconv.Atoi(parts[i+1])
						resp.Network.Sockets["tcp"] = v
					} else if p == "tw" && i+1 < len(parts) {
						v, _ := strconv.Atoi(parts[i+1])
						resp.Network.Sockets["tcp_tw"] = v
					}
				}
			} else if strings.HasPrefix(line, "UDP:") {
				for i, p := range parts {
					if p == "inuse" && i+1 < len(parts) {
						v, _ := strconv.Atoi(parts[i+1])
						resp.Network.Sockets["udp"] = v
					}
				}
			}
		}
	}

	// Processes
	resp.Processes = getTopProcesses()

	// GPU
	resp.GPU = getGPUDetails()

	// Boot Time
	bootTime, _ := host.BootTime()
	bt := time.Unix(int64(bootTime), 0)
	resp.BootTime = bt.Format("2006/01/02 15:04:05")

	// SSH Stats
	resp.SSHStats = getSSHStats()

	return resp
}

// --- Main ---

func infoHandler(w http.ResponseWriter, r *http.Request) {
	bootTime, _ := host.BootTime()
	uptimeSeconds := uint64(time.Now().Unix()) - bootTime

	// Pretty uptime
	uptimeString := formatUptime(uptimeSeconds)

	v, _ := mem.VirtualMemory()
	memStr := fmt.Sprintf("%s / %s (%.1f%%)", getSize(v.Used), getSize(v.Total), v.UsedPercent)

	s, _ := mem.SwapMemory()
	swapStr := fmt.Sprintf("%s / %s (%.1f%%)", getSize(s.Used), getSize(s.Total), s.UsedPercent)

	diskStr := "Unknown"
	if parts, err := disk.Partitions(false); err == nil && len(parts) > 0 {
		// Use root partition or first one
		for _, p := range parts {
			if p.Mountpoint == "/" || p.Mountpoint == "/hostfs" {
				if u, err := disk.Usage(p.Mountpoint); err == nil {
					diskStr = fmt.Sprintf("%s / %s (%.1f%%)", getSize(u.Used), getSize(u.Total), u.UsedPercent)
					break
				}
			}
		}
	}

	hostInfo, _ := host.Info()
	cpuInfo := getCPUInfo()

	// Get IP (outbound)
	ip := "Unknown"
	if conn, err := stdnet.Dial("udp", "8.8.8.8:80"); err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*stdnet.UDPAddr)
		ip = localAddr.IP.String()
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	locale := os.Getenv("LANG")
	if locale == "" {
		locale = "C"
	}
	osPretty := detectOSName()
	if osPretty == "" {
		osPretty = hostInfo.Platform
		if hostInfo.PlatformVersion != "" {
			osPretty = fmt.Sprintf("%s %s", osPretty, hostInfo.PlatformVersion)
		}
		if hostInfo.KernelVersion != "" {
			osPretty = fmt.Sprintf("%s (%s)", osPretty, hostInfo.KernelVersion)
		}
	} else {
		// Append kernel version to detected OS name
		if hostInfo.KernelVersion != "" {
			osPretty = fmt.Sprintf("%s (%s)", osPretty, hostInfo.KernelVersion)
		}
	}

	response := map[string]interface{}{
		"header": fmt.Sprintf("%s@%s", "root", hostInfo.Hostname),
		"os":     osPretty,
		"kernel": hostInfo.KernelVersion,
		"uptime": uptimeString,
		"shell":  shell,
		"cpu":    fmt.Sprintf("%s (%d) @ %.2f GHz", cpuInfo.Model, cpuInfo.Cores, cpuInfo.MaxFreq/1000),
		"gpu":    getGPUInfo(),
		"memory": memStr,
		"swap":   swapStr,
		"disk":   diskStr,
		"ip":     ip,
		"locale": locale,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
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

func main() {
	fs := http.FileServer(http.Dir("./templates"))
	http.Handle("/", fs)
	http.HandleFunc("/ws/stats", wsHandler)
	http.HandleFunc("/api/info", infoHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	fmt.Printf("Server starting on port %s...\n", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
