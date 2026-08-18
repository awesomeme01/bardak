/**
 * Нагрузочная проверка одного узла: сколько столов он держит (M9).
 *
 * ⭐ Мерим не «сколько ходов в секунду выжмет машина», а <b>задержку хода</b>: сколько
 * проходит от нажатия до момента, когда игрок видит результат. Пропускная способность
 * игроку не видна, а задержка видна каждым ходом. Предел узла — это тот момент, когда
 * задержка перестаёт быть незаметной, а не когда сервер падает.
 *
 * ⚠️ Боты думают перед ходом (`THINK_MS`, по умолчанию ~1.2 с с разбросом). Без паузы
 * стол генерирует ходы в сотни раз чаще живого, и «100 столов» такого прогона не имеют
 * ничего общего со 100 живыми столами. Пауза — не замедление ради приличия, а условие
 * того, чтобы число столов вообще что-то значило.
 *
 * Задержка считается как «команда → следующий снимок у этого же бота». Точной привязки
 * ответа к команде в протоколе нет (STATE_SYNC уходит всем сразу), но именно эта величина
 * и есть ожидание игрока.
 *
 *   node tools/smoke/loadtest.mjs 10           # 10 столов по 2 игрока
 *   node tools/smoke/loadtest.mjs 10 4         # 10 столов по 4 игрока
 *   node tools/smoke/loadtest.mjs ramp         # 2 → 5 → 10 → 20 → 40, до срыва
 *   THINK_MS=0 node tools/smoke/loadtest.mjs 5 # без пауз: потолок железа, не ёмкость
 */
const BASE = process.env.BARDAK_URL ?? 'http://localhost:8088';
const INVITE = process.env.BARDAK_INVITE ?? 'bardak-2026';
const PASSWORD = 'very-secret-password';
const THINK_MS = Number(process.env.THINK_MS ?? 1200);

/** Порог срыва: выше этого ожидание хода перестаёт читаться как «мгновенно». */
const P95_BUDGET_MS = 400;
/** Стол молчит дольше — считаем зависшим: боты отвечают сразу, ждать нечего. */
const STALL_MS = 25000;
/**
 * ⭐ Окно замера. Конца матча НЕ ждём: партия в дурака с человеческой паузой идёт
 * много минут и сотни ходов, а нас интересует установившийся режим, а не финал.
 * Замеряем минуту нагрузки — этого хватает, чтобы увидеть и задержку, и её хвост.
 */
const WINDOW_MS = Number(process.env.WINDOW_MS ?? 60000);

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const jitter = (ms) => Math.round(ms * (0.6 + Math.random() * 0.8));

async function api(path, {method = 'GET', body, token} = {}) {
    const res = await fetch(BASE + '/api' + path, {
        method,
        headers: {'Content-Type': 'application/json', ...(token ? {Authorization: 'Bearer ' + token} : {})},
        body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    if (!res.ok) {
        throw new Error(`${method} ${path} -> ${res.status} ${text.slice(0, 200)}`);
    }
    return text ? JSON.parse(text) : null;
}

/** Аккаунт бота: заводим, а если остался от прошлого прогона — просто входим. */
async function account(username) {
    try {
        return await api('/auth/register', {method: 'POST',
            body: {username, displayName: username, password: PASSWORD, inviteCode: INVITE}});
    } catch {
        return await api('/auth/login', {method: 'POST', body: {username, password: PASSWORD}});
    }
}

/**
 * ⚠️ Регистрация упирается в bcrypt и в пул на 10 соединений. Пускать её всей толпой
 * бессмысленно — очередь всё равно выстроится, зато прогон начнётся с искусственного
 * затора. Подготовка идёт до замера и в замер не входит.
 */
async function inBatches(items, size, worker) {
    const done = [];
    for (let start = 0; start < items.length; start += size) {
        done.push(...await Promise.all(items.slice(start, start + size).map(worker)));
    }
    return done;
}

/** Метрика актуатора: сервер видит себя изнутри лучше, чем секундомер снаружи. */
async function metric(name, tag) {
    try {
        const query = tag ? `?tag=${encodeURIComponent(tag)}` : '';
        const res = await fetch(`${BASE}/actuator/metrics/${name}${query}`);
        if (!res.ok) {
            return null;
        }
        const body = await res.json();
        return body.measurements?.[0]?.value ?? null;
    } catch {
        return null;
    }
}

class LoadBot {
    constructor(who, tableId, stats) {
        this.who = who;
        this.tableId = tableId;
        this.stats = stats;
        this.over = false;
        this.sentAt = null;
        this.actions = [];
    }

    async connect() {
        const {ticket} = await api('/auth/ws-ticket',
            {method: 'POST', body: {}, token: this.who.accessToken});
        this.ws = new WebSocket(BASE.replace(/^http/, 'ws') + `/ws?ticket=${encodeURIComponent(ticket)}`);
        await new Promise((ok, fail) => {
            this.ws.addEventListener('open', ok, {once: true});
            this.ws.addEventListener('error', fail, {once: true});
        });
        this.ws.addEventListener('message', (raw) => this.#onMessage(JSON.parse(raw.data)));
        this.ws.addEventListener('close', () => {
            if (!this.over) {
                this.stats.socketsDropped++;
            }
        });
    }

    #onMessage(envelope) {
        if (envelope.type === 'STATE_SYNC') {
            // ⭐ Снимок закрывает ожидание: игрок увидел результат своего хода.
            if (this.sentAt !== null) {
                this.stats.latencies.push(Date.now() - this.sentAt);
                this.sentAt = null;
            }
            this.stats.lastEventAt = Date.now();
            this.actions = envelope.payload?.availableActions ?? [];
        } else if (envelope.type === 'MATCH_OVER') {
            this.over = true;
            this.stats.lastEventAt = Date.now();
        } else if (envelope.type === 'ERROR') {
            const code = envelope.payload?.code;
            // ⚠️ После `over` отказы не считаем: на выходе их гарантированно насыплет —
            // партию отменяет первый ушедший, остальным прилетает NO_MATCH. Это шум
            // сворачивания, а не находка прогона.
            if (code && code !== 'NO_MATCH' && !this.over) {
                this.stats.rejections[code] = (this.stats.rejections[code] ?? 0) + 1;
            }
            this.sentAt = null;
        }
    }

    /**
     * ⚠️ Бот живёт своим циклом, а не только реакцией на событие. Чисто событийная схема
     * встаёт намертво на первом же отказе: сервер отклонил ход, нового снимка не будет,
     * и будить бота нечем. Список действий берётся АКТУАЛЬНЫЙ в момент хода — тот, что
     * выбран заранее, за время паузы успевает устареть.
     */
    async play() {
        while (!this.over && this.ws.readyState === WebSocket.OPEN) {
            if (!this.actions.length) {
                await sleep(40);
                continue;
            }
            await sleep(THINK_MS ? jitter(THINK_MS) : 0);
            if (this.over || this.ws.readyState !== WebSocket.OPEN || !this.actions.length) {
                continue;
            }
            const actions = this.actions;
            this.actions = [];
            const rank = (a) => (a.type === 'TAKE' ? 3 : a.type === 'PASS' ? 2 : 1);
            const best = Math.min(...actions.map(rank));
            const pool = actions.filter((a) => rank(a) === best);
            const action = pool[Math.floor(Math.random() * pool.length)];

            this.sentAt = Date.now();
            this.stats.moves++;
            this.ws.send(JSON.stringify({v: 1, id: `load-${this.stats.moves}-${Math.random()}`,
                type: action.type, tableId: this.tableId, ts: Date.now(), payload: action.payload ?? {}}));
        }
    }

    /**
     * ⚠️ Выйти из-за стола обязательно, а не «для чистоты». База держит инвариант
     * «один игрок — один стол» (V9), и брошенный стол навсегда занимает аккаунт:
     * следующая ступень лестницы упирается в честный 409 MATCH_IN_PROGRESS.
     *
     * ⚠️ Двумя шагами и именно в таком порядке. Пока стол `IN_MATCH`, `TABLE_LEAVE`
     * отказывает тем же 409 — сначала `MATCH_LEAVE` отменяет партию, и только потом
     * место можно освободить.
     */
    leave(type) {
        this.over = true;
        if (this.ws?.readyState !== WebSocket.OPEN) {
            return;
        }
        try {
            this.ws.send(JSON.stringify({v: 1, id: `leave-${Math.random()}`, type,
                tableId: this.tableId, ts: Date.now(), payload: {}}));
        } catch {
            // Сокет мог умереть сам — на итог прогона это уже не влияет.
        }
    }

    close() {
        this.over = true;
        try {
            this.ws?.close();
        } catch {
            // Сокет мог умереть сам — на итог прогона это уже не влияет.
        }
    }
}

function percentile(sorted, share) {
    if (!sorted.length) {
        return 0;
    }
    return sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * share))];
}

/** Один замер: N столов по P игроков играют одновременно до конца. */
async function runRound(tables, players, stamp) {
    const stats = {latencies: [], rejections: {}, moves: 0, socketsDropped: 0, lastEventAt: Date.now()};

    const names = [];
    for (let table = 0; table < tables; table++) {
        for (let seat = 0; seat < players; seat++) {
            // Ступень входит в имя: если выход из-за стола почему-то не прошёл, упадёт
            // одна ступень, а не вся лестница на чужих занятых аккаунтах.
            names.push(`load${players}_${stamp}_${tables}t${table}_${seat}`);
        }
    }
    process.stdout.write(`   готовлю ${names.length} аккаунтов… `);
    const accounts = await inBatches(names, 8, account);
    process.stdout.write('готово\n');

    const rooms = await inBatches(
        Array.from({length: tables}, (_, index) => index), 8,
        (index) => api('/tables', {method: 'POST', token: accounts[index * players].accessToken,
            body: {name: `нагрузка ${index}`, maxPlayers: players, rulesConfig: {}, isPrivate: false}}));

    const bots = [];
    for (let table = 0; table < tables; table++) {
        for (let seat = 0; seat < players; seat++) {
            bots.push(new LoadBot(accounts[table * players + seat], rooms[table].id, stats));
        }
    }

    // Подключаемся партиями: тысяча одновременных рукопожатий меряет не игру, а рукопожатия.
    await inBatches(bots, 16, async (bot) => {
        await bot.connect();
        bot.ws.send(JSON.stringify({v: 1, id: `join-${Math.random()}`, type: 'TABLE_JOIN',
            tableId: bot.tableId, ts: Date.now(), payload: {}}));
        bot.ws.send(JSON.stringify({v: 1, id: `ready-${Math.random()}`, type: 'TABLE_READY',
            tableId: bot.tableId, ts: Date.now(), payload: {ready: true}}));
    });
    await sleep(1200);

    // ⭐ Замер начинается здесь: подготовка позади, дальше только игра.
    const startedAt = Date.now();
    stats.lastEventAt = startedAt;
    const peak = {cpu: 0, pending: 0, active: 0, heap: 0};

    for (let table = 0; table < tables; table++) {
        const host = bots[table * players];
        host.ws.send(JSON.stringify({v: 1, id: `start-${Math.random()}`, type: 'MATCH_START',
            tableId: host.tableId, ts: Date.now(), payload: {}}));
    }
    // Циклы ботов крутятся сами; сторожевой цикл ниже только смотрит, когда всё кончится.
    bots.forEach((bot) => bot.play());

    const watcher = setInterval(async () => {
        const [cpu, pending, active, heap] = await Promise.all([
            metric('process.cpu.usage'),
            metric('hikaricp.connections.pending'),
            metric('hikaricp.connections.active'),
            metric('jvm.memory.used', 'area:heap'),
        ]);
        peak.cpu = Math.max(peak.cpu, cpu ?? 0);
        peak.pending = Math.max(peak.pending, pending ?? 0);
        peak.active = Math.max(peak.active, active ?? 0);
        peak.heap = Math.max(peak.heap, heap ?? 0);
    }, 1000);

    let verdict = 'ok';
    while (Date.now() - startedAt < WINDOW_MS) {
        // Все матчи кончились раньше окна — тоже законный конец замера.
        if (bots.every((bot) => bot.over)) {
            break;
        }
        // ⚠️ Тишина на всех столах разом — это не «медленно», а «встало».
        if (Date.now() - stats.lastEventAt > STALL_MS) {
            verdict = 'stall';
            break;
        }
        await sleep(200);
    }
    clearInterval(watcher);
    const elapsed = (Date.now() - startedAt) / 1000;
    // Освобождаем места до того, как рвать сокеты: закрытый сокет уже ничего не отправит,
    // а место за столом останется занятым до конца времён.
    bots.forEach((bot) => bot.leave('MATCH_LEAVE'));
    await sleep(1500);
    bots.forEach((bot) => bot.leave('TABLE_LEAVE'));
    await sleep(1500);
    bots.forEach((bot) => bot.close());
    await sleep(300);

    const sorted = [...stats.latencies].sort((a, b) => a - b);
    return {
        tables, players, verdict, elapsed,
        moves: stats.moves,
        measured: sorted.length,
        p50: percentile(sorted, 0.5),
        p95: percentile(sorted, 0.95),
        p99: percentile(sorted, 0.99),
        max: sorted[sorted.length - 1] ?? 0,
        rejections: stats.rejections,
        socketsDropped: stats.socketsDropped,
        peak,
    };
}

function printRound(round) {
    const mark = round.verdict === 'ok' ? (round.p95 <= P95_BUDGET_MS ? '✅' : '⚠️ ') : '❌';
    const note = round.verdict === 'stall' ? ' СТОЛЫ ВСТАЛИ' : '';
    console.log(`${mark} ${round.tables} стол(ов) × ${round.players}${note}`);
    console.log(`   задержка хода: p50 ${round.p50} мс · p95 ${round.p95} мс · p99 ${round.p99} мс · макс ${round.max} мс`
        + `  (замеров ${round.measured})`);
    console.log(`   ходов ${round.moves} за ${round.elapsed.toFixed(1)} с — ${(round.moves / round.elapsed).toFixed(1)}/с`);
    const {cpu, pending, active, heap} = round.peak;
    console.log(`   пик: CPU ${(cpu * 100).toFixed(0)}% · пул занято ${active}, в очереди ${pending}`
        + ` · heap ${(heap / 1048576).toFixed(0)} МБ`);
    if (round.socketsDropped) {
        console.log(`   ⚠️  оборвано сокетов: ${round.socketsDropped}`);
    }
    if (Object.keys(round.rejections).length) {
        console.log(`   отказы: ${Object.entries(round.rejections)
            .map(([code, count]) => `${code}:${count}`).join(' ')}`);
    }
}

const arg = process.argv[2] ?? '5';
const players = Number(process.argv[3] ?? 2);
const stamp = process.env.STAMP ?? String(Date.now()).slice(-6);

if (!await fetch(BASE + '/api/health').then((r) => r.ok).catch(() => false)) {
    console.error(`❌ Сервер не отвечает: ${BASE}`);
    process.exit(2);
}

console.log(`нагрузка на ${BASE} · пауза на ход ~${THINK_MS} мс · бюджет p95 ${P95_BUDGET_MS} мс\n`);

if (arg === 'ramp') {
    // ⭐ Лестницей, а не одним большим числом: предел — это место, где кривая ломается,
    // и увидеть его можно только сравнив ступени между собой.
    const results = [];
    // ⚠️ Верхняя ступень — не предел узла, а предел лестницы. Если последняя ступень
    // прошла с запасом, честный вывод — «держит не меньше», а не «предел найден»:
    // поднимать `STEPS`, пока запас не кончится.
    const steps = (process.env.STEPS ?? '2,5,10,20,40').split(',').map(Number);
    for (const tables of steps) {
        let round;
        try {
            round = await runRound(tables, players, `${stamp}r`);
        } catch (error) {
            // Ступень не собралась — это тоже результат, но не повод терять уже измеренное.
            console.log(`❌ ${tables} стол(ов) × ${players}: не удалось подготовить — ${error.message}\n`);
            break;
        }
        printRound(round);
        console.log();
        results.push(round);
        if (round.verdict !== 'ok' || round.p95 > P95_BUDGET_MS * 3) {
            console.log('дальше не поднимаю: узел уже за пределом\n');
            break;
        }
        await sleep(2000);
    }
    const last = [...results].reverse().find((r) => r.verdict === 'ok' && r.p95 <= P95_BUDGET_MS);
    console.log('─'.repeat(56));
    if (!last) {
        console.log('итог: ни одна ступень не уложилась в бюджет — разбираться с первой');
    } else if (last.tables === steps[steps.length - 1]) {
        // ⭐ Лестница кончилась раньше узла: это нижняя граница, а не предел.
        console.log(`итог: держит НЕ МЕНЬШЕ ${last.tables} столов по ${players} — p95 ${last.p95} мс,`
            + ` запас есть (CPU ${(last.peak.cpu * 100).toFixed(0)}%).`);
        console.log(`      предел не найден: поднимай ступени — STEPS=80,160 tools/smoke/loadtest.mjs ramp`);
    } else {
        console.log(`итог: предел найден — уверенно держит ${last.tables} стол(ов) по ${players},`
            + ` p95 ${last.p95} мс; выше бюджет уже не выдержан`);
    }
    process.exit(results.some((r) => r.verdict !== 'ok') ? 1 : 0);
}

const round = await runRound(Number(arg), players, stamp);
printRound(round);
process.exit(round.verdict === 'ok' ? 0 : 1);
