package collectors

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

// StatsAggregator 并行采集所有基础指标并聚合结果
type StatsAggregator struct {
	cpu     *CPUCollector
	memory  *MemoryCollector
	disk    *DiskCollector
	network *NetworkCollector
	sensors *SensorsCollector
	power   *PowerCollector
	gpu     *GPUCollector
	ssh     *SSHCollector
	system  *SystemCollector

	timeout time.Duration
}

// NewStatsAggregator 创建统计聚合器
func NewStatsAggregator() *StatsAggregator {
	return &StatsAggregator{
		cpu:     NewCPUCollector(),
		memory:  NewMemoryCollector(),
		disk:    NewDiskCollector(),
		network: NewNetworkCollector(),
		sensors: NewSensorsCollector(),
		power:   NewPowerCollector(),
		gpu:     NewGPUCollector(),
		ssh:     NewSSHCollector(),
		system:  NewSystemCollector(),
		timeout: 8 * time.Second, // 单次采集超时
	}
}

// CollectBaseStats 并行采集所有基础指标
func (a *StatsAggregator) CollectBaseStats() types.Response {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

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

	// 使用 WaitGroup 和 Channel 并行采集
	type result struct {
		name string
		data interface{}
	}
	resultCh := make(chan result, 9)

	var wg sync.WaitGroup

	// 启动所有采集器
	collectors := []struct {
		name    string
		collect func(context.Context) interface{}
	}{
		{"cpu", a.cpu.Collect},
		{"memory", a.memory.Collect},
		{"disk", a.disk.Collect},
		{"network", a.network.Collect},
		{"sensors", a.sensors.Collect},
		{"power", a.power.Collect},
		{"gpu", a.gpu.Collect},
		{"ssh", a.ssh.Collect},
		{"system", a.system.Collect},
	}

	for _, c := range collectors {
		wg.Add(1)
		go func(name string, collect func(context.Context) interface{}) {
			defer wg.Done()
			data := collect(ctx)
			select {
			case resultCh <- result{name: name, data: data}:
			case <-ctx.Done():
				log.Printf("collector %s timed out", name)
			}
		}(c.name, c.collect)
	}

	// 等待所有采集完成或超时
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果并组装 Response
	for r := range resultCh {
		switch r.name {
		case "cpu":
			if data, ok := r.data.(CPUData); ok {
				resp.CPU.Percent = data.Percent
				resp.CPU.PerCore = data.PerCore
				resp.CPU.Info = data.Info
				resp.CPU.LoadAvg = data.LoadAvg
				resp.CPU.Stats = data.Stats
				resp.CPU.Times = data.Times
				resp.CPU.Freq = data.Freq
			}

		case "memory":
			if data, ok := r.data.(MemoryData); ok {
				resp.Memory = data.Memory
				resp.Swap = data.Swap
			}

		case "disk":
			if data, ok := r.data.(DiskData); ok {
				resp.Disk = data.Disks
				resp.Inodes = data.Inodes
				resp.DiskIO = data.IO
			}

		case "network":
			if data, ok := r.data.(NetworkData); ok {
				resp.Network.BytesSent = data.BytesSent
				resp.Network.BytesRecv = data.BytesRecv
				resp.Network.RawSent = data.RawSent
				resp.Network.RawRecv = data.RawRecv
			}

		case "sensors":
			resp.Sensors = r.data

		case "power":
			resp.Power = r.data

		case "gpu":
			if data, ok := r.data.([]types.GPUDetail); ok {
				resp.GPU = data
			}

		case "ssh":
			if data, ok := r.data.(types.SSHStats); ok {
				resp.SSHStats = data
			}

		case "system":
			if data, ok := r.data.(SystemData); ok {
				resp.BootTime = data.BootTime
			}
		}
	}

	// 更新温度历史（需要从 sensors 数据中提取）
	currentTemp := extractAvgTemp(resp.Sensors)
	resp.CPU.TempHistory = a.cpu.UpdateTempHistory(currentTemp)

	// 检查告警
	monitoring.CheckAlerts(resp.CPU.Percent, resp.Memory.Percent, 0)

	return resp
}

// extractAvgTemp 从传感器数据中提取平均温度
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
