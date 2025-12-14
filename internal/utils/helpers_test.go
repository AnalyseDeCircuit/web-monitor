package utils

import (
	"testing"
)

func TestRound(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"整数", 10.0, 10.0},
		{"一位小数", 10.5, 10.5},
		{"两位小数", 10.55, 10.55},
		{"三位小数四舍五入", 10.555, 10.56},
		{"负数", -10.555, -10.56},
		{"零", 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Round(tt.input)
			if result != tt.expected {
				t.Errorf("Round(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetSize(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected string
	}{
		{"0字节", 0, "0 B"},
		{"1KB以下", 512, "512 B"},
		{"1KB", 1024, "1.00 KiB"},
		{"1MB", 1024 * 1024, "1.00 MiB"},
		{"1GB", 1024 * 1024 * 1024, "1.00 GiB"},
		{"1.5GB", 1024 * 1024 * 1024 * 3 / 2, "1.50 GiB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetSize(tt.input)
			if result != tt.expected {
				t.Errorf("GetSize(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected string
	}{
		{"0秒", 0, "0s"},
		{"30秒", 30, "30s"},
		{"1分钟", 60, "1m 0s"},
		{"1小时", 3600, "1h 0m 0s"},
		{"1天", 86400, "1d 0h 0m 0s"},
		{"1天1小时1分1秒", 86400 + 3600 + 60 + 1, "1d 1h 1m 1s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatUptime(tt.input)
			if result != tt.expected {
				t.Errorf("FormatUptime(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidatePasswordPolicy(t *testing.T) {
	tests := []struct {
		name     string
		password string
		expected bool
	}{
		{"太短", "Ab1!", false},
		{"只有小写字母", "abcdefgh", false},
		{"只有大写字母", "ABCDEFGH", false},
		{"只有数字", "12345678", false},
		{"只有特殊字符", "!@#$%^&*", false},
		{"大小写字母", "Abcdefgh", false},    // 只有2种类型
		{"字母+数字", "Abcdefg1", true},     // 大小写字母+数字=3种
		{"字母+特殊字符", "Abcdefg!", true},   // 大小写字母+特殊字符=3种
		{"数字+特殊字符", "1234567!", false},  // 只有2种类型
		{"符合要求1", "Abc123!@", true},     // 大小写字母+数字+特殊字符=4种
		{"符合要求2", "Password123!", true}, // 大小写字母+数字+特殊字符=4种
		{"符合要求3", "Test@2023", true},    // 大小写字母+数字+特殊字符=4种
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidatePasswordPolicy(tt.password)
			if result != tt.expected {
				t.Errorf("ValidatePasswordPolicy(%q) = %v, expected %v", tt.password, result, tt.expected)
			}
		})
	}
}

func TestValidateNetworkTarget(t *testing.T) {
	tests := []struct {
		name   string
		target string
		valid  bool
	}{
		{"空字符串", "", false},
		{"有效IPv4", "192.168.1.1", true},
		{"无效IPv4", "256.256.256.256", false},
		{"有效域名", "example.com", true},
		{"带子域名", "www.example.com", true},
		{"本地主机名", "localhost", true},
		{"无效域名", "example..com", false},
		{"IPv6简化", "::1", true},
		{"完整IPv6", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateNetworkTarget(tt.target)
			if result != tt.valid {
				t.Errorf("ValidateNetworkTarget(%q) = %v, expected %v", tt.target, result, tt.valid)
			}
		})
	}
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint64
	}{
		{"空字符串", "", 0},
		{"数字", "1024", 1024},
		{"带空格", " 2048 ", 2048},
		{"无效数字", "abc", 0},
		{"混合", "1024abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseBytes(tt.input)
			if result != tt.expected {
				t.Errorf("ParseBytes(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
	}{
		{"空字符串", "", 0},
		{"整数", "10", 10.0},
		{"小数", "10.5", 10.5},
		{"带空格", " 20.5 ", 20.5},
		{"无效数字", "abc", 0},
		{"科学计数法", "1.23e2", 123.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseFloat(tt.input)
			if result != tt.expected {
				t.Errorf("ParseFloat(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"正常输入", "test123", "test123"},
		{"带空格", "test 123", "test 123"},
		{"SQL注入尝试", "test'; DROP TABLE users; --", "test  TABLE users"},
		{"脚本尝试", "<script>alert('xss')</script>", "alert(xss)"},
		{"命令注入尝试", "test; rm -rf /", "test rm -rf /"},
		{"路径遍历尝试", "../../etc/passwd", "etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeInput(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeInput(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
