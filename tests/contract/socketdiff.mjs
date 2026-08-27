/**
 * Differential по СОКЕТУ — последний не гонявшийся уровень сравнения Java и Go.
 *
 * Две части, у каждой свой способ сравнивать:
 *
 * 1. **Детерминированные пробы** — рукопожатие, конверт, PING, ошибки, лобби. Здесь
 *    ответ не зависит от случайности, поэтому сравниваются НОРМАЛИЗОВАННЫЕ КОНВЕРТЫ
 *    ЦЕЛИКОМ: каждое поле, его наличие и отсутствие. Дыра в паре «есть/нет» — это
 *    ровно то, на чём падал фронт (NON_NULL-семантика, MD-003).
 *
 * 2. **Сыгранный матч** — боты играют матч на каждом бэкенде, записывается КАЖДЫЙ кадр
 *    каждому игроку. Карты в матчах разные по построению (MD-005), поэтому сравниваются
 *    не значения, а СХЕМЫ: какие типы сообщений бывают, какие поля конверта у каждого,
 *    какие пути есть в payload и каких типов там значения. Именно так ловится класс
 *    «hungCards вместо hung» — поле, переименованное только у одного бэкенда.
 *
 * ⭐ Инструмент не верит сам себе: `--selftest` кормит сравнение схем подделками
 *    (переименованное поле, пропавший тип, сменившийся тип значения) и требует, чтобы
 *    каждая была замечена, а различия карт и идентификаторов — нет. Самопроверка
 *    запускается перед каждым прогоном.
 *
 * ⚠️ Редкие механики (кость, потайной козырь, навесы) появляются в случайном матче
 *    не всегда. Если тип сообщения встретился только у одного бэкенда, прогон доигрывает
 *    дополнительные матчи (до MAX_MATCHES на бэкенд), и лишь потом называет это различием.
 *
 * Запуск (оба бэкенда обязаны смотреть в ОДНУ базу — как в matchdiff):
 *   OLD_BACKEND_URL=http://localhost:8098 NEW_BACKEND_URL=http://localhost:8088 \
 *     node tests/contract/socketdiff.mjs
 *   node tests/contract/socketdiff.mjs --selftest   # только самопроверка
 */

import http from 'node:http';
import {diff} from './lib.mjs';

const OLD = process.env.OLD_BACKEND_URL ?? 'http://localhost:8098';
const NEW = process.env.NEW_BACKEND_URL ?? 'http://localhost:8088';
const INVITE = process.env.BARDAK_INVITE ?? 'bardak-2026';
const PASSWORD = 'very-secret-password';
const RUN = process.env.RUN_TAG ?? String(Date.now()).slice(-7);

/** Сколько матчей доигрывать, если редкий тип сообщения встретился не у обоих. */
const MAX_MATCHES = Number(process.env.MAX_MATCHES ?? 4);

/** Столько тиков без единого хода считаем зависанием: боты отвечают мгновенно. */
const IDLE_LIMIT = 100;
const TICK_MS = 20;
const WAIT_MS = 8000;

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

// ── REST-обвязка ────────────────────────────────────────────────────────────

async function api(base, path, {method = 'GET', body, token} = {}) {
    const response = await fetch(base + '/api' + path, {
        method,
        headers: {
            'Content-Type': 'application/json',
            ...(token ? {Authorization: 'Bearer ' + token} : {}),
        },
        body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await response.text();
    if (!response.ok) {
        throw new Error(`${method} ${path} на ${base} -> ${response.status} ${text}`);
    }
    return text ? JSON.parse(text) : null;
}

/** Аккаунт на КОНКРЕТНОМ бэкенде: токены между бэкендами не носим (см. matchdiff). */
async function account(base, username) {
    try {
        return await api(base, '/auth/register', {method: 'POST',
            body: {username, displayName: username, password: PASSWORD, inviteCode: INVITE}});
    } catch {
        return await api(base, '/auth/login', {method: 'POST', body: {username, password: PASSWORD}});
    }
}

async function wsTicket(base, token) {
    const {ticket} = await api(base, '/auth/ws-ticket', {method: 'POST', body: {}, token});
    return ticket;
}

/**
 * Рукопожатие руками, чтобы увидеть HTTP-статус ОТКАЗА: браузерный WebSocket статус
 * прячет, а контракт требует ровно 401 до апгрейда.
 */
function upgradeStatus(base, query) {
    const url = new URL(base);
    return new Promise((resolve) => {
        const request = http.request({
            host: url.hostname,
            port: url.port,
            path: '/ws' + query,
            headers: {
                Connection: 'Upgrade',
                Upgrade: 'websocket',
                'Sec-WebSocket-Version': '13',
                'Sec-WebSocket-Key': 'c2FtcGxlIG5vbmNlMTIzNA==',
            },
        });
        request.on('response', (response) => {
            response.resume();
            resolve(response.statusCode);
        });
        request.on('upgrade', (_response, socket) => {
            socket.destroy();
            resolve(101);
        });
        request.on('error', () => resolve(0));
        request.end();
    });
}

// ── Сокет с записью всех кадров ─────────────────────────────────────────────

class Socket {
    constructor(base) {
        this.base = base;
        /** Все входящие кадры по порядку: {envelope, raw}. Ничего не выбрасывается. */
        this.frames = [];
        this.cursor = 0;
        this.closed = false;
    }

    async connect(token) {
        const ticket = await wsTicket(this.base, token);
        this.ws = new WebSocket(this.base.replace(/^http/, 'ws')
            + `/ws?ticket=${encodeURIComponent(ticket)}`);
        await new Promise((ok, fail) => {
            this.ws.addEventListener('open', ok, {once: true});
            this.ws.addEventListener('error', () => fail(new Error('сокет не открылся')), {once: true});
        });
        this.ws.addEventListener('message', (event) => {
            let envelope = null;
            try {
                envelope = JSON.parse(event.data);
            } catch {
                // Не-JSON от сервера — тоже наблюдение, кадр сохраняется сырым.
            }
            this.frames.push({envelope, raw: String(event.data)});
        });
        this.ws.addEventListener('close', () => {
            this.closed = true;
        });
        return this;
    }

    /** Отправить конверт; строка уходит как есть (для проб битого JSON). */
    send(message) {
        this.ws.send(typeof message === 'string' ? message : JSON.stringify(
            {v: 1, ts: Date.now(), ...message}));
    }

    /**
     * Дождаться кадра по условию, читая С ТЕКУЩЕГО МЕСТА: пробы идут по очереди,
     * и уже рассмотренный кадр не должен отвечать на следующий вопрос.
     */
    async next(predicate, timeoutMs = WAIT_MS) {
        const deadline = Date.now() + timeoutMs;
        while (Date.now() < deadline) {
            while (this.cursor < this.frames.length) {
                const frame = this.frames[this.cursor++];
                if (predicate(frame.envelope, frame.raw)) {
                    return frame;
                }
            }
            await sleep(10);
        }
        return null;
    }

    /** Пропустить всё накопившееся: следующая проба начинает с чистого места. */
    drain() {
        this.cursor = this.frames.length;
    }

    close() {
        try {
            this.ws.close();
        } catch {
            // Уже закрыт.
        }
    }
}

// ── Нормализация кадров для детерминированных проб ──────────────────────────

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const UUID_ANYWHERE_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi;

/**
 * Маскируются только заведомо динамические значения. `id` НЕ маскируется:
 * идентификаторы команд в пробах наши собственные и детерминированные, а `id`
 * в ответе — часть контракта (ERROR и PONG обязаны вернуть его).
 */
const VOLATILE_KEYS = new Set([
    'ts', 'sessionId', 'userId', 'displayName', 'username', 'matchId',
    'turnMillisLeft', 'turnSecondsLeft', 'fromName', 'tableName', 'tableCode',
]);

function normalizeFrame(value, key) {
    if (Array.isArray(value)) {
        return value.map((item) => normalizeFrame(item, key));
    }
    if (value && typeof value === 'object') {
        const out = {};
        for (const name of Object.keys(value).sort()) {
            out[name] = normalizeFrame(value[name], name);
        }
        return out;
    }
    if (VOLATILE_KEYS.has(key)) {
        return '<переменное>';
    }
    if (typeof value === 'string') {
        // UUID и целиком, и внутри текста ошибки: «Игрок не за этим столом: <uuid>».
        return value.replace(UUID_ANYWHERE_RE, '<uuid>');
    }
    return value;
}

/** Кадр как наблюдение: либо нормализованный конверт, либо признак таймаута. */
function observed(frame) {
    if (!frame) return {'нет ответа': true};
    if (!frame.envelope) return {'не-json': frame.raw.slice(0, 120)};
    return normalizeFrame(frame.envelope);
}

// ── Часть 1: детерминированные пробы ────────────────────────────────────────

/**
 * Каждая проба возвращает наблюдения `{метка: значение}`. Пробы исполняются на каждом
 * бэкенде НЕЗАВИСИМО (своё окружение: пользователи, стол), а сравниваются по меткам.
 */
async function runProbes(base, side) {
    const out = {};
    // ⚠️ База ОДНА на оба бэкенда, поэтому логины обязаны различаться меткой стороны:
    // игрок, севший за стол в прогоне старого, для нового уже «за другим столом».
    const tag = `sd_${RUN}_${side}`;
    const u1 = await account(base, `${tag}_p1`);
    const u2 = await account(base, `${tag}_p2`);

    // Рукопожатие: три способа не пройти, все обязаны кончаться одинаково.
    out['handshake: без тикета'] = await upgradeStatus(base, '');
    out['handshake: выдуманный тикет'] = await upgradeStatus(base, '?ticket=not-a-real-ticket');
    const burnt = await wsTicket(base, u1.accessToken);
    {
        const probe = new WebSocket(base.replace(/^http/, 'ws')
            + `/ws?ticket=${encodeURIComponent(burnt)}`);
        await new Promise((resolve) => {
            probe.addEventListener('open', resolve, {once: true});
            probe.addEventListener('error', resolve, {once: true});
        });
        probe.close();
        await sleep(100);
    }
    out['handshake: тикет использован повторно'] =
        await upgradeStatus(base, `?ticket=${encodeURIComponent(burnt)}`);

    const socket = await new Socket(base).connect(u1.accessToken);

    out['CONNECTED'] = observed(await socket.next((e) => e?.type === 'CONNECTED'));

    // Транспортный слой: конверт, версия, тип, heartbeat.
    socket.send({id: 'p-ping', type: 'PING'});
    const pong = await socket.next((e) => e?.type === 'PONG');
    out['PING -> PONG'] = observed(pong);
    out['PONG без ключа payload'] = pong ? !pong.raw.includes('"payload"') : null;

    socket.send({id: 'p-ping-t', type: 'PING', tableId: '3f0e39b2-58b7-4b02-9e46-6c7e69b3da11'});
    out['PING со столом -> PONG'] = observed(await socket.next((e) => e?.type === 'PONG'));

    socket.send({v: 2, id: 'p-v2', type: 'PING', tableId: '3f0e39b2-58b7-4b02-9e46-6c7e69b3da11'});
    out['версия 2'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    socket.send(JSON.stringify({id: 'p-nov', type: 'PING', ts: Date.now()}));
    out['без версии'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    socket.send({id: 'p-notype', tableId: '3f0e39b2-58b7-4b02-9e46-6c7e69b3da11'});
    out['без типа'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    socket.send({id: 'p-blank', type: '   '});
    out['пустой тип'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    socket.send('это не json');
    out['битый JSON'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    // Неизвестный тип: в эталоне это ЭХО — наследие M1, но контракт есть контракт.
    socket.send({id: 'p-echo', type: 'NO_SUCH_TYPE', payload: {x: 1}});
    out['неизвестный тип'] = observed(await socket.next(
        (e) => e && e.type !== 'PONG' && e.type !== 'CONNECTED'));

    socket.send({id: 'p-echo-null', type: 'NO_SUCH_TYPE', payload: null});
    const echoNull = await socket.next((e) => e && e.type !== 'PONG');
    out['неизвестный тип с payload null'] = observed(echoNull);

    // Идентификатор стола: отсутствует / не-UUID / несуществующий.
    socket.send({id: 'p-notable', type: 'PLAY_CARD', payload: {cardCode: '6-hearts'}});
    out['игровая команда без стола'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    socket.send({id: 'p-badtable', type: 'PLAY_CARD', tableId: 'abc',
        payload: {cardCode: '6-hearts'}});
    out['игровая команда с кривым столом'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    socket.send({id: 'p-lobby-notable', type: 'TABLE_JOIN'});
    out['команда лобби без стола'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    const ghost = '11111111-2222-4333-8444-555555555555';
    socket.send({id: 'p-ghost-join', type: 'TABLE_JOIN', tableId: ghost});
    out['вход в несуществующий стол'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    socket.send({id: 'p-ghost-play', type: 'PLAY_CARD', tableId: ghost,
        payload: {cardCode: '6-hearts'}});
    out['ход за несуществующим столом'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    // Лобби на настоящем столе: вход, готовность, ранний старт, выход, обрыв.
    const table = await api(base, '/tables', {method: 'POST', token: u1.accessToken,
        body: {name: 'socketdiff', maxPlayers: 3, rulesConfig: {}, isPrivate: false}});

    socket.drain();
    socket.send({id: 'p-join1', type: 'TABLE_JOIN', tableId: table.id});
    out['свой вход за стол'] = observed(await socket.next((e) => e?.type === 'PLAYER_JOINED'));

    const socket2 = await new Socket(base).connect(u2.accessToken);
    await socket2.next((e) => e?.type === 'CONNECTED');
    socket2.send({id: 'p-join2', type: 'TABLE_JOIN', tableId: table.id});
    out['чужой вход за стол'] = observed(await socket.next((e) => e?.type === 'PLAYER_JOINED'));

    socket2.send({id: 'p-ready-empty', type: 'TABLE_READY', tableId: table.id});
    out['готовность без payload'] = observed(await socket.next((e) => e?.type === 'PLAYER_READY'));

    socket2.send({id: 'p-ready-off', type: 'TABLE_READY', tableId: table.id,
        payload: {ready: false}});
    out['готовность снята'] = observed(await socket.next((e) => e?.type === 'PLAYER_READY'));

    socket.send({id: 'p-early', type: 'MATCH_START', tableId: table.id});
    out['старт неготового стола'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    socket.send({id: 'p-sync-nomatch', type: 'STATE_REQUEST', tableId: table.id});
    out['снимок без матча'] = observed(await socket.next((e) => e?.type === 'ERROR'));

    socket2.send({id: 'p-leave', type: 'TABLE_LEAVE', tableId: table.id});
    out['уход из-за стола'] = observed(await socket.next((e) => e?.type === 'PLAYER_LEFT'));
    // ⭐ Уходящий получает СВОЁ событие: рассылка идёт до отписки.
    out['уходящий видит свой уход'] = observed(await socket2.next((e) => e?.type === 'PLAYER_LEFT'));

    // Обрыв сокета сидящего за столом → остальные видят PLAYER_OFFLINE.
    socket2.send({id: 'p-rejoin', type: 'TABLE_JOIN', tableId: table.id});
    await socket.next((e) => e?.type === 'PLAYER_JOINED');
    socket2.close();
    out['обрыв соседа'] = observed(await socket.next((e) => e?.type === 'PLAYER_OFFLINE'));

    // Стол освобождается: сидящий «где-то» игрок ломал бы следующие прогоны.
    socket.send({id: 'p-bye', type: 'TABLE_LEAVE', tableId: table.id});
    await socket.next((e) => e?.type === 'PLAYER_LEFT');
    socket.close();
    return out;
}

// ── Часть 2: сыгранный матч и сравнение схем ────────────────────────────────

class MatchBot {
    constructor(socket, tableId) {
        this.socket = socket;
        this.tableId = tableId;
        this.actions = [];
        this.hand = [];
        this.over = false;
        this.counter = 0;
    }

    watch() {
        this.socket.ws.addEventListener('message', (event) => {
            let envelope;
            try {
                envelope = JSON.parse(event.data);
            } catch {
                return;
            }
            if (envelope.type === 'STATE_SYNC') {
                this.actions = envelope.payload.availableActions ?? [];
                this.hand = envelope.payload.myHand ?? [];
            } else if (envelope.type === 'MATCH_OVER') {
                this.over = true;
            }
        });
    }

    send(type, payload = {}) {
        this.socket.send({id: `sd-${RUN}-${++this.counter}-${Math.random()}`, type,
            tableId: this.tableId, payload});
    }

    /** «Беру» и «пас» выбираются последними, иначе матч кончится, не начавшись. */
    step() {
        if (!this.actions.length) return false;
        const rank = (action) => (action.type === 'TAKE' ? 3 : action.type === 'PASS' ? 2 : 1);
        const best = Math.min(...this.actions.map(rank));
        const pool = this.actions.filter((action) => rank(action) === best);
        const chosen = pool[Math.floor(Math.random() * pool.length)];
        this.actions = [];
        this.send(chosen.type, chosen.payload ?? {});
        return true;
    }
}

/** Карта, которой в руке заведомо нет: для детерминированного отказа движка. */
function cardNotInHand(hand) {
    for (const suit of ['hearts', 'diamonds', 'clubs', 'spades']) {
        for (const rank of ['6', '7', '8', '9', '10', 'J', 'Q', 'K', 'A']) {
            const code = `${rank}-${suit}`;
            if (!hand.includes(code)) return code;
        }
    }
    return '6-hearts';
}

/**
 * Сыграть матч на одном бэкенде, записав каждый кадр каждого участника.
 * По пути — детерминированные наблюдения, возможные только в живом матче.
 */
async function playRecordedMatch(base, tag, observations) {
    const names = [`sm_${RUN}_${tag}_a`, `sm_${RUN}_${tag}_b`, `sm_${RUN}_${tag}_o`];
    const [a, b, watcher] = await Promise.all(names.map((name) => account(base, name)));

    const table = await api(base, '/tables', {method: 'POST', token: a.accessToken,
        body: {name: `socketdiff ${tag}`, maxPlayers: 2, rulesConfig: {}, isPrivate: false}});

    const sockets = [await new Socket(base).connect(a.accessToken),
        await new Socket(base).connect(b.accessToken)];
    const bots = sockets.map((socket) => new MatchBot(socket, table.id));
    bots.forEach((bot) => bot.watch());
    for (const bot of bots) {
        bot.send('TABLE_JOIN');
        bot.send('TABLE_READY', {ready: true});
    }
    await sleep(500);
    bots[0].send('MATCH_START');
    await sleep(700);

    // Наблюдатель: подключён, но не за столом. Снимок ему не положен.
    const observerSocket = await new Socket(base).connect(watcher.accessToken);
    if (observations) {
        observerSocket.send({id: 'p-observer', type: 'STATE_REQUEST', tableId: table.id});
        // Ошибка ИЛИ снимок: если бэкенд вдруг отдаёт наблюдателю чужую проекцию,
        // это должно всплыть различием кадров, а не тихим таймаутом.
        observations['снимок для не-игрока'] = observed(await observerSocket.next(
            (e) => e?.type === 'ERROR' || e?.type === 'STATE_SYNC'));
    }

    let idle = 0;
    let moves = 0;
    // Пробы посреди матча идут по одной и ЖДУТ своего момента: атакующий с правом
    // хода существует не на каждом тике, и одноразовая попытка на Java промахивалась.
    const probeState = {rejector: null};
    const pendingProbes = observations ? [tryRejectionProbes, tryIdempotencyProbe] : [];
    let resyncProbed = observations === null;
    for (let tick = 0; tick < 20000; tick++) {
        if (bots.some((bot) => bot.over)) break;

        if (moves > 20 && pendingProbes.length) {
            if (await pendingProbes[0](bots, observations, probeState)) {
                pendingProbes.shift();
            }
        } else if (moves > 20 && !resyncProbed) {
            resyncProbed = true;
            await resyncProbe(bots, sockets, observations, probeState);
        }

        if (bots.map((bot) => bot.step()).some(Boolean)) {
            moves++;
            idle = 0;
        } else if (++idle > IDLE_LIMIT) {
            sockets.forEach((socket) => socket.close());
            observerSocket.close();
            throw new Error(`${tag}: матч встал после ${moves} ходов`);
        }
        await sleep(TICK_MS);
    }
    if (!bots.some((bot) => bot.over)) {
        sockets.forEach((socket) => socket.close());
        observerSocket.close();
        throw new Error(`${tag}: матч не кончился за отведённые ходы`);
    }
    await sleep(400);

    sockets.forEach((socket) => socket.close());
    observerSocket.close();
    console.log(`  сыграно на ${base}: ходов ${moves}`);
    return [...sockets, observerSocket].flatMap((socket) => socket.frames);
}

/**
 * Отказ движка и повтор отклонённой команды. Ждёт атакующего с правом хода —
 * вернуть `false` значит «попробуй на следующем тике».
 */
async function tryRejectionProbes(bots, observations, state) {
    const active = bots.find((bot) => bot.actions.some(
        (action) => action.type === 'PLAY_CARD' && !action.payload?.targetCardCode));
    if (!active) return false;

    // Детерминированный отказ: ход картой, которой нет в руке. Он же кладёт
    // MOVE_REJECTED в лог — иначе догон при RESYNC у бэкендов различался бы
    // из-за случайности, а не из-за различий.
    await sleep(200);
    active.socket.drain();
    const id = `sd-rej-${RUN}`;
    const badCard = cardNotInHand(active.hand);
    active.socket.send({id, type: 'PLAY_CARD', tableId: active.tableId,
        payload: {cardCode: badCard}});
    observations['ход чужой картой'] = observed(await active.socket.next(
        (e) => e?.type === 'ERROR'));

    // Повтор ОТКЛОНЁННОЙ команды: отказ не запоминается, клиент получает причину
    // снова. ⚠️ Справочник (websocket-contract.md §9.3) описывает обратное — Java
    // изменилась ПОСЛЕ снятия эталона (см. комментарий в GameCommandHandler), и оба
    // бэкенда обязаны совпадать в новом поведении.
    await sleep(200);
    active.socket.drain();
    active.socket.send({id, type: 'PLAY_CARD', tableId: active.tableId,
        payload: {cardCode: badCard}});
    observations['повтор отклонённой команды'] = observed(await active.socket.next(
        (e) => e?.type === 'STATE_SYNC' || (e?.type === 'ERROR' && e?.id === id), 4000));
    state.rejector = active;
    return true;
}

/** Повтор ПРИМЕНЁННОЙ команды с тем же id: ответ — снимок, не второй ход. */
async function tryIdempotencyProbe(bots, observations) {
    const mover = bots.find((bot) => bot.actions.length);
    if (!mover) return false;

    const chosen = mover.actions.find((action) => action.type === 'PASS')
        ?? mover.actions[0];
    const id = `sd-idem-${RUN}`;
    mover.socket.send({id, type: chosen.type, tableId: mover.tableId,
        payload: chosen.payload ?? {}});
    await mover.socket.next((e) => e?.type === 'STATE_SYNC');
    await sleep(200);
    mover.socket.drain();
    mover.socket.send({id, type: chosen.type, tableId: mover.tableId,
        payload: chosen.payload ?? {}});
    const reply = await mover.socket.next(
        (e) => e?.type === 'STATE_SYNC' || (e?.type === 'ERROR' && e?.id === id));
    observations['повтор команды: тип ответа'] = reply?.envelope?.type ?? null;
    mover.actions = [];
    return true;
}

/**
 * RESYNC с нуля: догон всего лога (включая MOVE_REJECTED от пробы отказа) плюс
 * снимок. Кадры догона попадают в общий корпус и сравняются по схеме.
 */
async function resyncProbe(bots, sockets, observations, state) {
    // Догоняется автор отказа: MOVE_REJECTED записан видимым только ему (§8),
    // и RESYNC с чужого места не показал бы его вовсе.
    const requester = (state.rejector ?? bots[0]).socket;
    const other = sockets.find((socket) => socket !== requester) ?? sockets[1];
    requester.drain();
    requester.send({id: `sd-resync-${RUN}`, type: 'RESYNC', tableId: bots[0].tableId,
        payload: {lastSeq: 0}});
    const caughtUp = await requester.next((e) => e?.type === 'STATE_SYNC');
    observations['RESYNC: снимок пришёл'] = caughtUp !== null;
    observations['RESYNC: возобновление разослано'] =
        (await other.next((e) => e?.type === 'MATCH_RESUMED', 3000)) !== null;
}

// ── Схема корпуса кадров ────────────────────────────────────────────────────

const ENVELOPE_KEYS = ['v', 'id', 'type', 'tableId', 'seq', 'ts', 'payload'];

/** Пути, по которым сравниваются и ЗНАЧЕНИЯ (словари конечны и детерминированы). */
const VOCABULARY = new Set([
    'STATE_SYNC payload.phase',
    'STATE_SYNC payload.availableActions[].type',
]);

function kindOf(value) {
    if (value === null) return 'null';
    if (Array.isArray(value)) return 'массив';
    return typeof value === 'object' ? 'объект' : typeof value;
}

/**
 * Схема корпуса: для каждого типа сообщения — сколько раз встретился, какие поля
 * конверта при нём бывают, и какие пути с какими видами значений живут в payload.
 */
function buildSchema(frames) {
    const schema = new Map();
    for (const frame of frames) {
        const envelope = frame.envelope ?? frame;
        if (!envelope?.type) continue;
        let entry = schema.get(envelope.type);
        if (!entry) {
            entry = {count: 0, envelope: new Map(), paths: new Map(), vocabulary: new Map()};
            schema.set(envelope.type, entry);
        }
        entry.count++;
        for (const key of ENVELOPE_KEYS) {
            entry.envelope.set(key, (entry.envelope.get(key) ?? 0) + (key in envelope ? 1 : 0));
        }
        if ('payload' in envelope) {
            walk(entry, envelope.type, 'payload', envelope.payload);
        }
    }
    return schema;
}

function walk(entry, type, path, value) {
    const kind = kindOf(value);
    let node = entry.paths.get(path);
    if (!node) {
        node = {kinds: new Set(), count: 0, children: 0};
        entry.paths.set(path, node);
    }
    node.count++;
    node.kinds.add(kind);

    if (kind === 'массив') {
        for (const item of value) {
            walk(entry, type, path + '[]', item);
        }
    } else if (kind === 'объект') {
        node.children++;
        for (const key of Object.keys(value)) {
            walk(entry, type, `${path}.${key}`, value[key]);
        }
    } else if (typeof value === 'string' && VOCABULARY.has(`${type} ${path}`)) {
        const seen = entry.vocabulary.get(path) ?? new Set();
        seen.add(value);
        entry.vocabulary.set(path, seen);
    }
}

/** Наличие как класс: сравнивать счётчики двух разных матчей бессмысленно. */
function presenceClass(count, total) {
    if (count === 0) return 'никогда';
    return count === total ? 'всегда' : 'иногда';
}

/** Родительский путь: у `payload.a.b` это `payload.a`, у `payload.a[]` — `payload.a`. */
function parentPath(path) {
    const cut = Math.max(path.lastIndexOf('.'), path.endsWith('[]') ? path.length - 2 : -1);
    return cut > 0 ? path.slice(0, cut) : null;
}

/**
 * Сравнить схемы двух корпусов. Возвращает [различия, встретившиеся не у обоих типы]:
 * вторые — кандидаты на дополнительный матч, а не приговор.
 */
function compareSchemas(left, right) {
    const problems = [];
    const oneSided = [];

    for (const type of new Set([...left.keys(), ...right.keys()])) {
        const l = left.get(type);
        const r = right.get(type);
        if (!l || !r) {
            oneSided.push(`${type}: ${l ? 'есть у старого, нет у нового' : 'есть у нового, нет у старого'}`);
            continue;
        }

        for (const key of ENVELOPE_KEYS) {
            const lClass = presenceClass(l.envelope.get(key) ?? 0, l.count);
            const rClass = presenceClass(r.envelope.get(key) ?? 0, r.count);
            if (lClass !== rClass) {
                problems.push(`${type} · конверт.${key}: у старого ${lClass}, у нового ${rClass}`);
            }
        }

        for (const path of new Set([...l.paths.keys(), ...r.paths.keys()])) {
            const lNode = l.paths.get(path);
            const rNode = r.paths.get(path);
            // ⚠️ Путь, виденный только одной стороной, — кандидат на РЕДКОСТЬ, а не
            // сразу различие: nullable-поле или поле редкой фазы могло не выпасть
            // в этом матче. Настоящее переименование от доигрывания не сойдётся
            // и будет названо после MAX_MATCHES.
            if (!lNode || !rNode) {
                oneSided.push(`${type} · ${path}: ${lNode
                    ? 'есть у старого, у нового не встретился'
                    : 'нет у старого, у нового встретился'}`);
                continue;
            }
            const lKinds = [...lNode.kinds].sort();
            const rKinds = [...rNode.kinds].sort();
            if (lKinds.join('|') !== rKinds.join('|')) {
                const subset = lKinds.every((kind) => rNode.kinds.has(kind))
                    || rKinds.every((kind) => lNode.kinds.has(kind));
                // Подмножество (string против null|string) — та же редкость;
                // несовместимые виды (number против string) — различие сразу.
                (subset ? oneSided : problems).push(`${type} · ${path}: вид значения — `
                    + `${lKinds.join('|')} против ${rKinds.join('|')}`);
            }
            // Наличие поля внутри объекта: доля появлений от появлений родителя.
            const parent = parentPath(path);
            const lParent = parent ? l.paths.get(parent) : null;
            const rParent = parent ? r.paths.get(parent) : null;
            if (lParent?.children && rParent?.children && !parent.endsWith('[]')) {
                const lClass = presenceClass(lNode.count, lParent.children);
                const rClass = presenceClass(rNode.count, rParent.children);
                if (lClass !== rClass) {
                    oneSided.push(`${type} · ${path}: наличие — у старого ${lClass}, `
                        + `у нового ${rClass}`);
                }
            }
        }

        for (const path of new Set([...l.vocabulary.keys(), ...r.vocabulary.keys()])) {
            const lSeen = l.vocabulary.get(path) ?? new Set();
            const rSeen = r.vocabulary.get(path) ?? new Set();
            for (const value of new Set([...lSeen, ...rSeen])) {
                if (!lSeen.has(value) || !rSeen.has(value)) {
                    oneSided.push(`${type} · ${path} = «${value}»: `
                        + `${lSeen.has(value) ? 'только у старого' : 'только у нового'}`);
                }
            }
        }
    }
    return [problems, oneSided];
}

// ── Самопроверка ────────────────────────────────────────────────────────────

/** Корпус-образец: маленький, но с теми же слоями, что настоящий. */
function sampleCorpus(tweak = {}) {
    const state = (hand, extra = {}) => ({
        v: 1, type: 'STATE_SYNC', tableId: 't', ts: 1,
        payload: {
            phase: 'ATTACK', myHand: hand, mySeat: 0, deckLeft: 10,
            players: [{seatNo: 0, [tweak.hungField ?? 'hung']: [], passed: false}],
            availableActions: [{type: 'PLAY_CARD', payload: {cardCode: hand[0]}}],
            ...extra,
        },
    });
    const frames = [
        {v: 1, type: 'CONNECTED', ts: 1, payload: {sessionId: 's', protocolVersion: 1}},
        state(['6-hearts'], tweak.trumpAlways ? {trumpSuit: 'HEARTS'} : {}),
        state(['A-spades'], tweak.trumpAlways ? {trumpSuit: 'HEARTS'} : {}),
        state(['7-clubs'], {trumpSuit: 'HEARTS'}),
        {v: 1, type: 'CARD_ATTACKED', tableId: 't',
            ...(tweak.noSeq ? {} : {seq: 1}), ts: 2,
            payload: {seatNo: 0, cardCode: tweak.card ?? '6-hearts'}},
        {v: 1, type: 'MATCH_OVER', tableId: 't', ts: 3,
            payload: {matchId: 'm', dealsPlayed: tweak.dealsAsString ? '3' : 3,
                players: [{seatNo: 0, place: 1, lossDegree: null}]}},
    ];
    if (!tweak.dropPassed) {
        frames.push({v: 1, type: 'PASSED', tableId: 't', seq: 2, ts: 4, payload: {seatNo: 1}});
    }
    return frames.map((envelope) => ({envelope, raw: JSON.stringify(envelope)}));
}

function selftest() {
    const failures = [];
    const base = buildSchema(sampleCorpus());

    const expectFound = (label, tweak, needle) => {
        const [problems, oneSided] = compareSchemas(base, buildSchema(sampleCorpus(tweak)));
        const all = [...problems, ...oneSided];
        if (!all.some((line) => line.includes(needle))) {
            failures.push(`${label}: различие НЕ замечено (${needle})`);
        }
    };

    expectFound('переименованное поле', {hungField: 'hungCards'}, 'hung');
    expectFound('пропавший тип сообщения', {dropPassed: true}, 'PASSED');
    expectFound('сменившийся тип значения', {dealsAsString: true}, 'dealsPlayed');
    expectFound('пропавший seq', {noSeq: true}, 'конверт.seq');
    expectFound('поле стало обязательным', {trumpAlways: true}, 'trumpSuit');

    const [clean, cleanSided] = compareSchemas(base, buildSchema(sampleCorpus({card: 'A-clubs'})));
    if (clean.length || cleanSided.length) {
        failures.push(`разные карты приняты за различие: ${[...clean, ...cleanSided][0]}`);
    }

    const normalized = normalizeFrame({v: 1, id: 'p-1', type: 'ERROR', ts: 123,
        payload: {code: 'NO_MATCH', message: 'нет', userId: 'кто-то'}});
    if (normalized.id !== 'p-1' || normalized.payload.code !== 'NO_MATCH'
        || normalized.ts !== '<переменное>' || normalized.payload.userId !== '<переменное>') {
        failures.push('нормализация кадра: маскирует не то или не маскирует нужное');
    }

    return failures;
}

// ── Прогон ──────────────────────────────────────────────────────────────────

const failedSelfChecks = selftest();
if (failedSelfChecks.length) {
    console.error('❌ Самопроверка инструмента провалена:');
    failedSelfChecks.forEach((line) => console.error('   ' + line));
    process.exit(2);
}
console.log('✅ самопроверка: подделки замечены, случайность — нет');
if (process.argv.includes('--selftest')) {
    process.exit(0);
}

async function reachable(base) {
    try {
        return (await api(base, '/health'))?.status === 'UP';
    } catch {
        return false;
    }
}

for (const [label, base] of [['старый', OLD], ['новый', NEW]]) {
    if (!await reachable(base)) {
        console.error(`❌ ${label} бэкенд (${base}) не отвечает`);
        process.exit(2);
    }
}
console.log(`старый: ${OLD}`);
console.log(`новый:  ${NEW}\n`);

let failed = 0;

console.log('▸ детерминированные пробы: конверт, ошибки, лобби');
const oldProbes = await runProbes(OLD, 'o');
const newProbes = await runProbes(NEW, 'n');
{
    const problems = [];
    for (const label of new Set([...Object.keys(oldProbes), ...Object.keys(newProbes)])) {
        problems.push(...diff(oldProbes[label], newProbes[label])
            .map((line) => `${label} · ${line}`));
    }
    if (problems.length) {
        failed++;
        console.log(`❌ различий: ${problems.length}`);
        problems.forEach((line) => console.log('   ' + line));
    } else {
        console.log('✅ все пробы совпали');
    }
}

console.log('\n▸ сыгранный матч: схема всех кадров всех участников');
const oldObservations = {};
const newObservations = {};
let oldFrames = await playRecordedMatch(OLD, 'o1', oldObservations);
let newFrames = await playRecordedMatch(NEW, 'n1', newObservations);

{
    const problems = [];
    for (const label of new Set([...Object.keys(oldObservations), ...Object.keys(newObservations)])) {
        problems.push(...diff(oldObservations[label], newObservations[label])
            .map((line) => `${label} · ${line}`));
    }
    if (problems.length) {
        failed++;
        console.log(`❌ пробы посреди матча разошлись: ${problems.length}`);
        problems.forEach((line) => console.log('   ' + line));
    } else {
        console.log('✅ пробы посреди матча совпали');
    }
}

// Схемы: если редкий тип встретился не у обоих — доиграть, а не объявлять различие.
let matchesPlayed = 1;
let [schemaProblems, oneSided] = compareSchemas(buildSchema(oldFrames), buildSchema(newFrames));
while (oneSided.length && matchesPlayed < MAX_MATCHES) {
    matchesPlayed++;
    console.log(`  редкое встретилось не у обоих (${oneSided.length}) — матч №${matchesPlayed}`);
    oldFrames = oldFrames.concat(await playRecordedMatch(OLD, `o${matchesPlayed}`, null));
    newFrames = newFrames.concat(await playRecordedMatch(NEW, `n${matchesPlayed}`, null));
    [schemaProblems, oneSided] = compareSchemas(buildSchema(oldFrames), buildSchema(newFrames));
}

if (schemaProblems.length || oneSided.length) {
    failed++;
    console.log(`❌ схемы кадров разошлись (${schemaProblems.length + oneSided.length}), `
        + `матчей сыграно ${matchesPlayed} на бэкенд:`);
    schemaProblems.forEach((line) => console.log('   ' + line));
    oneSided.forEach((line) => console.log('   [не у обоих] ' + line));
} else {
    console.log(`✅ схемы кадров совпали (матчей на бэкенд: ${matchesPlayed})`);
}

console.log(failed
    ? `\n❌ уровней с различиями: ${failed}`
    : '\n✅ различий нет: сокет двух бэкендов неотличим по форме на всех уровнях');
process.exit(failed ? 1 : 0);
