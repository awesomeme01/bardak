package ws

import (
	"encoding/json"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/transport/protocol"
)

// Broadcast рассылает события и снимок состояния игрокам матча.
//
// ⭐ ПОРЯДОК ЗДЕСЬ — ЧАСТЬ КОНТРАКТА, и он неочевиден. Разбор поведения Java дал четыре
// правила, каждое из которых легко нарушить незаметно:
//
//  1. Внешний цикл по ИГРОКАМ, внутренний по событиям. Для отдельного клиента это
//     неотличимо от «сначала все события, потом снимки», но порядок записи в сокеты
//     именно такой.
//  2. События уходят ПЕРЕД снимком. Клиент анимирует по событию, а состояние берёт
//     из снимка: приди снимок раньше, анимация играла бы из уже нового состояния —
//     то есть игрок увидел бы следствие раньше причины.
//  3. Номер события растёт НЕЗАВИСИМО от видимости. Игрок, которому событие не видно,
//     получает ДЫРУ в нумерации — и это штатно. Нумеровать только отправленное значит
//     разойтись с журналом матча, по которому потом воспроизводится партия.
//  4. Рассылка идёт только игрокам матча. Наблюдатели через этот путь не получают ничего.
func Broadcast(runtime *TableRuntime, session *application.MatchSession, firstSeq int,
	events []game.DealEvent, turnSecondsLeft *int) {
	state := session.State()

	for _, seat := range session.Seats {
		seq := firstSeq
		for _, event := range events {
			if protocol.IsVisibleTo(event, seat.SeatNo) {
				runtime.SendTo(seat.UserID, encode(GameEvent(
					protocol.EventType(event), session.TableID, seq,
					protocol.EventPayload(event))))
			}
			// ⚠️ Увеличиваем ВНЕ проверки видимости — см. правило 3.
			seq++
		}
		runtime.SendTo(seat.UserID, encode(stateSyncFor(session, seat, state, turnSecondsLeft)))
	}
}

// SendStateTo отправляет снимок одному игроку — ответ на STATE_REQUEST и на повтор команды.
func SendStateTo(session *application.MatchSession, seat application.SeatOwner,
	turnSecondsLeft *int, send func(Envelope)) {
	send(stateSyncFor(session, seat, session.State(), turnSecondsLeft))
}

func stateSyncFor(session *application.MatchSession, seat application.SeatOwner,
	state game.MatchState, turnSecondsLeft *int) Envelope {
	view, err := session.ProjectFor(seat.SeatNo)
	if err != nil {
		// ⚠️ Проекция не должна падать на живом состоянии. Если всё же упала — молчим
		// этому игроку, но не роняем рассылку остальным: их партия не виновата.
		return ErrorEvent(nil, StringPtr(session.TableID), "INTERNAL_ERROR",
			"Не удалось собрать состояние")
	}
	sync := protocol.ToStateSync(session.TableID, state.DealNo, view, session.Naming, turnSecondsLeft)
	return Event("STATE_SYNC", nil, StringPtr(session.TableID), sync)
}

func encode(envelope Envelope) []byte {
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil
	}
	return raw
}
