// Package main 提供Web监控服务器的主入口点
package main

import (
	"log"
	"os"

	"github.com/AnalyseDeCircuit/web-monitor/api/handlers"
	"github.com/AnalyseDeCircuit/web-monitor/internal/auth"
	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting web-monitor server...")

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

	// 设置HTTP路由
	router := handlers.SetupRouter()

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	log.Printf("Server starting on port %s...\n", port)
	if err := router.Start(":" + port); err != nil {
		log.Fatal("Server failed to start: ", err)
	}
}
