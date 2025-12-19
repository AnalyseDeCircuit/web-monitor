// Package handlers 提供HTTP路由处理器
package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/auth"
	"github.com/AnalyseDeCircuit/web-monitor/internal/logs"
	"github.com/AnalyseDeCircuit/web-monitor/internal/session"
	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

func clientIP(r *http.Request) string {
	// Prefer proxy headers when present.
	// SECURITY WARNING: Trusting these headers blindly is dangerous if the server is directly accessible.
	// Ensure your firewall (e.g., UFW, iptables) only allows traffic from trusted proxies (e.g., Cloudflare IPs).
	if v := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); v != "" {
		parts := strings.Split(v, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

// LoginHandler 处理登录请求
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req types.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// 速率限制检查（按 IP 与 username 分开限流，避免全局 limiter 被 DoS）
	ip := clientIP(r)
	if !auth.GetLoginLimiterForKey("ip:" + ip).Allow() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "Too many login attempts. Please try again later."})
		return
	}
	if req.Username != "" {
		uname := strings.ToLower(strings.TrimSpace(req.Username))
		if uname != "" && !auth.GetLoginLimiterForKey("user:"+uname).Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{"error": "Too many login attempts. Please try again later."})
			return
		}
	}

	// 验证用户
	user := auth.ValidateUser(req.Username, req.Password)
	if user == nil {
		// 记录失败登录
		session.RecordLogin(req.Username, ip, r.UserAgent(), false, "")
		auth.RecordFailedLogin(req.Username, ip)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid credentials"})
		return
	}

	// 生成JWT令牌
	jwtToken, err := auth.GenerateJWT(user.Username, user.Role)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// 创建会话
	expiresAt := time.Now().Add(24 * time.Hour)
	sess := session.CreateSession(user.Username, jwtToken, ip, r.UserAgent(), expiresAt)

	// 记录成功登录
	session.RecordLogin(user.Username, ip, r.UserAgent(), true, sess.SessionID)

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
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Server-side revoke: mark this JWT as invalid for future requests.
	// We accept either Authorization Bearer or cookie-based auth.
	var token string
	if authHeader := strings.TrimSpace(r.Header.Get("Authorization")); authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			token = strings.TrimSpace(parts[1])
			auth.RevokeJWT(token)
		}
	} else if cookie, err := r.Cookie("auth_token"); err == nil {
		token = cookie.Value
		auth.RevokeJWT(token)
	}

	// 撤销会话
	if token != "" {
		session.RevokeSessionByToken(token)
	}

	// 清理 Cookie（客户端也会清理 localStorage）
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}

// ChangePasswordHandler 处理修改密码请求
func ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	requesterUsername, requesterRole, err := getUserAndRoleFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Username    string `json:"username"` // Optional, if admin changing other's password
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if req.NewPassword == "" {
		writeJSONError(w, http.StatusBadRequest, "new_password is required")
		return
	}
	if !utils.ValidatePasswordPolicy(req.NewPassword) {
		writeJSONError(w, http.StatusBadRequest, "Password does not meet complexity requirements")
		return
	}

	target := req.Username
	if target == "" {
		target = requesterUsername
	}

	if target != requesterUsername && requesterRole != "admin" {
		writeJSONError(w, http.StatusForbidden, "Forbidden: Admin access required")
		return
	}

	if err := auth.ChangePassword(requesterUsername, requesterRole, target, req.OldPassword, req.NewPassword); err != nil {
		switch err.Error() {
		case "old_password is required", "new_password is required", "missing username":
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		case "invalid old password":
			writeJSONError(w, http.StatusUnauthorized, "Invalid old password")
			return
		case "forbidden":
			writeJSONError(w, http.StatusForbidden, "Forbidden: Admin access required")
			return
		case "user not found":
			writeJSONError(w, http.StatusNotFound, "User not found")
			return
		default:
			writeJSONError(w, http.StatusInternalServerError, "Failed to change password")
			return
		}
	}

	logs.LogOperation(requesterUsername, "change_password", "Changed password for: "+target, r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// ValidatePasswordHandler 验证密码策略
func ValidatePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	isValid := utils.ValidatePasswordPolicy(req.Password)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid": isValid,
	})
}

// UsersHandler 处理用户管理请求（仅管理员）
// GET    /api/users                 -> 列出所有用户
// POST   /api/users                 -> 创建新用户 {username,password,role}
// DELETE /api/users?username=alice  -> 删除指定用户

func UsersHandler(w http.ResponseWriter, r *http.Request) {
	username, role, err := getUserAndRoleFromRequest(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Unauthorized",
		})
		return
	}
	if role != "admin" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Forbidden: Admin access required",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		users := auth.GetAllUsers()
		respUsers := make([]map[string]interface{}, len(users))
		for i, u := range users {
			respUsers[i] = map[string]interface{}{
				"id":         u.ID,
				"username":   u.Username,
				"role":       u.Role,
				"created_at": u.CreatedAt,
				"last_login": u.LastLogin,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": respUsers,
		})

	case http.MethodPost:
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request"})
			return
		}

		if req.Username == "" || req.Password == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Username and password are required"})
			return
		}

		if req.Role != "admin" && req.Role != "user" {
			req.Role = "user"
		}

		if !utils.ValidatePasswordPolicy(req.Password) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Password does not meet complexity requirements. Must be at least 8 characters long and contain at least three of: uppercase letters, lowercase letters, digits, and special characters."})
			return
		}

		if err := auth.CreateUser(req.Username, req.Password, req.Role); err != nil {
			w.Header().Set("Content-Type", "application/json")
			if err.Error() == "user already exists" {
				w.WriteHeader(http.StatusConflict)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// 记录操作日志（此时 username 为当前登录用户，通常是 admin）
		logs.LogOperation(username, "create_user", "Created user: "+req.Username+" ("+req.Role+")", r.RemoteAddr)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"message":  "User created successfully",
			"username": req.Username,
		})

	case http.MethodDelete:
		target := r.URL.Query().Get("username")
		if target == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Username required"})
			return
		}

		if target == "admin" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Cannot delete admin user"})
			return
		}

		if err := auth.DeleteUser(target); err != nil {
			w.Header().Set("Content-Type", "application/json")
			if err.Error() == "user not found" {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		logs.LogOperation(username, "delete_user", "Deleted user: "+target, r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"})

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// LogsHandler 处理操作日志请求
func LogsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	// 返回操作日志
	opLogs := logs.GetLogs()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs": opLogs,
	})
}
