// Package network 提供网络诊断功能
package network

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	gopsutilnet "github.com/shirou/gopsutil/v3/net"
)

var (
	networkMutex sync.Mutex
)

// PingTarget 对目标进行Ping测试
func PingTarget(target string, count int) (*types.PingResult, error) {
	networkMutex.Lock()
	defer networkMutex.Unlock()

	// 验证目标
	if !ValidateNetworkTarget(target) {
		return nil, fmt.Errorf("invalid target: %s", target)
	}

	cmd := exec.Command("ping", "-c", fmt.Sprintf("%d", count), "-W", "2", target)
	output, err := cmd.CombinedOutput()
	result := &types.PingResult{
		Target:    target,
		Timestamp: time.Now().Format(time.RFC3339),
		Output:    string(output),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
		// 解析输出获取统计信息
		parsePingOutput(result)
	}

	return result, nil
}

// parsePingOutput 解析Ping命令输出
func parsePingOutput(result *types.PingResult) {
	lines := strings.Split(result.Output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 解析统计行
		if strings.Contains(line, "packets transmitted") {
			fields := strings.Fields(line)
			for i, field := range fields {
				switch field {
				case "packets":
					if i > 0 {
						result.PacketsTransmitted = int(parseField(fields[i-1]))
					}
				case "received,":
					if i > 0 {
						result.PacketsReceived = int(parseField(fields[i-1]))
					}
				case "packet":
					if i > 0 && fields[i-1] == "loss" {
						result.PacketLoss = parseField(strings.TrimSuffix(fields[i+1], "%"))
					}
				}
			}
		}

		// 解析时间统计
		if strings.Contains(line, "rtt min/avg/max/mdev") {
			parts := strings.Split(line, "=")
			if len(parts) > 1 {
				times := strings.Fields(parts[1])
				if len(times) > 0 {
					timeParts := strings.Split(times[0], "/")
					if len(timeParts) >= 4 {
						result.MinRTT = parseField(timeParts[0])
						result.AvgRTT = parseField(timeParts[1])
						result.MaxRTT = parseField(timeParts[2])
						result.MdevRTT = parseField(timeParts[3])
					}
				}
			}
		}
	}
}

// parseField 解析字段值
func parseField(s string) float64 {
	var value float64
	fmt.Sscanf(s, "%f", &value)
	return value
}

// TracerouteTarget 对目标进行Traceroute测试
func TracerouteTarget(target string, maxHops int) (*types.TracerouteResult, error) {
	networkMutex.Lock()
	defer networkMutex.Unlock()

	if !ValidateNetworkTarget(target) {
		return nil, fmt.Errorf("invalid target: %s", target)
	}

	cmd := exec.Command("traceroute", "-m", fmt.Sprintf("%d", maxHops), "-w", "2", target)
	output, err := cmd.CombinedOutput()
	result := &types.TracerouteResult{
		Target:    target,
		Timestamp: time.Now().Format(time.RFC3339),
		Output:    string(output),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
		parseTracerouteOutput(result)
	}

	return result, nil
}

// parseTracerouteOutput 解析Traceroute输出
func parseTracerouteOutput(result *types.TracerouteResult) {
	lines := strings.Split(result.Output, "\n")
	var hops []struct {
		IP       string  `json:"ip"`
		Hostname string  `json:"hostname"`
		Latency  float64 `json:"latency"`
	}

	for _, line := range lines[1:] { // 跳过第一行标题
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		var hop struct {
			IP       string  `json:"ip"`
			Hostname string  `json:"hostname"`
			Latency  float64 `json:"latency"`
		}

		// 解析IP地址和主机名
		for i := 1; i < len(fields); i++ {
			field := fields[i]
			if net.ParseIP(field) != nil {
				hop.IP = field
			} else if strings.Contains(field, "(") && strings.Contains(field, ")") {
				// 格式: hostname (IP)
				hop.Hostname = strings.TrimSuffix(strings.TrimPrefix(field, "("), ")")
			} else if !strings.Contains(field, "ms") && !strings.Contains(field, "*") {
				hop.Hostname = field
			}
		}

		// 解析延迟
		for i := 1; i < len(fields); i++ {
			if strings.Contains(fields[i], "ms") {
				hop.Latency = parseField(strings.TrimSuffix(fields[i], "ms"))
				break
			}
		}

		hops = append(hops, hop)
	}

	result.Hops = hops
}

// PortScan 端口扫描
func PortScan(target string, ports []int, timeout time.Duration) (*types.PortScanResult, error) {
	result := &types.PortScanResult{
		Target:    target,
		Timestamp: time.Now().Format(time.RFC3339),
		Ports:     make([]types.PortStatus, 0, len(ports)),
	}

	// 验证目标
	ip := net.ParseIP(target)
	if ip == nil {
		// 尝试解析主机名
		addrs, err := net.LookupHost(target)
		if err != nil || len(addrs) == 0 {
			return nil, fmt.Errorf("invalid target: %s", target)
		}
		target = addrs[0]
	}

	// 并发扫描端口
	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, 10) // 限制并发数

	for _, port := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			status := scanPort(target, p, timeout)
			mu.Lock()
			result.Ports = append(result.Ports, status)
			mu.Unlock()
		}(port)
	}

	wg.Wait()

	// 统计结果
	for _, port := range result.Ports {
		if port.Open {
			result.OpenPorts++
		} else {
			result.ClosedPorts++
		}
	}

	result.Success = true
	return result, nil
}

// scanPort 扫描单个端口
func scanPort(target string, port int, timeout time.Duration) types.PortStatus {
	status := types.PortStatus{
		Port:     port,
		Protocol: "tcp",
	}

	address := fmt.Sprintf("%s:%d", target, port)
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		status.Open = false
		status.Error = err.Error()
	} else {
		status.Open = true
		conn.Close()
	}

	status.Timestamp = time.Now().Format(time.RFC3339)
	return status
}

// DNSLookup DNS查询
func DNSLookup(hostname string, recordType string) (*types.DNSResult, error) {
	result := &types.DNSResult{
		Domain:    hostname,
		Hostname:  hostname,
		Type:      recordType,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	var records []string
	var err error

	switch strings.ToUpper(recordType) {
	case "A":
		addrs, e := net.LookupHost(hostname)
		err = e
		records = addrs
	case "CNAME":
		cname, e := net.LookupCNAME(hostname)
		err = e
		if e == nil {
			records = []string{cname}
		}
	case "MX":
		mxs, e := net.LookupMX(hostname)
		err = e
		if e == nil {
			for _, mx := range mxs {
				records = append(records, fmt.Sprintf("%s (pref=%d)", mx.Host, mx.Pref))
			}
		}
	case "NS":
		nss, e := net.LookupNS(hostname)
		err = e
		if e == nil {
			for _, ns := range nss {
				records = append(records, ns.Host)
			}
		}
	case "TXT":
		txts, e := net.LookupTXT(hostname)
		err = e
		records = txts
	default:
		return nil, fmt.Errorf("unsupported record type: %s", recordType)
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
		result.Records = records
	}

	return result, nil
}

// ValidateNetworkTarget 验证网络目标
func ValidateNetworkTarget(target string) bool {
	if target == "" {
		return false
	}

	// 检查是否是有效的IP地址
	if ip := net.ParseIP(target); ip != nil {
		return true
	}

	// 检查是否是有效的主机名
	if _, err := net.LookupHost(target); err == nil {
		return true
	}

	return false
}

// GetNetworkInfo 获取完整的网络信息
func GetNetworkInfo() (types.NetInfo, error) {
	info := types.NetInfo{}

	// IO Counters
	ioCounters, err := gopsutilnet.IOCounters(false)
	if err == nil && len(ioCounters) > 0 {
		info.RawSent = ioCounters[0].BytesSent
		info.RawRecv = ioCounters[0].BytesRecv

		// Initialize Errors map
		info.Errors = make(map[string]uint64)
		info.Errors["total_errors_in"] = ioCounters[0].Errin
		info.Errors["total_errors_out"] = ioCounters[0].Errout
		info.Errors["total_drops_in"] = ioCounters[0].Dropin
		info.Errors["total_drops_out"] = ioCounters[0].Dropout
	}

	// Connection States
	conns, err := gopsutilnet.Connections("all")
	if err == nil {
		states := make(map[string]int)
		sockets := make(map[string]int)

		for _, conn := range conns {
			states[conn.Status]++
			if conn.Type == 1 { // TCP
				sockets["tcp"]++
			} else if conn.Type == 2 { // UDP
				sockets["udp"]++
			}
			if conn.Status == "TIME_WAIT" {
				sockets["tcp_tw"]++
			}
		}
		info.ConnectionStates = states
		info.Sockets = sockets
	}

	// Interfaces
	ifaces, err := gopsutilnet.Interfaces()
	if err == nil {
		// Convert to map for frontend
		ifaceMap := make(map[string]types.Interface)

		// Get per-interface IO counters
		perIfaceIO, _ := gopsutilnet.IOCounters(true)
		ioMap := make(map[string]gopsutilnet.IOCountersStat)
		for _, io := range perIfaceIO {
			ioMap[io.Name] = io
		}

		for _, iface := range ifaces {
			stats := types.Interface{
				IsUp: false,
			}
			for _, flag := range iface.Flags {
				if flag == "up" {
					stats.IsUp = true
					break
				}
			}

			for _, addr := range iface.Addrs {
				// Simple IP extraction
				stats.IP = addr.Addr
				break
			}

			if io, ok := ioMap[iface.Name]; ok {
				stats.BytesSent = fmt.Sprintf("%d", io.BytesSent)
				stats.BytesRecv = fmt.Sprintf("%d", io.BytesRecv)
				stats.ErrorsIn = io.Errin
				stats.ErrorsOut = io.Errout
				stats.DropsIn = io.Dropin
				stats.DropsOut = io.Dropout
			}

			ifaceMap[iface.Name] = stats
		}
		info.Interfaces = ifaceMap
	}

	// Raw values for calculation
	info.RawSent = info.RawSent // Already uint64
	info.RawRecv = info.RawRecv

	// Format bytes for frontend
	info.BytesSent = fmt.Sprintf("%d", info.RawSent)
	info.BytesRecv = fmt.Sprintf("%d", info.RawRecv)

	// Listening Ports
	info.ListeningPorts = getListeningPorts()

	return info, nil
}

// GetNetworkInterfaces 获取网络接口信息
func GetNetworkInterfaces() ([]types.NetworkInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []types.NetworkInterface
	for _, iface := range interfaces {
		ni := types.NetworkInterface{
			Name: iface.Name,
			MTU:  iface.MTU,
		}

		// 获取接口地址
		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				ni.Addresses = append(ni.Addresses, addr.String())
			}
		}

		// 获取接口标志
		ni.Flags = []string{iface.Flags.String()}

		// 获取硬件地址
		ni.HardwareAddr = iface.HardwareAddr.String()

		result = append(result, ni)
	}

	return result, nil
}

// getListeningPorts 获取监听端口
func getListeningPorts() []types.ListeningPort {
	var ports []types.ListeningPort

	// Try gopsutil first
	conns, err := gopsutilnet.Connections("tcp")
	if err == nil {
		portMap := make(map[int]bool)
		for _, conn := range conns {
			if conn.Status == "LISTEN" {
				port := int(conn.Laddr.Port)
				if port > 0 && !portMap[port] {
					portMap[port] = true
					ports = append(ports, types.ListeningPort{
						Port:     port,
						Protocol: "tcp",
					})
				}
			}
		}

		// Also check UDP
		conns, _ = gopsutilnet.Connections("udp")
		for _, conn := range conns {
			if conn.Laddr.Port > 0 {
				port := int(conn.Laddr.Port)
				// Check if this port is already in TCP
				found := false
				for _, p := range ports {
					if p.Port == port {
						found = true
						break
					}
				}
				if !found {
					ports = append(ports, types.ListeningPort{
						Port:     port,
						Protocol: "udp",
					})
				}
			}
		}

		return ports
	}

	// Fallback: parse /proc/net/tcp manually
	paths := []string{"/hostfs/proc/net/tcp", "/proc/net/tcp"}
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			portMap := make(map[int]bool)

			for scanner.Scan() {
				line := scanner.Text()
				fields := strings.Fields(line)
				if len(fields) < 4 {
					continue
				}

				// Check if LISTEN state (0A in hex)
				if fields[3] != "0A" {
					continue
				}

				// Parse local address field
				localAddr := fields[1]
				parts := strings.Split(localAddr, ":")
				if len(parts) == 2 {
					portHex := parts[1]
					if portVal, err := strconv.ParseInt(portHex, 16, 32); err == nil {
						port := int(portVal)
						if port > 0 && !portMap[port] {
							portMap[port] = true
							ports = append(ports, types.ListeningPort{
								Port:     port,
								Protocol: "tcp",
							})
						}
					}
				}
			}
			break
		}
	}

	return ports
}
