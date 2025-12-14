// Package gpu 提供GPU信息获取功能
package gpu

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
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

		vendorBytes, err1 := ioutil.ReadFile(vendorFile)
		deviceBytes, err2 := ioutil.ReadFile(deviceFile)

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
				Index:       i,
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

			details = append(details, detail)
		}
	}

	gpuDetailsCache = details
	lastDetailsTime = time.Now()
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

	matches, _ := filepath.Glob("/sys/class/drm/card*")
	for _, cardPath := range matches {
		vendorFile := filepath.Join(cardPath, "device/vendor")
		deviceFile := filepath.Join(cardPath, "device/device")

		vendorBytes, err1 := ioutil.ReadFile(vendorFile)
		deviceBytes, err2 := ioutil.ReadFile(deviceFile)

		if err1 == nil && err2 == nil {
			vendor := strings.ToLower(strings.TrimSpace(string(vendorBytes)))
			device := strings.ToLower(strings.TrimSpace(string(deviceBytes)))

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
