// Package utils 提供项目中使用的通用工具函数
package utils

import (
	"fmt"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Round 四舍五入到两位小数
func Round(val float64) float64 {
	return math.Round(val*100) / 100
}

// GetSize 将字节数格式化为可读的字符串
func GetSize(bytes uint64) string {
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

// FormatUptime 将秒数格式化为可读的uptime字符串
func FormatUptime(sec uint64) string {
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

// ParseBytes 解析字符串为字节数
func ParseBytes(s string) uint64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

// ParseFloat 解析字符串为浮点数
func ParseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// ValidatePasswordPolicy 验证密码策略
func ValidatePasswordPolicy(password string) bool {
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

// ValidateNetworkTarget 验证网络目标（IPv4、IPv6、域名）
func ValidateNetworkTarget(target string) bool {
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
		if ip := net.ParseIP(target); ip != nil {
			return true
		}
	}

	return false
}

// SanitizeInput 清理用户输入，防止注入攻击
func SanitizeInput(input string) string {
	// 移除SQL注入常见字符
	sqlPatterns := []string{
		";", "--", "/*", "*/", "'", "\"", "`",
		"SELECT", "INSERT", "UPDATE", "DELETE", "DROP", "UNION",
		"EXEC", "EXECUTE", "TRUNCATE", "CREATE", "ALTER",
	}

	// 移除XSS攻击常见字符
	xssPatterns := []string{
		"<script", "</script>", "<iframe", "</iframe>",
		"<object", "</object>", "<embed", "</embed>",
		"javascript:", "onload=", "onerror=", "onclick=",
	}

	// 移除命令注入常见字符
	cmdPatterns := []string{
		"|", "&", "&&", "||", ";", "\n", "\r",
		"$(", "`", ">", "<", ">>", "<<",
	}

	// 移除路径遍历常见字符
	pathPatterns := []string{
		"../", "..\\", "/..", "\\..",
		"/etc/", "/bin/", "/usr/", "/var/",
		"C:\\", "D:\\", "E:\\",
	}

	result := input

	// 应用所有清理规则
	allPatterns := append(sqlPatterns, xssPatterns...)
	allPatterns = append(allPatterns, cmdPatterns...)
	allPatterns = append(allPatterns, pathPatterns...)

	for _, pattern := range allPatterns {
		result = strings.ReplaceAll(result, pattern, "")
	}

	// 移除多余的空格
	result = strings.TrimSpace(result)

	return result
}
