/* Service Worker "kill-switch".
 * Una PWA con caché causaba que el navegador sirviera bundles viejos tras cada
 * deploy. Este SW limpia TODOS los caches y se desregistra solo, luego recarga
 * la pestaña para servir siempre la versión fresca desde red.
 * (Se reintroducirá una PWA offline-first con versionado por hash más adelante.)
 */
self.addEventListener('install', () => self.skipWaiting())

self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    const keys = await caches.keys()
    await Promise.all(keys.map((k) => caches.delete(k)))
    try { await self.registration.unregister() } catch (e) { /* noop */ }
    const clients = await self.clients.matchAll({ type: 'window' })
    for (const client of clients) client.navigate(client.url)
  })())
})

// Sin interceptar fetch: todo va directo a la red.
