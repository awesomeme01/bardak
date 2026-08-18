/**
 * Дымовая проверка: матч на N игроков против живого сервера.
 *
 * ⭐ Проверяет то, чего не видят юнит-тесты: движок, протокол, сокеты и база вместе.
 * Боты ходят случайно из того, что сервер сам объявил разрешённым, — то есть проверяется
 * ещё и согласованность `availableActions` с движком.
 *
 * ⚠️ Главное, что здесь ловится, — <b>тупик</b>: состояние, из которого ни один игрок
 * не может сходить, а матч не закончен. Именно так вставала раздача, когда спасовавший
 * сохранял право хода.
 *
 * Запуск: node tools/smoke/playmatch.mjs <игроков 2..5> [метка]
 */
const BASE = process.env.BARDAK_URL ?? 'http://localhost:8088';
const INVITE = process.env.BARDAK_INVITE ?? 'bardak-2026';
const PASSWORD = 'very-secret-password';

/** Столько тиков без единого хода считаем зависанием: боты отвечают мгновенно. */
const IDLE_LIMIT = 80;
const TICK_MS = 20;

const players = Number(process.argv[2] ?? 2);
const stamp = process.argv[3] ?? String(Date.now()).slice(-5);

if (!Number.isInteger(players) || players < 2 || players > 5) {
    console.error('Игроков должно быть от 2 до 5');
    process.exit(2);
}

const seen = {};
const rejections = {};

async function api(path, {method = 'GET', body, token} = {}) {
    const res = await fetch(BASE + '/api' + path, {
        method,
        headers: {'Content-Type': 'application/json', ...(token ? {Authorization: 'Bearer ' + token} : {})},
        body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    if (!res.ok) {
        throw new Error(`${method} ${path} -> ${res.status} ${text}`);
    }
    return text ? JSON.parse(text) : null;
}

/** Аккаунт бота: заводим, а если уже есть от прошлого прогона — просто входим. */
async function account(username) {
    try {
        return await api('/auth/register', {method: 'POST',
            body: {username, displayName: username, password: PASSWORD, inviteCode: INVITE}});
    } catch {
        return await api('/auth/login', {method: 'POST', body: {username, password: PASSWORD}});
    }
}

class Bot {
    constructor(who, tableId) {
        this.who = who;
        this.tableId = tableId;
        this.actions = [];
        this.over = false;
        this.phase = null;
    }

    async connect() {
        const {ticket} = await api('/auth/ws-ticket',
            {method: 'POST', body: {}, token: this.who.accessToken});
        const url = BASE.replace(/^http/, 'ws') + `/ws?ticket=${encodeURIComponent(ticket)}`;
        this.ws = new WebSocket(url);
        await new Promise((ok, fail) => {
            this.ws.addEventListener('open', ok, {once: true});
            this.ws.addEventListener('error', fail, {once: true});
        });
        this.ws.addEventListener('message', (raw) => this.#onMessage(JSON.parse(raw.data)));
    }

    #onMessage(envelope) {
        if (envelope.type === 'STATE_SYNC') {
            this.actions = envelope.payload.availableActions ?? [];
            this.phase = envelope.payload.phase;
        } else if (envelope.type === 'MATCH_OVER') {
            this.over = true;
        } else if (envelope.type === 'ERROR') {
            const code = envelope.payload?.code;
            if (code && code !== 'NO_MATCH') {
                rejections[code] = (rejections[code] ?? 0) + 1;
            }
        } else {
            seen[envelope.type] = (seen[envelope.type] ?? 0) + 1;
        }
    }

    send(type, payload = {}) {
        this.ws.send(JSON.stringify({v: 1, id: `smoke-${Math.random()}`, type,
            tableId: this.tableId, ts: Date.now(), payload}));
    }

    /**
     * Ходит охотно: «беру» и «пас» выбираются последними, иначе матч кончается,
     * не начавшись, и половина правил остаётся непроверенной.
     */
    step() {
        if (!this.actions.length) {
            return false;
        }
        const rank = (a) => (a.type === 'TAKE' ? 3 : a.type === 'PASS' ? 2 : 1);
        const best = Math.min(...this.actions.map(rank));
        const pool = this.actions.filter((a) => rank(a) === best);
        const action = pool[Math.floor(Math.random() * pool.length)];
        this.actions = [];
        this.send(action.type, action.payload ?? {});
        return true;
    }
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const accounts = [];
for (let index = 0; index < players; index++) {
    accounts.push(await account(`smoke${players}_${stamp}_${index}`));
}
const table = await api('/tables', {method: 'POST', token: accounts[0].accessToken,
    body: {name: `дым ${players}`, maxPlayers: players, rulesConfig: {}, isPrivate: false}});

const bots = accounts.map((who) => new Bot(who, table.id));
for (const bot of bots) {
    await bot.connect();
    bot.send('TABLE_JOIN');
    bot.send('TABLE_READY', {ready: true});
}
await sleep(500);
bots[0].send('MATCH_START');
await sleep(700);

let idle = 0;
let moves = 0;
for (let tick = 0; tick < 8000; tick++) {
    if (bots.some((bot) => bot.over)) {
        report(true, moves);
    }
    const moved = bots.map((bot) => bot.step()).some(Boolean);
    if (moved) {
        moves++;
        idle = 0;
    } else {
        idle++;
    }
    if (idle > IDLE_LIMIT) {
        console.log(`❌ ${players}: ТУПИК — ходов нет ни у кого, сделано ${moves}`);
        console.log(`   фазы: ${bots.map((bot, i) => `${i}:${bot.phase}`).join(' ')}`);
        process.exit(1);
    }
    await sleep(TICK_MS);
}
console.log(`❌ ${players}: матч не закончился, сделано ${moves} ходов`);
process.exit(1);

function report(ok, madeMoves) {
    const kinds = Object.entries(seen).sort((a, b) => b[1] - a[1]).slice(0, 8)
        .map(([type, count]) => `${type}:${count}`).join(' ');
    console.log(`✅ ${players} игрока(ов): матч завершён, ходов ${madeMoves}`);
    console.log(`   события: ${kinds}`);
    // ⚠️ Отказы — норма: бот считает ход по снимку, который мог устареть, пока летел.
    // Смотреть надо на КОДЫ: NOT_YOUR_TURN и CARD_NOT_IN_HAND — гонка бота, всё
    // остальное стоит разобрать.
    if (Object.keys(rejections).length) {
        console.log(`   отказы: ${Object.entries(rejections)
            .map(([code, count]) => `${code}:${count}`).join(' ')}`);
    }
    process.exit(ok ? 0 : 1);
}
