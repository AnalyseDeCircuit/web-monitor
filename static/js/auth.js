// Authentication helpers
function checkAuthentication() {
    const token = localStorage.getItem('auth_token');
    if (!token) {
        window.location.href = '/login';
        return false;
    }
    return token;
}

function getAuthToken() {
    return localStorage.getItem('auth_token');
}

async function logout() {
    const token = getAuthToken();
    if (token) {
        try {
            await fetch('/api/logout', {
                method: 'POST',
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            });
        } catch (err) {
            console.error('Logout error:', err);
        }
    }

    try {
        localStorage.removeItem('auth_token');
        localStorage.removeItem('username');
        localStorage.removeItem('role');
    } catch (_) {}

    window.location.href = '/login';
}

// Intercept fetch to attach auth header for API requests
const originalFetch = window.fetch;
window.fetch = function (...args) {
    const token = getAuthToken();
    let options = args[1] || {};
    if (!options.headers) {
        options.headers = {};
    }

    const url = typeof args[0] === 'string'
        ? args[0]
        : (args[0] && args[0].url) ? args[0].url : '';

    if (token && (url.includes('/api/') || url.includes('/ws/'))) {
        options.headers['Authorization'] = `Bearer ${token}`;
    }

    args[1] = options;

    return originalFetch.apply(window, args).then((res) => {
        if (res && res.status === 401) {
            try {
                localStorage.removeItem('auth_token');
                localStorage.removeItem('username');
                localStorage.removeItem('role');
            } catch (_) {}

            if (window.location.pathname !== '/login') {
                window.location.href = '/login';
            }
        }
        return res;
    });
};
