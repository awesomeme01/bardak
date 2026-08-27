/**
 * Звуки стола: короткие шлепки карт на игровых событиях.
 *
 * ⭐ Источник истины о том, ЧТО случилось, — события матча, те же, по которым играют
 * анимации (см. animateGameEvent): звук — это ещё одна проекция события, а не своя
 * логика. Имена файлов — контракт с `assets/sounds/` (см. CREDITS.md там же).
 *
 * ⚠️ Браузер не даст звука до первого жеста пользователя — и не надо: первый звук
 * в игре всегда следует за чьим-то кликом. Отказ play() молча глотается: игра без
 * звука — это игра, а не ошибка.
 */

const KEY = 'bardak-sound';

function stored() {
    try {
        return localStorage.getItem(KEY) !== 'off';
    } catch {
        // Приватное окно или запрет на данные сайта: играем со звуком по умолчанию.
        return true;
    }
}

export const sound = $state({enabled: stored()});

export function toggleSound() {
    sound.enabled = !sound.enabled;
    try {
        localStorage.setItem(KEY, sound.enabled ? 'on' : 'off');
    } catch {
        // Не запомнилось — выключение доживёт до перезагрузки, и только.
    }
}

/** Немного тише пика: шлепок карты — фон игры, а не событие само по себе. */
const VOLUME = 0.6;

const cache = new Map();

export function play(name) {
    if (!sound.enabled) {
        return;
    }
    let base = cache.get(name);
    if (!base) {
        base = new Audio(`/assets/sounds/${name}.m4a`);
        base.preload = 'auto';
        cache.set(name, base);
    }
    // Клон на каждое воспроизведение: два быстрых хода не должны обрывать друг друга.
    const instance = /** @type {HTMLAudioElement} */ (base.cloneNode());
    instance.volume = VOLUME;
    instance.play().catch(() => {
        // Автоплей ещё заблокирован или файл не доехал — молчание, не поломка.
    });
}
