// Package types 定义整个项目中使用的公共类型
package types

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// --- 认证相关类型 ---

// User 用户结构体
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

// UserDatabase 用户数据库
type UserDatabase struct {
	Users []User `json:"users"`
}

// SessionInfo Session 信息
type SessionInfo struct {
	Username  string
	Role      string
	Token     string
	ExpiresAt time.Time
}

// OperationLog 操作日志
type OperationLog struct {
	Time      time.Time `json:"time"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
	IPAddress string    `json:"ip_address"`
}

// LoginRequest 登录请求结构体
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应结构体
type LoginResponse struct {
	Token    string `json:"token"`
	Message  string `json:"message"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// --- 告警配置 ---

// AlertConfig 告警配置
type AlertConfig struct {
	Enabled       bool    `json:"enabled"`
	WebhookURL    string  `json:"webhook_url"`
	CPUThreshold  float64 `json:"cpu_threshold"`  // Percent
	MemThreshold  float64 `json:"mem_threshold"`  // Percent
	DiskThreshold float64 `json:"disk_threshold"` // Percent
}

// --- 监控数据响应类型 ---

// Response 完整的监控数据响应
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

// GPUDetail GPU详细信息
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

// GPUProcess GPU进程信息
type GPUProcess struct {
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	VRAMUsed string `json:"vram_used"`
}

// CPUInfo CPU信息
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

// CPUFreq CPU频率信息
type CPUFreq struct {
	Avg     float64   `json:"avg"`
	PerCore []float64 `json:"per_core"`
}

// CPUDetail CPU详细信息
type CPUDetail struct {
	Model        string  `json:"model"`
	Architecture string  `json:"architecture"`
	Cores        int     `json:"cores"`
	Threads      int     `json:"threads"`
	MaxFreq      float64 `json:"max_freq"`
	MinFreq      float64 `json:"min_freq"`
}

// MemInfo 内存信息
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

// SwapInfo 交换分区信息
type SwapInfo struct {
	Total   string  `json:"total"`
	Used    string  `json:"used"`
	Free    string  `json:"free"`
	Percent float64 `json:"percent"`
	Sin     string  `json:"sin"`
	Sout    string  `json:"sout"`
}

// DiskInfo 磁盘信息
type DiskInfo struct {
	Device     string  `json:"device"`
	Mountpoint string  `json:"mountpoint"`
	Fstype     string  `json:"fstype"`
	Total      string  `json:"total"`
	Used       string  `json:"used"`
	Free       string  `json:"free"`
	Percent    float64 `json:"percent"`
}

// DiskIOInfo 磁盘IO信息
type DiskIOInfo struct {
	ReadBytes  string `json:"read_bytes"`
	WriteBytes string `json:"write_bytes"`
	ReadCount  uint64 `json:"read_count"`
	WriteCount uint64 `json:"write_count"`
	ReadTime   uint64 `json:"read_time"`
	WriteTime  uint64 `json:"write_time"`
}

// InodeInfo Inode信息
type InodeInfo struct {
	Mountpoint string  `json:"mountpoint"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Free       uint64  `json:"free"`
	Percent    float64 `json:"percent"`
}

// NetInfo 网络信息
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

// Interface 网络接口信息
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

// ListeningPort 监听端口信息
type ListeningPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// SSHStats SSH统计信息
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

// ProcessInfo 进程信息
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

// PowerProfileInfo 电源配置信息
type PowerProfileInfo struct {
	Current   string   `json:"current"`
	Available []string `json:"available"`
	Error     string   `json:"error,omitempty"`
}

// --- Docker相关类型 ---

// DockerContainer Docker容器信息
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

// DockerImage Docker镜像信息
type DockerImage struct {
	Id       string   `json:"Id"`
	RepoTags []string `json:"RepoTags"`
	Size     int64    `json:"Size"`
	Created  int64    `json:"Created"`
}

// --- Systemd相关类型 ---

// ServiceInfo Systemd服务信息
type ServiceInfo struct {
	Unit        string `json:"unit"`
	Load        string `json:"load"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
}

// --- Cron相关类型 ---

// CronJob Cron任务信息
type CronJob struct {
	ID       string `json:"id"` // Just an index for frontend
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
}

// --- JWT工具类型 ---

// JWTClaims JWT声明
type JWTClaims struct {
	jwt.RegisteredClaims
	Username string `json:"username"`
	Role     string `json:"role"`
}
