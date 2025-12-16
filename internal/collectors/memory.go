package collectors

import (
	"context"
	"sync"

	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	"github.com/shirou/gopsutil/v3/mem"
)

// MemoryCollector 采集内存相关指标
type MemoryCollector struct {
	history   []float64
	historyMu sync.Mutex
}

// MemoryData 包含内存采集结果
type MemoryData struct {
	Memory  types.MemInfo
	Swap    types.SwapInfo
	History []float64
}

// NewMemoryCollector 创建内存采集器
func NewMemoryCollector() *MemoryCollector {
	return &MemoryCollector{
		history: make([]float64, 0, 300),
	}
}

func (c *MemoryCollector) Name() string {
	return "memory"
}

func (c *MemoryCollector) Collect(ctx context.Context) interface{} {
	data := MemoryData{}

	// Virtual Memory
	v, _ := mem.VirtualMemory()
	data.Memory = types.MemInfo{
		Total:     utils.GetSize(v.Total),
		Used:      utils.GetSize(v.Used),
		Free:      utils.GetSize(v.Free),
		Percent:   utils.Round(v.UsedPercent),
		Available: utils.GetSize(v.Available),
		Buffers:   utils.GetSize(v.Buffers),
		Cached:    utils.GetSize(v.Cached),
		Shared:    utils.GetSize(v.Shared),
		Active:    utils.GetSize(v.Active),
		Inactive:  utils.GetSize(v.Inactive),
		Slab:      utils.GetSize(v.Slab),
	}

	// Update history
	c.historyMu.Lock()
	if len(c.history) >= 300 {
		copy(c.history, c.history[1:])
		c.history = c.history[:len(c.history)-1]
	}
	c.history = append(c.history, v.UsedPercent)
	data.History = make([]float64, len(c.history))
	copy(data.History, c.history)
	c.historyMu.Unlock()

	data.Memory.History = data.History

	// Swap Memory
	s, _ := mem.SwapMemory()
	data.Swap = types.SwapInfo{
		Total:   utils.GetSize(s.Total),
		Used:    utils.GetSize(s.Used),
		Free:    utils.GetSize(s.Free),
		Percent: utils.Round(s.UsedPercent),
		Sin:     utils.GetSize(s.Sin),
		Sout:    utils.GetSize(s.Sout),
	}

	return data
}
