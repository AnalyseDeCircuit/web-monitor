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

	"github.com/AnalyseDeCircuit/web-monitor/api/handlers"
	"github.com/AnalyseDeCircuit/web-monitor/internal/assets"
	"github.com/AnalyseDeCircuit/web-monitor/internal/auth"
	"github.com/AnalyseDeCircuit/web-monitor/internal/config"
	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
	"github.com/AnalyseDeCircuit/web-monitor/internal/websocket"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting web-monitor server...")

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

	// 加载操作日志
	logs.LoadOpLogs()

	// 加载告警配置
	monitoring.LoadAlerts()

	// 初始化静态资源哈希
	if err := assets.Init(); err != nil {
		log.Printf("Warning: Failed to initialize assets manager: %v\n", err)
	}

	// 设置HTTP路由
	router := handlers.SetupRouter()

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

	// 优雅关闭 HTTP 服务器（等待最多 10 秒）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
}
