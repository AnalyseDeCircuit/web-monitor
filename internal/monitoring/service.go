// Package monitoring 提供系统监控功能
package monitoring

import (
	"fmt"
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

	// 磁盘使用率 (使用 /hostfs 获取宿主机磁盘信息)
	// 注意：如果设置了 HOST_PROC 等环境变量，gopsutil 可能会自动处理，
	// 但 disk.Usage 需要明确的路径。
	// 如果在容器内，/hostfs 是宿主机的根。
	diskPath := "/hostfs"
	// 检查 /hostfs 是否存在，不存在则回退到 /
	// 这里简单假设如果 Usage("/hostfs") 成功则存在
	diskInfo, err := disk.Usage(diskPath)
	if err != nil {
		// Fallback to / if /hostfs fails
		diskInfo, err = disk.Usage("/")
		if err != nil {
			return nil, err
		}
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

	// 获取所有分区信息
	partitions, err := disk.Partitions(false)
	if err == nil {
		var disks []types.DiskInfo
		for _, p := range partitions {
			// Skip loop devices, snaps, etc.
			if p.Device == "" || p.Fstype == "squashfs" {
				continue
			}

			// Get usage for each partition
			// If running in container with /hostfs, we need to be careful.
			// gopsutil reads /proc/mounts (from host if HOST_PROC set).
			// But Usage() needs a path accessible to the container.
			// If p.Mountpoint is /mnt/data, we need to check /hostfs/mnt/data

			checkPath := p.Mountpoint
			if diskPath == "/hostfs" {
				checkPath = "/hostfs" + p.Mountpoint
			}

			usage, err := disk.Usage(checkPath)
			if err != nil {
				continue
			}

			disks = append(disks, types.DiskInfo{
				Device:     p.Device,
				Mountpoint: p.Mountpoint,
				Fstype:     p.Fstype,
				Total:      formatBytes(usage.Total),
				Used:       formatBytes(usage.Used),
				Free:       formatBytes(usage.Free),
				Percent:    usage.UsedPercent,
			})
		}
		metrics.Disk = disks
	}

	// Disk IO
	ioCounters, err := disk.IOCounters()
	if err == nil {
		ioMap := make(map[string]types.DiskIOInfo)
		for name, io := range ioCounters {
			ioMap[name] = types.DiskIOInfo{
				ReadBytes:  formatBytes(io.ReadBytes),
				WriteBytes: formatBytes(io.WriteBytes),
				ReadCount:  io.ReadCount,
				WriteCount: io.WriteCount,
				ReadTime:   io.ReadTime,
				WriteTime:  io.WriteTime,
			}
		}
		metrics.DiskIO = ioMap
	}

	return metrics, nil
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return "0 B"
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
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
