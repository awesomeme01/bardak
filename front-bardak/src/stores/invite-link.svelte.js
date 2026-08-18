/**
 * Приглашение по ссылке: `http://<хост>/?t=КОД`.
 *
 * ⭐ Ссылка ведёт за конкретный стол и работает для того, у кого учётки ещё нет. Поэтому
 * код нельзя держать только в памяти вкладки: между переходом по ссылке и посадкой за стол
 * человек успевает зарегистрироваться, а это форма, ошибки и перерисовки. Код живёт
 * в `sessionStorage` и переживает всё, кроме закрытия вкладки.
 *
 * ⚠️ Адрес чистится сразу, как только код прочитан. Иначе обновление страницы через час
 * снова тащило бы игрока за давно закрытый стол, а сама ссылка оставалась бы в адресной
 * строке и уезжала в закладки и историю браузера.
 *
 * Ссылка — параметр к корню, а не путь `/join/КОД`: сервер отдаёт статику без SPA-фолбэка,
 * и по пути пришла бы 404 вместо приложения.
 */

import {apiGet} from '../net/rest-client.js';

const KEY = 'bardak.invite';
const PARAM = 't';

export const inviteLink = $state({
    code: null,      // код стола, за который зовут
    table: null,     // {name, maxPlayers, seatsTaken, joinable} — чтобы показать, куда зовут
    error: null,
});

/** Ссылка, которую игрок отправляет друзьям. */
export function linkFor(code) {
    return `${location.origin}/?${PARAM}=${code}`;
}

/**
 * Прочитать код из адреса и убрать его оттуда. Вызывается один раз при старте приложения,
 * ДО проверки сессии: код нужен и вошедшему, и тому, кто ещё регистрируется.
 */
export function readInviteFromUrl() {
    let code = null;
    try {
        code = new URLSearchParams(location.search).get(PARAM);
    } catch {
        // Адрес может быть каким угодно — приглашение не повод падать.
    }
    if (code) {
        code = code.trim().toUpperCase();
        try {
            sessionStorage.setItem(KEY, code);
        } catch {
            // Приватный режим: обойдёмся памятью, до регистрации может и не дожить.
        }
        // Адрес чистим, но экран не трогаем: history.replaceState не перезагружает страницу.
        history.replaceState(null, '', location.pathname);
    }
    inviteLink.code = code ?? readStored();
    return inviteLink.code;
}

function readStored() {
    try {
        return sessionStorage.getItem(KEY);
    } catch {
        return null;
    }
}

/**
 * Узнать, куда зовут. Ручка открыта без токена — специально, чтобы незарегистрированный
 * увидел название стола раньше, чем форму регистрации.
 */
export async function loadInviteTable() {
    if (!inviteLink.code) {
        return null;
    }
    try {
        inviteLink.table = await apiGet(`/tables/invite/${inviteLink.code}`);
        inviteLink.error = null;
    } catch (e) {
        // Стол мог закрыться, пока ссылка лежала в переписке. Это не поломка приложения.
        inviteLink.table = null;
        inviteLink.error = 'Стол не найден — возможно, его уже закрыли';
    }
    return inviteLink.table;
}

/** Забрать код и забыть его: приглашение срабатывает один раз. */
export function consumeInvite() {
    const code = inviteLink.code;
    forgetInvite();
    return code;
}

export function forgetInvite() {
    inviteLink.code = null;
    inviteLink.table = null;
    inviteLink.error = null;
    try {
        sessionStorage.removeItem(KEY);
    } catch {
        // Нечего убирать — и не надо.
    }
}
