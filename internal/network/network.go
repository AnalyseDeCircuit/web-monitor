// Package network 提供网络诊断功能
package network

import (
	"bufio"
	"net"
	"os"
	"strconv"
	"strings"

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
