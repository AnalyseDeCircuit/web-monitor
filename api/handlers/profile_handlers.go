package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/AnalyseDeCircuit/opskernel/internal/auth"
	"github.com/AnalyseDeCircuit/opskernel/internal/session"
	"github.com/AnalyseDeCircuit/opskernel/pkg/types"
)

// ProfileHandler 获取完整的用户 Profile 信息
// GET /api/profile
func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	username, role, err := getUserAndRoleFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 获取当前 token
	currentToken := extractToken(r)

	// 获取用户信息
	user := auth.GetUserByUsername(username)
	if user == nil {
		writeJSONError(w, http.StatusNotFound, "User not found")
		return
	}

	// 构建响应
	response := types.UserProfileResponse{
		Username:           user.Username,
		Role:               user.Role,
		CreatedAt:          user.CreatedAt,
		LastLogin:          user.LastLogin,
		LastPasswordChange: user.LastPasswordChange,
		LastFailedLogin:    user.LastFailedLogin,
		LastFailedLoginIP:  user.LastFailedLoginIP,
		LoginHistory:       session.GetLoginHistory(username, 10),
		ActiveSessions:     session.GetUserSessions(username, currentToken),
		Permissions:        session.GetRolePermissions(role),
		Preferences:        *session.GetUserPreferences(username),
	}

	writeJSON(w, http.StatusOK, response)
}

// ProfileSessionsHandler 管理用户会话
// GET /api/profile/sessions - 获取所有活跃会话
// DELETE /api/profile/sessions - 撤销其它会话
// DELETE /api/profile/sessions/{id} - 撤销指定会话
func ProfileSessionsHandler(w http.ResponseWriter, r *http.Request) {
	username, _, err := getUserAndRoleFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	currentToken := extractToken(r)

	switch r.Method {
	case http.MethodGet:
		sessions := session.GetUserSessions(username, currentToken)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"sessions": sessions,
		})

	case http.MethodDelete:
		// 检查是否指定了 session ID
		sessionID := r.URL.Query().Get("id")

		if sessionID == "" {
			// 撤销所有其它会话
			count := session.RevokeOtherSessions(username, currentToken)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"message": "Other sessions revoked",
				"count":   count,
			})
		} else {
			// 撤销指定会话
			if session.RevokeSession(sessionID, username) {
				writeJSON(w, http.StatusOK, map[string]string{
					"message": "Session revoked",
				})
			} else {
				writeJSONError(w, http.StatusNotFound, "Session not found")
			}
		}

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// ProfilePreferencesHandler 管理用户偏好
// GET /api/profile/preferences - 获取偏好
// PUT /api/profile/preferences - 更新偏好
func ProfilePreferencesHandler(w http.ResponseWriter, r *http.Request) {
	username, _, err := getUserAndRoleFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	switch r.Method {
	case http.MethodGet:
		prefs := session.GetUserPreferences(username)
		writeJSON(w, http.StatusOK, prefs)

	case http.MethodPut:
		var prefs types.UserPreferences
		if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		prefs.Username = username // 确保用户名正确

		if err := session.SaveUserPreferences(&prefs); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to save preferences")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"message": "Preferences saved",
		})

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// ProfileLoginHistoryHandler 获取登录历史
// GET /api/profile/login-history
func ProfileLoginHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	username, _, err := getUserAndRoleFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		var n int
		if err := json.Unmarshal([]byte(l), &n); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	history := session.GetLoginHistory(username, limit)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"records": history,
	})
}

// extractToken 从请求中提取 token
func extractToken(r *http.Request) string {
	// 先从 Authorization header 提取
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1]
		}
	}

	// 再从 cookie 提取
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}

	return ""
}
