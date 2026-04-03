var CACHE_NAME = 'drohnenwetter-v1';
var PRECACHE = [
  '/',
  '/static/drohnenwetter-dark.svg',
  '/static/drohnenwetter-light.svg'
];

self.addEventListener('install', function(e) {
  e.waitUntil(
    caches.open(CACHE_NAME).then(function(cache) {
      return cache.addAll(PRECACHE);
    })
  );
  self.skipWaiting();
});

self.addEventListener('activate', function(e) {
  e.waitUntil(
    caches.keys().then(function(keys) {
      return Promise.all(
        keys.filter(function(k) { return k !== CACHE_NAME; })
            .map(function(k) { return caches.delete(k); })
      );
    })
  );
  self.clients.claim();
});

self.addEventListener('fetch', function(e) {
  // Only cache GET requests for same-origin navigation/assets
  if (e.request.method !== 'GET') return;

  // Never cache API responses — always go to network
  var url = new URL(e.request.url);
  if (['/results', '/zone-info', '/traffic', '/track', '/health'].indexOf(url.pathname) !== -1) return;

  e.respondWith(
    fetch(e.request).then(function(response) {
      // Cache successful same-origin responses
      if (response.ok && url.origin === self.location.origin) {
        var clone = response.clone();
        caches.open(CACHE_NAME).then(function(cache) {
          cache.put(e.request, clone);
        });
      }
      return response;
    }).catch(function() {
      // Network failed — serve from cache (offline fallback)
      return caches.match(e.request);
    })
  );
});
