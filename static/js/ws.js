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
    const useQueryToken = !!options.useQueryToken;
    const allowFallback = options.allowFallback !== false;

    if (websocket) {
        websocket.onclose = null;
        websocket.close();
    }
    if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
    }

    const token = getAuthToken();
    if (!token) {
        console.warn('No auth token, redirecting to login');
        window.location.href = '/login';
        return;
    }

    const interval = document.getElementById('interval-select').value;
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    let wsUrl = `${protocol}//${window.location.host}/ws/stats?interval=${interval}`;
    if (useQueryToken) {
        wsUrl += `&token=${encodeURIComponent(token)}`;
    }

    websocket = useQueryToken ? new WebSocket(wsUrl) : new WebSocket(wsUrl, ['jwt', token]);
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

        if (!opened && allowFallback && !useQueryToken) {
            console.warn('WebSocket closed before open; retrying with token in query string');
            connectWebSocket({ useQueryToken: true, allowFallback: false });
            return;
        }

        reconnectTimer = setTimeout(() => connectWebSocket({ useQueryToken, allowFallback: false }), 3000);
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
