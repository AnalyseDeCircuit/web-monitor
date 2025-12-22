package collectors

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/internal/config"
	"github.com/AnalyseDeCircuit/opskernel/internal/utils"
	"github.com/shirou/gopsutil/v3/host"
)

// SensorsCollector 采集传感器温度
type SensorsCollector struct{}

// NewSensorsCollector 创建传感器采集器
func NewSensorsCollector() *SensorsCollector {
	return &SensorsCollector{}
}

func (c *SensorsCollector) Name() string {
	return "sensors"
}

func (c *SensorsCollector) Collect(ctx context.Context) interface{} {
	sensors := make(map[string][]interface{})
	if temps, err := host.SensorsTemperatures(); err == nil {
		for _, t := range temps {
			sensors[t.SensorKey] = append(sensors[t.SensorKey], map[string]interface{}{
				"label":    t.SensorKey,
				"current":  t.Temperature,
				"high":     t.High,
				"critical": t.Critical,
			})
		}
	}
	return sensors
}

// PowerCollector 采集电源/功耗信息
type PowerCollector struct {
	raplReadings map[string]uint64
	raplTime     time.Time
	raplMu       sync.Mutex
}

// NewPowerCollector 创建电源采集器
func NewPowerCollector() *PowerCollector {
	return &PowerCollector{
		raplReadings: make(map[string]uint64),
	}
}

func (c *PowerCollector) Name() string {
	return "power"
}

func (c *PowerCollector) Collect(ctx context.Context) interface{} {
	powerStatus := make(map[string]interface{})

	// Try to read battery/adapter power consumption
	basePaths := []string{config.HostPath("/sys/class/power_supply"), "/sys/class/power_supply"}
	foundConsumption := false
	for _, basePath := range basePaths {
		if _, err := os.Stat(basePath); err != nil {
			continue
		}

		entries, _ := os.ReadDir(basePath)
		for _, e := range entries {
			supplyPath := filepath.Join(basePath, e.Name())

			// Prefer power_now (microwatts)
			if content, err := os.ReadFile(filepath.Join(supplyPath, "power_now")); err == nil {
				if pNow, err := strconv.ParseFloat(strings.TrimSpace(string(content)), 64); err == nil {
					powerStatus["consumption_watts"] = utils.Round(pNow / 1000000.0)
					foundConsumption = true
					break
				}
			}

			// Fallback: voltage_now * current_now
			vBytes, err1 := os.ReadFile(filepath.Join(supplyPath, "voltage_now"))
			cBytes, err2 := os.ReadFile(filepath.Join(supplyPath, "current_now"))
			if err1 == nil && err2 == nil {
				vNow, _ := strconv.ParseFloat(strings.TrimSpace(string(vBytes)), 64)
				cNow, _ := strconv.ParseFloat(strings.TrimSpace(string(cBytes)), 64)
				powerStatus["consumption_watts"] = utils.Round((vNow * cNow) / 1e12)
				foundConsumption = true
				break
			}
		}
		if foundConsumption {
			break
		}
	}

	// RAPL (Intel Power)
	c.raplMu.Lock()
	defer c.raplMu.Unlock()

	raplBasePaths := []string{config.HostPath("/sys/class/powercap"), "/sys/class/powercap"}
	now := time.Now()
	raplDomains := make(map[string]float64)
	totalWatts := 0.0
	hasNewReading := false

	for _, basePath := range raplBasePaths {
		matches, err := filepath.Glob(filepath.Join(basePath, "intel-rapl:*"))
		if err != nil || len(matches) == 0 {
			continue
		}

		for _, domainPath := range matches {
			nameFile := filepath.Join(domainPath, "name")
			nameBytes, err := os.ReadFile(nameFile)
			if err != nil {
				continue
			}
			name := strings.TrimSpace(string(nameBytes))

			energyFile := filepath.Join(domainPath, "energy_uj")
			maxEnergyFile := filepath.Join(domainPath, "max_energy_range_uj")

			energyBytes, err := os.ReadFile(energyFile)
			if err != nil {
				continue
			}
			energyUj, parseErr := strconv.ParseUint(strings.TrimSpace(string(energyBytes)), 10, 64)
			if parseErr != nil {
				continue
			}

			if lastEnergy, ok := c.raplReadings[domainPath]; ok && !c.raplTime.IsZero() {
				dt := now.Sub(c.raplTime).Seconds()
				if dt > 0 {
					var de uint64
					if energyUj >= lastEnergy {
						de = energyUj - lastEnergy
					} else {
						// Handle counter wrap
						var maxRange uint64
						if maxBytes, err := os.ReadFile(maxEnergyFile); err == nil {
							maxRange, _ = strconv.ParseUint(strings.TrimSpace(string(maxBytes)), 10, 64)
						}
						if maxRange > 0 {
							de = (maxRange - lastEnergy) + energyUj
						} else {
							de = 0
						}
					}

					if de > 0 {
						watts := (float64(de) / 1000000.0) / dt
						if watts < 0 {
							watts = 0
						}
						raplDomains[name] = utils.Round(watts)
						if strings.HasPrefix(strings.ToLower(name), "package") {
							totalWatts += watts
						}
					}
				}
			}

			c.raplReadings[domainPath] = energyUj
			hasNewReading = true
		}
	}

	if hasNewReading {
		c.raplTime = now
	}
	if totalWatts > 0 {
		powerStatus["consumption_watts"] = utils.Round(totalWatts)
	}
	if len(raplDomains) > 0 {
		powerStatus["rapl"] = raplDomains
	}

	return powerStatus
}
