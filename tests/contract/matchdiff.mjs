/**
 * Differential ручек, до которых доходит только СЫГРАННЫЙ матч.
 *
 * ⭐ Здесь сравниваются не формы, а САМИ ДАННЫЕ. Оба бэкенда смотрят в одну базу,
 * поэтому матч, сыгранный одним, второй обязан прочитать точно так же: та же история,
 * тот же разбор, тот же реплей, тот же рейтинг. Различие здесь — это не «поле переехало»,
 * а «игрок увидит другую партию».
 *
 * ⚠️ Матч играется ДВАЖДЫ — по одному на каждом бэкенде, — и каждый читается обоими.
 * Прямое направление (Go читает то, что записала Java) нужно для переключения, обратное
 * (Java читает то, что записал Go) — для отката. Проверять только одно значит закрыть
 * себе половину пути.
 *
 * Запуск:
 *   OLD_BACKEND_URL=http://localhost:8098 NEW_BACKEND_URL=http://localhost:8099 \
 *     node tests/contract/matchdiff.mjs
 */

import {diff, normalize} from './lib.mjs';

const OLD = process.env.OLD_BACKEND_URL ?? 'http://localhost:8088';
const NEW = process.env.NEW_BACKEND_URL ?? 'http://localhost:8099';
const INVITE = process.env.BARDAK_INVITE ?? 'bardak-2026';
const PASSWORD = 'very-secret-password';
const RUN = process.env.RUN_TAG ?? String(Date.now()).slice(-7);

/** Столько тиков без единого хода считаем зависанием: боты отвечают мгновенно. */
const IDLE_LIMIT = 100;
const TICK_MS = 20;

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

/**
 * Вход на КОНКРЕТНОМ бэкенде.
 *
 * ⚠️ Токен берётся у того, кого спрашиваем. Совместимость токенов доказана отдельно
 * (тот же секрет, тот же алгоритм), но опираться на неё здесь значило бы проверять
 * заодно и её — а тогда непонятно, что именно сломалось.
 */
async function login(base, username) {
    return api(base, '/auth/login', {method: 'POST', body: {username, password: PASSWORD}});
}

async function register(base, username) {
    return api(base, '/auth/register', {method: 'POST',
        body: {username, displayName: username, password: PASSWORD, inviteCode: INVITE}});
}

class Bot {
    constructor(base, who, tableId) {
        this.base = base;
        this.who = who;
        this.tableId = tableId;
        this.actions = [];
        this.over = false;
    }

    async connect() {
        const {ticket} = await api(this.base, '/auth/ws-ticket',
            {method: 'POST', body: {}, token: this.who.accessToken});
        this.ws = new WebSocket(this.base.replace(/^http/, 'ws')
            + `/ws?ticket=${encodeURIComponent(ticket)}`);
        await new Promise((ok, fail) => {
            this.ws.addEventListener('open', ok, {once: true});
            this.ws.addEventListener('error', fail, {once: true});
        });
        this.ws.addEventListener('message', (raw) => {
            const envelope = JSON.parse(raw.data);
            if (envelope.type === 'STATE_SYNC') {
                this.actions = envelope.payload.availableActions ?? [];
            } else if (envelope.type === 'MATCH_OVER') {
                this.over = true;
            }
        });
    }

    send(type, payload = {}) {
        this.ws.send(JSON.stringify({v: 1, id: `diff-${Math.random()}`, type,
            tableId: this.tableId, ts: Date.now(), payload}));
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

    close() {
        try {
            this.ws.close();
        } catch {
            // Уже закрыт — не повод падать в конце прогона.
        }
    }
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/** Сыграть матч целиком на одном бэкенде. Возвращает логины игроков и стол. */
async function playMatch(base, tag) {
    const names = [`md_${RUN}_${tag}_a`, `md_${RUN}_${tag}_b`];
    const accounts = [];
    for (const name of names) {
        accounts.push(await register(base, name).catch(() => login(base, name)));
    }

    const table = await api(base, '/tables', {method: 'POST', token: accounts[0].accessToken,
        body: {name: `differential ${tag}`, maxPlayers: 2, rulesConfig: {}, isPrivate: false}});

    const bots = accounts.map((who) => new Bot(base, who, table.id));
    for (const bot of bots) {
        await bot.connect();
        bot.send('TABLE_JOIN');
    }
    await sleep(400);
    for (const bot of bots) {
        bot.send('TABLE_READY', {ready: true});
    }
    await sleep(600);
    bots[0].send('MATCH_START');
    await sleep(700);

    let idle = 0;
    let moves = 0;
    for (let tick = 0; tick < 20000; tick++) {
        if (bots.some((bot) => bot.over)) {
            break;
        }
        if (bots.map((bot) => bot.step()).some(Boolean)) {
            moves++;
            idle = 0;
        } else if (++idle > IDLE_LIMIT) {
            bots.forEach((bot) => bot.close());
            throw new Error(`${tag}: матч встал после ${moves} ходов`);
        }
        await sleep(TICK_MS);
    }
    bots.forEach((bot) => bot.close());
    if (!bots.some((bot) => bot.over)) {
        throw new Error(`${tag}: матч не кончился за отведённые ходы`);
    }

    console.log(`  сыграно на ${base}: ходов ${moves}`);
    return {names, tableId: table.id};
}

/** Прочитать всё, что можно прочитать про матч, глазами одного игрока. */
async function readMatch(base, username) {
    const who = await login(base, username);
    const token = who.accessToken;

    const matches = await api(base, '/matches', {token});
    const finished = matches.find((match) => match.status === 'FINISHED');
    if (!finished) {
        throw new Error(`${base}: у ${username} нет ни одного законченного матча`);
    }
    return {
        matchId: finished.id,
        list: matches,
        details: await api(base, `/matches/${finished.id}`, {token}),
        replay: await api(base, `/matches/${finished.id}/replay`, {token}),
        rating: await api(base, '/rating/me', {token}),
        stats: await api(base, '/stats/me', {token}),
    };
}

/**
 * Сравнить чтение одного и того же матча двумя бэкендами.
 *
 * ⚠️ Реплей сравнивается ПО СОБЫТИЯМ, а не целиком: тела событий содержат карты, а карты
 * в матче, сыгранном другим бэкендом, свои (MD-005 — колода из seed не воспроизводится
 * побитово). Совпадать обязаны порядок, количество и ИМЕНА событий: разойдись они —
 * реплей у игрока пойдёт не так.
 */
function compareReads(left, right, label) {
    const problems = [];

    for (const [name, getter] of [
        ['история', (read) => read.list],
        ['разбор матча', (read) => read.details],
        ['рейтинг', (read) => read.rating],
        ['статистика', (read) => read.stats],
    ]) {
        problems.push(...diff(normalize(getter(left)), normalize(getter(right)))
            .map((line) => `${label} · ${name}: ${line}`));
    }

    if (left.matchId !== right.matchId) {
        problems.push(`${label} · история: бэкенды считают последним РАЗНЫЕ матчи`);
    }

    const leftTypes = left.replay.events.map((event) => event.type);
    const rightTypes = right.replay.events.map((event) => event.type);
    if (leftTypes.length !== rightTypes.length) {
        problems.push(`${label} · реплей: событий ${leftTypes.length} против ${rightTypes.length}`);
    } else {
        for (let index = 0; index < leftTypes.length; index++) {
            if (leftTypes[index] !== rightTypes[index]) {
                problems.push(`${label} · реплей: событие ${index} — `
                    + `${leftTypes[index]} против ${rightTypes[index]}`);
                break;
            }
        }
    }
    if (left.replay.mySeat !== right.replay.mySeat) {
        problems.push(`${label} · реплей: место смотрящего ${left.replay.mySeat} `
            + `против ${right.replay.mySeat}`);
    }
    if (left.replay.status !== right.replay.status) {
        problems.push(`${label} · реплей: статус ${left.replay.status} против ${right.replay.status}`);
    }

    return problems;
}

async function reachable(base) {
    try {
        const health = await api(base, '/health');
        return health?.status === 'UP';
    } catch {
        return false;
    }
}

// ── прогон ──────────────────────────────────────────────────────────────────

for (const [label, base] of [['старый', OLD], ['новый', NEW]]) {
    if (!await reachable(base)) {
        console.error(`❌ ${label} бэкенд (${base}) не отвечает`);
        process.exit(2);
    }
}

console.log(`старый: ${OLD}`);
console.log(`новый:  ${NEW}`);
console.log('⚠️ Оба бэкенда обязаны смотреть в ОДНУ базу — иначе сравнивать нечего.\n');

let failed = 0;

console.log('▸ матч, сыгранный СТАРЫМ бэкендом');
const onOld = await playMatch(OLD, 'old');
const oldByOld = await readMatch(OLD, onOld.names[0]);
const oldByNew = await readMatch(NEW, onOld.names[0]);
const forward = compareReads(oldByOld, oldByNew, 'матч старого');
if (forward.length) {
    failed++;
    console.log('❌ новый бэкенд читает чужой матч иначе:');
    forward.slice(0, 12).forEach((line) => console.log('   ' + line));
} else {
    console.log('✅ новый бэкенд читает матч старого точно так же');
}

console.log('\n▸ матч, сыгранный НОВЫМ бэкендом');
const onNew = await playMatch(NEW, 'new');
const newByNew = await readMatch(NEW, onNew.names[0]);
const newByOld = await readMatch(OLD, onNew.names[0]);
const backward = compareReads(newByNew, newByOld, 'матч нового');
if (backward.length) {
    failed++;
    console.log('❌ старый бэкенд читает матч нового иначе — откат будет неполным:');
    backward.slice(0, 12).forEach((line) => console.log('   ' + line));
} else {
    console.log('✅ старый бэкенд читает матч нового точно так же — откат безопасен');
}

console.log(failed ? `\n❌ направлений с различиями: ${failed} из 2`
    : '\n✅ различий нет: сыгранный матч читается обоими одинаково в обе стороны');
process.exit(failed ? 1 : 0);
