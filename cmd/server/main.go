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
	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
	"github.com/AnalyseDeCircuit/web-monitor/internal/websocket"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting web-monitor server...")

	// 配置 gopsutil 使用宿主机环境（如果挂载了 /hostfs）
	if _, err := os.Stat("/hostfs"); err == nil {
		log.Println("Detected /hostfs, configuring gopsutil to use host resources...")
		os.Setenv("HOST_PROC", "/hostfs/proc")
		os.Setenv("HOST_SYS", "/hostfs/sys")
		os.Setenv("HOST_ETC", "/hostfs/etc")
		os.Setenv("HOST_VAR", "/hostfs/var")
		os.Setenv("HOST_RUN", "/hostfs/run")
		os.Setenv("HOST_DEV", "/hostfs/dev")

		// 同时也设置 gopsutil 特定的环境变量
		// 注意：gopsutil v3 使用 common.EnvMap 来查找环境变量，但通常也会读取系统环境变量
		// 为了保险起见，我们也可以尝试直接设置 common 包的变量（如果它是导出的）
		// 但 gopsutil 通常通过环境变量工作。
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
