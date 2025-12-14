package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	stdnet "net"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"
)

// --- Structs matching JSON response ---

// 用户结构体
type User struct {
	ID               string     `json:"id"`
	Username         string     `json:"username"`
	Password         string     `json:"password"` // bcrypt hash
	Role             string     `json:"role"`     // "admin" 或 "user"
	CreatedAt        time.Time  `json:"created_at"`
	LastLogin        *time.Time `json:"last_login"`
	FailedLoginCount int        `json:"failed_login_count"`
	LockedUntil      *time.Time `json:"locked_until"`
}

// 告警配置
type AlertConfig struct {
	Enabled       bool    `json:"enabled"`
	WebhookURL    string  `json:"webhook_url"`
	CPUThreshold  float64 `json:"cpu_threshold"`  // Percent
	MemThreshold  float64 `json:"mem_threshold"`  // Percent
	DiskThreshold float64 `json:"disk_threshold"` // Percent
}

var (
	alertConfig   AlertConfig
	alertMutex    sync.RWMutex
	lastAlertTime time.Time
)

// 用户数据库
type UserDatabase struct {
	Users []User `json:"users"`
}

// Session 信息
type SessionInfo struct {
	Username  string
	Role      string
	Token     string
	ExpiresAt time.Time
}

// 操作日志
type OperationLog struct {
	Time      time.Time `json:"time"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
	IPAddress string    `json:"ip_address"`
}

var (
	sessions     = make(map[string]SessionInfo)
	sessions_mu  sync.RWMutex
	userDB       *UserDatabase
	userDB_mu    sync.RWMutex
	opLogs       []OperationLog
	opLogs_mu    sync.RWMutex
	loginLimiter = rate.NewLimiter(1, 5) // 每秒1个，最多5个突发请求

	// Prometheus Metrics
	promCpuUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "system_cpu_usage_percent",
		Help: "Total CPU usage percentage",
	})
	promMemUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "system_memory_usage_percent",
		Help: "Used memory percentage",
	})
	promMemTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "system_memory_total_bytes",
		Help: "Total memory in bytes",
	})
	promMemUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "system_memory_used_bytes",
		Help: "Used memory in bytes",
	})
	promDiskUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_disk_usage_percent",
		Help: "Disk usage percentage by mount point",
	}, []string{"mountpoint"})
	promNetSent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "system_network_sent_bytes_total",
		Help: "Total bytes sent",
	})
	promNetRecv = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "system_network_recv_bytes_total",
		Help: "Total bytes received",
	})
	promTemp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "system_temperature_celsius",
		Help: "Hardware temperature in Celsius",
	}, []string{"sensor"})
)

// 记录操作日志
func logOperation(username, action, details, ip string) {
	opLogs_mu.Lock()
	defer opLogs_mu.Unlock()

	logEntry := OperationLog{
		Time:      time.Now(),
		Username:  username,
		Action:    action,
		Details:   details,
		IPAddress: ip,
	}

	opLogs = append(opLogs, logEntry)
	// 保持最近 1000 条日志
	if len(opLogs) > 1000 {
		opLogs = opLogs[len(opLogs)-1000:]
	}

	// 异步保存到文件（简化处理，实际应使用更稳健的方式）
	go saveOpLogs()
}

// 保存日志到文件
func saveOpLogs() {
	opLogs_mu.RLock()
	data, err := json.MarshalIndent(opLogs, "", "  ")
	opLogs_mu.RUnlock()

	if err != nil {
		log.Printf("Error marshaling logs: %v", err)
		return
	}

	// 忽略错误，日志保存失败不应影响主流程
	_ = ioutil.WriteFile("/data/operations.json", data, 0666)
}

// 加载日志
func loadOpLogs() {
	data, err := ioutil.ReadFile("/data/operations.json")
	if err != nil {
		return
	}

	opLogs_mu.Lock()
	defer opLogs_mu.Unlock()
	json.Unmarshal(data, &opLogs)
}

// 初始化用户数据库
func initUserDatabase() error {
	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	// 确保 /data 目录存在
	log.Println("Ensuring /data directory exists...")
	if err := os.MkdirAll("/data", 0777); err != nil {
		log.Printf("Error creating /data directory: %v\n", err)
	}

	usersFilePath := "/data/users.json"
	log.Printf("Reading users from %s...\n", usersFilePath)

	data, err := ioutil.ReadFile(usersFilePath)
	if err != nil {
		log.Printf("Users file not found, creating default: %v\n", err)
		// 创建默认用户数据库
		now := time.Now()
		userDB = &UserDatabase{
			Users: []User{
				{
					ID:        "admin",
					Username:  "admin",
					Password:  "$2a$10$Spuxl0kXOXW2hFb//8Ylj.Nrr./Qpa2Ba0JA0eKprr0NoNHaMJwUC", // bcrypt hash of "admin123"
					Role:      "admin",
					CreatedAt: now,
					LastLogin: nil,
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
		if err := ioutil.WriteFile(usersFilePath, jsonData, 0666); err != nil {
			log.Printf("Error writing users file: %v\n", err)
			return err
		}
		log.Println("User database created successfully")
		return nil
	}

	log.Println("Parsing users from file...")
	userDB = &UserDatabase{}
	if err := json.Unmarshal(data, userDB); err != nil {
		log.Printf("Error parsing users: %v\n", err)
		return err
	}
	log.Printf("Loaded %d users\n", len(userDB.Users))
	return nil
}

// 保存用户数据库
func saveUserDatabase() error {
	log.Println("Saving user database...")
	data, err := json.MarshalIndent(userDB, "", "  ")
	if err != nil {
		log.Printf("Error marshaling users: %v\n", err)
		return err
	}

	usersFilePath := "/data/users.json"
	log.Printf("Writing to %s...\n", usersFilePath)
	if err := ioutil.WriteFile(usersFilePath, data, 0666); err != nil {
		log.Printf("Error writing users file: %v\n", err)
		return err
	}
	log.Println("User database saved successfully")
	return nil
}

func loadAlerts() {
	alertMutex.Lock()
	defer alertMutex.Unlock()

	data, err := ioutil.ReadFile("/data/alerts.json")
	if err != nil {
		// Default config
		alertConfig = AlertConfig{
			Enabled:       false,
			CPUThreshold:  90,
			MemThreshold:  90,
			DiskThreshold: 90,
		}
		return
	}
	json.Unmarshal(data, &alertConfig)
}

func saveAlerts() error {
	alertMutex.RLock()
	data, err := json.MarshalIndent(alertConfig, "", "  ")
	alertMutex.RUnlock()

	if err != nil {
		return err
	}
	return ioutil.WriteFile("/data/alerts.json", data, 0666)
}

func checkAlerts(cpuPercent float64, memPercent float64, diskPercent float64) {
	alertMutex.RLock()
	config := alertConfig
	alertMutex.RUnlock()

	if !config.Enabled || config.WebhookURL == "" {
		return
	}

	// Debounce: only alert once every 5 minutes
	if time.Since(lastAlertTime) < 5*time.Minute {
		return
	}

	var alerts []string
	if cpuPercent > config.CPUThreshold {
		alerts = append(alerts, fmt.Sprintf("CPU usage is high: %.1f%% (Threshold: %.1f%%)", cpuPercent, config.CPUThreshold))
	}
	if memPercent > config.MemThreshold {
		alerts = append(alerts, fmt.Sprintf("Memory usage is high: %.1f%% (Threshold: %.1f%%)", memPercent, config.MemThreshold))
	}
	if diskPercent > config.DiskThreshold {
		alerts = append(alerts, fmt.Sprintf("Disk usage is high: %.1f%% (Threshold: %.1f%%)", diskPercent, config.DiskThreshold))
	}

	if len(alerts) > 0 {
		message := strings.Join(alerts, "\n")
		go sendWebhook(config.WebhookURL, message)
		lastAlertTime = time.Now()
	}
}

func sendWebhook(url string, message string) {
	payload := map[string]string{
		"text": "Web Monitor Alert:\n" + message,
	}
	jsonPayload, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("Failed to send webhook: %v", err)
		return
	}
	defer resp.Body.Close()
}

// 验证密码策略
func validatePasswordPolicy(password string) bool {
	if len(password) < 8 {
		return false
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, ch := range password {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case (ch >= '!' && ch <= '/') || (ch >= ':' && ch <= '@') || (ch >= '[' && ch <= '`') || (ch >= '{' && ch <= '~'):
			hasSpecial = true
		}
	}

	// 至少包含大写字母、小写字母、数字和特殊字符中的三种
	count := 0
	if hasUpper {
		count++
	}
	if hasLower {
		count++
	}
	if hasDigit {
		count++
	}
	if hasSpecial {
		count++
	}

	return count >= 3
}

// 检查账户是否被锁定
func checkAccountLock(user *User) bool {
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return true
	}
	return false
}

// 记录失败登录
func recordFailedLogin(user *User) {
	user.FailedLoginCount++
	if user.FailedLoginCount >= 5 {
		lockDuration := 15 * time.Minute
		lockedUntil := time.Now().Add(lockDuration)
		user.LockedUntil = &lockedUntil
	}
}

// 记录成功登录
func recordSuccessfulLogin(user *User) {
	user.FailedLoginCount = 0
	user.LockedUntil = nil
	now := time.Now()
	user.LastLogin = &now
}

// 验证用户
func validateUser(username, password string) *User {
	userDB_mu.RLock()
	defer userDB_mu.RUnlock()

	log.Printf("Validating user: %s", username)

	for i, user := range userDB.Users {
		if user.Username == username {
			// 检查账户是否被锁定
			if checkAccountLock(&user) {
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
				saveUserDatabase()
			}
			return nil
		}
	}
	log.Printf("User not found: %s", username)
	return nil
}

// 生成密码哈希
func hashPasswordBcrypt(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// 登录请求结构体
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// 登录响应结构体
type LoginResponse struct {
	Token    string `json:"token"`
	Message  string `json:"message"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type Response struct {
	CPU       CPUInfo               `json:"cpu"`
	Fans      []interface{}         `json:"fans"`
	Sensors   interface{}           `json:"sensors"`
	Power     interface{}           `json:"power"`
	Memory    MemInfo               `json:"memory"`
	Swap      SwapInfo              `json:"swap"`
	Disk      []DiskInfo            `json:"disk"`
	DiskIO    map[string]DiskIOInfo `json:"disk_io"`
	Inodes    []InodeInfo           `json:"inodes"`
	Network   NetInfo               `json:"network"`
	SSHStats  SSHStats              `json:"ssh_stats"`
	BootTime  string                `json:"boot_time"`
	Processes []ProcessInfo         `json:"processes"`
	GPU       []GPUDetail           `json:"gpu"`
}

type GPUDetail struct {
	Index       int          `json:"index"`
	Name        string       `json:"name"`
	Vendor      string       `json:"vendor"`
	PCIAddress  string       `json:"pci_address"`
	DRMCard     string       `json:"drm_card"`
	VRAMTotal   string       `json:"vram_total"`
	VRAMUsed    string       `json:"vram_used"`
	VRAMPercent float64      `json:"vram_percent"`
	FreqMHz     float64      `json:"freq_mhz"`
	TempC       float64      `json:"temp_c"`
	PowerW      float64      `json:"power_w"`
	LoadPercent float64      `json:"load_percent"`
	Processes   []GPUProcess `json:"processes"`
}

type GPUProcess struct {
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	VRAMUsed string `json:"vram_used"`
}

type CPUInfo struct {
	Percent     float64     `json:"percent"`
	PerCore     []float64   `json:"per_core"`
	Times       interface{} `json:"times"`
	LoadAvg     []float64   `json:"load_avg"`
	Stats       interface{} `json:"stats"`
	Freq        CPUFreq     `json:"freq"`
	Info        CPUDetail   `json:"info"`
	TempHistory []float64   `json:"temp_history"`
}

type CPUFreq struct {
	Avg     float64   `json:"avg"`
	PerCore []float64 `json:"per_core"`
}

type CPUDetail struct {
	Model        string  `json:"model"`
	Architecture string  `json:"architecture"`
	Cores        int     `json:"cores"`
	Threads      int     `json:"threads"`
	MaxFreq      float64 `json:"max_freq"`
	MinFreq      float64 `json:"min_freq"`
}

type MemInfo struct {
	Total     string    `json:"total"`
	Used      string    `json:"used"`
	Free      string    `json:"free"`
	Percent   float64   `json:"percent"`
	Available string    `json:"available"`
	Buffers   string    `json:"buffers"`
	Cached    string    `json:"cached"`
	Shared    string    `json:"shared"`
	Active    string    `json:"active"`
	Inactive  string    `json:"inactive"`
	Slab      string    `json:"slab"`
	History   []float64 `json:"history"`
}

type SwapInfo struct {
	Total   string  `json:"total"`
	Used    string  `json:"used"`
	Free    string  `json:"free"`
	Percent float64 `json:"percent"`
	Sin     string  `json:"sin"`
	Sout    string  `json:"sout"`
}

type DiskInfo struct {
	Device     string  `json:"device"`
	Mountpoint string  `json:"mountpoint"`
	Fstype     string  `json:"fstype"`
	Total      string  `json:"total"`
	Used       string  `json:"used"`
	Free       string  `json:"free"`
	Percent    float64 `json:"percent"`
}

type DiskIOInfo struct {
	ReadBytes  string `json:"read_bytes"`
	WriteBytes string `json:"write_bytes"`
	ReadCount  uint64 `json:"read_count"`
	WriteCount uint64 `json:"write_count"`
	ReadTime   uint64 `json:"read_time"`
	WriteTime  uint64 `json:"write_time"`
}

type InodeInfo struct {
	Mountpoint string  `json:"mountpoint"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Free       uint64  `json:"free"`
	Percent    float64 `json:"percent"`
}

type NetInfo struct {
	BytesSent        string               `json:"bytes_sent"`
	BytesRecv        string               `json:"bytes_recv"`
	RawSent          uint64               `json:"raw_sent"`
	RawRecv          uint64               `json:"raw_recv"`
	Interfaces       map[string]Interface `json:"interfaces"`
	Sockets          map[string]int       `json:"sockets"`
	ConnectionStates map[string]int       `json:"connection_states"`
	Errors           map[string]uint64    `json:"errors"`
	ListeningPorts   []ListeningPort      `json:"listening_ports"`
}

type Interface struct {
	IP        string  `json:"ip"`
	BytesSent string  `json:"bytes_sent"`
	BytesRecv string  `json:"bytes_recv"`
	Speed     float64 `json:"speed"`
	IsUp      bool    `json:"is_up"`
	ErrorsIn  uint64  `json:"errors_in"`
	ErrorsOut uint64  `json:"errors_out"`
	DropsIn   uint64  `json:"drops_in"`
	DropsOut  uint64  `json:"drops_out"`
}

type ListeningPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type SSHStats struct {
	Status           string         `json:"status"`
	Connections      int            `json:"connections"`
	Sessions         []interface{}  `json:"sessions"`
	AuthMethods      map[string]int `json:"auth_methods"`
	HostKey          string         `json:"hostkey_fingerprint"`
	HistorySize      int            `json:"history_size"`
	OOMRiskProcesses []ProcessInfo  `json:"oom_risk_processes"`
	FailedLogins     int            `json:"failed_logins"`
	SSHProcessMemory float64        `json:"ssh_process_memory"`
}

type ProcessInfo struct {
	PID           int32         `json:"pid"`
	Name          string        `json:"name"`
	Username      string        `json:"username"`
	NumThreads    int32         `json:"num_threads"`
	MemoryPercent float64       `json:"memory_percent"`
	CPUPercent    float64       `json:"cpu_percent"`
	PPID          int32         `json:"ppid"`
	Uptime        string        `json:"uptime"`
	Cmdline       string        `json:"cmdline"`
	Cwd           string        `json:"cwd"`
	IORead        string        `json:"io_read"`
	IOWrite       string        `json:"io_write"`
	Children      []ProcessInfo `json:"children,omitempty"`
}

// Power Profile
type PowerProfileInfo struct {
	Current   string   `json:"current"`
	Available []string `json:"available"`
	Error     string   `json:"error,omitempty"`
}

// --- Global Caches ---

var (
	cpuTempHistory = make([]float64, 0, 300)
	memHistory     = make([]float64, 0, 300)
	historyMutex   sync.Mutex

	processCache     []ProcessInfo
	lastProcessTime  time.Time
	processCacheLock sync.Mutex

	gpuInfoCache    string
	lastGPUInfoTime time.Time
	gpuInfoLock     sync.Mutex

	connStatesCache    map[string]int
	lastConnStatesTime time.Time
	connStatesLock     sync.Mutex

	sshStatsCache    SSHStats
	lastSSHTime      time.Time
	sshAuthLogOffset int64
	sshAuthCounters  = map[string]int{"publickey": 0, "password": 0, "other": 0, "failed": 0}
	sshStatsLock     sync.Mutex

	// RAPL Cache
	raplReadings = make(map[string]uint64)
	raplTime     time.Time
	raplLock     sync.Mutex
)

// --- Helper Functions ---

func round(val float64) float64 {
	return math.Round(val*100) / 100
}

func getSize(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatUptime(sec uint64) string {
	days := sec / 86400
	sec %= 86400
	hours := sec / 3600
	sec %= 3600
	mins := sec / 60
	secs := sec % 60
	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 || len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 || len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	parts = append(parts, fmt.Sprintf("%ds", secs))
	return strings.Join(parts, " ")
}

func detectOSName() string {
	paths := []string{"/hostfs/etc/os-release", "/etc/os-release"}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		m := make(map[string]string)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if idx := strings.Index(line, "="); idx > 0 {
				key := line[:idx]
				val := strings.Trim(line[idx+1:], "\"")
				m[key] = val
			}
		}
		if v, ok := m["PRETTY_NAME"]; ok && v != "" {
			return v
		}
		name := m["NAME"]
		ver := m["VERSION"]
		if name != "" && ver != "" {
			return fmt.Sprintf("%s %s", name, ver)
		}
		if name != "" {
			return name
		}
	}
	return ""
}

func getCPUInfo() CPUDetail {
	info := CPUDetail{
		Model:        "Unknown",
		Architecture: runtime.GOARCH,
		Cores:        0,
		Threads:      0,
		MaxFreq:      0,
		MinFreq:      0,
	}

	// Try to read model from /proc/cpuinfo
	if file, err := os.Open("/proc/cpuinfo"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "model name") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					info.Model = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	info.Cores, _ = cpu.Counts(false)
	info.Threads, _ = cpu.Counts(true)

	// Get frequency info
	if freqs, err := cpu.Info(); err == nil && len(freqs) > 0 {
		info.MaxFreq = freqs[0].Mhz
	}

	return info
}

func lookupPCIName(vendorID, deviceID string) string {
	// Strip 0x prefix
	vendorID = strings.TrimPrefix(vendorID, "0x")
	deviceID = strings.TrimPrefix(deviceID, "0x")

	// Common locations for pci.ids
	paths := []string{
		"/usr/share/hwdata/pci.ids",
		"/usr/share/pci.ids",
		"/usr/share/misc/pci.ids",
		"/hostfs/usr/share/hwdata/pci.ids", // Try hostfs
		"/hostfs/usr/share/misc/pci.ids",   // Try hostfs
	}

	var file *os.File
	var err error
	for _, path := range paths {
		file, err = os.Open(path)
		if err == nil {
			break
		}
	}
	if file == nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inVendor := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		if !strings.HasPrefix(line, "\t") {
			// Vendor line: "1234  Vendor Name"
			if strings.HasPrefix(line, vendorID) {
				inVendor = true
			} else {
				inVendor = false
			}
		} else if inVendor && strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t") {
			// Device line: "\t5678  Device Name"
			trimmed := strings.TrimPrefix(line, "\t")
			if strings.HasPrefix(trimmed, deviceID) {
				// Found it. Extract name.
				// Usually "ID  Name" (two spaces)
				if len(trimmed) > len(deviceID) {
					return strings.TrimSpace(trimmed[len(deviceID):])
				}
			}
		}
	}
	return ""
}

func parseBytes(s string) uint64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func getGPUDetails() []GPUDetail {
	var details []GPUDetail
	matches, _ := filepath.Glob("/sys/class/drm/card*")
	sort.Strings(matches)

	for i, cardPath := range matches {
		// Skip virtual devices if they don't have a physical device link
		// But for some iGPUs, the link might be different.
		// Let's try to read vendor/device directly first.

		vendorFile := filepath.Join(cardPath, "device/vendor")
		deviceFile := filepath.Join(cardPath, "device/device")

		// If device/vendor doesn't exist, try direct vendor (some drivers expose it differently)
		if _, err := os.Stat(vendorFile); os.IsNotExist(err) {
			vendorFile = filepath.Join(cardPath, "vendor")
			deviceFile = filepath.Join(cardPath, "device")
		}

		vendorBytes, err1 := ioutil.ReadFile(vendorFile)
		deviceBytes, err2 := ioutil.ReadFile(deviceFile)

		if err1 == nil && err2 == nil {
			vendor := strings.ToLower(strings.TrimSpace(string(vendorBytes)))
			device := strings.ToLower(strings.TrimSpace(string(deviceBytes)))

			// Ensure we have valid hex IDs
			if !strings.HasPrefix(vendor, "0x") {
				vendor = "0x" + vendor
			}
			if !strings.HasPrefix(device, "0x") {
				device = "0x" + device
			}

			realName := lookupPCIName(vendor, device)
			var gpuName string
			if realName != "" {
				lowerName := strings.ToLower(realName)
				if vendor == "0x8086" && !strings.Contains(lowerName, "intel") {
					gpuName = "Intel " + realName
				} else if vendor == "0x10de" && !strings.Contains(lowerName, "nvidia") {
					gpuName = "NVIDIA " + realName
				} else if vendor == "0x1002" && !strings.Contains(lowerName, "amd") {
					gpuName = "AMD " + realName
				} else {
					gpuName = realName
				}
			} else {
				gpuName = fmt.Sprintf("Unknown [%s:%s]", vendor, device)
			}

			pciAddr := ""
			if link, err := os.Readlink(filepath.Join(cardPath, "device")); err == nil {
				pciAddr = filepath.Base(link)
			}

			var vramTotal, vramUsed uint64
			// Intel i915 specific paths
			if content, err := ioutil.ReadFile(filepath.Join(cardPath, "gt/gt0/mem_info_vram_total")); err == nil {
				vramTotal = parseBytes(string(content))
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/mem_info_vram_total")); err == nil {
				vramTotal = parseBytes(string(content))
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/drm/card0/gt/gt0/mem_info_vram_total")); err == nil {
				// Try deeper path for some setups
				vramTotal = parseBytes(string(content))
			}

			if content, err := ioutil.ReadFile(filepath.Join(cardPath, "gt/gt0/mem_info_vram_used")); err == nil {
				vramUsed = parseBytes(string(content))
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/mem_info_vram_used")); err == nil {
				vramUsed = parseBytes(string(content))
			}

			var freq float64
			if content, err := ioutil.ReadFile(filepath.Join(cardPath, "gt_act_freq_mhz")); err == nil {
				freq = parseFloat(string(content))
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/pp_dpm_sclk")); err == nil {
				lines := strings.Split(string(content), "\n")
				for _, line := range lines {
					if strings.Contains(line, "*") {
						parts := strings.Fields(line)
						if len(parts) >= 2 {
							valStr := strings.TrimSuffix(parts[1], "Mhz")
							freq = parseFloat(valStr)
						}
						break
					}
				}
			} else if content, err := ioutil.ReadFile(filepath.Join(cardPath, "gt_cur_freq_mhz")); err == nil {
				freq = parseFloat(string(content))
			}

			var temp float64
			hwmonGlob, _ := filepath.Glob(filepath.Join(cardPath, "device/hwmon/hwmon*"))
			for _, hwmon := range hwmonGlob {
				if content, err := ioutil.ReadFile(filepath.Join(hwmon, "temp1_input")); err == nil {
					temp = parseFloat(string(content)) / 1000
					break
				}
			}

			var power float64
			for _, hwmon := range hwmonGlob {
				if content, err := ioutil.ReadFile(filepath.Join(hwmon, "power1_average")); err == nil {
					power = parseFloat(string(content)) / 1000000
					break
				}
			}

			var loadVal float64
			if content, err := ioutil.ReadFile(filepath.Join(cardPath, "device/gpu_busy_percent")); err == nil {
				loadVal = parseFloat(string(content))
			}

			detail := GPUDetail{
				Index:       i,
				Name:        gpuName,
				Vendor:      vendor,
				PCIAddress:  pciAddr,
				DRMCard:     filepath.Base(cardPath),
				VRAMTotal:   getSize(vramTotal),
				VRAMUsed:    getSize(vramUsed),
				FreqMHz:     freq,
				TempC:       temp,
				PowerW:      power,
				LoadPercent: loadVal,
			}
			if vramTotal > 0 {
				detail.VRAMPercent = round(float64(vramUsed) / float64(vramTotal) * 100)
			}

			details = append(details, detail)
		} else {
			fmt.Printf("Failed to read vendor/device for %s: %v, %v\n", cardPath, err1, err2)
		}
	}
	return details
}

func getGPUInfo() string {
	gpuInfoLock.Lock()
	defer gpuInfoLock.Unlock()

	if time.Since(lastGPUInfoTime) < 60*time.Second && gpuInfoCache != "" {
		return gpuInfoCache
	}

	var gpus []string
	seen := make(map[string]bool)

	matches, _ := filepath.Glob("/sys/class/drm/card*")
	for _, cardPath := range matches {
		vendorFile := filepath.Join(cardPath, "device/vendor")
		deviceFile := filepath.Join(cardPath, "device/device")

		vendorBytes, err1 := ioutil.ReadFile(vendorFile)
		deviceBytes, err2 := ioutil.ReadFile(deviceFile)

		if err1 == nil && err2 == nil {
			vendor := strings.ToLower(strings.TrimSpace(string(vendorBytes)))
			device := strings.ToLower(strings.TrimSpace(string(deviceBytes)))

			// Deduplicate based on vendor+device ID
			key := vendor + ":" + device
			if seen[key] {
				continue
			}
			seen[key] = true

			// Try to lookup real name
			realName := lookupPCIName(vendor, device)
			var gpuName string

			if realName != "" {
				// Ensure vendor name is present for clarity
				lowerName := strings.ToLower(realName)
				if vendor == "0x8086" && !strings.Contains(lowerName, "intel") {
					gpuName = "Intel " + realName
				} else if vendor == "0x10de" && !strings.Contains(lowerName, "nvidia") {
					gpuName = "NVIDIA " + realName
				} else if vendor == "0x1002" && !strings.Contains(lowerName, "amd") {
					gpuName = "AMD " + realName
				} else {
					gpuName = realName
				}
			} else {
				// Fallback
				switch vendor {
				case "0x8086":
					gpuName = fmt.Sprintf("Intel [%s]", device)
				case "0x10de":
					gpuName = fmt.Sprintf("NVIDIA [%s]", device)
				case "0x1002":
					gpuName = fmt.Sprintf("AMD [%s]", device)
				default:
					gpuName = fmt.Sprintf("Generic [%s:%s]", vendor, device)
				}
			}
			gpus = append(gpus, gpuName)
		}
	}

	if len(gpus) == 0 {
		gpuInfoCache = "Unknown GPU"
	} else {
		gpuInfoCache = strings.Join(gpus, " + ")
	}

	lastGPUInfoTime = time.Now()
	return gpuInfoCache
}

func getCPUStats() map[string]uint64 {
	stats := make(map[string]uint64)
	if file, err := os.Open("/proc/stat"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}

			key := parts[0]
			val, _ := strconv.ParseUint(parts[1], 10, 64)

			switch key {
			case "ctxt":
				stats["ctx_switches"] = val
			case "intr":
				stats["interrupts"] = val
			case "softirq":
				stats["soft_interrupts"] = val
			}
		}
	}
	// syscalls is not typically in /proc/stat on Linux, usually 0 or requires other source
	stats["syscalls"] = 0
	return stats
}

func getSensors() interface{} {
	sensors := make(map[string][]interface{})
	if temps, err := host.SensorsTemperatures(); err == nil {
		for _, t := range temps {
			sensors[t.SensorKey] = append(sensors[t.SensorKey], map[string]interface{}{
				"label":    t.SensorKey,
				"current":  t.Temperature,
				"high":     t.High,
				"critical": t.Critical,
			})
		}
	}
	return sensors
}

func getPower() interface{} {
	powerStatus := make(map[string]interface{})

	// Battery
	basePath := "/sys/class/power_supply"
	if _, err := os.Stat(basePath); err == nil {
		files, _ := ioutil.ReadDir(basePath)
		for _, f := range files {
			supplyPath := filepath.Join(basePath, f.Name())

			// Try power_now
			if content, err := ioutil.ReadFile(filepath.Join(supplyPath, "power_now")); err == nil {
				if pNow, err := strconv.ParseFloat(strings.TrimSpace(string(content)), 64); err == nil {
					powerStatus["consumption_watts"] = round(pNow / 1000000.0)
					break
				}
			}

			// Try voltage_now * current_now
			vBytes, err1 := ioutil.ReadFile(filepath.Join(supplyPath, "voltage_now"))
			cBytes, err2 := ioutil.ReadFile(filepath.Join(supplyPath, "current_now"))
			if err1 == nil && err2 == nil {
				vNow, _ := strconv.ParseFloat(strings.TrimSpace(string(vBytes)), 64)
				cNow, _ := strconv.ParseFloat(strings.TrimSpace(string(cBytes)), 64)
				powerStatus["consumption_watts"] = round((vNow * cNow) / 1e12)
				break
			}
		}
	}

	// RAPL (Intel Power)
	raplLock.Lock()
	defer raplLock.Unlock()

	raplBasePath := "/sys/class/powercap"
	now := time.Now()

	// Find RAPL domains
	if matches, err := filepath.Glob(filepath.Join(raplBasePath, "intel-rapl:*")); err == nil {
		totalWatts := 0.0
		hasNewReading := false

		for _, domainPath := range matches {
			// Check if it's a package domain
			nameFile := filepath.Join(domainPath, "name")
			if nameBytes, err := ioutil.ReadFile(nameFile); err == nil {
				name := strings.TrimSpace(string(nameBytes))
				// We typically care about "package-X" for total CPU power
				// But let's just sum up everything that looks like a package or dram if we want detailed
				// The Python code sums up "Package" domains for total.

				energyFile := filepath.Join(domainPath, "energy_uj")
				maxEnergyFile := filepath.Join(domainPath, "max_energy_range_uj")

				if energyBytes, err := ioutil.ReadFile(energyFile); err == nil {
					energyUj, _ := strconv.ParseUint(strings.TrimSpace(string(energyBytes)), 10, 64)

					// Calculate watts if we have previous reading
					if lastEnergy, ok := raplReadings[domainPath]; ok && !raplTime.IsZero() {
						dt := now.Sub(raplTime).Seconds()
						if dt > 0 {
							// Handle overflow/reset
							var de uint64
							if energyUj >= lastEnergy {
								de = energyUj - lastEnergy
							} else {
								// Wrapped around
								maxRange := uint64(0)
								if maxBytes, err := ioutil.ReadFile(maxEnergyFile); err == nil {
									maxRange, _ = strconv.ParseUint(strings.TrimSpace(string(maxBytes)), 10, 64)
								}
								if maxRange > 0 {
									de = (maxRange - lastEnergy) + energyUj
								} else {
									de = 0 // Can't calculate
								}
							}

							watts := (float64(de) / 1000000.0) / dt

							if strings.HasPrefix(name, "package") || strings.HasPrefix(name, "dram") {
								// Add to detailed map if we want to return it
								// But for now, let's just update total consumption if not already set by battery
								if strings.HasPrefix(name, "package") {
									totalWatts += watts
								}
							}
						}
					}

					raplReadings[domainPath] = energyUj
					hasNewReading = true
				}
			}
		}

		if hasNewReading {
			raplTime = now
			if totalWatts > 0 {
				// Prefer RAPL over battery if available (or sum them? usually one or other)
				// If we already have battery, maybe this is desktop and battery is UPS?
				// Usually RAPL is more accurate for CPU power.
				// Let's just overwrite or add? Python code:
				// if total_watts > 0: power_status["consumption_watts"] = total_watts
				powerStatus["consumption_watts"] = round(totalWatts)
			}
		}
	}

	return powerStatus
}

func getTopProcesses() []ProcessInfo {
	processCacheLock.Lock()
	defer processCacheLock.Unlock()

	if time.Since(lastProcessTime) < 15*time.Second && processCache != nil {
		return processCache
	}

	procs, err := process.Processes()
	if err != nil {
		return []ProcessInfo{}
	}

	var result []ProcessInfo
	for _, p := range procs {
		name, _ := p.Name()
		username, _ := p.Username()
		if username == "" {
			if uids, err := p.Uids(); err == nil && len(uids) > 0 {
				username = fmt.Sprintf("uid:%d", uids[0])
			} else {
				username = "unknown"
			}
		}
		numThreads, _ := p.NumThreads()
		memPercent, _ := p.MemoryPercent()
		cpuPercent, _ := p.CPUPercent()
		ppid, _ := p.Ppid()
		createTime, _ := p.CreateTime() // ms

		uptimeSec := time.Now().Unix() - (createTime / 1000)
		uptimeStr := "-"
		if uptimeSec < 60 {
			uptimeStr = fmt.Sprintf("%ds", uptimeSec)
		} else if uptimeSec < 3600 {
			uptimeStr = fmt.Sprintf("%dm", uptimeSec/60)
		} else if uptimeSec < 86400 {
			uptimeStr = fmt.Sprintf("%dh", uptimeSec/3600)
		} else {
			uptimeStr = fmt.Sprintf("%dd", uptimeSec/86400)
		}

		cmdline, _ := p.Cmdline()

		result = append(result, ProcessInfo{
			PID:           p.Pid,
			Name:          name,
			Username:      username,
			NumThreads:    numThreads,
			MemoryPercent: round(float64(memPercent)),
			CPUPercent:    round(cpuPercent),
			PPID:          ppid,
			Uptime:        uptimeStr,
			Cmdline:       cmdline,
			Cwd:           "-",
			IORead:        "-",
			IOWrite:       "-",
		})
	}

	// Sort by memory percent desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].MemoryPercent > result[j].MemoryPercent
	})

	processCache = result
	lastProcessTime = time.Now()
	return result
}

func getConnectionStates() map[string]int {
	connStatesLock.Lock()
	defer connStatesLock.Unlock()

	if time.Since(lastConnStatesTime) < 10*time.Second && connStatesCache != nil {
		return connStatesCache
	}

	states := map[string]int{
		"ESTABLISHED": 0, "SYN_SENT": 0, "SYN_RECV": 0, "FIN_WAIT1": 0,
		"FIN_WAIT2": 0, "TIME_WAIT": 0, "CLOSE": 0, "CLOSE_WAIT": 0,
		"LAST_ACK": 0, "LISTEN": 0, "CLOSING": 0,
	}

	// Try gopsutil first
	conns, err := net.Connections("tcp")
	if err == nil && len(conns) > 0 {
		for _, c := range conns {
			if _, ok := states[c.Status]; ok {
				states[c.Status]++
			}
		}
	} else {
		// Fallback: Parse /proc/net/tcp directly
		states = parseNetTCP("/proc/net/tcp")
		if len(states) == 0 || (states["LISTEN"]+states["ESTABLISHED"] == 0) {
			states = parseNetTCP("/hostfs/proc/net/tcp")
		}
	}

	connStatesCache = states
	lastConnStatesTime = time.Now()
	return states
}

func parseNetTCP(path string) map[string]int {
	states := map[string]int{
		"ESTABLISHED": 0, "SYN_SENT": 0, "SYN_RECV": 0, "FIN_WAIT1": 0,
		"FIN_WAIT2": 0, "TIME_WAIT": 0, "CLOSE": 0, "CLOSE_WAIT": 0,
		"LAST_ACK": 0, "LISTEN": 0, "CLOSING": 0,
	}

	// TCP state numbers in /proc/net/tcp
	stateMap := map[string]string{
		"01": "ESTABLISHED",
		"02": "SYN_SENT",
		"03": "SYN_RECV",
		"04": "FIN_WAIT1",
		"05": "FIN_WAIT2",
		"06": "TIME_WAIT",
		"07": "CLOSE",
		"08": "CLOSE_WAIT",
		"09": "LAST_ACK",
		"0A": "LISTEN",
		"0B": "CLOSING",
	}

	file, err := os.Open(path)
	if err != nil {
		return states
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		if lineCount == 1 { // Skip header
			continue
		}

		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		stateHex := fields[3]
		if stateName, ok := stateMap[strings.ToUpper(stateHex)]; ok {
			states[stateName]++
		}
	}

	return states
}

func getSSHConnectionCount() int {
	// Try using gopsutil first
	conns, err := net.Connections("tcp")
	if err == nil && len(conns) > 0 {
		count := 0
		for _, c := range conns {
			if c.Laddr.Port == 22 && c.Status == "ESTABLISHED" {
				count++
			}
		}
		if count > 0 {
			return count
		}
	}

	// Fallback: Parse /proc/net/tcp directly
	for _, path := range []string{"/proc/net/tcp", "/hostfs/proc/net/tcp"} {
		if count := countSSHConnectionsFromProc(path); count > 0 {
			return count
		}
	}

	return 0
}

func countSSHConnectionsFromProc(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	lineCount := 0

	for scanner.Scan() {
		lineCount++
		if lineCount == 1 { // Skip header
			continue
		}

		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		// Get local port (field 1: local address in format IP:PORT in hex)
		localAddr := fields[1]
		parts := strings.Split(localAddr, ":")
		if len(parts) < 2 {
			continue
		}

		// Parse port from hex
		portHex := parts[len(parts)-1]
		port, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil || port != 22 {
			continue
		}

		// Check if state is ESTABLISHED (01)
		state := fields[3]
		if state == "01" {
			count++
		}
	}

	return count
}

func getListeningPorts() []ListeningPort {
	portsMap := make(map[int]map[string]string) // port -> protocol -> process

	conns, err := net.Connections("all")
	if err == nil && len(conns) > 0 {
		for _, c := range conns {
			if c.Status == "LISTEN" {
				port := int(c.Laddr.Port)
				proto := "TCP"
				if c.Type == 2 {
					proto = "UDP"
				}

				if _, ok := portsMap[port]; !ok {
					portsMap[port] = make(map[string]string)
				}

				procName := fmt.Sprintf("PID %d", c.Pid)
				if p, err := process.NewProcess(c.Pid); err == nil {
					if name, err := p.Name(); err == nil {
						procName = name
					}
				}
				portsMap[port][strings.ToLower(proto)] = procName
			}
		}
	} else {
		// Fallback: Parse /proc/net/tcp and /proc/net/udp
		parseListeningPorts("/proc/net/tcp", portsMap, "TCP")
		parseListeningPorts("/proc/net/udp", portsMap, "UDP")

		// If still empty, try /hostfs
		if len(portsMap) == 0 {
			parseListeningPorts("/hostfs/proc/net/tcp", portsMap, "TCP")
			parseListeningPorts("/hostfs/proc/net/udp", portsMap, "UDP")
		}
	}

	var result []ListeningPort
	for port, protos := range portsMap {
		var protoStrs []string
		if name, ok := protos["tcp"]; ok {
			protoStrs = append(protoStrs, fmt.Sprintf("TCP:%s", name))
		}
		if name, ok := protos["udp"]; ok {
			protoStrs = append(protoStrs, fmt.Sprintf("UDP:%s", name))
		}

		result = append(result, ListeningPort{
			Port:     port,
			Protocol: strings.Join(protoStrs, ", "),
		})
	}

	// Sort by port
	sort.Slice(result, func(i, j int) bool {
		return result[i].Port < result[j].Port
	})

	if len(result) > 20 {
		return result[:20]
	}
	return result
}

func parseListeningPorts(path string, portsMap map[int]map[string]string, proto string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		if lineCount == 1 { // Skip header
			continue
		}

		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		// Parse local address (format: 127.0.0.1:8080 -> hex:hex)
		localAddr := fields[1]
		parts := strings.Split(localAddr, ":")
		if len(parts) < 2 {
			continue
		}

		// Parse port (hex format)
		portHex := parts[len(parts)-1]
		port, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil {
			continue
		}

		// Check state (field 3 is state, should be 0A for LISTEN)
		state := fields[3]
		if state != "0A" { // Only LISTEN ports
			continue
		}

		portNum := int(port)
		if _, ok := portsMap[portNum]; !ok {
			portsMap[portNum] = make(map[string]string)
		}

		// Try to find process name (field 9 is inode, but we'll just use a generic name)
		portsMap[portNum][strings.ToLower(proto)] = "System"
	}
}

func getSSHStats() SSHStats {
	sshStatsLock.Lock()
	defer sshStatsLock.Unlock()

	if time.Since(lastSSHTime) < 120*time.Second && sshStatsCache.Status != "" {
		return sshStatsCache
	}

	stats := SSHStats{
		Status:       "Stopped",
		Connections:  0,
		Sessions:     []interface{}{},
		AuthMethods:  map[string]int{"password": 0, "publickey": 0, "other": 0},
		FailedLogins: 0,
		HostKey:      "-",
		HistorySize:  0,
	}

	// 1. Check SSH port status and connections
	conns, err := net.Connections("tcp")
	if err == nil && len(conns) > 0 {
		for _, c := range conns {
			if c.Laddr.Port == 22 {
				if c.Status == "LISTEN" {
					stats.Status = "Running"
				} else if c.Status == "ESTABLISHED" {
					stats.Connections++
				}
			}
		}
	}

	// Check SSH status and fallback for connections
	if stats.Status != "Running" {
		// Fallback: Check /proc/net/tcp for port 22
		for _, path := range []string{"/proc/net/tcp", "/hostfs/proc/net/tcp"} {
			if tcpStates := parseNetTCP(path); tcpStates["LISTEN"] > 0 {
				stats.Status = "Running"
				break
			}
		}
	}

	// Get SSH connection count with fallback
	if stats.Connections == 0 {
		stats.Connections = getSSHConnectionCount()
	}

	// Use `who` for sessions
	if out, err := exec.Command("who").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) >= 5 {
				user := parts[0]
				startedStr := parts[2] + " " + parts[3]

				// Try to parse time and convert to ISO 8601 (UTC)
				// who output format: YYYY-MM-DD HH:MM
				if t, err := time.ParseInLocation("2006-01-02 15:04", startedStr, time.Local); err == nil {
					startedStr = t.UTC().Format(time.RFC3339)
				}

				// Extract IP from all fields with parentheses and deduplicate
				ipsFound := make(map[string]bool)
				var ip string
				for _, part := range parts {
					if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") {
						candidate := part[1 : len(part)-1]
						// Filter out local X11 or tmux if they don't look like IPs
						if stdnet.ParseIP(candidate) != nil && !ipsFound[candidate] {
							ipsFound[candidate] = true
							ip = candidate // Keep the first valid IP
							break          // Only take first valid IP, ignore duplicates
						}
					}
				}

				// Only add if we found a valid IP
				if ip != "" {
					stats.Sessions = append(stats.Sessions, map[string]string{
						"user":    user,
						"ip":      ip,
						"started": startedStr,
					})
				}
			}
		}
	}

	// 2. Host Key
	keyPaths := []string{"/etc/ssh/ssh_host_rsa_key.pub", "/hostfs/etc/ssh/ssh_host_rsa_key.pub"}
	for _, path := range keyPaths {
		if content, err := ioutil.ReadFile(path); err == nil {
			parts := strings.Fields(string(content))
			if len(parts) >= 2 {
				stats.HostKey = parts[1]
				break
			}
		}
	}

	// 3. Auth Logs (Incremental)
	logPaths := []string{"/var/log/auth.log", "/hostfs/var/log/auth.log", "/var/log/secure", "/hostfs/var/log/secure"}
	var authLogPath string
	for _, path := range logPaths {
		if _, err := os.Stat(path); err == nil {
			authLogPath = path
			break
		}
	}

	if authLogPath != "" {
		file, err := os.Open(authLogPath)
		if err == nil {
			stat, _ := file.Stat()
			fileSize := stat.Size()

			if sshAuthLogOffset > 0 && sshAuthLogOffset <= fileSize {
				file.Seek(sshAuthLogOffset, 0)
			} else {
				start := fileSize - 10000
				if start < 0 {
					start = 0
				}
				file.Seek(start, 0)
			}

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if !strings.Contains(line, "sshd") {
					continue
				}

				if strings.Contains(line, "Failed password") || strings.Contains(line, "Invalid user") || strings.Contains(line, "authentication failure") {
					sshAuthCounters["failed"]++
				} else if strings.Contains(line, "Accepted publickey") {
					sshAuthCounters["publickey"]++
				} else if strings.Contains(line, "Accepted password") {
					sshAuthCounters["password"]++
				} else if strings.Contains(line, "Accepted") {
					sshAuthCounters["other"]++
				}
			}
			sshAuthLogOffset, _ = file.Seek(0, 1) // Current position
			file.Close()
		}
	}

	stats.AuthMethods["publickey"] = sshAuthCounters["publickey"]
	stats.AuthMethods["password"] = sshAuthCounters["password"]
	stats.AuthMethods["other"] = sshAuthCounters["other"]
	stats.FailedLogins = sshAuthCounters["failed"]

	// 4. History Size (known_hosts)
	knownHostsPaths := []string{
		"/root/.ssh/known_hosts",
		"/hostfs/root/.ssh/known_hosts",
		os.ExpandEnv("$HOME/.ssh/known_hosts"),
	}
	// Also try common user home locations on host
	globPatterns := []string{
		"/home/*/.ssh/known_hosts",
		"/hostfs/home/*/.ssh/known_hosts",
	}
	for _, pattern := range globPatterns {
		if matches, err := filepath.Glob(pattern); err == nil {
			knownHostsPaths = append(knownHostsPaths, matches...)
		}
	}
	for _, path := range knownHostsPaths {
		if content, err := ioutil.ReadFile(path); err == nil {
			stats.HistorySize = len(strings.Split(strings.TrimSpace(string(content)), "\n"))
			if stats.HistorySize > 0 {
				break
			}
		}
	}

	// 5. SSH Process Memory
	// Calculate total memory usage of all sshd processes
	procs := getTopProcesses()
	var totalMem float64
	for _, p := range procs {
		if p.Name == "sshd" || strings.Contains(p.Cmdline, "sshd") {
			totalMem += p.MemoryPercent
		}
	}
	stats.SSHProcessMemory = round(totalMem)

	// Fallback: if no sessions captured but connections exist, infer from TCP 22
	// Also try to find sshd processes to get real users
	if len(stats.Sessions) == 0 {
		// Get active SSH connections first to try to match IPs
		activeIPs := []string{}
		if conns, err := net.Connections("tcp"); err == nil {
			for _, c := range conns {
				if c.Laddr.Port == 22 && c.Status == "ESTABLISHED" {
					activeIPs = append(activeIPs, c.Raddr.IP)
				}
			}
		}

		// Use getTopProcesses to leverage cache and avoid double scanning
		procs := getTopProcesses()
		for _, pInfo := range procs {
			if pInfo.Name == "sshd" {
				// Pattern: sshd: user@pts/0 OR sshd: user@notty
				if strings.Contains(pInfo.Cmdline, "@") {
					parts := strings.Split(pInfo.Cmdline, "@")
					if len(parts) > 0 {
						userPart := parts[0]
						// sshd: user
						userParts := strings.Fields(userPart)
						if len(userParts) > 1 {
							user := userParts[len(userParts)-1]

							// Find IP from connection
							ip := "Unknown"
							if len(activeIPs) == 1 {
								ip = activeIPs[0]
							} else if len(activeIPs) > 0 {
								ip = strings.Join(activeIPs, ", ")
							}

							var started int64 = 0
							// Need CreateTime from actual process
							if p, err := process.NewProcess(pInfo.PID); err == nil {
								if createTime, err := p.CreateTime(); err == nil {
									started = createTime
								}
							}

							stats.Sessions = append(stats.Sessions, map[string]interface{}{
								"user":    user,
								"ip":      ip,
								"started": started,
							})
						}
					}
				}
			}
		}
	}

	// If still empty, use the TCP fallback
	if len(stats.Sessions) == 0 && stats.Connections > 0 {
		conns, _ := net.Connections("tcp")
		for _, c := range conns {
			if c.Laddr.Port == 22 && c.Status == "ESTABLISHED" {
				ip := c.Raddr.IP
				stats.Sessions = append(stats.Sessions, map[string]interface{}{
					"user":    "ssh (est)",
					"ip":      ip,
					"started": 0,
				})
			}
		}
	}

	sshStatsCache = stats
	lastSSHTime = time.Now()
	return stats
}

func getPowerProfile() PowerProfileInfo {
	// Try powerprofilesctl first (via chroot)
	cmd := exec.Command("chroot", "/hostfs", "powerprofilesctl", "list")
	out, err := cmd.Output()
	if err == nil {
		output := string(out)
		info := PowerProfileInfo{Available: []string{}}
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasSuffix(line, ":") {
				name := strings.TrimSuffix(line, ":")
				isCurrent := false
				if strings.HasPrefix(name, "* ") {
					name = strings.TrimPrefix(name, "* ")
					isCurrent = true
				}
				name = strings.TrimSpace(name)
				info.Available = append(info.Available, name)
				if isCurrent {
					info.Current = name
				}
			}
		}
		if info.Current != "" {
			return info
		}
	}

	// Fallback to sysfs
	if content, err := ioutil.ReadFile("/sys/firmware/acpi/platform_profile"); err == nil {
		current := strings.TrimSpace(string(content))
		choices := []string{}
		if choicesContent, err := ioutil.ReadFile("/sys/firmware/acpi/platform_profile_choices"); err == nil {
			choices = strings.Fields(string(choicesContent))
		}
		return PowerProfileInfo{
			Current:   current,
			Available: choices,
		}
	}

	return PowerProfileInfo{Error: "Not supported"}
}

func setPowerProfile(profile string) error {
	// Validate
	info := getPowerProfile()
	valid := false
	for _, p := range info.Available {
		if p == profile {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid profile")
	}

	// Try powerprofilesctl
	cmd := exec.Command("chroot", "/hostfs", "powerprofilesctl", "set", profile)
	if output, err := cmd.CombinedOutput(); err == nil {
		return nil
	} else {
		log.Printf("powerprofilesctl failed: %v, output: %s", err, string(output))
	}

	// Fallback to sysfs
	sysfsProfile := profile
	if profile == "power-saver" {
		sysfsProfile = "low-power"
	}
	return ioutil.WriteFile("/sys/firmware/acpi/platform_profile", []byte(sysfsProfile), 0644)
}

func collectStats() Response {
	var resp Response

	// CPU
	cpuPercent, _ := cpu.Percent(0, false)
	if len(cpuPercent) > 0 {
		resp.CPU.Percent = round(cpuPercent[0])
	}

	perCore, _ := cpu.Percent(0, true)
	resp.CPU.PerCore = make([]float64, len(perCore))
	for i, v := range perCore {
		resp.CPU.PerCore[i] = round(v)
	}

	resp.CPU.Info = getCPUInfo()

	// Load Avg
	if avg, err := load.Avg(); err == nil {
		resp.CPU.LoadAvg = []float64{round(avg.Load1), round(avg.Load5), round(avg.Load15)}
	}

	// CPU Stats
	stats := getCPUStats()
	resp.CPU.Stats = map[string]uint64{
		"ctx_switches":    stats["ctx_switches"],
		"interrupts":      stats["interrupts"],
		"soft_interrupts": stats["soft_interrupts"],
		"syscalls":        stats["syscalls"],
	}

	// CPU Times
	if times, err := cpu.Times(false); err == nil && len(times) > 0 {
		t := times[0]
		total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice
		if total <= 0 {
			total = 1
		}
		resp.CPU.Times = map[string]float64{
			"user":    round((t.User / total) * 100),
			"system":  round((t.System / total) * 100),
			"idle":    round((t.Idle / total) * 100),
			"iowait":  round((t.Iowait / total) * 100),
			"irq":     round((t.Irq / total) * 100),
			"softirq": round((t.Softirq / total) * 100),
		}
	}

	// CPU Freq
	if freqs, err := cpu.Info(); err == nil && len(freqs) > 0 {
		// Try to get real-time freq
		var perCoreFreq []float64

		// Manual parsing of /proc/cpuinfo for real-time frequency
		// because gopsutil's Info() returns max freq, and Freq() is missing in this version
		var realFreqs []float64
		if file, err := os.Open("/proc/cpuinfo"); err == nil {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "cpu MHz") {
					parts := strings.Split(line, ":")
					if len(parts) > 1 {
						valStr := strings.TrimSpace(parts[1])
						val, err := strconv.ParseFloat(valStr, 64)
						if err == nil {
							realFreqs = append(realFreqs, val)
						}
					}
				}
			}
			file.Close()
		}

		if len(realFreqs) > 0 {
			for _, f := range realFreqs {
				perCoreFreq = append(perCoreFreq, round(f))
			}
		} else {
			// Fallback to Info() Mhz if manual parsing fails
			for _, f := range freqs {
				perCoreFreq = append(perCoreFreq, round(f.Mhz))
			}
		}

		avgFreq := 0.0
		if len(perCoreFreq) > 0 {
			sum := 0.0
			for _, f := range perCoreFreq {
				sum += f
			}
			avgFreq = round(sum / float64(len(perCoreFreq)))
		}

		resp.CPU.Freq = CPUFreq{
			Avg:     avgFreq,
			PerCore: perCoreFreq,
		}
	}

	// Sensors
	resp.Sensors = getSensors()

	// Power
	resp.Power = getPower()

	// Update history
	historyMutex.Lock()
	// Mock temp for now
	currentTemp := 0.0
	if sensors, ok := resp.Sensors.(map[string][]interface{}); ok {
		count := 0.0
		sum := 0.0
		for _, list := range sensors {
			for _, item := range list {
				if m, ok := item.(map[string]interface{}); ok {
					if t, ok := m["current"].(float64); ok && t > 0 {
						sum += t
						count++
					}
				}
			}
		}
		if count > 0 {
			currentTemp = round(sum / count)
		}
	}

	if len(cpuTempHistory) >= 300 {
		copy(cpuTempHistory, cpuTempHistory[1:])
		cpuTempHistory = cpuTempHistory[:len(cpuTempHistory)-1]
	}
	cpuTempHistory = append(cpuTempHistory, currentTemp)
	resp.CPU.TempHistory = make([]float64, len(cpuTempHistory))
	copy(resp.CPU.TempHistory, cpuTempHistory)
	historyMutex.Unlock()

	// Memory
	v, _ := mem.VirtualMemory()
	resp.Memory = MemInfo{
		Total:     getSize(v.Total),
		Used:      getSize(v.Used),
		Free:      getSize(v.Free),
		Percent:   round(v.UsedPercent),
		Available: getSize(v.Available),
		Buffers:   getSize(v.Buffers),
		Cached:    getSize(v.Cached),
		Shared:    getSize(v.Shared),
		Active:    getSize(v.Active),
		Inactive:  getSize(v.Inactive),
		Slab:      getSize(v.Slab),
	}

	historyMutex.Lock()
	if len(memHistory) >= 300 {
		copy(memHistory, memHistory[1:])
		memHistory = memHistory[:len(memHistory)-1]
	}
	memHistory = append(memHistory, v.UsedPercent)
	resp.Memory.History = make([]float64, len(memHistory))
	copy(resp.Memory.History, memHistory)
	historyMutex.Unlock()

	// Swap
	s, _ := mem.SwapMemory()
	resp.Swap = SwapInfo{
		Total:   getSize(s.Total),
		Used:    getSize(s.Used),
		Free:    getSize(s.Free),
		Percent: round(s.UsedPercent),
		Sin:     getSize(s.Sin),
		Sout:    getSize(s.Sout),
	}

	// Disk
	parts, _ := disk.Partitions(false)
	for _, part := range parts {
		if strings.Contains(part.Device, "loop") || part.Fstype == "squashfs" {
			continue
		}

		mountpoint := part.Mountpoint
		checkPath := mountpoint
		if _, err := os.Stat("/hostfs"); err == nil {
			checkPath = filepath.Join("/hostfs", strings.TrimPrefix(mountpoint, "/"))
		}

		u, err := disk.Usage(checkPath)
		if err == nil {
			resp.Disk = append(resp.Disk, DiskInfo{
				Device:     part.Device,
				Mountpoint: part.Mountpoint,
				Fstype:     part.Fstype,
				Total:      getSize(u.Total),
				Used:       getSize(u.Used),
				Free:       getSize(u.Free),
				Percent:    round(u.UsedPercent),
			})

			// Inodes
			resp.Inodes = append(resp.Inodes, InodeInfo{
				Mountpoint: part.Mountpoint,
				Total:      u.InodesTotal,
				Used:       u.InodesUsed,
				Free:       u.InodesFree,
				Percent:    round(u.InodesUsedPercent),
			})
		}
	}

	// Disk IO
	resp.DiskIO = make(map[string]DiskIOInfo)
	if ioCounters, err := disk.IOCounters(); err == nil {
		for name, io := range ioCounters {
			resp.DiskIO[name] = DiskIOInfo{
				ReadBytes:  getSize(io.ReadBytes),
				WriteBytes: getSize(io.WriteBytes),
				ReadCount:  io.ReadCount,
				WriteCount: io.WriteCount,
				ReadTime:   io.ReadTime,
				WriteTime:  io.WriteTime,
			}
		}
	}

	// Network
	netIO, _ := net.IOCounters(false)
	if len(netIO) > 0 {
		resp.Network.BytesSent = getSize(netIO[0].BytesSent)
		resp.Network.BytesRecv = getSize(netIO[0].BytesRecv)
		resp.Network.RawSent = netIO[0].BytesSent
		resp.Network.RawRecv = netIO[0].BytesRecv
		resp.Network.Errors = map[string]uint64{
			"total_errors_in":  netIO[0].Errin,
			"total_errors_out": netIO[0].Errout,
			"total_drops_in":   netIO[0].Dropin,
			"total_drops_out":  netIO[0].Dropout,
		}
	}

	// Interfaces
	resp.Network.Interfaces = make(map[string]Interface)
	netIfs, _ := net.Interfaces()
	netIOPerNic, _ := net.IOCounters(true)
	ioMap := make(map[string]net.IOCountersStat)
	for _, io := range netIOPerNic {
		ioMap[io.Name] = io
	}

	for _, nic := range netIfs {
		var ip string
		for _, addr := range nic.Addrs {
			if strings.Contains(addr.Addr, ".") { // IPv4
				ip = addr.Addr
				break
			}
		}

		io := ioMap[nic.Name]
		resp.Network.Interfaces[nic.Name] = Interface{
			IP:        ip,
			BytesSent: getSize(io.BytesSent),
			BytesRecv: getSize(io.BytesRecv),
			IsUp:      strings.Contains(strings.Join(nic.Flags, ","), "up"),
			ErrorsIn:  io.Errin,
			ErrorsOut: io.Errout,
			DropsIn:   io.Dropin,
			DropsOut:  io.Dropout,
		}
	}

	// Network Extras
	resp.Network.ConnectionStates = getConnectionStates()
	resp.Network.ListeningPorts = getListeningPorts()

	// Socket Stats (from /proc/net/sockstat)
	resp.Network.Sockets = map[string]int{"tcp": 0, "udp": 0, "tcp_tw": 0}
	if file, err := os.Open("/proc/net/sockstat"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.Fields(line)
			if strings.HasPrefix(line, "TCP:") {
				for i, p := range parts {
					if p == "inuse" && i+1 < len(parts) {
						v, _ := strconv.Atoi(parts[i+1])
						resp.Network.Sockets["tcp"] = v
					} else if p == "tw" && i+1 < len(parts) {
						v, _ := strconv.Atoi(parts[i+1])
						resp.Network.Sockets["tcp_tw"] = v
					}
				}
			} else if strings.HasPrefix(line, "UDP:") {
				for i, p := range parts {
					if p == "inuse" && i+1 < len(parts) {
						v, _ := strconv.Atoi(parts[i+1])
						resp.Network.Sockets["udp"] = v
					}
				}
			}
		}
	}

	// Processes
	resp.Processes = getTopProcesses()

	// GPU
	resp.GPU = getGPUDetails()

	// Boot Time
	bootTime, _ := host.BootTime()
	bt := time.Unix(int64(bootTime), 0)
	resp.BootTime = bt.Format("2006/01/02 15:04:05")

	// SSH Stats
	resp.SSHStats = getSSHStats()

	return resp
}

// --- Main ---

func infoHandler(w http.ResponseWriter, r *http.Request) {
	bootTime, _ := host.BootTime()
	uptimeSeconds := uint64(time.Now().Unix()) - bootTime

	// Pretty uptime
	uptimeString := formatUptime(uptimeSeconds)

	v, _ := mem.VirtualMemory()
	memStr := fmt.Sprintf("%s / %s (%.1f%%)", getSize(v.Used), getSize(v.Total), v.UsedPercent)

	s, _ := mem.SwapMemory()
	swapStr := fmt.Sprintf("%s / %s (%.1f%%)", getSize(s.Used), getSize(s.Total), s.UsedPercent)

	diskStr := "Unknown"
	if parts, err := disk.Partitions(false); err == nil && len(parts) > 0 {
		// Use root partition or first one
		for _, p := range parts {
			if p.Mountpoint == "/" || p.Mountpoint == "/hostfs" {
				if u, err := disk.Usage(p.Mountpoint); err == nil {
					diskStr = fmt.Sprintf("%s / %s (%.1f%%)", getSize(u.Used), getSize(u.Total), u.UsedPercent)
					break
				}
			}
		}
	}

	hostInfo, _ := host.Info()
	cpuInfo := getCPUInfo()

	// Get IP (outbound)
	ip := "Unknown"
	if conn, err := stdnet.Dial("udp", "8.8.8.8:80"); err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*stdnet.UDPAddr)
		ip = localAddr.IP.String()
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	locale := os.Getenv("LANG")
	if locale == "" {
		locale = "C"
	}
	osPretty := detectOSName()
	if osPretty == "" {
		osPretty = hostInfo.Platform
		if hostInfo.PlatformVersion != "" {
			osPretty = fmt.Sprintf("%s %s", osPretty, hostInfo.PlatformVersion)
		}
		if hostInfo.KernelVersion != "" {
			osPretty = fmt.Sprintf("%s (%s)", osPretty, hostInfo.KernelVersion)
		}
	} else {
		// Append kernel version to detected OS name
		if hostInfo.KernelVersion != "" {
			osPretty = fmt.Sprintf("%s (%s)", osPretty, hostInfo.KernelVersion)
		}
	}

	response := map[string]interface{}{
		"header": fmt.Sprintf("%s@%s", "root", hostInfo.Hostname),
		"os":     osPretty,
		"kernel": hostInfo.KernelVersion,
		"uptime": uptimeString,
		"shell":  shell,
		"cpu":    fmt.Sprintf("%s (%d) @ %.2f GHz", cpuInfo.Model, cpuInfo.Cores, cpuInfo.MaxFreq/1000),
		"gpu":    getGPUInfo(),
		"memory": memStr,
		"swap":   swapStr,
		"disk":   diskStr,
		"ip":     ip,
		"locale": locale,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	intervalStr := r.URL.Query().Get("interval")
	interval, err := strconv.ParseFloat(intervalStr, 64)
	if err != nil || interval < 2.0 {
		interval = 2.0
	}
	if interval > 60 {
		interval = 60
	}

	ticker := time.NewTicker(time.Duration(interval * float64(time.Second)))
	defer ticker.Stop()

	for {
		stats := collectStats()
		err := c.WriteJSON(stats)
		if err != nil {
			log.Println("write:", err)
			break
		}
		<-ticker.C
	}
}

// 登录处理
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 速率限制检查
	if !loginLimiter.Allow() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "Too many login attempts. Please try again later."})
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// 验证用户
	user := validateUser(req.Username, req.Password)
	if user == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid credentials"})
		return
	}

	// 生成JWT令牌
	jwtToken, err := generateJWT(user.Username, user.Role)
	if err != nil {
		log.Printf("Failed to generate JWT token for user %s: %v", user.Username, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// 存储会话信息（使用用户名作为键）
	sessions_mu.Lock()
	sessions[user.Username] = SessionInfo{
		Username:  user.Username,
		Role:      user.Role,
		Token:     jwtToken,
		ExpiresAt: time.Now().Add(24 * time.Hour), // 会话24小时后过期
	}
	sessions_mu.Unlock()

	// 更新最后登录时间
	userDB_mu.Lock()
	for i, u := range userDB.Users {
		if u.ID == user.ID {
			now := time.Now()
			userDB.Users[i].LastLogin = &now
			break
		}
	}
	userDB_mu.Unlock()
	saveUserDatabase()

	// 设置安全的HTTP Cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    jwtToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil, // 仅在HTTPS下设置Secure
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400, // 24小时
	})

	// 返回响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Token:    jwtToken,
		Message:  "Login successful",
		Username: user.Username,
		Role:     user.Role,
	})
}

// 登出处理
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.Header.Get("Authorization")
	if token != "" {
		token = strings.TrimPrefix(token, "Bearer ")
		sessions_mu.Lock()
		delete(sessions, token)
		sessions_mu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}

// 认证检查函数
func isAuthenticated(r *http.Request) bool {
	session, err := getSessionInfo(r)
	if err != nil {
		return false
	}
	// 检查Token是否过期
	if session.ExpiresAt.Before(time.Now()) {
		sessions_mu.Lock()
		delete(sessions, session.Token)
		sessions_mu.Unlock()
		return false
	}
	return true
}

// JWT 密钥，实际应用中应从环境变量或配置文件中读取，这里使用固定密钥（仅用于演示）
var jwtKey = []byte("your-secret-key-change-in-production")

// 生成JWT令牌
func generateJWT(username, role string) (string, error) {
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

// 验证JWT令牌并返回声明
func validateJWT(tokenString string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*jwt.RegisteredClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

// 从请求中提取并验证JWT令牌
func extractAndValidateJWT(r *http.Request) (*jwt.RegisteredClaims, error) {
	token := r.Header.Get("Authorization")
	if token == "" {
		// 尝试从 Cookie 读取
		cookie, err := r.Cookie("auth_token")
		if err != nil {
			// 尝试从查询参数读取（WebSocket 使用）
			token = r.URL.Query().Get("token")
			if token == "" {
				return nil, fmt.Errorf("no token found")
			}
		} else {
			token = cookie.Value
		}
	} else {
		// 移除 "Bearer " 前缀
		token = strings.TrimPrefix(token, "Bearer ")
	}

	return validateJWT(token)
}

// 获取 Session 信息
func getSessionInfo(r *http.Request) (*SessionInfo, error) {
	claims, err := extractAndValidateJWT(r)
	if err != nil {
		return nil, err
	}

	// 从JWT声明中获取用户名
	username := claims.Subject

	// 从内存中获取会话信息（或从数据库/缓存中获取）
	sessions_mu.RLock()
	session, exists := sessions[username] // 注意：这里我们使用username作为键，而不是token
	sessions_mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	// 检查会话是否过期（JWT已经验证过期，这里双重检查）
	if session.ExpiresAt.Before(time.Now()) {
		sessions_mu.Lock()
		delete(sessions, username)
		sessions_mu.Unlock()
		return nil, fmt.Errorf("session expired")
	}

	return &session, nil
}

// 获取当前用户角色
func getCurrentRole(r *http.Request) string {
	session, err := getSessionInfo(r)
	if err != nil {
		return ""
	}
	return session.Role
} // 安全HTTP头中间件
func securityHeadersMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 基础安全头
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// HSTS - 仅在HTTPS下启用（这里我们根据实际情况设置，如果确定是HTTPS则启用）
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Content Security Policy (CSP) - 限制资源加载
		// 默认只允许同源，内联脚本和样式需要nonce或hash（这里简化，允许内联）
		csp := []string{
			"default-src 'self'",
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com", // 允许内联样式和外部字体样式
			"script-src 'self' 'unsafe-inline'",                                            // 允许内联脚本（简化，生产环境应使用nonce）
			"font-src 'self' data: https://fonts.gstatic.com https://cdnjs.cloudflare.com", // 允许外部字体
			"img-src 'self' data:",
			"connect-src 'self' ws: wss:", // 允许WebSocket连接
			"frame-ancestors 'none'",      // 等同于X-Frame-Options DENY
			"base-uri 'self'",
			"form-action 'self'",
		}
		w.Header().Set("Content-Security-Policy", strings.Join(csp, "; "))

		// 防止缓存敏感信息
		if strings.Contains(r.URL.Path, "/api/") || strings.Contains(r.URL.Path, "/ws/") {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}

		next(w, r)
	}
}

// 认证中间件
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// 登录页面处理
func loginPageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./templates/login.html")
}

// 用户管理 API - 列出所有用户（仅管理员）
func listUsersHandler(w http.ResponseWriter, r *http.Request) {
	if getCurrentRole(r) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userDB_mu.RLock()
	users := make([]map[string]interface{}, len(userDB.Users))
	for i, u := range userDB.Users {
		users[i] = map[string]interface{}{
			"id":         u.ID,
			"username":   u.Username,
			"role":       u.Role,
			"created_at": u.CreatedAt,
			"last_login": u.LastLogin,
		}
	}
	userDB_mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"users": users,
	})
}

// 创建用户（仅管理员）
func createUserHandler(w http.ResponseWriter, r *http.Request) {
	if getCurrentRole(r) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// 验证输入
	if req.Username == "" || req.Password == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Username and password are required"})
		return
	}

	if req.Role != "admin" && req.Role != "user" {
		req.Role = "user"
	}

	// 生成密码哈希
	hash, err := hashPasswordBcrypt(req.Password)
	if err != nil {
		http.Error(w, "Password hashing failed", http.StatusInternalServerError)
		return
	}

	// 检查用户是否存在
	userDB_mu.Lock()
	for _, u := range userDB.Users {
		if u.Username == req.Username {
			userDB_mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "User already exists"})
			return
		}
	}

	// 添加新用户
	newUser := User{
		ID:        fmt.Sprintf("user_%d", len(userDB.Users)),
		Username:  req.Username,
		Password:  hash,
		Role:      req.Role,
		CreatedAt: time.Now(),
		LastLogin: nil,
	}
	userDB.Users = append(userDB.Users, newUser)
	userDB_mu.Unlock()
	saveUserDatabase()

	// 记录日志
	session, _ := getSessionInfo(r)
	logOperation(session.Username, "create_user", fmt.Sprintf("Created user: %s (%s)", newUser.Username, newUser.Role), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message":  "User created successfully",
		"username": newUser.Username,
	})
}

// 删除用户（仅管理员）
func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	if getCurrentRole(r) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "Username required", http.StatusBadRequest)
		return
	}

	// 防止删除管理员
	if username == "admin" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Cannot delete admin user"})
		return
	}

	userDB_mu.Lock()
	found := false
	for i, u := range userDB.Users {
		if u.Username == username {
			userDB.Users = append(userDB.Users[:i], userDB.Users[i+1:]...)
			found = true
			break
		}
	}
	userDB_mu.Unlock()

	if !found {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "User not found"})
		return
	}

	saveUserDatabase()

	// 记录日志
	session, _ := getSessionInfo(r)
	logOperation(session.Username, "delete_user", fmt.Sprintf("Deleted user: %s", username), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"})
}

// 修改密码
func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
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

	session, err := getSessionInfo(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	targetUsername := session.Username
	// If admin wants to change another user's password
	if req.Username != "" && req.Username != session.Username {
		if session.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		targetUsername = req.Username
	} else {
		// Changing own password requires old password verification
		if session.Role != "admin" && req.OldPassword == "" {
			http.Error(w, "Old password required", http.StatusBadRequest)
			return
		}
	}

	userDB_mu.Lock()
	defer userDB_mu.Unlock()

	var targetUser *User
	for i := range userDB.Users {
		if userDB.Users[i].Username == targetUsername {
			targetUser = &userDB.Users[i]
			break
		}
	}

	if targetUser == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Verify old password if changing own password (and not admin changing others)
	if targetUsername == session.Username {
		if err := bcrypt.CompareHashAndPassword([]byte(targetUser.Password), []byte(req.OldPassword)); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid old password"})
			return
		}
	}

	// Hash new password
	hash, err := hashPasswordBcrypt(req.NewPassword)
	if err != nil {
		http.Error(w, "Password hashing failed", http.StatusInternalServerError)
		return
	}

	targetUser.Password = hash

	// Save DB (we hold the lock, so we need to be careful if saveUserDatabase also locks.
	// Checked: saveUserDatabase does NOT lock. It expects caller to hold lock or be safe.)
	// Wait, I need to verify saveUserDatabase again.
	// In previous turn I saw:
	// func saveUserDatabase() error {
	// 	log.Println("Saving user database...")
	//  ...
	// }
	// It does NOT lock. So it is safe to call here.

	// However, I need to make sure I am not calling it with RLock if it needs Write.
	// I am holding Lock() (Write lock), so it is safe.

	// But wait, saveUserDatabase reads userDB.
	// If I call it, it reads userDB.

	// Let's double check saveUserDatabase implementation I saw earlier.
	// It uses json.MarshalIndent(userDB, ...).
	// This is safe as I hold the lock.

	if err := saveUserDatabase(); err != nil {
		log.Printf("Error saving user DB: %v", err)
	}

	// Log
	logOperation(session.Username, "change_password", fmt.Sprintf("Changed password for: %s", targetUsername), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Password changed successfully"})
}

// 获取操作日志（仅管理员）
func listLogsHandler(w http.ResponseWriter, r *http.Request) {
	if getCurrentRole(r) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	opLogs_mu.RLock()
	defer opLogs_mu.RUnlock()

	// Return logs in reverse order (newest first)
	count := len(opLogs)
	logs := make([]OperationLog, count)
	for i, log := range opLogs {
		logs[count-1-i] = log
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs": logs,
	})
}

// --- Docker Integration ---

type DockerContainer struct {
	Id     string   `json:"Id"`
	Names  []string `json:"Names"`
	Image  string   `json:"Image"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
	Ports  []struct {
		PrivatePort int    `json:"PrivatePort"`
		PublicPort  int    `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
}

type DockerImage struct {
	Id       string   `json:"Id"`
	RepoTags []string `json:"RepoTags"`
	Size     int64    `json:"Size"`
	Created  int64    `json:"Created"`
}

func dockerRequest(method, path string) (*http.Response, error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (stdnet.Conn, error) {
				return stdnet.Dial("unix", "/var/run/docker.sock")
			},
		},
	}
	req, err := http.NewRequest(method, "http://docker"+path, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func listContainersHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := dockerRequest("GET", "/containers/json?all=1")
	if err != nil {
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		http.Error(w, "Docker API error", resp.StatusCode)
		return
	}

	var containers []DockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		http.Error(w, "Failed to decode Docker response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"containers": containers})
}

func listImagesHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := dockerRequest("GET", "/images/json")
	if err != nil {
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		http.Error(w, "Docker API error", resp.StatusCode)
		return
	}

	var images []DockerImage
	if err := json.NewDecoder(resp.Body).Decode(&images); err != nil {
		http.Error(w, "Failed to decode Docker response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"images": images})
}

func dockerActionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check for admin role
	session, err := getSessionInfo(r)
	if err != nil || session.Role != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req struct {
		Id     string `json:"id"`
		Action string `json:"action"` // start, stop, restart, remove
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	var path string
	method := "POST"
	switch req.Action {
	case "start":
		path = fmt.Sprintf("/containers/%s/start", req.Id)
	case "stop":
		path = fmt.Sprintf("/containers/%s/stop", req.Id)
	case "restart":
		path = fmt.Sprintf("/containers/%s/restart", req.Id)
	case "remove":
		path = fmt.Sprintf("/containers/%s", req.Id)
		method = "DELETE"
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	resp, err := dockerRequest(method, path)
	if err != nil {
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := ioutil.ReadAll(resp.Body)
		http.Error(w, fmt.Sprintf("Docker error: %s", string(body)), resp.StatusCode)
		return
	}

	// Log the action
	logOperation(session.Username, "docker_action", fmt.Sprintf("%s container %s", req.Action, req.Id), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Action successful"})
}

// --- Systemd Service Management ---

type ServiceInfo struct {
	Unit        string `json:"unit"`
	Load        string `json:"load"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
}

func listServices() ([]ServiceInfo, error) {
	// Use chroot to execute systemctl on the host
	cmd := exec.Command("chroot", "/hostfs", "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var services []ServiceInfo
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			svc := ServiceInfo{
				Unit:        parts[0],
				Load:        parts[1],
				Active:      parts[2],
				Sub:         parts[3],
				Description: strings.Join(parts[4:], " "),
			}
			services = append(services, svc)
		}
	}
	return services, nil
}

func listServicesHandler(w http.ResponseWriter, r *http.Request) {
	services, err := listServices()
	if err != nil {
		http.Error(w, "Failed to list services: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func serviceActionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check Admin Role
	sess, err := getSessionInfo(r)
	if err != nil || sess.Role != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	var req struct {
		Unit   string `json:"unit"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	allowedActions := map[string]bool{
		"start":   true,
		"stop":    true,
		"restart": true,
		"enable":  true,
		"disable": true,
	}
	if !allowedActions[req.Action] {
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	// Execute action via chroot
	cmd := exec.Command("chroot", "/hostfs", "systemctl", req.Action, req.Unit)
	out, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, fmt.Sprintf("Action failed: %s, Output: %s", err.Error(), string(out)), http.StatusInternalServerError)
		return
	}

	logOperation(sess.Username, "systemd_action", fmt.Sprintf("%s %s", req.Action, req.Unit), r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// --- Cron Job Management ---

type CronJob struct {
	ID       string `json:"id"` // Just an index for frontend
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
}

func listCronHandler(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("chroot", "/hostfs", "crontab", "-l")
	output, err := cmd.Output()

	// If no crontab exists, it returns exit code 1, which is fine, just return empty list
	if err != nil {
		// Check if it's just "no crontab for root"
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]CronJob{})
			return
		}
		// Real error
		http.Error(w, "Failed to list cron jobs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var jobs []CronJob
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	id := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Simple parsing: first 5 fields are schedule, rest is command
		parts := strings.Fields(line)
		if len(parts) >= 6 {
			schedule := strings.Join(parts[:5], " ")
			command := strings.Join(parts[5:], " ")
			jobs = append(jobs, CronJob{
				ID:       strconv.Itoa(id),
				Schedule: schedule,
				Command:  command,
			})
			id++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func saveCronHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check Admin Role
	sess, err := getSessionInfo(r)
	if err != nil || sess.Role != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	var jobs []CronJob
	if err := json.NewDecoder(r.Body).Decode(&jobs); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Reconstruct crontab file content
	var sb strings.Builder
	sb.WriteString("# Generated by Web Monitor\n")
	for _, job := range jobs {
		sb.WriteString(fmt.Sprintf("%s %s\n", job.Schedule, job.Command))
	}

	// Write to crontab via stdin
	cmd := exec.Command("chroot", "/hostfs", "crontab", "-")
	cmd.Stdin = strings.NewReader(sb.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save crontab: %s, Output: %s", err.Error(), string(output)), http.StatusInternalServerError)
		return
	}

	logOperation(sess.Username, "cron_update", "Updated crontab", r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// 验证网络目标（IPv4、IPv6、域名）
func validateNetworkTarget(target string) bool {
	if target == "" {
		return false
	}

	// IPv4 地址 (1.2.3.4)
	ipv4Regex := `^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`
	// IPv6 地址 (简化版)
	ipv6Regex := `^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$|^::([0-9a-fA-F]{1,4}:){0,6}[0-9a-fA-F]{1,4}$|^([0-9a-fA-F]{1,4}:){1,7}:$`
	// 域名 (包括子域名)
	domainRegex := `^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`
	// 简单的主机名 (localhost)
	hostnameRegex := `^[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]$`

	// 检查是否匹配任一格式
	if matched, _ := regexp.MatchString(ipv4Regex, target); matched {
		return true
	}
	if matched, _ := regexp.MatchString(ipv6Regex, target); matched {
		return true
	}
	if matched, _ := regexp.MatchString(domainRegex, target); matched {
		return true
	}
	if matched, _ := regexp.MatchString(hostnameRegex, target); matched {
		return true
	}

	// 检查是否是有效的IPv6简化格式
	if strings.Contains(target, ":") {
		// 尝试解析为IPv6
		if ip := stdnet.ParseIP(target); ip != nil {
			return true
		}
	}

	return false
}

// --- Network Diagnostics ---

func networkTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check Admin Role
	sess, err := getSessionInfo(r)
	if err != nil || sess.Role != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	var req struct {
		Tool   string `json:"tool"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 验证工具类型
	validTools := map[string]bool{
		"ping":  true,
		"trace": true,
		"dig":   true,
		"curl":  true,
	}
	if !validTools[req.Tool] {
		http.Error(w, "Invalid tool", http.StatusBadRequest)
		return
	}

	// 使用正则表达式验证目标，防止命令注入
	if !validateNetworkTarget(req.Target) {
		http.Error(w, "Invalid target format. Must be IPv4, IPv6, or valid domain name", http.StatusBadRequest)
		return
	}

	// 额外安全检查：限制目标长度
	if len(req.Target) > 253 {
		http.Error(w, "Target too long", http.StatusBadRequest)
		return
	}

	// 防止内部网络探测（可选，根据需求调整）
	privateIPs := []string{"10.", "172.16.", "172.17.", "172.18.", "172.19.", "172.20.", "172.21.", "172.22.", "172.23.", "172.24.", "172.25.", "172.26.", "172.27.", "172.28.", "172.29.", "172.30.", "172.31.", "192.168."}
	if strings.Contains(req.Target, ".") {
		for _, prefix := range privateIPs {
			if strings.HasPrefix(req.Target, prefix) {
				http.Error(w, "Private network addresses are not allowed", http.StatusForbidden)
				return
			}
		}
	}

	var cmd *exec.Cmd
	switch req.Tool {
	case "ping":
		// 使用安全参数：限制包数量，设置超时
		cmd = exec.Command("chroot", "/hostfs", "ping", "-c", "4", "-W", "2", req.Target)
	case "trace":
		// 使用tracepath（无特权），限制跳数
		cmd = exec.Command("chroot", "/hostfs", "tracepath", "-m", "15", req.Target)
	case "dig":
		// 使用dig，限制查询类型为A/AAAA
		cmd = exec.Command("chroot", "/hostfs", "dig", "+short", "+time=3", "+tries=2", req.Target)
	case "curl":
		// 使用curl，仅获取头部，限制时间
		cmd = exec.Command("chroot", "/hostfs", "curl", "-I", "-m", "5", "--max-filesize", "10240", req.Target)
	default:
		http.Error(w, "Invalid tool", http.StatusBadRequest)
		return
	}

	// 设置执行环境，限制资源
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
	output, err := cmd.CombinedOutput()
	result := string(output)
	if err != nil {
		result += fmt.Sprintf("\nError: %s", err.Error())
	}

	// 记录操作日志
	logOperation(sess.Username, "network_test", fmt.Sprintf("%s %s", req.Tool, req.Target), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"output": result})
}

func powerProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		info := getPowerProfile()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
		return
	} else if r.Method == "POST" {
		// Check Admin
		sess, err := getSessionInfo(r)
		if err != nil || sess.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var req struct {
			Profile string `json:"profile"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if err := setPowerProfile(req.Profile); err != nil {
			http.Error(w, "Failed to set profile: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Log
		logOperation(sess.Username, "power_profile", "Set power profile to "+req.Profile, r.RemoteAddr)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func initPrometheus() {
	prometheus.MustRegister(promCpuUsage)
	prometheus.MustRegister(promMemUsage)
	prometheus.MustRegister(promMemTotal)
	prometheus.MustRegister(promMemUsed)
	prometheus.MustRegister(promDiskUsage)
	prometheus.MustRegister(promNetSent)
	prometheus.MustRegister(promNetRecv)
	prometheus.MustRegister(promTemp)
}

func updatePrometheusMetrics() {
	for {
		var cpuVal, memVal, diskVal float64

		// CPU
		percent, err := cpu.Percent(0, false)
		if err == nil && len(percent) > 0 {
			promCpuUsage.Set(percent[0])
			cpuVal = percent[0]
		}

		// Memory
		v, err := mem.VirtualMemory()
		if err == nil {
			promMemUsage.Set(v.UsedPercent)
			promMemTotal.Set(float64(v.Total))
			promMemUsed.Set(float64(v.Used))
			memVal = v.UsedPercent
		}

		// Disk
		parts, err := disk.Partitions(false)
		if err == nil {
			for _, part := range parts {
				usage, err := disk.Usage(part.Mountpoint)
				if err == nil {
					promDiskUsage.WithLabelValues(part.Mountpoint).Set(usage.UsedPercent)
					if part.Mountpoint == "/" {
						diskVal = usage.UsedPercent
					}
				}
			}
		}

		// Network
		netStats, err := net.IOCounters(false)
		if err == nil && len(netStats) > 0 {
			promNetSent.Set(float64(netStats[0].BytesSent))
			promNetRecv.Set(float64(netStats[0].BytesRecv))
		}

		// Temperature
		temps, err := host.SensorsTemperatures()
		if err == nil {
			for _, t := range temps {
				promTemp.WithLabelValues(t.SensorKey).Set(t.Temperature)
			}
		}

		checkAlerts(cpuVal, memVal, diskVal)

		time.Sleep(5 * time.Second)
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting web-monitor server...")

	// Prometheus Init
	initPrometheus()
	go updatePrometheusMetrics()

	// 初始化用户数据库
	log.Println("Initializing user database...")
	if err := initUserDatabase(); err != nil {
		log.Printf("Warning: Failed to initialize user database: %v\n", err)
	}
	log.Println("User database initialized")

	// 加载操作日志
	loadOpLogs()

	// Load alerts
	loadAlerts()

	// 公开路由（不需要认证）
	http.HandleFunc("/api/login", securityHeadersMiddleware(loginHandler))
	http.HandleFunc("/api/logout", securityHeadersMiddleware(logoutHandler))
	http.HandleFunc("/login", securityHeadersMiddleware(loginPageHandler))
	http.Handle("/metrics", promhttp.Handler())

	// 受保护的路由（需要认证）
	http.HandleFunc("/ws/stats", authMiddleware(wsHandler))
	http.HandleFunc("/api/info", authMiddleware(infoHandler))
	http.HandleFunc("/api/password", authMiddleware(changePasswordHandler))
	http.HandleFunc("/api/logs", authMiddleware(listLogsHandler))

	// Docker API
	http.HandleFunc("/api/docker/containers", authMiddleware(listContainersHandler))
	http.HandleFunc("/api/docker/images", authMiddleware(listImagesHandler))
	http.HandleFunc("/api/docker/action", authMiddleware(dockerActionHandler))

	// Systemd API
	http.HandleFunc("/api/systemd/services", authMiddleware(listServicesHandler))
	http.HandleFunc("/api/systemd/action", authMiddleware(serviceActionHandler))

	// Cron API
	http.HandleFunc("/api/cron", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == "GET" {
			listCronHandler(w, r)
		} else if r.Method == "POST" {
			saveCronHandler(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Network API
	http.HandleFunc("/api/network/test", authMiddleware(networkTestHandler))

	// Alerts API
	http.HandleFunc("/api/alerts", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			alertMutex.RLock()
			config := alertConfig
			alertMutex.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(config)
		} else if r.Method == "POST" {
			// Check Admin
			sess, err := getSessionInfo(r)
			if err != nil || sess.Role != "admin" {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			var newConfig AlertConfig
			if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			alertMutex.Lock()
			alertConfig = newConfig
			alertMutex.Unlock()

			if err := saveAlerts(); err != nil {
				http.Error(w, "Failed to save alerts", http.StatusInternalServerError)
				return
			}

			logOperation(sess.Username, "alerts", "Updated alert configuration", r.RemoteAddr)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Power Profile API
	http.HandleFunc("/api/power/profile", authMiddleware(powerProfileHandler))

	// 用户管理 API（仅管理员）
	http.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == "GET" {
			listUsersHandler(w, r)
		} else if r.Method == "POST" {
			createUserHandler(w, r)
		} else if r.Method == "DELETE" {
			deleteUserHandler(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// 主页和其他静态文件
	// 重定向 / 到 index.html，由前端检查认证
	http.HandleFunc("/", securityHeadersMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// 检查是否已认证，否则重定向到登录页
		if !isAuthenticated(r) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		http.ServeFile(w, r, "./templates/index.html")
	}))

	// 提供其他静态文件（CSS、JS等）
	fs := http.FileServer(http.Dir("./templates"))
	http.Handle("/assets/", fs)
	http.Handle("/css/", fs)
	http.Handle("/js/", fs)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	fmt.Printf("Server starting on port %s...\n", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
