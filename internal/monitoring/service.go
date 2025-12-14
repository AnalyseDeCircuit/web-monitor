// Package monitoring 提供系统监控功能
package monitoring

import (
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/cache"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// MonitoringService 监控服务
type MonitoringService struct {
	cache *cache.MetricsCache
}

// NewMonitoringService 创建新的监控服务
func NewMonitoringService() *MonitoringService {
	return &MonitoringService{
		cache: cache.GlobalMetricsCache,
	}
}

// GetSystemMetrics 获取系统指标（带缓存）
func (s *MonitoringService) GetSystemMetrics() (*types.SystemMetrics, error) {
	// 尝试从缓存获取
	if cached, found := s.cache.Get(cache.CacheKeySystemMetrics); found {
		if metrics, ok := cached.(*types.SystemMetrics); ok {
			return metrics, nil
		}
	}

	// 缓存未命中，获取实时数据
	metrics, err := s.getRealSystemMetrics()
	if err != nil {
		return nil, err
	}

	// 存入缓存
	s.cache.Set(cache.CacheKeySystemMetrics, metrics, cache.DefaultTTL)

	return metrics, nil
}

// getRealSystemMetrics 获取实时系统指标
func (s *MonitoringService) getRealSystemMetrics() (*types.SystemMetrics, error) {
	// CPU使用率
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		return nil, err
	}

	// 内存使用率
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	// 磁盘使用率
	diskInfo, err := disk.Usage("/")
	if err != nil {
		return nil, err
	}

	// 检查告警
	CheckAlerts(cpuPercent[0], memInfo.UsedPercent, diskInfo.UsedPercent)

	metrics := &types.SystemMetrics{
		CPUPercent:    cpuPercent[0],
		MemoryPercent: memInfo.UsedPercent,
		MemoryTotal:   memInfo.Total,
		MemoryUsed:    memInfo.Used,
		MemoryFree:    memInfo.Free,
		DiskPercent:   diskInfo.UsedPercent,
		DiskTotal:     diskInfo.Total,
		DiskUsed:      diskInfo.Used,
		DiskFree:      diskInfo.Free,
		Timestamp:     time.Now(),
	}

	return metrics, nil
}

// GetCachedMetrics 获取缓存的系统指标（不触发实时更新）
func (s *MonitoringService) GetCachedMetrics() (*types.SystemMetrics, bool) {
	if cached, found := s.cache.Get(cache.CacheKeySystemMetrics); found {
		if metrics, ok := cached.(*types.SystemMetrics); ok {
			return metrics, true
		}
	}
	return nil, false
}

// ForceRefresh 强制刷新系统指标
func (s *MonitoringService) ForceRefresh() (*types.SystemMetrics, error) {
	// 删除缓存
	s.cache.Delete(cache.CacheKeySystemMetrics)

	// 获取新数据
	return s.GetSystemMetrics()
}

// GetMetricsWithFallback 获取指标，如果缓存过期则使用实时数据
func (s *MonitoringService) GetMetricsWithFallback() (*types.SystemMetrics, error) {
	// 先尝试获取缓存
	if metrics, found := s.GetCachedMetrics(); found {
		return metrics, nil
	}

	// 缓存未命中，获取实时数据
	return s.GetSystemMetrics()
}

// GlobalMonitoringService 全局监控服务实例
var GlobalMonitoringService = NewMonitoringService()
