// Package monitoring 提供系统监控功能
package monitoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

var (
	alertConfig   types.AlertConfig
	alertMutex    sync.RWMutex
	lastAlertTime time.Time
)

func getDataDir() string {
	if v := os.Getenv("DATA_DIR"); v != "" {
		return v
	}
	if _, err := os.Stat("/data"); err == nil {
		return "/data"
	}
	return "./data"
}

// LoadAlerts 加载告警配置
func LoadAlerts() {
	alertMutex.Lock()
	defer alertMutex.Unlock()

	path := filepath.Join(getDataDir(), "alerts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		// Default config
		alertConfig = types.AlertConfig{
			Enabled:       false,
			CPUThreshold:  90,
			MemThreshold:  90,
			DiskThreshold: 90,
		}
		return
	}
	json.Unmarshal(data, &alertConfig)
}

// SaveAlerts 保存告警配置
func SaveAlerts() error {
	alertMutex.RLock()
	data, err := json.MarshalIndent(alertConfig, "", "  ")
	alertMutex.RUnlock()

	if err != nil {
		return err
	}
	path := filepath.Join(getDataDir(), "alerts.json")
	return os.WriteFile(path, data, 0666)
}

// CheckAlerts 检查告警条件
func CheckAlerts(cpuPercent float64, memPercent float64, diskPercent float64) {
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

// sendWebhook 发送Webhook告警
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

// GetAlertConfig 获取告警配置
func GetAlertConfig() types.AlertConfig {
	alertMutex.RLock()
	defer alertMutex.RUnlock()
	return alertConfig
}

// UpdateAlertConfig 更新告警配置
func UpdateAlertConfig(config types.AlertConfig) error {
	alertMutex.Lock()
	alertConfig = config
	alertMutex.Unlock()
	return SaveAlerts()
}
