// Package handlers provides HTTP route setup for plugin system v2.
package handlers

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/internal/assets"
	"github.com/AnalyseDeCircuit/opskernel/internal/config"
	"github.com/AnalyseDeCircuit/opskernel/internal/logs"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin"
	"github.com/AnalyseDeCircuit/opskernel/internal/websocket"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// SetupRouterV2 sets up all routes using the new modular plugin system.
// It keeps backward compatibility with /api/plugins/* while adding /plugins/*
func SetupRouterV2(pluginManager *plugin.ManagerV2) *Router {
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
	router.mux.HandleFunc("/api/info", StaticInfoHandler)
	router.mux.HandleFunc("/api/system/info", SystemInfoHandler)
	router.mux.HandleFunc("/api/alerts", AlertsHandler)
	router.mux.HandleFunc("/api/network/info", NetworkInfoHandler)
	router.mux.HandleFunc("/api/cache/info", CacheInfoHandler)
	router.mux.HandleFunc("/api/health", HealthCheckHandler)
	router.mux.HandleFunc("/api/metrics", PrometheusMetricsHandler)

	// 告警系统路由
	router.mux.HandleFunc("/api/alerts/config", AlertsConfigHandler)
	router.mux.HandleFunc("/api/alerts/rules", AlertsRulesHandler)
	router.mux.HandleFunc("/api/alerts/rules/", AlertsRuleHandler)
	router.mux.HandleFunc("/api/alerts/presets", AlertsPresetsHandler)
	router.mux.HandleFunc("/api/alerts/presets/", AlertsPresetEnableHandler)
	router.mux.HandleFunc("/api/alerts/disable-all", AlertsDisableAllHandler)
	router.mux.HandleFunc("/api/alerts/history", AlertsHistoryHandler)
	router.mux.HandleFunc("/api/alerts/active", AlertsActiveHandler)
	router.mux.HandleFunc("/api/alerts/summary", AlertsSummaryHandler)
	router.mux.HandleFunc("/api/alerts/test", AlertsTestHandler)
	router.mux.HandleFunc("/api/alerts/metrics", AlertsMetricsHandler)

	if cfg.EnablePower {
		router.mux.HandleFunc("/api/power/profile", PowerProfileHandler)
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

	if cfg.EnableCron {
		router.mux.HandleFunc("/api/cron", CronLegacyHandler)
		router.mux.HandleFunc("/api/cron/jobs", CronJobsHandler)
		router.mux.HandleFunc("/api/cron/action", CronActionHandler)
		router.mux.HandleFunc("/api/cron/logs", CronLogsHandler)
	}

	router.mux.HandleFunc("/api/process/io", ProcessIOHandler)
	router.mux.HandleFunc("/api/process/kill", ProcessKillHandler)

	if cfg.EnableSSH {
		router.mux.HandleFunc("/api/ssh/stats", SSHStatsHandler)
	}

	router.mux.HandleFunc("/api/settings", SystemSettingsHandler)
	router.mux.HandleFunc("/ws/stats", websocket.HandleWebSocket)

	// =========================================================================
	// Plugin Routes V2
	// =========================================================================
	if pluginManager != nil {
		ph := NewPluginHandlers(pluginManager)

		// API endpoints
		router.mux.HandleFunc("/api/plugins/list", ph.HandleList)
		router.mux.HandleFunc("/api/plugins/action", ph.HandleAction)
		router.mux.HandleFunc("/api/plugins/enable", ph.HandleEnable)
		router.mux.HandleFunc("/api/plugins/disable", ph.HandleDisable)
		router.mux.HandleFunc("/api/plugins/install", ph.HandleInstall)
		router.mux.HandleFunc("/api/plugins/uninstall", ph.HandleUninstall)
		router.mux.HandleFunc("/api/plugins/manifest", ph.HandleManifest)
		router.mux.HandleFunc("/api/plugins/security", ph.HandleSecuritySummary)

		// New unified proxy path: /plugins/{name}/...
		router.mux.HandleFunc("/plugins/", ph.HandleProxy)

		// Legacy proxy path: /api/plugins/{name}/... (backward compatibility)
		router.mux.HandleFunc("/api/plugins/", func(w http.ResponseWriter, r *http.Request) {
			// Skip if it's an API endpoint (already handled above)
			path := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
			parts := strings.SplitN(path, "/", 2)
			if len(parts) == 0 || parts[0] == "" {
				http.NotFound(w, r)
				return
			}
			pluginName := parts[0]

			// Check if it's an API endpoint
			apiEndpoints := map[string]bool{
				"list": true, "action": true, "enable": true, "disable": true,
				"install": true, "uninstall": true, "manifest": true, "security": true,
			}
			if apiEndpoints[pluginName] {
				// Already handled by explicit routes
				http.NotFound(w, r)
				return
			}

			ph.HandleProxy(w, r)
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

	// 强缓存静态文件服务
	router.mux.HandleFunc("/static-hashed/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		parts := strings.SplitN(strings.TrimPrefix(path, "/static-hashed/"), "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		realPath := parts[1]
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, "./static/"+realPath)
	})

	// 主页
	router.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")

		tmpl, err := template.New("index.html").Funcs(template.FuncMap{
			"asset": assets.GetHashedPath,
		}).ParseFiles("./templates/index.html")

		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}

		data := struct {
			AppVersion   string
			StaticPrefix string
		}{
			AppVersion:   time.Now().Format("20060102150405"),
			StaticPrefix: "",
		}

		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
		}
	})

	// Login page
	router.mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")

		tmpl, err := template.New("login.html").Funcs(template.FuncMap{
			"asset": assets.GetHashedPath,
		}).ParseFiles("./templates/login.html")

		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}

		data := struct {
			AppVersion   string
			StaticPrefix string
		}{
			AppVersion:   time.Now().Format("20060102150405"),
			StaticPrefix: "",
		}

		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
		}
	})

	return router
}

// Suppress unused import warnings
var (
	_ = logs.LogOperation
)
