/**
 * WebSocket-клиент с переподключением.
 *
 * Здесь уже реализовано то, что понадобится в реальной игре и не является
 * преждевременным усложнением:
 *   - экспоненциальный backoff с джиттером: мобильный интернет рвётся постоянно,
 *     а наивный реконнект без задержки устраивает серверу DDoS;
 *   - прикладной heartbeat PING/PONG: TCP-таймауты измеряются минутами,
 *     а ход в игре длится 30 секунд — «висящее, но мёртвое» соединение
 *     иначе не отличить от живого;
 *   - очередь неотправленных команд: ход, нажатый в момент разрыва, не теряется.
 *
 * ⭐ Перед КАЖДЫМ подключением, включая переподключение, запрашивается одноразовый
 * тикет (ADR-005): браузерный WebSocket не умеет слать заголовок Authorization, а тикет
 * живёт секунды и сгорает при использовании. Поэтому реконнект — это всегда сначала
 * REST-запрос, и только потом сокет.
 *
 * Чего ещё нет: отслеживания seq и RESYNC (M4).
 */

import {apiPost} from './rest-client.js';

const PROTOCOL_VERSION = 1;

const BACKOFF_START_MS = 1000;
const BACKOFF_MAX_MS = 30000;
const HEARTBEAT_INTERVAL_MS = 20000;
const HEARTBEAT_MISS_LIMIT = 2;

export class WsClient {
    #url;
    #onEvent;
    #onStatus;

    #socket = null;
    #reconnectDelay = BACKOFF_START_MS;
    #reconnectTimer = null;
    #heartbeatTimer = null;
    #missedPongs = 0;
    #pending = [];
    #commandCounter = 0;
    #closedByUs = false;

    constructor({url, onEvent, onStatus} = {}) {
        this.#url = url ?? defaultUrl();
        this.#onEvent = onEvent ?? (() => {});
        this.#onStatus = onStatus ?? (() => {});
    }

    async connect() {
        this.#closedByUs = false;
        this.#onStatus('connecting');

        let ticket;
        try {
            ticket = (await apiPost('/auth/ws-ticket', {})).ticket;
        } catch {
            // Тикет не выдали — значит, сессии нет. Молча ретраить бессмысленно:
            // пока пользователь не войдёт заново, сокет всё равно не откроется.
            this.#onStatus('unauthorized');
            return;
        }
        if (this.#closedByUs) {
            return;
        }

        const socket = new WebSocket(`${this.#url}?ticket=${encodeURIComponent(ticket)}`);
        this.#socket = socket;

        socket.addEventListener('open', () => {
            this.#reconnectDelay = BACKOFF_START_MS;
            this.#missedPongs = 0;
            this.#onStatus('open');
            this.#startHeartbeat();
            this.#flushPending();
        });

        socket.addEventListener('message', (raw) => {
            let envelope;
            try {
                envelope = JSON.parse(raw.data);
            } catch {
                this.#onEvent({type: 'PARSE_ERROR', payload: {raw: raw.data}});
                return;
            }
            if (envelope.type === 'PONG') {
                this.#missedPongs = 0;
            }
            this.#onEvent(envelope);
        });

        socket.addEventListener('close', () => {
            this.#stopHeartbeat();
            this.#onStatus('closed');
            if (!this.#closedByUs) this.#scheduleReconnect();
        });

        socket.addEventListener('error', () => {
            // 'error' всегда сопровождается 'close' — реконнект планируем только там,
            // иначе получаем две попытки на один разрыв.
            this.#onStatus('error');
        });
    }

    /** Отправляет команду; если соединения нет — кладёт в очередь до восстановления. */
    send(type, payload = null, tableId = null) {
        const envelope = {
            v: PROTOCOL_VERSION,
            id: `c-${++this.#commandCounter}`,
            type,
            ts: Date.now(),
        };
        if (tableId) envelope.tableId = tableId;
        if (payload !== null) envelope.payload = payload;

        if (this.#socket?.readyState === WebSocket.OPEN) {
            this.#socket.send(JSON.stringify(envelope));
        } else {
            this.#pending.push(envelope);
        }
        return envelope;
    }

    close() {
        this.#closedByUs = true;
        this.#stopHeartbeat();
        clearTimeout(this.#reconnectTimer);
        this.#socket?.close();
    }

    #flushPending() {
        const queued = this.#pending;
        this.#pending = [];
        for (const envelope of queued) {
            this.#socket.send(JSON.stringify(envelope));
        }
    }

    #scheduleReconnect() {
        // Джиттер нужен, чтобы после падения сервера все клиенты не вернулись разом.
        const jitter = Math.random() * 0.3 * this.#reconnectDelay;
        const delay = this.#reconnectDelay + jitter;
        this.#onStatus('reconnecting', Math.round(delay));

        this.#reconnectTimer = setTimeout(() => this.connect(), delay);
        this.#reconnectDelay = Math.min(this.#reconnectDelay * 2, BACKOFF_MAX_MS);
    }

    #startHeartbeat() {
        this.#stopHeartbeat();
        this.#heartbeatTimer = setInterval(() => {
            if (this.#missedPongs >= HEARTBEAT_MISS_LIMIT) {
                // Соединение выглядит открытым, но ответа нет — рвём сами,
                // чтобы сработал обычный путь переподключения.
                this.#socket?.close();
                return;
            }
            this.#missedPongs++;
            this.send('PING');
        }, HEARTBEAT_INTERVAL_MS);
    }

    #stopHeartbeat() {
        clearInterval(this.#heartbeatTimer);
        this.#heartbeatTimer = null;
    }
}

/** Сокет всегда на том же origin, что и страница: dev-сервер Vite проксирует /ws на бэкенд. */
function defaultUrl() {
    const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
    return `${scheme}://${location.host}/ws`;
}
