package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
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

// CommandRouter — куда уходят разобранные команды.
//
// ⭐ Обработчик сокета не знает ни правил, ни столов: его дело — разобрать конверт
// и передать дальше. Игровая логика в транспорте — верный способ развести поведение
// между REST и сокетом.
type CommandRouter interface {
	// Handles — берётся ли этот тип команды.
	Handles(commandType string) bool
	// Handle обрабатывает команду; send отправляет ответ лично этому соединению.
	Handle(ctx context.Context, envelope Envelope, userID string, send func(Envelope))
	// Disconnect вызывается при обрыве.
	Disconnect(ctx context.Context, tableID, userID string)
}

// Handler — точка входа /ws.
type Handler struct {
	Tickets TicketRedeemer
	Routers []CommandRouter
	Origins []string
	Log     *slog.Logger
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

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	send := h.sender(ctx, conn)
	send(Event("CONNECTED", nil, nil, map[string]any{
		"userId":  userID,
		"session": uuid.NewString(),
	}))

	// ⭐ Heartbeat своей goroutine: без него мёртвое соединение висит до таймаута
	// операционной системы, а игроки за столом ждут ушедшего, которого уже нет.
	go h.heartbeat(ctx, conn, cancel)

	h.readLoop(ctx, conn, userID, send)
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

func (h Handler) readLoop(ctx context.Context, conn *websocket.Conn, userID string,
	send func(Envelope)) {
	var lastTable string

	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			// Обрыв — обычное дело: вкладку закрыли, телефон уснул.
			for _, router := range h.Routers {
				router.Disconnect(ctx, lastTable, userID)
			}
			return
		}

		envelope, err := ParseEnvelope(raw)
		if err != nil {
			send(ErrorEvent(nil, nil, "BAD_ENVELOPE", "Сообщение не разобрано как конверт протокола"))
			continue
		}
		if envelope.V != ProtocolVersion {
			send(ErrorEvent(envelope.ID, envelope.TableID, "PROTOCOL_VERSION_UNSUPPORTED",
				"Поддерживается версия протокола 1"))
			continue
		}
		if strings.TrimSpace(envelope.Type) == "" {
			send(ErrorEvent(envelope.ID, envelope.TableID, "TYPE_REQUIRED", "Не указан тип сообщения"))
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
			// и клиент получал обрыв вместо ошибки.
			if envelope.TableID == nil || !isUUID(*envelope.TableID) {
				send(ErrorEvent(envelope.ID, nil, "TABLE_ID_INVALID",
					"Идентификатор стола не разобран"))
				routed = true
				break
			}
			lastTable = *envelope.TableID
			router.Handle(ctx, envelope, userID, send)
			routed = true
			break
		}
		if !routed {
			send(ErrorEvent(envelope.ID, envelope.TableID, "UNKNOWN_COMMAND",
				"Неизвестная команда"))
		}
	}
}

// sender — отправка с таймаутом. Возвращается замыканием, чтобы обработчики команд
// не знали ни про соединение, ни про контекст.
func (h Handler) sender(ctx context.Context, conn *websocket.Conn) func(Envelope) {
	return func(envelope Envelope) {
		raw, err := json.Marshal(envelope)
		if err != nil {
			return
		}
		writeCtx, done := context.WithTimeout(ctx, writeTimeout)
		defer done()
		if err := conn.Write(writeCtx, websocket.MessageText, raw); err != nil && h.Log != nil {
			h.Log.Debug("не удалось отправить", "type", envelope.Type, "err", err)
		}
	}
}

// isUUID — форма идентификатора. Разбор, а не регулярка: так же, как это делает база.
func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}
