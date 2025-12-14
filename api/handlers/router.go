// Package handlers 提供HTTP路由处理器
package handlers

import (
	"net/http"

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

	// WebSocket路由
	router.mux.HandleFunc("/ws/stats", websocket.HandleWebSocket)

	// 静态文件服务
	fs := http.FileServer(http.Dir("./templates"))
	router.mux.Handle("/assets/", fs)
	router.mux.Handle("/css/", fs)
	router.mux.Handle("/js/", fs)

	// 主页
	router.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "./templates/index.html")
	})

	// 登录页面
	router.mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./templates/login.html")
	})

	return router
}

// Start 启动HTTP服务器
func (r *Router) Start(addr string) error {
	return http.ListenAndServe(addr, r.mux)
}
