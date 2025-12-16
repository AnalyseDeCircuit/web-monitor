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

	bootTime, _ := host.BootTime()
	bt := time.Unix(int64(bootTime), 0)
	data.BootTime = bt.Format("2006/01/02 15:04:05")

	return data
}
