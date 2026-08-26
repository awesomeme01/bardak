package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/push"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
	"github.com/awesomeme01/bardak/back-go/internal/transport/protocol"
)

// gameCommands — что берёт этот маршрутизатор.
var gameCommands = map[string]bool{
	"MATCH_START": true, "PLAY_CARD": true, "PASS": true, "TAKE": true, "TRANSFER": true,
	"HANG_CARD": true, "HANG_SKIP": true, "CHOOSE_TRUMP": true, "REVEAL_FACE_DOWN": true,
	"STATE_REQUEST": true, "RESYNC": true, "MATCH_LEAVE": true,
}

// MatchPort — идущие матчи.
type MatchPort interface {
	Find(ctx context.Context, tableID string) (*application.MatchSession, bool)
	Start(ctx context.Context, tableID string) (*application.MatchSession, error)
	Finish(tableID string)
}

// MatchResultPort — запись итога матча и пересчёт рейтинга.
type MatchResultPort interface {
	FinishMatch(ctx context.Context, matchID string, seats []application.SeatOwner,
		state game.MatchState, scale game.NavesScale) ([]application.RatingChange, error)
}

// DealRecorderPort — запись раздач, закрытых этим ходом.
type DealRecorderPort interface {
	RecordFinished(ctx context.Context, matchID string, state game.MatchState,
		scale game.NavesScale, playedBefore int) error
}

// MatchLogPort — журнал матча.
type MatchLogPort interface {
	Append(ctx context.Context, matchID string, firstSeq, dealNo int,
		events []repository.MatchEvent) (int, error)
	AppendRejected(ctx context.Context, matchID string, seq, dealNo, actorSeat int,
		commandType, reason string) error
	SaveSnapshot(ctx context.Context, matchID string, seq int, state string) error
	DealsPlayed(ctx context.Context, matchID string, played int) error
	Since(ctx context.Context, matchID string, afterSeq int) ([]repository.MatchEvent, error)
	Finish(ctx context.Context, matchID, status string, loserUserID, abortReason *string,
		at time.Time) error
}

// MatchLobbyPort — что нужно матчу от лобби.
type MatchLobbyPort interface {
	FinishMatch(ctx context.Context, tableID string) error
	Leave(ctx context.Context, tableID, userID string) error
	ByID(ctx context.Context, tableID string) (application.TableSnapshot, error)
}

// TurnNotifierPort — зов к столу того, кого за ним нет.
//
// ⭐ Интерфейс на стороне потребителя, и он же — выключатель: nil означает, что звать
// некому и незачем. Уведомления не должны быть условием игры.
type TurnNotifierPort interface {
	TurnOf(userID, tableID string, present bool, nameOf func() string)
	PausedFor(userID, tableID string, secondsLeft int64, nameOf func() string)
	Present(userID string)
}

// ⭐ Проверка сборкой: зов к столу подходит сокету.
var _ TurnNotifierPort = (*push.TurnNotifier)(nil)

// StateCodecPort — снимок состояния матча.
type StateCodecPort interface {
	EncodeState(state game.MatchState) (string, error)
}

// GameRouter — игровые команды поверх сокета.
//
// ⭐ После каждой принятой команды каждому игроку уходит СВОЯ проекция состояния
// (ADR-002): одно общее сообщение здесь невозможно в принципе — в нём были бы чужие карты.
//
// ⭐ Всё исполняется на очереди стола: один стол — одна goroutine, и гонок между ходами
// не существует по построению.
type GameRouter struct {
	Matches  MatchPort
	Results  MatchResultPort
	Deals    DealRecorderPort
	Log      MatchLogPort
	Lobby    MatchLobbyPort
	State    StateCodecPort
	Registry *TableRegistry
	Clock    *application.TurnClock

	// Notifier — зов к столу отсутствующих. nil — уведомлений нет вовсе.
	Notifier TurnNotifierPort

	// AutoMove — ходить ли за молчащего. ⚠️ Выключено — часов нет вовсе: ход ждёт
	// своего хозяина сколько угодно.
	AutoMove        bool
	TurnTimeout     time.Duration
	DisconnectGrace time.Duration

	Logger *slog.Logger
}

// Handles — берётся ли этот тип команды.
func (r GameRouter) Handles(commandType string) bool { return gameCommands[commandType] }

// Handle ставит команду в очередь стола.
func (r GameRouter) Handle(ctx context.Context, envelope Envelope, client Client) {
	tableID := *envelope.TableID
	runtime := r.Registry.RuntimeFor(tableID)

	if err := runtime.Submit(func() { r.execute(ctx, envelope, tableID, runtime, client) }); err != nil {
		client.Send(ErrorEvent(envelope.ID, &tableID, "TABLE_BUSY", "Стол не принимает команды"))
	}
}

// Disconnect: игрок пропал — матч встаёт на паузу, таймер хода ОСТАНАВЛИВАЕТСЯ (§5.2).
// Не вернулся за отведённое время — матч отменяется, рейтинг не трогается (§5.3).
func (r GameRouter) Disconnect(ctx context.Context, tableID string, client Client) {
	if tableID == "" {
		return
	}
	session, playing := r.Matches.Find(ctx, tableID)
	if !playing {
		return
	}
	if _, seated := session.SeatOf(client.UserID); !seated {
		return
	}
	runtime, ok := r.Registry.Find(tableID)
	if !ok {
		return
	}

	_ = runtime.Submit(func() {
		left := r.Clock.Pause(tableID)
		runtime.Broadcast(encode(Event("MATCH_PAUSED", nil, &tableID, map[string]any{
			"userId":         client.UserID,
			"turnMillisLeft": left.Milliseconds(),
			"graceSeconds":   int(r.DisconnectGrace.Seconds()),
		})))
		// ⭐ Позвать пропавшего: у него есть ровно это окно, чтобы вернуться. Это и есть
		// главный повод для уведомления — цена молчания здесь не «неудобно», а отменённый
		// матч у всех за столом.
		r.callBack(ctx, session.TableID, client.UserID, int64(r.DisconnectGrace.Seconds()))
		r.Clock.ScheduleAbort(tableID, r.DisconnectGrace, func() {
			_ = runtime.Submit(func() { r.abort(ctx, runtime, session, client.UserID) })
		})
	})
}

func (r GameRouter) execute(ctx context.Context, envelope Envelope, tableID string,
	runtime *TableRuntime, client Client) {
	if envelope.Type == "MATCH_START" {
		session, err := r.Matches.Start(ctx, tableID)
		if err != nil {
			r.refuse(envelope, client, err)
			return
		}
		r.Registry.Subscribe(runtime, client)
		r.broadcast(runtime, session, session.NextSeq(), nil)
		r.restartTurnClock(ctx, runtime, session)
		return
	}

	session, playing := r.Matches.Find(ctx, tableID)
	if !playing {
		client.Send(ErrorEvent(envelope.ID, &tableID, "NO_MATCH", "За этим столом матч не идёт"))
		return
	}

	switch envelope.Type {
	case "MATCH_LEAVE":
		// ⭐ Уйти из идущего матча можно, но только явно и с последствиями для всех:
		// партия отменяется целиком. Тихо освободить своё место нельзя — движок продолжал
		// бы ждать ушедшего, а на освободившийся стул сел бы посторонний.
		if _, seated := session.SeatOf(client.UserID); !seated {
			client.Send(ErrorEvent(envelope.ID, &tableID, "NOT_AT_TABLE", "Ты не за этим столом"))
			return
		}
		r.leaveMatch(ctx, runtime, session, client.UserID)
		return

	case "RESYNC":
		r.Registry.Subscribe(runtime, client)
		r.resync(ctx, session, client, envelope)
		r.resumeIfSeated(runtime, session, client)
		return

	case "STATE_REQUEST":
		r.Registry.Subscribe(runtime, client)
		r.sendStateTo(session, client)
		r.resumeIfSeated(runtime, session, client)
		return
	}

	// ⭐ Клиент переотправляет команду после разрыва: он не знает, дошла ли она.
	// Повтор не применяем, но состояние отдаём — иначе он останется в неведении.
	if envelope.ID != nil && session.AlreadyApplied(*envelope.ID) {
		r.sendStateTo(session, client)
		return
	}

	seatNo, seated := session.SeatOf(client.UserID)
	if !seated {
		client.Send(ErrorEvent(envelope.ID, &tableID, "NOT_A_PLAYER", "Ты не играешь за этим столом"))
		return
	}

	command, err := protocol.ToCommand(envelope.Type, seatNo, envelope.Payload)
	if err != nil {
		client.Send(ErrorEvent(envelope.ID, &tableID, "BAD_COMMAND", err.Error()))
		return
	}

	dealsBefore := len(session.State().Results)
	outcome, err := session.Apply(command)
	if err != nil {
		// ⚠️ Поломка движка — не отказ игроку: состояние не менялось, и говорить
		// «ход недопустим» здесь было бы неправдой.
		r.warn("игровая команда упала", "type", envelope.Type, "table", tableID, "err", err)
		client.Send(ErrorEvent(envelope.ID, &tableID, "INTERNAL_ERROR", "Что-то пошло не так"))
		return
	}

	if !outcome.Applied {
		// Отклонённая попытка — часть истории стола, хотя состояние не меняет (§2.1).
		//
		// ⚠️ Отклонённую команду НЕ запоминаем применённой: повтор отклонённого хода —
		// а клиент повторяет сам после обрыва (ADR-052) — обязан вернуть причину отказа,
		// а не снимок. Иначе игрок так и не узнает, почему ход не прошёл.
		seq := session.NextSeq()
		if err := r.Log.AppendRejected(ctx, session.MatchID, seq, session.State().DealNo,
			seatNo, envelope.Type, string(outcome.Reason)); err != nil {
			r.warn("отклонённая попытка не записана", "err", err)
		}
		session.SetLastSeq(seq)
		client.Send(ErrorEvent(envelope.ID, &tableID, string(outcome.Reason), "Ход отклонён"))
		return
	}

	if envelope.ID != nil {
		session.Remember(*envelope.ID)
	}

	firstSeq := session.NextSeq()
	// ⭐ Сначала лог, потом рассылка (ADR-004): иначе после падения между ними клиенты
	// видели бы ход, которого в истории нет.
	last, err := r.Log.Append(ctx, session.MatchID, firstSeq, session.State().DealNo,
		logEventsOf(outcome.Events))
	if err != nil {
		r.warn("события матча не записаны", "err", err)
		client.Send(ErrorEvent(envelope.ID, &tableID, "INTERNAL_ERROR", "Что-то пошло не так"))
		return
	}
	session.SetLastSeq(last)

	state := session.State()
	if err := r.Log.DealsPlayed(ctx, session.MatchID, len(state.Results)); err != nil {
		r.warn("счётчик раздач не обновлён", "err", err)
	}
	if err := r.Deals.RecordFinished(ctx, session.MatchID, state,
		session.Config().NavesScale, dealsBefore); err != nil {
		r.warn("раздача не записана в историю", "err", err)
	}
	r.saveSnapshot(ctx, session)

	r.broadcast(runtime, session, firstSeq, outcome.Events)
	r.restartTurnClock(ctx, runtime, session)
	if r.abortIfStuck(ctx, runtime, session) {
		return
	}
	r.finishIfOver(ctx, runtime, session)
}

// abortIfStuck отменяет матч, если раздача перестала двигаться.
//
// ⚠️ Проверяется ПОСЛЕ рассылки: игроки должны увидеть последний ход и лишь затем узнать,
// что матч отменён. И до finishIfOver — иначе заклинившая раздача, до конца матча
// не доходящая, никогда бы сюда и не добралась.
//
// ⭐ Отмена, а не «пропустить ход»: сервер не знает, какой ход разомкнёт цикл, и гадать
// в этом месте опаснее, чем честно закончить партию. Рейтинг не трогается — это отмена.
func (r GameRouter) abortIfStuck(ctx context.Context, runtime *TableRuntime,
	session *application.MatchSession) bool {
	if !session.DealIsStuck() {
		return false
	}

	state := session.State()
	// Громко: срабатывание предохранителя — это НАХОДКА, а не рядовое событие. Без записи
	// с местом и фазой воспроизвести цикл будет не по чему.
	if r.Logger != nil {
		r.Logger.Error("раздача не двигается — отменяю матч",
			"table", session.TableID, "match", session.MatchID,
			"deal", state.DealNo, "moves", session.MovesInDeal(),
			"phase", state.Deal.Phase, "attackRight", state.Deal.AttackRightSeat,
			"defender", state.Deal.DefenderSeat)
	}

	r.Clock.Cancel(session.TableID)
	r.Clock.CancelAbort(session.TableID)
	r.abortMatch(ctx, session, "Раздача перестала двигаться")
	runtime.Broadcast(encode(Event("MATCH_ABORTED", nil, &session.TableID, map[string]any{
		"reason": "DEAL_STUCK",
	})))
	return true
}

// broadcast рассылает события и персональные снимки.
func (r GameRouter) broadcast(runtime *TableRuntime, session *application.MatchSession,
	firstSeq int, events []game.DealEvent) {
	Broadcast(runtime, session, firstSeq, events, r.turnSecondsLeft(session.TableID))
}

func (r GameRouter) sendStateTo(session *application.MatchSession, client Client) {
	seatNo, seated := session.SeatOf(client.UserID)
	seat := application.SeatOwner{SeatNo: seatNo, UserID: client.UserID}
	if !seated {
		// ⚠️ Наблюдатель смотрит глазами первого места, как в Java: своей проекции
		// у него нет, а отдать состояние «как есть» значило бы показать все руки.
		seat = application.SeatOwner{SeatNo: 0, UserID: client.UserID}
	}
	SendStateTo(session, seat, r.turnSecondsLeft(session.TableID), client.Send)
}

// resync догоняет пропущенное после обрыва.
//
// ⭐ События берутся из лога, но уже отфильтрованные по ЗАПИСАННОЙ видимости: сырой лог
// содержит скрытую информацию и наружу не отдаётся никогда. Следом уходит полный снимок —
// так клиент сходится, даже если пропустил больше, чем помнит сервер.
func (r GameRouter) resync(ctx context.Context, session *application.MatchSession,
	client Client, envelope Envelope) {
	seatNo, seated := session.SeatOf(client.UserID)
	if seated {
		missed, err := r.Log.Since(ctx, session.MatchID, lastSeqOf(envelope.Payload))
		if err != nil {
			r.warn("догон по журналу не собрался", "err", err)
		}
		for _, event := range missed {
			if event.PrivateToSeat != nil && *event.PrivateToSeat != seatNo {
				continue
			}
			client.Send(GameEvent(event.Type, session.TableID, event.Seq,
				json.RawMessage(event.Payload)))
		}
	}
	r.sendStateTo(session, client)
}

// resumeIfSeated — игрок вернулся: продолжаем с остатка, а не с полного хода.
func (r GameRouter) resumeIfSeated(runtime *TableRuntime, session *application.MatchSession,
	client Client) {
	if _, seated := session.SeatOf(client.UserID); !seated {
		return
	}
	r.Clock.CancelAbort(session.TableID)
	if r.Notifier != nil {
		// Игрок вернулся: следующий его ход снова достоин звонка.
		r.Notifier.Present(client.UserID)
	}
	r.Clock.Resume(session.TableID)
	runtime.Broadcast(encode(Event("MATCH_RESUMED", nil, &session.TableID, map[string]any{})))
}

// restartTurnClock перезапускает таймер хода.
//
// ⭐ Срабатывание не делает ничего игрового само: оно кладёт команду в очередь стола.
// Иначе автодействие пришло бы с чужой goroutine и могло бы пересечься с настоящим
// ходом игрока, который успел в последнюю секунду.
func (r GameRouter) restartTurnClock(ctx context.Context, runtime *TableRuntime,
	session *application.MatchSession) {
	state := session.State()
	if state.IsOver() {
		r.Clock.Cancel(session.TableID)
		return
	}
	seat, onTheClock := application.SeatOnTheClock(state.Deal)
	if !onTheClock {
		r.Clock.Cancel(session.TableID)
		return
	}
	r.callToTable(ctx, runtime, session, seat)
	// ⭐ Часы идут, только если стол согласился ходить за молчащего. Без этого ход просто
	// ждёт своего хозяина — сколько угодно, и никто его не отбирает.
	if !r.AutoMove {
		r.Clock.Cancel(session.TableID)
		return
	}
	r.Clock.Start(session.TableID, r.TurnTimeout, func() {
		_ = runtime.Submit(func() { r.applyTimeout(ctx, runtime, session) })
	})
}

// callToTable зовёт к столу того, чей ход.
//
// ⭐ «Нет за столом» определяется по ПОДПИСКЕ на события стола, а не по сокету вообще:
// игрок мог открыть приложение и уйти в другой стол или в историю. Само уведомление
// уходит с чужой goroutine — на goroutine стола ждать ответа push-сервиса нельзя (ADR-007).
func (r GameRouter) callToTable(ctx context.Context, runtime *TableRuntime,
	session *application.MatchSession, seat int) {
	if r.Notifier == nil {
		return
	}
	userID, _ := session.Naming(seat)
	if userID == "" {
		return
	}
	r.Notifier.TurnOf(userID, session.TableID, runtime.Subscribed(userID),
		r.tableNameOf(ctx, session.TableID))
}

// callBack зовёт обратно пропавшего, из-за которого матч встал на паузу.
func (r GameRouter) callBack(ctx context.Context, tableID, userID string, secondsLeft int64) {
	if r.Notifier == nil {
		return
	}
	r.Notifier.PausedFor(userID, tableID, secondsLeft, r.tableNameOf(ctx, tableID))
}

// tableNameOf — имя стола для текста уведомления, добываемое только если звонок состоится.
//
// ⚠️ Имя — украшение уведомления, а не его условие: стол мог закрыться, база — ответить
// ошибкой, и молчать из-за этого нельзя. Текст без имени у отправителя предусмотрен.
func (r GameRouter) tableNameOf(ctx context.Context, tableID string) func() string {
	return func() string {
		snapshot, err := r.Lobby.ByID(ctx, tableID)
		if err != nil {
			r.warn("имя стола для уведомления не прочиталось", "table", tableID, "err", err)
			return ""
		}
		return snapshot.Table.Name
	}
}

// applyTimeout — ход не сделан за отведённое время, сервер делает самое безобидное (§5.1).
func (r GameRouter) applyTimeout(ctx context.Context, runtime *TableRuntime,
	session *application.MatchSession) {
	auto, ok := application.AutoActionFor(session.State().Deal)
	if !ok {
		return
	}
	dealsBefore := len(session.State().Results)
	outcome, err := session.Apply(auto)
	if err != nil || !outcome.Applied {
		r.warn("автодействие не прошло", "table", session.TableID, "err", err)
		return
	}

	firstSeq := session.NextSeq()
	last, err := r.Log.Append(ctx, session.MatchID, firstSeq, session.State().DealNo,
		logEventsOf(outcome.Events))
	if err != nil {
		r.warn("события автодействия не записаны", "err", err)
		return
	}
	session.SetLastSeq(last)

	state := session.State()
	if err := r.Log.DealsPlayed(ctx, session.MatchID, len(state.Results)); err != nil {
		r.warn("счётчик раздач не обновлён", "err", err)
	}
	if err := r.Deals.RecordFinished(ctx, session.MatchID, state,
		session.Config().NavesScale, dealsBefore); err != nil {
		r.warn("раздача не записана в историю", "err", err)
	}
	r.saveSnapshot(ctx, session)

	runtime.Broadcast(encode(Event("TURN_TIMEOUT", nil, &session.TableID,
		map[string]any{"seatNo": auto.SeatNo()})))
	r.broadcast(runtime, session, firstSeq, outcome.Events)
	r.restartTurnClock(ctx, runtime, session)
	if r.abortIfStuck(ctx, runtime, session) {
		return
	}
	r.finishIfOver(ctx, runtime, session)
}

// finishIfOver — матч окончен: записать итог, пересчитать рейтинг, освободить стол.
//
// ⭐ Итог и рейтинг пишутся ОДНОЙ транзакцией, и только после её успеха стол объявляется
// свободным. Иначе стол успел бы открыться для нового матча, а результат прошлого —
// не записаться.
func (r GameRouter) finishIfOver(ctx context.Context, runtime *TableRuntime,
	session *application.MatchSession) {
	state := session.State()
	if !state.IsOver() {
		return
	}

	changes, err := r.Results.FinishMatch(ctx, session.MatchID, session.Seats, state,
		session.Config().NavesScale)
	if err != nil {
		r.warn("итог матча не записан", "match", session.MatchID, "err", err)
		return
	}

	r.Clock.Cancel(session.TableID)
	r.Matches.Finish(session.TableID)
	if err := r.Lobby.FinishMatch(ctx, session.TableID); err != nil {
		r.warn("стол не вернулся в лобби", "table", session.TableID, "err", err)
	}
	runtime.Broadcast(encode(Event("MATCH_OVER", nil, &session.TableID,
		matchOverPayload(session, state, changes))))
}

// leaveMatch — игрок вышел из матча по своей воле.
//
// ⚠️ Отличается от пропажи со связи (§5.2) тем, что ждать некого: человек ушёл сознательно,
// и держать остальных минуту на паузе незачем. Матч отменяется сразу, места освобождаются,
// стол снова открыт — можно собраться заново.
//
// Отменённый матч в рейтинг не идёт (§5.3): уйти из проигранной партии, чтобы она
// не считалась, всё равно нельзя — она не считается ни для кого.
func (r GameRouter) leaveMatch(ctx context.Context, runtime *TableRuntime,
	session *application.MatchSession, userID string) {
	r.Clock.Cancel(session.TableID)
	r.Clock.CancelAbort(session.TableID)
	r.abortMatch(ctx, session, "Игрок вышел из матча")

	runtime.Broadcast(encode(Event("MATCH_ABORTED", nil, &session.TableID, map[string]any{
		"userId": userID,
		"reason": "PLAYER_LEFT",
	})))
	if err := r.Lobby.Leave(ctx, session.TableID, userID); err != nil {
		r.warn("место не освободилось", "table", session.TableID, "err", err)
	}
	r.Registry.Unsubscribe(session.TableID, userID)
}

// abort — пропавший не вернулся за отведённое время.
func (r GameRouter) abort(ctx context.Context, runtime *TableRuntime,
	session *application.MatchSession, userID string) {
	r.Clock.Cancel(session.TableID)
	r.abortMatch(ctx, session, "Игрок не вернулся за отведённое время")
	runtime.Broadcast(encode(Event("MATCH_ABORTED", nil, &session.TableID,
		map[string]any{"userId": userID})))
}

// abortMatch — общая часть отмены: журнал, память, стол.
//
// ⚠️ Стол ОБЯЗАН вернуться в ожидание. Без этого отменённый матч оставлял бы его
// в IN_MATCH навсегда: сесть за него больше нельзя, начать новый матч тоже, и в лобби
// он до конца дней показывал бы «матч идёт». Штатное завершение это делает, а отмена —
// когда-то нет.
func (r GameRouter) abortMatch(ctx context.Context, session *application.MatchSession,
	reason string) {
	if err := r.Log.Finish(ctx, session.MatchID, repository.MatchAborted, nil, &reason,
		time.Now()); err != nil {
		r.warn("матч не помечен отменённым", "match", session.MatchID, "err", err)
	}
	r.Matches.Finish(session.TableID)
	if err := r.Lobby.FinishMatch(ctx, session.TableID); err != nil {
		r.warn("стол не вернулся в лобби", "table", session.TableID, "err", err)
	}
}

// saveSnapshot — состояние после хода: из него матч поднимется, если сервер перезапустят.
func (r GameRouter) saveSnapshot(ctx context.Context, session *application.MatchSession) {
	encoded, err := r.State.EncodeState(session.State())
	if err != nil {
		r.warn("снимок не собрался", "match", session.MatchID, "err", err)
		return
	}
	if err := r.Log.SaveSnapshot(ctx, session.MatchID, session.LastSeq(), encoded); err != nil {
		r.warn("снимок не сохранён", "match", session.MatchID, "err", err)
	}
}

// turnSecondsLeft — остаток хода в секундах, вверх.
//
// ⭐ Считает СЕРВЕР: по этим же часам он сходит за молчащего, и клиентская догадка
// выглядела бы как отобранный ход.
func (r GameRouter) turnSecondsLeft(tableID string) *int {
	left, running := r.Clock.Remaining(tableID)
	if !running {
		return nil
	}
	seconds := int(math.Ceil(float64(left.Milliseconds()) / 1000))
	return &seconds
}

// refuse переводит отказ сценария в код протокола.
func (r GameRouter) refuse(envelope Envelope, client Client, err error) {
	switch {
	case errors.Is(err, application.ErrMatchAlreadyStarted):
		client.Send(ErrorEvent(envelope.ID, envelope.TableID, "MATCH_ALREADY_STARTED",
			"Матч уже идёт"))
	case errors.Is(err, application.ErrTableNotReady):
		client.Send(ErrorEvent(envelope.ID, envelope.TableID, "TABLE_NOT_READY",
			"Не все за столом готовы"))
	default:
		code, message := lobbyFailure(err)
		if code == "INTERNAL_ERROR" {
			r.warn("игровая команда упала", "type", envelope.Type, "err", err)
		}
		client.Send(ErrorEvent(envelope.ID, envelope.TableID, code, message))
	}
}

func (r GameRouter) warn(message string, args ...any) {
	if r.Logger != nil {
		r.Logger.Warn(message, args...)
	}
}

// logEventsOf переводит события движка в строки журнала.
//
// ⚠️ Видимость едет ВМЕСТЕ с событием: по ней потом фильтруется догон после обрыва.
func logEventsOf(events []game.DealEvent) []repository.MatchEvent {
	out := make([]repository.MatchEvent, 0, len(events))
	for _, event := range events {
		payload, err := json.Marshal(protocol.EventPayload(event))
		if err != nil {
			payload = []byte("{}")
		}
		seat := event.SeatNo()
		record := repository.MatchEvent{
			Type:      protocol.EventType(event),
			ActorSeat: &seat,
			Payload:   string(payload),
		}
		if private, isPrivate := event.PrivateToSeat(); isPrivate {
			record.PrivateToSeat = &private
		}
		out = append(out, record)
	}
	return out
}

// lastSeqOf — с какого номера догонять. Нет тела — с начала матча.
func lastSeqOf(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var payload struct {
		LastSeq int `json:"lastSeq"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0
	}
	return payload.LastSeq
}

// matchOverPayload — экран итога матча.
func matchOverPayload(session *application.MatchSession, state game.MatchState,
	changes []application.RatingChange) map[string]any {
	players := make([]map[string]any, 0, len(changes))
	for _, change := range changes {
		_, displayName := session.Naming(change.SeatNo)
		players = append(players, map[string]any{
			"userId":       change.UserID,
			"seatNo":       change.SeatNo,
			"displayName":  displayName,
			"place":        change.Place,
			"navesLevel":   change.NavesLevel,
			"lossDegree":   change.LossDegree,
			"ratingBefore": change.Before,
			"ratingAfter":  change.After,
			"ratingDelta":  change.Delta,
		})
	}

	// ⭐ Последняя атака — часть приговора: по восьмёркам в ней различаются «Королевский»
	// и «Супер-мега-сак» (§0.3). Карты были на столе у всех на виду, скрывать нечего.
	lastAttack := []string{}
	if outcome, ok := state.LastResult(); ok {
		lastAttack = protocol.EncodeCards(outcome.LastAttackCards)
	}

	return map[string]any{
		"matchId": session.MatchID,
		// Сколько раздач шёл матч: степень проигрыша читается иначе, если до неё ехали
		// двадцать раздач, а не три.
		"dealsPlayed":     len(state.Results),
		"players":         players,
		"lastAttackCards": lastAttack,
	}
}
