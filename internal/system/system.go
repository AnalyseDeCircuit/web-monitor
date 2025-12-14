// Package system 提供系统信息获取功能
package system

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/gpu"
	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// StaticInfo 静态系统信息
type StaticInfo struct {
	Header string `json:"header"`
	OS     string `json:"os"`
	Kernel string `json:"kernel"`
	Uptime string `json:"uptime"`
	Shell  string `json:"shell"`
	CPU    string `json:"cpu"`
	GPU    string `json:"gpu"`
	Memory string `json:"memory"`
	Swap   string `json:"swap"`
	Disk   string `json:"disk"`
	IP     string `json:"ip"`
	Locale string `json:"locale"`
}

// GetStaticInfo 获取静态系统信息
func GetStaticInfo() *StaticInfo {
	info := &StaticInfo{
		Header: getHostname(),
		OS:     getOSInfo(),
		Kernel: getKernelVersion(),
		Uptime: getUptime(),
		Shell:  getShell(),
		CPU:    getCPUModel(),
		GPU:    gpu.GetSimpleGPUInfo(),
		Memory: getMemoryInfo(),
		Swap:   getSwapInfo(),
		Disk:   getDiskInfo(),
		IP:     getLocalIP(),
		Locale: getLocale(),
	}
	return info
}

func getHostname() string {
	name, _ := os.Hostname()
	if name == "" {
		name = "Unknown"
	}
	return name
}

func getOSInfo() string {
	paths := []string{
		"/hostfs/etc/os-release",
		"/etc/os-release",
	}

	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		osName := ""
		osVersion := ""

		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "NAME=") {
				osName = strings.Trim(strings.TrimPrefix(line, "NAME="), `"`)
			} else if strings.HasPrefix(line, "VERSION=") {
				osVersion = strings.Trim(strings.TrimPrefix(line, "VERSION="), `"`)
			}
		}

		if osName != "" {
			if osVersion != "" {
				return fmt.Sprintf("%s %s", osName, osVersion)
			}
			return osName
		}
	}

	return runtime.GOOS
}

func getKernelVersion() string {
	paths := []string{
		"/hostfs/proc/version",
		"/proc/version",
	}

	for _, path := range paths {
		data, err := ioutil.ReadFile(path)
		if err == nil {
			fields := strings.Fields(string(data))
			if len(fields) >= 3 {
				return fields[2]
			}
		}
	}

	cmd := exec.Command("uname", "-r")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}

	return "Unknown"
}

func getUptime() string {
	uptime, err := host.Uptime()
	if err != nil {
		return "Unknown"
	}

	duration := time.Duration(uptime) * time.Second
	days := duration / (24 * time.Hour)
	duration -= days * 24 * time.Hour
	hours := duration / time.Hour
	duration -= hours * time.Hour
	minutes := duration / time.Minute

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func getShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return shell
}

func getCPUModel() string {
	paths := []string{
		"/hostfs/proc/cpuinfo",
		"/proc/cpuinfo",
	}

	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "model name") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}

	return "Unknown CPU"
}

func getMemoryInfo() string {
	v, err := mem.VirtualMemory()
	if err != nil {
		return "Unknown"
	}
	return fmt.Sprintf("%s / %s", utils.GetSize(v.Used), utils.GetSize(v.Total))
}

func getSwapInfo() string {
	s, err := mem.SwapMemory()
	if err != nil {
		return "Unknown"
	}
	if s.Total == 0 {
		return "No swap"
	}
	return fmt.Sprintf("%s / %s", utils.GetSize(s.Used), utils.GetSize(s.Total))
}

func getDiskInfo() string {
	paths := []string{"/hostfs", "/"}

	for _, path := range paths {
		u, err := disk.Usage(path)
		if err == nil {
			return fmt.Sprintf("%s / %s", utils.GetSize(u.Used), utils.GetSize(u.Total))
		}
	}

	return "Unknown"
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "Unknown"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "Unknown"
}

func getLocale() string {
	locale := os.Getenv("LANG")
	if locale == "" {
		locale = "C"
	}
	return locale
}
