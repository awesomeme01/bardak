/**
 * Установка приложения, обновление Service Worker и подписка на уведомления.
 *
 * ⭐ Главное правило этапа: обновление не рвёт идущую партию. Новый воркер ждёт
 * в стороне, а страница применяет его сама — когда за столом ничего не происходит.
 */

import {apiGet, apiPost} from '../net/rest-client.js';

export const pwa = $state({
    updateReady: false,   // новый воркер ждёт применения
    installPrompt: null,  // событие браузера «можно поставить на домашний экран»
    online: true,
    pushEnabled: false,
    pushError: null,
});

let waitingWorker = null;

export function initPwa() {
    pwa.online = navigator.onLine;
    pwa.pushEnabled = 'Notification' in window && Notification.permission === 'granted';
    window.addEventListener('online', () => (pwa.online = true));
    window.addEventListener('offline', () => (pwa.online = false));

    // Браузер сам решает, когда предложить установку; событие надо перехватить,
    // иначе оно пропадёт, и кнопку показать будет уже не по чему.
    window.addEventListener('beforeinstallprompt', (event) => {
        event.preventDefault();
        pwa.installPrompt = event;
    });
    window.addEventListener('appinstalled', () => (pwa.installPrompt = null));

    if (!('serviceWorker' in navigator) || !import.meta.env.PROD) {
        // В разработке воркер только мешает: Vite отдаёт модули по своим адресам,
        // и закэшированная оболочка перекрывает горячую замену.
        return;
    }
    navigator.serviceWorker.register('/sw.js').then(watchForUpdate).catch(() => {
        // Не зарегистрировался — приложение работает как обычная страница.
    });

    // Перезагрузка ровно одна: без флага браузер уходит в цикл при смене воркера.
    let reloading = false;
    navigator.serviceWorker.addEventListener('controllerchange', () => {
        if (reloading) {
            return;
        }
        reloading = true;
        window.location.reload();
    });
}

function watchForUpdate(registration) {
    if (registration.waiting) {
        markReady(registration.waiting);
    }
    registration.addEventListener('updatefound', () => {
        const installing = registration.installing;
        installing?.addEventListener('statechange', () => {
            // installed + есть управляющий воркер = это именно обновление, а не первая установка.
            if (installing.state === 'installed' && navigator.serviceWorker.controller) {
                markReady(installing);
            }
        });
    });
}

function markReady(worker) {
    waitingWorker = worker;
    pwa.updateReady = true;
}

/** Применить обновление: страница перезагрузится через `controllerchange`. */
export function applyUpdate() {
    waitingWorker?.postMessage('SKIP_WAITING');
    pwa.updateReady = false;
}

/** Показать системное окно установки. Второй раз то же событие использовать нельзя. */
export async function installApp() {
    const prompt = pwa.installPrompt;
    if (!prompt) {
        return;
    }
    pwa.installPrompt = null;
    await prompt.prompt();
}

/**
 * Подписка на уведомления «твой ход».
 *
 * ⭐ Разрешение спрашивается только по нажатию кнопки. Браузеры блокируют запрос без
 * действия пользователя, а тот, у кого спросили сразу при входе, почти всегда жмёт
 * «запретить» — и второй раз спросить будет уже нельзя.
 */
export async function enablePush() {
    const config = await apiGet('/push/key').catch(() => null);
    if (!config?.enabled) {
        pwa.pushError = 'Уведомления на сервере не настроены';
        return false;
    }
    if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
        pwa.pushError = 'Браузер не умеет уведомления';
        return false;
    }
    if (await Notification.requestPermission() !== 'granted') {
        pwa.pushError = 'Уведомления запрещены в настройках браузера';
        return false;
    }

    const registration = await navigator.serviceWorker.ready;
    const subscription = await registration.pushManager.subscribe({
        // Без этого флага подписаться нельзя: браузер требует, чтобы каждый push
        // заканчивался видимым уведомлением.
        userVisibleOnly: true,
        applicationServerKey: base64UrlToBytes(config.publicKey),
    });
    const keys = subscription.toJSON().keys;
    await apiPost('/push/subscriptions', {
        endpoint: subscription.endpoint,
        p256dh: keys.p256dh,
        auth: keys.auth,
    });
    pwa.pushEnabled = true;
    pwa.pushError = null;
    return true;
}

/** Ключ приходит в base64url, а `subscribe` требует байты. */
function base64UrlToBytes(value) {
    const padded = (value + '='.repeat((4 - value.length % 4) % 4))
        .replace(/-/g, '+').replace(/_/g, '/');
    const binary = atob(padded);
    return Uint8Array.from(binary, (character) => character.charCodeAt(0));
}
