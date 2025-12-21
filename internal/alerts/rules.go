package alerts

import (
	"time"
)

// BuiltinRules 预置告警规则
var BuiltinRules = []AlertRule{
	{
		ID:          "cpu_high",
		Name:        "CPU High Usage",
		Description: "Alert when CPU usage exceeds threshold",
		Metric:      "cpu",
		Operator:    OpGreaterThan,
		Threshold:   90,
		Duration:    "1m",
		Severity:    SeverityCritical,
		Enabled:     false,
		Builtin:     true,
	},
	{
		ID:          "cpu_warning",
		Name:        "CPU Warning",
		Description: "CPU usage reached warning threshold",
		Metric:      "cpu",
		Operator:    OpGreaterThan,
		Threshold:   80,
		Duration:    "2m",
		Severity:    SeverityWarning,
		Enabled:     false,
		Builtin:     true,
	},
	{
		ID:          "memory_high",
		Name:        "Memory High Usage",
		Description: "Alert when memory usage exceeds threshold",
		Metric:      "memory",
		Operator:    OpGreaterThan,
		Threshold:   90,
		Duration:    "1m",
		Severity:    SeverityCritical,
		Enabled:     false,
		Builtin:     true,
	},
	{
		ID:          "memory_warning",
		Name:        "Memory Warning",
		Description: "Memory usage reached warning threshold",
		Metric:      "memory",
		Operator:    OpGreaterThan,
		Threshold:   80,
		Duration:    "2m",
		Severity:    SeverityWarning,
		Enabled:     false,
		Builtin:     true,
	},
	{
		ID:          "disk_high",
		Name:        "Disk High Usage",
		Description: "Root partition disk usage exceeds threshold",
		Metric:      "disk",
		Operator:    OpGreaterThan,
		Threshold:   90,
		Duration:    "5m",
		Severity:    SeverityCritical,
		Enabled:     false,
		Builtin:     true,
	},
	{
		ID:          "disk_warning",
		Name:        "Disk Warning",
		Description: "Root partition disk usage reached warning threshold",
		Metric:      "disk",
		Operator:    OpGreaterThan,
		Threshold:   80,
		Duration:    "5m",
		Severity:    SeverityWarning,
		Enabled:     false,
		Builtin:     true,
	},
	{
		ID:          "load_high",
		Name:        "System Load High",
		Description: "1-minute load average exceeds 2x CPU cores",
		Metric:      "load1",
		Operator:    OpGreaterThan,
		Threshold:   2.0,
		Duration:    "3m",
		Severity:    SeverityCritical,
		Enabled:     false,
		Builtin:     true,
	},
	{
		ID:          "swap_high",
		Name:        "Swap High Usage",
		Description: "Swap usage exceeds threshold, possible memory pressure",
		Metric:      "swap",
		Operator:    OpGreaterThan,
		Threshold:   50,
		Duration:    "5m",
		Severity:    SeverityWarning,
		Enabled:     false,
		Builtin:     true,
	},
}

// RulePreset 规则预设组
type RulePreset struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	RuleIDs     []string `json:"rule_ids"`
}

// BuiltinPresets 预置规则组
var BuiltinPresets = []RulePreset{
	{
		ID:          "essential",
		Name:        "Essential",
		Description: "Basic alert rules for CPU, memory, disk (critical only)",
		RuleIDs:     []string{"cpu_high", "memory_high", "disk_high"},
	},
	{
		ID:          "standard",
		Name:        "Standard",
		Description: "Complete monitoring with warning and critical levels",
		RuleIDs:     []string{"cpu_high", "cpu_warning", "memory_high", "memory_warning", "disk_high", "disk_warning"},
	},
	{
		ID:          "comprehensive",
		Name:        "Comprehensive",
		Description: "All builtin alert rules",
		RuleIDs:     []string{"cpu_high", "cpu_warning", "memory_high", "memory_warning", "disk_high", "disk_warning", "load_high", "swap_high"},
	},
}

// GetBuiltinRule 获取内置规则（复制一份）
func GetBuiltinRule(id string) *AlertRule {
	for _, rule := range BuiltinRules {
		if rule.ID == id {
			// 返回副本，避免修改原始规则
			ruleCopy := rule
			ruleCopy.CreatedAt = time.Now()
			ruleCopy.UpdatedAt = time.Now()
			return &ruleCopy
		}
	}
	return nil
}

// GetPreset 获取预设组
func GetPreset(id string) *RulePreset {
	for _, preset := range BuiltinPresets {
		if preset.ID == id {
			return &preset
		}
	}
	return nil
}

// ValidateRule 验证规则配置
func ValidateRule(rule *AlertRule) error {
	if rule.ID == "" {
		return ErrInvalidRuleID
	}
	if rule.Name == "" {
		return ErrInvalidRuleName
	}
	if !isValidMetric(rule.Metric) {
		return ErrInvalidMetric
	}
	if !isValidOperator(rule.Operator) {
		return ErrInvalidOperator
	}
	if rule.Threshold < 0 {
		return ErrInvalidThreshold
	}
	if rule.Duration != "" {
		if _, err := time.ParseDuration(rule.Duration); err != nil {
			return ErrInvalidDuration
		}
	}
	return nil
}

// SupportedMetrics 支持的指标列表
var SupportedMetrics = []string{
	"cpu",    // CPU usage (%)
	"memory", // Memory usage (%)
	"disk",   // Disk usage (%)
	"swap",   // Swap usage (%)
	"load1",  // 1-minute load (relative to cores)
	"load5",  // 5-minute load
	"load15", // 15-minute load
	// Future support:
	// "network_rx", // Network receive rate (bytes/s)
	// "network_tx", // Network send rate (bytes/s)
	// "disk_read",  // Disk read rate (bytes/s)
	// "disk_write", // Disk write rate (bytes/s)
}

func isValidMetric(metric string) bool {
	for _, m := range SupportedMetrics {
		if m == metric {
			return true
		}
	}
	return false
}

func isValidOperator(op Operator) bool {
	switch op {
	case OpGreaterThan, OpLessThan, OpEqual, OpNotEqual, OpGTE, OpLTE:
		return true
	}
	return false
}
