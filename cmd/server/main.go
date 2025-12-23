// Package main 提供Web监控服务器的主入口点
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/api/handlers"
	"github.com/AnalyseDeCircuit/opskernel/internal/alerts"
	"github.com/AnalyseDeCircuit/opskernel/internal/assets"
	"github.com/AnalyseDeCircuit/opskernel/internal/auth"
	"github.com/AnalyseDeCircuit/opskernel/internal/config"
	"github.com/AnalyseDeCircuit/opskernel/internal/logs"
	"github.com/AnalyseDeCircuit/opskernel/internal/monitoring"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin"
	"github.com/AnalyseDeCircuit/opskernel/internal/session"
	"github.com/AnalyseDeCircuit/opskernel/internal/settings"
	"github.com/AnalyseDeCircuit/opskernel/internal/websocket"

	_ "github.com/AnalyseDeCircuit/opskernel/docs" // swagger docs
)

// @title OpsKernel API
// @version 2.0
// @description 轻量级系统监控API服务，提供实时CPU、内存、磁盘、网络、GPU监控，以及Docker和Systemd管理功能
// @description
// @description 特性:
// @description - 实时系统监控 (CPU/内存/磁盘/网络/GPU/传感器)
// @description - WebSocket推送 (动态订阅主题)
// @description - Docker容器管理
// @description - Systemd服务管理
// @description - SSH会话监控
// @description - Cron任务管理
// @description - 用户权限管理 (admin/user)
// @description - JWT认证 (HttpOnly Cookie)
// @description - Prometheus指标导出

// @contact.name API Support
// @contact.url https://github.com/AnalyseDeCircuit/opskernel
// @contact.email support@example.com

// @license.name CC BY-NC 4.0
// @license.url https://creativecommons.org/licenses/by-nc/4.0/

// @host localhost:8000
// @BasePath /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT令牌 (格式: "Bearer {token}")

// @securityDefinitions.apikey CookieAuth
// @in cookie
// @name auth_token
// @description JWT令牌 (HttpOnly Cookie, 优先使用)

// @tag.name Authentication
// @tag.description 用户认证相关接口

// @tag.name Monitoring
// @tag.description 系统监控数据接口

// @tag.name Docker
// @tag.description Docker容器管理接口 (需要admin权限)

// @tag.name Systemd
// @tag.description Systemd服务管理接口 (需要admin权限)

// @tag.name Users
// @tag.description 用户管理接口 (需要admin权限)

// @tag.name Cron
// @tag.description Cron任务管理接口 (需要admin权限)

// @tag.name WebSocket
// @tag.description WebSocket实时推送接口

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting opskernel server...")

	// 加载全局配置
	cfg := config.Load()
	if cfg.HostFS != "" {
		log.Printf("Configured to use HostFS at: %s", cfg.HostFS)
		// 配置 gopsutil 使用宿主机环境
		os.Setenv("HOST_PROC", cfg.HostProc)
		os.Setenv("HOST_SYS", cfg.HostSys)
		os.Setenv("HOST_ETC", cfg.HostEtc)
		os.Setenv("HOST_VAR", cfg.HostVar)
		os.Setenv("HOST_RUN", cfg.HostRun)
		// os.Setenv("HOST_DEV", cfg.HostDev) // Config struct doesn't have HostDev yet, maybe add it or ignore if not used
	} else {
		log.Println("Running in Bare Metal mode (no HostFS)")
	}

	// 初始化用户数据库
	log.Println("Initializing user database...")
	if err := auth.InitUserDatabase(); err != nil {
		log.Printf("Warning: Failed to initialize user database: %v\n", err)
	}
	log.Println("User database initialized")

	// 初始化JWT密钥
	auth.InitJWTKey()

	// 初始化会话管理器
	log.Println("Initializing session manager...")
	session.LoadPreferencesFromDisk()
	session.LoadLoginHistoryFromDisk()
	session.StartCleanupRoutine()
	log.Println("Session manager initialized")

	// 加载操作日志
	logs.LoadOpLogs()

	// 加载告警配置 (旧版兼容)
	monitoring.LoadAlerts()

	// 初始化告警管理器 (新版)
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	alertManager := alerts.NewManager(dataDir)
	if err := alertManager.Initialize(); err != nil {
		log.Printf("Warning: Failed to initialize alert manager: %v\n", err)
	}
	handlers.SetAlertManager(alertManager)
	monitoring.SetAlertManager(alertManager)

	// 加载系统设置
	settings.Load()

	// 初始化静态资源哈希
	if err := assets.Init(); err != nil {
		log.Printf("Warning: Failed to initialize assets manager: %v\n", err)
	}

	// 初始化插件管理器 (V2 - 模块化架构)
	pluginConfig := &plugin.Config{
		PluginsDir: "/app/plugins", // 容器内插件清单目录
		DataDir:    dataDir,        // 状态持久化目录 (/data)
		// 默认安全策略：需要确认、允许高风险插件（但需 admin 显式批准）
		RequireConfirmation: true,
		AllowCriticalRisk:   true,
		AllowPrivileged:     true,
	}
	pluginManagerV2 := plugin.NewManagerV2(pluginConfig)

	// 加载插件清单并启动已启用的插件
	if err := pluginManagerV2.LoadPlugins(""); err != nil {
		log.Printf("Warning: Failed to load plugins: %v\n", err)
	}

	// 设置HTTP路由 (V2 - 支持 /plugins/{name}/ 统一路径)
	router := handlers.SetupRouterV2(pluginManagerV2)

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           router.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// 启动服务器（非阻塞）
	go func() {
		log.Printf("Server starting on port %s...\n", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start: ", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// 优雅关闭 WebSocket hub
	websocket.Shutdown()

	// 清理插件（V2：停止所有运行中的插件容器）
	pluginManagerV2.Cleanup()

	// 优雅关闭 HTTP 服务器（等待最多 10 秒）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
}
