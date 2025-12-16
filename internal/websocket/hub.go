package websocket

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/collectors"
	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

type netDetailSnapshot struct {
	Network  types.NetInfo
	SSHStats types.SSHStats
}

type dynamicResponseCollector struct {
	started sync.Once
	ready   chan struct{}

	collect func() types.Response

	ctx      context.Context
	cancel   context.CancelFunc
	updateCh chan time.Duration
	last     atomic.Value // stores types.Response
}

func newDynamicResponseCollector(collect func() types.Response) *dynamicResponseCollector {
	ctx, cancel := context.WithCancel(context.Background())
	return &dynamicResponseCollector{
		ready:    make(chan struct{}),
		collect:  collect,
		ctx:      ctx,
		cancel:   cancel,
		updateCh: make(chan time.Duration, 1),
	}
}

func (c *dynamicResponseCollector) Start(initialInterval time.Duration) {
	c.started.Do(func() {
		if initialInterval <= 0 {
			initialInterval = 5 * time.Second
		}
		go func() {
			// Prime immediately.
			c.last.Store(c.collect())
			close(c.ready)

			interval := initialInterval
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-c.ctx.Done():
					log.Println("dynamicResponseCollector: shutting down")
					return
				case <-ticker.C:
					c.last.Store(c.collect())
				case d := <-c.updateCh:
					if d <= 0 {
						continue
					}
					if d == interval {
						continue
					}
					ticker.Stop()
					interval = d
					ticker = time.NewTicker(interval)
				}
			}
		}()
	})
}

func (c *dynamicResponseCollector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *dynamicResponseCollector) SetInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	// Keep only the latest interval request.
	select {
	case <-c.updateCh:
	default:
	}
	select {
	case c.updateCh <- d:
	default:
	}
}

func (c *dynamicResponseCollector) WaitReady(timeout time.Duration) {
	if timeout <= 0 {
		<-c.ready
		return
	}
	select {
	case <-c.ready:
	case <-time.After(timeout):
	}
}

func (c *dynamicResponseCollector) Latest() (types.Response, bool) {
	v := c.last.Load()
	if v == nil {
		return types.Response{}, false
	}
	stats, ok := v.(types.Response)
	return stats, ok
}

type conditionalCollector[T any] struct {
	interval time.Duration
	collect  func() T

	mu       sync.Mutex
	subs     int
	running  bool
	ctx      context.Context
	cancel   context.CancelFunc
	ready    chan struct{}
	last     atomic.Value // stores T
	shutdown bool         // permanent shutdown flag
}

func newConditionalCollector[T any](interval time.Duration, collect func() T) *conditionalCollector[T] {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &conditionalCollector[T]{
		interval: interval,
		collect:  collect,
	}
}

func (c *conditionalCollector[T]) Subscribe() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shutdown {
		return
	}
	c.subs++
	if c.running {
		return
	}
	c.running = true
	c.ready = make(chan struct{})
	c.ctx, c.cancel = context.WithCancel(context.Background())

	go func() {
		// Prime immediately.
		c.last.Store(c.collect())
		close(c.ready)

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-c.ctx.Done():
				log.Println("conditionalCollector: shutting down")
				return
			case <-ticker.C:
				c.last.Store(c.collect())
			}
		}
	}()
}

func (c *conditionalCollector[T]) Unsubscribe() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subs > 0 {
		c.subs--
	}
	if c.subs != 0 {
		return
	}
	if !c.running {
		return
	}
	c.running = false
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
}

func (c *conditionalCollector[T]) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	c.subs = 0
	if c.running {
		c.running = false
		if c.cancel != nil {
			c.cancel()
			c.cancel = nil
		}
	}
}

func (c *conditionalCollector[T]) WaitReady(timeout time.Duration) {
	c.mu.Lock()
	ready := c.ready
	running := c.running
	c.mu.Unlock()
	if !running || ready == nil {
		return
	}
	if timeout <= 0 {
		<-ready
		return
	}
	select {
	case <-ready:
	case <-time.After(timeout):
	}
}

func (c *conditionalCollector[T]) Latest() (T, bool) {
	v := c.last.Load()
	if v == nil {
		var zero T
		return zero, false
	}
	vv, ok := v.(T)
	return vv, ok
}

type statsHub struct {
	base *dynamicResponseCollector

	processes *conditionalCollector[[]types.ProcessInfo]
	netDetail *conditionalCollector[netDetailSnapshot]

	// 并行采集器
	aggregator       *collectors.StatsAggregator
	processCollector *collectors.ProcessCollector
	netDetailColl    *collectors.NetworkDetailCollector
	sshCollector     *collectors.SSHCollector

	mu             sync.Mutex
	clientInterval map[uint64]time.Duration
	shutdown       bool
}

func newStatsHub() *statsHub {
	aggregator := collectors.NewStatsAggregator()
	processCollector := collectors.NewProcessCollector()
	netDetailColl := collectors.NewNetworkDetailCollector()
	sshCollector := collectors.NewSSHCollector()

	h := &statsHub{
		aggregator:       aggregator,
		processCollector: processCollector,
		netDetailColl:    netDetailColl,
		sshCollector:     sshCollector,
		clientInterval:   make(map[uint64]time.Duration),
	}

	// 使用新的并行采集器
	h.base = newDynamicResponseCollector(aggregator.CollectBaseStats)
	h.processes = newConditionalCollector(15*time.Second, func() []types.ProcessInfo {
		if data, ok := processCollector.Collect(context.Background()).([]types.ProcessInfo); ok {
			return data
		}
		return []types.ProcessInfo{}
	})
	h.netDetail = newConditionalCollector(15*time.Second, func() netDetailSnapshot {
		out := netDetailSnapshot{
			Network: types.NetInfo{
				Interfaces:       map[string]types.Interface{},
				Sockets:          map[string]int{},
				ConnectionStates: map[string]int{},
				Errors:           map[string]uint64{},
				ListeningPorts:   []types.ListeningPort{},
			},
			SSHStats: types.SSHStats{
				Sessions:         []types.SSHSession{},
				AuthMethods:      map[string]int{},
				OOMRiskProcesses: []types.ProcessInfo{},
			},
		}

		if data, ok := netDetailColl.Collect(context.Background()).(collectors.NetworkDetailData); ok {
			out.Network.Interfaces = data.Interfaces
			out.Network.Sockets = data.Sockets
			out.Network.ConnectionStates = data.ConnectionStates
			out.Network.Errors = data.Errors
			out.Network.ListeningPorts = data.ListeningPorts
		}

		if sshData, ok := sshCollector.Collect(context.Background()).(types.SSHStats); ok {
			out.SSHStats = sshData
		}

		return out
	})

	// Start base immediately with a sane default.
	h.base.Start(5 * time.Second)
	return h
}

// Shutdown gracefully stops all collectors
func (h *statsHub) Shutdown() {
	h.mu.Lock()
	if h.shutdown {
		h.mu.Unlock()
		return
	}
	h.shutdown = true
	h.mu.Unlock()

	log.Println("statsHub: shutting down all collectors")
	h.base.Stop()
	h.processes.Stop()
	h.netDetail.Stop()
}

func (h *statsHub) RegisterClient(id uint64, interval time.Duration) {
	interval = clampInterval(interval)
	h.mu.Lock()
	h.clientInterval[id] = interval
	min := minIntervalLocked(h.clientInterval)
	h.mu.Unlock()
	h.base.SetInterval(min)
}

func (h *statsHub) UnregisterClient(id uint64) {
	h.mu.Lock()
	delete(h.clientInterval, id)
	min := minIntervalLocked(h.clientInterval)
	h.mu.Unlock()
	if min > 0 {
		h.base.SetInterval(min)
	}
}

func (h *statsHub) Subscribe(topic string) {
	switch topic {
	case "processes", "top_processes":
		// Both share the same collector - top_processes just returns fewer items
		h.processes.Subscribe()
	case "net_detail":
		h.netDetail.Subscribe()
	}
}

func (h *statsHub) Unsubscribe(topic string) {
	switch topic {
	case "processes", "top_processes":
		h.processes.Unsubscribe()
	case "net_detail":
		h.netDetail.Unsubscribe()
	}
}

// LatestTopProcesses returns top N processes from the shared collector.
func (h *statsHub) LatestTopProcesses(n int) ([]types.ProcessInfo, bool) {
	procs, ok := h.processes.Latest()
	if !ok || len(procs) == 0 {
		return nil, false
	}
	if len(procs) > n {
		return procs[:n], true
	}
	return procs, true
}

func (h *statsHub) WaitReady(timeout time.Duration) {
	h.base.WaitReady(timeout)
}

func (h *statsHub) LatestBase() (types.Response, bool) {
	return h.base.Latest()
}

func (h *statsHub) LatestProcesses() ([]types.ProcessInfo, bool) {
	return h.processes.Latest()
}

func (h *statsHub) LatestNetDetail() (netDetailSnapshot, bool) {
	return h.netDetail.Latest()
}

func clampInterval(d time.Duration) time.Duration {
	if d < 2*time.Second {
		d = 2 * time.Second
	}
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

func minIntervalLocked(m map[uint64]time.Duration) time.Duration {
	min := time.Duration(0)
	for _, d := range m {
		if d <= 0 {
			continue
		}
		if min == 0 || d < min {
			min = d
		}
	}
	if min == 0 {
		min = 5 * time.Second
	}
	return min
}
