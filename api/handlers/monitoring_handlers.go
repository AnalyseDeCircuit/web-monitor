// Package handlers 提供HTTP路由处理器
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/auth"
	"github.com/AnalyseDeCircuit/web-monitor/internal/cache"
	"github.com/AnalyseDeCircuit/web-monitor/internal/cron"
	"github.com/AnalyseDeCircuit/web-monitor/internal/docker"
	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
	"github.com/AnalyseDeCircuit/web-monitor/internal/network"
	"github.com/AnalyseDeCircuit/web-monitor/internal/power"
	"github.com/AnalyseDeCircuit/web-monitor/internal/prometheus"
	"github.com/AnalyseDeCircuit/web-monitor/internal/system"
	"github.com/AnalyseDeCircuit/web-monitor/internal/systemd"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

// SystemInfoHandler 处理系统信息请求
// @Summary 获取系统信息
// @Description 获取完整的系统监控信息，包括系统指标、Docker容器、Systemd服务、网络状态、SSH统计等
// @Tags Monitoring
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} types.Response "系统信息"
// @Failure 401 {object} map[string]string "未授权"
// @Router /api/info [get]
func SystemInfoHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	response := map[string]interface{}{}

	// 获取系统指标（直接调用，不超时）
	metrics, err := monitoring.GlobalMonitoringService.GetSystemMetrics()
	if err != nil {
		response["system_metrics_error"] = err.Error()
		response["system_metrics"] = &types.SystemMetrics{}
	} else {
		response["system_metrics"] = metrics
	}

	// 获取Docker容器
	containers, err := docker.ListContainers()
	if err != nil {
		response["docker_error"] = err.Error()
		containers = []types.DockerContainer{}
	}
	response["docker"] = map[string]interface{}{
		"containers": containers,
	}

	// 获取Systemd服务
	services, err := systemd.ListServices()
	if err != nil {
		response["systemd_error"] = err.Error()
		services = []types.ServiceInfo{}
	}
	response["systemd"] = map[string]interface{}{
		"services": services,
	}

	// 获取网络信息
	// 尝试获取 SSH 统计信息
	sshStats := network.GetSSHStats()
	response["ssh_stats"] = sshStats

	// 获取网络信息
	netInfo, err := network.GetNetworkInfo()
	if err != nil {
		response["network_error"] = err.Error()
		response["network"] = types.NetInfo{}
	} else {
		response["network"] = netInfo
	}

	// 获取电源信息
	powerInfo, err := power.GetPowerInfo()
	if err != nil {
		response["power_error"] = err.Error()
		powerInfo = &types.PowerInfo{}
	}
	response["power"] = powerInfo

	// 缓存信息
	response["cache"] = map[string]interface{}{
		"size": cache.GlobalMetricsCache.Size(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DockerContainersHandler 处理Docker容器请求
// DockerContainersHandler 处理Docker容器列表请求
// @Summary 获取Docker容器列表
// @Description 获取所有Docker容器的信息
// @Tags Docker
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} object{containers=[]types.DockerContainer} "容器列表"
// @Failure 401 {object} map[string]string "未授权"
// @Router /api/docker/containers [get]
func DockerContainersHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	containers, err := docker.ListContainers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to get Docker containers: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"containers": containers,
	})
}

// DockerImagesHandler 处理Docker镜像请求
func DockerImagesHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	images, err := docker.ListImages()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to get Docker images: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"images": images,
	})
}

// DockerActionHandler 处理Docker操作请求
func DockerActionHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	username, role, err := getUserAndRoleFromRequest(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Unauthorized",
		})
		return
	}
	if role != "admin" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Forbidden: Admin access required",
		})
		return
	}

	var request struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if err := docker.ContainerAction(request.ID, request.Action); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Docker action failed: "+err.Error())
		return
	}

	// 记录操作日志
	logs.LogOperation(username, "docker_action", fmt.Sprintf("%s %s", request.Action, request.ID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Docker action completed",
	})
}

// DockerLogsHandler 获取容器日志（仅管理员）
// GET /api/docker/logs?id=container_id&tail=200
func DockerLogsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	username, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	containerID := strings.TrimSpace(r.URL.Query().Get("id"))
	if containerID == "" {
		writeJSONError(w, http.StatusBadRequest, "Missing id")
		return
	}

	// 默认尾部 200 行，可通过 tail 参数调整，限制上限避免 OOM
	tail := 200
	if v := strings.TrimSpace(r.URL.Query().Get("tail")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 2000 {
				n = 2000
			}
			tail = n
		}
	}

	logsText, err := docker.GetContainerLogs(containerID, tail)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to fetch logs: "+err.Error())
		return
	}

	logs.LogOperation(username, "docker_logs", fmt.Sprintf("fetch logs tail=%d for %s", tail, containerID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"logs": logsText,
	})
}

// DockerPruneHandler 清理未使用的 Docker 资源（仅管理员）
func DockerPruneHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	username, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	result, err := docker.PruneSystem()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Docker prune failed: "+err.Error())
		return
	}

	// 记录操作日志
	logs.LogOperation(username, "docker_prune", "Pruned unused Docker resources", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Docker prune completed",
		"result":  result,
	})
}

// DockerImageRemoveHandler 删除Docker镜像（仅管理员）
func DockerImageRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	username, role, err := getUserAndRoleFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if role != "admin" {
		writeJSONError(w, http.StatusForbidden, "Forbidden: Admin access required")
		return
	}

	var request struct {
		ID      string `json:"id"`
		Force   bool   `json:"force,omitempty"`
		NoPrune bool   `json:"noprune,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if request.ID == "" {
		writeJSONError(w, http.StatusBadRequest, "Missing image id")
		return
	}

	if err := docker.RemoveImage(request.ID, request.Force, request.NoPrune); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Docker image remove failed: "+err.Error())
		return
	}

	logs.LogOperation(username, "docker_image_remove", fmt.Sprintf("remove %s", request.ID), r.RemoteAddr)

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Docker image removed",
	})
}

// SystemdServicesHandler 处理Systemd服务请求
// SystemdServicesHandler 处理Systemd服务列表请求
// @Summary 获取Systemd服务列表
// @Description 获取系统中所有Systemd服务的状态
// @Tags Systemd
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} object{services=[]types.ServiceInfo} "服务列表"
// @Failure 401 {object} map[string]string "未授权"
// @Router /api/systemd/services [get]
func SystemdServicesHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	services, err := systemd.ListServices()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to get Systemd services: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// SystemdActionHandler 处理 Systemd 服务操作请求
func SystemdActionHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	username, role, err := getUserAndRoleFromRequest(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Unauthorized",
		})
		return
	}
	if role != "admin" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Forbidden: Admin access required",
		})
		return
	}

	var req struct {
		Unit   string `json:"unit"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	allowedActions := map[string]bool{
		"start":   true,
		"stop":    true,
		"restart": true,
		"reload":  true,
		"enable":  true,
		"disable": true,
	}
	if !allowedActions[req.Action] {
		writeJSONError(w, http.StatusBadRequest, "Invalid action")
		return
	}

	if err := systemd.ServiceAction(req.Unit, req.Action); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 记录操作日志
	logs.LogOperation(username, "systemd_action", fmt.Sprintf("%s %s", req.Action, req.Unit), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// NetworkInfoHandler 处理网络信息请求
func NetworkInfoHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	// 获取网络接口信息
	interfaces, err := network.GetNetworkInterfaces()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to get network info: "+err.Error())
		return
	}

	// 创建简单的网络信息响应
	info := map[string]interface{}{
		"interfaces": interfaces,
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// PowerInfoHandler 处理电源信息请求
func PowerInfoHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	info, err := power.GetPowerInfo()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to get power info: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// PrometheusMetricsHandler 处理Prometheus指标请求
// PrometheusMetricsHandler Prometheus指标导出
// @Summary Prometheus指标
// @Description 以Prometheus格式导出系统指标
// @Tags Monitoring
// @Produce plain
// @Success 200 {string} string "Prometheus指标文本"
// @Router /api/metrics [get]
func PrometheusMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		// metrics 输出为 text/plain；这里保持简单且明确
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("Method not allowed"))
		return
	}

	handler := prometheus.GetMetricsHandler()
	handler.ServeHTTP(w, r)
}

// CacheInfoHandler 处理缓存信息请求
func CacheInfoHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	info := map[string]interface{}{
		"size":  cache.GlobalMetricsCache.Size(),
		"keys":  cache.GlobalMetricsCache.Keys(),
		"stats": "Cache system is operational",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// HealthCheckHandler 处理健康检查请求
// HealthCheckHandler 健康检查
// @Summary 健康检查
// @Description 返回服务健康状态，用于容器编排健康探针
// @Tags Monitoring
// @Produce json
// @Success 200 {object} map[string]string "健康状态"
// @Router /api/health [get]
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "healthy",
		"version": "1.0.0",
		"message": "Web Monitor is running",
	})
}

// PowerActionHandler 处理电源操作请求
func PowerActionHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	// power action 有副作用：按约定改为 admin-only
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	var request struct {
		Action string `json:"action"`
		Delay  int    `json:"delay,omitempty"`
		Reason string `json:"reason,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	var result *types.PowerActionResult
	var err error

	switch request.Action {
	case "shutdown":
		result, err = power.ShutdownSystem(request.Delay, request.Reason)
	case "reboot":
		result, err = power.RebootSystem(request.Delay, request.Reason)
	case "cancel":
		result, err = power.CancelShutdown()
	case "suspend":
		result, err = power.SuspendSystem()
	case "hibernate":
		result, err = power.HibernateSystem()
	default:
		writeJSONError(w, http.StatusBadRequest, "Invalid power action: "+request.Action)
		return
	}

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Power action failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ShutdownStatusHandler 处理关机状态请求
func ShutdownStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	status, err := power.GetShutdownStatus()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to get shutdown status: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// getUserAndRoleFromRequest 从请求中解析JWT并返回用户名和角色
func getUserAndRoleFromRequest(r *http.Request) (string, string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		cookie, err := r.Cookie("auth_token")
		if err != nil {
			return "", "", fmt.Errorf("no auth token")
		}
		authHeader = "Bearer " + cookie.Value
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", "", fmt.Errorf("invalid auth header")
	}

	token := parts[1]
	claims, err := auth.ValidateJWT(token)
	if err != nil {
		return "", "", fmt.Errorf("invalid token")
	}

	username := claims.Subject
	user := auth.GetUserByUsername(username)
	if user == nil {
		return "", "", fmt.Errorf("user not found")
	}

	return username, user.Role, nil
}

// CronLegacyHandler 兼容旧版 /api/cron 接口
// GET: 返回当前 crontab 中的所有任务
// POST: 使用给定的任务列表覆盖 crontab（仅限管理员）
func CronLegacyHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobs, err := cron.ListCronJobs()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to get cron jobs: "+err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobs)
	case http.MethodPost:
		username, role, err := getUserAndRoleFromRequest(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Unauthorized",
			})
			return
		}
		if role != "admin" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Forbidden: Admin access required",
			})
			return
		}

		var jobs []types.CronJob
		if err := json.NewDecoder(r.Body).Decode(&jobs); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if err := cron.SaveCronJobs(jobs); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// 记录操作日志
		logs.LogOperation(username, "cron_update", "Updated crontab", r.RemoteAddr)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
		})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// CronJobsHandler 处理Cron任务请求
func CronJobsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	jobs, err := cron.ListCronJobs()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to get cron jobs: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

// CronActionHandler 处理Cron操作请求
func CronActionHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	// cron action 有副作用：按约定改为 admin-only
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	var request struct {
		Action   string `json:"action"`
		User     string `json:"user,omitempty"`
		JobID    string `json:"job_id,omitempty"`
		Schedule string `json:"schedule,omitempty"`
		Command  string `json:"command,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	var err error
	switch request.Action {
	case "add":
		err = cron.AddCronJob(request.User, request.Schedule, request.Command)
	case "remove":
		err = cron.RemoveCronJob(request.User, request.JobID)
	case "enable":
		err = cron.EnableCronJob(request.User, request.JobID)
	case "disable":
		err = cron.DisableCronJob(request.User, request.JobID)
	default:
		writeJSONError(w, http.StatusBadRequest, "Invalid cron action: "+request.Action)
		return
	}

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Cron action failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Cron action completed",
	})
}

// CronLogsHandler 处理Cron日志请求
func CronLogsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	lines := 50
	if linesParam := r.URL.Query().Get("lines"); linesParam != "" {
		fmt.Sscanf(linesParam, "%d", &lines)
	}

	logs, err := cron.GetCronLogs(lines)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to get cron logs: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(logs))
}

// StaticInfoHandler 处理静态系统信息请求
func StaticInfoHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	info := system.GetStaticInfo()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
