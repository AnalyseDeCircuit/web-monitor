// Package handlers 提供HTTP路由处理器
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"syscall"

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

	if req.PID <= 1 {
		writeJSONError(w, http.StatusBadRequest, "Invalid pid")
		return
	}

	if req.PID == os.Getpid() {
		writeJSONError(w, http.StatusBadRequest, "Refusing to kill current server process")
		return
	}

	if err := syscall.Kill(req.PID, syscall.SIGKILL); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	logs.LogOperation(username, "kill_process", fmt.Sprintf("Killed pid %d", req.PID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
