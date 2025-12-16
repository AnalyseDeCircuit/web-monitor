package collectors

import (
	"context"

	"github.com/AnalyseDeCircuit/web-monitor/internal/gpu"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

// GPUCollector 采集 GPU 信息
type GPUCollector struct{}

// NewGPUCollector 创建 GPU 采集器
func NewGPUCollector() *GPUCollector {
	return &GPUCollector{}
}

func (c *GPUCollector) Name() string {
	return "gpu"
}

func (c *GPUCollector) Collect(ctx context.Context) interface{} {
	// gpu.GetGPUInfo() 内部已有缓存机制
	return gpu.GetGPUInfo()
}

// GPUData 用于类型断言
type GPUData = []types.GPUDetail
