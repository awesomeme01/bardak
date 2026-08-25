package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// LobbyRouter — команды лобби поверх сокета: сесть, встать, объявить готовность.
//
// ⭐ Всё исполняется НА ОЧЕРЕДИ СТОЛА (ADR-007), а не на goroutine сокета: посадка,
// уход и готовность меняют один и тот же стол, и в очереди они выстраиваются в порядок.
// Гонок между «сел» и «встал» не существует по построению.
type LobbyRouter struct {
	Lobby    LobbyPort
	Names    DisplayNames
	Registry *TableRegistry
	Log      *slog.Logger
}

// LobbyPort — что нужно сокету от лобби.
//
// ⭐ Интерфейс на стороне потребителя: посадка и готовность проверяются здесь без базы
// и без сценариев, а application.LobbyService удовлетворяет ему как есть.
type LobbyPort interface {
	Join(ctx context.Context, tableID, userID string) (repository.TablePlayer, error)
	Leave(ctx context.Context, tableID, userID string) error
	SetReady(ctx context.Context, tableID, userID string, ready bool) (repository.TablePlayer, error)
}

// ⭐ Проверка сборкой: сценарий лобби подходит сокету.
var _ LobbyPort = application.LobbyService{}

// DisplayNames — имена игроков для событий лобби.
type DisplayNames interface {
	DisplayNamesOf(ctx context.Context, userIDs []string) (map[string]string, error)
}

// lobbyCommands — что берёт этот маршрутизатор.
var lobbyCommands = map[string]bool{
	"TABLE_JOIN":  true,
	"TABLE_LEAVE": true,
	"TABLE_READY": true,
}

// Handles — берётся ли этот тип команды.
func (r LobbyRouter) Handles(commandType string) bool { return lobbyCommands[commandType] }

// Handle ставит команду в очередь стола.
func (r LobbyRouter) Handle(ctx context.Context, envelope Envelope, client Client) {
	tableID := *envelope.TableID
	runtime := r.Registry.RuntimeFor(tableID)

	if err := runtime.Submit(func() { r.execute(ctx, envelope, tableID, runtime, client) }); err != nil {
		// ⚠️ Очередь переполнена или стол закрыт — отказ, а не молчание: клиент, не
		// получивший ни ответа, ни ошибки, ждёт вечно и не переспрашивает.
		client.Send(ErrorEvent(envelope.ID, &tableID, "TABLE_BUSY", "Стол не принимает команды"))
	}
}

// Disconnect: снимаем подписку и сообщаем остальным, что игрок не на связи.
func (r LobbyRouter) Disconnect(ctx context.Context, tableID string, client Client) {
	if tableID == "" {
		return
	}
	runtime, ok := r.Registry.Find(tableID)
	if !ok {
		return
	}
	_ = runtime.Submit(func() {
		// ⚠️ Порядок обратный посадке: сначала отписка, потом рассылка. Ушедшему
		// «его нет в сети» посылать некуда — соединения уже нет.
		r.Registry.Unsubscribe(tableID, client.UserID)
		runtime.Broadcast(encode(Event("PLAYER_OFFLINE", nil, &tableID,
			r.payload(ctx, client.UserID, nil))))
	})
}

func (r LobbyRouter) execute(ctx context.Context, envelope Envelope, tableID string,
	runtime *TableRuntime, client Client) {
	switch envelope.Type {
	case "TABLE_JOIN":
		seat, err := r.Lobby.Join(ctx, tableID, client.UserID)
		if err != nil {
			r.refuse(envelope, client, err)
			return
		}
		// ⭐ Подписка ПОСЛЕ посадки и ДО рассылки: свой собственный PLAYER_JOINED
		// севший обязан получить — по нему клиент понимает, что сел.
		r.Registry.Subscribe(runtime, client)
		runtime.Broadcast(encode(Event("PLAYER_JOINED", nil, &tableID,
			r.payload(ctx, client.UserID, &seat))))

	case "TABLE_LEAVE":
		if err := r.Lobby.Leave(ctx, tableID, client.UserID); err != nil {
			r.refuse(envelope, client, err)
			return
		}
		// ⭐ Рассылка ДО отписки: уходящий получает своё событие последним сообщением
		// от стола. Отпишись он раньше — встал бы, не увидев подтверждения.
		runtime.Broadcast(encode(Event("PLAYER_LEFT", nil, &tableID,
			r.payload(ctx, client.UserID, nil))))
		r.Registry.Unsubscribe(tableID, client.UserID)

	case "TABLE_READY":
		seat, err := r.Lobby.SetReady(ctx, tableID, client.UserID, readyFlagOf(envelope.Payload))
		if err != nil {
			r.refuse(envelope, client, err)
			return
		}
		runtime.Broadcast(encode(Event("PLAYER_READY", nil, &tableID,
			r.payload(ctx, client.UserID, &seat))))

	default:
		client.Send(ErrorEvent(envelope.ID, &tableID, "UNKNOWN_COMMAND", "Неизвестная команда лобби"))
	}
}

// readyFlagOf — готовность из полезной нагрузки. Нагрузки нет или поле не разобралось —
// это «готов»: команда без тела означает согласие, а не отказ.
func readyFlagOf(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var payload struct {
		Ready *bool `json:"ready"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Ready == nil {
		return true
	}
	return *payload.Ready
}

// payload — тело события лобби.
//
// ⚠️ `seatNo` и `ready` есть только там, где место известно: у PLAYER_LEFT и
// PLAYER_OFFLINE их нет вовсе, а не «пустые». Имя не нашлось — поля тоже нет.
func (r LobbyRouter) payload(ctx context.Context, userID string,
	seat *repository.TablePlayer) map[string]any {
	payload := map[string]any{"userId": userID}
	if names, err := r.Names.DisplayNamesOf(ctx, []string{userID}); err == nil {
		if name, ok := names[userID]; ok {
			payload["displayName"] = name
		}
	}
	if seat != nil {
		payload["seatNo"] = seat.SeatNo
		payload["ready"] = seat.IsReady()
	}
	return payload
}

// refuse переводит отказ сценария в код протокола.
//
// ⭐ Коды те же, что у REST: одно и то же «стол полон» не должно называться по-разному
// в зависимости от того, каким путём игрок постучался.
func (r LobbyRouter) refuse(envelope Envelope, client Client, err error) {
	code, message := lobbyFailure(err)
	if code == "INTERNAL_ERROR" && r.Log != nil {
		r.Log.Error("команда лобби упала", "type", envelope.Type, "err", err)
	}
	client.Send(ErrorEvent(envelope.ID, envelope.TableID, code, message))
}

func lobbyFailure(err error) (string, string) {
	var inMatch application.MatchInProgressError
	if errors.As(err, &inMatch) {
		return "MATCH_IN_PROGRESS", inMatch.Error()
	}
	var seated application.AlreadyAtTableError
	if errors.As(err, &seated) {
		return "ALREADY_AT_TABLE", seated.Error()
	}

	switch {
	case errors.Is(err, application.ErrTableNotFound):
		return "TABLE_NOT_FOUND", "Стол не найден"
	case errors.Is(err, application.ErrTableNotOpen):
		return "TABLE_NOT_OPEN", "За этот стол уже нельзя сесть"
	case errors.Is(err, application.ErrTableFull):
		return "TABLE_FULL", "За столом нет свободных мест"
	case errors.Is(err, application.ErrNotAtTable):
		return "NOT_AT_TABLE", "Ты не за этим столом"
	default:
		return "INTERNAL_ERROR", "Что-то пошло не так"
	}
}
