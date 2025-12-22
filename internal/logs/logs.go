// Package logs 提供操作日志功能
package logs

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/pkg/types"
)

var (
	opLogs    []types.OperationLog
	opLogs_mu sync.RWMutex
)

// LogOperation 记录操作日志
func LogOperation(username, action, details, ip string) {
	opLogs_mu.Lock()
	defer opLogs_mu.Unlock()

	logEntry := types.OperationLog{
		Time:      time.Now(),
		Username:  username,
		Action:    action,
		Details:   details,
		IPAddress: ip,
	}

	opLogs = append(opLogs, logEntry)
	// 保持最近 1000 条日志
	if len(opLogs) > 1000 {
		opLogs = opLogs[len(opLogs)-1000:]
	}

	// 异步保存到文件
	go saveOpLogs()
}

// saveOpLogs 保存日志到文件
func saveOpLogs() {
	opLogs_mu.RLock()
	data, err := json.MarshalIndent(opLogs, "", "  ")
	opLogs_mu.RUnlock()

	if err != nil {
		log.Printf("Error marshaling logs: %v", err)
		return
	}

	// 忽略错误，日志保存失败不应影响主流程
	_ = os.WriteFile("/data/operations.json", data, 0666)
}

// LoadOpLogs 加载日志
func LoadOpLogs() {
	data, err := os.ReadFile("/data/operations.json")
	if err != nil {
		return
	}

	opLogs_mu.Lock()
	defer opLogs_mu.Unlock()
	json.Unmarshal(data, &opLogs)
}

// GetLogs 获取操作日志
func GetLogs() []types.OperationLog {
	opLogs_mu.RLock()
	defer opLogs_mu.RUnlock()

	// Return logs in reverse order (newest first)
	count := len(opLogs)
	logs := make([]types.OperationLog, count)
	for i, log := range opLogs {
		logs[count-1-i] = log
	}
	return logs
}

// GetRecentLogs 获取最近的操作日志
func GetRecentLogs(limit int) []types.OperationLog {
	opLogs_mu.RLock()
	defer opLogs_mu.RUnlock()

	if limit <= 0 || limit > len(opLogs) {
		limit = len(opLogs)
	}

	// Return logs in reverse order (newest first)
	logs := make([]types.OperationLog, limit)
	for i := 0; i < limit; i++ {
		logs[i] = opLogs[len(opLogs)-1-i]
	}
	return logs
}
