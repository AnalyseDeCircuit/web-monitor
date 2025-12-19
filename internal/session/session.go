// Package session 提供会话管理和登录历史追踪功能
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

var (
	sessionStore    = &SessionStore{}
	loginHistoryMu  sync.RWMutex
	loginHistoryMap = make(map[string][]types.LoginRecord) // username -> history
	preferencesMap  = make(map[string]*types.UserPreferences)
	preferencesMu   sync.RWMutex
	maxLoginHistory = 50 // 每用户最多保留的登录记录数
)

// SessionStore 会话存储
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*types.ActiveSession // sessionID -> session
}

func init() {
	sessionStore.sessions = make(map[string]*types.ActiveSession)
}

// GenerateSessionID 生成唯一会话 ID
func GenerateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// HashToken 对 token 做哈希（用于安全存储）
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession 创建新会话
func CreateSession(username, token, ip, userAgent string, expiresAt time.Time) *types.ActiveSession {
	sessionStore.mu.Lock()
	defer sessionStore.mu.Unlock()

	sessionID := GenerateSessionID()
	deviceType, browser, osName := parseUserAgent(userAgent)

	session := &types.ActiveSession{
		SessionID:  sessionID,
		TokenHash:  HashToken(token),
		Username:   username,
		CreatedAt:  time.Now(),
		ExpiresAt:  expiresAt,
		LastActive: time.Now(),
		IP:         ip,
		UserAgent:  userAgent,
		DeviceType: deviceType,
		Browser:    browser,
		OS:         osName,
	}

	sessionStore.sessions[sessionID] = session
	return session
}

// GetSessionByToken 根据 token 获取会话
func GetSessionByToken(token string) *types.ActiveSession {
	sessionStore.mu.RLock()
	defer sessionStore.mu.RUnlock()

	tokenHash := HashToken(token)
	for _, s := range sessionStore.sessions {
		if s.TokenHash == tokenHash {
			return s
		}
	}
	return nil
}

// UpdateSessionActivity 更新会话活跃时间
func UpdateSessionActivity(token string) {
	sessionStore.mu.Lock()
	defer sessionStore.mu.Unlock()

	tokenHash := HashToken(token)
	for _, s := range sessionStore.sessions {
		if s.TokenHash == tokenHash {
			s.LastActive = time.Now()
			return
		}
	}
}

// GetUserSessions 获取用户的所有活跃会话
func GetUserSessions(username string, currentToken string) []types.ActiveSession {
	sessionStore.mu.RLock()
	defer sessionStore.mu.RUnlock()

	currentTokenHash := HashToken(currentToken)
	var sessions []types.ActiveSession

	now := time.Now()
	for _, s := range sessionStore.sessions {
		if s.Username == username && s.ExpiresAt.After(now) {
			sessionCopy := *s
			sessionCopy.IsCurrent = (s.TokenHash == currentTokenHash)
			sessions = append(sessions, sessionCopy)
		}
	}

	return sessions
}

// RevokeSession 撤销指定会话
func RevokeSession(sessionID, username string) bool {
	sessionStore.mu.Lock()
	defer sessionStore.mu.Unlock()

	if s, ok := sessionStore.sessions[sessionID]; ok {
		if s.Username == username {
			delete(sessionStore.sessions, sessionID)
			return true
		}
	}
	return false
}

// RevokeOtherSessions 撤销除当前会话外的所有其它会话
func RevokeOtherSessions(username, currentToken string) int {
	sessionStore.mu.Lock()
	defer sessionStore.mu.Unlock()

	currentTokenHash := HashToken(currentToken)
	count := 0

	for id, s := range sessionStore.sessions {
		if s.Username == username && s.TokenHash != currentTokenHash {
			delete(sessionStore.sessions, id)
			count++
		}
	}

	return count
}

// RevokeSessionByToken 根据 token 撤销会话
func RevokeSessionByToken(token string) {
	sessionStore.mu.Lock()
	defer sessionStore.mu.Unlock()

	tokenHash := HashToken(token)
	for id, s := range sessionStore.sessions {
		if s.TokenHash == tokenHash {
			delete(sessionStore.sessions, id)
			return
		}
	}
}

// CleanExpiredSessions 清理过期会话（定期调用）
func CleanExpiredSessions() {
	sessionStore.mu.Lock()
	defer sessionStore.mu.Unlock()

	now := time.Now()
	for id, s := range sessionStore.sessions {
		if s.ExpiresAt.Before(now) {
			delete(sessionStore.sessions, id)
		}
	}
}

// --- 登录历史 ---

// RecordLogin 记录登录（成功或失败）
func RecordLogin(username, ip, userAgent string, success bool, sessionID string) {
	loginHistoryMu.Lock()
	defer loginHistoryMu.Unlock()

	_, browser, osName := parseUserAgent(userAgent)

	record := types.LoginRecord{
		Time:      time.Now(),
		IP:        ip,
		UserAgent: userAgent,
		Browser:   browser,
		OS:        osName,
		Location:  guessLocationFromIP(ip),
		Success:   success,
		SessionID: sessionID,
	}

	history := loginHistoryMap[username]
	history = append([]types.LoginRecord{record}, history...) // 最新在前

	// 限制历史记录数量
	if len(history) > maxLoginHistory {
		history = history[:maxLoginHistory]
	}

	loginHistoryMap[username] = history
}

// GetLoginHistory 获取用户登录历史
func GetLoginHistory(username string, limit int) []types.LoginRecord {
	loginHistoryMu.RLock()
	defer loginHistoryMu.RUnlock()

	history := loginHistoryMap[username]
	if limit > 0 && len(history) > limit {
		return history[:limit]
	}
	return history
}

// --- 用户偏好 ---

// GetUserPreferences 获取用户偏好
func GetUserPreferences(username string) *types.UserPreferences {
	preferencesMu.RLock()
	defer preferencesMu.RUnlock()

	if prefs, ok := preferencesMap[username]; ok {
		return prefs
	}

	// 返回默认偏好
	return &types.UserPreferences{
		Username: username,
		AlertPreferences: types.AlertPreferences{
			InAppOnly:        true,
			SubscribedAlerts: []string{"cpu", "memory", "disk"},
		},
		UIPreferences: types.UIPreferences{
			DefaultPage:  "general",
			TimeFormat:   "24h",
			Timezone:     "local",
			ByteFormat:   "iec",
			TableDensity: "normal",
		},
	}
}

// SaveUserPreferences 保存用户偏好
func SaveUserPreferences(prefs *types.UserPreferences) error {
	preferencesMu.Lock()
	defer preferencesMu.Unlock()

	preferencesMap[prefs.Username] = prefs
	return savePreferencesToDisk()
}

// --- 角色权限描述 ---

// GetRolePermissions 获取角色权限描述
func GetRolePermissions(role string) types.RolePermissions {
	switch role {
	case "admin":
		return types.RolePermissions{
			Role:        "admin",
			Description: "Full system administrator with complete access",
			CanDo: []string{
				"View all system metrics and processes",
				"Start/Stop/Restart Docker containers",
				"Manage system services (systemd)",
				"Create, edit, and delete cron jobs",
				"Kill processes",
				"Manage users (create, delete, reset passwords)",
				"View and export operation logs",
				"Configure system settings",
			},
			CannotDo: []string{
				"Delete the admin account",
			},
		}
	case "user":
		return types.RolePermissions{
			Role:        "user",
			Description: "Read-only access to system monitoring",
			CanDo: []string{
				"View all system metrics",
				"View process list",
				"View Docker containers and images",
				"View system services status",
				"View cron jobs",
				"Change own password",
			},
			CannotDo: []string{
				"Modify Docker containers",
				"Manage system services",
				"Edit cron jobs",
				"Kill processes",
				"Manage users",
				"View operation logs",
				"Configure system settings",
			},
		}
	default:
		return types.RolePermissions{
			Role:        role,
			Description: "Unknown role",
			CanDo:       []string{},
			CannotDo:    []string{"All administrative actions"},
		}
	}
}

// --- 辅助函数 ---

// parseUserAgent 解析 User-Agent 字符串
func parseUserAgent(ua string) (deviceType, browser, osName string) {
	ua = strings.ToLower(ua)

	// 设备类型
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "android") && !strings.Contains(ua, "tablet") {
		deviceType = "mobile"
	} else if strings.Contains(ua, "tablet") || strings.Contains(ua, "ipad") {
		deviceType = "tablet"
	} else {
		deviceType = "desktop"
	}

	// 浏览器
	switch {
	case strings.Contains(ua, "edg/"):
		browser = "Edge"
	case strings.Contains(ua, "chrome/") && !strings.Contains(ua, "edg/"):
		browser = "Chrome"
	case strings.Contains(ua, "firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "safari/") && !strings.Contains(ua, "chrome/"):
		browser = "Safari"
	case strings.Contains(ua, "opera") || strings.Contains(ua, "opr/"):
		browser = "Opera"
	default:
		browser = "Unknown"
	}

	// 操作系统 (注意: iOS 检测必须在 macOS 之前，因为 iPhone/iPad UA 也包含 "Mac OS")
	switch {
	case strings.Contains(ua, "iphone"):
		osName = "iOS"
	case strings.Contains(ua, "ipad"):
		osName = "iPadOS"
	case strings.Contains(ua, "android"):
		osName = "Android"
	case strings.Contains(ua, "windows"):
		osName = "Windows"
	case strings.Contains(ua, "mac os") || strings.Contains(ua, "macos"):
		osName = "macOS"
	case strings.Contains(ua, "linux"):
		osName = "Linux"
	case strings.Contains(ua, "cros"):
		osName = "ChromeOS"
	default:
		osName = "Unknown"
	}

	return
}

// guessLocationFromIP 根据 IP 猜测大致地点（简单实现）
func guessLocationFromIP(ip string) string {
	// 内网 IP
	if isPrivateIP(ip) {
		return "Local Network"
	}
	// TODO: 可以集成 IP 地理位置数据库 (MaxMind GeoIP 等)
	return ""
}

// isPrivateIP 判断是否为内网 IP
func isPrivateIP(ip string) bool {
	privateIPPatterns := []string{
		`^10\.`,
		`^172\.(1[6-9]|2[0-9]|3[0-1])\.`,
		`^192\.168\.`,
		`^127\.`,
		`^::1$`,
		`^localhost$`,
	}

	for _, pattern := range privateIPPatterns {
		if matched, _ := regexp.MatchString(pattern, ip); matched {
			return true
		}
	}
	return false
}

// --- 持久化 ---

func getDataDir() string {
	if v := os.Getenv("DATA_DIR"); v != "" {
		return v
	}
	if _, err := os.Stat("/data"); err == nil {
		return "/data"
	}
	return "./data"
}

func savePreferencesToDisk() error {
	dataDir := getDataDir()
	filePath := filepath.Join(dataDir, "user_preferences.json")

	data, err := json.MarshalIndent(preferencesMap, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0600)
}

// LoadPreferencesFromDisk 从磁盘加载用户偏好
func LoadPreferencesFromDisk() error {
	preferencesMu.Lock()
	defer preferencesMu.Unlock()

	dataDir := getDataDir()
	filePath := filepath.Join(dataDir, "user_preferences.json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在是正常的
		}
		return err
	}

	return json.Unmarshal(data, &preferencesMap)
}

// SaveLoginHistoryToDisk 保存登录历史到磁盘
func SaveLoginHistoryToDisk() error {
	loginHistoryMu.RLock()
	defer loginHistoryMu.RUnlock()

	dataDir := getDataDir()
	filePath := filepath.Join(dataDir, "login_history.json")

	data, err := json.MarshalIndent(loginHistoryMap, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0600)
}

// LoadLoginHistoryFromDisk 从磁盘加载登录历史
func LoadLoginHistoryFromDisk() error {
	loginHistoryMu.Lock()
	defer loginHistoryMu.Unlock()

	dataDir := getDataDir()
	filePath := filepath.Join(dataDir, "login_history.json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &loginHistoryMap)
}

// StartCleanupRoutine 启动定期清理例程
func StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			CleanExpiredSessions()
		}
	}()
}
