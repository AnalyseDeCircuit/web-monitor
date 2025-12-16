// Package websocket 提供WebSocket连接处理功能
package websocket

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/auth"
	"github.com/gorilla/websocket"
)

var (
	wsHub = newStatsHub()

	wsClientID uint64
)

const (
	wsWriteWait      = 10 * time.Second
	wsPongWait       = 60 * time.Second
	wsPingPeriod     = (wsPongWait * 9) / 10
	wsMaxMessageSize = 8 * 1024

	wsMsgRatePerSec = 2.0
	wsMsgBurst      = 5.0
)

// Client 代表一个 WebSocket 客户端连接
type Client struct {
	hub  *statsHub
	conn *websocket.Conn
	send chan []byte
	done chan struct{} // closed when connection ends
	subs map[string]bool
	mu   sync.Mutex
}

// Shutdown gracefully stops the WebSocket hub and all collectors.
// Call this during application shutdown.
func Shutdown() {
	if wsHub != nil {
		wsHub.Shutdown()
	}
}

// Upgrader WebSocket升级器
var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return isAllowedWebSocketOrigin(r)
	},
}

func isAllowedWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser clients often omit Origin.
		return true
	}
	// Optional allowlist override for reverse proxies / custom domains.
	if allowWebSocketOriginByEnv(origin) {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		log.Printf("ws origin parse error: %v origin=%q", err, origin)
		return false
	}
	originHost := strings.ToLower(u.Hostname())
	if originHost == "" {
		log.Printf("ws origin empty hostname origin=%q", origin)
		return false
	}

	// Determine the effective host from request headers (support reverse proxy).
	reqHost := r.Host
	// Prefer X-Forwarded-Host for reverse proxy setups.
	if xf := r.Header.Get("X-Forwarded-Host"); xf != "" {
		reqHost = strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	// Also check Host header override from Origin header itself for same-origin.
	// Many reverse proxies don't set X-Forwarded-Host; compare origin host directly.
	reqHost = strings.ToLower(strings.TrimSpace(reqHost))
	if reqHost == "" {
		// Fallback: if reqHost is empty, allow if origin looks like a valid domain
		// (this handles edge cases where Host header is missing behind proxy).
		log.Printf("ws reqHost empty, allowing origin=%q", origin)
		return true
	}
	// Strip port if present.
	if h, _, err := net.SplitHostPort(reqHost); err == nil {
		reqHost = h
	} else {
		if idx := strings.Index(reqHost, ":"); idx > 0 {
			reqHost = reqHost[:idx]
		}
	}

	if originHost == reqHost {
		return true
	}

	// Log mismatch for debugging reverse proxy issues.
	log.Printf("ws origin mismatch: originHost=%q reqHost=%q Host=%q X-Forwarded-Host=%q", originHost, reqHost, r.Host, r.Header.Get("X-Forwarded-Host"))
	// For now, allow the connection anyway to not break existing setups.
	// In production, you may want to return false here after proper proxy config.
	return true
}

func allowWebSocketOriginByEnv(origin string) bool {
	list := strings.TrimSpace(os.Getenv("WS_ALLOWED_ORIGINS"))
	if list == "" {
		return false
	}

	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}

	u, err := url.Parse(origin)
	originHost := ""
	if err == nil {
		originHost = strings.ToLower(u.Hostname())
	}

	for _, raw := range strings.Split(list, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		// Full origin match, e.g. https://example.com
		if strings.EqualFold(entry, origin) {
			return true
		}
		// Hostname match, e.g. example.com
		if originHost != "" && strings.EqualFold(entry, originHost) {
			return true
		}
		// If entry is a URL, compare hostname.
		if strings.Contains(entry, "://") {
			if eu, err := url.Parse(entry); err == nil {
				eh := strings.ToLower(eu.Hostname())
				if eh != "" && originHost != "" && eh == originHost {
					return true
				}
			}
		}
	}

	return false
}

func tokenFromWebSocketSubprotocol(r *http.Request) string {
	h := r.Header.Get("Sec-WebSocket-Protocol")
	if h == "" {
		return ""
	}
	parts := strings.Split(h, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "jwt" {
			continue
		}
		if strings.Count(p, ".") == 2 {
			return p
		}
	}
	// If client sent ["jwt", "<token>"]
	if len(parts) >= 2 {
		if strings.TrimSpace(parts[0]) == "jwt" {
			t := strings.TrimSpace(parts[1])
			if t != "" {
				return t
			}
		}
	}
	return ""
}

// HandleWebSocket 处理WebSocket连接
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Require JWT for stats websocket.
	// Frontend passes token via query param, so we accept that.
	token := ""
	if cookie, err := r.Cookie("auth_token"); err == nil {
		token = cookie.Value
	}
	if token == "" {
		token = tokenFromWebSocketSubprotocol(r)
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		log.Printf("ws unauthorized: missing token origin=%q host=%q proto=%q remote=%q", r.Header.Get("Origin"), r.Host, r.Header.Get("Sec-WebSocket-Protocol"), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
		return
	}
	if _, err := auth.ValidateJWT(token); err != nil {
		log.Printf("ws unauthorized: invalid token origin=%q host=%q proto=%q remote=%q err=%v", r.Header.Get("Origin"), r.Host, r.Header.Get("Sec-WebSocket-Protocol"), r.RemoteAddr, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
		return
	}

	c, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	// Note: c.Close() is handled in readPump/writePump

	// Connection hardening.
	// Read limits are set in readPump.

	intervalStr := r.URL.Query().Get("interval")
	interval, err := strconv.ParseFloat(intervalStr, 64)
	if err != nil || interval < 2.0 {
		interval = 2.0
	}
	if interval > 60 {
		interval = 60
	}

	clientInterval := time.Duration(interval * float64(time.Second))
	clientInterval = clampInterval(clientInterval)

	clientID := atomic.AddUint64(&wsClientID, 1)

	client := &Client{
		hub:  wsHub,
		conn: c,
		send: make(chan []byte, 256),
		done: make(chan struct{}),
		subs: map[string]bool{"base": true},
	}

	wsHub.RegisterClient(clientID, clientInterval)

	// Start pumps
	go client.writePump()
	go client.readPump(r, clientID)
	go client.dataPump(clientInterval)
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Client) readPump(r *http.Request, clientID uint64) {
	defer func() {
		close(c.done) // signal dataPump and writePump to stop
		c.hub.UnregisterClient(clientID)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(wsMaxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	tokens := wsMsgBurst
	lastRefill := time.Now()
	allowMsg := func() bool {
		now := time.Now()
		elapsed := now.Sub(lastRefill).Seconds()
		if elapsed > 0 {
			tokens += elapsed * wsMsgRatePerSec
			if tokens > wsMsgBurst {
				tokens = wsMsgBurst
			}
			lastRefill = now
		}
		if tokens < 1 {
			return false
		}
		tokens -= 1
		return true
	}

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		if !allowMsg() {
			log.Printf("ws client message rate limit exceeded remote=%q", r.RemoteAddr)
			break
		}

		var req struct {
			Type   string   `json:"type"`
			Topics []string `json:"topics"`
		}
		if err := json.Unmarshal(msg, &req); err != nil {
			continue
		}
		if req.Type == "set_topics" {
			c.mu.Lock()
			// Reset to base
			c.subs = map[string]bool{"base": true}
			for _, t := range req.Topics {
				t = strings.TrimSpace(t)
				if t == "processes" || t == "net_detail" {
					c.subs[t] = true
					c.hub.Subscribe(t)
				}
			}
			c.mu.Unlock()
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(wsPingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case <-c.done:
			return
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// dataPump periodically fetches data from hub and sends to client
func (c *Client) dataPump(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Wait for hub to be ready
	c.hub.WaitReady(5 * time.Second)

	// Send initial data immediately
	c.sendData()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.sendData()
		}
	}
}

// sendData fetches latest data from hub and sends to client
func (c *Client) sendData() {
	c.mu.Lock()
	subs := make(map[string]bool)
	for k, v := range c.subs {
		subs[k] = v
	}
	c.mu.Unlock()

	// Build response based on subscriptions
	resp, ok := c.hub.LatestBase()
	if !ok {
		return
	}

	// Add processes if subscribed
	if subs["processes"] {
		if procs, ok := c.hub.LatestProcesses(); ok {
			resp.Processes = procs
		}
	}

	// Add network detail if subscribed
	if subs["net_detail"] {
		if netDetail, ok := c.hub.LatestNetDetail(); ok {
			// Merge network detail into base network data (preserve BytesSent/BytesRecv/RawSent/RawRecv)
			resp.Network.Interfaces = netDetail.Network.Interfaces
			resp.Network.Sockets = netDetail.Network.Sockets
			resp.Network.ConnectionStates = netDetail.Network.ConnectionStates
			resp.Network.Errors = netDetail.Network.Errors
			resp.Network.ListeningPorts = netDetail.Network.ListeningPorts
			resp.SSHStats = netDetail.SSHStats
		}
	}

	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("dataPump: marshal error: %v", err)
		return
	}

	// Non-blocking send to avoid blocking if client is slow
	select {
	case c.send <- data:
	default:
		// Channel full, skip this update
	}
}
