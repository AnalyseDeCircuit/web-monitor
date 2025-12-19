package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
	"github.com/AnalyseDeCircuit/web-monitor/internal/settings"
)

// SystemSettingsHandler 处理系统设置请求
// GET: 获取当前设置
// PUT: 更新设置 (仅管理员)
func SystemSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getSystemSettings(w, r)
	case http.MethodPut:
		updateSystemSettings(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getSystemSettings(w http.ResponseWriter, r *http.Request) {
	// 只需要登录即可查看设置
	_, _, ok := requireAuth(w, r)
	if !ok {
		return
	}

	s := settings.Get()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"monitoringMode": s.MonitoringMode,
	})
}

func updateSystemSettings(w http.ResponseWriter, r *http.Request) {
	// 仅管理员可以修改设置
	username, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	var req struct {
		MonitoringMode string `json:"monitoringMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 验证值
	if req.MonitoringMode != "on-demand" && req.MonitoringMode != "always-on" {
		writeJSONError(w, http.StatusBadRequest, "Invalid monitoringMode: must be 'on-demand' or 'always-on'")
		return
	}

	if err := settings.SetMonitoringMode(req.MonitoringMode); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to save settings: "+err.Error())
		return
	}

	logs.LogOperation(username, "settings_update", "monitoringMode="+req.MonitoringMode, clientIP(r))

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Settings updated",
	})
}
