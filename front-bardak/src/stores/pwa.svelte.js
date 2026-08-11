/**
 * Установка приложения и обновление Service Worker.
 *
 * ⭐ Главное правило этапа: обновление не рвёт идущую партию. Новый воркер ждёт
 * в стороне, а страница применяет его сама — когда за столом ничего не происходит.
 */

export const pwa = $state({
    updateReady: false,   // новый воркер ждёт применения
    installPrompt: null,  // событие браузера «можно поставить на домашний экран»
    online: true,
});

let waitingWorker = null;

export function initPwa() {
    pwa.online = navigator.onLine;
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
