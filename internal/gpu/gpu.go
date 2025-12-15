// Package gpu 提供GPU信息获取功能
package gpu

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

var (
	gpuInfoLock     sync.Mutex
	lastGPUInfoTime time.Time
	gpuDetailsCache []types.GPUDetail
	lastDetailsTime time.Time

	// nvidia-smi availability cache
	nvidiaSmiAvailable     bool
	nvidiaSmiChecked       bool
	nvidiaSmiCheckTime     time.Time
	nvidiaSmiCheckInterval = 5 * time.Minute
)

// GetGPUInfo 获取GPU详细信息
func GetGPUInfo() []types.GPUDetail {
	gpuInfoLock.Lock()
	defer gpuInfoLock.Unlock()

	// Cache for 5 seconds
	if time.Since(lastDetailsTime) < 5*time.Second && len(gpuDetailsCache) > 0 {
		return gpuDetailsCache
	}

	var details []types.GPUDetail

	// Try nvidia-smi first for NVIDIA GPUs (provides much richer data)
	nvidiaGPUs := getNvidiaGPUInfo()
	if len(nvidiaGPUs) > 0 {
		details = append(details, nvidiaGPUs...)
	}

	// Then scan DRM for other GPUs (Intel, AMD, or NVIDIA without nvidia-smi)
	drmGPUs := getDRMGPUInfo(len(details))

	// Merge: skip DRM entries that are already covered by nvidia-smi
	nvidiaIndices := make(map[string]bool)
	for _, g := range nvidiaGPUs {
		nvidiaIndices[g.PCIAddress] = true
	}
	for _, g := range drmGPUs {
		// Skip if this is an NVIDIA GPU already reported by nvidia-smi
		if strings.Contains(strings.ToLower(g.Vendor), "10de") && len(nvidiaGPUs) > 0 {
			continue
		}
		details = append(details, g)
	}

	gpuDetailsCache = details
	lastDetailsTime = time.Now()
	return details
}

// checkNvidiaSmi checks if nvidia-smi is available
func checkNvidiaSmi() bool {
	now := time.Now()
	if nvidiaSmiChecked && now.Sub(nvidiaSmiCheckTime) < nvidiaSmiCheckInterval {
		return nvidiaSmiAvailable
	}

	nvidiaSmiChecked = true
	nvidiaSmiCheckTime = now

	// Check common paths
	paths := []string{
		"nvidia-smi",
		"/usr/bin/nvidia-smi",
		"/usr/local/bin/nvidia-smi",
		"/hostfs/usr/bin/nvidia-smi",
	}
	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			nvidiaSmiAvailable = true
			return true
		}
	}
	nvidiaSmiAvailable = false
	return false
}

// getNvidiaGPUInfo gets detailed NVIDIA GPU info via nvidia-smi
func getNvidiaGPUInfo() []types.GPUDetail {
	if !checkNvidiaSmi() {
		return nil
	}

	// Query format: index, name, pci.bus_id, memory.total, memory.used, utilization.gpu, temperature.gpu, power.draw
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=index,name,pci.bus_id,memory.total,memory.used,utilization.gpu,temperature.gpu,power.draw",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var gpus []types.GPUDetail
	reader := csv.NewReader(strings.NewReader(string(output)))
	reader.TrimLeadingSpace = true

	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		if len(record) < 8 {
			continue
		}

		idx, _ := strconv.Atoi(strings.TrimSpace(record[0]))
		name := strings.TrimSpace(record[1])
		pciAddr := strings.TrimSpace(record[2])
		memTotal, _ := strconv.ParseFloat(strings.TrimSpace(record[3]), 64)
		memUsed, _ := strconv.ParseFloat(strings.TrimSpace(record[4]), 64)
		gpuUtil, _ := strconv.ParseFloat(strings.TrimSpace(record[5]), 64)
		temp, _ := strconv.ParseFloat(strings.TrimSpace(record[6]), 64)
		power, _ := strconv.ParseFloat(strings.TrimSpace(record[7]), 64)

		var vramPercent float64
		if memTotal > 0 {
			vramPercent = (memUsed / memTotal) * 100
		}

		gpu := types.GPUDetail{
			Index:       idx,
			Name:        "NVIDIA " + name,
			Vendor:      "0x10de",
			PCIAddress:  pciAddr,
			DRMCard:     fmt.Sprintf("card%d", idx),
			VRAMTotal:   fmt.Sprintf("%.0f MiB", memTotal),
			VRAMUsed:    fmt.Sprintf("%.0f MiB", memUsed),
			VRAMPercent: vramPercent,
			FreqMHz:     0, // Could query separately if needed
			TempC:       temp,
			PowerW:      power,
			LoadPercent: gpuUtil,
		}
		gpus = append(gpus, gpu)
	}

	// Try to get GPU processes
	if len(gpus) > 0 {
		procs := getNvidiaProcesses()
		// Attach processes to GPUs
		for i := range gpus {
			for _, p := range procs {
				if p.GPUIndex == gpus[i].Index {
					gpus[i].Processes = append(gpus[i].Processes, types.GPUProcess{
						PID:      p.PID,
						Name:     p.Name,
						VRAMUsed: p.VRAMUsed,
					})
				}
			}
		}
	}

	return gpus
}

type nvidiaProcess struct {
	GPUIndex int
	PID      int
	Name     string
	VRAMUsed string
}

// getNvidiaProcesses gets processes using NVIDIA GPUs
func getNvidiaProcesses() []nvidiaProcess {
	cmd := exec.Command("nvidia-smi",
		"--query-compute-apps=gpu_bus_id,pid,process_name,used_memory",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var procs []nvidiaProcess
	reader := csv.NewReader(strings.NewReader(string(output)))
	reader.TrimLeadingSpace = true

	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		if len(record) < 4 {
			continue
		}

		pid, _ := strconv.Atoi(strings.TrimSpace(record[1]))
		name := strings.TrimSpace(record[2])
		mem := strings.TrimSpace(record[3])

		procs = append(procs, nvidiaProcess{
			GPUIndex: 0, // Will be matched by bus_id in a more complete implementation
			PID:      pid,
			Name:     name,
			VRAMUsed: mem + " MiB",
		})
	}

	return procs
}

// detectGPUDriver detects the driver used by a DRM card
func detectGPUDriver(cardPath string) string {
	driverLink := filepath.Join(cardPath, "device/driver")
	if target, err := os.Readlink(driverLink); err == nil {
		return filepath.Base(target) // "i915", "amdgpu", "nouveau", etc.
	}
	return ""
}

// readSysfsInt reads an integer value from a sysfs file
func readSysfsInt(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// readSysfsFloat reads a float value from a sysfs file
func readSysfsFloat(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
}

// findHwmonPath finds the hwmon directory for a device
func findHwmonPath(devicePath string) string {
	hwmonBase := filepath.Join(devicePath, "hwmon")
	entries, err := os.ReadDir(hwmonBase)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "hwmon") {
			return filepath.Join(hwmonBase, entry.Name())
		}
	}
	return ""
}

// getIntelGPUMetrics gets detailed Intel iGPU data (i915 driver)
func getIntelGPUMetrics(cardPath string) (freqMHz float64, tempC float64, loadPercent float64) {
	// 1. Read current frequency from /sys/class/drm/card*/gt/gt_cur_freq_mhz
	//    or /sys/class/drm/card*/gt_cur_freq_mhz (older kernels)
	freqPaths := []string{
		filepath.Join(cardPath, "gt/gt_cur_freq_mhz"),
		filepath.Join(cardPath, "gt_cur_freq_mhz"),
		filepath.Join(cardPath, "device/gt/gt_cur_freq_mhz"),
	}
	for _, p := range freqPaths {
		if val, err := readSysfsInt(p); err == nil {
			freqMHz = float64(val)
			break
		}
	}

	// 2. Read max frequency to calculate load percentage
	maxFreqPaths := []string{
		filepath.Join(cardPath, "gt/gt_max_freq_mhz"),
		filepath.Join(cardPath, "gt_max_freq_mhz"),
		filepath.Join(cardPath, "device/gt/gt_max_freq_mhz"),
	}
	var maxFreq float64
	for _, p := range maxFreqPaths {
		if val, err := readSysfsInt(p); err == nil {
			maxFreq = float64(val)
			break
		}
	}

	// 3. Read actual frequency (when GPU is active)
	actFreqPaths := []string{
		filepath.Join(cardPath, "gt/gt_act_freq_mhz"),
		filepath.Join(cardPath, "gt_act_freq_mhz"),
	}
	var actFreq float64
	for _, p := range actFreqPaths {
		if val, err := readSysfsInt(p); err == nil {
			actFreq = float64(val)
			break
		}
	}

	// Calculate load: use actual freq vs max freq ratio
	if maxFreq > 0 && actFreq > 0 {
		loadPercent = (actFreq / maxFreq) * 100
		if loadPercent > 100 {
			loadPercent = 100
		}
	}

	// 4. Read temperature from hwmon
	hwmonPath := findHwmonPath(filepath.Join(cardPath, "device"))
	if hwmonPath != "" {
		if val, err := readSysfsInt(filepath.Join(hwmonPath, "temp1_input")); err == nil {
			tempC = float64(val) / 1000.0 // Convert millidegrees to degrees
		}
	}

	return
}

// getAMDGPUMetrics gets detailed AMD APU/GPU data (amdgpu driver)
func getAMDGPUMetrics(cardPath string) (freqMHz float64, tempC float64, powerW float64, loadPercent float64, vramUsed float64, vramTotal float64) {
	devicePath := filepath.Join(cardPath, "device")

	// 1. Read GPU load from /sys/class/drm/card*/device/gpu_busy_percent
	if val, err := readSysfsInt(filepath.Join(devicePath, "gpu_busy_percent")); err == nil {
		loadPercent = float64(val)
	}

	// 2. Read current frequency from pp_dpm_sclk (parse line with *)
	sclkPath := filepath.Join(devicePath, "pp_dpm_sclk")
	if data, err := os.ReadFile(sclkPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.Contains(line, "*") {
				// Format: "0: 200Mhz *" or "1: 1800Mhz *"
				parts := strings.Fields(line)
				for _, part := range parts {
					if strings.HasSuffix(strings.ToLower(part), "mhz") {
						freqStr := strings.TrimSuffix(strings.ToLower(part), "mhz")
						if val, err := strconv.ParseFloat(freqStr, 64); err == nil {
							freqMHz = val
							break
						}
					}
				}
				break
			}
		}
	}

	// 3. Read temperature from hwmon
	hwmonPath := findHwmonPath(devicePath)
	if hwmonPath != "" {
		// Temperature (temp1_input is in millidegrees)
		if val, err := readSysfsInt(filepath.Join(hwmonPath, "temp1_input")); err == nil {
			tempC = float64(val) / 1000.0
		}

		// Power (power1_average is in microwatts)
		if val, err := readSysfsInt(filepath.Join(hwmonPath, "power1_average")); err == nil {
			powerW = float64(val) / 1000000.0
		}
	}

	// 4. Read VRAM usage (for dedicated GPUs or APUs with dedicated VRAM)
	if val, err := readSysfsInt(filepath.Join(devicePath, "mem_info_vram_used")); err == nil {
		vramUsed = float64(val)
	}
	if val, err := readSysfsInt(filepath.Join(devicePath, "mem_info_vram_total")); err == nil {
		vramTotal = float64(val)
	}

	return
}

// formatBytes formats bytes to human-readable string (MiB/GiB)
func formatBytes(bytes float64) string {
	const (
		MiB = 1024 * 1024
		GiB = 1024 * 1024 * 1024
	)
	if bytes >= GiB {
		return fmt.Sprintf("%.1f GiB", bytes/GiB)
	}
	return fmt.Sprintf("%.0f MiB", bytes/MiB)
}

// getDRMGPUInfo gets GPU info from /sys/class/drm (for Intel, AMD, and NVIDIA without nvidia-smi)
func getDRMGPUInfo(startIndex int) []types.GPUDetail {
	var details []types.GPUDetail
	matches, _ := filepath.Glob("/sys/class/drm/card*")
	sort.Strings(matches)

	for i, cardPath := range matches {
		vendorFile := filepath.Join(cardPath, "device/vendor")
		deviceFile := filepath.Join(cardPath, "device/device")

		// Fallback to direct vendor/device if device/vendor doesn't exist
		if _, err := os.Stat(vendorFile); os.IsNotExist(err) {
			vendorFile = filepath.Join(cardPath, "vendor")
			deviceFile = filepath.Join(cardPath, "device")
		}

		vendorBytes, err1 := os.ReadFile(vendorFile)
		deviceBytes, err2 := os.ReadFile(deviceFile)

		if err1 == nil && err2 == nil {
			vendor := strings.ToLower(strings.TrimSpace(string(vendorBytes)))
			device := strings.ToLower(strings.TrimSpace(string(deviceBytes)))

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

			detail := types.GPUDetail{
				Index:       startIndex + i,
				Name:        gpuName,
				Vendor:      vendor,
				PCIAddress:  filepath.Base(cardPath),
				DRMCard:     filepath.Base(cardPath),
				VRAMTotal:   "N/A",
				VRAMUsed:    "N/A",
				VRAMPercent: 0,
				FreqMHz:     0,
				TempC:       0,
				PowerW:      0,
				LoadPercent: 0,
			}

			// Detect driver and collect metrics based on GPU type
			driver := detectGPUDriver(cardPath)

			switch driver {
			case "i915", "xe": // Intel iGPU (i915 for older, xe for newer Arc)
				freq, temp, load := getIntelGPUMetrics(cardPath)
				detail.FreqMHz = freq
				detail.TempC = temp
				detail.LoadPercent = load
				// Intel iGPU shares system memory
				detail.VRAMTotal = "Shared"
				detail.VRAMUsed = "Shared"

			case "amdgpu": // AMD APU or discrete GPU
				freq, temp, power, load, vramUsed, vramTotal := getAMDGPUMetrics(cardPath)
				detail.FreqMHz = freq
				detail.TempC = temp
				detail.PowerW = power
				detail.LoadPercent = load
				if vramTotal > 0 {
					detail.VRAMTotal = formatBytes(vramTotal)
					detail.VRAMUsed = formatBytes(vramUsed)
					detail.VRAMPercent = (vramUsed / vramTotal) * 100
				} else {
					// APU with shared memory
					detail.VRAMTotal = "Shared"
					detail.VRAMUsed = "Shared"
				}

			case "nouveau": // Open-source NVIDIA driver (limited metrics)
				// Nouveau has limited sysfs exposure
				hwmonPath := findHwmonPath(filepath.Join(cardPath, "device"))
				if hwmonPath != "" {
					if val, err := readSysfsInt(filepath.Join(hwmonPath, "temp1_input")); err == nil {
						detail.TempC = float64(val) / 1000.0
					}
				}
			}

			details = append(details, detail)
		}
	}

	return details
}

// lookupPCIName 查找PCI设备名称
func lookupPCIName(vendorID, deviceID string) string {
	// Try both lowercase and uppercase
	vendorID = strings.TrimPrefix(strings.ToLower(vendorID), "0x")
	deviceID = strings.TrimPrefix(strings.ToLower(deviceID), "0x")

	paths := []string{
		"/usr/share/hwdata/pci.ids",
		"/usr/share/pci.ids",
		"/usr/share/misc/pci.ids",
		"/hostfs/usr/share/hwdata/pci.ids",
		"/hostfs/usr/share/misc/pci.ids",
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
			// Vendor line
			if strings.HasPrefix(line, vendorID) {
				inVendor = true
			} else {
				inVendor = false
			}
		} else if inVendor && strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t") {
			// Device line
			trimmed := strings.TrimPrefix(line, "\t")
			if strings.HasPrefix(trimmed, deviceID) {
				if len(trimmed) > len(deviceID) {
					return strings.TrimSpace(trimmed[len(deviceID):])
				}
			}
		}
	}
	return ""
}

// GetSimpleGPUInfo 获取简单的GPU信息字符串（用于 /api/info）
func GetSimpleGPUInfo() string {
	gpuInfoLock.Lock()
	defer gpuInfoLock.Unlock()

	var gpus []string
	seen := make(map[string]bool)

	// Try nvidia-smi first
	if checkNvidiaSmi() {
		cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
		if output, err := cmd.Output(); err == nil {
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, line := range lines {
				name := strings.TrimSpace(line)
				if name != "" && !seen[name] {
					seen[name] = true
					gpus = append(gpus, "NVIDIA "+name)
				}
			}
		}
	}

	// Scan DRM for other GPUs
	matches, _ := filepath.Glob("/sys/class/drm/card*")
	for _, cardPath := range matches {
		vendorFile := filepath.Join(cardPath, "device/vendor")
		deviceFile := filepath.Join(cardPath, "device/device")

		vendorBytes, err1 := os.ReadFile(vendorFile)
		deviceBytes, err2 := os.ReadFile(deviceFile)

		if err1 == nil && err2 == nil {
			vendor := strings.ToLower(strings.TrimSpace(string(vendorBytes)))
			device := strings.ToLower(strings.TrimSpace(string(deviceBytes)))

			// Skip NVIDIA if we already got them from nvidia-smi
			if (vendor == "0x10de" || vendor == "10de") && len(gpus) > 0 {
				continue
			}

			key := vendor + ":" + device
			if seen[key] {
				continue
			}
			seen[key] = true

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
		return "Unknown GPU"
	}
	return strings.Join(gpus, " + ")
}
