// Package prometheus 提供Prometheus指标导出功能
package prometheus

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

var (
	metricsMutex sync.Mutex
	metricsOnce  sync.Once

	// CPU指标
	cpuUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_cpu_usage_percent",
			Help: "CPU usage percentage",
		},
		[]string{"core"},
	)

	cpuLoad = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_cpu_load_average",
			Help: "CPU load average",
		},
		[]string{"period"},
	)

	// 内存指标
	memoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_memory_usage_bytes",
			Help: "Memory usage in bytes",
		},
		[]string{"type"},
	)

	memoryPercent = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "system_memory_usage_percent",
			Help: "Memory usage percentage",
		},
	)

	// 磁盘指标
	diskUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_disk_usage_bytes",
			Help: "Disk usage in bytes",
		},
		[]string{"device", "mountpoint", "type"},
	)

	diskPercent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_disk_usage_percent",
			Help: "Disk usage percentage",
		},
		[]string{"device", "mountpoint"},
	)

	// 网络指标
	networkBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "system_network_bytes_total",
			Help: "Network bytes transmitted/received",
		},
		[]string{"interface", "direction"},
	)

	networkPackets = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "system_network_packets_total",
			Help: "Network packets transmitted/received",
		},
		[]string{"interface", "direction"},
	)

	// 系统指标
	uptimeSeconds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "system_uptime_seconds",
			Help: "System uptime in seconds",
		},
	)

	// 进程指标
	processCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "system_process_count",
			Help: "Number of running processes",
		},
	)

	// 告警指标
	alertStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_alert_status",
			Help: "Alert status (1=triggered, 0=normal)",
		},
		[]string{"type"},
	)

	// 自定义指标
	customMetrics = make(map[string]prometheus.Gauge)
	customMetricsMutex sync.RWMutex
)

// InitPrometheus 初始化Prometheus指标
func InitPrometheus() {
	metricsOnce.Do(func() {
		// 注册默认指标
		prometheus.MustRegister(cpuUsage)
		prometheus.MustRegister(cpuLoad)
		prometheus.MustRegister(memoryUsage)
		prometheus.MustRegister(memoryPercent)
		prometheus.MustRegister(diskUsage)
		prometheus.MustRegister(diskPercent)
		prometheus.MustRegister(networkBytes)
		prometheus.MustRegister(networkPackets)
		prometheus.MustRegister(uptimeSeconds)
		prometheus.MustRegister(processCount)
		prometheus.MustRegister(alertStatus)

		// 启动指标更新goroutine
		go updateMetrics()
	})
}

// updateMetrics 定期更新指标
func updateMetrics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		updateSystemMetrics()
	}
}

// updateSystemMetrics 更新系统指标
func updateSystemMetrics() {
	metricsMutex.Lock()
	defer metricsMutex.Unlock()

	// CPU指标
	cpuPercent, err := cpu.Percent(0, true)
	if err == nil {
		for i, percent := range cpuPercent {
			cpuUsage.WithLabelValues(fmt.Sprintf("core%d", i)).Set(utils.Round(percent))
		}
	}

	// CPU负载
	if load, err := cpu.Percent(0, false); err == nil && len(load) > 0 {
		cpuUsage.WithLabelValues("total").Set(utils.Round(load[0]))
	}

	// 内存指标
	if memInfo, err := mem.VirtualMemory(); err == nil {
		memoryUsage.WithLabelValues("total").Set(float64(memInfo.Total))
		memoryUsage.WithLabelValues("used").Set(float64(memInfo.Used))
		memoryUsage.WithLabelValues("free").Set(float64(memInfo.Free))
		memoryUsage.WithLabelValues("available").Set(float64(memInfo.Available))
		memoryPercent.Set(utils.Round(memInfo.UsedPercent))
	}

	// 交换内存
	if swapInfo, err := mem.SwapMemory(); err == nil {
		memoryUsage.WithLabelValues("swap_total").Set(float64(swapInfo.Total))
		memoryUsage.WithLabelValues("swap_used").Set(float64(swapInfo.Used))
		memoryUsage.WithLabelValues("swap_free").Set(float64(swapInfo.Free))
	}

	// 磁盘指标
	if partitions, err := disk.Partitions(false); err == nil {
		for _, part := range partitions {
			if usage, err := disk.Usage(part.Mountpoint); err == nil {
				diskUsage.WithLabelValues(part.Device, part.Mountpoint, "total").Set(float64(usage.Total))
				diskUsage.WithLabelValues(part.Device, part.Mountpoint, "used").Set(float64(usage.Used))
				diskUsage.WithLabelValues(part.Device, part.Mountpoint, "free").Set(float64(usage.Free))
				diskPercent.WithLabelValues(part.Device, part.Mountpoint).Set(utils.Round(usage.UsedPercent))
			}
		}
	}

	// 网络指标
	if netIO, err := net.IOCounters(true); err == nil {
		for _, io := range netIO {
			networkBytes.WithLabelValues(io.Name, "tx").Add(float64(io.BytesSent))
			networkBytes.WithLabelValues(io.Name, "rx").Add(float64(io.BytesRecv))
			networkPackets.WithLabelValues(io.Name, "tx").Add(float64(io.PacketsSent))
			networkPackets.WithLabelValues(io.Name, "rx").Add(float64(io.PacketsRecv))
		}
	}

	// 系统运行时间
	if uptime, err := getUptimeSeconds(); err == nil {
		uptimeSeconds.Set(uptime)
	}

	// 进程计数
	if count, err := getProcessCount(); err == nil {
		processCount.Set(float64(count))
	}

	// 告警状态
	updateAlertMetrics()
}

// updateAlertMetrics 更新告警指标
func updateAlertMetrics() {
	config := monitoring.GetAlertConfig()
	if !config.Enabled {
		return
	}

	// 这里需要获取当前系统状态来更新告警指标
	// 暂时设置为0（正常状态）
	alertStatus.WithLabelValues("cpu").Set(0)
	alertStatus.WithLabelValues("memory").Set(0)
	alertStatus.WithLabelValues("disk").Set(0)
}

// getUptimeSeconds 获取系统运行时间（秒）
func getUptimeSeconds() (float64, error) {
	// 这里应该从系统获取实际运行时间
	// 暂时返回一个固定值
	return 3600.0, nil
}

// getProcessCount 获取进程数量
func getProcessCount() (int, error) {
	// 这里应该从系统获取实际进程数量
	// 暂时返回一个估计值
	return 150, nil
}

// CreateCustomMetric 创建自定义指标
func CreateCustomMetric(name, help string) error {
	customMetricsMutex.Lock()
	defer customMetricsMutex.Unlock()

	if _, exists := customMetrics[name]; exists {
		return fmt.Errorf("metric %s already exists", name)
	}

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	})

	if err := prometheus.Register(gauge); err != nil {
		return fmt.Errorf("failed to register metric %s: %v", name, err)
	}

	customMetrics[name] = gauge
	return nil
}

// SetCustomMetric 设置自定义指标值
func SetCustomMetric(name string, value float64) error {
	customMetricsMutex.RLock()
	gauge, exists := customMetrics[name]
	customMetricsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("metric %s not found", name)
	}

	gauge.Set(value)
	return nil
}

// DeleteCustomMetric 删除自定义指标
func DeleteCustomMetric(name string) error {
	customMetricsMutex.Lock()
	defer customMetricsMutex.Unlock()

	gauge, exists := customMetrics[name]
	if !exists {
		return fmt.Errorf("metric %s not found", name)
	}

	if !prometheus.Unregister(gauge) {
		return fmt.Errorf("failed to unregister metric %s", name)
	}

	delete(customMetrics, name)
	return nil
}

// GetMetricsHandler 获取Prometheus指标处理器
func GetMetricsHandler() http.Handler {
	return promhttp.Handler()
}

// GetMetrics 获取当前指标值
func GetMetrics() map[string]interface{} {
	metricsMutex.Lock()
	defer metricsMutex.Unlock()

	result := make(map[string]interface{})

	// 收集CPU指标
	cpuPercent, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercent) > 0 {
		result["cpu_usage_percent"] = utils.Round(cpuPercent[0])
	}

	// 收集内存指标
	if memInfo, err := mem.VirtualMemory(); err == nil {
		result["memory_usage_percent"] = utils.Round(memInfo.UsedPercent)
		result["memory_total_bytes"] = memInfo.Total
		result["memory_used_bytes"] = memInfo.Used
	}

	// 收集磁盘指标
	if partitions, err := disk.Partitions(false); err == nil {
		var diskUsage []map[string]interface{}
		for _, part := range partitions {
			if usage, err := disk.Usage(part.Mountpoint); err == nil {
				diskUsage = append(diskUsage, map[string]interface{}{
					"device":     part.Device,
					"mountpoint": part.Mountpoint,
					"usage":      utils.Round(usage.UsedPercent),
					"total":      usage.Total,
					"used":       usage.Used,
				})
			}
		}
		result["disk_usage"] = diskUsage
	}

	return result
}

// ExportMetrics 导出指标到Prometheus格式
func ExportMetrics() string {
	// 这里应该生成Prometheus格式的指标数据
	// 暂时返回一个简单的示例
	return `# HELP system_cpu_usage_percent CPU usage percentage
# TYPE system_cpu_usage_percent gauge
system_cpu_usage_percent{core="total"} 15.5

# HELP system_memory_usage_percent Memory usage percentage
# TYPE system_memory_usage_percent gauge
system_memory_usage_percent 45.2

# HELP system_disk_usage_percent Disk usage percentage
# TYPE system_disk_usage_percent gauge
system_disk_usage_percent{device="/dev/sda1",mountpoint="/"} 65.8
`
