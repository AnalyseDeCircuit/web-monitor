// Package handlers 提供HTTP路由处理器
package handlers

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/assets"
	"github.com/AnalyseDeCircuit/web-monitor/internal/config"
	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
	"github.com/AnalyseDeCircuit/web-monitor/internal/plugin"
	"github.com/AnalyseDeCircuit/web-monitor/internal/websocket"

	httpSwagger "github.com/swaggo/http-swagger/v2"
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
func SetupRouter(pluginManager *plugin.Manager) *Router {
	router := NewRouter()

	// 认证路由
	router.mux.HandleFunc("/api/login", LoginHandler)
	router.mux.HandleFunc("/api/logout", LogoutHandler)
	router.mux.HandleFunc("/api/password", ChangePasswordHandler)
	router.mux.HandleFunc("/api/validate-password", ValidatePasswordHandler)

	// 用户 Profile 路由
	router.mux.HandleFunc("/api/profile", ProfileHandler)
	router.mux.HandleFunc("/api/profile/sessions", ProfileSessionsHandler)
	router.mux.HandleFunc("/api/profile/preferences", ProfilePreferencesHandler)
	router.mux.HandleFunc("/api/profile/login-history", ProfileLoginHistoryHandler)

	// 用户管理路由
	router.mux.HandleFunc("/api/users", UsersHandler)
	router.mux.HandleFunc("/api/logs", LogsHandler)

	// 监控数据路由
	cfg := config.Load()
	router.mux.HandleFunc("/api/info", StaticInfoHandler)        // Static system information for header
	router.mux.HandleFunc("/api/system/info", SystemInfoHandler) // Real-time monitoring data
	router.mux.HandleFunc("/api/alerts", AlertsHandler)          // Legacy-compatible alerts config
	router.mux.HandleFunc("/api/network/info", NetworkInfoHandler)
	router.mux.HandleFunc("/api/cache/info", CacheInfoHandler)
	router.mux.HandleFunc("/api/health", HealthCheckHandler)
	router.mux.HandleFunc("/api/metrics", PrometheusMetricsHandler)

	if cfg.EnablePower {
		router.mux.HandleFunc("/api/power/profile", PowerProfileHandler) // Legacy-compatible power profile
		router.mux.HandleFunc("/api/power/info", PowerInfoHandler)
		router.mux.HandleFunc("/api/power/action", PowerActionHandler)
		router.mux.HandleFunc("/api/power/shutdown-status", ShutdownStatusHandler)
	}

	router.mux.HandleFunc("/api/gui/status", GUIStatusHandler)
	router.mux.HandleFunc("/api/gui/action", GUIActionHandler)

	if cfg.EnableDocker {
		router.mux.HandleFunc("/api/docker/containers", DockerContainersHandler)
		router.mux.HandleFunc("/api/docker/images", DockerImagesHandler)
		router.mux.HandleFunc("/api/docker/action", DockerActionHandler)
		router.mux.HandleFunc("/api/docker/image/remove", DockerImageRemoveHandler)
		router.mux.HandleFunc("/api/docker/prune", DockerPruneHandler)
		router.mux.HandleFunc("/api/docker/logs", DockerLogsHandler)
	}

	if cfg.EnableSystemd {
		router.mux.HandleFunc("/api/systemd/services", SystemdServicesHandler)
		router.mux.HandleFunc("/api/systemd/action", SystemdActionHandler)
	}

	// Cron任务路由
	if cfg.EnableCron {
		router.mux.HandleFunc("/api/cron", CronLegacyHandler)
		router.mux.HandleFunc("/api/cron/jobs", CronJobsHandler)
		router.mux.HandleFunc("/api/cron/action", CronActionHandler)
		router.mux.HandleFunc("/api/cron/logs", CronLogsHandler)
	}

	// Process 管理路由
	router.mux.HandleFunc("/api/process/io", ProcessIOHandler)     // 懒加载进程 IO 数据
	router.mux.HandleFunc("/api/process/kill", ProcessKillHandler) // 仅管理员

	// SSH stats (manual refresh)
	if cfg.EnableSSH {
		router.mux.HandleFunc("/api/ssh/stats", SSHStatsHandler)
	}

	// 系统设置路由
	router.mux.HandleFunc("/api/settings", SystemSettingsHandler)

	// WebSocket路由
	router.mux.HandleFunc("/ws/stats", websocket.HandleWebSocket)

	// 插件路由
	if pluginManager != nil {
		router.mux.HandleFunc("/api/plugins/list", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			// Auth check
			_, role, ok := requireAuth(w, r)
			if !ok {
				return
			}

			plugins := pluginManager.ListPluginsForRole(role)
			writeJSON(w, http.StatusOK, plugins)
		})

		router.mux.HandleFunc("/api/plugins/action", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			// Auth check (Admin only)
			username, role, ok := requireAuth(w, r)
			if !ok {
				return
			}
			if role != "admin" {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			var req struct {
				Name    string `json:"name"`
				Enabled bool   `json:"enabled"`
			}
			if err := decodeJSONBody(w, r, &req); err != nil {
				return
			}

			if err := pluginManager.TogglePlugin(req.Name, req.Enabled); err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			logs.LogOperation(username, "plugin_toggle", req.Name+" enabled="+boolToString(req.Enabled), clientIP(r))
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})

		router.mux.HandleFunc("/api/plugins/", func(w http.ResponseWriter, r *http.Request) {
			// 路径格式: /api/plugins/<plugin_name>/...
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) < 4 || parts[3] == "" {
				http.NotFound(w, r)
				return
			}
			pluginName := parts[3]

			// Auth check
			username, role, ok := requireAuth(w, r)
			if !ok {
				return
			}

			// Enforce admin-only plugins at the gateway (defense-in-depth)
			if p, ok := pluginManager.GetPlugin(pluginName); ok {
				if p.AdminOnly && role != "admin" {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}

			// Minimal audit hooks
			if pluginName == "webshell" {
				// Log websocket connection attempts (Upgrade happens here, before proxying)
				if strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/ws") {
					logs.LogOperation(username, "webshell_connect", "websocket connect", clientIP(r))
				}
			}

			pluginManager.ServeHTTP(w, r, pluginName)
		})
	}

	// Swagger API文档
	router.mux.Handle("/swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DomID("swagger-ui"),
	))

	// 静态文件服务
	fs := http.FileServer(http.Dir("./templates"))
	staticFs := http.StripPrefix("/static/", http.FileServer(http.Dir("./static")))
	router.mux.Handle("/assets/", fs)
	router.mux.Handle("/css/", fs)
	router.mux.Handle("/js/", fs)
	router.mux.Handle("/sw.js", fs)
	router.mux.Handle("/manifest.json", fs)
	router.mux.Handle("/static/", staticFs)

	// 强缓存静态文件服务 (Immutable)
	// 路径格式: /static-hashed/{hash}/{path...}
	// 例如: /static-hashed/a1b2c3d4/js/app.js -> ./static/js/app.js
	router.mux.HandleFunc("/static-hashed/", func(w http.ResponseWriter, r *http.Request) {
		// 提取真实路径
		// /static-hashed/a1b2c3d4/js/app.js -> /js/app.js
		path := r.URL.Path
		parts := strings.SplitN(strings.TrimPrefix(path, "/static-hashed/"), "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		realPath := parts[1]

		// 设置强缓存头
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		// 服务文件
		http.ServeFile(w, r, "./static/"+realPath)
	})

	// 主页
	router.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")

		// 使用模板渲染，注入 asset 函数
		tmpl, err := template.New("index.html").Funcs(template.FuncMap{
			"asset": assets.GetHashedPath,
		}).ParseFiles("./templates/index.html")

		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		tmpl.Execute(w, nil)
	})

	// 登录页面
	router.mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		// 使用模板渲染，注入 asset 函数
		tmpl, err := template.New("login.html").Funcs(template.FuncMap{
			"asset": assets.GetHashedPath,
		}).ParseFiles("./templates/login.html")

		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		tmpl.Execute(w, nil)
	})

	return router
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers (defense in depth)
		// CSP: Allow scripts from self + CDN (Chart.js), inline for legacy compat.
		// In production, consider removing 'unsafe-inline' and refactoring inline handlers.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src 'self' https://fonts.gstatic.com; "+
				"img-src 'self' data:; "+
				"connect-src 'self' wss: ws:; "+
				"frame-ancestors 'self'; "+
				"base-uri 'self'; "+
				"form-action 'self'")
		// Allow the app to embed its own content (plugins are shown in an iframe)
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		next.ServeHTTP(w, r)
	})
}

func wrapWithAPIAuthorization(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Basic request hardening for API endpoints.
		// Prevent large request bodies from exhausting memory/CPU.
		if strings.HasPrefix(path, "/api/") {
			const maxAPIRequestBodyBytes int64 = 2 << 20 // 2 MiB
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				if r.ContentLength > maxAPIRequestBodyBytes {
					writeJSONError(w, http.StatusRequestEntityTooLarge, "Request body too large")
					return
				}
				if r.Body != nil {
					r.Body = http.MaxBytesReader(w, r.Body, maxAPIRequestBodyBytes)
				}
			}
		}

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
			case "/api/users", "/api/logs", "/api/docker/action", "/api/docker/image/remove", "/api/docker/logs", "/api/systemd/action", "/api/power/action", "/api/cron/action", "/api/cron/logs", "/api/process/kill", "/api/gui/action", "/api/gui/status":
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
	server := &http.Server{
		Addr:              addr,
		Handler:           wrapWithAPIAuthorization(r.mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return server.ListenAndServe()
}

// Handler 返回包装了授权中间件的 HTTP Handler
func (r *Router) Handler() http.Handler {
	return securityHeaders(wrapWithAPIAuthorization(r.mux))
}
