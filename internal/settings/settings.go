// Package settings 提供系统运行时设置的持久化功能
package settings

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// SystemSettings 系统运行时设置
type SystemSettings struct {
	// MonitoringMode: "on-demand" (按需采集) 或 "always-on" (后台常驻)
	MonitoringMode string `json:"monitoringMode"`
}

var (
	current     SystemSettings
	mu          sync.RWMutex
	listeners   []func(SystemSettings)
	listenersMu sync.Mutex
)

func getSettingsPath() string {
	if v := os.Getenv("DATA_DIR"); v != "" {
		return filepath.Join(v, "settings.json")
	}
	if _, err := os.Stat("/data"); err == nil {
		return "/data/settings.json"
	}
	return "./data/settings.json"
}

// Load 加载设置（启动时调用）
func Load() {
	mu.Lock()
	defer mu.Unlock()

	// 设置默认值
	current = SystemSettings{
		MonitoringMode: "on-demand",
	}

	path := getSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Settings: using defaults (file not found: %s)", path)
		return
	}

	if err := json.Unmarshal(data, &current); err != nil {
		log.Printf("Settings: parse error, using defaults: %v", err)
		return
	}

	// 验证值
	if current.MonitoringMode != "on-demand" && current.MonitoringMode != "always-on" {
		current.MonitoringMode = "on-demand"
	}

	log.Printf("Settings: loaded from %s (monitoringMode=%s)", path, current.MonitoringMode)
}

// Save 保存设置
func Save() error {
	mu.RLock()
	data, err := json.MarshalIndent(current, "", "  ")
	mu.RUnlock()

	if err != nil {
		return err
	}

	path := getSettingsPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	return os.WriteFile(path, data, 0666)
}

// Get 获取当前设置（只读副本）
func Get() SystemSettings {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// SetMonitoringMode 设置采集模式
func SetMonitoringMode(mode string) error {
	if mode != "on-demand" && mode != "always-on" {
		mode = "on-demand"
	}

	mu.Lock()
	oldMode := current.MonitoringMode
	current.MonitoringMode = mode
	mu.Unlock()

	if err := Save(); err != nil {
		// 回滚
		mu.Lock()
		current.MonitoringMode = oldMode
		mu.Unlock()
		return err
	}

	// 通知监听器
	notifyListeners()
	log.Printf("Settings: monitoringMode changed to %s", mode)
	return nil
}

// OnChange 注册设置变更监听器
func OnChange(fn func(SystemSettings)) {
	listenersMu.Lock()
	listeners = append(listeners, fn)
	listenersMu.Unlock()
}

func notifyListeners() {
	listenersMu.Lock()
	fns := make([]func(SystemSettings), len(listeners))
	copy(fns, listeners)
	listenersMu.Unlock()

	s := Get()
	for _, fn := range fns {
		go fn(s)
	}
}

// IsOnDemand 返回是否为按需采集模式
func IsOnDemand() bool {
	mu.RLock()
	defer mu.RUnlock()
	return current.MonitoringMode == "on-demand"
}
