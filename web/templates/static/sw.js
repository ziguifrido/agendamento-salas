const CACHE = 'salas-v36';
const ASSETS = ['/static/css/tokens.css', '/static/css/app.css', '/static/js/htmx.js', '/static/js/app.js', '/static/manifest.webmanifest', '/static/icons/icon.svg'];

self.addEventListener('install', event => {
  event.waitUntil(caches.open(CACHE).then(cache => cache.addAll(ASSETS)));
});

self.addEventListener('activate', event => {
  event.waitUntil(caches.keys().then(keys => Promise.all(keys.filter(key => key !== CACHE).map(key => caches.delete(key)))).then(() => self.clients.claim()));
});

self.addEventListener('fetch', event => {
  if (event.request.method !== 'GET' || !new URL(event.request.url).pathname.startsWith('/static/')) return;
  event.respondWith(caches.open(CACHE).then(cache => cache.match(event.request).then(cached => cached || fetch(event.request).then(response => {
    const copy = response.clone();
    cache.put(event.request, copy);
    return response;
  }))));
});
