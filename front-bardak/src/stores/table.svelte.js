/**
 * Стол: комната ожидания и сама игра на одном соединении.
 *
 * ⭐ Состояние игры целиком приходит с сервера снимком `STATE_SYNC`. Клиент ничего
 * не досчитывает: ни что чем бьётся, ни какой ранг сейчас летит по шкале навесов.
 * Список разрешённых ходов тоже приходит с сервера (ADR-003) — фронт правил не знает.
 */

import {apiGet} from '../net/rest-client.js';
import {applyTableEvent} from './lobby.svelte.js';
import {connection, isConnected, onMessage, onReconnect, send as wsSend}
    from './connection.svelte.js';
import {TIMING, clearFlights, flyCard, rememberDraw, rememberOrigin, showSpotlight}
    from '../lib/motion.svelte.js';

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
    // ⭐ Состояние соединения не своё: сокет общий на приложение, и держать его копию
    // здесь значило бы завести второй источник правды об одном и том же.
    notice: null,      // пауза, отмена, таймаут — то, что нужно показать человеком
    result: null,      // итог матча: места, уровни, дельта рейтинга
    decisions: {},     // место → что игрок только что решил; живёт несколько секунд
    lastSeq: 0,
});

/**
 * Что игрок только что сделал — видно всем за столом.
 *
 * <p>⭐ Снимок состояния этого не отдаёт и отдавать не должен: «спасовал» и «отбился» —
 * это случившееся, а не положение дел. В снимке от них остаётся только след (флаг паса,
 * карта в слоте), по которому не понять, произошло это секунду назад или три хода назад.
 * Поэтому решения собираются из событий и сами гаснут.
 */
const DECISION_TTL_MS = 4000;

const decisionTimers = new Map();

function decided(seatNo, text, tone = 'plain') {
    if (seatNo === null || seatNo === undefined) {
        return;
    }
    table.decisions = {...table.decisions, [seatNo]: {text, tone}};
    clearTimeout(decisionTimers.get(seatNo));
    decisionTimers.set(seatNo, setTimeout(() => {
        const {[seatNo]: gone, ...rest} = table.decisions;
        table.decisions = rest;
        decisionTimers.delete(seatNo);
    }, DECISION_TTL_MS));
}

function forgetDecisions() {
    decisionTimers.forEach(clearTimeout);
    decisionTimers.clear();
    table.decisions = {};
}

/** Откуда вылетает карта этого места: своя рука — снизу, чужая — из-под аватара. */
function handOf(seatNo) {
    return table.game && seatNo === table.game.mySeat ? 'hand' : `seat-${seatNo}`;
}

/**
 * Карты, лежащие сейчас на столе, вместе с точкой, откуда каждая улетит.
 *
 * <p>Считается до применения снимка: через мгновение стол опустеет, и спросить будет уже
 * не у кого.
 */
function leavingCards() {
    return (table.game?.table ?? []).flatMap((slot) => [slot.attack, slot.defend]
            .filter(Boolean)
            .map((code) => ({code, from: `slot-${slot.attack}`})));
}

/**
 * ⭐ Соединение общее на приложение и столу не принадлежит: игрок сидит на сокете и в лобби,
 * иначе друзья не видели бы его в сети, а приглашение за стол было бы некуда доставить.
 * Стол лишь подписывается на сообщения и отписывается, когда уходит.
 */
let unsubscribe = null;

export async function enterTable(info) {
    // Возврат за тот же стол — просто просим снимок, ничего не переоткрывая.
    if (table.info?.id === info.id && unsubscribe) {
        wsSend('STATE_REQUEST', {}, info.id);
        return;
    }
    table.info = info;
    table.game = null;
    table.notice = null;
    table.result = null;
    forgetDecisions();
    clearFlights();
    table.manifest = await loadManifest(info.cardSetId);

    unsubscribe?.();
    const offMessage = onMessage(onEnvelope);
    const offReconnect = onReconnect(resync);
    unsubscribe = () => {
        offMessage();
        offReconnect();
    };
    wsSend('TABLE_JOIN', {}, info.id);
    // Если матч уже идёт — сервер пришлёт снимок; если нет, ответит, что матча нет.
    wsSend('STATE_REQUEST', {}, info.id);
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
    wsSend('RESYNC', {lastSeq: table.lastSeq}, table.info.id);
    notify('Связь восстановлена — догоняю стол');
}

/** Встать из-за стола совсем: место освобождается. Посреди матча сервер откажет. */
export function leaveTable() {
    if (table.info) {
        wsSend('TABLE_LEAVE', {}, table.info.id);
    }
    detachTable();
}

/**
 * Выйти из идущего матча.
 *
 * <p>⚠️ Это не то же самое, что закрыть вкладку: пропавшего ждут минуту и только потом
 * отменяют партию (§5.2). Здесь человек уходит сознательно — ждать некого, и матч
 * отменяется сразу у всех. В рейтинг он не пойдёт ни для кого (§5.3).
 */
export function leaveMatch() {
    if (!table.info) {
        return;
    }
    wsSend('MATCH_LEAVE', {}, table.info.id);
    table.game = null;
    table.result = null;
    forgetDecisions();
    clearFlights();
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
    // ⚠️ Сокет не закрываем: он общий на приложение. Стол только перестаёт слушать.
    unsubscribe?.();
    unsubscribe = null;
    table.info = null;
    table.game = null;
    table.result = null;
    forgetDecisions();
    clearFlights();
}

export function setReady(ready) {
    wsSend('TABLE_READY', {ready}, table.info.id);
}

export function startMatch() {
    table.result = null;
    table.game = null;
    forgetDecisions();
    clearFlights();
    wsSend('MATCH_START', {}, table.info.id);
}

/**
 * Ход: тип и payload берутся из `availableActions` — фронт их не сочиняет.
 *
 * ⚠️ Молчать про неотправленный ход нельзя. Раньше здесь стоял `client?.send(...)`: при
 * мёртвом сокете команда уходила в очередь, карта развыбиралась, кнопка исчезала — и всё
 * выглядело так, будто ход сделан. На деле стол не двигался, а игрок думал, что кнопки
 * сломаны. Теперь про это говорят вслух.
 */
export function play(action) {
    wsSend(action.type, action.payload ?? {}, table.info.id);
    if (!isConnected()) {
        notify('Связь пропала — ход уйдёт, как только она вернётся');
    }
}

function onEnvelope(envelope) {
    if (typeof envelope.seq === 'number') {
        // Пропуск в нумерации означает, что мы что-то не увидели: просим догон.
        if (envelope.seq > table.lastSeq + 1) {
            wsSend('RESYNC', {lastSeq: table.lastSeq}, table.info.id);
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
            applyTableEvent(envelope);
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
            // ⭐ Игровые события приходят перед снимком именно затем, чтобы их успели
            // показать: причина раньше следствия. Раньше их здесь просто выбрасывали.
            animateGameEvent(envelope);
            break;
    }
}

/**
 * Событие раздачи в движение карт и в подпись «что решил соперник».
 *
 * <p>⚠️ Координаты снимаются прямо здесь, пока на экране ещё старое состояние: снимок
 * придёт следующим сообщением. Любая отложенная обработка сломает точку вылета.
 */
function animateGameEvent(envelope) {
    if (!table.game) {
        return;
    }
    const {seatNo, cardCode, victimSeat, count} = envelope.payload ?? {};
    switch (envelope.type) {
        // ⭐ Прилетающие карты запоминают только точку вылета: лететь будет сам узел из
        // своего конечного места (см. flyFrom). Так карта садится ровно в свой слот, даже
        // если соседние карты при этом разъезжаются, и в конце не двоится.
        case 'CARD_ATTACKED':
            rememberOrigin(cardCode, handOf(seatNo), -4);
            decided(seatNo, table.game.table.length ? 'подкинул' : 'атакует', 'attack');
            break;
        case 'CARD_DEFENDED':
            rememberOrigin(cardCode, handOf(seatNo), 5);
            decided(seatNo, 'отбил', 'defend');
            break;
        case 'ATTACK_TRANSFERRED':
            rememberOrigin(cardCode, handOf(seatNo), -8);
            decided(seatNo, 'перевёл', 'attack');
            break;
        case 'CARD_HUNG':
            // Навес садится именно в слот жертвы: по нему потом читают, кто близок к джокеру.
            rememberOrigin(cardCode, handOf(seatNo), 12);
            decided(seatNo, 'навесил', 'hang');
            break;
        case 'CARDS_DRAWN':
            if (seatNo === table.game.mySeat) {
                // Свои карты приедут в руку сами; какие именно — станет известно из снимка.
                rememberDraw(count ?? 0, 'deck');
            } else {
                // Чужой добор показать нечем — рубашки летят призраком к его месту.
                for (let index = 0; index < (count ?? 0); index++) {
                    flyCard({from: 'deck', to: `seat-${seatNo}`, faceDown: true,
                        delay: index * TIMING.dealStagger, spin: -6});
                }
            }
            break;
        // ⚠️ Уходящим картам конечного узла не существует — они исчезают со стола. Только
        // здесь и нужен призрак поверх: лететь больше нечему. Вылетает каждая из своего
        // слота, а не из общего центра, иначе стол «схлопывается» в одну точку.
        case 'ROUND_BEATEN':
            leavingCards().forEach(({code, from}, index) => flyCard({
                from, to: 'discard', code, delay: index * 40, spin: index % 2 ? 14 : -11,
            }));
            decided(seatNo, 'бито', 'beaten');
            break;
        case 'CARDS_TAKEN':
            leavingCards().forEach(({code, from}, index) => flyCard({
                from, to: handOf(seatNo), code, delay: index * 50, spin: 6,
            }));
            decided(seatNo, 'забрал', 'take');
            break;
        case 'TAKE_ANNOUNCED':
            decided(seatNo, 'беру', 'take');
            break;
        case 'PASSED':
            decided(seatNo, 'пас', 'pass');
            break;
        case 'HIDDEN_TRUMP_REVEALED':
            // Козырь меняется всему столу — карту показывают крупно, а не строкой в логе.
            showSpotlight(cardCode);
            break;
        case 'TRUMP_CHOSEN':
        case 'TRUMP_CHANGED':
            decided(seatNo, 'козырь', 'attack');
            break;
        case 'PLAYER_LEFT_DEAL':
            decided(seatNo, 'вышел', 'pass');
            break;
        default:
            break;
    }
}

async function loadManifest(cardSetId) {
    try {
        return (await apiGet(`/card-sets/${cardSetId}/manifest`)).cards ?? {};
    } catch {
        // Без картинок играть можно — карта покажется кодом.
        return {};
    }
}
