package game

import "testing"

// Перенос AttackLegalityTest, DefenceLegalityTest и TransferLegalityTest.
//
// Это сердце правил: ошибка здесь не роняет сервер, а тихо разрешает недопустимый ход,
// и партия расходится с тем, во что играют за столом.

func rules() MoveRules { return NewMoveRules(DefaultRulesConfig()) }

func expectAllowed(t *testing.T, verdict MoveVerdict, what string) {
	t.Helper()
	if !verdict.IsAllowed() {
		t.Errorf("%s: отклонено с причиной %s, а должно быть разрешено", what, verdict.Reason())
	}
}

func expectRejected(t *testing.T, verdict MoveVerdict, reason RejectionReason, what string) {
	t.Helper()
	if verdict.IsAllowed() {
		t.Errorf("%s: разрешено, а должно быть отклонено (%s)", what, reason)
		return
	}
	if verdict.Reason() != reason {
		t.Errorf("%s: причина %s, ждали %s", what, verdict.Reason(), reason)
	}
}

// ── Атака ────────────────────────────────────────────────────────────────────

func TestFirstAttackAllowsAnyCard(t *testing.T) {
	six := NewPip(Six, Clubs)
	// Защите нужна хотя бы одна карта: пустая рука упирается в её же бюджет,
	// и проверка «любая карта первой» не про то.
	state := aDeal().withHand(0, six).withHand(1, NewPip(Ace, Clubs)).build()

	expectAllowed(t, rules().CanAttack(state, 0, six), "первая карта раунда")
}

func TestAttackWithoutRightIsRejected(t *testing.T) {
	six := NewPip(Six, Clubs)
	state := aDeal().withHand(2, six).withAttackRight(0).build()

	expectRejected(t, rules().CanAttack(state, 2, six), NotYourTurn, "чужое право хода")
}

// ⚠️ Спасовавший теряет право до конца раунда: назад оно не возвращается.
func TestPassedPlayerCannotAttack(t *testing.T) {
	six := NewPip(Six, Clubs)
	state := aDeal().withHand(0, six).withAttackRight(0).withPassed(0).build()

	expectRejected(t, rules().CanAttack(state, 0, six), NotYourTurn, "спасовавший подкидывает")
}

func TestAttackWithCardNotInHandIsRejected(t *testing.T) {
	state := aDeal().withHand(0, NewPip(Six, Clubs)).build()

	expectRejected(t, rules().CanAttack(state, 0, NewPip(Ace, Spades)),
		CardNotInHand, "карта не на руках")
}

func TestFollowUpMatchesRankOnTable(t *testing.T) {
	sevenClubs := NewPip(Seven, Clubs)
	state := aDeal().
		withHand(0, sevenClubs).
		withHand(1, NewPip(Ace, Hearts), NewPip(King, Hearts)).
		withAttack(NewPip(Seven, Diamonds)).
		build()

	expectAllowed(t, rules().CanAttack(state, 0, sevenClubs), "подкид того же ранга")
}

// ⭐ Ранг ищется и среди КАРТ ЗАЩИТЫ: забыть их — значит запретить законный подкид.
func TestFollowUpMatchesRankOnDefence(t *testing.T) {
	nineClubs := NewPip(Nine, Clubs)
	state := aDeal().
		withHand(0, nineClubs).
		withHand(1, NewPip(Ace, Hearts), NewPip(King, Hearts)).
		withBeaten(NewPip(Seven, Diamonds), NewPip(Nine, Diamonds)).
		build()

	expectAllowed(t, rules().CanAttack(state, 0, nineClubs), "подкид под ранг карты защиты")
}

func TestFollowUpOfAbsentRankIsRejected(t *testing.T) {
	king := NewPip(King, Clubs)
	state := aDeal().
		withHand(0, king).
		withHand(1, NewPip(Ace, Hearts), NewPip(Queen, Hearts)).
		withAttack(NewPip(Seven, Diamonds)).
		build()

	expectRejected(t, rules().CanAttack(state, 0, king), RankNotOnTable, "чужой ранг")
}

// Потолок атаки зависит от того, уходили ли карты в отбой в этой раздаче.
func TestAttackLimitBeforeAndAfterDiscard(t *testing.T) {
	extra := NewPip(Six, Clubs)
	defenderHand := []Card{
		NewPip(Ace, Hearts), NewPip(King, Hearts), NewPip(Queen, Hearts),
		NewPip(Jack, Hearts), NewPip(Ten, Hearts), NewPip(Nine, Hearts),
		NewPip(Eight, Hearts),
	}
	onTable := []Card{
		NewPip(Six, Diamonds), NewPip(Six, Hearts), NewPip(Six, Spades),
		NewPip(Seven, Diamonds), NewPip(Seven, Hearts),
	}

	// До первого отбоя потолок пять — пятая карта уже лежит, шестая лишняя.
	before := aDeal().withHand(0, extra).withHand(1, defenderHand...).
		withAttack(onTable...).build()
	expectRejected(t, rules().CanAttack(before, 0, extra), AttackLimitReached,
		"шестая карта до отбоя")

	// После отбоя потолок шесть — та же шестая карта проходит.
	after := aDeal().withHand(0, extra).withHand(1, defenderHand...).
		withAttack(onTable...).withPileDiscarded().build()
	expectAllowed(t, rules().CanAttack(after, 0, extra), "шестая карта после отбоя")
}

// ⭐ Второй потолок — рука защищающегося: нельзя выложить больше, чем он способен отбить.
func TestAttackCappedByDefenderHand(t *testing.T) {
	extra := NewPip(Six, Clubs)
	state := aDeal().
		withHand(0, extra).
		withHand(1, NewPip(Ace, Hearts)). // всего одна карта
		withAttack(NewPip(Six, Diamonds)).
		build()

	expectRejected(t, rules().CanAttack(state, 0, extra), DefenderHasTooFewCards,
		"защите нечем отбить вторую карту")
}

// Отбитые карты из бюджета защиты выбывают: считаются только неотбитые.
func TestBeatenCardsLeaveTheDefenceBudget(t *testing.T) {
	extra := NewPip(Six, Clubs)
	state := aDeal().
		withHand(0, extra).
		withHand(1, NewPip(Ace, Hearts)).
		withBeaten(NewPip(Six, Diamonds), NewPip(Seven, Diamonds)).
		build()

	expectAllowed(t, rules().CanAttack(state, 0, extra), "отбитая карта не занимает бюджет")
}

// ⚠️ Скрытая карта входит в бюджет защиты, ТОЛЬКО когда колода пуста: пока колода есть,
// атака не вправе вынудить её вскрыть.
func TestFaceDownCountsOnlyWhenDeckIsEmpty(t *testing.T) {
	extra := NewPip(Six, Clubs)

	withDeck := aDeal().
		withHand(0, extra).
		withHand(1, NewPip(Ace, Hearts)).
		withFaceDown(1, NewPip(King, Spades)).
		withAttack(NewPip(Six, Diamonds)).
		build()
	expectRejected(t, rules().CanAttack(withDeck, 0, extra), DefenderHasTooFewCards,
		"скрытая карта при непустой колоде")

	deckEmpty := aDeal().
		withHand(0, extra).
		withHand(1, NewPip(Ace, Hearts)).
		withFaceDown(1, NewPip(King, Spades)).
		withAttack(NewPip(Six, Diamonds)).
		withEmptyDeck().
		build()
	expectAllowed(t, rules().CanAttack(deckEmpty, 0, extra), "скрытая карта при пустой колоде")
}

// ⭐ Джокер совпадает только с джокером: под него нельзя подкинуть обычную карту.
func TestJokerOnTableAcceptsOnlyJoker(t *testing.T) {
	plain := NewPip(Seven, Clubs)
	second := MustJoker(2)
	base := func(hand Card) DealState {
		return aDeal().
			withHand(0, hand).
			withHand(1, NewPip(Ace, Hearts), NewPip(King, Hearts)).
			withAttack(MustJoker(1)).
			build()
	}

	expectRejected(t, rules().CanAttack(base(plain), 0, plain), RankNotOnTable,
		"обычная карта под джокер")
	expectAllowed(t, rules().CanAttack(base(second), 0, second), "джокер под джокер")
}

func TestJokerCanStartTheRound(t *testing.T) {
	joker := MustJoker(1)
	state := aDeal().withHand(0, joker).withHand(1, NewPip(Ace, Clubs)).build()

	expectAllowed(t, rules().CanAttack(state, 0, joker), "джокер первой картой")
}

// ── Защита ───────────────────────────────────────────────────────────────────

func TestDefenceBeatsNamedTarget(t *testing.T) {
	target := NewPip(Seven, Diamonds)
	card := NewPip(Nine, Diamonds)
	state := aDeal().withHand(1, card).withAttack(target).build()

	expectAllowed(t, rules().CanDefend(state, 1, card, target), "старшая той же масти")
}

func TestOnlyDefenderMayDefend(t *testing.T) {
	target := NewPip(Seven, Diamonds)
	card := NewPip(Nine, Diamonds)
	state := aDeal().withHand(2, card).withAttack(target).withDefender(1).build()

	expectRejected(t, rules().CanDefend(state, 2, card, target), NotYourTurn, "отбивается чужой")
}

func TestDefenceTargetMustBeOnTable(t *testing.T) {
	card := NewPip(Nine, Diamonds)
	state := aDeal().withHand(1, card).withAttack(NewPip(Seven, Diamonds)).build()

	expectRejected(t, rules().CanDefend(state, 1, card, NewPip(King, Clubs)),
		TargetNotOnTable, "цели нет на столе")
}

func TestDefenceAgainstBeatenTargetIsRejected(t *testing.T) {
	target := NewPip(Seven, Diamonds)
	card := NewPip(Ten, Diamonds)
	state := aDeal().withHand(1, card).withBeaten(target, NewPip(Nine, Diamonds)).build()

	expectRejected(t, rules().CanDefend(state, 1, card, target),
		TargetAlreadyBeaten, "цель уже покрыта")
}

func TestWeakDefenceIsRejected(t *testing.T) {
	target := NewPip(Nine, Diamonds)
	card := NewPip(Seven, Diamonds)
	state := aDeal().withHand(1, card).withAttack(target).build()

	expectRejected(t, rules().CanDefend(state, 1, card, target), CardDoesNotBeat, "младшая карта")
}

// ⭐ Защищённую масть козырь не берёт — главное отличие бардака от дурака.
func TestTrumpCannotTakeProtectedSuit(t *testing.T) {
	spade := NewPip(Six, Spades)
	trumpCard := NewPip(Ace, Hearts)
	state := aDeal().withTrump(Hearts).withHand(1, trumpCard).withAttack(spade).build()

	expectRejected(t, rules().CanDefend(state, 1, trumpCard, spade),
		CardDoesNotBeat, "козырь против защищённой масти")

	higher := NewPip(Seven, Spades)
	ok := aDeal().withTrump(Hearts).withHand(1, higher).withAttack(spade).build()
	expectAllowed(t, rules().CanDefend(ok, 1, higher, spade), "старшая пика против пики")

	joker := MustJoker(1)
	byJoker := aDeal().withTrump(Hearts).withHand(1, joker).withAttack(spade).build()
	expectAllowed(t, rules().CanDefend(byJoker, 1, joker, spade), "джокер против защищённой масти")
}

func TestJokerAttackIsBeatenOnlyByJoker(t *testing.T) {
	attack := MustJoker(1)
	trumpAce := NewPip(Ace, Hearts)
	state := aDeal().withTrump(Hearts).withHand(1, trumpAce).withAttack(attack).build()

	expectRejected(t, rules().CanDefend(state, 1, trumpAce, attack),
		CardDoesNotBeat, "козырный туз против джокера")

	second := MustJoker(2)
	byJoker := aDeal().withHand(1, second).withAttack(attack).build()
	expectAllowed(t, rules().CanDefend(byJoker, 1, second, attack), "джокер кроет джокера")
}

// После «беру» защищающийся больше не отбивается.
func TestDefenceAfterTakingIsRejected(t *testing.T) {
	target := NewPip(Seven, Diamonds)
	card := NewPip(Nine, Diamonds)
	state := aDeal().withPhase(PhaseTaking).withHand(1, card).withAttack(target).build()

	expectRejected(t, rules().CanDefend(state, 1, card, target),
		DefenderAlreadyTook, "отбой после «беру»")
}

// ── Скрытая карта ────────────────────────────────────────────────────────────

// ⚠️ Скрытую карту нельзя назвать по имени ДАЖЕ СВОЮ: её не видит никто, включая
// владельца. Иначе клиент перебором выяснил бы её по ответам движка.
func TestFaceDownCardCannotBeNamedDirectly(t *testing.T) {
	hidden := NewPip(King, Spades)
	state := aDeal().withHand(0).withFaceDown(0, hidden).withEmptyDeck().build()

	expectRejected(t, rules().CanAttack(state, 0, hidden), CardNotInHand,
		"скрытая карта названа напрямую")
}

func TestRevealFaceDownNeedsEmptyDeckAndEmptyHand(t *testing.T) {
	hidden := NewPip(King, Spades)

	ready := aDeal().withHand(0).withFaceDown(0, hidden).withEmptyDeck().build()
	expectAllowed(t, rules().CanRevealFaceDown(ready, 0), "колода пуста и рука пуста")

	handLeft := aDeal().withHand(0, NewPip(Six, Clubs)).withFaceDown(0, hidden).
		withEmptyDeck().build()
	expectRejected(t, rules().CanRevealFaceDown(handLeft, 0), FaceDownCardNotPlayable,
		"в руке ещё есть карты")

	deckLeft := aDeal().withHand(0).withFaceDown(0, hidden).build()
	expectRejected(t, rules().CanRevealFaceDown(deckLeft, 0), FaceDownCardNotPlayable,
		"колода ещё не пуста")

	none := aDeal().withHand(0).withEmptyDeck().build()
	expectRejected(t, rules().CanRevealFaceDown(none, 0), FaceDownCardNotPlayable,
		"скрытой карты нет вовсе")
}

// ── Перевод ──────────────────────────────────────────────────────────────────

func TestTransferOfSameRankIsAllowed(t *testing.T) {
	card := NewPip(Seven, Clubs)
	state := aDeal().withPlayers(3).
		withHand(1, card).
		withHand(2, NewPip(Ace, Hearts), NewPip(King, Hearts), NewPip(Queen, Hearts)).
		withAttack(NewPip(Seven, Diamonds)).
		build()

	expectAllowed(t, rules().CanTransfer(state, 1, card), "перевод одноранговой картой")
}

// ⭐ Перевод жив, только пока не отбита ни одна карта.
func TestTransferAfterFirstBeatIsRejected(t *testing.T) {
	card := NewPip(Seven, Clubs)
	state := aDeal().withPlayers(3).
		withHand(1, card).
		withHand(2, NewPip(Ace, Hearts), NewPip(King, Hearts), NewPip(Queen, Hearts)).
		withBeaten(NewPip(Seven, Diamonds), NewPip(Nine, Diamonds)).
		build()

	expectRejected(t, rules().CanTransfer(state, 1, card),
		TransferAfterFirstBeat, "перевод после отбоя")
}

func TestTransferOfAnotherRankIsRejected(t *testing.T) {
	card := NewPip(King, Clubs)
	state := aDeal().withPlayers(3).
		withHand(1, card).
		withHand(2, NewPip(Ace, Hearts), NewPip(Queen, Hearts), NewPip(Jack, Hearts)).
		withAttack(NewPip(Seven, Diamonds)).
		build()

	expectRejected(t, rules().CanTransfer(state, 1, card),
		TransferRankMismatch, "перевод другим рангом")
}

// ⚠️ Принимающему должно хватать карт отбить выросшую атаку, иначе перевод ставил бы
// его в заведомо безвыходное положение.
func TestTransferIsRejectedWhenReceiverCannotCope(t *testing.T) {
	card := NewPip(Seven, Clubs)
	state := aDeal().withPlayers(3).
		withHand(1, card).
		withHand(2, NewPip(Ace, Hearts)). // одна карта против двух
		withAttack(NewPip(Seven, Diamonds)).
		build()

	expectRejected(t, rules().CanTransfer(state, 1, card),
		NextPlayerHasTooFewCards, "принимающему нечем отбить")
}

// Вышедшие из раздачи пропускаются при выборе принимающего.
func TestTransferSkipsPlayersOutOfDeal(t *testing.T) {
	card := NewPip(Seven, Clubs)
	state := aDeal().withPlayers(4).
		withHand(1, card).
		withOutOfDeal(2).
		withHand(3, NewPip(Ace, Hearts), NewPip(King, Hearts), NewPip(Queen, Hearts)).
		withAttack(NewPip(Seven, Diamonds)).
		build()

	expectAllowed(t, rules().CanTransfer(state, 1, card), "принимающий — следующий активный")
}

// Перевод можно выключить настройкой стола.
func TestTransfersCanBeDisabled(t *testing.T) {
	card := NewPip(Seven, Clubs)
	config := DefaultRulesConfig()
	config.TransfersEnabled = false
	state := aDeal().withPlayers(3).
		withHand(1, card).
		withHand(2, NewPip(Ace, Hearts), NewPip(King, Hearts), NewPip(Queen, Hearts)).
		withAttack(NewPip(Seven, Diamonds)).
		build()

	expectRejected(t, NewMoveRules(config).CanTransfer(state, 1, card),
		TransfersDisabled, "перевод выключен настройкой")
}
