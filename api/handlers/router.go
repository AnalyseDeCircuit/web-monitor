// Package handlers 提供HTTP路由处理器
package handlers

import (
	"net/http"
	"strings"

	"github.com/AnalyseDeCircuit/web-monitor/internal/websocket"
)

// Router 封装HTTP路由器
type Router struct {
	mux *http.ServeMux
}

// NewRouter 创建新的路由器
func NewRouter() *Router {
	return &Router{
		mux: http.NewServeMux(),
	}
}

// SetupRouter 设置所有路由
func SetupRouter() *Router {
	router := NewRouter()

	// 认证路由
	router.mux.HandleFunc("/api/login", LoginHandler)
	router.mux.HandleFunc("/api/logout", LogoutHandler)
	router.mux.HandleFunc("/api/password", ChangePasswordHandler)
	router.mux.HandleFunc("/api/validate-password", ValidatePasswordHandler)

	// 用户管理路由
	router.mux.HandleFunc("/api/users", UsersHandler)
	router.mux.HandleFunc("/api/logs", LogsHandler)

	// 监控数据路由
	router.mux.HandleFunc("/api/info", StaticInfoHandler)            // Static system information for header
	router.mux.HandleFunc("/api/system/info", SystemInfoHandler)     // Real-time monitoring data
	router.mux.HandleFunc("/api/alerts", AlertsHandler)              // Legacy-compatible alerts config
	router.mux.HandleFunc("/api/power/profile", PowerProfileHandler) // Legacy-compatible power profile
	router.mux.HandleFunc("/api/docker/containers", DockerContainersHandler)
	router.mux.HandleFunc("/api/docker/images", DockerImagesHandler)
	router.mux.HandleFunc("/api/docker/action", DockerActionHandler)
	router.mux.HandleFunc("/api/systemd/services", SystemdServicesHandler)
	router.mux.HandleFunc("/api/systemd/action", SystemdActionHandler)
	router.mux.HandleFunc("/api/network/info", NetworkInfoHandler)
	router.mux.HandleFunc("/api/power/info", PowerInfoHandler)
	router.mux.HandleFunc("/api/cache/info", CacheInfoHandler)
	router.mux.HandleFunc("/api/health", HealthCheckHandler)
	router.mux.HandleFunc("/api/metrics", PrometheusMetricsHandler)

	// 电源操作路由
	router.mux.HandleFunc("/api/power/action", PowerActionHandler)
	router.mux.HandleFunc("/api/power/shutdown-status", ShutdownStatusHandler)

	// Cron任务路由
	router.mux.HandleFunc("/api/cron", CronLegacyHandler)
	router.mux.HandleFunc("/api/cron/jobs", CronJobsHandler)
	router.mux.HandleFunc("/api/cron/action", CronActionHandler)
	router.mux.HandleFunc("/api/cron/logs", CronLogsHandler)

	// Process 管理路由（仅管理员）
	router.mux.HandleFunc("/api/process/kill", ProcessKillHandler)

	// WebSocket路由
	router.mux.HandleFunc("/ws/stats", websocket.HandleWebSocket)

	// 静态文件服务
	fs := http.FileServer(http.Dir("./templates"))
	router.mux.Handle("/assets/", fs)
	router.mux.Handle("/css/", fs)
	router.mux.Handle("/js/", fs)
	router.mux.Handle("/sw.js", fs)

	// 主页
	router.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, "./templates/index.html")
	})

	// 登录页面
	router.mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, "./templates/login.html")
	})

	return router
}

func wrapWithAPIAuthorization(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/") {
			// Public endpoints
			switch path {
			case "/api/login", "/api/validate-password", "/api/health", "/api/metrics", "/api/logout":
				next.ServeHTTP(w, r)
				return
			}

			_, role, ok := requireAuth(w, r)
			if !ok {
				return
			}

			// Admin-only endpoints (defense in depth)
			adminOnly := false
			switch path {
			case "/api/users", "/api/logs", "/api/docker/action", "/api/systemd/action", "/api/power/action", "/api/cron/action", "/api/cron/logs", "/api/process/kill":
				adminOnly = true
			case "/api/cron":
				adminOnly = (r.Method != http.MethodGet)
			}
			if adminOnly && role != "admin" {
				writeJSONError(w, http.StatusForbidden, "Forbidden: Admin access required")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// Start 启动HTTP服务器
func (r *Router) Start(addr string) error {
	return http.ListenAndServe(addr, wrapWithAPIAuthorization(r.mux))
}
