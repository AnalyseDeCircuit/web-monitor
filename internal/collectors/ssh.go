package collectors

import (
	"context"

	"github.com/AnalyseDeCircuit/web-monitor/internal/network"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

// SSHCollector 采集 SSH 连接信息
type SSHCollector struct{}

// NewSSHCollector 创建 SSH 采集器
func NewSSHCollector() *SSHCollector {
	return &SSHCollector{}
}

func (c *SSHCollector) Name() string {
	return "ssh"
}

func (c *SSHCollector) Collect(ctx context.Context) interface{} {
	stats := network.GetSSHStats()

	// Ensure non-nil fields
	if stats.AuthMethods == nil {
		stats.AuthMethods = map[string]int{}
	}
	if stats.Sessions == nil {
		stats.Sessions = []types.SSHSession{}
	}
	if stats.OOMRiskProcesses == nil {
		stats.OOMRiskProcesses = []types.ProcessInfo{}
	}

	return stats
}
