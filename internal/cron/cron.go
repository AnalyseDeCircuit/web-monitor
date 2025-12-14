// Package cron 提供Cron任务管理功能
package cron

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

var (
	cronMutex sync.Mutex
)

// ListCronJobs 列出所有Cron任务
func ListCronJobs() ([]types.CronJob, error) {
	cronMutex.Lock()
	defer cronMutex.Unlock()

	cmd := exec.Command("chroot", "/hostfs", "crontab", "-l")
	output, err := cmd.Output()

	// If no crontab exists, it returns exit code 1, which is fine, just return empty list
	if err != nil {
		// Check if it's just "no crontab for root"
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			return []types.CronJob{}, nil
		}
		// Real error
		return nil, fmt.Errorf("failed to list cron jobs: %v", err)
	}

	var jobs []types.CronJob
	scanner := bufio.NewScanner(bytes.NewReader(output))
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
			jobs = append(jobs, types.CronJob{
				ID:       fmt.Sprintf("%d", id),
				Schedule: schedule,
				Command:  command,
			})
			id++
		}
	}

	return jobs, nil
}

// parseCronLine 解析Cron行
func parseCronLine(line, user string, lineNum int) *types.CronJob {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return nil
	}

	// 前5个字段是时间表达式
	schedule := strings.Join(fields[:5], " ")
	command := strings.Join(fields[5:], " ")

	return &types.CronJob{
		ID:       fmt.Sprintf("%s-%d", user, lineNum),
		Schedule: schedule,
		Command:  command,
	}
}

// AddCronJob 添加Cron任务
func AddCronJob(user, schedule, command string) error {
	cronMutex.Lock()
	defer cronMutex.Unlock()

	// 获取现有crontab
	cmd := exec.Command("crontab", "-u", user, "-l")
	output, err := cmd.Output()
	if err != nil && !strings.Contains(err.Error(), "no crontab for") {
		return fmt.Errorf("failed to get crontab: %v", err)
	}

	// 添加新任务
	newLine := fmt.Sprintf("%s %s\n", schedule, command)
	newCrontab := string(output) + newLine

	// 写入新crontab
	cmd = exec.Command("crontab", "-u", user, "-")
	cmd.Stdin = strings.NewReader(newCrontab)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update crontab: %v\nOutput: %s", err, string(output))
	}

	return nil
}

// RemoveCronJob 删除Cron任务
func RemoveCronJob(user, jobID string) error {
	cronMutex.Lock()
	defer cronMutex.Unlock()

	// 获取现有crontab
	cmd := exec.Command("crontab", "-u", user, "-l")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get crontab: %v", err)
	}

	// 解析并删除指定任务
	var newLines []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	lineNum := 1
	for scanner.Scan() {
		line := scanner.Text()
		currentID := fmt.Sprintf("%s-%d", user, lineNum)

		if currentID != jobID {
			newLines = append(newLines, line)
		}
		lineNum++
	}

	// 写入新crontab
	newCrontab := strings.Join(newLines, "\n")
	if newCrontab != "" {
		newCrontab += "\n"
	}

	cmd = exec.Command("crontab", "-u", user, "-")
	cmd.Stdin = strings.NewReader(newCrontab)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update crontab: %v\nOutput: %s", err, string(output))
	}

	return nil
}

// EnableCronJob 启用Cron任务
func EnableCronJob(user, jobID string) error {
	return toggleCronJob(user, jobID, false)
}

// DisableCronJob 禁用Cron任务
func DisableCronJob(user, jobID string) error {
	return toggleCronJob(user, jobID, true)
}

// toggleCronJob 切换Cron任务状态
func toggleCronJob(user, jobID string, disable bool) error {
	cronMutex.Lock()
	defer cronMutex.Unlock()

	// 获取现有crontab
	cmd := exec.Command("crontab", "-u", user, "-l")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get crontab: %v", err)
	}

	// 解析并修改指定任务
	var newLines []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	lineNum := 1
	for scanner.Scan() {
		line := scanner.Text()
		currentID := fmt.Sprintf("%s-%d", user, lineNum)

		if currentID == jobID {
			if disable {
				// 添加注释禁用
				if !strings.HasPrefix(line, "#") {
					line = "# " + line
				}
			} else {
				// 移除注释启用
				line = strings.TrimPrefix(line, "# ")
			}
		}
		newLines = append(newLines, line)
		lineNum++
	}

	// 写入新crontab
	newCrontab := strings.Join(newLines, "\n")
	if newCrontab != "" {
		newCrontab += "\n"
	}

	cmd = exec.Command("crontab", "-u", user, "-")
	cmd.Stdin = strings.NewReader(newCrontab)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update crontab: %v\nOutput: %s", err, string(output))
	}

	return nil
}

// GetCronLogs 获取Cron日志
func GetCronLogs(lines int) (string, error) {
	cronMutex.Lock()
	defer cronMutex.Unlock()

	cmd := exec.Command("journalctl", "-u", "cron", "-n", fmt.Sprintf("%d", lines), "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get cron logs: %v", err)
	}

	return string(output), nil
}

// ValidateCronSchedule 验证Cron表达式
func ValidateCronSchedule(schedule string) bool {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return false
	}

	// 简单的验证 - 实际应该使用cron解析库
	for _, field := range fields {
		if field == "" {
			return false
		}
		// 检查是否包含有效字符
		validChars := "0123456789*,-/"
		for _, ch := range field {
			if !strings.ContainsRune(validChars, ch) {
				return false
			}
		}
	}

	return true
}
