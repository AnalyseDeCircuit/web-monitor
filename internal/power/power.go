// Package power 提供电源管理功能
package power

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

var (
	powerMutex sync.Mutex
)

// ShutdownSystem 关闭系统
func ShutdownSystem(delayMinutes int, reason string) (*types.PowerActionResult, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	if delayMinutes < 0 {
		return nil, fmt.Errorf("delay minutes cannot be negative")
	}

	var cmd *exec.Cmd
	if delayMinutes == 0 {
		cmd = exec.Command("shutdown", "-h", "now")
	} else {
		cmd = exec.Command("shutdown", "-h", fmt.Sprintf("+%d", delayMinutes), reason)
	}

	output, err := cmd.CombinedOutput()
	result := &types.PowerActionResult{
		Action:    "shutdown",
		Timestamp: time.Now().Format(time.RFC3339),
		Delay:     fmt.Sprintf("%d", delayMinutes),
		Reason:    reason,
		Output:    string(output),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	return result, nil
}

// RebootSystem 重启系统
func RebootSystem(delayMinutes int, reason string) (*types.PowerActionResult, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	if delayMinutes < 0 {
		return nil, fmt.Errorf("delay minutes cannot be negative")
	}

	var cmd *exec.Cmd
	if delayMinutes == 0 {
		cmd = exec.Command("shutdown", "-r", "now")
	} else {
		cmd = exec.Command("shutdown", "-r", fmt.Sprintf("+%d", delayMinutes), reason)
	}

	output, err := cmd.CombinedOutput()
	result := &types.PowerActionResult{
		Action:    "reboot",
		Timestamp: time.Now().Format(time.RFC3339),
		Delay:     fmt.Sprintf("%d", delayMinutes),
		Reason:    reason,
		Output:    string(output),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	return result, nil
}

// CancelShutdown 取消关机/重启
func CancelShutdown() (*types.PowerActionResult, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	cmd := exec.Command("shutdown", "-c")
	output, err := cmd.CombinedOutput()
	result := &types.PowerActionResult{
		Action:    "cancel",
		Timestamp: time.Now().Format(time.RFC3339),
		Output:    string(output),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	return result, nil
}

// GetShutdownStatus 获取关机状态
func GetShutdownStatus() (*types.ShutdownStatus, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	// 检查是否有计划的关机
	cmd := exec.Command("shutdown", "-c")
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	status := &types.ShutdownStatus{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if strings.Contains(outputStr, "shutdown scheduled") || strings.Contains(outputStr, "Shutdown scheduled") {
		status.Scheduled = true

		// 解析关机时间
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "shutdown scheduled for") || strings.Contains(line, "Shutdown scheduled for") {
				// 尝试解析时间
				parts := strings.Split(line, "scheduled for")
				if len(parts) > 1 {
					status.ScheduledTime = strings.TrimSpace(parts[1])
				}
				break
			}
		}
	} else {
		status.Scheduled = false
	}

	// 检查系统运行时间
	uptime, err := getSystemUptime()
	if err == nil {
		status.Uptime = uptime
	}

	return status, nil
}

// getSystemUptime 获取系统运行时间
func getSystemUptime() (string, error) {
	cmd := exec.Command("uptime", "-p")
	output, err := cmd.Output()
	if err != nil {
		// 尝试另一种格式
		cmd = exec.Command("uptime")
		output, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}

	uptimeStr := strings.TrimSpace(string(output))
	// 简化输出
	uptimeStr = strings.TrimPrefix(uptimeStr, "up ")
	uptimeStr = strings.Split(uptimeStr, ",")[0]

	return uptimeStr, nil
}

// SuspendSystem 挂起系统
func SuspendSystem() (*types.PowerActionResult, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	cmd := exec.Command("systemctl", "suspend")
	output, err := cmd.CombinedOutput()
	result := &types.PowerActionResult{
		Action:    "suspend",
		Timestamp: time.Now().Format(time.RFC3339),
		Output:    string(output),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	return result, nil
}

// HibernateSystem 休眠系统
func HibernateSystem() (*types.PowerActionResult, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	cmd := exec.Command("systemctl", "hibernate")
	output, err := cmd.CombinedOutput()
	result := &types.PowerActionResult{
		Action:    "hibernate",
		Timestamp: time.Now().Format(time.RFC3339),
		Output:    string(output),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	return result, nil
}

// GetPowerInfo 获取电源信息
func GetPowerInfo() (*types.PowerInfo, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	info := &types.PowerInfo{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// 获取电池信息（用于电源管理页面等）
	if batteryInfo, err := getBatteryInfo(); err == nil {
		info.Battery = batteryInfo
		// 兼容前端 Sensors & Power 中对 data.power.percent 的使用
		// 前端会读取 data.power.percent 作为电池电量百分比
		info.Profile = "battery"
	}

	// 获取AC电源状态
	if acStatus, err := getACStatus(); err == nil {
		info.ACPower = acStatus
		if acStatus {
			info.ACStatus = "online"
		} else {
			info.ACStatus = "offline"
		}
	}

	// 获取系统运行时间
	if uptime, err := getSystemUptime(); err == nil {
		info.Uptime = uptime
	}

	// 获取关机状态
	if shutdownStatus, err := GetShutdownStatus(); err == nil {
		info.ShutdownScheduled = shutdownStatus.Scheduled
		info.ScheduledTime = shutdownStatus.ScheduledTime
	}

	return info, nil
}

// getBatteryInfo 获取电池信息
func getBatteryInfo() (*types.BatteryInfo, error) {
	// 尝试从/sys/class/power_supply读取电池信息
	cmd := exec.Command("bash", "-c", "cat /sys/class/power_supply/BAT*/capacity 2>/dev/null || echo ''")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	capacityStr := strings.TrimSpace(string(output))
	if capacityStr == "" {
		return nil, fmt.Errorf("no battery capacity found")
	}

	cmd = exec.Command("bash", "-c", "cat /sys/class/power_supply/BAT*/status 2>/dev/null || echo ''")
	output, err = cmd.Output()
	if err != nil {
		return nil, err
	}

	status := strings.TrimSpace(string(output))

	percentage := 0.0
	fmt.Sscanf(capacityStr, "%f", &percentage)

	return &types.BatteryInfo{
		Present:    true,
		Capacity:   percentage,
		Percentage: percentage,
		Status:     status,
	}, nil
}

// getACStatus 获取AC电源状态
func getACStatus() (bool, error) {
	cmd := exec.Command("bash", "-c", "cat /sys/class/power_supply/AC*/online 2>/dev/null || echo '0'")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	status := strings.TrimSpace(string(output))
	return status == "1", nil
}

// ValidatePowerAction 验证电源操作
func ValidatePowerAction(action string, delayMinutes int) error {
	switch action {
	case "shutdown", "reboot", "suspend", "hibernate", "cancel":
		// 有效操作
	default:
		return fmt.Errorf("invalid power action: %s", action)
	}

	if delayMinutes < 0 {
		return fmt.Errorf("delay minutes cannot be negative")
	}

	if delayMinutes > 1440 { // 24小时
		return fmt.Errorf("delay minutes cannot exceed 1440 (24 hours)")
	}

	return nil
}
