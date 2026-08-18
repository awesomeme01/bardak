/**
 * Друзья: список, заявки и приглашения за стол.
 *
 * ⭐ Онлайн-статус приходит вместе со списком и не живёт своей жизнью: отдельная подписка
 * на присутствие означала бы второй источник правды о том же самом. Список перечитывается,
 * когда экран открыт, — это и есть обновление статусов.
 */

import {apiDelete, apiGet, apiPost} from '../net/rest-client.js';
import {onMessage} from './connection.svelte.js';

export const friends = $state({
    list: [],        // принятые
    incoming: [],    // заявки ко мне
    outgoing: [],    // мои заявки
    error: null,
    loading: false,
    notice: null,    // «позвал» / «его нет в сети» — живёт несколько секунд
});

/**
 * Приглашение за стол, которое сейчас на экране.
 *
 * ⭐ Оно не хранится ни на сервере, ни здесь дольше показа: позвать за стол — это оклик.
 * Пропустил — значит пропустил, и звать будут заново, а не откроется завтра письмо.
 */
export const invite = $state({from: null, tableName: null, tableCode: null, tableId: null});

/** Слушаем приглашения всё время, пока приложение открыто, — не только за столом. */
onMessage((envelope) => {
    if (envelope.type !== 'TABLE_INVITE') {
        return;
    }
    const p = envelope.payload ?? {};
    invite.from = p.fromName;
    invite.tableName = p.tableName;
    invite.tableCode = p.tableCode;
    invite.tableId = p.tableId;
});

export function dismissInvite() {
    invite.from = null;
    invite.tableId = null;
}

let noticeTimer = null;

function notify(text) {
    friends.notice = text;
    clearTimeout(noticeTimer);
    noticeTimer = setTimeout(() => (friends.notice = null), 5000);
}

export async function loadFriends() {
    friends.loading = true;
    friends.error = null;
    try {
        const data = await apiGet('/friends');
        friends.list = data.friends;
        friends.incoming = data.incoming;
        friends.outgoing = data.outgoing;
    } catch (e) {
        friends.error = e.message;
    } finally {
        friends.loading = false;
    }
}

/** Позвать в друзья по логину. Регистр не важен — как и при входе. */
export async function addFriend(username) {
    friends.error = null;
    try {
        await apiPost('/friends/requests', {username});
        await loadFriends();
        return true;
    } catch (e) {
        friends.error = e.message;
        return false;
    }
}

export async function acceptFriend(userId) {
    try {
        await apiPost(`/friends/${userId}/accept`, {});
        await loadFriends();
    } catch (e) {
        friends.error = e.message;
    }
}

/** Убрать из друзей или отклонить заявку — для сервера это одно и то же. */
export async function removeFriend(userId) {
    try {
        await apiDelete(`/friends/${userId}`);
        await loadFriends();
    } catch (e) {
        friends.error = e.message;
    }
}

/**
 * Позвать друга за свой стол.
 *
 * ⚠️ Приглашение не хранится: это оклик, а не письмо. Поэтому ответ честно говорит,
 * услышал ли друг прямо сейчас, — иначе экран обещал бы то, чего не было.
 */
export async function inviteFriend(userId, tableId, name) {
    try {
        const {delivered} = await apiPost(`/friends/${userId}/invite`, {tableId});
        notify(delivered ? `${name} позван за стол` : `${name} не в сети — отправил уведомление`);
    } catch (e) {
        friends.error = e.message;
    }
}
