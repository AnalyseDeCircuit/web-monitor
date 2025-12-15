package websocket

import (
"context"
"encoding/json"
"log"
"sync"
"sync/atomic"
"time"

"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
"github.com/gorilla/websocket"
)

// Client represents a connected WebSocket client.
type Client struct {
	hub  *statsHub
	conn *websocket.Conn
	send chan []byte
	subs map[string]bool
	mu   sync.Mutex
}

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

func (c *dynamicResponseCollector) Start(initialInterval time.Duration, onUpdate func()) {
	c.started.Do(func() {
		if initialInterval <= 0 {
			initialInterval = 5 * time.Second
		}
		go func() {
			// Prime immediately.
			c.last.Store(c.collect())
			close(c.ready)
			if onUpdate != nil {
				onUpdate()
			}

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
					if onUpdate != nil {
						onUpdate()
					}
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

	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan struct{}

	mu             sync.Mutex
	clientInterval map[uint64]time.Duration
	shutdown       bool
}

func newStatsHub() *statsHub {
	h := &statsHub{
		base:           newDynamicResponseCollector(collectBaseStats),
		processes:      newConditionalCollector(15*time.Second, collectProcesses),
		netDetail:      newConditionalCollector(15*time.Second, collectNetDetail),
		clients:        make(map[*Client]bool),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		broadcast:      make(chan struct{}, 1),
		clientInterval: make(map[uint64]time.Duration),
	}
	// Start base immediately with a sane default.
	// Pass a callback to trigger broadcast on update.
	h.base.Start(5*time.Second, func() {
		select {
		case h.broadcast <- struct{}{}:
		default:
		}
	})
	go h.run()
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

func (h *statsHub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			// Update interval
			h.updateInterval()
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.updateInterval()
			}
		case <-h.broadcast:
			h.broadcastSnapshot()
		}
	}
}

func (h *statsHub) updateInterval() {
	// Re-calculate min interval based on registered clients (if we tracked their interval in Client struct)
	// For now, we rely on the external RegisterClient/UnregisterClient calls to update h.clientInterval
	// But since we moved client management here, we should probably move interval tracking here too.
	// However, to minimize changes, we'll keep using h.clientInterval which is updated by RegisterClient.
// Wait, RegisterClient is called by HandleWebSocket.
}

func (h *statsHub) broadcastSnapshot() {
base, ok := h.base.Latest()
if !ok {
return
}

// Pre-calculate payloads for 4 combinations:
// 0: Base
// 1: Base + Procs
// 2: Base + Net
// 3: Base + Procs + Net
payloads := make([][]byte, 4)

// Helper to merge and marshal
getPayload := func(idx int) []byte {
if payloads[idx] != nil {
return payloads[idx]
}
// Clone base to avoid modifying shared state (though we are marshaling immediately)
// Actually, we need a deep copy if we modify slices/maps.
// But here we are just assigning to fields.
// types.Response is a struct. Assigning it copies the struct, but slices/maps are references.
// We must be careful not to modify the underlying arrays of 'base'.
// The collectors return fresh data or cached data.
// We are only assigning to .Processes and .Network fields which are slices/maps.
// It is safe to assign them to a copy of the struct for marshaling.

data := base // struct copy

if idx&1 != 0 { // Bit 0: Procs
if procs, ok := h.processes.Latest(); ok {
data.Processes = procs
}
}
if idx&2 != 0 { // Bit 1: Net
if nd, ok := h.netDetail.Latest(); ok {
data.Network.Interfaces = nd.Network.Interfaces
data.Network.Sockets = nd.Network.Sockets
data.Network.ConnectionStates = nd.Network.ConnectionStates
data.Network.Errors = nd.Network.Errors
data.Network.ListeningPorts = nd.Network.ListeningPorts
data.SSHStats = nd.SSHStats
}
}

b, err := json.Marshal(data)
if err != nil {
log.Printf("broadcast marshal error: %v", err)
return nil
}
payloads[idx] = b
return b
}

for client := range h.clients {
client.mu.Lock()
wantProcs := client.subs["processes"]
wantNet := client.subs["net_detail"]
client.mu.Unlock()

idx := 0
if wantProcs {
idx |= 1
}
if wantNet {
idx |= 2
}

payload := getPayload(idx)
if payload == nil {
continue
}

select {
case client.send <- payload:
default:
// Backpressure: drop message if client is too slow
// log.Printf("ws client slow, dropping frame")
}
}
}

func (h *statsHub) RegisterClient(client *Client, id uint64, interval time.Duration) {
interval = clampInterval(interval)
h.mu.Lock()
h.clientInterval[id] = interval
min := minIntervalLocked(h.clientInterval)
h.mu.Unlock()
h.base.SetInterval(min)
h.register <- client
}

func (h *statsHub) UnregisterClient(client *Client, id uint64) {
h.mu.Lock()
delete(h.clientInterval, id)
min := minIntervalLocked(h.clientInterval)
h.mu.Unlock()
if min > 0 {
h.base.SetInterval(min)
}
h.unregister <- client
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
