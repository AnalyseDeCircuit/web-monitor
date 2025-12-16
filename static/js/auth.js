// Authentication helpers
async function checkAuthentication() {
    // Auth is cookie-based (HttpOnly). JS cannot read it.
    // Validate by calling a lightweight authenticated endpoint.
    try {
        const res = await fetch('/api/info', { cache: 'no-store' });
        if (res && res.status === 401) {
            window.location.href = '/login';
            return false;
        }
        return true;
    } catch (err) {
        // Network errors should not hard-redirect; allow UI to load.
        console.warn('Auth check failed (network?):', err);
        return true;
    }
}

// Backward compatibility: token-based auth is disabled.
function getAuthToken() {
    return null;
}

async function logout() {
    try {
        await fetch('/api/logout', {
            method: 'POST',
        });
    } catch (err) {
        console.error('Logout error:', err);
    }

    try {
        localStorage.removeItem('username');
        localStorage.removeItem('role');
    } catch (_) {}

    window.location.href = '/login';
}

// Intercept fetch to attach auth header for API requests
const originalFetch = window.fetch;
window.fetch = function (...args) {
    let options = args[1] || {};
    if (!options.headers) {
        options.headers = {};
    }

    const url = typeof args[0] === 'string'
        ? args[0]
        : (args[0] && args[0].url) ? args[0].url : '';

    // Ensure same-origin requests carry cookies (default, but keep explicit).
    try {
        const resolved = new URL(url || '', window.location.href);
        if (resolved.origin === window.location.origin) {
            if (!options.credentials) {
                options.credentials = 'same-origin';
            }
        }
    } catch (_) {
        // If URL parsing fails (e.g. empty), do nothing.
    }

    args[1] = options;

    return originalFetch.apply(window, args).then((res) => {
        if (res && res.status === 401) {
            try {
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
