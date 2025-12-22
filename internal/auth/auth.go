// Package auth 提供用户认证和授权功能
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/pkg/types"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"
)

var (
	userDB        *types.UserDatabase
	userDB_mu     sync.RWMutex
	loginLimiters = newLoginLimiterStore(1, 5, 10*time.Minute)
	revokedJWTs   = newRevokedJWTStore(30 * time.Minute)
	jwtKey        []byte
)

type revokedJWTStore struct {
	mu         sync.Mutex
	items      map[string]time.Time // tokenHash -> expiresAt
	lastGC     time.Time
	gcInterval time.Duration
}

func newRevokedJWTStore(gcInterval time.Duration) *revokedJWTStore {
	if gcInterval <= 0 {
		gcInterval = 30 * time.Minute
	}
	return &revokedJWTStore{
		items:      make(map[string]time.Time),
		lastGC:     time.Now(),
		gcInterval: gcInterval,
	}
}

func (s *revokedJWTStore) revoke(tokenHash string, expiresAt time.Time) {
	if tokenHash == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.lastGC) >= s.gcInterval {
		for k, exp := range s.items {
			if !exp.After(now) {
				delete(s.items, k)
			}
		}
		s.lastGC = now
	}
	s.items[tokenHash] = expiresAt
}

func (s *revokedJWTStore) isRevoked(tokenHash string) bool {
	if tokenHash == "" {
		return false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.items[tokenHash]
	if !ok {
		return false
	}
	if !exp.After(now) {
		delete(s.items, tokenHash)
		return false
	}
	return true
}

func hashToken(tokenString string) string {
	if strings.TrimSpace(tokenString) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(tokenString))
	return hex.EncodeToString(sum[:])
}

func parseAndValidateJWT(tokenString string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

type loginLimiterStore struct {
	mu         sync.Mutex
	limiters   map[string]*rate.Limiter
	lastSeen   map[string]time.Time
	r          rate.Limit
	burst      int
	maxIdle    time.Duration
	lastGC     time.Time
	gcInterval time.Duration
}

func newLoginLimiterStore(r rate.Limit, burst int, maxIdle time.Duration) *loginLimiterStore {
	if maxIdle <= 0 {
		maxIdle = 10 * time.Minute
	}
	return &loginLimiterStore{
		limiters:   make(map[string]*rate.Limiter),
		lastSeen:   make(map[string]time.Time),
		r:          r,
		burst:      burst,
		maxIdle:    maxIdle,
		gcInterval: 5 * time.Minute,
		lastGC:     time.Now(),
	}
}

func (s *loginLimiterStore) get(key string) *rate.Limiter {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	if now.Sub(s.lastGC) >= s.gcInterval {
		for k, seen := range s.lastSeen {
			if now.Sub(seen) > s.maxIdle {
				delete(s.lastSeen, k)
				delete(s.limiters, k)
			}
		}
		s.lastGC = now
	}

	lim, ok := s.limiters[key]
	if !ok {
		lim = rate.NewLimiter(s.r, s.burst)
		s.limiters[key] = lim
	}
	s.lastSeen[key] = now
	return lim
}

func getDataDir() string {
	if v := os.Getenv("DATA_DIR"); v != "" {
		return v
	}
	if _, err := os.Stat("/data"); err == nil {
		return "/data"
	}
	return "./data"
}

// InitUserDatabase 初始化用户数据库
func InitUserDatabase() error {
	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	dataDir := getDataDir()
	log.Printf("Ensuring %s directory exists...\n", dataDir)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Printf("Error creating %s directory: %v\n", dataDir, err)
	}
	_ = os.Chmod(dataDir, 0700)

	usersFilePath := filepath.Join(dataDir, "users.json")
	// One-time migration: if we are now using /data but old file exists under ./data
	if dataDir == "/data" {
		legacyPath := filepath.Join("./data", "users.json")
		if _, err := os.Stat(usersFilePath); os.IsNotExist(err) {
			if legacyData, err2 := os.ReadFile(legacyPath); err2 == nil {
				if err3 := os.WriteFile(usersFilePath, legacyData, 0600); err3 == nil {
					log.Printf("Migrated users database from %s to %s\n", legacyPath, usersFilePath)
				}
			}
		}
	}
	log.Printf("Reading users from %s...\n", usersFilePath)

	data, err := os.ReadFile(usersFilePath)
	if err != nil {
		log.Printf("Users file not found, creating default: %v\n", err)
		// 创建默认用户数据库
		now := time.Now()
		userDB = &types.UserDatabase{
			Users: []types.User{
				{
					ID:                 "admin",
					Username:           "admin",
					Password:           "$2a$10$Spuxl0kXOXW2hFb//8Ylj.Nrr./Qpa2Ba0JA0eKprr0NoNHaMJwUC", // bcrypt hash of "admin123"
					Role:               "admin",
					CreatedAt:          now,
					LastLogin:          nil,
					MustChangePassword: true, // Force password change on first login
				},
			},
		}

		// 保存前先解锁
		log.Println("Marshaling user data...")
		jsonData, err := json.MarshalIndent(userDB, "", "  ")
		if err != nil {
			log.Printf("Error marshaling users: %v\n", err)
			return err
		}

		log.Println("Writing to file...")
		if err := os.WriteFile(usersFilePath, jsonData, 0600); err != nil {
			log.Printf("Error writing users file: %v\n", err)
			return err
		}
		_ = os.Chmod(usersFilePath, 0600)
		log.Println("User database created successfully")
		return nil
	}

	log.Println("Parsing users from file...")
	userDB = &types.UserDatabase{}
	if err := json.Unmarshal(data, userDB); err != nil {
		log.Printf("Error parsing users: %v\n", err)
		return err
	}
	log.Printf("Loaded %d users\n", len(userDB.Users))
	return nil
}

// SaveUserDatabase 保存用户数据库
func SaveUserDatabase() error {
	log.Println("Saving user database...")
	data, err := json.MarshalIndent(userDB, "", "  ")
	if err != nil {
		log.Printf("Error marshaling users: %v\n", err)
		return err
	}

	// 使用与InitUserDatabase相同的路径
	dataDir := getDataDir()
	usersFilePath := filepath.Join(dataDir, "users.json")
	log.Printf("Writing to %s...\n", usersFilePath)
	if err := os.WriteFile(usersFilePath, data, 0600); err != nil {
		log.Printf("Error writing users file: %v\n", err)
		return err
	}
	_ = os.Chmod(usersFilePath, 0600)
	log.Println("User database saved successfully")
	return nil
}

// InitJWTKey 初始化JWT密钥
func InitJWTKey() {
	key := os.Getenv("JWT_SECRET")
	if key == "" {
		// 仅允许开发环境使用随机密钥
		if os.Getenv("ENV") == "development" || os.Getenv("DEV") == "true" {
			log.Println("WARNING: JWT_SECRET environment variable is not set, generating random key for development only")
			log.Println("In production, JWT_SECRET must be set and be at least 32 bytes long")
			// 生成随机的32字节密钥（仅用于开发环境）
			randomKey := make([]byte, 32)
			if _, err := rand.Read(randomKey); err != nil {
				log.Fatalf("Failed to generate random JWT key: %v", err)
			}
			jwtKey = randomKey
			return
		}
		// 生产环境必须配置JWT_SECRET
		log.Fatal("ERROR: JWT_SECRET environment variable is required in production")
		log.Fatal("Please set a strong JWT_SECRET (minimum 32 bytes)")
		os.Exit(1)
	}

	jwtKey = []byte(key)
	if len(jwtKey) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 bytes long for security")
	}
	log.Println("JWT key loaded from environment variable")

	// 检查JWT_SECRET强度
	if len(jwtKey) < 64 {
		log.Println("WARNING: JWT_SECRET is less than 64 bytes. Consider using a longer secret for better security.")
	}
}

// GenerateJWT 生成JWT令牌
func GenerateJWT(username, role string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &jwt.RegisteredClaims{
		Subject:   username,
		ExpiresAt: jwt.NewNumericDate(expirationTime),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
		ID:        fmt.Sprintf("%s-%d", username, time.Now().UnixNano()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// ValidateJWT 验证JWT令牌并返回声明
func ValidateJWT(tokenString string) (*jwt.RegisteredClaims, error) {
	claims, err := parseAndValidateJWT(tokenString)
	if err != nil {
		return nil, err
	}
	if revokedJWTs.isRevoked(hashToken(tokenString)) {
		return nil, fmt.Errorf("token revoked")
	}
	return claims, nil
}

// RevokeJWT marks a JWT string as revoked until its expiration time.
// This makes logout effective server-side (defense in depth).
func RevokeJWT(tokenString string) {
	claims, err := parseAndValidateJWT(tokenString)
	if err != nil {
		return
	}
	exp := time.Now().Add(24 * time.Hour)
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Time
	}
	revokedJWTs.revoke(hashToken(tokenString), exp)
}

// HashPasswordBcrypt 生成密码哈希
func HashPasswordBcrypt(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// ValidateUser 验证用户
func ValidateUser(username, password string) *types.User {
	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	log.Printf("Validating user: %s", username)

	for i, user := range userDB.Users {
		if user.Username == username {
			// 检查账户是否被锁定
			if CheckAccountLock(&user) {
				log.Printf("Account locked for user: %s", username)
				return nil
			}

			log.Printf("Found user: %s, checking password...", username)
			// 使用 bcrypt 验证密码
			if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err == nil {
				log.Printf("Password valid for user: %s", username)
				// 更新用户状态
				userDB.Users[i].FailedLoginCount = 0
				userDB.Users[i].LockedUntil = nil
				now := time.Now()
				userDB.Users[i].LastLogin = &now
				_ = SaveUserDatabase()
				return &userDB.Users[i]
			} else {
				log.Printf("Password invalid for user: %s, error: %v", username, err)
				// 记录失败登录
				userDB.Users[i].FailedLoginCount++
				if userDB.Users[i].FailedLoginCount >= 5 {
					lockDuration := 15 * time.Minute
					lockedUntil := time.Now().Add(lockDuration)
					userDB.Users[i].LockedUntil = &lockedUntil
					log.Printf("Account locked for user: %s due to 5 failed attempts", username)
				}
				_ = SaveUserDatabase()
			}
			return nil
		}
	}
	log.Printf("User not found: %s", username)
	return nil
}

// CheckAccountLock 检查账户是否被锁定
func CheckAccountLock(user *types.User) bool {
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return true
	}
	return false
}

// RecordFailedLogin 记录失败登录信息
func RecordFailedLogin(username, ip string) {
	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	for i := range userDB.Users {
		if userDB.Users[i].Username == username {
			now := time.Now()
			userDB.Users[i].LastFailedLogin = &now
			userDB.Users[i].LastFailedLoginIP = ip
			_ = SaveUserDatabase()
			return
		}
	}
}

// RecordPasswordChange 记录密码修改时间
func RecordPasswordChange(username string) {
	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	for i := range userDB.Users {
		if userDB.Users[i].Username == username {
			now := time.Now()
			userDB.Users[i].LastPasswordChange = &now
			_ = SaveUserDatabase()
			return
		}
	}
}

// GetUserByUsername 根据用户名获取用户
func GetUserByUsername(username string) *types.User {
	userDB_mu.RLock()
	defer userDB_mu.RUnlock()

	for i := range userDB.Users {
		if userDB.Users[i].Username == username {
			return &userDB.Users[i]
		}
	}
	return nil
}

// ChangePassword updates a user's password.
// - If targetUsername is empty, it defaults to requesterUsername.
// - If changing own password, oldPassword must match.
// - If changing another user's password, requesterRole must be "admin".
func ChangePassword(requesterUsername, requesterRole, targetUsername, oldPassword, newPassword string) error {
	requesterUsername = strings.TrimSpace(requesterUsername)
	requesterRole = strings.TrimSpace(requesterRole)
	targetUsername = strings.TrimSpace(targetUsername)
	if targetUsername == "" {
		targetUsername = requesterUsername
	}
	if requesterUsername == "" || targetUsername == "" {
		return errors.New("missing username")
	}
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("new_password is required")
	}

	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	// Locate target user
	idx := -1
	for i := range userDB.Users {
		if userDB.Users[i].Username == targetUsername {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("user not found")
	}

	if targetUsername != requesterUsername {
		if requesterRole != "admin" {
			return fmt.Errorf("forbidden")
		}
	} else {
		// Self-change requires old password verification.
		if strings.TrimSpace(oldPassword) == "" {
			return errors.New("old_password is required")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(userDB.Users[idx].Password), []byte(oldPassword)); err != nil {
			return errors.New("invalid old password")
		}
	}

	hash, err := HashPasswordBcrypt(newPassword)
	if err != nil {
		return fmt.Errorf("password hashing failed: %v", err)
	}
	userDB.Users[idx].Password = hash
	userDB.Users[idx].FailedLoginCount = 0
	userDB.Users[idx].LockedUntil = nil
	userDB.Users[idx].MustChangePassword = false // Clear forced change flag
	now := time.Now()
	userDB.Users[idx].LastPasswordChange = &now
	return SaveUserDatabase()
}

// GetAllUsers 获取所有用户
func GetAllUsers() []types.User {
	userDB_mu.RLock()
	defer userDB_mu.RUnlock()

	users := make([]types.User, len(userDB.Users))
	copy(users, userDB.Users)
	return users
}

// CreateUser 创建新用户
func CreateUser(username, password, role string) error {
	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	// 检查用户是否存在
	for _, u := range userDB.Users {
		if u.Username == username {
			return fmt.Errorf("user already exists")
		}
	}

	// 生成密码哈希
	hash, err := HashPasswordBcrypt(password)
	if err != nil {
		return fmt.Errorf("password hashing failed: %v", err)
	}

	// 添加新用户
	newUser := types.User{
		ID:        fmt.Sprintf("user_%d", len(userDB.Users)),
		Username:  username,
		Password:  hash,
		Role:      role,
		CreatedAt: time.Now(),
		LastLogin: nil,
	}
	userDB.Users = append(userDB.Users, newUser)

	return SaveUserDatabase()
}

// DeleteUser 删除用户
func DeleteUser(username string) error {
	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	// 防止删除管理员
	if username == "admin" {
		return fmt.Errorf("cannot delete admin user")
	}

	found := false
	for i, u := range userDB.Users {
		if u.Username == username {
			userDB.Users = append(userDB.Users[:i], userDB.Users[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("user not found")
	}

	return SaveUserDatabase()
}

// UpdateUserPassword 更新用户密码
func UpdateUserPassword(username, newPassword string) error {
	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	for i := range userDB.Users {
		if userDB.Users[i].Username == username {
			hash, err := HashPasswordBcrypt(newPassword)
			if err != nil {
				return fmt.Errorf("password hashing failed: %v", err)
			}
			userDB.Users[i].Password = hash
			return SaveUserDatabase()
		}
	}

	return fmt.Errorf("user not found")
}

// GetLoginLimiterForKey returns a rate limiter scoped to a caller-provided key
// (e.g. ip address, username, or a composite). This avoids a global limiter DoS.
func GetLoginLimiterForKey(key string) *rate.Limiter {
	if key == "" {
		key = "_"
	}
	return loginLimiters.get(key)
}
