// Package middleware 提供HTTP中间件
package middleware

import (
	"net/http"
	"strings"

	"github.com/AnalyseDeCircuit/opskernel/internal/auth"
)

// AuthMiddleware 认证中间件
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 从Authorization头获取token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// 尝试从Cookie获取
			cookie, err := r.Cookie("auth_token")
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			authHeader = "Bearer " + cookie.Value
		}

		// 检查Bearer token格式
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token := parts[1]
		claims, err := auth.ValidateJWT(token)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 将用户信息存储到请求上下文中（可选）
		// 这里我们只验证token的有效性
		_ = claims

		next(w, r)
	}
}

// AdminOnlyMiddleware 管理员专用中间件
func AdminOnlyMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 从Authorization头获取token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// 尝试从Cookie获取
			cookie, err := r.Cookie("auth_token")
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			authHeader = "Bearer " + cookie.Value
		}

		// 检查Bearer token格式
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token := parts[1]
		claims, err := auth.ValidateJWT(token)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 检查用户角色
		username := claims.Subject
		user := auth.GetUserByUsername(username)
		if user == nil || user.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}
