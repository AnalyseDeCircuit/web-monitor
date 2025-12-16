// Package systemd 提供Systemd服务管理功能
package systemd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	"github.com/coreos/go-systemd/v22/dbus"
)

var (
	systemdMutex sync.Mutex
)

// ListServices 列出所有Systemd服务
func ListServices() ([]types.ServiceInfo, error) {
	systemdMutex.Lock()
	defer systemdMutex.Unlock()

	// Use NewSystemConnectionContext to force using the system bus (and respect DBUS_SYSTEM_BUS_ADDRESS)
	// instead of falling back to the private socket which might not be mounted.
	conn, err := dbus.NewSystemConnectionContext(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to systemd dbus: %v", err)
	}
	defer conn.Close()

	units, err := conn.ListUnits()
	if err != nil {
		return nil, fmt.Errorf("failed to list units: %v", err)
	}

	var services []types.ServiceInfo
	for _, unit := range units {
		if !strings.HasSuffix(unit.Name, ".service") {
			continue
		}

		service := types.ServiceInfo{
			Unit:        unit.Name,
			Load:        unit.LoadState,
			Active:      unit.ActiveState,
			Sub:         unit.SubState,
			Description: unit.Description,
		}
		services = append(services, service)
	}

	return services, nil
}

// ServiceAction 执行服务操作
func ServiceAction(serviceName, action string) error {
	systemdMutex.Lock()
	defer systemdMutex.Unlock()

	conn, err := dbus.NewSystemConnectionContext(context.Background())
	if err != nil {
		return fmt.Errorf("failed to connect to systemd dbus: %v", err)
	}
	defer conn.Close()

	ch := make(chan string)
	var jobId int
	var jobErr error

	switch action {
	case "start":
		jobId, jobErr = conn.StartUnit(serviceName, "replace", ch)
	case "stop":
		jobId, jobErr = conn.StopUnit(serviceName, "replace", ch)
	case "restart":
		jobId, jobErr = conn.RestartUnit(serviceName, "replace", ch)
	case "reload":
		jobId, jobErr = conn.ReloadUnit(serviceName, "replace", ch)
	case "enable":
		_, _, err := conn.EnableUnitFiles([]string{serviceName}, false, false)
		return err
	case "disable":
		_, err := conn.DisableUnitFiles([]string{serviceName}, false)
		return err
	default:
		return fmt.Errorf("invalid action: %s", action)
	}

	if jobErr != nil {
		return fmt.Errorf("failed to %s service %s: %v", action, serviceName, jobErr)
	}

	// Wait for job completion
	result := <-ch
	if result != "done" {
		return fmt.Errorf("job %d failed with result: %s", jobId, result)
	}

	return nil
}

// GetServiceStatus 获取服务状态
func GetServiceStatus(serviceName string) (map[string]string, error) {
	systemdMutex.Lock()
	defer systemdMutex.Unlock()

	conn, err := dbus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to systemd dbus: %v", err)
	}
	defer conn.Close()

	props, err := conn.GetUnitProperties(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get unit properties: %v", err)
	}

	status := make(map[string]string)
	for k, v := range props {
		status[k] = fmt.Sprintf("%v", v)
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

	conn, err := dbus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to systemd dbus: %v", err)
	}
	defer conn.Close()

	status := make(map[string]interface{})

	// Failed services
	units, err := conn.ListUnits()
	if err == nil {
		var failed []string
		for _, unit := range units {
			if unit.ActiveState == "failed" && strings.HasSuffix(unit.Name, ".service") {
				failed = append(failed, unit.Name)
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

	conn, err := dbus.New()
	if err != nil {
		return fmt.Errorf("failed to connect to systemd dbus: %v", err)
	}
	defer conn.Close()

	return conn.Reload()
}
