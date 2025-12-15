// Package handlers 提供HTTP路由处理器
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"syscall"

	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
)

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
