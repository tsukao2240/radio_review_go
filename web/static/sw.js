const CACHE_VERSION = 'v1.0.1';
const CACHE_NAME = `radio-review-${CACHE_VERSION}`;
const RECORDING_CACHE_NAME = `radio-review-recordings-${CACHE_VERSION}`;

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
          if (cacheName !== CACHE_NAME && cacheName !== RECORDING_CACHE_NAME) {
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

  if (url.pathname === '/recording/stream' || url.pathname === '/recording/download') {
    event.respondWith(recordingCacheResponse(request));
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

function recordingCacheKey(url) {
  const recordingId = url.searchParams.get('recording_id');
  if (!recordingId) {
    return null;
  }
  return `${location.origin}/recording/stream?recording_id=${encodeURIComponent(recordingId)}`;
}

async function recordingCacheResponse(request) {
  const url = new URL(request.url);
  const cacheKey = recordingCacheKey(url);
  if (!cacheKey) {
    return fetch(request);
  }
  const cache = await caches.open(RECORDING_CACHE_NAME);
  const cached = await cache.match(cacheKey);
  if (!cached) {
    return fetch(request);
  }

  const range = request.headers.get('range');
  if (!range) {
    return cached;
  }

  const buffer = await cached.arrayBuffer();
  const match = /^bytes=(\d*)-(\d*)$/.exec(range);
  if (!match) {
    return cached;
  }
  const start = match[1] ? Number(match[1]) : 0;
  const end = match[2] ? Number(match[2]) : buffer.byteLength - 1;
  if (start >= buffer.byteLength || end >= buffer.byteLength || start > end) {
    return new Response(null, {
      status: 416,
      headers: {
        'Content-Range': `bytes */${buffer.byteLength}`
      }
    });
  }
  const sliced = buffer.slice(start, end + 1);
  return new Response(sliced, {
    status: 206,
    statusText: 'Partial Content',
    headers: {
      'Content-Type': cached.headers.get('Content-Type') || 'audio/aac',
      'Content-Length': String(sliced.byteLength),
      'Content-Range': `bytes ${start}-${end}/${buffer.byteLength}`,
      'Accept-Ranges': 'bytes'
    }
  });
}

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

  if (event.data && event.data.type === 'CACHE_RECORDING') {
    event.waitUntil(cacheRecording(event.data));
  }
});

async function cacheRecording(data) {
  const clientsList = await self.clients.matchAll();
  const notify = (message) => {
    clientsList.forEach((client) => client.postMessage(message));
  };
  try {
    const url = new URL(data.url, location.origin);
    const cacheKey = recordingCacheKey(url);
    if (!cacheKey) {
      throw new Error('recording_id is required');
    }
    const response = await fetch(url.toString(), { credentials: 'same-origin' });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    const cache = await caches.open(RECORDING_CACHE_NAME);
    await cache.put(cacheKey, response.clone());
    notify({ type: 'RECORDING_CACHED', recordingId: data.recordingId });
  } catch (error) {
    notify({ type: 'RECORDING_CACHE_FAILED', recordingId: data.recordingId, error: String(error) });
  }
}

// プッシュ通知イベント
self.addEventListener('push', (event) => {
  if (!event.data) {
    return;
  }

  let data = {};
  try {
    data = event.data.json();
  } catch (error) {
    data = { title: 'RadioProgram Review', body: event.data.text() };
  }
  const options = {
    body: data.body || '新しい通知があります',
    icon: '/static/icons/icon-192x192.png',
    badge: '/static/icons/icon-72x72.png',
    vibrate: [100, 50, 100],
    data: {
      dateOfArrival: Date.now(),
      primaryKey: data.id,
      url: data.url || '/'
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
    clients.openWindow(event.notification.data && event.notification.data.url ? event.notification.data.url : '/')
  );
});
