package collectors

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
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

	ioRefresh  time.Duration
	cwdRefresh time.Duration
}

type processCacheEntry struct {
	proc       *process.Process
	name       string
	username   string
	cmdline    string
	createTime int64
	ppid       int32

	ioRead       string
	ioWrite      string
	lastIOUpdate time.Time

	cwd           string
	lastCwdUpdate time.Time
}

// NewProcessCollector 创建进程采集器
func NewProcessCollector() *ProcessCollector {
	return &ProcessCollector{
		cache:      make(map[int32]*processCacheEntry),
		ioRefresh:  envDuration("PROCESS_IO_REFRESH", 30*time.Second),
		cwdRefresh: envDuration("PROCESS_CWD_REFRESH", 60*time.Second),
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
	now := time.Now()

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	seenPids := make(map[int32]bool, len(pids))
	result := make([]types.ProcessInfo, 0, len(pids))
	canceled := false

	for _, pid := range pids {
		// Check context cancellation periodically
		select {
		case <-ctx.Done():
			canceled = true
		default:
		}
		if canceled {
			break
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
				ioRead:     "-",
				ioWrite:    "-",
				cwd:        "-",
			}
			c.cache[pid] = entry
		}

		// Fetch dynamic info
		cpuPercent, _ := entry.proc.CPUPercent()
		memPercent, _ := entry.proc.MemoryPercent()
		numThreads, _ := entry.proc.NumThreads()

		// IO counters: expensive on large process counts, refresh less frequently.
		if c.ioRefresh > 0 && now.Sub(entry.lastIOUpdate) >= c.ioRefresh {
			if ioCounters, err := entry.proc.IOCounters(); err == nil {
				entry.ioRead = utils.GetSize(ioCounters.ReadBytes)
				entry.ioWrite = utils.GetSize(ioCounters.WriteBytes)
			} else {
				entry.ioRead = "-"
				entry.ioWrite = "-"
			}
			entry.lastIOUpdate = now
		}

		// Cwd: can be expensive (readlink) and often unchanged.
		if c.cwdRefresh > 0 && now.Sub(entry.lastCwdUpdate) >= c.cwdRefresh {
			cwd, _ := entry.proc.Cwd()
			if cwd == "" {
				cwd = "-"
			}
			entry.cwd = cwd
			entry.lastCwdUpdate = now
		}

		uptimeSec := now.Unix() - (entry.createTime / 1000)
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
			Cwd:           entry.cwd,
			IORead:        entry.ioRead,
			IOWrite:       entry.ioWrite,
		})
	}

	// Cleanup dead processes
	if !canceled {
		for pid := range c.cache {
			if !seenPids[pid] {
				delete(c.cache, pid)
			}
		}
	}

	// Sort by memory percent desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].MemoryPercent > result[j].MemoryPercent
	})

	return result
}

func envDuration(name string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	// Allow plain seconds like "30".
	allDigits := true
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		v += "s"
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
