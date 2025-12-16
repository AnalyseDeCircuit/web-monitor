let websocket = null;
let reconnectTimer = null;
let currentTopics = ['top_processes', 'net_detail']; // Default: lightweight subscription

/**
 * Update WebSocket topic subscriptions dynamically.
 * Call this when switching pages to optimize data transfer.
 * @param {string[]} topics - Array of topics to subscribe to.
 *   - 'top_processes': Only top 10 processes (lightweight, for General page)
 *   - 'processes': Full process list (for Processes/Memory pages)
 *   - 'net_detail': Network connections detail
 */
function updateWebSocketTopics(topics) {
    currentTopics = topics;
    if (websocket && websocket.readyState === WebSocket.OPEN) {
        try {
            websocket.send(JSON.stringify({ type: 'set_topics', topics: topics }));
            console.log('WebSocket topics updated:', topics);
        } catch (e) {
            console.warn('Failed to update WS topics:', e);
        }
    }
}

function connectWebSocket(options = {}) {
    if (websocket) {
        websocket.onclose = null;
        websocket.close();
    }
    if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
    }

    const interval = document.getElementById('interval-select').value;
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/stats?interval=${interval}`;

    // Cookie-based auth (HttpOnly). Browser will include same-origin cookies automatically.
    websocket = new WebSocket(wsUrl);
    const statusDot = document.getElementById('status-dot');

    let opened = false;

    websocket.onopen = function () {
        console.log('WebSocket connected');
        opened = true;
        statusDot.classList.add('connected');

        // Subscribe to topics based on current page context
        try {
            websocket.send(JSON.stringify({ type: 'set_topics', topics: currentTopics }));
        } catch (e) {
            console.warn('Failed to send WS subscription message:', e);
        }
    };

    websocket.onmessage = function (event) {
        try {
            const data = JSON.parse(event.data);
            renderStats(data);
        } catch (e) {
            console.error('Error processing message:', e);
        }
    };

    websocket.onclose = function () {
        console.log('WebSocket closed');
        statusDot.classList.remove('connected');

        reconnectTimer = setTimeout(() => connectWebSocket(), 3000);
    };

    websocket.onerror = function (err) {
        console.error('WebSocket error:', err);
        statusDot.classList.remove('connected');
        websocket.close();
    };
}

function changeInterval() {
    connectWebSocket();
}
