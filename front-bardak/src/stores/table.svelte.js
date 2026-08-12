/**
 * Стол: комната ожидания и сама игра на одном соединении.
 *
 * ⭐ Состояние игры целиком приходит с сервера снимком `STATE_SYNC`. Клиент ничего
 * не досчитывает: ни что чем бьётся, ни какой ранг сейчас летит по шкале навесов.
 * Список разрешённых ходов тоже приходит с сервера (ADR-003) — фронт правил не знает.
 */

import {apiGet} from '../net/rest-client.js';
import {WsClient} from '../net/ws-client.js';

let noticeTimer = null;

/** Уведомление живёт несколько секунд: это подсказка о случившемся, а не состояние стола. */
function notify(text) {
    table.notice = text;
    clearTimeout(noticeTimer);
    noticeTimer = setTimeout(() => (table.notice = null), 6000);
}

export const table = $state({
    info: null,        // стол из REST: id, code, name, maxPlayers, seats
    game: null,        // последний STATE_SYNC
    manifest: {},      // код карты → URL картинки
    status: 'idle',    // состояние сокета
    notice: null,      // пауза, отмена, таймаут — то, что нужно показать человеком
    result: null,      // итог матча: места, уровни, дельта рейтинга
    lastSeq: 0,
});

let client = null;

export async function enterTable(info) {
    // ⭐ Соединение переживает уход с экрана: игрок мог заглянуть в историю или в лобби,
    // а стол при этом продолжает жить. Второй сокет к тому же столу означал бы, что
    // сервер шлёт снимки в мёртвое соединение, а игрок числится и там и там.
    if (client && table.info?.id === info.id) {
        client.send('STATE_REQUEST', {}, info.id);
        return;
    }
    table.info = info;
    table.game = null;
    table.notice = null;
    table.result = null;
    table.manifest = await loadManifest(info.cardSetId);

    client = new WsClient({
        onStatus: (status) => (table.status = status),
        onEvent: onEnvelope,
        onReconnect: resync,
    });
    await client.connect();
    client.send('TABLE_JOIN', {}, info.id);
    // Если матч уже идёт — сервер пришлёт снимок; если нет, ответит, что матча нет.
    client.send('STATE_REQUEST', {}, info.id);
}

/**
 * Догнать пропущенное после разрыва.
 *
 * ⭐ Просим и события с последнего известного номера, и полный снимок: за время разрыва
 * могла закончиться раздача, и тогда события собрать состояние уже не помогут — вернуть
 * в него можно только снимком.
 */
function resync() {
    if (!table.info) {
        return;
    }
    client?.send('RESYNC', {lastSeq: table.lastSeq}, table.info.id);
    notify('Связь восстановлена — догоняю стол');
}

/** Встать из-за стола совсем: место освобождается. Посреди матча сервер откажет. */
export function leaveTable() {
    if (client && table.info) {
        client.send('TABLE_LEAVE', {}, table.info.id);
    }
    detachTable();
}

/**
 * Уйти с экрана стола, не вставая из-за него.
 *
 * <p>⚠️ Пока идёт матч, соединение <b>не рвём</b>: разрыв ставит партию на паузу, и через
 * минуту она отменяется у всех. Посмотреть историю посреди игры — не повод потерять матч.
 */
export function detachTable() {
    if (table.game && !table.result) {
        return;
    }
    client?.close();
    client = null;
    table.info = null;
    table.game = null;
    table.result = null;
}

export function setReady(ready) {
    client?.send('TABLE_READY', {ready}, table.info.id);
}

export function startMatch() {
    table.result = null;
    table.game = null;
    client?.send('MATCH_START', {}, table.info.id);
}

/** Ход: тип и payload берутся из `availableActions` — фронт их не сочиняет. */
export function play(action) {
    client?.send(action.type, action.payload ?? {}, table.info.id);
}

function onEnvelope(envelope) {
    if (typeof envelope.seq === 'number') {
        // Пропуск в нумерации означает, что мы что-то не увидели: просим догон.
        if (envelope.seq > table.lastSeq + 1) {
            client?.send('RESYNC', {lastSeq: table.lastSeq}, table.info.id);
        }
        table.lastSeq = Math.max(table.lastSeq, envelope.seq);
    }

    switch (envelope.type) {
        case 'STATE_SYNC':
            table.game = envelope.payload;
            break;
        case 'PLAYER_JOINED':
        case 'PLAYER_LEFT':
        case 'PLAYER_READY':
            applySeatEvent(envelope);
            break;
        case 'MATCH_PAUSED':
            // Пауза — состояние, а не подсказка: висит, пока игрок не вернётся.
            table.notice = 'Игрок пропал со связи — ждём возвращения';
            clearTimeout(noticeTimer);
            break;
        case 'MATCH_RESUMED':
            table.notice = null;
            break;
        case 'MATCH_OVER':
            // ⭐ Итог приходит один раз и остаётся на экране: снимок состояния сюда
            // не годится — после матча стол уже пуст, а посмотреть, кто чем кончил, надо.
            table.result = envelope.payload;
            break;
        case 'MATCH_ABORTED':
            table.notice = 'Матч отменён: игрок не вернулся';
            clearTimeout(noticeTimer);
            break;
        case 'COMMAND_EXPIRED':
            // Ход ждал восстановления связи дольше таймаута хода: сервер уже сходил сам.
            notify('Ход не отправлен: связь вернулась слишком поздно');
            break;
        case 'TURN_TIMEOUT':
            notify('Ход не сделан вовремя — сервер сходил сам');
            break;
        case 'ERROR':
            // «Матч не идёт» — обычный ответ при входе в комнату ожидания, а не ошибка.
            if (envelope.payload?.code !== 'NO_MATCH') {
                notify(envelope.payload?.message ?? 'Ход отклонён');
            }
            break;
        default:
            break;
    }
}

function applySeatEvent(envelope) {
    if (!table.info) {
        return;
    }
    const {userId, displayName, seatNo, ready} = envelope.payload ?? {};
    const seats = table.info.seats.filter((seat) => seat.userId !== userId);
    if (envelope.type !== 'PLAYER_LEFT') {
        seats.push({seatNo, userId, displayName, ready: ready ?? false, online: true});
    }
    seats.sort((left, right) => left.seatNo - right.seatNo);
    table.info = {...table.info, seats};
}

async function loadManifest(cardSetId) {
    try {
        return (await apiGet(`/card-sets/${cardSetId}/manifest`)).cards ?? {};
    } catch {
        // Без картинок играть можно — карта покажется кодом.
        return {};
    }
}
