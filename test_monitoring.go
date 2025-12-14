package main

import (
	"fmt"
	"log"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/monitoring"
)

func main() {
	fmt.Println("Testing monitoring module...")

	// 测试GetSystemMetrics
	start := time.Now()
	metrics, err := monitoring.GlobalMonitoringService.GetSystemMetrics()
	elapsed := time.Since(start)

	if err != nil {
		log.Printf("Error getting system metrics: %v", err)
	} else {
		fmt.Printf("Success! Got metrics in %v\n", elapsed)
		fmt.Printf("CPU: %.2f%%, Memory: %.2f%%, Disk: %.2f%%\n",
			metrics.CPUPercent, metrics.MemoryPercent, metrics.DiskPercent)
	}
}
