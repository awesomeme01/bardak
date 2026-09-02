/**
 * Адрес страницы как состояние приложения.
 *
 * ⭐ До этого весь фронт жил на одном адресе: в строке всегда стоял корень, кнопка «назад»
 * уводила из игры целиком, а перезагрузка возвращала в лобби, где бы игрок ни находился.
 * Раздел — это часть состояния, и место ему в адресе, а не только в памяти вкладки.
 *
 * ⚠️ Роутер намеренно крошечный: разделов семь, вложенности нет, и библиотека здесь
 * весила бы больше самой задачи (ADR-061 — сокращаем, а не расширяем).
 *
 * Сервер отдаёт `index.html` на любой путь (SPA-заглушка в static_handlers.go), а service
 * worker обрабатывает переходы, поэтому прямой заход на /history работает и офлайн.
 */

const start = typeof window === 'undefined' ? '/' : window.location.pathname;

export const route = $state({path: normalize(start)});

/** Хвостовой слэш — тот же раздел: «/history/» и «/history» различать незачем. */
function normalize(path) {
    if (!path || path === '/') {
        return '/';
    }
    return path.endsWith('/') ? path.slice(0, -1) : path;
}

if (typeof window !== 'undefined') {
    // ⭐ Кнопка «назад» браузера — обычный переход, а не исключение: слушаем popstate
    // и просто читаем новый адрес. Своей истории приложение не ведёт.
    window.addEventListener('popstate', () => {
        route.path = normalize(window.location.pathname);
    });
}

/** Перейти в раздел, оставив след в истории: «назад» вернёт откуда пришли. */
export function go(path) {
    const next = normalize(path);
    if (next === route.path) {
        return;
    }
    window.history.pushState({}, '', next);
    route.path = next;
}

/**
 * Заменить адрес, НЕ оставляя следа.
 *
 * ⚠️ Нужно там, где переход сделан не человеком: приглашение по ссылке усаживает за стол
 * само, и «назад» из-за стола обязано вести в лобби, а не на ссылку-приглашение, которая
 * тут же усадит обратно.
 */
export function replace(path) {
    const next = normalize(path);
    window.history.replaceState({}, '', next);
    route.path = next;
}

/**
 * Разбор адреса в экран.
 *
 * Возвращает `{screen, param}`: параметр есть только у чужого профиля.
 * Неизвестный адрес — это лобби: выдумывать страницу 404 внутри игры незачем.
 */
export function screenOf(path) {
    const [, head, tail] = normalize(path).split('/');
    switch (head) {
        case 'table':
            return {screen: 'table', param: null};
        case 'history':
            return {screen: 'history', param: tail ?? null};
        case 'friends':
            return {screen: 'friends', param: null};
        case 'changelog':
            return {screen: 'changelog', param: null};
        case 'leaders':
            return {screen: 'leaders', param: null};
        case 'profile':
            return {screen: 'profile', param: null};
        case 'stats':
            return {screen: 'stats', param: null};
        case 'player':
            // Чужая статистика: имя подгрузит сам экран, в адресе только идентификатор.
            return tail ? {screen: 'player', param: tail} : {screen: 'stats', param: null};
        default:
            return {screen: 'lobby', param: null};
    }
}
