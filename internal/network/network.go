// Package network 提供网络诊断功能
package network

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AnalyseDeCircuit/web-monitor/internal/config"
	"github.com/AnalyseDeCircuit/web-monitor/internal/utils"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
	gopsutilnet "github.com/shirou/gopsutil/v3/net"
)

var tcpStateMap = map[string]string{
	"01": "ESTABLISHED",
	"02": "SYN_SENT",
	"03": "SYN_RECV",
	"04": "FIN_WAIT1",
	"05": "FIN_WAIT2",
	"06": "TIME_WAIT",
	"07": "CLOSE",
	"08": "CLOSE_WAIT",
	"09": "LAST_ACK",
	"0A": "LISTEN",
	"0B": "CLOSING",
}

type procNetSummary struct {
	States        map[string]int
	Sockets       map[string]int
	ListeningPort []types.ListeningPort
}

func readProcNetSummary() (procNetSummary, error) {
	out := procNetSummary{
		States:        make(map[string]int),
		Sockets:       make(map[string]int),
		ListeningPort: make([]types.ListeningPort, 0),
	}

	seenPorts := make(map[string]bool) // key: protocol:port

	procRoot := config.GlobalConfig.HostProc
	if procRoot == "" {
		procRoot = "/proc"
	}

	// TCP
	for _, name := range []string{"tcp", "tcp6"} {
		path := filepath.Join(procRoot, "net", name)
		_ = scanProcNet(path, func(localPort int, stateHex string) {
			out.Sockets["tcp"]++

			stateName := tcpStateMap[strings.ToUpper(stateHex)]
			if stateName == "" {
				stateName = "UNKNOWN"
			}
			out.States[stateName]++
			if stateName == "TIME_WAIT" {
				out.Sockets["tcp_tw"]++
			}
			if stateName == "LISTEN" {
				key := "tcp:" + strconv.Itoa(localPort)
				if !seenPorts[key] {
					seenPorts[key] = true
					out.ListeningPort = append(out.ListeningPort, types.ListeningPort{Port: localPort, Protocol: "tcp"})
				}
			}
		})
	}

	// UDP
	for _, name := range []string{"udp", "udp6"} {
		path := filepath.Join(procRoot, "net", name)
		_ = scanProcNet(path, func(localPort int, _ string) {
			out.Sockets["udp"]++
			out.States["NONE"]++ // keep behavior similar to gopsutil (UDP has status NONE)

			key := "udp:" + strconv.Itoa(localPort)
			if !seenPorts[key] {
				seenPorts[key] = true
				out.ListeningPort = append(out.ListeningPort, types.ListeningPort{Port: localPort, Protocol: "udp"})
			}
		})
	}

	// If we couldn't read anything at all, treat as error.
	if len(out.States) == 0 && len(out.Sockets) == 0 && len(out.ListeningPort) == 0 {
		return procNetSummary{}, os.ErrNotExist
	}

	return out, nil
}

// scanProcNet scans /proc/net/{tcp,tcp6,udp,udp6} style files.
// It calls onEntry(localPort, stateHex) for each row.
func scanProcNet(path string, onEntry func(localPort int, stateHex string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if fields[0] == "sl" {
			continue
		}
		local := fields[1]
		stateHex := fields[3]
		// local_address is like 0100007F:1F90
		parts := strings.Split(local, ":")
		if len(parts) != 2 {
			continue
		}
		portHex := parts[1]
		portU, err := strconv.ParseUint(portHex, 16, 32)
		if err != nil {
			continue
		}
		port := int(portU)
		if port <= 0 {
			continue
		}

		onEntry(port, stateHex)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
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
	if summary, err := readProcNetSummary(); err == nil {
		info.ConnectionStates = summary.States
		info.Sockets = summary.Sockets
		info.ListeningPorts = summary.ListeningPort
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
