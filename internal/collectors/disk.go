package collectors

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/config"
	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	"github.com/shirou/gopsutil/v3/disk"
)

// DiskCollector 采集磁盘相关指标
type DiskCollector struct {
	usageCache    []types.DiskInfo
	inodeCache    []types.InodeInfo
	lastUpdate    time.Time
	cacheMu       sync.Mutex
	cacheInterval time.Duration
}

// DiskData 包含磁盘采集结果
type DiskData struct {
	Disks  []types.DiskInfo
	Inodes []types.InodeInfo
	IO     map[string]types.DiskIOInfo
}

// NewDiskCollector 创建磁盘采集器
func NewDiskCollector() *DiskCollector {
	return &DiskCollector{
		cacheInterval: 10 * time.Second,
	}
}

func (c *DiskCollector) Name() string {
	return "disk"
}

func (c *DiskCollector) Collect(ctx context.Context) interface{} {
	data := DiskData{
		Disks:  []types.DiskInfo{},
		Inodes: []types.InodeInfo{},
		IO:     make(map[string]types.DiskIOInfo),
	}

	// Throttled disk usage collection (expensive operation)
	c.cacheMu.Lock()
	now := time.Now()
	if now.Sub(c.lastUpdate) > c.cacheInterval {
		var newDiskInfo []types.DiskInfo
		var newInodeInfo []types.InodeInfo

		parts, _ := disk.Partitions(false)
		for _, part := range parts {
			// Skip loop devices and squashfs
			if strings.Contains(part.Device, "loop") || part.Fstype == "squashfs" {
				continue
			}

			// Check context cancellation
			select {
			case <-ctx.Done():
				c.cacheMu.Unlock()
				return data
			default:
			}

			checkPath := config.HostPath(part.Mountpoint)

			u, err := disk.Usage(checkPath)
			if err == nil {
				newDiskInfo = append(newDiskInfo, types.DiskInfo{
					Device:     part.Device,
					Mountpoint: part.Mountpoint,
					Fstype:     part.Fstype,
					Total:      utils.GetSize(u.Total),
					Used:       utils.GetSize(u.Used),
					Free:       utils.GetSize(u.Free),
					Percent:    utils.Round(u.UsedPercent),
				})

				if u.InodesTotal > 0 {
					newInodeInfo = append(newInodeInfo, types.InodeInfo{
						Mountpoint: part.Mountpoint,
						Total:      u.InodesTotal,
						Used:       u.InodesUsed,
						Free:       u.InodesFree,
						Percent:    utils.Round(u.InodesUsedPercent),
					})
				}
			}
		}
		c.usageCache = newDiskInfo
		c.inodeCache = newInodeInfo
		c.lastUpdate = now
	}

	// Use cached values
	if c.usageCache != nil {
		data.Disks = make([]types.DiskInfo, len(c.usageCache))
		copy(data.Disks, c.usageCache)
	}
	if c.inodeCache != nil {
		data.Inodes = make([]types.InodeInfo, len(c.inodeCache))
		copy(data.Inodes, c.inodeCache)
	}
	c.cacheMu.Unlock()

	// Disk IO (fast operation)
	ioCounters, _ := disk.IOCounters()
	if ioCounters != nil {
		for name, io := range ioCounters {
			data.IO[name] = types.DiskIOInfo{
				ReadBytes:  utils.GetSize(io.ReadBytes),
				WriteBytes: utils.GetSize(io.WriteBytes),
				ReadCount:  io.ReadCount,
				WriteCount: io.WriteCount,
				ReadTime:   io.ReadTime,
				WriteTime:  io.WriteTime,
			}
		}
	}

	return data
}
