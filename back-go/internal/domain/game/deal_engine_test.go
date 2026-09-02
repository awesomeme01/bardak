package game

import "testing"

// Перенос DealEngineTest и DealRefillAndExitTest — проверки самого автомата раздачи.
//
// ⭐ Здесь живёт починка тупика (ADR-051): состояние, из которого ни один игрок не мог
// сходить, а раздача не была окончена. Тесты в конце файла держат именно её.

func engine() DealEngine { return NewDefaultDealEngine() }

func mustApply(t *testing.T, state DealState, cmd DealCommand) MoveResult {
	t.Helper()
	result := engine().Apply(state, cmd)
	if !result.Applied {
		t.Fatalf("команда отклонена с причиной %s, а должна была примениться", result.Reason)
	}
	return result
}

func mustReject(t *testing.T, state DealState, cmd DealCommand, reason RejectionReason) MoveResult {
	t.Helper()
	result := engine().Apply(state, cmd)
	if result.Applied {
		t.Fatalf("команда применена, а должна была быть отклонена (%s)", reason)
	}
	if result.Reason != reason {
		t.Fatalf("причина отказа %s, ждали %s", result.Reason, reason)
	}
	return result
}

func TestAttackMovesCardFromHandToTable(t *testing.T) {
	six := NewPip(Six, Clubs)
	state := aDeal().withHand(0, six).withHand(1, NewPip(Ace, Clubs)).build()

	result := mustApply(t, state, AttackCommand{Seat: 0, Card: six})

	if len(result.State.Table) != 1 || result.State.Table[0].Attack != six {
		t.Error("карта не легла на стол")
	}
	if result.State.MustPlayerAt(0).HoldsInHand(six) {
		t.Error("карта осталась в руке — она должна была уйти на стол")
	}
	if result.State.Phase != PhaseDefend {
		t.Errorf("фаза %s, ждали DEFEND", result.State.Phase)
	}
}

// ⚠️ Отклонённая команда не меняет состояние ВООБЩЕ. Иначе отказ оставлял бы следы,
// и партия расходилась бы с тем, что видели игроки.
func TestRejectedCommandLeavesStateUntouched(t *testing.T) {
	six := NewPip(Six, Clubs)
	state := aDeal().withHand(0, six).withHand(1, NewPip(Ace, Clubs)).build()

	result := engine().Apply(state, AttackCommand{Seat: 2, Card: six})

	if result.Applied {
		t.Fatal("ход не с того места должен отклоняться")
	}
	if len(result.State.Table) != 0 || len(result.State.Players) != 0 {
		t.Error("отклонённая команда вернула состояние — она не должна возвращать ничего")
	}
	if len(state.Table) != 0 {
		t.Error("исходное состояние изменилось")
	}
}

func TestDefenceMarksNamedSlotBeaten(t *testing.T) {
	target := NewPip(Seven, Diamonds)
	card := NewPip(Nine, Diamonds)
	// ⚠️ Подкидывающему нужна карта на руках, иначе раунд закрывается прямо здесь:
	// право подкидывать к пустой руке больше не переходит, и стол уезжает в отбой
	// до того, как тест успеет посмотреть на слот.
	state := aDeal().withHand(0, NewPip(King, Clubs)).
		withHand(1, card).withAttack(target).withPhase(PhaseDefend).build()

	result := mustApply(t, state, DefendCommand{Seat: 1, Card: card, Target: target})

	if !result.State.Table[0].IsBeaten() {
		t.Error("слот не отмечен отбитым")
	}
	if result.State.Table[0].Defence != card {
		t.Error("отбито не той картой, что назвали")
	}
}

func TestStaysInDefenceWhileUnbeatenRemain(t *testing.T) {
	first := NewPip(Seven, Diamonds)
	second := NewPip(Seven, Clubs)
	card := NewPip(Nine, Diamonds)
	state := aDeal().withHand(1, card, NewPip(Ace, Hearts)).
		withAttack(first, second).withPhase(PhaseDefend).build()

	result := mustApply(t, state, DefendCommand{Seat: 1, Card: card, Target: first})

	if result.State.Phase != PhaseDefend {
		t.Errorf("фаза %s, а на столе осталась неотбитая карта", result.State.Phase)
	}
}

// Перевод делает переводящего атакующим, а защиту сдвигает по кругу.
func TestTransferMakesTransferrerTheAttacker(t *testing.T) {
	card := NewPip(Seven, Clubs)
	state := aDeal().withPlayers(3).
		withHand(1, card).
		withHand(2, NewPip(Ace, Hearts), NewPip(King, Hearts), NewPip(Queen, Hearts)).
		withAttack(NewPip(Seven, Diamonds)).withPhase(PhaseDefend).build()

	result := mustApply(t, state, TransferCommand{Seat: 1, Card: card})

	if result.State.AttackRightSeat != 1 || result.State.RoundStarterSeat != 1 {
		t.Error("переводящий обязан стать атакующим")
	}
	if result.State.DefenderSeat != 2 {
		t.Errorf("защита ушла на место %d, ждали 2", result.State.DefenderSeat)
	}
	if len(result.State.PassedSeats) != 0 {
		t.Error("перевод открывает раунд заново — пасы обнуляются")
	}
}

// ⭐ Право подкидывать уходит ВТОРОМУ СОСЕДУ, то есть следующему за защищающимся.
func TestPassHandsRightToSecondNeighbour(t *testing.T) {
	state := aDeal().withPlayers(3).
		withHand(0, NewPip(Six, Clubs)).
		withHand(2, NewPip(Six, Hearts)).
		withAttack(NewPip(Seven, Diamonds)).withPhase(PhaseDefend).build()

	result := mustApply(t, state, PassCommand{Seat: 0})

	if result.State.AttackRightSeat != 2 {
		t.Errorf("право у места %d, ждали второго соседа — место 2", result.State.AttackRightSeat)
	}
}

// ⚠️ Право назад НЕ возвращается: спасовавший в этом раунде больше не подкидывает.
func TestPassedPlayerNeverGetsTheRightBack(t *testing.T) {
	six := NewPip(Six, Clubs)
	state := aDeal().withPlayers(3).
		withHand(0, six).
		withAttack(NewPip(Seven, Diamonds)).
		withPassed(0).withAttackRight(0).withPhase(PhaseDefend).build()

	mustReject(t, state, AttackCommand{Seat: 0, Card: six}, NotYourTurn)
}

// «Беру» раунд не закрывает: подкидывающие докидывают, пока не спасуют.
func TestTakeKeepsRoundAliveForFollowUps(t *testing.T) {
	state := aDeal().withPlayers(3).
		withHand(0, NewPip(Seven, Clubs)).
		withHand(1, NewPip(Ace, Hearts)).
		withAttack(NewPip(Seven, Diamonds)).withPhase(PhaseDefend).build()

	result := mustApply(t, state, TakeCommand{Seat: 1})

	if result.State.Phase != PhaseTaking {
		t.Errorf("фаза %s, ждали TAKING", result.State.Phase)
	}
	if len(result.State.Table) == 0 {
		t.Error("стол уехал сразу — а подкидывающие ещё не спасовали")
	}
}

// ⭐ Рука защищающегося ограничивает подкид И ПОСЛЕ «беру» (§1.4.2, решение владельца
// 2026-08-31). Раньше было наоборот — «он всё равно заберёт всё», — и живая партия
// показала, во что это превращается: у игрока одна карта, а ему кладут четыре.
func TestTakersHandSizeStillCapsFollowUps(t *testing.T) {
	extra := NewPip(Seven, Clubs)
	oneCard := aDeal().withPlayers(3).
		withHand(0, extra).
		withHand(1, NewPip(Ace, Hearts)). // одна карта — одну и отобьёт
		withAttack(NewPip(Seven, Diamonds)).
		withPhase(PhaseTaking).withAttackRight(0).build()

	mustReject(t, oneCard, AttackCommand{Seat: 0, Card: extra}, DefenderHasTooFewCards)

	// Две карты в руке — второй подкид проходит: запас есть.
	twoCards := aDeal().withPlayers(3).
		withHand(0, extra).
		withHand(1, NewPip(Ace, Hearts), NewPip(King, Spades)).
		withAttack(NewPip(Seven, Diamonds)).
		withPhase(PhaseTaking).withAttackRight(0).build()

	mustApply(t, twoCards, AttackCommand{Seat: 0, Card: extra})
}

// ...но потолок раунда остаётся.
func TestFollowUpsStillCappedByRoundLimit(t *testing.T) {
	extra := NewPip(Six, Clubs)
	onTable := []Card{
		NewPip(Six, Diamonds), NewPip(Six, Hearts), NewPip(Six, Spades),
		NewPip(Seven, Diamonds), NewPip(Seven, Hearts),
	}
	state := aDeal().withPlayers(3).
		withHand(0, extra).
		withHand(1, NewPip(Ace, Hearts)).
		withAttack(onTable...).
		withPhase(PhaseTaking).withAttackRight(0).build()

	mustReject(t, state, AttackCommand{Seat: 0, Card: extra}, AttackLimitReached)
}

func TestDefenceAndTransferRefusedAfterTake(t *testing.T) {
	target := NewPip(Seven, Diamonds)
	card := NewPip(Nine, Diamonds)
	state := aDeal().withPlayers(3).withHand(1, card).
		withAttack(target).withPhase(PhaseTaking).build()

	mustReject(t, state, DefendCommand{Seat: 1, Card: card, Target: target}, DefenderAlreadyTook)
	mustReject(t, state, TransferCommand{Seat: 1, Card: NewPip(Seven, Clubs)}, DefenderAlreadyTook)
}

func TestNothingToTakeOnEmptyTable(t *testing.T) {
	state := aDeal().withPhase(PhaseAttack).build()

	mustReject(t, state, TakeCommand{Seat: 1}, NothingToTake)
}

func TestEveryCommandRefusedWhenDealIsOver(t *testing.T) {
	six := NewPip(Six, Clubs)
	state := aDeal().withHand(0, six).withPhase(PhaseDealOver).build()

	mustReject(t, state, AttackCommand{Seat: 0, Card: six}, NotYourTurn)
	mustReject(t, state, PassCommand{Seat: 0}, NotYourTurn)
	mustReject(t, state, TakeCommand{Seat: 1}, NotYourTurn)
}

// ── Тупик, ради которого всё и чинилось (ADR-051) ────────────────────────────

// ⚠️ Отбита последняя карта, а право подкидывать осталось за СПАСОВАВШИМ. Он не может
// ни подкинуть, ни спасовать снова — и раздача вставала намертво У ВСЕХ за столом.
//
// Проверяем не «у кого-то есть ход», а именно то, что ход есть у обладателя права:
// первая версия этого теста проходила на сломанном коде, потому что у защиты
// оставалось бессмысленное «беру».
func TestNoDeadlockAfterLastCardBeaten(t *testing.T) {
	attack := NewPip(Seven, Diamonds)
	defence := NewPip(Nine, Diamonds)

	state := aDeal().withPlayers(3).
		withHand(0, NewPip(King, Clubs)).
		withHand(1, defence).
		withHand(2, NewPip(Queen, Clubs)).
		withAttack(attack).
		withPassed(0).withAttackRight(0).
		withPhase(PhaseDefend).
		build()

	result := mustApply(t, state, DefendCommand{Seat: 1, Card: defence, Target: attack})
	next := result.State

	if next.Phase == PhaseDealOver {
		return // раздача закрылась — тупика нет по определению
	}

	// ⭐ Главное утверждение: у того, за кем право, есть хотя бы один законный ход.
	holder := next.AttackRightSeat
	if next.HasPassed(holder) {
		t.Fatalf("право осталось за спасовавшим местом %d — это и есть тупик", holder)
	}

	rules := NewMoveRules(DefaultRulesConfig())
	hasMove := false
	for _, card := range next.MustPlayerAt(holder).Hand {
		if rules.CanAttack(next, holder, card).IsAllowed() {
			hasMove = true
			break
		}
	}
	if !hasMove && engine().Apply(next, PassCommand{Seat: holder}).Applied {
		hasMove = true
	}
	if !hasMove {
		t.Fatalf("у обладателя права (место %d) нет ни одного законного хода — стол встал", holder)
	}
}

// Раунд закрывается, когда отбита последняя карта и подкидывать больше некому.
func TestRoundClosesWhenNobodyCanFollowUp(t *testing.T) {
	attack := NewPip(Seven, Diamonds)
	defence := NewPip(Nine, Diamonds)

	state := aDeal().withPlayers(2).
		withHand(0, NewPip(King, Clubs)).
		withHand(1, defence).
		withAttack(attack).
		withPassed(0).withAttackRight(0).withDefender(1).
		withPhase(PhaseDefend).
		build()

	result := mustApply(t, state, DefendCommand{Seat: 1, Card: defence, Target: attack})

	if len(result.State.Table) != 0 {
		t.Error("стол не уехал в отбой, хотя подкидывать некому")
	}
	if !hasEvent(result.Events, func(e DealEvent) bool { _, ok := e.(RoundBeaten); return ok }) {
		t.Error("нет события «бито»")
	}
}

// ── Добор и выход ────────────────────────────────────────────────────────────

// ⚠️ Порядок добора не формальность: колода конечна, и начавший раунд берёт первым.
func TestScarceCardsGoToRoundStarterFirst(t *testing.T) {
	attack := NewPip(Seven, Diamonds)
	defence := NewPip(Nine, Diamonds)

	state := aDeal().withPlayers(2).
		withHand(0, NewPip(King, Clubs)).
		withHand(1, defence).
		withAttack(attack).
		withDeck(NewPip(Ace, Clubs)). // ровно одна карта на двоих
		withPassed(0).withAttackRight(0).withDefender(1).
		withRoundStarter(0).
		withPhase(PhaseDefend).
		build()

	result := mustApply(t, state, DefendCommand{Seat: 1, Card: defence, Target: attack})

	// Место 0 начало раунд — единственная карта его.
	if result.State.MustPlayerAt(0).HandSize() != 2 {
		t.Errorf("у начавшего раунд %d карт, он должен был добрать первым",
			result.State.MustPlayerAt(0).HandSize())
	}
}

func hasEvent(events []DealEvent, match func(DealEvent) bool) bool {
	for _, event := range events {
		if match(event) {
			return true
		}
	}
	return false
}

// Пас при пустом столе — не ход, а отказ начать раунд.
//
// ⚠️ Живая партия: у атакующего пустой стол, подсказка «твоя атака», и рядом кнопка
// «Пас». Нажатие закрывало раунд, не сыграв ни одной карты, — предложение, которое
// ничего не значит и только путает. Пас означает «больше не подкидываю», а подкидывать
// в этот момент ещё нечего.
//
// ⭐ С пустой рукой пас, наоборот, единственный выход: вышедший из раздачи закрывает
// им раунд, и без этого раздача не заканчивалась бы вовсе.
func TestPassIsRefusedOnAnEmptyTableWhileThereIsSomethingToPlay(t *testing.T) {
	engine := NewDealEngineFor(DefaultRulesConfig())

	withCards := aDeal().withPlayers(2).withPhase(PhaseAttack).
		withHand(0, NewPip(Ace, Clubs)).withHand(1, NewPip(King, Hearts)).
		withAttackRight(0).withDefender(1).build()

	if result := engine.Apply(withCards.Clone(), PassCommand{Seat: 0}); result.Applied {
		t.Error("пас при пустом столе принят, хотя игроку есть чем атаковать")
	} else if result.Reason != NothingToPass {
		t.Errorf("причина отказа %s, ждали NOTHING_TO_PASS", result.Reason)
	}

	// Стол не пуст — пас законен: это и есть «больше не подкидываю».
	withAttack := aDeal().withPlayers(2).withPhase(PhaseAttack).
		withHand(0, NewPip(Ace, Clubs)).withHand(1, NewPip(King, Hearts)).
		withAttack(NewPip(Six, Spades)).
		withAttackRight(0).withDefender(1).build()

	if result := engine.Apply(withAttack.Clone(), PassCommand{Seat: 0}); !result.Applied {
		t.Errorf("пас при непустом столе отклонён: %s", result.Reason)
	}
}

// Отдавший последнюю карту пасует сам: раунду незачем ждать подтверждения от того,
// кому подкидывать нечем (ADR-067).
//
// ⚠️ Разведка перед правкой показала: у такого игрока PASS оставался ЕДИНСТВЕННЫМ
// доступным действием. Спрятать кнопку на клиенте значило повесить стол.
func TestLastCardPassesByItself(t *testing.T) {
	engine := NewDealEngineFor(DefaultRulesConfig())
	last := NewPip(Six, Clubs)
	state := aDeal().withPlayers(2).withTrump(Hearts).
		withHand(0, last).
		withHand(1, NewPip(Ten, Hearts)).
		withDeck(NewPip(Ace, Spades)).
		withDefender(1).withAttackRight(0).build()

	move := engine.Apply(state, AttackCommand{Seat: 0, Card: last})

	if !move.Applied {
		t.Fatalf("атака последней картой отклонена: %s", move.Reason)
	}
	passed := hasEvent(move.Events, func(e DealEvent) bool {
		_, is := e.(Passed)
		return is
	})
	if !passed {
		t.Fatal("пас за опустевшего игрока не проведён: раунд будет ждать его кнопки")
	}
	// Атака на столе осталась — пас не закрыл раунд, защищающийся ещё отбивается.
	if move.State.Phase != PhaseDefend {
		t.Errorf("фаза после авто-паса = %s, ждали защиту", move.State.Phase)
	}
	if move.State.UnbeatenCount() != 1 {
		t.Errorf("неотбитых на столе %d, ждали 1", move.State.UnbeatenCount())
	}
}

// Право подкинуть идёт мимо игрока с пустой рукой: объявлять ему нечего (ADR-067).
func TestEmptyHandGetsNoAttackRight(t *testing.T) {
	attack := NewPip(Six, Clubs)
	state := aDeal().withPlayers(3).withTrump(Hearts).
		withHand(0, NewPip(Six, Spades)).
		withHand(1, NewPip(Ten, Hearts)).
		// Место 2 без карт: колода ещё есть, потайной нет — подкинуть нечем.
		withHand(2).
		withDeck(NewPip(Ace, Spades)).
		withAttack(attack).
		withDefender(1).withAttackRight(0).withPhase(PhaseDefend).build()

	if isEligibleAttacker(state, 2) {
		t.Fatal("пустая рука получила право подкинуть — игроку покажут «Пас» без выбора")
	}
	if !isEligibleAttacker(state, 0) {
		t.Error("место 0 с картой права лишилось")
	}
}
