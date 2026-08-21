package protocol

import "github.com/awesomeme01/bardak/back-go/internal/domain/game"

// StateSync — персональный снимок состояния для клиента.
//
// ⚠️ Nullable-поля — указатели с omitempty, остальные — значения БЕЗ него. Java вырезает
// только null: `mySeat: 0` и `deckLeft: 0` обязаны остаться, а `trumpSuit` при споре
// за козырь — исчезнуть (MD-003).
type StateSync struct {
	TableID           string       `json:"tableId"`
	DealNo            int          `json:"dealNo"`
	Phase             string       `json:"phase"`
	TrumpSuit         *string      `json:"trumpSuit,omitempty"`
	TrumpCard         *string      `json:"trumpCard,omitempty"`
	ProtectedSuit     *string      `json:"protectedSuit,omitempty"`
	DeckLeft          int          `json:"deckLeft"`
	DiscardCount      int          `json:"discardCount"`
	MyHand            []string     `json:"myHand"`
	IHaveHiddenCard   bool         `json:"iHaveHiddenCard"`
	MySeat            int          `json:"mySeat"`
	Table             []SlotView   `json:"table"`
	Players           []SeatState  `json:"players"`
	RoundStarterSeat  int          `json:"roundStarterSeat"`
	DefenderSeat      int          `json:"defenderSeat"`
	CanAttackSeat     int          `json:"canAttackSeat"`
	HangingVictimSeat *int         `json:"hangingVictimSeat,omitempty"`
	TurnSecondsLeft   *int         `json:"turnSecondsLeft,omitempty"`
	AvailableActions  []ActionView `json:"availableActions"`
}

// SlotView — пара «атака — чем бита» на столе.
type SlotView struct {
	Attack string  `json:"attack"`
	Defend *string `json:"defend,omitempty"`
}

// SeatState — место за столом глазами смотрящего.
type SeatState struct {
	SeatNo        int      `json:"seatNo"`
	UserID        string   `json:"userId"`
	DisplayName   string   `json:"displayName"`
	CardsCount    int      `json:"cardsCount"`
	HasHiddenCard bool     `json:"hasHiddenCard"`
	HungCards     []string `json:"hungCards"`
	NavesLevel    int      `json:"navesLevel"`
	NextNavesRank *string  `json:"nextNavesRank,omitempty"`
	NextIsJoker   bool     `json:"nextIsJoker"`
	Passed        bool     `json:"passed"`
	InDeal        bool     `json:"inDeal"`
	ExitPlace     *int     `json:"exitPlace,omitempty"`
	StepsToJoker  int      `json:"stepsToJoker"`
}

// ActionView — что игрок может сделать прямо сейчас.
//
// ⭐ Считает СЕРВЕР: фронт правил не знает и знать не должен. Иначе правила жили бы
// в двух местах и однажды разошлись бы — а разошедшиеся правила видит игрок, не тест.
type ActionView struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// SeatNaming — как назвать место: кто там сидит и под каким именем.
type SeatNaming func(seatNo int) (userID, displayName string)

// ToStateSync переводит проекцию в снимок для клиента.
func ToStateSync(tableID string, dealNo int, view game.PlayerView, naming SeatNaming,
	turnSecondsLeft *int) StateSync {
	sync := StateSync{
		TableID:          tableID,
		DealNo:           dealNo,
		Phase:            view.Phase.String(),
		DeckLeft:         view.DeckLeft,
		DiscardCount:     view.DiscardCount,
		MyHand:           EncodeCards(view.MyHand),
		IHaveHiddenCard:  view.IHaveHiddenCard,
		MySeat:           view.MySeat,
		Table:            make([]SlotView, 0, len(view.Table)),
		Players:          make([]SeatState, 0, len(view.Seats)),
		RoundStarterSeat: view.RoundStarterSeat,
		DefenderSeat:     view.DefenderSeat,
		CanAttackSeat:    view.CanAttackSeat,
		TurnSecondsLeft:  turnSecondsLeft,
		AvailableActions: make([]ActionView, 0, len(view.AvailableActions)),
	}

	if view.TrumpSuit != nil {
		name := SuitName(*view.TrumpSuit)
		sync.TrumpSuit = &name
	}
	if view.TrumpCard != nil {
		code := EncodeCard(view.TrumpCard)
		sync.TrumpCard = &code
	}
	if view.ProtectedSuit != nil {
		name := SuitName(*view.ProtectedSuit)
		sync.ProtectedSuit = &name
	}
	if view.HangingVictimSeat != nil {
		victim := *view.HangingVictimSeat
		sync.HangingVictimSeat = &victim
	}

	for _, slot := range view.Table {
		entry := SlotView{Attack: EncodeCard(slot.Attack)}
		if slot.Defence != nil {
			defence := EncodeCard(slot.Defence)
			entry.Defend = &defence
		}
		sync.Table = append(sync.Table, entry)
	}

	for _, seat := range view.Seats {
		userID, displayName := "", ""
		if naming != nil {
			userID, displayName = naming(seat.SeatNo)
		}
		entry := SeatState{
			SeatNo:        seat.SeatNo,
			UserID:        userID,
			DisplayName:   displayName,
			CardsCount:    seat.CardsCount,
			HasHiddenCard: seat.HasHiddenCard,
			HungCards:     EncodeCards(seat.HungCards),
			NavesLevel:    seat.NavesLevel,
			NextIsJoker:   seat.NextIsJoker,
			Passed:        seat.Passed,
			InDeal:        seat.InDeal,
			StepsToJoker:  seat.StepsToJoker,
		}
		if seat.NextNavesRank != nil {
			code := seat.NextNavesRank.Code()
			entry.NextNavesRank = &code
		}
		if seat.ExitPlace != nil {
			place := *seat.ExitPlace
			entry.ExitPlace = &place
		}
		sync.Players = append(sync.Players, entry)
	}

	for _, action := range view.AvailableActions {
		sync.AvailableActions = append(sync.AvailableActions, ToActionView(action))
	}
	return sync
}

// ToActionView переводит команду движка в описание доступного действия.
//
// ⚠️ Атака и защита обе приходят как PLAY_CARD и различаются наличием targetCardCode:
// смысл действия задаёт роль, а не отдельный тип команды. Так же в Java.
func ToActionView(command game.DealCommand) ActionView {
	payload := map[string]any{}

	switch actual := command.(type) {
	case game.AttackCommand:
		payload["cardCode"] = EncodeCard(actual.Card)
		return ActionView{Type: "PLAY_CARD", Payload: payload}
	case game.DefendCommand:
		payload["cardCode"] = EncodeCard(actual.Card)
		payload["targetCardCode"] = EncodeCard(actual.Target)
		return ActionView{Type: "PLAY_CARD", Payload: payload}
	case game.TransferCommand:
		payload["cardCode"] = EncodeCard(actual.Card)
		return ActionView{Type: "TRANSFER", Payload: payload}
	case game.HangCardCommand:
		payload["cardCode"] = EncodeCard(actual.Card)
		return ActionView{Type: "HANG_CARD", Payload: payload}
	case game.ChooseTrumpCommand:
		payload["suit"] = SuitName(actual.Suit)
		return ActionView{Type: "CHOOSE_TRUMP", Payload: payload}
	case game.RevealFaceDownToDefendCommand:
		payload["targetCardCode"] = EncodeCard(actual.Target)
		return ActionView{Type: "REVEAL_FACE_DOWN", Payload: payload}
	case game.RevealFaceDownCommand:
		return ActionView{Type: "REVEAL_FACE_DOWN", Payload: payload}
	case game.PassCommand:
		return ActionView{Type: "PASS", Payload: payload}
	case game.TakeCommand:
		return ActionView{Type: "TAKE", Payload: payload}
	case game.HangSkipCommand:
		return ActionView{Type: "HANG_SKIP", Payload: payload}
	default:
		return ActionView{Type: "UNKNOWN", Payload: payload}
	}
}
