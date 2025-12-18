package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config 全局配置结构体
type Config struct {
	// HostFS 是宿主机文件系统的挂载点
	// 在 Docker 中通常是 "/hostfs"
	// 在宿主机直接运行时应为空 ""
	HostFS string

	// HostProc 是宿主机 /proc 目录的路径
	HostProc string

	// HostSys 是宿主机 /sys 目录的路径
	HostSys string

	// HostEtc 是宿主机 /etc 目录的路径
	HostEtc string

	// HostVar 是宿主机 /var 目录的路径
	HostVar string

	// HostRun 是宿主机 /run 目录的路径
	HostRun string

	// Module flags
	EnableCPU     bool
	EnableMemory  bool
	EnableDisk    bool
	EnableNetwork bool
	EnableSensors bool
	EnablePower   bool
	EnableGPU     bool
	EnableSSH     bool
	EnableSystem  bool
	EnableDocker  bool
	EnableCron    bool
	EnableSystemd bool
}

var (
	// GlobalConfig 全局配置实例
	GlobalConfig *Config
	once         sync.Once
)

// Load 加载配置
func Load() *Config {
	once.Do(func() {
		GlobalConfig = &Config{
			HostFS:   getEnv("HOST_FS", "/hostfs"),
			HostProc: getEnv("HOST_PROC", "/hostfs/proc"),
			HostSys:  getEnv("HOST_SYS", "/hostfs/sys"),
			HostEtc:  getEnv("HOST_ETC", "/hostfs/etc"),
			HostVar:  getEnv("HOST_VAR", "/hostfs/var"),
			HostRun:  getEnv("HOST_RUN", "/hostfs/run"),

			EnableCPU:     getEnvBool("ENABLE_CPU", true),
			EnableMemory:  getEnvBool("ENABLE_MEMORY", true),
			EnableDisk:    getEnvBool("ENABLE_DISK", true),
			EnableNetwork: getEnvBool("ENABLE_NETWORK", true),
			EnableSensors: getEnvBool("ENABLE_SENSORS", true),
			EnablePower:   getEnvBool("ENABLE_POWER", true),
			EnableGPU:     getEnvBool("ENABLE_GPU", true),
			EnableSSH:     getEnvBool("ENABLE_SSH", true),
			EnableSystem:  getEnvBool("ENABLE_SYSTEM", true),
			EnableDocker:  getEnvBool("ENABLE_DOCKER", true),
			EnableCron:    getEnvBool("ENABLE_CRON", true),
			EnableSystemd: getEnvBool("ENABLE_SYSTEMD", true),
		}

		// 如果 HOST_FS 为空（Bare Metal 模式），则调整其他路径的默认值
		// 但如果用户显式设置了 HOST_PROC 等，则以用户设置为准
		if GlobalConfig.HostFS == "" {
			if os.Getenv("HOST_PROC") == "" {
				GlobalConfig.HostProc = "/proc"
			}
			if os.Getenv("HOST_SYS") == "" {
				GlobalConfig.HostSys = "/sys"
			}
			if os.Getenv("HOST_ETC") == "" {
				GlobalConfig.HostEtc = "/etc"
			}
			if os.Getenv("HOST_VAR") == "" {
				GlobalConfig.HostVar = "/var"
			}
			if os.Getenv("HOST_RUN") == "" {
				GlobalConfig.HostRun = "/run"
			}
		}
	})
	return GlobalConfig
}

// HostPath 将绝对路径转换为宿主机挂载路径
// 例如: HostPath("/etc/ssh/sshd_config") -> "/hostfs/etc/ssh/sshd_config"
func HostPath(path string) string {
	if GlobalConfig == nil {
		Load()
	}

	// 如果 HostFS 为空，直接返回原始路径
	if GlobalConfig.HostFS == "" {
		return path
	}

	// 如果路径已经包含 HostFS 前缀，直接返回
	if strings.HasPrefix(path, GlobalConfig.HostFS) {
		return path
	}

	// 拼接路径
	return filepath.Join(GlobalConfig.HostFS, path)
}

// getEnv 获取环境变量，如果为空则返回默认值
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvBool 获取布尔类型环境变量
func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		value = strings.ToLower(value)
		return value == "true" || value == "1" || value == "yes" || value == "on"
	}
	return defaultValue
}
