// Package handlers 提供HTTP路由处理器
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
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
func SystemInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
func DockerContainersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	containers, err := docker.ListContainers()
	if err != nil {
		http.Error(w, "Failed to get Docker containers: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"containers": containers,
	})
}

// DockerImagesHandler 处理Docker镜像请求
func DockerImagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	images, err := docker.ListImages()
	if err != nil {
		http.Error(w, "Failed to get Docker images: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"images": images,
	})
}

// DockerActionHandler 处理Docker操作请求
func DockerActionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := docker.ContainerAction(request.ID, request.Action); err != nil {
		http.Error(w, "Docker action failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Docker action completed",
	})
}

// SystemdServicesHandler 处理Systemd服务请求
func SystemdServicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	services, err := systemd.ListServices()
	if err != nil {
		http.Error(w, "Failed to get Systemd services: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// NetworkInfoHandler 处理网络信息请求
func NetworkInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取网络接口信息
	interfaces, err := network.GetNetworkInterfaces()
	if err != nil {
		http.Error(w, "Failed to get network info: "+err.Error(), http.StatusInternalServerError)
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info, err := power.GetPowerInfo()
	if err != nil {
		http.Error(w, "Failed to get power info: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// PrometheusMetricsHandler 处理Prometheus指标请求
func PrometheusMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	handler := prometheus.GetMetricsHandler()
	handler.ServeHTTP(w, r)
}

// CacheInfoHandler 处理缓存信息请求
func CacheInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"version": "1.0.0",
		"message": "Web Monitor is running",
	})
}

// NetworkDiagnosticsHandler 处理网络诊断请求
func NetworkDiagnosticsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Action string `json:"action"`
		Target string `json:"target"`
		Count  int    `json:"count,omitempty"`
		Ports  []int  `json:"ports,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var result interface{}
	var err error

	switch request.Action {
	case "ping":
		count := 4
		if request.Count > 0 {
			count = request.Count
		}
		result, err = network.PingTarget(request.Target, count)
	case "traceroute":
		result, err = network.TracerouteTarget(request.Target, 30)
	case "portscan":
		ports := request.Ports
		if len(ports) == 0 {
			// 默认扫描常用端口
			ports = []int{22, 80, 443, 8080, 3306, 5432}
		}
		result, err = network.PortScan(request.Target, ports, 2*time.Second)
	case "dns":
		result, err = network.DNSLookup(request.Target, "A")
	case "interfaces":
		result, err = network.GetNetworkInterfaces()
	default:
		http.Error(w, "Invalid network action: "+request.Action, http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Network diagnostic failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// PowerActionHandler 处理电源操作请求
func PowerActionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Action string `json:"action"`
		Delay  int    `json:"delay,omitempty"`
		Reason string `json:"reason,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
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
		http.Error(w, "Invalid power action: "+request.Action, http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Power action failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ShutdownStatusHandler 处理关机状态请求
func ShutdownStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status, err := power.GetShutdownStatus()
	if err != nil {
		http.Error(w, "Failed to get shutdown status: "+err.Error(), http.StatusInternalServerError)
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
			// 保持与前端错误处理兼容，返回非200状态
			http.Error(w, "Failed to get cron jobs: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobs)
	case http.MethodPost:
		username, role, err := getUserAndRoleFromRequest(r)
		if err != nil || role != "admin" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Forbidden: Admin access required",
			})
			return
		}

		var jobs []types.CronJob
		if err := json.NewDecoder(r.Body).Decode(&jobs); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Invalid request body",
			})
			return
		}

		if err := cron.SaveCronJobs(jobs); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}

		// 记录操作日志
		logs.LogOperation(username, "cron_update", "Updated crontab", r.RemoteAddr)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// CronJobsHandler 处理Cron任务请求
func CronJobsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobs, err := cron.ListCronJobs()
	if err != nil {
		http.Error(w, "Failed to get cron jobs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

// CronActionHandler 处理Cron操作请求
func CronActionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
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
		http.Error(w, "Invalid cron action: "+request.Action, http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Cron action failed: "+err.Error(), http.StatusInternalServerError)
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lines := 50
	if linesParam := r.URL.Query().Get("lines"); linesParam != "" {
		fmt.Sscanf(linesParam, "%d", &lines)
	}

	logs, err := cron.GetCronLogs(lines)
	if err != nil {
		http.Error(w, "Failed to get cron logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(logs))
}

// StaticInfoHandler 处理静态系统信息请求
func StaticInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := system.GetStaticInfo()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
