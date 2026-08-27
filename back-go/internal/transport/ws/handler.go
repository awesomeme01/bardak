package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// Пороги соединения.
const (
	// pingEvery — как часто сервер напоминает о себе.
	pingEvery = 25 * time.Second
	// readLimit — потолок одного сообщения. Игровая команда маленькая; всё, что больше,
	// либо ошибка клиента, либо попытка занять память.
	readLimit = 64 * 1024
	// writeTimeout — сколько ждём отправки одного сообщения.
	writeTimeout = 10 * time.Second
)

// TicketRedeemer — гашение одноразового тикета рукопожатия.
type TicketRedeemer interface {
	Redeem(value string) (string, bool)
}

// Client — одно соединение глазами маршрутизатора.
//
// ⭐ Отправок ДВЕ, и это не дублирование. Send — прямой ответ этому соединению (ошибка,
// снимок по запросу). SendRaw качает очередь подписки на стол: там лежат уже собранные
// сообщения, общие для всех подписчиков, и разбирать их обратно в конверт ради типа
// значило бы делать двойную работу на каждом ходе за каждым столом.
type Client struct {
	UserID  string
	Send    func(Envelope)
	SendRaw func([]byte)
}

// CommandRouter — куда уходят разобранные команды.
//
// ⭐ Обработчик сокета не знает ни правил, ни столов: его дело — разобрать конверт
// и передать дальше. Игровая логика в транспорте — верный способ развести поведение
// между REST и сокетом.
type CommandRouter interface {
	// Handles — берётся ли этот тип команды.
	Handles(commandType string) bool
	// Handle обрабатывает команду.
	Handle(ctx context.Context, envelope Envelope, client Client)
	// Disconnect вызывается при обрыве.
	Disconnect(ctx context.Context, tableID string, client Client)
}

// Handler — точка входа /ws.
type Handler struct {
	Tickets TicketRedeemer
	Routers []CommandRouter
	Origins []string

	// Presence — реестр «кто сейчас в сети». Может быть nil: игра от него не зависит,
	// от него зависят друзья.
	Presence PresenceRegistry

	// Base — контекст СЕРВЕРА, а не соединения.
	//
	// ⚠️ Команда стола исполняется в очереди и переживает своё соединение: игрок мог
	// закрыть вкладку сразу после хода. Возьми она контекст сокета — запись хода в базу
	// отменялась бы на полпути вместе с соединением, а матч оставался бы с состоянием
	// в памяти и без записи в журнале. Живой прогон нашёл ровно это.
	Base context.Context

	Log *slog.Logger
}

// PresenceRegistry — то, что сокету нужно от присутствия.
//
// ⭐ Присутствие — это ЖИВОЙ СОКЕТ, и заводится оно здесь, а не в лобби и не за столом:
// друг «в сети» ровно пока открыто соединение, и через это же соединение до него доходит
// приглашение за стол. Java делает так же (EchoWebSocketHandler).
type PresenceRegistry interface {
	// Attach регистрирует канал доставки и возвращает функцию отключения.
	Attach(userID, channelID string, send func([]byte)) func()
}

// ServeHTTP выполняет рукопожатие и ведёт соединение.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// ⚠️ Тикет гасится ДО апгрейда: отказ должен быть обычным HTTP 401, а не разрывом
	// уже открытого сокета — клиент иначе не отличит «не пустили» от «связь упала».
	userID, ok := h.Tickets.Redeem(r.URL.Query().Get("ticket"))
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.Origins,
	})
	if err != nil {
		if h.Log != nil {
			h.Log.Warn("рукопожатие не удалось", "err", err)
		}
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(readLimit)

	// ⚠️ Ни чтение, ни запись НЕ БЕРУТ контекст HTTP-запроса. После апгрейда соединение
	// перехвачено, и net/http гасит контекст запроса почти сразу — сокет при этом живой.
	// Взять его — значит через мгновение молча перестать отправлять (`conn.Write` падает
	// с «context canceled», а игрок видит зависший стол) и оборвать чтение.
	//
	// Живой прогон нашёл именно это: первые сообщения доходили, следующие — нет.
	base := h.Base
	if base == nil {
		base = context.Background()
	}
	// Жизнь соединения: рвётся своим закрытием, обрывом или остановкой сервера.
	ctx, cancel := context.WithCancel(base)
	defer cancel()

	// Контекст стола живёт со всем сервером: команда переживает своё соединение.
	tableCtx := base

	sendRaw := h.rawSender(ctx, conn)
	send := func(envelope Envelope) {
		raw, err := json.Marshal(envelope)
		if err != nil {
			return
		}
		sendRaw(raw)
	}
	client := Client{UserID: userID, Send: send, SendRaw: sendRaw}

	sessionID := uuid.NewString()

	// ⭐ В сети — с этой секунды и ровно до закрытия сокета. Отметка времени врала бы
	// в обе стороны: закрывший вкладку числился бы онлайн ещё минуту, а задумавшийся
	// над ходом успел бы «уйти».
	//
	// ⚠️ Канал доставки — sendRaw, а не send: приглашение за стол приходит уже собранным
	// конвертом, и заворачивать его второй раз значит отправить игроку конверт в конверте.
	if h.Presence != nil {
		detach := h.Presence.Attach(userID, sessionID, sendRaw)
		defer detach()
	}

	// Payload — как у Java: {sessionId, protocolVersion}. Фронт его не читает,
	// но differential по сокету сверяет каждый кадр, и «почти такой же» не проходит.
	send(Event("CONNECTED", nil, nil, map[string]any{
		"sessionId":       sessionID,
		"protocolVersion": ProtocolVersion,
	}))

	// ⭐ Heartbeat своей goroutine: без него мёртвое соединение висит до таймаута
	// операционной системы, а игроки за столом ждут ушедшего, которого уже нет.
	go h.heartbeat(ctx, conn, cancel)

	h.readLoop(ctx, tableCtx, conn, client)
}

func (h Handler) heartbeat(ctx context.Context, conn *websocket.Conn, cancel context.CancelFunc) {
	ticker := time.NewTicker(pingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, done := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pingCtx)
			done()
			if err != nil {
				cancel()
				return
			}
		}
	}
}

func (h Handler) readLoop(ctx, tableCtx context.Context, conn *websocket.Conn, client Client) {
	send := client.Send
	var lastTable string

	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			// Обрыв — обычное дело: вкладку закрыли, телефон уснул.
			for _, router := range h.Routers {
				router.Disconnect(tableCtx, lastTable, client)
			}
			return
		}

		envelope, err := ParseEnvelope(raw)
		if err != nil {
			send(ErrorEvent(nil, nil, "BAD_ENVELOPE", "Сообщение не разобрано как конверт протокола"))
			continue
		}
		// ⚠️ Транспортные ошибки уходят БЕЗ tableId, даже если команда его несла:
		// так делает Java, и differential по сокету сверяет это дословно.
		if envelope.V != ProtocolVersion {
			send(ErrorEvent(envelope.ID, nil, "PROTOCOL_VERSION_UNSUPPORTED",
				"Поддерживается версия протокола 1"))
			continue
		}
		if strings.TrimSpace(envelope.Type) == "" {
			send(ErrorEvent(envelope.ID, nil, "TYPE_REQUIRED", "Не указан тип сообщения"))
			continue
		}

		if envelope.Type == "PING" {
			send(Event("PONG", envelope.ID, envelope.TableID, nil))
			continue
		}

		routed := false
		for _, router := range h.Routers {
			if !router.Handles(envelope.Type) {
				continue
			}
			// ⚠️ Идентификатор стола разбирается ЗДЕСЬ и мягко: опечатка в нём не должна
			// рвать соединение. В Java голый UUID.fromString рвал сокет с SERVER_ERROR,
			// и клиент получал обрыв вместо ошибки. Тексты — дословно как у Java:
			// «нет стола» и «стол не разобран» — разные ответы.
			if envelope.TableID == nil {
				send(ErrorEvent(envelope.ID, nil, "TABLE_ID_INVALID", "Не указан стол"))
				routed = true
				break
			}
			if !isUUID(*envelope.TableID) {
				send(ErrorEvent(envelope.ID, nil, "TABLE_ID_INVALID",
					"Идентификатор стола не разобран"))
				routed = true
				break
			}
			lastTable = *envelope.TableID
			router.Handle(tableCtx, envelope, client)
			routed = true
			break
		}
		if !routed {
			// Эхо, а не ошибка — наследие M1 в эталоне, но контракт есть контракт:
			// клиент с опечаткой в типе получает своё сообщение назад, не отказ.
			// ⚠️ Отсутствовавший payload возвращается ЯВНЫМ null — как ObjectNode.set
			// у Jackson, а не выпадает по omitempty.
			send(Event("ECHO", envelope.ID, envelope.TableID, map[string]any{
				"echoOf":  envelope.Type,
				"payload": envelope.Payload,
			}))
		}
	}
}

// rawSender — отправка готового сообщения с таймаутом. Возвращается замыканием, чтобы
// обработчики команд не знали ни про соединение, ни про контекст.
//
// ⚠️ Запись сериализуется мьютексом: сообщения приходят из ДВУХ источников — прямой ответ
// с goroutine сокета и рассылка стола со своей. Две одновременные записи в один
// websocket.Conn перемешали бы кадры, а выглядело бы это как «клиент получил мусор».
func (h Handler) rawSender(ctx context.Context, conn *websocket.Conn) func([]byte) {
	var mu sync.Mutex
	return func(raw []byte) {
		mu.Lock()
		defer mu.Unlock()
		writeCtx, done := context.WithTimeout(ctx, writeTimeout)
		defer done()
		if err := conn.Write(writeCtx, websocket.MessageText, raw); err != nil && h.Log != nil {
			h.Log.Debug("не удалось отправить", "err", err)
		}
	}
}

// isUUID — форма идентификатора. Разбор, а не регулярка: так же, как это делает база.
func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}
