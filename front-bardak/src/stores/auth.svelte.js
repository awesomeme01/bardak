/**
 * Сессия игрока.
 *
 * ⭐ Access-токен живёт ТОЛЬКО в памяти: он короткий, и переживать перезагрузку ему
 * незачем. Refresh лежит в localStorage — это осознанный компромисс: без него
 * перезагрузка страницы выкидывала бы из игры. Цена — при XSS токен угоняется;
 * защита от этого одна и лежит не здесь: не вставлять чужой HTML в страницу.
 * Сервер, со своей стороны, ротирует refresh при каждом обмене и убивает всю серию,
 * если один и тот же токен предъявили дважды.
 */

import {apiGet, apiPostAnonymous, configureAuth, setAccessToken} from '../net/rest-client.js';

const REFRESH_KEY = 'bardak.refreshToken';

export const session = $state({
    user: null,
    status: 'unknown', // unknown | anonymous | authenticated
});

configureAuth({
    getRefreshToken: () => localStorage.getItem(REFRESH_KEY),
    onTokens: (tokens) => applyTokens(tokens),
    onLost: () => clearSession(),
});

/** Восстановление сессии после перезагрузки: обмениваем refresh на свежую пару. */
export async function restoreSession() {
    if (!localStorage.getItem(REFRESH_KEY)) {
        session.status = 'anonymous';
        return;
    }
    try {
        const response = await apiPostAnonymous('/auth/refresh',
            {refreshToken: localStorage.getItem(REFRESH_KEY)});
        if (!response.ok) {
            clearSession();
            return;
        }
        applyTokens(await response.json());
    } catch {
        clearSession();
    }
}

export async function login(username, password) {
    await exchange('/auth/login', {username, password});
}

export async function register(form) {
    await exchange('/auth/register', form);
}

export async function logout() {
    const refreshToken = localStorage.getItem(REFRESH_KEY);
    if (refreshToken) {
        await apiPostAnonymous('/auth/logout', {refreshToken}).catch(() => null);
    }
    clearSession();
}

export function loadProfile() {
    return apiGet('/profile');
}

async function exchange(path, body) {
    const response = await apiPostAnonymous(path, body);
    const payload = await response.json().catch(() => null);
    if (!response.ok) {
        const error = new Error(payload?.message ?? 'Не получилось');
        error.code = payload?.code;
        error.details = payload?.details;
        throw error;
    }
    applyTokens(payload);
}

function applyTokens(tokens) {
    setAccessToken(tokens.accessToken);
    localStorage.setItem(REFRESH_KEY, tokens.refreshToken);
    session.user = tokens.user ?? session.user;
    session.status = 'authenticated';
}

function clearSession() {
    setAccessToken(null);
    localStorage.removeItem(REFRESH_KEY);
    session.user = null;
    session.status = 'anonymous';
}
