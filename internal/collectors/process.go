package collectors

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	"github.com/shirou/gopsutil/v3/process"
)

// ProcessCollector 采集进程信息
type ProcessCollector struct {
	cache   map[int32]*processCacheEntry
	cacheMu sync.Mutex
}

type processCacheEntry struct {
	proc       *process.Process
	name       string
	username   string
	cmdline    string
	createTime int64
	ppid       int32
}

// NewProcessCollector 创建进程采集器
func NewProcessCollector() *ProcessCollector {
	return &ProcessCollector{
		cache: make(map[int32]*processCacheEntry),
	}
}

func (c *ProcessCollector) Name() string {
	return "processes"
}

func (c *ProcessCollector) Collect(ctx context.Context) interface{} {
	pids, err := process.Pids()
	if err != nil {
		return []types.ProcessInfo{}
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	seenPids := make(map[int32]bool)
	var result []types.ProcessInfo

	for _, pid := range pids {
		// Check context cancellation periodically
		select {
		case <-ctx.Done():
			return result
		default:
		}

		seenPids[pid] = true

		entry, exists := c.cache[pid]
		if !exists {
			proc, err := process.NewProcess(pid)
			if err != nil {
				continue
			}

			// Fetch static info once
			name, _ := proc.Name()
			username, _ := proc.Username()
			if username == "" {
				if uids, err := proc.Uids(); err == nil && len(uids) > 0 {
					username = fmt.Sprintf("uid:%d", uids[0])
				} else {
					username = "unknown"
				}
			}
			cmdline, _ := proc.Cmdline()
			createTime, _ := proc.CreateTime()
			ppid, _ := proc.Ppid()

			entry = &processCacheEntry{
				proc:       proc,
				name:       name,
				username:   username,
				cmdline:    cmdline,
				createTime: createTime,
				ppid:       ppid,
			}
			c.cache[pid] = entry
		}

		// Fetch dynamic info
		cpuPercent, _ := entry.proc.CPUPercent()
		memPercent, _ := entry.proc.MemoryPercent()
		numThreads, _ := entry.proc.NumThreads()

		ioRead := "-"
		ioWrite := "-"
		if ioCounters, err := entry.proc.IOCounters(); err == nil {
			ioRead = utils.GetSize(ioCounters.ReadBytes)
			ioWrite = utils.GetSize(ioCounters.WriteBytes)
		}

		cwd, _ := entry.proc.Cwd()
		if cwd == "" {
			cwd = "-"
		}

		uptimeSec := time.Now().Unix() - (entry.createTime / 1000)
		uptimeStr := "-"
		if uptimeSec < 60 {
			uptimeStr = fmt.Sprintf("%ds", uptimeSec)
		} else if uptimeSec < 3600 {
			uptimeStr = fmt.Sprintf("%dm", uptimeSec/60)
		} else if uptimeSec < 86400 {
			uptimeStr = fmt.Sprintf("%dh", uptimeSec/3600)
		} else {
			uptimeStr = fmt.Sprintf("%dd", uptimeSec/86400)
		}

		result = append(result, types.ProcessInfo{
			PID:           pid,
			Name:          entry.name,
			Username:      entry.username,
			NumThreads:    numThreads,
			MemoryPercent: utils.Round(float64(memPercent)),
			CPUPercent:    utils.Round(cpuPercent),
			PPID:          entry.ppid,
			Uptime:        uptimeStr,
			Cmdline:       entry.cmdline,
			Cwd:           cwd,
			IORead:        ioRead,
			IOWrite:       ioWrite,
		})
	}

	// Cleanup dead processes
	for pid := range c.cache {
		if !seenPids[pid] {
			delete(c.cache, pid)
		}
	}

	// Sort by memory percent desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].MemoryPercent > result[j].MemoryPercent
	})

	return result
}
