/**
 * Service Worker: оболочка приложения офлайн и картинки карт из кэша.
 *
 * ⭐ Игра офлайн не работает и работать не может: состояние стола живёт на сервере
 * и приходит по сокету. Кэш нужен для другого — чтобы приложение открывалось мгновенно
 * и не белым экраном, когда сеть на секунду пропала в лифте.
 *
 * ⚠️ `/api/**` и `/ws` не кэшируются никогда и даже не проходят через обработчик:
 * отданный из кэша ответ на «начать матч» или на историю — это ложь клиенту, которую
 * он не может отличить от правды.
 */

const VERSION = 'v3';
const SHELL = `bardak-shell-${VERSION}`;
const CARDS = `bardak-cards-${VERSION}`;
const SHELL_URL = '/';

self.addEventListener('install', (event) => {
    // Ждущий воркер не активируется сам: партия важнее свежести (см. message ниже).
    event.waitUntil(caches.open(SHELL).then((cache) => cache.add(SHELL_URL)));
});

self.addEventListener('activate', (event) => {
    event.waitUntil((async () => {
        const names = await caches.keys();
        await Promise.all(names
            .filter((name) => name.startsWith('bardak-') && name !== SHELL && name !== CARDS)
            .map((name) => caches.delete(name)));
        await self.clients.claim();
    })());
});

/**
 * ⭐ Обновление применяется только по команде страницы.
 *
 * Сам по себе новый воркер ждёт: перезагрузка посреди раздачи стоит хода, а иногда
 * и партии. Страница решает, когда безопасно, — см. `stores/pwa.svelte.js`.
 */
self.addEventListener('message', (event) => {
    if (event.data === 'SKIP_WAITING') {
        self.skipWaiting();
    }
});

/**
 * Уведомление «твой ход».
 *
 * ⭐ Показать его обязательно: браузер требует показать уведомление на каждый полученный
 * push, иначе он отзовёт подписку целиком. Поэтому даже на непонятную нагрузку показывается
 * что-то осмысленное, а не ничего.
 *
 * Тег один на все ходы: второе уведомление заменяет первое, а не копится столбиком.
 */
self.addEventListener('push', (event) => {
    let payload = {};
    try {
        payload = event.data ? event.data.json() : {};
    } catch {
        payload = {};
    }
    event.waitUntil(self.registration.showNotification(payload.title ?? 'Бардак', {
        body: payload.body ?? 'За столом ждут тебя',
        icon: '/icons/icon-192.png',
        badge: '/icons/icon-192.png',
        tag: 'bardak-turn',
        renotify: true,
        data: {tableId: payload.tableId ?? null},
    }));
});

/** Клик по уведомлению открывает уже открытую вкладку, а не вторую копию игры. */
self.addEventListener('notificationclick', (event) => {
    event.notification.close();
    event.waitUntil((async () => {
        const clients = await self.clients.matchAll({type: 'window', includeUncontrolled: true});
        for (const client of clients) {
            if (new URL(client.url).origin === self.location.origin) {
                await client.focus();
                client.postMessage({type: 'OPEN_TABLE', tableId: event.notification.data?.tableId});
                return;
            }
        }
        await self.clients.openWindow('/');
    })());
});

self.addEventListener('fetch', (event) => {
    const request = event.request;
    if (request.method !== 'GET') {
        return;
    }
    const url = new URL(request.url);
    if (url.origin !== self.location.origin) {
        return;
    }
    if (url.pathname.startsWith('/api') || url.pathname.startsWith('/ws')) {
        return;
    }

    if (request.mode === 'navigate') {
        event.respondWith(shellFirstFromNetwork(request));
        return;
    }
    if (url.pathname.startsWith('/assets/')) {
        event.respondWith(cacheFirst(request, CARDS));
        return;
    }
    if (url.pathname.startsWith('/app/') || url.pathname.startsWith('/icons/')
        || url.pathname.startsWith('/fonts/') || url.pathname === '/manifest.webmanifest') {
        event.respondWith(cacheFirst(request, SHELL));
    }
});

/** Навигация: сначала сеть, кэш — на случай, когда сети нет. */
async function shellFirstFromNetwork(request) {
    try {
        const response = await fetch(request);
        const cache = await caches.open(SHELL);
        cache.put(SHELL_URL, response.clone());
        return response;
    } catch (error) {
        const cached = await caches.match(SHELL_URL);
        return cached ?? new Response('Нет сети и нет сохранённой копии', {
            status: 503,
            headers: {'Content-Type': 'text/plain; charset=utf-8'},
        });
    }
}

/**
 * Картинки карт и бандл: сначала кэш.
 *
 * Имя файла бандла содержит хеш, а картинка карты не меняется вовсе — значит,
 * содержимое по этому адресу постоянно, и спрашивать сеть незачем.
 */
async function cacheFirst(request, cacheName) {
    const cached = await caches.match(request);
    if (cached) {
        return cached;
    }
    const response = await fetch(request);
    if (response.ok) {
        const cache = await caches.open(cacheName);
        cache.put(request, response.clone());
    }
    return response;
}
