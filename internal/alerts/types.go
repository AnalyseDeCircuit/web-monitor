// Package alerts 提供告警管理功能
package alerts

import (
	"time"
)

// Severity 告警严重级别
type Severity string

const (
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// AlertStatus 告警状态
type AlertStatus string

const (
	StatusFiring   AlertStatus = "firing"
	StatusResolved AlertStatus = "resolved"
)

// Operator 比较运算符
type Operator string

const (
	OpGreaterThan Operator = ">"
	OpLessThan    Operator = "<"
	OpEqual       Operator = "=="
	OpNotEqual    Operator = "!="
	OpGTE         Operator = ">="
	OpLTE         Operator = "<="
)

// AlertRule 告警规则
type AlertRule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Metric      string    `json:"metric"`   // cpu, memory, disk, network_rx, network_tx, etc.
	Operator    Operator  `json:"operator"` // >, <, ==, !=, >=, <=
	Threshold   float64   `json:"threshold"`
	Duration    string    `json:"duration"` // 持续时间，如 "1m", "5m"
	Severity    Severity  `json:"severity"` // warning, critical
	Enabled     bool      `json:"enabled"`
	Builtin     bool      `json:"builtin"` // 是否为内置规则
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AlertEvent 告警事件
type AlertEvent struct {
	ID         string      `json:"id"`
	RuleID     string      `json:"rule_id"`
	RuleName   string      `json:"rule_name"`
	Metric     string      `json:"metric"`
	Status     AlertStatus `json:"status"` // firing, resolved
	Severity   Severity    `json:"severity"`
	Value      float64     `json:"value"`
	Threshold  float64     `json:"threshold"`
	Operator   Operator    `json:"operator"`
	Message    string      `json:"message"`
	FiredAt    time.Time   `json:"fired_at"`
	ResolvedAt *time.Time  `json:"resolved_at,omitempty"`
	Notified   bool        `json:"notified"`
	NotifiedAt *time.Time  `json:"notified_at,omitempty"`
}

// NotificationChannel 通知渠道配置
type NotificationChannel struct {
	Type    string            `json:"type"` // webhook, email, dashboard
	Enabled bool              `json:"enabled"`
	Config  map[string]string `json:"config,omitempty"` // type-specific config
}

// AlertConfig 全局告警配置
type AlertConfig struct {
	Enabled             bool                  `json:"enabled"`
	NotifyOnResolved    bool                  `json:"notify_on_resolved"`    // 恢复时是否通知
	GlobalSilencePeriod string                `json:"global_silence_period"` // 全局静默期，如 "5m"
	Channels            []NotificationChannel `json:"channels"`
}

// RuleState 规则运行时状态（内存中）
type RuleState struct {
	RuleID         string
	FirstTriggered *time.Time // 首次触发时间
	LastValue      float64    // 最后检测值
	IsFiring       bool       // 当前是否触发中
	FiringEventID  string     // 关联的事件ID
}

// MetricValue 指标值（用于检查）
type MetricValue struct {
	Name      string
	Value     float64
	Timestamp time.Time
}

// AlertSummary 告警摘要（用于 API 响应）
type AlertSummary struct {
	TotalRules   int `json:"total_rules"`
	EnabledRules int `json:"enabled_rules"`
	FiringAlerts int `json:"firing_alerts"`
	TodayEvents  int `json:"today_events"`
}

// ActiveAlert 活跃告警（用于 API 响应）
type ActiveAlert struct {
	AlertEvent
	Duration string `json:"duration"` // 已持续时间
}

// AlertHistoryQuery 历史查询参数
type AlertHistoryQuery struct {
	RuleID   string      `json:"rule_id,omitempty"`
	Status   AlertStatus `json:"status,omitempty"`
	Severity Severity    `json:"severity,omitempty"`
	Since    *time.Time  `json:"since,omitempty"`
	Until    *time.Time  `json:"until,omitempty"`
	Limit    int         `json:"limit,omitempty"`
	Offset   int         `json:"offset,omitempty"`
}

// PaginatedHistory 分页历史响应
type PaginatedHistory struct {
	Events     []AlertEvent `json:"events"`
	Total      int          `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"page_size"`
	TotalPages int          `json:"total_pages"`
}
