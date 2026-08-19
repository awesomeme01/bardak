package game

import "testing"

// Персональная проекция состояния — туман войны (§1.8, §2.3, ADR-026).
//
// ⭐ Главная проверка здесь не про поля, а про то, чего в проекции быть НЕ МОЖЕТ:
// ни одной чужой карты и ни одной скрытой, включая собственную.

var (
	projSevenDiamonds = NewPip(Seven, Diamonds)
	projNineDiamonds  = NewPip(Nine, Diamonds)
	projAceClubs      = NewPip(Ace, Clubs)
	projKingClubs     = NewPip(King, Clubs)
	projSixSpades     = NewPip(Six, Spades)
	projQueenHearts   = NewPip(Queen, Hearts)
	projTenClubs      = NewPip(Ten, Clubs)
	projSixClubs      = NewPip(Six, Clubs)
)

// projRulesEngine — облегчённый двойник движка: применяет ровно те проверки, что уже
// перенесены в MoveRules.
//
// ⚠️ Это НЕ движок: полный автомат раздачи переносится отдельно. Тесты проекции им
// пользуются затем, чтобы проверить сам механизм — «в список попадает только то, что
// движок принял», — а не переписать здесь правила во второй раз.
type projRulesEngine struct{ rules MoveRules }

func projEngine() projRulesEngine { return projRulesEngine{rules: NewMoveRules(DefaultRulesConfig())} }

func (e projRulesEngine) Apply(state DealState, command DealCommand) MoveResult {
	verdict := e.verdict(state, command)
	if !verdict.IsAllowed() {
		return RejectedResult(verdict.Reason())
	}
	return AppliedResult(state, nil)
}

func (e projRulesEngine) verdict(state DealState, command DealCommand) MoveVerdict {
	switch cmd := command.(type) {
	case AttackCommand:
		return e.rules.CanAttack(state, cmd.Seat, cmd.Card)
	case DefendCommand:
		return e.rules.CanDefend(state, cmd.Seat, cmd.Card, cmd.Target)
	case TransferCommand:
		return e.rules.CanTransfer(state, cmd.Seat, cmd.Card)
	case RevealFaceDownCommand:
		return e.rules.CanRevealFaceDown(state, cmd.Seat)
	case RevealFaceDownToDefendCommand:
		return e.rules.CanRevealFaceDown(state, cmd.Seat)
	case TakeCommand:
		if state.DefenderSeat != cmd.Seat {
			return Rejected(NotYourTurn)
		}
		if len(state.Table) == 0 {
			return Rejected(NothingToTake)
		}
		return Allowed()
	case PassCommand:
		if state.AttackRightSeat != cmd.Seat || state.HasPassed(cmd.Seat) {
			return Rejected(NotYourTurn)
		}
		return Allowed()
	case HangCardCommand, HangSkipCommand:
		// Навешивать можно только в открытом окне; окна в этих тестах нет.
		if state.HangingWindow == nil {
			return Rejected(NotInHangingWindow)
		}
		return Allowed()
	case ChooseTrumpCommand:
		if state.Phase != PhaseDice {
			return Rejected(TrumpNotInDispute)
		}
		return Allowed()
	}
	return Rejected(NotYourTurn)
}

// projAcceptAllEngine — движок, принимающий вообще всё.
//
// ⭐ Самый злой случай для тумана войны: фильтр движка не отсекает ничего, и в списке
// действий остаётся ровно то, что проекция вообще СОБРАЛА. Если чужая карта попадает
// в кандидаты, здесь это видно.
type projAcceptAllEngine struct{}

func (projAcceptAllEngine) Apply(state DealState, _ DealCommand) MoveResult {
	return AppliedResult(state, nil)
}

func projProject(t *testing.T, projection StateProjection, state DealState, seat int) PlayerView {
	t.Helper()
	view, err := projection.Project(state, seat)
	if err != nil {
		t.Fatalf("проекция места %d не собралась: %v", seat, err)
	}
	return view
}

func projSeat(t *testing.T, view PlayerView, seatNo int) SeatView {
	t.Helper()
	seat, err := view.Seat(seatNo)
	if err != nil {
		t.Fatalf("в проекции нет места %d: %v", seatNo, err)
	}
	return seat
}

// projVisibleCards — все карты, которые ФИЗИЧЕСКИ присутствуют в проекции, включая
// спрятанные в списке доступных действий: команда с картой — такая же утечка, как карта
// в руке соседа.
func projVisibleCards(view PlayerView) []Card {
	cards := make([]Card, 0, 32)
	cards = append(cards, view.MyHand...)
	if view.TrumpCard != nil {
		cards = append(cards, view.TrumpCard)
	}
	for _, seat := range view.Seats {
		cards = append(cards, seat.HungCards...)
	}
	for _, slot := range view.Table {
		cards = append(cards, slot.Attack)
		if slot.Defence != nil {
			cards = append(cards, slot.Defence)
		}
	}
	for _, action := range view.AvailableActions {
		switch cmd := action.(type) {
		case AttackCommand:
			cards = append(cards, cmd.Card)
		case TransferCommand:
			cards = append(cards, cmd.Card)
		case HangCardCommand:
			cards = append(cards, cmd.Card)
		case DefendCommand:
			cards = append(cards, cmd.Card, cmd.Target)
		case RevealFaceDownToDefendCommand:
			cards = append(cards, cmd.Target)
		}
	}
	return cards
}

func projContainsCard(cards []Card, card Card) bool { return indexOfCard(cards, card) >= 0 }

func projContainsCommand(commands []DealCommand, wanted DealCommand) bool {
	for _, command := range commands {
		if command == wanted {
			return true
		}
	}
	return false
}

func TestProjectionShowsOwnHandInFull(t *testing.T) {
	state := aDeal().withHand(0, projSevenDiamonds, projAceClubs).withHand(1, projKingClubs).build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if view.MySeat != 0 {
		t.Fatalf("проекция собрана не для того места: %d", view.MySeat)
	}
	if len(view.MyHand) != 2 ||
		!projContainsCard(view.MyHand, projSevenDiamonds) ||
		!projContainsCard(view.MyHand, projAceClubs) {
		t.Fatalf("своя рука видна не целиком, играть будет нечем: %v", view.MyHand)
	}
}

func TestProjectionShowsOnlyCardCountOfOthers(t *testing.T) {
	state := aDeal().
		withHand(0, projSevenDiamonds).
		withHand(1, projKingClubs, projAceClubs, projNineDiamonds).
		build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if seat := projSeat(t, view, 1); seat.CardsCount != 3 {
		t.Fatalf("сосед держит 3 карты, проекция показывает %d", seat.CardsCount)
	}
	visible := projVisibleCards(view)
	for _, card := range []Card{projKingClubs, projAceClubs, projNineDiamonds} {
		if projContainsCard(visible, card) {
			t.Fatalf("карта соседа %s попала в проекцию — игра испорчена", card.Code())
		}
	}
}

// ⭐ Сводная проверка тумана войны: каждое место смотрит на одно и то же состояние,
// и ни к кому не приходит ни чужая рука, ни чья-либо скрытая карта.
func TestProjectionNeverLeaksForeignOrHiddenCards(t *testing.T) {
	state := aDeal().
		withHand(0, projSevenDiamonds, projAceClubs).
		withHand(1, projKingClubs).
		withHand(2, projNineDiamonds).
		withFaceDown(0, projSixSpades).
		withFaceDown(1, projQueenHearts).
		withAttack(projTenClubs).
		build()

	for _, engine := range []MoveApplier{projEngine(), projAcceptAllEngine{}} {
		projection := NewStateProjection(DefaultRulesConfig(), engine)
		for _, viewer := range state.Players {
			visible := projVisibleCards(projProject(t, projection, state, viewer.SeatNo))

			for _, other := range state.Players {
				if other.SeatNo != viewer.SeatNo {
					for _, card := range other.Hand {
						if projContainsCard(visible, card) {
							t.Fatalf("рука места %d (%s) попала в проекцию места %d",
								other.SeatNo, card.Code(), viewer.SeatNo)
						}
					}
				}
				if other.HasFaceDownCard() && projContainsCard(visible, other.FaceDownCard) {
					t.Fatalf("скрытая карта места %d (%s) попала в проекцию места %d — её не видит даже владелец",
						other.SeatNo, other.FaceDownCard.Code(), viewer.SeatNo)
				}
			}
		}
	}
}

func TestProjectionHidesDeckContents(t *testing.T) {
	state := aDeal().
		withDeck(projKingClubs, projAceClubs, projNineDiamonds, projQueenHearts, projTenClubs).
		withHand(0, projSevenDiamonds).
		build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if view.DeckLeft != 5 {
		t.Fatalf("в колоде 5 карт, проекция говорит %d", view.DeckLeft)
	}
	// Козырная карта из-под колоды видна всем законно (§1.9), остальная колода — нет.
	for _, card := range []Card{projKingClubs, projAceClubs, projNineDiamonds, projTenClubs} {
		if projContainsCard(projVisibleCards(view), card) {
			t.Fatalf("карта колоды %s видна игроку — так можно считать всю раздачу", card.Code())
		}
	}
}

func TestProjectionTellsOwnerOnlyTheFactOfHiddenCard(t *testing.T) {
	state := aDeal().withHand(0, projSevenDiamonds).withFaceDown(0, projSixSpades).build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if !view.IHaveHiddenCard {
		t.Fatal("владелец должен знать, что скрытая карта у него ещё есть")
	}
	if !projSeat(t, view, 0).HasHiddenCard {
		t.Fatal("факт скрытой карты виден за столом всем, включая своё место")
	}
	if projContainsCard(view.MyHand, projSixSpades) {
		t.Fatal("скрытая карта попала в свою же руку — владелец не должен её видеть")
	}
}

func TestProjectionShowsNavesSlotAndLevelOfEveryone(t *testing.T) {
	state := aDeal().withHand(0, projSevenDiamonds).withNavesLevel(1, 0).build()

	seat := projSeat(t, projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0), 1)

	if seat.NavesLevel != 0 {
		t.Fatalf("уровень соседа по шкале виден всем, ожидалось 0, получено %d", seat.NavesLevel)
	}
	rank, ok := seat.NextRank()
	if !ok || rank != Seven {
		t.Fatalf("соседу с навешенной шестёркой летит семёрка, а проекция говорит (%v, %v)", rank, ok)
	}
	if seat.NextIsJoker {
		t.Fatal("до джокера соседу ещё вся шкала")
	}
}

func TestProjectionAnnouncesJokerAsNextStep(t *testing.T) {
	aceLevel := FullNavesScale().JokerLevel() - 1
	state := aDeal().withHand(0, projSevenDiamonds).withNavesLevel(1, aceLevel).build()

	seat := projSeat(t, projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0), 1)

	if !seat.NextIsJoker {
		t.Fatal("на тузе следующим летит джокер — стол обязан это видеть")
	}
	if _, ok := seat.NextRank(); ok {
		t.Fatal("после туза обычного ранга не осталось, ранг должен быть пустым")
	}
}

func TestProjectionSendsProtectedSuitAlongsideTrump(t *testing.T) {
	state := aDeal().withTrump(Spades).withHand(0, projSevenDiamonds).build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	suit, ok := view.Trump()
	if !ok || suit != Spades {
		t.Fatalf("козырь пики, проекция отдаёт (%v, %v)", suit, ok)
	}
	if view.ProtectedSuit == nil || *view.ProtectedSuit != Clubs {
		t.Fatalf("при козыре пики защищена трефа, иначе фронт будет считать масти сам: %v", view.ProtectedSuit)
	}
}

// ⭐ Козырная карта открыта всем — она лежит на столе лицом вверх (§1.9). Это не дыра
// в тумане войны, а его граница: следом за ней лежит потайной козырь, и вот его
// не видит никто.
func TestProjectionShowsTrumpCardWhileDeckHoldsIt(t *testing.T) {
	state := aDeal().withDeck(projAceClubs, projNineDiamonds, projSevenDiamonds, projSixSpades).build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if view.TrumpCard != Card(projSevenDiamonds) {
		t.Fatalf("козырная — предпоследняя в колоде, последняя это потайной козырь; получено %v", view.TrumpCard)
	}
}

func TestProjectionHidesTrumpCardWhenOnlyHiddenTrumpIsLeft(t *testing.T) {
	state := aDeal().withDeck(projSixSpades).build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if view.TrumpCard != nil {
		t.Fatalf("последняя карта колоды — потайной козырь, её не видит никто; показано %v", view.TrumpCard)
	}
}

func TestProjectionNumbersPlayersByExitOrder(t *testing.T) {
	state := aDeal().withOutOfDeal(2).withOutOfDeal(1).withExitOrder(2, 1).build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if place, ok := projSeat(t, view, 2).ExitedAt(); !ok || place != 1 {
		t.Fatalf("вышедший первым получает первое место (от него зависит −1 по шкале), получено (%d, %v)", place, ok)
	}
	if place, ok := projSeat(t, view, 1).ExitedAt(); !ok || place != 2 {
		t.Fatalf("вышедший вторым получает второе место, получено (%d, %v)", place, ok)
	}
	if _, ok := projSeat(t, view, 0).ExitedAt(); ok {
		t.Fatal("место 0 ещё играет, места выхода у него быть не может")
	}
}

func TestProjectionCountsStepsLeftToJoker(t *testing.T) {
	scale := FullNavesScale()
	state := aDeal().withNavesLevel(1, NoNaves).withNavesLevel(2, scale.JokerLevel()-1).build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if steps := projSeat(t, view, 1).StepsToJoker; steps != scale.JokerLevel()+1 {
		t.Fatalf("нетронутому лететь всю шкалу и джокер сверху (%d), проекция считает %d",
			scale.JokerLevel()+1, steps)
	}
	if steps := projSeat(t, view, 2).StepsToJoker; steps != 1 {
		t.Fatalf("стоящему на тузе следующим летит джокер, осталась 1 ступень, а не %d", steps)
	}
}

func TestProjectionLeavesTrumpEmptyWhileItIsRolledFor(t *testing.T) {
	state := aDeal().withPhase(PhaseDice).build()
	// Нижней картой оказался джокер: масть ещё не названа, и выдумывать её нельзя.
	state.Trump = nil

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if view.Phase != PhaseDice {
		t.Fatalf("фаза броска кости потерялась: %v", view.Phase)
	}
	if _, ok := view.Trump(); ok {
		t.Fatal("козырь ещё не назван — проекция не вправе показывать масть")
	}
	if view.ProtectedSuit != nil {
		t.Fatal("без козыря нет и защищённой масти")
	}
}

func TestProjectionOffersExactlyTheLegalMoves(t *testing.T) {
	state := aDeal().
		withPhase(PhaseDefend).
		withAttack(projSevenDiamonds).
		withHand(1, projNineDiamonds, projSixClubs).
		build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 1)

	if !projContainsCommand(view.AvailableActions, DefendCommand{Seat: 1, Card: projNineDiamonds, Target: projSevenDiamonds}) {
		t.Fatalf("девятка бубён бьёт семёрку бубён, отбой обязан быть в списке: %v", view.AvailableActions)
	}
	if !projContainsCommand(view.AvailableActions, TakeCommand{Seat: 1}) {
		t.Fatal("защищающийся всегда может взять — иначе он окажется без хода")
	}
	if projContainsCommand(view.AvailableActions, DefendCommand{Seat: 1, Card: projSixClubs, Target: projSevenDiamonds}) {
		t.Fatal("шестёрка треф семёрку бубён не бьёт, а клиент поверит списку и пошлёт этот ход")
	}
}

func TestProjectionOffersNoActionWhenItIsNotMyTurn(t *testing.T) {
	state := aDeal().
		withPhase(PhaseDefend).
		withAttack(projSevenDiamonds).
		withHand(2, projNineDiamonds).
		build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 2)

	if len(view.AvailableActions) != 0 {
		t.Fatalf("не его ход — у него не должно быть ни одной кнопки, а предложено: %v", view.AvailableActions)
	}
}

func TestProjectionOffersRevealWithoutNamingTheCard(t *testing.T) {
	state := aDeal().
		withEmptyDeck().
		withFaceDown(0, projSixSpades).
		withHand(1, projAceClubs).
		build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if !projContainsCommand(view.AvailableActions, RevealFaceDownCommand{Seat: 0}) {
		t.Fatalf("остался только вскрытый ход скрытой картой, его обязаны предложить: %v", view.AvailableActions)
	}
	if projContainsCard(projVisibleCards(view), projSixSpades) {
		t.Fatal("вскрытие предлагается БЕЗ имени карты: игрок сам её не видит")
	}
}

// ⭐ Кандидаты берутся только из своих карт: даже движок, принимающий что угодно,
// не может протащить в список чужую карту, потому что её там некому построить.
func TestProjectionBuildsCandidatesOnlyFromOwnMaterial(t *testing.T) {
	state := aDeal().
		withHand(0, projSevenDiamonds).
		withHand(1, projKingClubs, projAceClubs).
		withFaceDown(0, projSixSpades).
		withAttack(projTenClubs).
		build()

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projAcceptAllEngine{}), state, 0)

	allowed := []Card{projSevenDiamonds, projTenClubs}
	for _, card := range projVisibleCards(view) {
		if !projContainsCard(allowed, card) {
			t.Fatalf("в проекции места 0 оказалась карта %s: своими являются только рука и стол", card.Code())
		}
	}
	if len(view.AvailableActions) == 0 {
		t.Fatal("движок принял всё, но список действий пуст — проекция вообще ничего не перебрала")
	}
}

func TestProjectionShowsHungCardsOfNeighbours(t *testing.T) {
	player := aDeal().build().MustPlayerAt(1).WithHungCard(projSixSpades, 0)
	state := aDeal().withHand(0, projSevenDiamonds).build().WithPlayer(player)

	seat := projSeat(t, projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0), 1)

	if len(seat.HungCards) != 1 || seat.HungCards[0] != Card(projSixSpades) {
		t.Fatalf("навесы соседа открыты всем легально (§2.3) — они заменяют счёт; получено %v", seat.HungCards)
	}
}

func TestProjectionKeepsRevealEventToItsOwner(t *testing.T) {
	events := []DealEvent{
		FaceDownRevealed{Seat: 0, Card: projSixSpades},
		NewCardAttacked(0, projSevenDiamonds),
	}
	projection := NewStateProjection(DefaultRulesConfig(), projEngine())

	if own := projection.EventsFor(events, 0); len(own) != 2 {
		t.Fatalf("владелец видит и вскрытие своей карты, и атаку: %v", own)
	}
	others := projection.EventsFor(events, 1)
	if len(others) != 1 {
		t.Fatalf("соседу уходит только публичная атака, получено %v", others)
	}
	if _, private := others[0].PrivateToSeat(); private {
		t.Fatal("приватное событие просочилось соседу — он узнал чужую скрытую карту")
	}
}

// Полная сдача, собранная руками: все карты колоды где-то есть, поэтому счёт отбоя
// обязан сойтись. ⚠️ Счёт выводится от обратного, и ошибка здесь тихая — «Бито 12»
// просто разойдётся с реальностью, и никто не заметит.
func projFullyDealtState(t *testing.T) DealState {
	t.Helper()
	full, err := BuildOrderedDeck(3)
	if err != nil {
		t.Fatalf("колода на троих не собралась: %v", err)
	}
	state := aDeal().
		withHand(0, full[0:6]...).
		withHand(1, full[6:12]...).
		withHand(2, full[12:18]...).
		withFaceDown(0, full[18]).
		withFaceDown(1, full[19]).
		withFaceDown(2, full[20]).
		withDeck(full[21:]...).
		build()
	return state
}

func TestProjectionShowsEmptyDiscardOnFreshDeal(t *testing.T) {
	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), projFullyDealtState(t), 0)

	if view.DiscardCount != 0 {
		t.Fatalf("сразу после сдачи в отбое ничего нет, проекция насчитала %d", view.DiscardCount)
	}
}

func TestProjectionCountsCardsThatLeftTheDeal(t *testing.T) {
	state := projFullyDealtState(t)
	// Раунд ушёл в отбой: две карты исчезли со стола и больше нигде не лежат.
	state.Deck = state.Deck[2:]

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if view.DiscardCount != 2 {
		t.Fatalf("из игры вышли 2 карты, значит в отбое 2, а не %d", view.DiscardCount)
	}
}

func TestProjectionCountsTableAndHungCardsAsStillInPlay(t *testing.T) {
	base := projFullyDealtState(t)
	player := base.MustPlayerAt(1)
	attack := player.Hand[0]
	defence := player.Hand[1]
	hung := player.Hand[2]
	state := base.WithPlayer(player.WithHand(player.Hand[3:]).WithHungCard(hung, 0))
	state = state.WithTable([]TableSlot{{Attack: attack, Defence: defence}})

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if view.DiscardCount != 0 {
		t.Fatalf("карты на столе и в навесах ещё в игре, в отбое должно быть 0, а не %d", view.DiscardCount)
	}
}

func TestProjectionNeverReportsNegativeDiscard(t *testing.T) {
	// Снимок собран вручную и карт в нём больше, чем в колоде: отрицательное «Бито»
	// на клиенте выглядело бы как поломка игры, а не как кривой снимок.
	state := projFullyDealtState(t)
	state = state.WithPlayer(state.MustPlayerAt(0).WithHand(append(copyCards(state.Deck), projSixSpades)))

	view := projProject(t, NewStateProjection(DefaultRulesConfig(), projEngine()), state, 0)

	if view.DiscardCount != 0 {
		t.Fatalf("отбой не может быть отрицательным, получено %d", view.DiscardCount)
	}
}

func TestProjectionRefusesSeatOutsideTable(t *testing.T) {
	state := aDeal().build()

	if _, err := NewStateProjection(DefaultRulesConfig(), projEngine()).Project(state, 7); err == nil {
		t.Fatal("места 7 за столом нет — проекция обязана отказать, а не собрать пустую")
	}
}

func TestProjectionWithoutEngineRefusesToProject(t *testing.T) {
	if _, err := NewStateProjection(DefaultRulesConfig(), nil).Project(aDeal().build(), 0); err == nil {
		t.Fatal("без движка список действий считать нечем — молчаливый пустой список скрыл бы поломку сборки")
	}
}
