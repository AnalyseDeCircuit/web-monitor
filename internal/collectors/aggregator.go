package collectors

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/internal/config"
	"github.com/AnalyseDeCircuit/opskernel/internal/monitoring"
	"github.com/AnalyseDeCircuit/opskernel/pkg/types"
)

// ModuleData holds the latest data from a single collector module
type ModuleData struct {
	value atomic.Value
}

func (m *ModuleData) Store(v interface{}) {
	m.value.Store(v)
}

func (m *ModuleData) Load() interface{} {
	return m.value.Load()
}

// StreamingAggregator runs collectors independently and merges results on demand
type StreamingAggregator struct {
	cpu     *CPUCollector
	memory  *MemoryCollector
	disk    *DiskCollector
	network *NetworkCollector
	sensors *SensorsCollector
	power   *PowerCollector
	gpu     *GPUCollector
	ssh     *SSHCollector
	system  *SystemCollector

	// Atomic storage for each module's latest data
	cpuData     ModuleData
	memoryData  ModuleData
	diskData    ModuleData
	networkData ModuleData
	sensorsData ModuleData
	powerData   ModuleData
	gpuData     ModuleData
	sshData     ModuleData
	systemData  ModuleData

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	interval atomic.Int64 // in milliseconds
	mu       sync.Mutex
	running  bool
}

// NewStreamingAggregator creates a new streaming aggregator
func NewStreamingAggregator() *StreamingAggregator {
	a := &StreamingAggregator{
		cpu:     NewCPUCollector(),
		memory:  NewMemoryCollector(),
		disk:    NewDiskCollector(),
		network: NewNetworkCollector(),
		sensors: NewSensorsCollector(),
		power:   NewPowerCollector(),
		gpu:     NewGPUCollector(),
		ssh:     NewSSHCollector(),
		system:  NewSystemCollector(),
	}
	a.interval.Store(5000) // default 5s
	return a
}

// SetInterval changes the collection interval dynamically
func (a *StreamingAggregator) SetInterval(d time.Duration) {
	if d < time.Second {
		d = time.Second
	}
	a.interval.Store(d.Milliseconds())
}

// Start begins all independent collector goroutines
func (a *StreamingAggregator) Start() {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.running = true
	a.mu.Unlock()

	cfg := config.Load()

	type collectorDef struct {
		name     string
		enabled  bool
		collect  func(context.Context) interface{}
		store    *ModuleData
		interval time.Duration // custom interval, 0 = use default
	}

	// Fast collectors: CPU, Memory, Network (core metrics)
	// Slow collectors: Disk, Sensors, GPU, SSH, System
	collectors := []collectorDef{
		{"cpu", cfg.EnableCPU, a.cpu.Collect, &a.cpuData, 0},
		{"memory", cfg.EnableMemory, a.memory.Collect, &a.memoryData, 0},
		{"network", cfg.EnableNetwork, a.network.Collect, &a.networkData, 0},
		{"disk", cfg.EnableDisk, a.disk.Collect, &a.diskData, 2 * time.Second},             // slower
		{"sensors", cfg.EnableSensors, a.sensors.Collect, &a.sensorsData, 2 * time.Second}, // slower
		{"power", cfg.EnablePower, a.power.Collect, &a.powerData, 3 * time.Second},         // slowest
		{"gpu", cfg.EnableGPU, a.gpu.Collect, &a.gpuData, 2 * time.Second},                 // slower
		{"ssh", cfg.EnableSSH, a.ssh.Collect, &a.sshData, 5 * time.Second},                 // slowest
		{"system", cfg.EnableSystem, a.system.Collect, &a.systemData, 10 * time.Second},    // very slow, rarely changes
	}

	for _, c := range collectors {
		if !c.enabled {
			continue
		}
		a.wg.Add(1)
		go a.runCollector(c.name, c.collect, c.store, c.interval)
	}
}

func (a *StreamingAggregator) runCollector(name string, collect func(context.Context) interface{}, store *ModuleData, customInterval time.Duration) {
	defer a.wg.Done()

	// Collect immediately on start
	ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	data := collect(ctx)
	cancel()
	if data != nil {
		store.Store(data)
	}

	for {
		// Determine interval
		interval := time.Duration(a.interval.Load()) * time.Millisecond
		if customInterval > 0 && customInterval > interval {
			interval = customInterval
		}

		select {
		case <-a.ctx.Done():
			log.Printf("collector %s: shutting down", name)
			return
		case <-time.After(interval):
			ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
			data := collect(ctx)
			cancel()
			if data != nil {
				store.Store(data)
			}
		}
	}
}

// Stop gracefully stops all collectors
func (a *StreamingAggregator) Stop() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	if a.cancel != nil {
		a.cancel()
	}
	a.running = false
	a.mu.Unlock()

	a.wg.Wait()
}

// GetLatestStats merges all latest module data into a single Response
// This is very fast as it just reads atomic values, no blocking
func (a *StreamingAggregator) GetLatestStats() types.Response {
	var resp types.Response

	// Initialize all fields to avoid null in JSON
	resp.Fans = []interface{}{}
	resp.Disk = []types.DiskInfo{}
	resp.DiskIO = map[string]types.DiskIOInfo{}
	resp.Inodes = []types.InodeInfo{}
	resp.Processes = []types.ProcessInfo{}
	resp.GPU = []types.GPUDetail{}
	resp.Network.Interfaces = map[string]types.Interface{}
	resp.Network.Sockets = map[string]int{}
	resp.Network.ConnectionStates = map[string]int{}
	resp.Network.Errors = map[string]uint64{}
	resp.Network.ListeningPorts = []types.ListeningPort{}
	resp.SSHStats = types.SSHStats{
		Sessions:         []types.SSHSession{},
		AuthMethods:      map[string]int{},
		OOMRiskProcesses: []types.ProcessInfo{},
	}

	// Merge CPU data
	if data := a.cpuData.Load(); data != nil {
		if cpuData, ok := data.(CPUData); ok {
			resp.CPU.Percent = cpuData.Percent
			resp.CPU.PerCore = cpuData.PerCore
			resp.CPU.Info = cpuData.Info
			resp.CPU.LoadAvg = cpuData.LoadAvg
			resp.CPU.Freq = cpuData.Freq
		}
	}

	// Merge Memory data
	if data := a.memoryData.Load(); data != nil {
		if memData, ok := data.(MemoryData); ok {
			resp.Memory = memData.Memory
			resp.Swap = memData.Swap
		}
	}

	// Merge Disk data
	if data := a.diskData.Load(); data != nil {
		if diskData, ok := data.(DiskData); ok {
			resp.Disk = diskData.Disks
			resp.Inodes = diskData.Inodes
			resp.DiskIO = diskData.IO
		}
	}

	// Merge Network data
	if data := a.networkData.Load(); data != nil {
		if netData, ok := data.(NetworkData); ok {
			resp.Network.BytesSent = netData.BytesSent
			resp.Network.BytesRecv = netData.BytesRecv
			resp.Network.RawSent = netData.RawSent
			resp.Network.RawRecv = netData.RawRecv
		}
	}

	// Merge Sensors data
	if data := a.sensorsData.Load(); data != nil {
		resp.Sensors = data
	}

	// Merge Power data
	if data := a.powerData.Load(); data != nil {
		resp.Power = data
	}

	// Merge GPU data
	if data := a.gpuData.Load(); data != nil {
		if gpuData, ok := data.([]types.GPUDetail); ok {
			resp.GPU = gpuData
		}
	}

	// Merge SSH data
	if data := a.sshData.Load(); data != nil {
		if sshData, ok := data.(types.SSHStats); ok {
			resp.SSHStats = sshData
		}
	}

	// Merge System data
	if data := a.systemData.Load(); data != nil {
		if sysData, ok := data.(SystemData); ok {
			resp.BootTime = sysData.BootTime
		}
	}

	// Update temperature history
	currentTemp := extractAvgTemp(resp.Sensors)
	resp.CPU.TempHistory = a.cpu.UpdateTempHistory(currentTemp)

	// Update CPU percent history
	resp.CPU.PercentHistory = a.cpu.UpdatePercentHistory(resp.CPU.Percent)

	// Calculate max disk usage for alerts
	maxDisk := 0.0
	for _, d := range resp.Disk {
		if d.Percent > maxDisk {
			maxDisk = d.Percent
		}
	}

	// Check alerts
	monitoring.CheckAlerts(resp.CPU.Percent, resp.Memory.Percent, maxDisk)

	return resp
}

// extractAvgTemp extracts average temperature from sensors data
func extractAvgTemp(sensors interface{}) float64 {
	if sensorsMap, ok := sensors.(map[string][]interface{}); ok {
		count := 0.0
		sum := 0.0
		for _, list := range sensorsMap {
			for _, item := range list {
				if m, ok := item.(map[string]interface{}); ok {
					if t, ok := m["current"].(float64); ok && t > 0 {
						sum += t
						count++
					}
				}
			}
		}
		if count > 0 {
			return sum / count
		}
	}
	return 0
}

// ========== Legacy support (for backward compatibility) ==========

// StatsAggregator is kept for backward compatibility
type StatsAggregator struct {
	streaming *StreamingAggregator
}

// NewStatsAggregator creates a new aggregator using the streaming backend
func NewStatsAggregator() *StatsAggregator {
	return &StatsAggregator{
		streaming: NewStreamingAggregator(),
	}
}

// Start begins the streaming collectors
func (a *StatsAggregator) Start() {
	a.streaming.Start()
}

// Stop gracefully stops the collectors
func (a *StatsAggregator) Stop() {
	a.streaming.Stop()
}

// SetInterval changes the collection interval
func (a *StatsAggregator) SetInterval(d time.Duration) {
	a.streaming.SetInterval(d)
}

// CollectBaseStats returns the latest merged stats (non-blocking)
func (a *StatsAggregator) CollectBaseStats() types.Response {
	return a.streaming.GetLatestStats()
}
