/**
 * Лобби: список столов и живое состояние комнаты ожидания.
 *
 * Список столов приходит по REST — он меняется редко. Состав стола, наоборот, живёт
 * по WebSocket: кто сел и кто готов, остальные должны видеть сразу.
 */

import {apiGet, apiPost} from '../net/rest-client.js';

export const lobby = $state({
    tables: [],
    themes: [],      // каталог тем стола; пусто, пока не спросили
    current: null,   // {id, code, name, maxPlayers, seats: [...]}
    inMatch: false,  // за текущим столом идёт матч
    error: null,
});

export async function loadTables() {
    lobby.tables = await apiGet('/tables');
}

export async function createTable(name, maxPlayers, isPrivate, themeId = null) {
    const table = await apiPost('/tables', {name, maxPlayers, isPrivate, themeId});
    lobby.current = table;
    return table;
}

/**
 * Каталог тем стола.
 *
 * ⚠️ Тем в базе может быть одна — тогда выбирать не из чего, и экран не должен показывать
 * список из одного пункта. Это не ошибка загрузки: наполнение каталога живёт в миграциях,
 * а не в интерфейсе.
 */
export async function loadThemes() {
    lobby.themes = await apiGet('/table-themes').catch(() => []);
    return lobby.themes;
}

/**
 * Цвет сукна выбранной темы — или `null` для темы по умолчанию.
 *
 * ⚠️ Умолчание намеренно не красится цветом из базы. В макете сукно — трёхточечный
 * градиент, подобранный руками; `feltColor` там плоский и заметно ярче. Подставить его
 * значило бы испортить вид стола ради формальной «поддержки тем». Своим цветом красятся
 * только НЕ дефолтные темы, где другого источника вида нет.
 */
export function feltOf(themeId) {
    const theme = lobby.themes.find((item) => item.id === themeId);
    return !theme || theme.isDefault ? null : theme.feltColor;
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
    lobby.inMatch = false;
}

/**
 * Вернуться туда, где сидишь.
 *
 * <p>⭐ Вкладку закрыли, телефон уснул, браузер перезагрузился — место осталось за игроком,
 * и искать свой стол глазами в общем списке он не должен. Спрашиваем сервер и садимся
 * обратно сами.
 */
export async function restoreTable() {
    const current = await apiGet('/tables/current').catch(() => null);
    if (!current?.table) {
        return null;
    }
    lobby.current = current.table;
    lobby.inMatch = current.inMatch;
    return current;
}

/**
 * Событие стола из WS — единственное место, где меняется состав.
 *
 * ⭐ Место игрока приходит в событии, а не вычисляется на клиенте: порядок мест
 * определяет очерёдность хода, и вычислять его в двух местах — верный способ разойтись
 * с сервером.
 *
 * ⚠️ Владелец состава здесь один. Раньше стол вёл свою копию мест, а эта функция вообще
 * никем не вызывалась: карточка «Ты за столом» в лобби показывала состав на момент входа
 * и не менялась — «1 из 2», когда за столом уже двое. Две копии одних и тех же данных
 * расходятся всегда; вопрос только когда это заметят.
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
