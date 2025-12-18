package collectors

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/host"
)

// SystemCollector 采集系统基础信息（启动时间等）
type SystemCollector struct{}

// SystemData 包含系统信息
type SystemData struct {
	BootTime string
	Hostname string
	OS       string
	Platform string
	Kernel   string
	Uptime   string
	Procs    uint64
}

// NewSystemCollector 创建系统采集器
func NewSystemCollector() *SystemCollector {
	return &SystemCollector{}
}

func (c *SystemCollector) Name() string {
	return "system"
}

func (c *SystemCollector) Collect(ctx context.Context) interface{} {
	data := SystemData{}

	info, _ := host.InfoWithContext(ctx)
	if info != nil {
		data.Hostname = info.Hostname
		data.OS = info.OS
		data.Platform = info.Platform
		data.Kernel = info.KernelVersion
		data.Procs = info.Procs

		bt := time.Unix(int64(info.BootTime), 0)
		data.BootTime = bt.Format("2006/01/02 15:04:05")

		uptime := time.Duration(info.Uptime) * time.Second
		data.Uptime = uptime.String()
	}

	return data
}
