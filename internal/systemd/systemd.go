// Package systemd 提供Systemd服务管理功能
package systemd

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

var (
	systemdMutex sync.Mutex
)

// ListServices 列出所有Systemd服务
func ListServices() ([]types.ServiceInfo, error) {
	systemdMutex.Lock()
	defer systemdMutex.Unlock()

	// 使用 chroot /hostfs 在宿主机环境中执行 systemctl
	cmd := exec.Command("chroot", "/hostfs", "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %v", err)
	}

	var services []types.ServiceInfo
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		service := types.ServiceInfo{
			Unit:        fields[0],
			Load:        fields[1],
			Active:      fields[2],
			Sub:         fields[3],
			Description: fields[4],
		}
		services = append(services, service)
	}

	return services, nil
}

// ServiceAction 执行服务操作
func ServiceAction(serviceName, action string) error {
	systemdMutex.Lock()
	defer systemdMutex.Unlock()

	var args []string
	switch action {
	case "start":
		args = []string{"start", serviceName}
	case "stop":
		args = []string{"stop", serviceName}
	case "restart":
		args = []string{"restart", serviceName}
	case "reload":
		args = []string{"reload", serviceName}
	case "enable":
		args = []string{"enable", serviceName}
	case "disable":
		args = []string{"disable", serviceName}
	default:
		return fmt.Errorf("invalid action: %s", action)
	}

	// 使用 chroot /hostfs 执行 systemctl
	fullArgs := append([]string{"/hostfs", "systemctl"}, args...)
	cmd := exec.Command("chroot", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to %s service %s: %v\nOutput: %s", action, serviceName, err, string(output))
	}

	return nil
}

// GetServiceStatus 获取服务状态
func GetServiceStatus(serviceName string) (map[string]string, error) {
	systemdMutex.Lock()
	defer systemdMutex.Unlock()

	cmd := exec.Command("chroot", "/hostfs", "systemctl", "show", serviceName, "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get service status: %v", err)
	}

	status := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "="); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			status[key] = value
		}
	}

	return status, nil
}

// GetServiceLogs 获取服务日志
func GetServiceLogs(serviceName string, lines int) (string, error) {
	systemdMutex.Lock()
	defer systemdMutex.Unlock()

	cmd := exec.Command("journalctl", "-u", serviceName, "-n", fmt.Sprintf("%d", lines), "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get service logs: %v", err)
	}

	return string(output), nil
}

// GetSystemStatus 获取系统状态
func GetSystemStatus() (map[string]interface{}, error) {
	systemdMutex.Lock()
	defer systemdMutex.Unlock()

	status := make(map[string]interface{})

	// System uptime
	cmd := exec.Command("systemctl", "show", "--property=SystemState", "--property=SystemStartTimestamp", "--no-pager")
	output, err := cmd.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(output))
		for scanner.Scan() {
			line := scanner.Text()
			if idx := strings.Index(line, "="); idx != -1 {
				key := strings.TrimSpace(line[:idx])
				value := strings.TrimSpace(line[idx+1:])
				status[key] = value
			}
		}
	}

	// Failed services
	cmd = exec.Command("systemctl", "list-units", "--state=failed", "--no-pager", "--no-legend")
	output, err = cmd.Output()
	if err == nil {
		var failed []string
		scanner := bufio.NewScanner(bytes.NewReader(output))
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) > 0 {
				failed = append(failed, fields[0])
			}
		}
		status["FailedServices"] = failed
	}

	return status, nil
}

// ReloadSystemd 重新加载Systemd配置
func ReloadSystemd() error {
	systemdMutex.Lock()
	defer systemdMutex.Unlock()

	cmd := exec.Command("systemctl", "daemon-reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload systemd: %v\nOutput: %s", err, string(output))
	}

	return nil
}
