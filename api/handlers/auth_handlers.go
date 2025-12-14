// Package handlers 提供HTTP路由处理器
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/AnalyseDeCircuit/web-monitor/internal/auth"
	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

// LoginHandler 处理登录请求
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 速率限制检查
	if !auth.GetLoginLimiter().Allow() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "Too many login attempts. Please try again later."})
		return
	}

	var req types.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// 验证用户
	user := auth.ValidateUser(req.Username, req.Password)
	if user == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid credentials"})
		return
	}

	// 生成JWT令牌
	jwtToken, err := auth.GenerateJWT(user.Username, user.Role)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// 设置安全的HTTP Cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    jwtToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400, // 24小时
	})

	// 记录操作日志
	logs.LogOperation(user.Username, "login", "User logged in", r.RemoteAddr)

	// 返回响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.LoginResponse{
		Token:    jwtToken,
		Message:  "Login successful",
		Username: user.Username,
		Role:     user.Role,
	})
}

// LogoutHandler 处理登出请求
func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 由于我们使用JWT，登出由客户端删除token即可
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}

// ChangePasswordHandler 处理修改密码请求
func ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username    string `json:"username"` // Optional, if admin changing other's password
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// 这里需要实现JWT验证和用户角色检查
	// 暂时返回一个简单的响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Password change endpoint (to be implemented)"})
}

// ValidatePasswordHandler 验证密码策略
func ValidatePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	isValid := utils.ValidatePasswordPolicy(req.Password)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid": isValid,
	})
}
