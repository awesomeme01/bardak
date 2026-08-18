/**
 * Дымовая проверка друзей: заявка, согласие, присутствие и приглашение за стол.
 *
 * ⭐ Проверяется то, что не проверить юнит-тестом: онлайн считается по живому сокету,
 * а приглашение доходит именно по нему. Здесь поднимаются настоящие WebSocket-соединения.
 *
 * Запуск: node tools/smoke/friends.mjs [метка]
 */
const BASE = process.env.BARDAK_URL ?? 'http://localhost:8088';
const INVITE = process.env.BARDAK_INVITE ?? 'bardak-2026';
const PASSWORD = 'very-secret-password';

const stamp = process.argv[2] ?? String(Date.now()).slice(-5);

let failures = 0;

function check(label, actual, expected) {
    const ok = JSON.stringify(actual) === JSON.stringify(expected);
    console.log(`${ok ? '✅' : '❌'} ${label}${ok ? '' : ` — ждали ${JSON.stringify(expected)}, получили ${JSON.stringify(actual)}`}`);
    if (!ok) {
        failures++;
    }
}

async function call(path, {method = 'GET', body, token} = {}) {
    const res = await fetch(BASE + '/api' + path, {
        method,
        headers: {'Content-Type': 'application/json', ...(token ? {Authorization: 'Bearer ' + token} : {})},
        body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    return {status: res.status, body: text ? JSON.parse(text) : null};
}

async function account(username) {
    let res = await call('/auth/register', {method: 'POST',
        body: {username, displayName: username, password: PASSWORD, inviteCode: INVITE}});
    if (res.status !== 200) {
        res = await call('/auth/login', {method: 'POST', body: {username, password: PASSWORD}});
    }
    return res.body;
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const a = await account(`fr${stamp}a`);
const b = await account(`fr${stamp}b`);
const stranger = await account(`fr${stamp}c`);

// ⚠️ Логин намеренно в другом регистре: телефонная клавиатура делает так сама.
const asked = await call('/friends/requests', {method: 'POST', token: a.accessToken,
    body: {username: `FR${stamp.toUpperCase()}B`}});
check('заявка по логину в другом регистре', asked.status, 200);

const incoming = (await call('/friends', {token: b.accessToken})).body;
check('заявка видна получателю', incoming.incoming.length, 1);
check('до согласия друзей нет', incoming.friends.length, 0);

check('согласие принято',
    (await call(`/friends/${a.user.id}/accept`, {method: 'POST', token: b.accessToken})).status, 200);
check('друг появился у обоих', [
    (await call('/friends', {token: a.accessToken})).body.friends.length,
    (await call('/friends', {token: b.accessToken})).body.friends.length,
], [1, 1]);

check('пока сокета нет — не в сети',
    (await call('/friends', {token: a.accessToken})).body.friends[0].online, false);

const {ticket} = (await call('/auth/ws-ticket', {method: 'POST', token: b.accessToken, body: {}})).body;
const ws = new WebSocket(BASE.replace(/^http/, 'ws') + `/ws?ticket=${encodeURIComponent(ticket)}`);
await new Promise((ok) => ws.addEventListener('open', ok, {once: true}));
await sleep(300);
check('сокет открыт — в сети',
    (await call('/friends', {token: a.accessToken})).body.friends[0].online, true);

const table = (await call('/tables', {method: 'POST', token: a.accessToken,
    body: {name: 'дым друзей', maxPlayers: 2, isPrivate: false}})).body;

const invited = new Promise((resolve) => ws.addEventListener('message', (event) => {
    const envelope = JSON.parse(event.data);
    if (envelope.type === 'TABLE_INVITE') {
        resolve(envelope.payload);
    }
}));
const sent = await call(`/friends/${b.user.id}/invite`,
    {method: 'POST', token: a.accessToken, body: {tableId: table.id}});
check('приглашение доставлено сразу', sent.body?.delivered, true);
const payload = await Promise.race([invited, sleep(3000).then(() => null)]);
check('приглашение пришло по сокету с кодом стола', payload?.tableCode, table.code);

check('незнакомец звать не может',
    (await call(`/friends/${b.user.id}/invite`,
        {method: 'POST', token: stranger.accessToken, body: {tableId: table.id}})).status, 403);

ws.close();
await sleep(400);
check('сокет закрыт — снова не в сети',
    (await call('/friends', {token: a.accessToken})).body.friends[0].online, false);

console.log(failures ? `\n❌ провалов: ${failures}` : '\n✅ друзья: все проверки прошли');
process.exit(failures ? 1 : 0);
