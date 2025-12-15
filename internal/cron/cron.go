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
// 改进：只返回 Web-Monitor 管理的任务，或者标记哪些是管理的。
// 目前为了兼容性，我们只解析位于管理区块内的任务。
func ListCronJobs() ([]types.CronJob, error) {
	cronMutex.Lock()
	defer cronMutex.Unlock()

	cmd := exec.Command("chroot", "/hostfs", "crontab", "-l")
	output, err := cmd.CombinedOutput()

	// If no crontab exists, it returns exit code 1, which is fine, just return empty list
	if err != nil {
		// Check if it's just "no crontab for root" or "no crontab for user"
		outputStr := string(output)
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			if strings.Contains(outputStr, "no crontab") {
				return []types.CronJob{}, nil
			}
		}
		// Real error
		return nil, fmt.Errorf("failed to list cron jobs: %v, output: %s", err, outputStr)
	}

	var jobs []types.CronJob
	scanner := bufio.NewScanner(bytes.NewReader(output))
	id := 0
	inManagedBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == startMarker {
			inManagedBlock = true
			continue
		}
		if line == endMarker {
			inManagedBlock = false
			continue
		}

		// 如果没有发现任何标记，为了兼容旧版本或手动添加的任务，
		// 我们可以选择解析所有非注释行，或者只解析管理块内的。
		// 为了安全起见，如果存在管理块，只返回管理块内的。
		// 如果不存在管理块（首次运行或旧版升级），则解析所有行。
		// 这里我们需要先扫描一遍看是否有标记。
		// 简单起见：我们解析所有行，但在 Save 时会把它们都放入管理块（如果是首次）。
		// 但这样会接管用户的所有任务。

		// 更好的策略：
		// 1. 如果我们在管理块内，解析它。
		// 2. 如果我们不在管理块内，忽略它（防止误删用户手动添加的重要任务）。
		// 3. 但是，如果整个文件都没有标记，说明是旧版数据或纯净环境，我们应该解析所有内容以便迁移。

		// 让我们先简单实现：只解析管理块内的。如果文件里没有标记，则解析所有非注释行（视为旧版数据）。
		// 这需要两遍扫描或先读入内存。
	}

	// 重新实现 ListCronJobs 逻辑
	lines := strings.Split(string(output), "\n")
	hasMarkers := false
	for _, line := range lines {
		if strings.TrimSpace(line) == startMarker {
			hasMarkers = true
			break
		}
	}

	inManagedBlock = false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if trimmed == startMarker {
			inManagedBlock = true
			continue
		}
		if trimmed == endMarker {
			inManagedBlock = false
			continue
		}

		// 决定是否解析此行
		shouldParse := false
		if hasMarkers {
			shouldParse = inManagedBlock
		} else {
			// 无标记模式：解析所有非注释行（兼容旧版）
			shouldParse = !strings.HasPrefix(trimmed, "#")
		}

		if shouldParse && !strings.HasPrefix(trimmed, "#") {
			parts := strings.Fields(trimmed)
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
	}

	return jobs, nil
}

// SaveCronJobs 安全地保存Cron任务
// 策略：读取现有 crontab，保留所有非 Web-Monitor 管理的行，
// 然后追加/更新 Web-Monitor 管理的任务。
// 我们使用特殊的注释标记来界定 Web-Monitor 管理的区域。
const (
	startMarker = "# --- BEGIN WEB MONITOR MANAGED BLOCK ---"
	endMarker   = "# --- END WEB MONITOR MANAGED BLOCK ---"
)

func SaveCronJobs(jobs []types.CronJob) error {
	cronMutex.Lock()
	defer cronMutex.Unlock()

	// 1. 读取现有 crontab
	cmdRead := exec.Command("chroot", "/hostfs", "crontab", "-l")
	output, err := cmdRead.CombinedOutput()

	var existingLines []string
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(output))
		for scanner.Scan() {
			existingLines = append(existingLines, scanner.Text())
		}
	} else {
		// 如果读取失败且不是因为没有 crontab，则报错
		if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() != 1 {
			return fmt.Errorf("failed to read existing crontab: %v", err)
		}
		// 如果没有 crontab，existingLines 为空，继续
	}

	// 2. 构建新的 crontab 内容
	var newLines []string
	inManagedBlock := false

	// 检查是否存在 marker
	hasMarkers := false
	for _, line := range existingLines {
		if strings.TrimSpace(line) == startMarker {
			hasMarkers = true
			break
		}
	}

	// 保留非管理区域的内容
	for _, line := range existingLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == startMarker {
			inManagedBlock = true
			continue
		}
		if trimmed == endMarker {
			inManagedBlock = false
			continue
		}
		if !inManagedBlock {
			// 如果是首次接管（!hasMarkers），且该行看起来像个任务，则跳过（视为已移动到管理块）
			// 这样可以防止旧任务被保留在外部，导致无法删除
			if !hasMarkers && !strings.HasPrefix(trimmed, "#") && len(strings.Fields(trimmed)) >= 6 {
				continue
			}
			newLines = append(newLines, line)
		}
	}

	// 3. 追加管理区域
	if len(newLines) > 0 && newLines[len(newLines)-1] != "" {
		newLines = append(newLines, "")
	}
	newLines = append(newLines, startMarker)
	newLines = append(newLines, "# Do not edit this block manually. It is managed by Web Monitor.")

	for _, job := range jobs {
		if strings.TrimSpace(job.Schedule) == "" || strings.TrimSpace(job.Command) == "" {
			continue
		}
		newLines = append(newLines, fmt.Sprintf("%s %s", job.Schedule, job.Command))
	}
	newLines = append(newLines, endMarker)
	newLines = append(newLines, "") // Ensure trailing newline

	// 4. 写入新的 crontab
	cmdWrite := exec.Command("chroot", "/hostfs", "crontab", "-")
	cmdWrite.Stdin = strings.NewReader(strings.Join(newLines, "\n"))
	output, err = cmdWrite.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to save crontab: %v, output: %s", err, string(output))
	}

	return nil
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
