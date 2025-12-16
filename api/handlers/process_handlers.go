// Package handlers 提供HTTP路由处理器
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"regexp"
	"runtime"
	"syscall"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
	"github.com/shirou/gopsutil/v3/process"
)

// ProcessIOHandler 获取单个进程的 IO 统计（懒加载）
// GET /api/process/io?pid=1234
func ProcessIOHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	pidStr := r.URL.Query().Get("pid")
	if pidStr == "" {
		writeJSONError(w, http.StatusBadRequest, "Missing pid parameter")
		return
	}

	var pid int
	if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil || pid <= 0 {
		writeJSONError(w, http.StatusBadRequest, "Invalid pid")
		return
	}

	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "Process not found")
		return
	}

	ioRead := "-"
	ioWrite := "-"
	if ioCounters, err := proc.IOCounters(); err == nil {
		ioRead = formatSize(ioCounters.ReadBytes)
		ioWrite = formatSize(ioCounters.WriteBytes)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"io_read":  ioRead,
		"io_write": ioWrite,
	})
}

// formatSize converts bytes to human-readable format
func formatSize(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ProcessKillHandler 终止指定 PID 的进程（仅管理员）
// POST /api/process/kill {"pid":1234}
func ProcessKillHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	username, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		PID int `json:"pid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 验证PID格式
	if req.PID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "Invalid PID")
		return
	}

	// 禁止终止系统关键进程
	if req.PID == 1 {
		writeJSONError(w, http.StatusForbidden, "Refusing to kill system init process (PID 1)")
		return
	}

	// 禁止终止当前服务器进程
	if req.PID == os.Getpid() {
		writeJSONError(w, http.StatusForbidden, "Refusing to kill current server process")
		return
	}

	// 禁止终止kernel线程 (PID < 1000)
	if runtime.GOOS == "linux" && req.PID < 1000 {
		writeJSONError(w, http.StatusForbidden, "Refusing to kill kernel or system processes")
		return
	}

	// 获取进程信息
	proc, err := process.NewProcess(int32(req.PID))
	if err != nil {
		// 进程不存在
		writeJSONError(w, http.StatusNotFound, "Process not found")
		return
	}

	// 检查进程是否存在 (避免竞争条件)
	if exists, _ := proc.IsRunning(); !exists {
		writeJSONError(w, http.StatusNotFound, "Process not found or already terminated")
		return
	}

	// 获取当前操作用户信息
	currentUser, err := user.Current()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Unable to verify current user")
		return
	}

	// 获取进程所有者
	procUsername, err := proc.Username()
	if err != nil {
		// 无法获取进程信息，表示没有权限
		writeJSONError(w, http.StatusForbidden, "Permission denied: unable to access process information")
		return
	}

	// 检查是否为root用户
	if currentUser.Uid != "0" && currentUser.Username != "root" {
		// 如果进程属于其他用户，拒绝操作
		if procUsername != currentUser.Username {
			writeJSONError(w, http.StatusForbidden, fmt.Sprintf("Permission denied: process belongs to user '%s'", procUsername))
			return
		}
	}

	// 获取命令行和进程名，用于日志和验证
	procName, _ := proc.Name()
	cmdline, _ := proc.Cmdline()
	ppid, _ := proc.Ppid()

	// 禁止终止父进程 (防止杀死整个进程树)
	if ppid == int32(os.Getpid()) {
		writeJSONError(w, http.StatusForbidden, "Refusing to kill child process of current server")
		return
	}

	// 验证进程名，防止误操作系统服务
	dangerousPatterns := []string{
		`^systemd$`,
		`^dockerd$`,
		`^containerd$`,
		`^sshd$`,
		`^init$`,
		`^kernel`,
	}

	for _, pattern := range dangerousPatterns {
		if matched, _ := regexp.MatchString(pattern, procName); matched {
			writeJSONError(w, http.StatusForbidden, fmt.Sprintf("Refusing to kill system service: %s", procName))
			return
		}
	}

	// 执行进程终止
	// 先尝试SIGTERM，等待5秒后强制SIGKILL
	err = syscall.Kill(req.PID, syscall.SIGTERM)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to send SIGTERM: %s", err.Error()))
		return
	}

	// 异步等待进程退出
	go func() {
		time.Sleep(5 * time.Second)
		if proc, err := process.NewProcess(int32(req.PID)); err == nil {
			if isRunning, _ := proc.IsRunning(); isRunning {
				// 进程仍在运行，发送SIGKILL
				syscall.Kill(req.PID, syscall.SIGKILL)
			}
		}
	}()

	// 记录操作日志
	logMsg := fmt.Sprintf("Killed PID %d (%s)", req.PID, procName)
	if cmdline != "" {
		// 限制命令行长度
		if len(cmdline) > 100 {
			cmdline = cmdline[:100] + "..."
		}
		logMsg += fmt.Sprintf(" - %s", cmdline)
	}

	logs.LogOperation(username, "kill_process", logMsg, r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"pid":    req.PID,
		"name":   procName,
	})
}
