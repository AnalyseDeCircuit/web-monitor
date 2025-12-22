package collectors

import (
	"context"

	"github.com/AnalyseDeCircuit/opskernel/internal/network"
	"github.com/AnalyseDeCircuit/opskernel/internal/utils"
	"github.com/AnalyseDeCircuit/opskernel/pkg/types"
	gopsutilnet "github.com/shirou/gopsutil/v3/net"
)

// NetworkCollector 采集网络基础指标
type NetworkCollector struct{}

// NetworkData 包含网络采集结果
type NetworkData struct {
	BytesSent string
	BytesRecv string
	RawSent   uint64
	RawRecv   uint64
}

// NewNetworkCollector 创建网络采集器
func NewNetworkCollector() *NetworkCollector {
	return &NetworkCollector{}
}

func (c *NetworkCollector) Name() string {
	return "network"
}

func (c *NetworkCollector) Collect(ctx context.Context) interface{} {
	data := NetworkData{}

	netIO, _ := gopsutilnet.IOCounters(false)
	if len(netIO) > 0 {
		data.BytesSent = utils.GetSize(netIO[0].BytesSent)
		data.BytesRecv = utils.GetSize(netIO[0].BytesRecv)
		data.RawSent = netIO[0].BytesSent
		data.RawRecv = netIO[0].BytesRecv
	}

	return data
}

// NetworkDetailCollector 采集网络详细信息（接口、连接状态等）
type NetworkDetailCollector struct{}

// NetworkDetailData 包含网络详细采集结果
type NetworkDetailData struct {
	Interfaces       map[string]types.Interface
	Sockets          map[string]int
	ConnectionStates map[string]int
	Errors           map[string]uint64
	ListeningPorts   []types.ListeningPort
}

// NewNetworkDetailCollector 创建网络详情采集器
func NewNetworkDetailCollector() *NetworkDetailCollector {
	return &NetworkDetailCollector{}
}

func (c *NetworkDetailCollector) Name() string {
	return "net_detail"
}

func (c *NetworkDetailCollector) Collect(ctx context.Context) interface{} {
	data := NetworkDetailData{
		Interfaces:       map[string]types.Interface{},
		Sockets:          map[string]int{},
		ConnectionStates: map[string]int{},
		Errors:           map[string]uint64{},
		ListeningPorts:   []types.ListeningPort{},
	}

	if netInfo, err := network.GetNetworkInfo(); err == nil {
		data.Interfaces = netInfo.Interfaces
		data.Sockets = netInfo.Sockets
		data.ConnectionStates = netInfo.ConnectionStates
		data.Errors = netInfo.Errors
		data.ListeningPorts = netInfo.ListeningPorts
	}

	return data
}
