package game

// MoveRules — легальность хода: чистые проверки над снимком раздачи, без состояния
// и без побочных эффектов.
//
// ⭐ Проверки намеренно отделены от переходов: сервер обязан валидировать каждый ход,
// а отклонённый ход не меняет состояние вообще.
type MoveRules struct {
	Config RulesConfig
}

// NewMoveRules собирает проверяльщик для правил стола.
func NewMoveRules(config RulesConfig) MoveRules { return MoveRules{Config: config} }

// CanAttack — можно ли положить карту в атаку: первой картой раунда или подкидом.
//
// ⭐ После объявленного «беру» (фаза TAKING) подкидывание продолжается, но потолком
// остаётся только лимит раунда: рука взявшего больше ничего не ограничивает — он всё
// равно забирает стол.
func (r MoveRules) CanAttack(state DealState, seatNo int, card Card) MoveVerdict {
	if state.AttackRightSeat != seatNo || state.HasPassed(seatNo) {
		return Rejected(NotYourTurn)
	}
	if holding := r.canPlayFromHand(state, seatNo, card); !holding.IsAllowed() {
		return holding
	}
	if state.AttackCardCount() >= r.Config.AttackLimit(state.AnyPileDiscarded) {
		return Rejected(AttackLimitReached)
	}
	if len(state.Table) > 0 && !state.HasRankOnTable(card) {
		return Rejected(RankNotOnTable)
	}
	if state.Phase == PhaseTaking {
		return Allowed()
	}
	if state.UnbeatenCount()+1 > state.Defender().DefendableCards(state.IsDeckEmpty()) {
		return Rejected(DefenderHasTooFewCards)
	}
	return Allowed()
}

// CanDefend — можно ли отбить конкретную атакующую карту.
//
// ⚠️ Цель обязательна: при нескольких картах на столе иначе не зафиксировать,
// что чем отбито.
func (r MoveRules) CanDefend(state DealState, seatNo int, card, target Card) MoveVerdict {
	if state.DefenderSeat != seatNo {
		return Rejected(NotYourTurn)
	}
	if state.Phase == PhaseTaking {
		return Rejected(DefenderAlreadyTook)
	}
	if holding := r.canPlayFromHand(state, seatNo, card); !holding.IsAllowed() {
		return holding
	}

	slot, found := findSlot(state.Table, target)
	if !found {
		return Rejected(TargetNotOnTable)
	}
	if slot.IsBeaten() {
		return Rejected(TargetAlreadyBeaten)
	}
	if state.Trump == nil {
		return Rejected(TrumpNotChosenYet)
	}
	if !state.Trump.Beats(card, target) {
		return Rejected(CardDoesNotBeat)
	}
	return Allowed()
}

// CanTransfer — можно ли перевести атаку дальше по кругу.
//
// ⭐ Перевод жив, только пока не отбита ни одна карта, — и потому вся переводимая атака
// всегда одноранговая. Достаточно сверить с первой картой стола.
func (r MoveRules) CanTransfer(state DealState, seatNo int, card Card) MoveVerdict {
	if !r.Config.TransfersEnabled {
		return Rejected(TransfersDisabled)
	}
	if state.DefenderSeat != seatNo {
		return Rejected(NotYourTurn)
	}
	if state.Phase == PhaseTaking {
		return Rejected(DefenderAlreadyTook)
	}
	if holding := r.canPlayFromHand(state, seatNo, card); !holding.IsAllowed() {
		return holding
	}
	if len(state.Table) == 0 {
		return Rejected(TransferRankMismatch)
	}
	if state.AnyCardBeatenThisRound {
		return Rejected(TransferAfterFirstBeat)
	}
	if !state.Table[0].Attack.SameRankAs(card) {
		return Rejected(TransferRankMismatch)
	}
	return r.verdictOnReceiver(state, seatNo)
}

// verdictOnReceiver — принимающему должно хватать карт отбить выросшую атаку.
//
// ⚠️ Иначе перевод ставил бы его в заведомо безвыходное положение: он обязан отбить
// на одну карту больше, чем было, и не может.
func (r MoveRules) verdictOnReceiver(state DealState, seatNo int) MoveVerdict {
	nextSeat, err := state.NextActiveSeatAfter(seatNo)
	if err != nil {
		return Rejected(NotYourTurn)
	}
	receiver := state.MustPlayerAt(nextSeat)
	attackAfterTransfer := state.AttackCardCount() + 1
	if receiver.DefendableCards(state.IsDeckEmpty()) < attackAfterTransfer {
		return Rejected(NextPlayerHasTooFewCards)
	}
	return Allowed()
}

// canPlayFromHand — держит ли игрок эту карту.
//
// ⭐ Скрытую карту назвать по имени нельзя, ДАЖЕ СВОЮ: её не видит никто, включая
// владельца. Разрешить это значило бы дать клиенту перебором выяснить карту по ответам
// движка. Скрытая карта играется отдельной командой, которая её сначала вскрывает.
func (r MoveRules) canPlayFromHand(state DealState, seatNo int, card Card) MoveVerdict {
	player, err := state.PlayerAt(seatNo)
	if err != nil {
		return Rejected(NotYourTurn)
	}
	if player.HoldsInHand(card) {
		return Allowed()
	}
	return Rejected(CardNotInHand)
}

// CanRevealFaceDown — можно ли вообще вскрыть скрытую карту: колода пуста и обычных
// карт не осталось.
func (r MoveRules) CanRevealFaceDown(state DealState, seatNo int) MoveVerdict {
	player, err := state.PlayerAt(seatNo)
	if err != nil {
		return Rejected(FaceDownCardNotPlayable)
	}
	if !player.CanPlayFaceDown(state.IsDeckEmpty()) {
		return Rejected(FaceDownCardNotPlayable)
	}
	return Allowed()
}

// findSlot ищет слот по атакующей карте.
func findSlot(table []TableSlot, attack Card) (TableSlot, bool) {
	for _, slot := range table {
		if slot.Attack == attack {
			return slot, true
		}
	}
	return TableSlot{}, false
}
