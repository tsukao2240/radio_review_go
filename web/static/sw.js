const CACHE_VERSION = 'v1.0.0';
const CACHE_NAME = `radio-review-${CACHE_VERSION}`;

// キャッシュするリソースのリスト
const STATIC_CACHE_URLS = [
  '/',
  '/static/app.css',
  '/static/app.js',
  '/manifest.json',
  '/offline.html',
  '/static/icons/icon-192x192.png',
  '/static/icons/icon-512x512.png'
];

// オフライン時に表示するページ
const OFFLINE_URL = '/offline.html';

// インストールイベント
self.addEventListener('install', (event) => {
  console.log('[ServiceWorker] Install');

  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      console.log('[ServiceWorker] Caching static resources');
      return cache.addAll(STATIC_CACHE_URLS).catch((error) => {
        console.error('[ServiceWorker] Failed to cache:', error);
      });
    })
  );

  self.skipWaiting();
});

// アクティベーションイベント
self.addEventListener('activate', (event) => {
  console.log('[ServiceWorker] Activate');

  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== CACHE_NAME) {
            console.log('[ServiceWorker] Deleting old cache:', cacheName);
            return caches.delete(cacheName);
          }
        })
      );
    })
  );

  return self.clients.claim();
});

// フェッチイベント - ネットワーク優先戦略
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // 外部リクエストはそのまま通す
  if (url.origin !== location.origin) {
    return;
  }

  // APIリクエストとGET以外はService Workerを通さない
  if (url.pathname.startsWith('/api/') || request.method !== 'GET') {
    return;
  }

  event.respondWith(
    fetch(request)
      .then((response) => {
        if (response && response.status === 200 && request.method === 'GET') {
          const responseClone = response.clone();
          caches.open(CACHE_NAME).then((cache) => {
            cache.put(request, responseClone);
          });
        }
        return response;
      })
      .catch(() => {
        return caches.match(request).then((cachedResponse) => {
          if (cachedResponse) {
            return cachedResponse;
          }

          if (request.headers.get('accept') && request.headers.get('accept').includes('text/html')) {
            return caches.match(OFFLINE_URL);
          }

          return new Response('Network error', {
            status: 408,
            headers: { 'Content-Type': 'text/plain' }
          });
        });
      })
  );
});

// メッセージイベント - キャッシュのクリア
self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }

  if (event.data && event.data.type === 'CLEAR_CACHE') {
    event.waitUntil(
      caches.keys().then((cacheNames) => {
        return Promise.all(
          cacheNames.map((cacheName) => {
            return caches.delete(cacheName);
          })
        );
      }).then(() => {
        return self.clients.matchAll();
      }).then((clients) => {
        clients.forEach((client) => {
          client.postMessage({ type: 'CACHE_CLEARED' });
        });
      })
    );
  }
});

// プッシュ通知イベント
self.addEventListener('push', (event) => {
  if (!event.data) {
    return;
  }

  const data = event.data.json();
  const options = {
    body: data.body || '新しい通知があります',
    icon: '/static/icons/icon-192x192.png',
    badge: '/static/icons/icon-72x72.png',
    vibrate: [100, 50, 100],
    data: {
      dateOfArrival: Date.now(),
      primaryKey: data.id
    }
  };

  event.waitUntil(
    self.registration.showNotification(data.title || 'RadioProgram Review', options)
  );
});

// 通知クリックイベント
self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  event.waitUntil(
    clients.openWindow('/')
  );
});
