/**
 * REST-клиент.
 *
 * Здесь же живёт вся работа с токенами:
 *   - access подставляется в заголовок сам;
 *   - на 401 делается ОДНА попытка обновить пару и повторить запрос;
 *   - параллельные запросы, поймавшие 401, ждут один общий refresh, а не гоняют
 *     его каждый по разу: иначе первый же обмен отзовёт токен, и остальные
 *     получат «повторное использование» — то есть выкинут пользователя.
 */

const BASE = '/api';

let accessToken = null;
let onSessionLost = () => {};
let refreshInFlight = null;

/** Подключает хранилище сессии: откуда брать refresh и куда сообщать о разлогине. */
export function configureAuth({getRefreshToken, onTokens, onLost}) {
    restoreRefreshToken = getRefreshToken;
    saveTokens = onTokens;
    onSessionLost = onLost ?? (() => {});
}

let restoreRefreshToken = () => null;
let saveTokens = () => {};

export function setAccessToken(token) {
    accessToken = token;
}

export function apiGet(path) {
    return request('GET', path, null);
}

export function apiPost(path, body) {
    return request('POST', path, body);
}

export function apiPatch(path, body) {
    return request('PATCH', path, body);
}

/** Запрос без токена и без авто-refresh — для самого входа и обновления пары. */
export async function apiPostAnonymous(path, body) {
    return send('POST', path, body, null);
}

async function request(method, path, body, retrying = false) {
    const response = await send(method, path, body, accessToken);
    if (response.status !== 401 || retrying) {
        return unwrap(response);
    }

    const refreshed = await refreshOnce();
    if (!refreshed) {
        onSessionLost();
        return unwrap(response);
    }
    return request(method, path, body, true);
}

/**
 * Обновление пары. Все, кто пришёл во время обмена, ждут один и тот же промис —
 * см. комментарий в шапке файла.
 */
function refreshOnce() {
    if (refreshInFlight) {
        return refreshInFlight;
    }
    const refreshToken = restoreRefreshToken();
    if (!refreshToken) {
        return Promise.resolve(false);
    }

    refreshInFlight = send('POST', '/auth/refresh', {refreshToken}, null)
        .then(async (response) => {
            if (!response.ok) {
                return false;
            }
            const tokens = await response.json();
            accessToken = tokens.accessToken;
            saveTokens(tokens);
            return true;
        })
        .catch(() => false)
        .finally(() => {
            refreshInFlight = null;
        });

    return refreshInFlight;
}

/**
 * ⭐ Сетевой сбой отличается от отказа сервера.
 *
 * `fetch` бросает `TypeError` и когда сети нет, и когда сервер не отвечает, — а вызывающий
 * по такому исключению не может понять, что делать: ждать и повторить или разлогинить
 * пользователя. Приводим его к тому же виду, что и ответы с ошибкой, с отдельным кодом.
 */
async function send(method, path, body, token) {
    const headers = {'Accept': 'application/json'};
    if (body !== null && body !== undefined) {
        headers['Content-Type'] = 'application/json';
    }
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }
    try {
        return await fetch(`${BASE}${path}`, {
            method,
            headers,
            body: body === null || body === undefined ? undefined : JSON.stringify(body),
        });
    } catch {
        throw new ApiError('NETWORK_UNAVAILABLE', 'Сервер недоступен', null);
    }
}

async function unwrap(response) {
    if (response.status === 204) {
        return null;
    }
    const body = await response.json().catch(() => null);
    if (!response.ok) {
        // Единый формат ошибок описан в planning/05-api-contracts.md.
        throw new ApiError(body?.code ?? 'HTTP_' + response.status,
            body?.message ?? response.statusText, body);
    }
    return body;
}

export class ApiError extends Error {
    constructor(code, message, body) {
        super(message);
        this.name = 'ApiError';
        this.code = code;
        this.body = body;
    }
}
