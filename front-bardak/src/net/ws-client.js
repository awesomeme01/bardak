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
 * ⭐ Восстановление после разрыва — не только «сокет снова открыт». Пока нас не было,
 * за столом что-то происходило, и клиент об этом не знает. Поэтому у соединения есть
 * отдельный обработчик `onReconnect`: он вызывается только на повторных подключениях
 * и просит догон (`RESYNC`) — при первом подключении догонять нечего.
 */

import {apiPost} from './rest-client.js';

const PROTOCOL_VERSION = 1;

const BACKOFF_START_MS = 1000;
const BACKOFF_MAX_MS = 30000;
const HEARTBEAT_INTERVAL_MS = 20000;
const HEARTBEAT_MISS_LIMIT = 2;

/**
 * ⭐ Сколько живёт отложенная команда.
 *
 * Ход, нажатый в момент разрыва, терять нельзя — но и отправлять его через минуту тоже:
 * за это время сервер сходил за игрока по таймауту (§5.1), раздача ушла вперёд, и старый
 * ход означал бы уже совсем другое. Срок равен таймауту хода: позже него команда
 * заведомо опоздала.
 */
const COMMAND_TTL_MS = 30000;

/**
 * Сколько раз пробовать взять тикет, прежде чем признать сессию потерянной.
 *
 * ⚠️ Не «один раз»: отказ мог быть временным — истёкший access-токен, не успевшее
 * обновление пары, секундная недоступность. Сдаться сразу означает тихо мёртвый сокет
 * до конца партии.
 */
const TICKET_ATTEMPTS = 4;

export class WsClient {
    #url;
    #onEvent;
    #onStatus;
    #onReconnect;

    #socket = null;
    #reconnectDelay = BACKOFF_START_MS;
    #reconnectTimer = null;
    #heartbeatTimer = null;
    #missedPongs = 0;
    #pending = [];
    #commandCounter = 0;

    /**
     * ⚠️ Префикс уникален для этого экземпляра клиента и переживает только его.
     *
     * Сервер отсеивает повторы по идентификатору команды: клиент переотправляет ход после
     * разрыва, не зная, дошёл ли он, и применить его дважды нельзя. Но идентификатор был
     * простым счётчиком `c-1`, `c-2`, а счётчик обнулялся на каждой перезагрузке страницы.
     * После обновления первый же ход снова назывался `c-1`, сервер узнавал в нём уже
     * применённую команду и молча возвращал состояние. Со стороны игрока — «кнопка
     * не реагирует», причём тем чаще, чем больше он ходил по экранам.
     *
     * ⚠️ `crypto.randomUUID` здесь не годится: он есть только в защищённом контексте,
     * а по локальной сети игра открывается по обычному http.
     */
    #commandPrefix = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;
    #closedByUs = false;
    #ticketFailures = 0;

    /** Было ли соединение уже открыто: отличает переподключение от первого входа. */
    #wasConnected = false;

    /**
     * @param {{url?: string, onEvent?: (envelope: any) => void,
     *          onStatus?: (status: string, retryInMs?: number) => void,
     *          onReconnect?: () => void}} [options]
     */
    constructor({url, onEvent, onStatus, onReconnect} = {}) {
        this.#url = url ?? defaultUrl();
        this.#onEvent = onEvent ?? (() => {});
        this.#onStatus = onStatus ?? (() => {});
        this.#onReconnect = onReconnect ?? (() => {});
    }

    async connect() {
        this.#closedByUs = false;
        this.#onStatus('connecting');

        let ticket;
        try {
            ticket = (await apiPost('/auth/ws-ticket', {})).ticket;
        } catch (error) {
            // ⭐ Сервер не ответил — это не отказ в доступе, а обрыв связи. Раньше здесь
            // обе беды лечились одинаково: попытка прекращалась навсегда, и партия
            // не переживала даже секундного пропадания сети.
            if (error?.code === 'NETWORK_UNAVAILABLE') {
                this.#onStatus('offline');
                this.#scheduleReconnect();
                return;
            }
            // ⚠️ Тикет не выдали по существу. Раньше попытки прекращались здесь навсегда,
            // и это было хуже, чем кажется: сокет тихо умирал, экран оставался прежним,
            // а каждый ход уходил в очередь, из которой его уже никто не доставал.
            // Со стороны игрока это выглядело как «кнопки не реагируют».
            //
            // Отказ не всегда вечный: access-токен живёт минуты, и обновление пары могло
            // просто не успеть. Поэтому пробуем ещё несколько раз и только потом сдаёмся.
            this.#ticketFailures++;
            if (this.#ticketFailures < TICKET_ATTEMPTS) {
                this.#onStatus('reconnecting');
                this.#scheduleReconnect();
                return;
            }
            this.#onStatus('unauthorized');
            return;
        }
        this.#ticketFailures = 0;
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
            // ⭐ Сначала догон, потом отложенные ходы: сервер должен получить их
            // в состоянии, которое мы уже знаем, а мы — увидеть пропущенное.
            if (this.#wasConnected) {
                this.#onReconnect();
            }
            this.#wasConnected = true;
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

    /** Открыт ли сокот прямо сейчас — по нему видно, уйдёт команда сразу или в очередь. */
    get connected() {
        return this.#socket?.readyState === WebSocket.OPEN;
    }

    /** Отправляет команду; если соединения нет — кладёт в очередь до восстановления. */
    send(type, payload = null, tableId = null) {
        const envelope = {
            v: PROTOCOL_VERSION,
            id: `${this.#commandPrefix}-${++this.#commandCounter}`,
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
        const now = Date.now();
        for (const envelope of queued) {
            if (now - envelope.ts > COMMAND_TTL_MS) {
                this.#onEvent({type: 'COMMAND_EXPIRED', payload: {type: envelope.type}});
                continue;
            }
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
