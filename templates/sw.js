const CACHE_NAME = 'opskernel-no-cache-v1';
const urlsToCache = [];

// Installation event - cache files
self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => {
        return cache.addAll(urlsToCache);
      })
      .catch(err => console.log('Cache installation error:', err))
  );
  self.skipWaiting();
});

// Activation event - clean up old caches
self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(cacheNames => {
      return Promise.all(
        cacheNames.map(cacheName => {
          if (cacheName !== CACHE_NAME) {
            return caches.delete(cacheName);
          }
        })
      );
    })
  );
  self.clients.claim();
});

// Fetch event - pass-through (no caching).
self.addEventListener('fetch', event => {
  const { request } = event;
  const url = new URL(request.url);

  if (request.method !== 'GET') {
    return;
  }
  if (url.origin !== location.origin) {
    return;
  }

  event.respondWith(fetch(request));
});

// Background sync for future API requests
self.addEventListener('sync', event => {
  if (event.tag === 'sync-api-data') {
    event.waitUntil(
      // Attempt to sync any pending API requests
      Promise.resolve()
    );
  }
});

// Push notifications
self.addEventListener('push', event => {
  if (!event.data) {
    return;
  }

  const options = {
    body: event.data.text(),
    icon: 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 192 192"><rect fill="%231a1a1a" width="192" height="192"/><circle cx="96" cy="96" r="70" fill="%2300a8ff"/></svg>',
    badge: 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 192 192"><circle cx="96" cy="96" r="90" fill="%2300a8ff"/></svg>',
    tag: 'opskernel-notification',
    requireInteraction: false,
    actions: [
      {
        action: 'open',
        title: 'Open App',
        icon: 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 192 192"><circle cx="96" cy="96" r="70" fill="%2300a8ff"/></svg>'
      },
      {
        action: 'close',
        title: 'Close',
        icon: 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 192 192"><path stroke="%2300a8ff" stroke-width="20" d="M60 60 l72 72 M132 60 l-72 72"/></svg>'
      }
    ]
  };

  event.waitUntil(
    self.registration.showNotification('OpsKernel Alert', options)
  );
});

// Handle notification clicks
self.addEventListener('notificationclick', event => {
  event.notification.close();
  
  if (event.action === 'close') {
    return;
  }

  event.waitUntil(
    clients.matchAll({ type: 'window' }).then(clientList => {
      // Check if app is already open
      for (let i = 0; i < clientList.length; i++) {
        if (clientList[i].url === '/' && 'focus' in clientList[i]) {
          return clientList[i].focus();
        }
      }
      // If not open, open new window
      if (clients.openWindow) {
        return clients.openWindow('/');
      }
    })
  );
});
