package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/internal/alerts"
)

// AlertManager 全局告警管理器引用
var AlertManager *alerts.Manager

// SetAlertManager 设置告警管理器
func SetAlertManager(m *alerts.Manager) {
	AlertManager = m
}

// ============================================================================
//  告警配置 API
// ============================================================================

// AlertsConfigHandler 告警配置处理
// @Summary 告警配置
// @Description 获取或更新告警全局配置
// @Tags Alerts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/alerts/config [get]
// @Router /api/alerts/config [put]
func AlertsConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getAlertsConfig(w, r)
	case http.MethodPut:
		updateAlertsConfig(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func getAlertsConfig(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	writeJSON(w, http.StatusOK, AlertManager.GetConfig())
}

func updateAlertsConfig(w http.ResponseWriter, r *http.Request) {
	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "admin only")
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	var cfg alerts.AlertConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := AlertManager.UpdateConfig(cfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "config updated"})
}

// ============================================================================
//  告警规则 API
// ============================================================================

// AlertsRulesHandler 告警规则处理
// @Summary 告警规则
// @Description 获取所有告警规则或创建新规则
// @Tags Alerts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/alerts/rules [get]
// @Router /api/alerts/rules [post]
func AlertsRulesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getAlertsRules(w, r)
	case http.MethodPost:
		createAlertRule(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func getAlertsRules(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	writeJSON(w, http.StatusOK, AlertManager.GetRules())
}

func createAlertRule(w http.ResponseWriter, r *http.Request) {
	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "admin only")
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	var rule alerts.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := AlertManager.CreateRule(&rule); err != nil {
		if err == alerts.ErrRuleExists {
			writeJSONError(w, http.StatusConflict, err.Error())
		} else {
			writeJSONError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, http.StatusOK, rule)
}

// AlertsRuleHandler 单个告警规则处理
// @Summary 单个告警规则
// @Description 获取、更新或删除单个告警规则
// @Tags Alerts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "规则 ID"
// @Router /api/alerts/rules/{id} [get]
// @Router /api/alerts/rules/{id} [put]
// @Router /api/alerts/rules/{id} [delete]
func AlertsRuleHandler(w http.ResponseWriter, r *http.Request) {
	// 从路径提取 rule ID
	// /api/alerts/rules/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/alerts/rules/")
	ruleID := strings.Split(path, "/")[0]

	if ruleID == "" {
		writeJSONError(w, http.StatusBadRequest, "rule ID required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		getAlertRule(w, r, ruleID)
	case http.MethodPut:
		updateAlertRule(w, r, ruleID)
	case http.MethodDelete:
		deleteAlertRule(w, r, ruleID)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func getAlertRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	rule, err := AlertManager.GetRule(ruleID)
	if err != nil {
		if err == alerts.ErrRuleNotFound {
			writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, rule)
}

func updateAlertRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "admin only")
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	var rule alerts.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := AlertManager.UpdateRule(ruleID, &rule); err != nil {
		if err == alerts.ErrRuleNotFound {
			writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			writeJSONError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "rule updated"})
}

func deleteAlertRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "admin only")
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	if err := AlertManager.DeleteRule(ruleID); err != nil {
		if err == alerts.ErrRuleNotFound {
			writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "rule deleted"})
}

// ============================================================================
//  规则启用/禁用 API
// ============================================================================

// AlertsRuleEnableHandler 启用/禁用规则
// @Summary 启用或禁用规则
// @Description 启用或禁用指定的告警规则
// @Tags Alerts
// @Produce json
// @Security BearerAuth
// @Param id path string true "规则 ID"
// @Router /api/alerts/rules/{id}/enable [post]
// @Router /api/alerts/rules/{id}/disable [post]
func AlertsRuleEnableHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "admin only")
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	// /api/alerts/rules/{id}/enable 或 /api/alerts/rules/{id}/disable
	path := strings.TrimPrefix(r.URL.Path, "/api/alerts/rules/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	ruleID := parts[0]
	action := parts[1]

	var err error
	switch action {
	case "enable":
		err = AlertManager.EnableRule(ruleID)
	case "disable":
		err = AlertManager.DisableRule(ruleID)
	default:
		writeJSONError(w, http.StatusBadRequest, "invalid action")
		return
	}

	if err != nil {
		if err == alerts.ErrRuleNotFound {
			writeJSONError(w, http.StatusNotFound, err.Error())
		} else {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "rule " + action + "d"})
}

// ============================================================================
//  预设规则组 API
// ============================================================================

// AlertsPresetsHandler 获取预设组列表
// @Summary 获取预设组
// @Description 获取所有可用的告警规则预设组
// @Tags Alerts
// @Produce json
// @Security BearerAuth
// @Router /api/alerts/presets [get]
func AlertsPresetsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	writeJSON(w, http.StatusOK, alerts.BuiltinPresets)
}

// AlertsPresetEnableHandler 启用预设组
// @Summary 启用预设组
// @Description 一键启用预设组中的所有规则
// @Tags Alerts
// @Produce json
// @Security BearerAuth
// @Param id path string true "预设组 ID"
// @Router /api/alerts/presets/{id}/enable [post]
func AlertsPresetEnableHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "admin only")
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	// /api/alerts/presets/{id}/enable
	path := strings.TrimPrefix(r.URL.Path, "/api/alerts/presets/")
	presetID := strings.TrimSuffix(path, "/enable")

	if err := AlertManager.EnablePreset(presetID); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "preset enabled"})
}

// AlertsDisableAllHandler 禁用所有规则
// @Summary 禁用所有规则
// @Description 一键禁用所有告警规则
// @Tags Alerts
// @Produce json
// @Security BearerAuth
// @Router /api/alerts/disable-all [post]
func AlertsDisableAllHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "admin only")
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	if err := AlertManager.DisableAllRules(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "all rules disabled"})
}

// ============================================================================
//  告警历史 API
// ============================================================================

// AlertsHistoryHandler 告警历史
// @Summary 获取告警历史
// @Description 获取告警事件历史，支持分页和过滤
// @Tags Alerts
// @Produce json
// @Security BearerAuth
// @Param rule_id query string false "按规则 ID 过滤"
// @Param status query string false "按状态过滤 (firing/resolved)"
// @Param severity query string false "按严重级别过滤 (warning/critical)"
// @Param limit query int false "每页数量" default(50)
// @Param offset query int false "偏移量" default(0)
// @Router /api/alerts/history [get]
func AlertsHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	query := alerts.AlertHistoryQuery{
		RuleID: r.URL.Query().Get("rule_id"),
		Limit:  50,
		Offset: 0,
	}

	if status := r.URL.Query().Get("status"); status != "" {
		query.Status = alerts.AlertStatus(status)
	}
	if severity := r.URL.Query().Get("severity"); severity != "" {
		query.Severity = alerts.Severity(severity)
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			query.Limit = l
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			query.Offset = o
		}
	}
	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			query.Since = &t
		}
	}

	writeJSON(w, http.StatusOK, AlertManager.GetHistory(query))
}

// AlertsActiveHandler 活跃告警
// @Summary 获取活跃告警
// @Description 获取当前正在触发的告警
// @Tags Alerts
// @Produce json
// @Security BearerAuth
// @Router /api/alerts/active [get]
func AlertsActiveHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	writeJSON(w, http.StatusOK, AlertManager.GetActiveAlerts())
}

// AlertsSummaryHandler 告警摘要
// @Summary 获取告警摘要
// @Description 获取告警系统的统计摘要
// @Tags Alerts
// @Produce json
// @Security BearerAuth
// @Router /api/alerts/summary [get]
func AlertsSummaryHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	writeJSON(w, http.StatusOK, AlertManager.GetSummary())
}

// ============================================================================
//  测试通知 API
// ============================================================================

// AlertsTestHandler 测试通知
// @Summary 发送测试通知
// @Description 向指定渠道发送测试告警通知
// @Tags Alerts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param channel query string true "通知渠道 (webhook/email/dashboard)"
// @Router /api/alerts/test [post]
func AlertsTestHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "admin only")
		return
	}

	if AlertManager == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alert manager not initialized")
		return
	}

	channel := r.URL.Query().Get("channel")
	if channel == "" {
		channel = "webhook"
	}

	if err := AlertManager.TestNotification(channel); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "test notification sent"})
}

// ============================================================================
//  支持的指标列表 API
// ============================================================================

// AlertsMetricsHandler 获取支持的指标
// @Summary 获取支持的指标
// @Description 获取可用于告警规则的指标列表
// @Tags Alerts
// @Produce json
// @Security BearerAuth
// @Router /api/alerts/metrics [get]
func AlertsMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	metrics := []map[string]string{
		{"id": "cpu", "name": "CPU 使用率", "unit": "%", "description": "CPU 总体使用率百分比"},
		{"id": "memory", "name": "内存使用率", "unit": "%", "description": "内存使用率百分比"},
		{"id": "disk", "name": "磁盘使用率", "unit": "%", "description": "根分区磁盘使用率百分比"},
		{"id": "swap", "name": "Swap 使用率", "unit": "%", "description": "Swap 空间使用率百分比"},
		{"id": "load1", "name": "1分钟负载", "unit": "", "description": "系统 1 分钟平均负载"},
		{"id": "load5", "name": "5分钟负载", "unit": "", "description": "系统 5 分钟平均负载"},
		{"id": "load15", "name": "15分钟负载", "unit": "", "description": "系统 15 分钟平均负载"},
	}

	writeJSON(w, http.StatusOK, metrics)
}
