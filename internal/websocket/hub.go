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

// statsHub manages the streaming aggregator and topic-based collectors
type statsHub struct {
	// Streaming aggregator for base metrics (CPU, Memory, Disk, etc.)
	aggregator *collectors.StatsAggregator

	// Topic-based collectors for on-demand data
	processes *conditionalCollector[[]types.ProcessInfo]
	netDetail *conditionalCollector[netDetailSnapshot]

	processCollector *collectors.ProcessCollector
	netDetailColl    *collectors.NetworkDetailCollector
	sshCollector     *collectors.SSHCollector

	mu             sync.Mutex
	clientInterval map[uint64]time.Duration
	shutdown       bool
	ready          chan struct{}
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
		ready:            make(chan struct{}),
	}

	// Topic-based collectors (only run when subscribed)
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

	// Start the streaming aggregator (all collectors run independently)
	aggregator.Start()

	// Mark as ready after a short delay for initial data
	go func() {
		time.Sleep(500 * time.Millisecond)
		close(h.ready)
	}()

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

	log.Println("statsHub: shutting down all collectors...")

	log.Println("statsHub: stopping aggregator...")
	h.aggregator.Stop()
	log.Println("statsHub: aggregator stopped")

	log.Println("statsHub: stopping processes collector...")
	h.processes.Stop()
	log.Println("statsHub: processes collector stopped")

	log.Println("statsHub: stopping netDetail collector...")
	h.netDetail.Stop()
	log.Println("statsHub: netDetail collector stopped")

	log.Println("statsHub: all collectors have been stopped successfully")
}

func (h *statsHub) RegisterClient(id uint64, interval time.Duration) {
	interval = clampInterval(interval)
	h.mu.Lock()
	h.clientInterval[id] = interval
	min := minIntervalLocked(h.clientInterval)
	h.mu.Unlock()
	h.aggregator.SetInterval(min)
}

func (h *statsHub) UnregisterClient(id uint64) {
	h.mu.Lock()
	delete(h.clientInterval, id)
	min := minIntervalLocked(h.clientInterval)
	h.mu.Unlock()
	if min > 0 {
		h.aggregator.SetInterval(min)
	}
}

func (h *statsHub) Subscribe(topic string) {
	switch topic {
	case "processes", "top_processes":
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

// LatestTopProcesses returns top N processes from the shared collector
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
	if timeout <= 0 {
		<-h.ready
		return
	}
	select {
	case <-h.ready:
	case <-time.After(timeout):
	}
}

// LatestBase returns the latest merged stats (non-blocking, instant)
func (h *statsHub) LatestBase() (types.Response, bool) {
	return h.aggregator.CollectBaseStats(), true
}

func (h *statsHub) LatestProcesses() ([]types.ProcessInfo, bool) {
	return h.processes.Latest()
}

func (h *statsHub) LatestNetDetail() (netDetailSnapshot, bool) {
	return h.netDetail.Latest()
}

// ========== Conditional Collector (unchanged) ==========

type conditionalCollector[T any] struct {
	interval time.Duration
	collect  func() T

	mu       sync.Mutex
	subs     int
	running  bool
	ctx      context.Context
	cancel   context.CancelFunc
	ready    chan struct{}
	last     atomic.Value
	shutdown bool
	wg       sync.WaitGroup
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

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
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
	c.shutdown = true
	c.subs = 0
	var running bool
	var cancel context.CancelFunc
	if c.running {
		running = true
		c.running = false
		cancel = c.cancel
		c.cancel = nil
	}
	c.mu.Unlock()

	if running && cancel != nil {
		cancel()
		c.wg.Wait()
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

// ========== Helpers ==========

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
