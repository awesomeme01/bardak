/**
 * Общие части сравнения двух бэкендов: нормализация, различия, самопроверка.
 *
 * ⭐ Вынесено из `compare.mjs`, чтобы этим же пользовался `matchdiff.mjs`: правила
 * «что считать переменным» обязаны быть ОДНИ. Разъехавшись, они дали бы двум прогонам
 * разное представление об одном и том же ответе — и различие нашлось бы только в одном.
 */

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


export {normalize, diff, describe, isErrorBody, VOLATILE, call};
