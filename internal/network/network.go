// Package network 提供网络诊断功能
package network

import (
	"net"

	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	gopsutilnet "github.com/shirou/gopsutil/v3/net"
)

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
				// 与旧版行为保持一致，使用可读的容量字符串
				stats.BytesSent = utils.GetSize(io.BytesSent)
				stats.BytesRecv = utils.GetSize(io.BytesRecv)
				stats.ErrorsIn = io.Errin
				stats.ErrorsOut = io.Errout
				stats.DropsIn = io.Dropin
				stats.DropsOut = io.Dropout
			}

			ifaceMap[iface.Name] = stats
		}
		info.Interfaces = ifaceMap
	}

	// Format bytes for frontend（与旧版保持一致，返回可读字符串）
	info.BytesSent = utils.GetSize(info.RawSent)
	info.BytesRecv = utils.GetSize(info.RawRecv)

	// Listening Ports
	if err == nil {
		info.ListeningPorts = getListeningPorts(conns)
	} else {
		info.ListeningPorts = []types.ListeningPort{}
	}

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
func getListeningPorts(conns []gopsutilnet.ConnectionStat) []types.ListeningPort {
	var ports []types.ListeningPort
	seen := make(map[int]bool)

	for _, conn := range conns {
		port := int(conn.Laddr.Port)
		if port <= 0 {
			continue
		}

		// TCP Listen
		if conn.Type == 1 && conn.Status == "LISTEN" {
			if !seen[port] {
				seen[port] = true
				ports = append(ports, types.ListeningPort{
					Port:     port,
					Protocol: "tcp",
				})
			}
		}
	}

	// UDP (Type 2) - UDP sockets are stateless, so just list them
	for _, conn := range conns {
		port := int(conn.Laddr.Port)
		if port <= 0 {
			continue
		}
		if conn.Type == 2 {
			if !seen[port] {
				seen[port] = true
				ports = append(ports, types.ListeningPort{
					Port:     port,
					Protocol: "udp",
				})
			}
		}
	}

	return ports
}
