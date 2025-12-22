package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Notifier 通知器
type Notifier struct {
	manager    *Manager
	httpClient *http.Client
}

// NewNotifier 创建通知器
func NewNotifier(m *Manager) *Notifier {
	return &Notifier{
		manager: m,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Notify 发送通知到所有启用的渠道
func (n *Notifier) Notify(event *AlertEvent) {
	config := n.manager.GetConfig()

	for _, channel := range config.Channels {
		if !channel.Enabled {
			continue
		}

		if err := n.NotifyChannel(event, channel.Type); err != nil {
			log.Printf("[Alerts] Failed to notify via %s: %v", channel.Type, err)
		}
	}

	// 标记已通知
	n.manager.mu.Lock()
	for i := range n.manager.history {
		if n.manager.history[i].ID == event.ID {
			n.manager.history[i].Notified = true
			now := time.Now()
			n.manager.history[i].NotifiedAt = &now
			break
		}
	}
	n.manager.mu.Unlock()
}

// NotifyChannel 发送通知到指定渠道
func (n *Notifier) NotifyChannel(event *AlertEvent, channelType string) error {
	config := n.manager.GetConfig()

	var channel *NotificationChannel
	for _, ch := range config.Channels {
		if ch.Type == channelType {
			channel = &ch
			break
		}
	}

	if channel == nil {
		return fmt.Errorf("channel not configured: %s", channelType)
	}

	switch channelType {
	case "webhook":
		return n.sendWebhook(event, channel.Config)
	case "dashboard":
		// Dashboard 通知通过 WebSocket 推送，这里只做标记
		return n.sendDashboard(event)
	case "email":
		return n.sendEmail(event, channel.Config)
	default:
		return fmt.Errorf("unknown channel type: %s", channelType)
	}
}

// sendWebhook 发送 Webhook 通知
func (n *Notifier) sendWebhook(event *AlertEvent, config map[string]string) error {
	url := config["url"]
	if url == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	// 构建 payload（兼容 Slack/Discord 格式）
	payload := map[string]interface{}{
		"text": formatWebhookMessage(event),
		// 额外的结构化数据
		"alert": map[string]interface{}{
			"id":        event.ID,
			"rule_id":   event.RuleID,
			"rule_name": event.RuleName,
			"status":    event.Status,
			"severity":  event.Severity,
			"metric":    event.Metric,
			"value":     event.Value,
			"threshold": event.Threshold,
			"fired_at":  event.FiredAt.Format(time.RFC3339),
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := n.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.Printf("[Alerts] Webhook sent: %s", event.RuleName)
	return nil
}

// sendDashboard 发送 Dashboard 通知（标记，实际推送由 WebSocket 处理）
func (n *Notifier) sendDashboard(event *AlertEvent) error {
	// Dashboard 通知的实际推送在 websocket 包中处理
	// 这里只做日志记录
	log.Printf("[Alerts] Dashboard notification: %s", event.RuleName)
	return nil
}

// sendEmail 发送邮件通知（预留接口）
func (n *Notifier) sendEmail(event *AlertEvent, config map[string]string) error {
	// TODO: 实现 SMTP 邮件发送
	// 需要配置: smtp_host, smtp_port, smtp_user, smtp_pass, from, to
	to := config["to"]
	if to == "" {
		return fmt.Errorf("email recipient not configured")
	}

	log.Printf("[Alerts] Email notification (not implemented): %s -> %s", event.RuleName, to)
	return fmt.Errorf("email notification not implemented")
}

// formatWebhookMessage 格式化 Webhook 消息
func formatWebhookMessage(event *AlertEvent) string {
	var statusEmoji, statusText string
	if event.Status == StatusFiring {
		statusEmoji = "🔴"
		statusText = "FIRING"
	} else {
		statusEmoji = "✅"
		statusText = "RESOLVED"
	}

	severityEmoji := "⚠️"
	if event.Severity == SeverityCritical {
		severityEmoji = "🚨"
	}

	unit := "%"
	if event.Metric == "load1" || event.Metric == "load5" || event.Metric == "load15" {
		unit = ""
	}

	return fmt.Sprintf(`%s **OpsKernel Alert** %s

**Status:** %s %s
**Severity:** %s %s
**Rule:** %s
**Metric:** %s
**Value:** %.2f%s %s %.2f%s
**Time:** %s`,
		statusEmoji, statusEmoji,
		statusEmoji, statusText,
		severityEmoji, event.Severity,
		event.RuleName,
		event.Metric,
		event.Value, unit, event.Operator, event.Threshold, unit,
		event.FiredAt.Format("2006-01-02 15:04:05"),
	)
}

// WebhookPayload Webhook 请求体（用于自定义模板）
type WebhookPayload struct {
	Text  string                 `json:"text"`
	Alert map[string]interface{} `json:"alert"`
}
