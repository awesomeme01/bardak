package protocol

import "github.com/awesomeme01/bardak/back-go/internal/domain/game"

// EventType — имя события на проводе.
//
// В Java оно выводилось из имени класса (CardAttacked → CARD_ATTACKED); здесь список
// явный. Явный список длиннее, зато новый тип события нельзя забыть назвать: компилятор
// промолчит, а клиент получит пустое имя.
func EventType(event game.DealEvent) string {
	switch event.(type) {
	case game.CardAttacked:
		return "CARD_ATTACKED"
	case game.CardDefended:
		return "CARD_DEFENDED"
	case game.AttackTransferred:
		return "ATTACK_TRANSFERRED"
	case game.FaceDownRevealed:
		return "FACE_DOWN_REVEALED"
	case game.Passed:
		return "PASSED"
	case game.AttackRightMoved:
		return "ATTACK_RIGHT_MOVED"
	case game.RoundBeaten:
		return "ROUND_BEATEN"
	case game.TakeAnnounced:
		return "TAKE_ANNOUNCED"
	case game.CardsTaken:
		return "CARDS_TAKEN"
	case game.CardsDrawn:
		return "CARDS_DRAWN"
	case game.PlayerLeftDeal:
		return "PLAYER_LEFT_DEAL"
	case game.HiddenTrumpRevealed:
		return "HIDDEN_TRUMP_REVEALED"
	case game.TrumpChanged:
		return "TRUMP_CHANGED"
	case game.TrumpChosen:
		return "TRUMP_CHOSEN"
	case game.HangingWindowOpened:
		return "HANGING_WINDOW_OPENED"
	case game.CardHung:
		return "CARD_HUNG"
	case game.NavesLevelChanged:
		return "NAVES_LEVEL_CHANGED"
	case game.DiceRolled:
		return "DICE_ROLLED"
	case game.HangingWindowClosed:
		return "HANGING_WINDOW_CLOSED"
	case game.DealFinished:
		return "DEAL_FINISHED"
	default:
		return "UNKNOWN_EVENT"
	}
}

// EventPayload — полезная нагрузка события.
//
// ⚠️ Здесь же проходит граница тумана войны: CARDS_TAKEN, ROUND_BEATEN и CARDS_DRAWN
// отдают только КОЛИЧЕСТВО карт, а не сами карты. Отдать состав — значит показать всем,
// что именно уехало в чужую руку, и игра перестанет быть игрой.
//
// Приватность самого события (FACE_DOWN_REVEALED) решает не здесь, а рассылка:
// она сверяет PrivateToSeat.
func EventPayload(event game.DealEvent) map[string]any {
	payload := map[string]any{"seatNo": event.SeatNo()}

	switch actual := event.(type) {
	case game.CardAttacked:
		payload["cardCode"] = EncodeCard(actual.Card)
	case game.CardDefended:
		payload["cardCode"] = EncodeCard(actual.Card)
		payload["targetCardCode"] = EncodeCard(actual.Target)
	case game.AttackTransferred:
		payload["cardCode"] = EncodeCard(actual.Card)
		payload["toSeatNo"] = actual.ToSeatNo
	case game.FaceDownRevealed:
		payload["cardCode"] = EncodeCard(actual.Card)
	case game.HiddenTrumpRevealed:
		// ⭐ Здесь карта видна ВСЕМ, в отличие от скрытой карты игрока: она меняет
		// козырь всему столу.
		payload["cardCode"] = EncodeCard(actual.Card)
	case game.TrumpChanged:
		payload["suit"] = SuitName(actual.Suit)
	case game.TrumpChosen:
		payload["suit"] = SuitName(actual.Suit)
	case game.CardHung:
		payload["cardCode"] = EncodeCard(actual.Card)
		payload["victimSeat"] = actual.VictimSeat
	case game.NavesLevelChanged:
		payload["level"] = actual.Level
	case game.CardsTaken:
		payload["count"] = len(actual.Cards)
	case game.RoundBeaten:
		payload["count"] = len(actual.Discarded)
	case game.CardsDrawn:
		payload["count"] = len(actual.Cards)
	case game.DiceRolled:
		payload["participants"] = actual.Participants
	}
	return payload
}

// IsVisibleTo — видно ли событие игроку на этом месте.
//
// ⭐ Единственный приватный случай — вскрытие скрытой карты: она уходит в руку владельца
// и дальше играется как обычная, а чужую руку не видит никто. Остальные узнают только то,
// что видно в проекции: скрытой карты у него больше нет.
func IsVisibleTo(event game.DealEvent, seatNo int) bool {
	private, isPrivate := event.PrivateToSeat()
	return !isPrivate || private == seatNo
}
