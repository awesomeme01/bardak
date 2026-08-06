/**
 * M1: страница-проба. Показывает, что связка браузер → Spring → Postgres живая,
 * и что конверт WS-протокола ходит в обе стороны.
 */

import {apiGet, ApiError} from './net/rest-client.js';
import {WsClient} from './net/ws-client.js';

const el = (id) => document.getElementById(id);

// --- REST ---------------------------------------------------------------

async function checkHealth() {
    const badge = el('health-badge');
    badge.className = 'badge badge-wait';
    badge.textContent = 'проверяю…';

    try {
        const health = await apiGet('/health');
        const dbUp = health.db?.status === 'UP';
        badge.className = `badge ${dbUp ? 'badge-ok' : 'badge-warn'}`;
        badge.textContent = dbUp ? 'приложение и БД живы' : 'приложение живо, БД недоступна';
        el('health-output').textContent = JSON.stringify(health, null, 2);
    } catch (e) {
        badge.className = 'badge badge-fail';
        badge.textContent = 'бэкенд недоступен';
        el('health-output').textContent = e instanceof ApiError
            ? `${e.code}: ${e.message}`
            : String(e);
    }
}

// --- WebSocket ----------------------------------------------------------

const wsLog = el('ws-log');
const MAX_LOG_LINES = 40;

function log(direction, text) {
    const time = new Date().toLocaleTimeString('ru-RU');
    const line = document.createElement('div');
    line.className = `log-line log-${direction}`;
    line.textContent = `${time}  ${direction === 'in' ? '←' : direction === 'out' ? '→' : '·'}  ${text}`;
    wsLog.append(line);

    while (wsLog.childElementCount > MAX_LOG_LINES) wsLog.firstElementChild.remove();
    wsLog.scrollTop = wsLog.scrollHeight;
}

const wsUrl = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`;

const ws = new WsClient({
    url: wsUrl,
    onStatus: (status, delayMs) => {
        const badge = el('ws-badge');
        const view = {
            connecting: ['badge-wait', 'подключаюсь…'],
            open: ['badge-ok', 'соединение открыто'],
            closed: ['badge-warn', 'соединение закрыто'],
            error: ['badge-fail', 'ошибка соединения'],
            reconnecting: ['badge-wait', `переподключение через ${Math.round((delayMs ?? 0) / 100) / 10} с`],
        }[status] ?? ['badge-wait', status];

        badge.className = `badge ${view[0]}`;
        badge.textContent = view[1];
        log('sys', `статус: ${view[1]}`);
    },
    onEvent: (envelope) => {
        // PONG приходит каждые 20 секунд — не засоряем им лог.
        if (envelope.type === 'PONG') return;
        log('in', `${envelope.type}  ${envelope.payload ? JSON.stringify(envelope.payload) : ''}`);
    },
});

el('ws-send').addEventListener('click', () => {
    const text = el('ws-input').value;
    const sent = ws.send('TEST_MESSAGE', {text});
    log('out', `${sent.type}  ${JSON.stringify(sent.payload)}`);
});

el('ws-ping').addEventListener('click', () => {
    ws.send('PING');
    log('out', 'PING');
});

el('health-refresh').addEventListener('click', checkHealth);

checkHealth();
ws.connect();
