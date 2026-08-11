/**
 * Лобби: список столов и живое состояние комнаты ожидания.
 *
 * Список столов приходит по REST — он меняется редко. Состав стола, наоборот, живёт
 * по WebSocket: кто сел и кто готов, остальные должны видеть сразу.
 */

import {apiGet, apiPost} from '../net/rest-client.js';

export const lobby = $state({
    tables: [],
    current: null,   // {id, code, name, maxPlayers, seats: [...]}
    error: null,
});

export async function loadTables() {
    lobby.tables = await apiGet('/tables');
}

export async function createTable(name, maxPlayers, isPrivate) {
    const table = await apiPost('/tables', {name, maxPlayers, isPrivate});
    lobby.current = table;
    return table;
}

export async function openTable(tableId) {
    lobby.current = await apiGet(`/tables/${tableId}`);
    return lobby.current;
}

export async function openByCode(code) {
    lobby.current = await apiGet(`/tables/by-code/${code.toUpperCase()}`);
    return lobby.current;
}

export function leaveTable() {
    lobby.current = null;
}

/**
 * Событие стола из WS.
 *
 * ⭐ Место игрока приходит в событии, а не вычисляется на клиенте: порядок мест
 * определяет очерёдность хода, и вычислять его в двух местах — верный способ разойтись
 * с сервером.
 */
export function applyTableEvent(envelope) {
    if (!lobby.current || envelope.tableId !== lobby.current.id) {
        return;
    }
    const {userId, displayName, seatNo, ready} = envelope.payload ?? {};

    if (envelope.type === 'PLAYER_JOINED') {
        const seats = lobby.current.seats.filter((seat) => seat.userId !== userId);
        seats.push({seatNo, userId, displayName, ready: ready ?? false, online: true});
        seats.sort((left, right) => left.seatNo - right.seatNo);
        lobby.current = {...lobby.current, seats};
    }
    if (envelope.type === 'PLAYER_LEFT') {
        lobby.current = {
            ...lobby.current,
            seats: lobby.current.seats.filter((seat) => seat.userId !== userId),
        };
    }
    if (envelope.type === 'PLAYER_READY') {
        lobby.current = {
            ...lobby.current,
            seats: lobby.current.seats.map((seat) =>
                seat.userId === userId ? {...seat, ready} : seat),
        };
    }
    if (envelope.type === 'PLAYER_OFFLINE') {
        lobby.current = {
            ...lobby.current,
            seats: lobby.current.seats.map((seat) =>
                seat.userId === userId ? {...seat, online: false} : seat),
        };
    }
}
