// Network-only: no account data, private media or stale app bundles.
self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()))
self.addEventListener('fetch', event => {
  if (event.request.mode !== 'navigate') return
  event.respondWith(fetch(event.request).catch(() => new Response(
    '<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Lumic</title><body style="font-family:system-ui;padding:48px 24px;background:#101319;color:#f5f6fa"><h1>Lumic</h1><p>暂时无法连接，请检查网络后重试。</p><button onclick="location.reload()" style="padding:12px 20px;border:0;border-radius:8px">重新连接</button></body></html>',
    { status: 503, headers: { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' } }
  )))
})
