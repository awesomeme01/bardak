/**
 * Differential-проверка: одни и те же сценарии против двух бэкендов, сравнение ответов.
 *
 *   OLD_BACKEND_URL=http://localhost:8088 NEW_BACKEND_URL=http://localhost:8099 \
 *     node tests/contract/compare.mjs
 *
 * ⭐ Сравнивается ФОРМА ответа, а не значения, которые заведомо различны. Нормализуются
 * только реально динамические вещи: идентификаторы, время, токены, коды столов, колода.
 * Всё остальное — коды состояния, имена полей, их НАЛИЧИЕ и ОТСУТСТВИЕ, типы — обязано
 * совпасть.
 *
 * ⚠️ Отсутствие поля — это тоже контракт. Java вырезает null-поля глобально, поэтому
 * «поле есть со значением null» и «поля нет» — разные ответы, и путать их нельзя
 * (см. docs/migration-decisions.md, MD-003).
 */
import {scenarios} from './scenarios.mjs';

const OLD = process.env.OLD_BACKEND_URL ?? 'http://localhost:8088';
const NEW = process.env.NEW_BACKEND_URL ?? 'http://localhost:8099';

/** Метка прогона: логины должны быть свежими, иначе упрёмся в занятые. */
const RUN = process.env.RUN_TAG ?? String(Date.now()).slice(-7);

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const ISO_RE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/;

/** Поля, значения которых сравнивать бессмысленно: они динамические по своей природе. */
const VOLATILE = new Set([
    'id', 'userId', 'matchId', 'tableId', 'hostUserId', 'cardSetId', 'themeId', 'friendId',
    'accessToken', 'refreshToken', 'ticket', 'traceId', 'code', 'ts', 'seq',
    'createdAt', 'startedAt', 'closedAt', 'finishedAt', 'playedAt', 'expiresIn',
    'previewUrl', 'url', 'avatarUrl', 'username', 'displayName', 'name',
]);

/**
 * ⚠️ `code` в VOLATILE — это КОД СТОЛА (шесть символов), он случайный. Но `code` в теле
 * ошибки — машиночитаемая причина, и она обязана совпадать. Различаем по соседям.
 */
function isErrorBody(value) {
    return value && typeof value === 'object' && 'code' in value && 'message' in value
        && 'traceId' in value;
}

function normalize(value, key) {
    if (Array.isArray(value)) {
        return value.map((item) => normalize(item, key));
    }
    if (value && typeof value === 'object') {
        const errorBody = isErrorBody(value);
        const out = {};
        for (const name of Object.keys(value).sort()) {
            // Код ошибки сохраняем как есть: это контракт, а не случайная строка.
            if (errorBody && name === 'code') {
                out[name] = value[name];
                continue;
            }
            out[name] = normalize(value[name], name);
        }
        return out;
    }
    if (typeof value === 'string') {
        if (VOLATILE.has(key)) return '<переменное>';
        if (UUID_RE.test(value)) return '<uuid>';
        if (ISO_RE.test(value)) return '<время>';
        return value;
    }
    if (typeof value === 'number' && VOLATILE.has(key)) return '<число>';
    return value;
}

async function call(base, spec) {
    const headers = {};
    if (spec.body || spec.raw) headers['Content-Type'] = 'application/json';
    if (spec.token) headers.Authorization = `Bearer ${spec.token}`;

    let response;
    try {
        response = await fetch(base + spec.path, {
            method: spec.method,
            headers,
            body: spec.raw ?? (spec.body === undefined ? undefined : JSON.stringify(spec.body)),
        });
    } catch (e) {
        return {status: 0, body: {'сеть': e.message}};
    }
    const text = await response.text();
    let body = null;
    if (text) {
        try {
            body = JSON.parse(text);
        } catch {
            body = {'не-json': text.slice(0, 200)};
        }
    }
    return {status: response.status, body};
}

/** Читаемое различие: путь до поля, а не «объекты не равны». */
function diff(left, right, path = '') {
    const out = [];
    if (typeof left !== typeof right || Array.isArray(left) !== Array.isArray(right)) {
        return [`${path || '<корень>'}: типы разошлись — ${describe(left)} против ${describe(right)}`];
    }
    if (left && typeof left === 'object') {
        const keys = new Set([...Object.keys(left), ...Object.keys(right)]);
        for (const key of [...keys].sort()) {
            const here = path ? `${path}.${key}` : key;
            if (!(key in left)) {
                out.push(`${here}: поля НЕТ у старого, но есть у нового`);
                continue;
            }
            if (!(key in right)) {
                out.push(`${here}: поле есть у старого, но у нового ОТСУТСТВУЕТ`);
                continue;
            }
            out.push(...diff(left[key], right[key], here));
        }
        return out;
    }
    if (left !== right) {
        out.push(`${path}: ${JSON.stringify(left)} против ${JSON.stringify(right)}`);
    }
    return out;
}

function describe(value) {
    if (value === null) return 'null';
    if (Array.isArray(value)) return 'массив';
    return typeof value;
}

async function runScenario(scenario) {
    // ⚠️ Состояние копится ОТДЕЛЬНО для каждого бэкенда: токен, выданный одним,
    // на другом не действует, а идентификаторы у них свои.
    let oldState = {run: RUN + 'o'};
    let newState = {run: RUN + 'n'};
    const problems = [];

    for (const step of scenario.steps) {
        const oldResponse = await call(OLD, step.request(oldState));
        const newResponse = await call(NEW, step.request(newState));

        if (oldResponse.status !== newResponse.status) {
            problems.push(`${step.what}: статус ${oldResponse.status} против ${newResponse.status}`);
        }
        const differences = diff(normalize(oldResponse.body), normalize(newResponse.body));
        for (const line of differences) {
            problems.push(`${step.what}: ${line}`);
        }

        if (step.keep) {
            if (oldResponse.body) oldState = step.keep(oldState, oldResponse.body);
            if (newResponse.body) newState = step.keep(newState, newResponse.body);
        }
    }
    return problems;
}

/**
 * Самопроверка инструмента.
 *
 * ⚠️ «Различий нет» ничего не стоит, пока не показано, что инструмент их ВИДИТ.
 * Нормализатор легко переусердствует и начнёт скрывать настоящие расхождения —
 * тогда parity-отчёт будет врать в самую опасную сторону.
 */
function selfTest() {
    const cases = [
        {
            name: 'пропавшее поле замечено',
            left: {matches: 0, wins: 0},
            right: {matches: 0},
            expect: (d) => d.some((line) => line.includes('ОТСУТСТВУЕТ')),
        },
        {
            name: 'лишнее поле замечено',
            left: {matches: 0},
            right: {matches: 0, wins: 0},
            expect: (d) => d.some((line) => line.includes('НЕТ у старого')),
        },
        {
            name: 'null против отсутствия замечены',
            left: {avgPlace: null},
            right: {},
            expect: (d) => d.length > 0,
        },
        {
            name: 'код ошибки не нормализуется',
            left: {code: 'TABLE_FULL', message: 'a', traceId: 'x'},
            right: {code: 'NOT_FRIENDS', message: 'a', traceId: 'y'},
            expect: (d) => d.some((line) => line.includes('TABLE_FULL')),
        },
        {
            name: 'смена типа замечена',
            left: {rating: 1000},
            right: {rating: '1000'},
            expect: (d) => d.length > 0,
        },
        {
            name: 'разные идентификаторы различием НЕ считаются',
            left: {id: 'a1b2', tableId: 'x'},
            right: {id: 'zzzz', tableId: 'y'},
            expect: (d) => d.length === 0,
        },
        {
            name: 'разное время различием НЕ считается',
            left: {startedAt: '2026-01-01T00:00:00Z'},
            right: {startedAt: '2026-08-20T10:00:00Z'},
            expect: (d) => d.length === 0,
        },
    ];

    let bad = 0;
    for (const test of cases) {
        const differences = diff(normalize(test.left), normalize(test.right));
        if (!test.expect(differences)) {
            bad++;
            console.log(`❌ самопроверка: ${test.name}`);
            console.log(`   получили: ${JSON.stringify(differences)}`);
        }
    }
    if (bad === 0) {
        console.log(`✅ самопроверка инструмента: ${cases.length} случаев\n`);
    }
    return bad === 0;
}

if (process.argv.includes('--selftest')) {
    process.exit(selfTest() ? 0 : 1);
}

if (!selfTest()) {
    console.error('❌ инструмент сравнения неисправен — результатам верить нельзя');
    process.exit(3);
}

const reachable = async (url) => fetch(url + '/api/health').then((r) => r.ok).catch(() => false);

if (!await reachable(OLD)) {
    console.error(`❌ Старый бэкенд не отвечает: ${OLD}`);
    process.exit(2);
}
if (!await reachable(NEW)) {
    console.error(`❌ Новый бэкенд не отвечает: ${NEW}`);
    console.error('   Подними Go-версию и повтори — сравнивать пока не с чем.');
    process.exit(2);
}

console.log(`старый: ${OLD}`);
console.log(`новый:  ${NEW}\n`);

let failed = 0;
for (const scenario of scenarios) {
    const problems = await runScenario(scenario);
    if (problems.length === 0) {
        console.log(`✅ ${scenario.name}`);
        continue;
    }
    failed++;
    console.log(`❌ ${scenario.name}`);
    for (const problem of problems) {
        console.log(`   ${problem}`);
    }
}

console.log();
console.log(failed === 0
    ? '✅ различий нет'
    : `❌ сценариев с различиями: ${failed} из ${scenarios.length}`);
process.exit(failed === 0 ? 0 : 1);
