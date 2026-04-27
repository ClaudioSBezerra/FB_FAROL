// Self-destructing Service Worker.
// Um SW antigo (de um deploy anterior) continuava interceptando requisições
// mesmo após Ctrl+Shift+R, servindo HTML stale e causando 404 em assets.
// Quando o browser buscar a atualização deste SW, esta versão toma o controle,
// limpa TODOS os caches e se desregistra — recarregando todas as abas abertas.

self.addEventListener('install', () => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    try {
      const names = await caches.keys();
      await Promise.all(names.map((n) => caches.delete(n)));
    } catch (_e) { /* ignore */ }

    try {
      await self.registration.unregister();
    } catch (_e) { /* ignore */ }

    const clients = await self.clients.matchAll({ includeUncontrolled: true, type: 'window' });
    for (const client of clients) {
      try { client.navigate(client.url); } catch (_e) { /* ignore */ }
    }
  })());
});

// Passthrough — nunca interceptar nada.
self.addEventListener('fetch', () => { /* noop */ });
