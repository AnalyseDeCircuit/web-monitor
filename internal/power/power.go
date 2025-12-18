// Package power 提供电源管理功能
package power

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/config"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

var (
	powerMutex sync.Mutex
)

const (
	powerCommandTimeout = 10 * time.Second
)

func sanitizeReason(reason string) string {
	reason = strings.TrimSpace(reason)
	reason = strings.ReplaceAll(reason, "\r", " ")
	reason = strings.ReplaceAll(reason, "\n", " ")
	if len(reason) > 200 {
		reason = reason[:200]
	}
	return reason
}

func hostCmdContext(ctx context.Context, args ...string) *exec.Cmd {
	hostFS := config.Load().HostFS
	if hostFS != "" {
		return exec.CommandContext(ctx, "chroot", append([]string{hostFS}, args...)...)
	}
	return exec.CommandContext(ctx, args[0], args[1:]...)
}

func isExecNotFound(err error) bool {
	// Covers common cases: exec.ErrNotFound and "no such file" from chroot.
	if err == nil {
		return false
	}
	return errors.Is(err, exec.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "no such file")
}

// normalizeProfile maps user-friendly names to powerprofilesctl values.
func normalizeProfile(profile string) string {
	p := strings.ToLower(strings.TrimSpace(profile))
	switch p {
	case "powersaver", "power-saver", "powersave":
		return "power-saver"
	case "balanced":
		return "balanced"
	case "performance":
		return "performance"
	default:
		return ""
	}
}

// ShutdownSystem 关闭系统
func ShutdownSystem(delayMinutes int, reason string) (*types.PowerActionResult, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	if delayMinutes < 0 {
		return nil, fmt.Errorf("delay minutes cannot be negative")
	}

	var cmd *exec.Cmd
	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()
	reason = sanitizeReason(reason)
	if delayMinutes == 0 {
		cmd = exec.CommandContext(ctx, "shutdown", "-h", "now")
	} else {
		cmd = exec.CommandContext(ctx, "shutdown", "-h", fmt.Sprintf("+%d", delayMinutes), reason)
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
	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()
	reason = sanitizeReason(reason)
	if delayMinutes == 0 {
		cmd = exec.CommandContext(ctx, "shutdown", "-r", "now")
	} else {
		cmd = exec.CommandContext(ctx, "shutdown", "-r", fmt.Sprintf("+%d", delayMinutes), reason)
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

	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "shutdown", "-c")
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
	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "shutdown", "-c")
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
	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "uptime", "-p")
	output, err := cmd.Output()
	if err != nil {
		// 尝试另一种格式
		cmd = exec.CommandContext(ctx, "uptime")
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

	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "suspend")
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

	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "hibernate")
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

// GetPowerProfiles returns supported profiles using powerprofilesctl when available.
func GetPowerProfiles() (*types.PowerProfileInfo, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	info := &types.PowerProfileInfo{Available: []string{}, Supported: false}

	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()

	// current mode via `powerprofilesctl get`
	getCmd := hostCmdContext(ctx, "powerprofilesctl", "get")
	getOut, err := getCmd.CombinedOutput()
	if err == nil {
		info.Current = normalizeProfile(strings.TrimSpace(string(getOut)))
	} else if isExecNotFound(err) {
		info.Error = "powerprofilesctl not found"
		return info, nil
	}

	// available via `powerprofilesctl list`
	listCmd := hostCmdContext(ctx, "powerprofilesctl", "list")
	out, err := listCmd.CombinedOutput()
	if err != nil {
		if isExecNotFound(err) {
			info.Error = "powerprofilesctl not found"
			return info, nil
		}
		info.Error = strings.TrimSpace(string(out))
		return info, nil
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		orig := line
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Profile lines look like "  performance:" or "* power-saver:"
		// Skip indented property lines (e.g., "    CpuDriver:  intel_pstate")
		if strings.HasPrefix(orig, "    ") || strings.HasPrefix(orig, "\t\t") {
			continue
		}
		// Strip leading * and trailing :
		cleaned := strings.TrimPrefix(line, "*")
		cleaned = strings.TrimSpace(cleaned)
		cleaned = strings.TrimSuffix(cleaned, ":")
		p := normalizeProfile(cleaned)
		if p == "" {
			continue
		}
		info.Available = append(info.Available, p)
		if strings.HasPrefix(line, "*") {
			info.Current = p
		}
	}

	if len(info.Available) == 0 {
		info.Error = "no profiles detected"
		return info, nil
	}
	info.Supported = true
	if info.Current == "" {
		info.Current = info.Available[0]
	}
	return info, nil
}

// SetPowerProfile switches profile via powerprofilesctl.
func SetPowerProfile(profile string) error {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	norm := normalizeProfile(profile)
	if norm == "" {
		return fmt.Errorf("invalid profile")
	}

	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()

	cmd := hostCmdContext(ctx, "powerprofilesctl", "set", norm)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isExecNotFound(err) {
			return fmt.Errorf("powerprofilesctl not found")
		}
		return fmt.Errorf("set failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// GetGUIStatus inspects the display manager service state.
func GetGUIStatus() (*types.GUIStatus, error) {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	status := &types.GUIStatus{Supported: false, Running: false, Service: "display-manager.service"}

	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()

	cmd := hostCmdContext(ctx, "systemctl", "is-active", status.Service)
	out, err := cmd.CombinedOutput()
	txt := strings.TrimSpace(string(out))
	if err != nil {
		if isExecNotFound(err) {
			status.Error = "systemctl not available"
			return status, nil
		}
		// If unit missing, treat as unsupported.
		if strings.Contains(txt, "could not be found") || strings.Contains(txt, "not-found") {
			status.Error = txt
			return status, nil
		}
	}

	status.Supported = true
	status.Running = (txt == "active")

	// default target like graphical.target or multi-user.target
	defCmd := hostCmdContext(ctx, "systemctl", "get-default")
	defOut, err := defCmd.CombinedOutput()
	if err == nil {
		status.DefaultTarget = strings.TrimSpace(string(defOut))
	}
	return status, nil
}

// SetGUIRunning toggles the display manager service.
func SetGUIRunning(enable bool) error {
	powerMutex.Lock()
	defer powerMutex.Unlock()

	action := "stop"
	if enable {
		action = "start"
	}

	ctx, cancel := context.WithTimeout(context.Background(), powerCommandTimeout)
	defer cancel()

	cmd := hostCmdContext(ctx, "systemctl", action, "display-manager.service")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isExecNotFound(err) {
			return fmt.Errorf("systemctl not available")
		}
		return fmt.Errorf("%s failed: %s", action, strings.TrimSpace(string(out)))
	}
	return nil
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
	// 尝试从 /sys/class/power_supply 读取电池信息
	paths, err := filepath.Glob("/sys/class/power_supply/BAT*/capacity")
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("no battery capacity found")
	}
	capBytes, err := os.ReadFile(paths[0])
	if err != nil {
		return nil, err
	}
	capacityStr := strings.TrimSpace(string(capBytes))
	if capacityStr == "" {
		return nil, fmt.Errorf("no battery capacity found")
	}
	capVal, err := strconv.ParseFloat(capacityStr, 64)
	if err != nil {
		return nil, err
	}

	statusPaths, _ := filepath.Glob("/sys/class/power_supply/BAT*/status")
	status := ""
	if len(statusPaths) > 0 {
		if b, err := os.ReadFile(statusPaths[0]); err == nil {
			status = strings.TrimSpace(string(b))
		}
	}

	return &types.BatteryInfo{
		Present:    true,
		Capacity:   capVal,
		Percentage: capVal,
		Status:     status,
	}, nil
}

// getACStatus 获取AC电源状态
func getACStatus() (bool, error) {
	paths, err := filepath.Glob("/sys/class/power_supply/AC*/online")
	if err != nil || len(paths) == 0 {
		return false, fmt.Errorf("no AC status found")
	}
	b, err := os.ReadFile(paths[0])
	if err != nil {
		return false, err
	}
	status := strings.TrimSpace(string(b))
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
