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

/** Шкала навесов как порядок: «что летит» сравнимо между раздачами. */
const SCALE = ['6', '7', '8', '9', '10', 'J', 'Q', 'K', 'A', 'JOKER'];
const flightOf = (p) => (p.nextIsJoker ? 'JOKER' : (p.nextNavesRank ?? '6'));
const stepOf = (flight) => SCALE.indexOf(flight);

/** Переходы между раздачами глазами бота 0: [{deal, before, after}]. */
const transitions = [];

async function api(path, {method = 'GET', body, token} = {}) {
    // ⚠️ Двери (вход, регистрация, тикет) огорожены пределом частоты, и прогон ботов
    // упирается в него сам: четыре состава подряд — это до 28 гостевых запросов.
    // 429 — не провал, а просьба подождать; ждём и повторяем.
    for (;;) {
        const res = await fetch(BASE + '/api' + path, {
            method,
            headers: {'Content-Type': 'application/json', ...(token ? {Authorization: 'Bearer ' + token} : {})},
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        if (res.status === 429) {
            const wait = Number(res.headers.get('Retry-After') ?? 5);
            console.log(`   … предел частоты, жду ${wait}с (${path})`);
            await sleep(wait * 1000);
            continue;
        }
        const text = await res.text();
        if (!res.ok) {
            throw new Error(`${method} ${path} -> ${res.status} ${text}`);
        }
        return text ? JSON.parse(text) : null;
    }
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
            // ⭐ Смена раздачи видна только по снимку: фиксируем перенос уровней.
            if (this.watchDeals && this.game && envelope.payload.dealNo > this.game.dealNo) {
                transitions.push({
                    deal: `${this.game.dealNo} -> ${envelope.payload.dealNo}`,
                    before: (this.game.players ?? []).map(flightOf),
                    after: (envelope.payload.players ?? []).map(flightOf),
                });
            }
            this.game = envelope.payload;
            this.actions = envelope.payload.availableActions ?? [];
            this.phase = envelope.payload.phase;
        } else if (envelope.type === 'MATCH_OVER') {
            this.result = envelope.payload;
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
bots[0].watchDeals = true;
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
        await report(true, moves);
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

async function report(ok, madeMoves) {
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
    let failed = !ok;
    failed = !(await checkDealFlow()) || failed;
    failed = !(await checkRating()) || failed;
    process.exit(failed ? 1 : 0);
}

/**
 * ⭐ Переходы между раздачами. Живая партия нашла пересдачу, стиравшую навесы
 * посреди матча, — теперь каждый дым проверяет перенос уровней явно:
 * никто не прыгает вверх больше чем на ступень (§0.1: +1 проигравшему раздачу),
 * и уровни не обнуляются всем столом разом (симптом той самой пересдачи).
 */
async function checkDealFlow() {
    const result = bots.find((bot) => bot.result)?.result;
    const expected = (result?.dealsPlayed ?? 1) - 1;
    console.log(`   раздач ${result?.dealsPlayed}, переходов увидено ${transitions.length}`);
    let ok = true;
    if (transitions.length !== expected) {
        console.log(`   ❌ переходов ${transitions.length}, ждали ${expected}`);
        ok = false;
    }
    for (const t of transitions) {
        console.log(`   раздача ${t.deal}: [${t.before.join(' ')}] -> [${t.after.join(' ')}]`);
        const someoneWasUp = t.before.some((f) => f !== '6');
        const allReset = t.after.every((f) => f === '6');
        if (someoneWasUp && allReset) {
            console.log('   ❌ уровни обнулились всем столом: раздача пересдана вместо сыгранной');
            ok = false;
        }
        for (let seat = 0; seat < t.before.length; seat++) {
            if (stepOf(t.after[seat]) - stepOf(t.before[seat]) > 1) {
                console.log(`   ❌ место ${seat}: скачок ${t.before[seat]} -> ${t.after[seat]} — больше ступени за раздачу`);
                ok = false;
            }
        }
    }
    return ok;
}

/**
 * ⭐ Рейтинг после матча: Elo — игра с нулевой суммой, MATCH_OVER обязан сходиться
 * с /rating/me каждого игрока, а матч — быть виден в истории завершённым.
 */
async function checkRating() {
    const result = bots.find((bot) => bot.result)?.result;
    if (!result?.players?.length) {
        console.log('   ❌ в MATCH_OVER нет игроков');
        return false;
    }
    let ok = true;
    const sum = result.players.reduce((total, p) => total + Number(p.ratingDelta), 0);
    if (Math.abs(sum) > 0.011) {
        console.log(`   ❌ сумма дельт рейтинга ${sum.toFixed(2)} — рейтинг создался из воздуха`);
        ok = false;
    }
    if (!result.players.some((p) => p.lossDegree)) {
        console.log('   ❌ у матча нет проигравшего со степенью');
        ok = false;
    }
    for (let index = 0; index < bots.length; index++) {
        const me = await api('/rating/me', {token: accounts[index].accessToken});
        const mine = result.players.find((p) => p.userId === me.userId)
            ?? result.players.find((p) => p.displayName === accounts[index].displayName);
        if (!mine || Number(me.rating) !== Number(mine.ratingAfter)) {
            console.log(`   ❌ рейтинг бота ${index}: /rating/me=${me.rating}, MATCH_OVER=${mine?.ratingAfter}`);
            ok = false;
        }
    }
    const matches = await api('/matches', {token: accounts[0].accessToken});
    const finished = matches.find((m) => m.status === 'FINISHED'
        && m.dealsPlayed === result.dealsPlayed);
    if (!finished) {
        console.log('   ❌ сыгранный матч не найден в истории завершённым');
        ok = false;
    }
    console.log(`   рейтинг: сумма дельт ${sum.toFixed(2)}, REST сходится, матч в истории — ${ok ? '✅' : 'см. выше'}`);
    return ok;
}
