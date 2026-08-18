/**
 * Одно соединение на всё приложение.
 *
 * ⚠️ Раньше сокет открывался только при входе за стол и закрывался при выходе. Из-за этого
 * игрок в лобби для сервера не существовал вовсе: друзья видели его «не в сети», а
 * приглашение за стол доходить было некуда — как раз в тот момент, когда оно и нужно.
 *
 * ⭐ Соединение принадлежит сессии, а не экрану. Открывается после входа, живёт до выхода,
 * переживает переходы между лобби, столом и разборами. Экраны на него подписываются.
 */

import {WsClient} from '../net/ws-client.js';

export const connection = $state({
    status: 'idle',
});

let client = null;
const listeners = new Set();

/** Подписаться на сообщения. Возвращает отписку — экраны приходят и уходят. */
export function onMessage(handler) {
    listeners.add(handler);
    return () => listeners.delete(handler);
}

const reconnectHandlers = new Set();

/** Отдельно — переподключение: после него экрану надо догнать пропущенное. */
export function onReconnect(handler) {
    reconnectHandlers.add(handler);
    return () => reconnectHandlers.delete(handler);
}

export async function openConnection() {
    if (client) {
        return;
    }
    client = new WsClient({
        onStatus: (status) => (connection.status = status),
        onEvent: (envelope) => listeners.forEach((handler) => handler(envelope)),
        onReconnect: () => reconnectHandlers.forEach((handler) => handler()),
    });
    await client.connect();
}

export function closeConnection() {
    client?.close();
    client = null;
    connection.status = 'idle';
}

export function send(type, payload = null, tableId = null) {
    return client?.send(type, payload, tableId);
}

/** Открыт ли сокет прямо сейчас: по нему видно, уйдёт команда сразу или в очередь. */
export function isConnected() {
    return Boolean(client?.connected);
}

export function hasConnection() {
    return client !== null;
}
