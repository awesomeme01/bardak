/**
 * Установка приложения, обновление Service Worker и подписка на уведомления.
 *
 * ⭐ Главное правило этапа: обновление не рвёт идущую партию. Новый воркер ждёт
 * в стороне, а страница применяет его сама — когда за столом ничего не происходит.
 */

import {apiDelete, apiGet, apiPost} from '../net/rest-client.js';

export const pwa = $state({
    updateReady: false,   // новый воркер ждёт применения
    installPrompt: null,  // событие браузера «можно поставить на домашний экран»
    /**
     * ⚠️ На iOS ставить приложение можно ТОЛЬКО руками: события
     * `beforeinstallprompt` в Safari не существует, и кнопки установки не будет
     * никогда — это решение Apple, а не наш недосмотр. Единственное, что мы можем, —
     * сказать, куда нажимать.
     *
     * ⭐ И только в самом Safari: из встроенных браузеров (Instagram, Telegram)
     * пункта «На экран «Домой»» нет вовсе, сколько бы манифестов мы ни отдали.
     */
    iosHint: false,
    online: true,
    pushEnabled: false,
    pushError: null,
});

let waitingWorker = null;

export function initPwa() {
    pwa.online = navigator.onLine;
    // ⚠️ Разрешение и подписка — разные вещи. Разрешение остаётся выданным и после того,
    // как игрок выключил уведомления, поэтому спрашиваем у браузера саму подписку.
    refreshPushState();
    window.addEventListener('online', () => (pwa.online = true));
    window.addEventListener('offline', () => (pwa.online = false));

    // Браузер сам решает, когда предложить установку; событие надо перехватить,
    // иначе оно пропадёт, и кнопку показать будет уже не по чему.
    // Уже стоит на домашнем экране — подсказывать нечего.
    // ⚠️ `navigator.standalone` — нестандартное свойство iOS Safari, и типов у него нет.
    // Именно оно отвечает «приложение уже на домашнем экране» там, где display-mode
    // старым Safari не поддерживается.
    const iosStandalone = /** @type {{standalone?: boolean}} */ (window.navigator).standalone;
    const standalone = window.matchMedia('(display-mode: standalone)').matches
        || iosStandalone === true;
    const ios = /iphone|ipad|ipod/i.test(window.navigator.userAgent)
        || (window.navigator.platform === 'MacIntel' && window.navigator.maxTouchPoints > 1);
    pwa.iosHint = ios && !standalone;

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

/**
 * Выключить уведомления на ЭТОМ устройстве.
 *
 * ⭐ Две половины, и обе обязательны: браузер перестаёт принимать push, а сервер забывает
 * подписку. Отписаться только в браузере — оставить серверу мёртвый адрес, в который он
 * будет стучаться на каждом ходу; удалить только на сервере — оставить браузеру подписку,
 * которую он считает живой, и «включить» второй раз уже ничего не изменит.
 *
 * ⚠️ Кнопка выключения нужна не для симметрии. Уведомления, которые нельзя выключить
 * в самой игре, выключают в настройках браузера — вместе с возможностью включить их
 * обратно, потому что второй раз разрешение уже не спросят.
 */
export async function disablePush() {
    pwa.pushError = null;
    if (!('serviceWorker' in navigator)) {
        pwa.pushEnabled = false;
        return true;
    }

    const registration = await navigator.serviceWorker.ready;
    const subscription = await registration.pushManager.getSubscription();
    if (subscription) {
        // Сначала сервер: отписавшись в браузере, адрес узнать уже не у кого.
        await apiDelete('/push/subscriptions', {endpoint: subscription.endpoint}).catch(() => null);
        await subscription.unsubscribe().catch(() => false);
    }
    pwa.pushEnabled = false;
    return true;
}

/**
 * Узнать, включены ли уведомления на этом устройстве.
 *
 * ⚠️ Без этой проверки после перезагрузки страницы кнопка снова предлагала «включить»
 * уже включённые уведомления: `pushEnabled` жил только в памяти вкладки.
 */
export async function refreshPushState() {
    if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
        return;
    }
    try {
        const registration = await navigator.serviceWorker.ready;
        pwa.pushEnabled = Boolean(await registration.pushManager.getSubscription());
    } catch {
        // Нет воркера — нет и подписки; это не ошибка, о которой стоит говорить игроку.
    }
}

/** Ключ приходит в base64url, а `subscribe` требует байты. */
function base64UrlToBytes(value) {
    const padded = (value + '='.repeat((4 - value.length % 4) % 4))
        .replace(/-/g, '+').replace(/_/g, '/');
    const binary = atob(padded);
    return Uint8Array.from(binary, (character) => character.charCodeAt(0));
}
