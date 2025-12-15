package websocket

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

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

	updateCh chan time.Duration
	last     atomic.Value // stores types.Response
}

func newDynamicResponseCollector(collect func() types.Response) *dynamicResponseCollector {
	return &dynamicResponseCollector{
		ready:    make(chan struct{}),
		collect:  collect,
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

	mu      sync.Mutex
	subs    int
	running bool
	cancel  context.CancelFunc
	ready   chan struct{}
	last    atomic.Value // stores T
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
	c.subs++
	if c.running {
		return
	}
	c.running = true
	c.ready = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	go func() {
		// Prime immediately.
		c.last.Store(c.collect())
		close(c.ready)

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
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

	mu             sync.Mutex
	clientInterval map[uint64]time.Duration
}

func newStatsHub() *statsHub {
	h := &statsHub{
		base:           newDynamicResponseCollector(collectBaseStats),
		processes:      newConditionalCollector(15*time.Second, collectProcesses),
		netDetail:      newConditionalCollector(15*time.Second, collectNetDetail),
		clientInterval: make(map[uint64]time.Duration),
	}
	// Start base immediately with a sane default.
	h.base.Start(5 * time.Second)
	return h
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
	case "processes":
		h.processes.Subscribe()
	case "net_detail":
		h.netDetail.Subscribe()
	}
}

func (h *statsHub) Unsubscribe(topic string) {
	switch topic {
	case "processes":
		h.processes.Unsubscribe()
	case "net_detail":
		h.netDetail.Unsubscribe()
	}
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
