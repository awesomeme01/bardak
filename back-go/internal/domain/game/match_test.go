package game

import (
	"slices"
	"testing"
)

// Перенос MatchEngineTest.
//
// Автомат матча — §0, §4.1. Раздача заканчивается, матч не обязан: уровни переносятся,
// колода собирается заново, игра продолжается до чьего-то джокера.

const anyMatchSeed = int64(20260810)

// ── Подставные части ─────────────────────────────────────────────────────────
//
// ⭐ Автомат матча проверяется на подставных частях, а не на честной сдаче: досидеть
// случайной игрой до «кому-то навесили джокера» нельзя, а проверяется здесь правило
// закрытия матча, а не умение бота играть.

type stubDeals struct {
	apply    func(state DealState, command DealCommand) MoveResult
	calls    int
	commands []DealCommand
}

func (s *stubDeals) Apply(state DealState, command DealCommand) MoveResult {
	s.calls++
	s.commands = append(s.commands, command)
	return s.apply(state, command)
}

type stubScoring struct {
	outcome DealOutcome
	calls   int
}

func (s *stubScoring) Score(DealState) (DealOutcome, error) {
	s.calls++
	return s.outcome, nil
}

type stubDealer struct {
	// trumpInHands — что отвечать на вопрос «козырь есть хоть у кого-то».
	trumpInHands bool
	// seeds и levels — с чем звали сдачу; на них и держится проверка под-seed.
	seeds  []int64
	levels [][]int
}

func (d *stubDealer) StartDeal(navesLevels []int, dealSeed int64) (DealState, error) {
	d.seeds = append(d.seeds, dealSeed)
	d.levels = append(d.levels, slices.Clone(navesLevels))
	deal := aDeal().withPlayers(len(navesLevels)).build()
	for seat, level := range navesLevels {
		deal = deal.WithPlayer(deal.MustPlayerAt(seat).WithNavesLevel(level))
	}
	deal.RngSeed = dealSeed
	return deal, nil
}

func (d *stubDealer) HasAnyTrumpInHands(DealState) bool { return d.trumpInHands }

func (d *stubDealer) ReshuffleSeed(seed int64, attempt int) int64 {
	return seed*31 + int64(attempt) + 1
}

// stubMatch — матч на подставных частях: движок раздачи отдаёт заранее заданный итог.
func stubMatch(move MoveResult, outcome DealOutcome) (*MatchEngine, *stubDeals, *stubScoring, *stubDealer) {
	deals := &stubDeals{apply: func(DealState, DealCommand) MoveResult { return move }}
	scoring := &stubScoring{outcome: outcome}
	dealer := &stubDealer{trumpInHands: true}
	return NewMatchEngine(deals, scoring, dealer), deals, scoring, dealer
}

// dealOverMove — ход, которым раздача закончилась.
func dealOverMove() MoveResult {
	deal := aDeal().withPlayers(3).withPhase(PhaseDealOver).build()
	return AppliedResult(deal, []DealEvent{NewDealFinished(1)})
}

// outcomeWithLevels — итог раздачи без проигравших матч.
func outcomeWithLevels(levels ...int) DealOutcome {
	players := make([]PlayerOutcome, 0, len(levels))
	for seat, level := range levels {
		players = append(players, NewPlayerOutcome(seat, NoNaves, level, NoLossDegree))
	}
	return NewDealOutcome(players, 0)
}

func applyOrFail(t *testing.T, engine *MatchEngine, state MatchState, command DealCommand) MatchOutcome {
	t.Helper()
	outcome, err := engine.Apply(state, command)
	if err != nil {
		t.Fatalf("матч сорвался на команде %T: %v", command, err)
	}
	return outcome
}

// ── Начало матча и под-seed ──────────────────────────────────────────────────

func TestStartMatchPutsEveryoneAtTheBottomOfTheScale(t *testing.T) {
	engine, _, _, dealer := stubMatch(dealOverMove(), outcomeWithLevels())

	state, err := engine.StartMatch(3, anyMatchSeed)
	if err != nil {
		t.Fatalf("матч не начался: %v", err)
	}

	if state.Phase != MatchInDeal {
		t.Errorf("матч стартовал в фазе %s, а должен идти", state.Phase)
	}
	for seat, level := range state.NavesLevels {
		if level != NoNaves {
			t.Errorf("место %d стартует с уровня %d: навесов ещё не было, ждали %d",
				seat, level, NoNaves)
		}
	}
	if state.DealNo != 1 {
		t.Errorf("первая раздача пронумерована %d, а счёт идёт с единицы", state.DealNo)
	}
	if len(state.Results) != 0 {
		t.Errorf("в новом матче уже %d сыгранных раздач", len(state.Results))
	}
	if len(dealer.seeds) != 1 || dealer.seeds[0] != DealSeed(anyMatchSeed, 1) {
		t.Errorf("первая раздача сдана от seed %v, ждали %d", dealer.seeds,
			DealSeed(anyMatchSeed, 1))
	}
}

// ⭐ Весь матч воспроизводим по паре «seed матча + последовательность команд»: под-seed
// обязан однозначно выводиться из seed матча и номера раздачи, иначе повторить партию
// нечем.
func TestDealSeedIsDerivedFromMatchSeedAndDealNo(t *testing.T) {
	if got, want := DealSeed(anyMatchSeed, 1), anyMatchSeed*1_000_003+1; got != want {
		t.Errorf("под-seed первой раздачи %d, ждали %d", got, want)
	}
	if DealSeed(anyMatchSeed, 1) == DealSeed(anyMatchSeed, 2) {
		t.Error("две раздачи одного матча получили одинаковый под-seed: расклад повторится")
	}
	if DealSeed(anyMatchSeed, 1) == DealSeed(anyMatchSeed+1, 1) {
		t.Error("разные матчи получили одинаковый под-seed первой раздачи")
	}
}

// ── Ход по раздаче ───────────────────────────────────────────────────────────

func TestApplyKeepsMatchFieldsWhileTheDealGoesOn(t *testing.T) {
	moved := aDeal().withPlayers(3).withPhase(PhaseDefend).build()
	engine, _, scoring, dealer := stubMatch(
		AppliedResult(moved, []DealEvent{NewPassed(0)}), outcomeWithLevels())
	state := NewMatchState(MatchInDeal, []int{NoNaves, 2, NoNaves}, 1, anyMatchSeed,
		aDeal().withPlayers(3).build(), nil)

	outcome := applyOrFail(t, engine, state, PassCommand{Seat: 0})

	if !outcome.Applied {
		t.Fatalf("обычный ход отклонён матчем: %s", outcome.Reason)
	}
	if outcome.State.Deal.Phase != PhaseDefend {
		t.Errorf("раздача осталась в фазе %s, а движок перевёл её в %s",
			outcome.State.Deal.Phase, PhaseDefend)
	}
	if outcome.State.DealNo != 1 || len(outcome.State.Results) != 0 {
		t.Errorf("незаконченная раздача уже посчитана: номер %d, итогов %d",
			outcome.State.DealNo, len(outcome.State.Results))
	}
	if !slices.Equal(outcome.State.NavesLevels, state.NavesLevels) {
		t.Errorf("уровни поехали посреди раздачи: %v, было %v",
			outcome.State.NavesLevels, state.NavesLevels)
	}
	if len(outcome.Events) != 1 {
		t.Errorf("матч отдал %d событий раздачи, а должен пропускать их как есть",
			len(outcome.Events))
	}
	if scoring.calls != 0 || len(dealer.seeds) != 0 {
		t.Error("матч посчитал итог и пересдал, хотя раздача ещё идёт")
	}
}

// ⭐ Отклонённая команда не меняет ничего: матч возвращает отказ раздачи как свой.
func TestApplyPassesTheDealRejectionThrough(t *testing.T) {
	engine, _, _, dealer := stubMatch(RejectedResult(CardNotInHand), outcomeWithLevels())
	state := NewMatchState(MatchInDeal, []int{NoNaves, NoNaves}, 1, anyMatchSeed,
		aDeal().withPlayers(2).build(), nil)

	outcome := applyOrFail(t, engine, state, AttackCommand{Seat: 0, Card: NewPip(Six, Clubs)})

	if outcome.Applied {
		t.Fatal("матч принял ход, отклонённый раздачей")
	}
	if outcome.Reason != CardNotInHand {
		t.Errorf("причина отказа %s, ждали %s", outcome.Reason, CardNotInHand)
	}
	if len(dealer.seeds) != 0 {
		t.Error("после отказа была сдана новая раздача")
	}
}

// ── Конец раздачи ────────────────────────────────────────────────────────────

func TestDealsAgainWhenTheDealEndsWithoutAJoker(t *testing.T) {
	engine, _, _, dealer := stubMatch(dealOverMove(), outcomeWithLevels(NoNaves, 1, 0))
	state := NewMatchState(MatchInDeal, []int{NoNaves, 0, NoNaves}, 1, anyMatchSeed,
		aDeal().withPlayers(3).build(), nil)

	outcome := applyOrFail(t, engine, state, PassCommand{Seat: 0})

	if outcome.State.Phase != MatchInDeal {
		t.Errorf("матч закончился в фазе %s, хотя джокера никому не навесили",
			outcome.State.Phase)
	}
	if outcome.State.DealNo != 2 {
		t.Errorf("после первой раздачи идёт раздача №%d, ждали 2", outcome.State.DealNo)
	}
	if len(outcome.State.Results) != 1 {
		t.Errorf("итогов накоплено %d, а сыграна одна раздача", len(outcome.State.Results))
	}
	want := []int{NoNaves, 1, 0}
	if !slices.Equal(outcome.State.NavesLevels, want) {
		t.Errorf("счёт матча %v, ждали %v", outcome.State.NavesLevels, want)
	}
	// ⭐ Уровни переносятся в новую сдачу: они живут матч, а не раздачу.
	if len(dealer.levels) != 1 || !slices.Equal(dealer.levels[0], want) {
		t.Errorf("новая раздача сдана с уровнями %v, а перенести надо %v", dealer.levels, want)
	}
	if len(dealer.seeds) != 1 || dealer.seeds[0] != DealSeed(anyMatchSeed, 2) {
		t.Errorf("вторая раздача сдана от seed %v, ждали %d", dealer.seeds,
			DealSeed(anyMatchSeed, 2))
	}
}

// ⭐ Матч кончается только тогда, когда после всех сдвигов у кого-то остался джокер.
func TestMatchEndsWhenSomebodyIsLeftWithTheJoker(t *testing.T) {
	jokerLevel := FullNavesScale().JokerLevel()
	loser := NewPlayerOutcome(1, jokerLevel-1, jokerLevel, LossSuperFail)
	outcomeOfDeal := NewDealOutcome(
		[]PlayerOutcome{NewPlayerOutcome(0, NoNaves, NoNaves, NoLossDegree), loser}, 1)
	engine, _, _, dealer := stubMatch(dealOverMove(), outcomeOfDeal)
	state := NewMatchState(MatchInDeal, []int{NoNaves, jokerLevel - 1}, 1, anyMatchSeed,
		aDeal().withPlayers(2).build(), nil)

	outcome := applyOrFail(t, engine, state, PassCommand{Seat: 0})

	if outcome.State.Phase != MatchOver {
		t.Fatalf("матч в фазе %s, хотя месту 1 навешен джокер", outcome.State.Phase)
	}
	if outcome.State.NavesLevelAt(1) != jokerLevel {
		t.Errorf("уровень проигравшего %d, ждали джокер (%d)",
			outcome.State.NavesLevelAt(1), jokerLevel)
	}
	main, ok := outcome.State.MainLoser()
	if !ok {
		t.Fatal("матч закончен, а главного проигравшего нет")
	}
	if main.SeatNo != 1 || main.LossDegree != LossSuperFail {
		t.Errorf("главный проигравший — место %d со степенью %s, ждали место 1 и %s",
			main.SeatNo, main.LossDegree, LossSuperFail)
	}
	// ⚠️ Номер раздачи не растёт и новая сдача не происходит: играть больше нечего.
	if outcome.State.DealNo != 1 {
		t.Errorf("после конца матча номер раздачи %d, ждали 1", outcome.State.DealNo)
	}
	if len(dealer.seeds) != 0 {
		t.Error("после конца матча сдана новая раздача")
	}
}

func TestRefusesAnyCommandWhenTheMatchIsOver(t *testing.T) {
	engine, deals, _, _ := stubMatch(dealOverMove(), outcomeWithLevels())
	finished := NewMatchState(MatchOver, []int{NoNaves, NoNaves}, 1, anyMatchSeed,
		aDeal().withPlayers(2).build(), nil)

	outcome := applyOrFail(t, engine, finished, PassCommand{Seat: 0})

	if outcome.Applied {
		t.Fatal("законченный матч принял команду")
	}
	if outcome.Reason != NotYourTurn {
		t.Errorf("причина отказа %s, ждали %s", outcome.Reason, NotYourTurn)
	}
	if deals.calls != 0 {
		t.Error("команда ушла в движок раздачи, хотя матч уже закончен")
	}
}

// ── Пересдача, когда козырь назвали, а козырей нет ───────────────────────────

// ⭐ Козырь могли назвать костью — и назвать масть, которой нет ни у кого (§1.2).
// Тогда первый ход определять не из чего, и раздача пересдаётся (OQ-22).
func TestReshufflesWhenNobodyHoldsTheChosenTrump(t *testing.T) {
	fresh := aDeal().withPlayers(3).withPhase(PhaseAttack).build()
	engine, _, _, dealer := stubMatch(
		AppliedResult(fresh, []DealEvent{NewTrumpChosen(0, Hearts)}), outcomeWithLevels())
	dealer.trumpInHands = false
	levels := []int{NoNaves, 3, NoNaves}
	state := NewMatchState(MatchInDeal, levels, 4, anyMatchSeed,
		aDeal().withPlayers(3).build(), nil)

	outcome := applyOrFail(t, engine, state, ChooseTrumpCommand{Seat: 0, Suit: Hearts})

	wantSeed := dealer.ReshuffleSeed(DealSeed(anyMatchSeed, 4), 0)
	if len(dealer.seeds) != 1 || dealer.seeds[0] != wantSeed {
		t.Fatalf("пересдача пошла от seed %v, ждали производный от матча %d",
			dealer.seeds, wantSeed)
	}
	// ⭐ Уровни переживают пересдачу: она меняет расклад, а не счёт.
	if !slices.Equal(dealer.levels[0], levels) {
		t.Errorf("пересдача пошла с уровнями %v, а перенести надо %v", dealer.levels[0], levels)
	}
	if outcome.State.Deal.RngSeed != wantSeed {
		t.Errorf("в матче осталась старая раздача (seed %d), а должна быть пересданная",
			outcome.State.Deal.RngSeed)
	}
	if outcome.State.DealNo != 4 {
		t.Errorf("пересдача сменила номер раздачи на %d: это та же раздача, %d",
			outcome.State.DealNo, 4)
	}
}

// ⚠️ Пересдача привязана к НАЧАЛУ раздачи. Позже козырей на руках может не остаться
// совершенно законно, и пересдавать там нечего.
func TestKeepsTheDealWhenReshuffleIsNotCalledFor(t *testing.T) {
	withoutTrump := aDeal().withPlayers(3).build()
	withoutTrump.Trump = nil

	cases := []struct {
		name         string
		deal         DealState
		trumpInHands bool
	}{
		{"козырь есть у кого-то", aDeal().withPlayers(3).build(), true},
		{"стол не пуст", aDeal().withPlayers(3).withAttack(NewPip(Six, Clubs)).build(), false},
		{"раздача уже идёт", aDeal().withPlayers(3).withPhase(PhaseDefend).build(), false},
		{"козырь ещё не назван", withoutTrump, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine, _, _, dealer := stubMatch(
				AppliedResult(testCase.deal, nil), outcomeWithLevels())
			dealer.trumpInHands = testCase.trumpInHands
			state := NewMatchState(MatchInDeal, []int{NoNaves, NoNaves, NoNaves}, 1,
				anyMatchSeed, aDeal().withPlayers(3).build(), nil)

			applyOrFail(t, engine, state, PassCommand{Seat: 0})

			if len(dealer.seeds) != 0 {
				t.Errorf("раздача пересдана зря: %s", testCase.name)
			}
		})
	}
}

// ── Неизменяемость снимка ────────────────────────────────────────────────────

// ⚠️ Срез в Go разделяет память с тем, из чего он сделан: без защитных копий «прошлое»
// состояние матча менялось бы задним числом.
func TestMatchStateCopiesItsSlices(t *testing.T) {
	levels := []int{NoNaves, 1}
	state := NewMatchState(MatchInDeal, levels, 1, anyMatchSeed,
		aDeal().withPlayers(2).build(), nil)

	levels[1] = 8
	if state.NavesLevelAt(1) != 1 {
		t.Error("счёт матча поменялся через срез, отданный при сборке")
	}

	next := state.WithNavesLevels([]int{2, 2}).WithResult(outcomeWithLevels(2, 2))
	if state.NavesLevelAt(0) != NoNaves {
		t.Error("WithNavesLevels изменил исходный снимок вместо копии")
	}
	if len(state.Results) != 0 {
		t.Errorf("WithResult дописал итог в исходный снимок: их там %d", len(state.Results))
	}
	if _, ok := state.LastResult(); ok {
		t.Error("в матче без сыгранных раздач нашёлся последний итог")
	}
	if _, ok := next.LastResult(); !ok {
		t.Error("после сыгранной раздачи последний итог не найден")
	}
	if _, ok := state.MainLoser(); ok {
		t.Error("главный проигравший объявился, пока матч идёт")
	}
}

// ── Живой матч на боевых частях ──────────────────────────────────────────────

// matchAboutToFinish — матч на пороге конца: место 0 уже вышло, у места 1 остались карты
// и уровень loserLevel. Один пас закрывает раунд, раздачу и — если уровень был тузом —
// весь матч.
//
// ⭐ Состояние собирается руками намеренно: досидеть до этого места случайной игрой
// нельзя, а проверяется здесь правило закрытия матча, а не умение бота играть.
func matchAboutToFinish(loserLevel int) MatchState {
	deal := aDeal().
		withPlayers(2).
		withEmptyDeck().
		withOutOfDeal(0).
		withExitOrder(0).
		withHand(1, NewPip(Ace, Clubs)).
		withNavesLevel(1, loserLevel).
		withDefender(1).
		build()
	return NewMatchState(MatchInDeal, []int{NoNaves, loserLevel}, 1, anyMatchSeed, deal, nil)
}

func TestLiveMatchEndsWhenTheDealLoserReachesTheJoker(t *testing.T) {
	engine := NewDefaultMatchEngine()
	jokerLevel := FullNavesScale().JokerLevel()

	outcome := applyOrFail(t, engine, matchAboutToFinish(jokerLevel-1), PassCommand{Seat: 0})

	if !outcome.Applied {
		t.Fatalf("пас, закрывающий раздачу, отклонён: %s", outcome.Reason)
	}
	next := outcome.State
	if next.Phase != MatchOver {
		t.Fatalf("матч в фазе %s, хотя проигравший раздачу дошёл до джокера", next.Phase)
	}
	if next.NavesLevelAt(1) != jokerLevel {
		t.Errorf("уровень проигравшего %d, ждали джокер (%d)", next.NavesLevelAt(1), jokerLevel)
	}
	main, ok := next.MainLoser()
	if !ok {
		t.Fatal("матч закончен, а главного проигравшего нет")
	}
	// Джокер получен не картой, а +1 за проигранную раздачу — это SUPER_FAIL (§0.3).
	if main.LossDegree != LossSuperFail {
		t.Errorf("степень проигрыша %s, ждали %s", main.LossDegree, LossSuperFail)
	}
	if next.DealNo != 1 {
		t.Errorf("после конца матча номер раздачи %d, ждали 1", next.DealNo)
	}
}

func TestLiveMatchDealsAgainWhenTheDealEndsWithoutAJoker(t *testing.T) {
	engine := NewDefaultMatchEngine()

	outcome := applyOrFail(t, engine, matchAboutToFinish(NoNaves), PassCommand{Seat: 0})

	next := outcome.State
	if next.Phase != MatchInDeal {
		t.Fatalf("матч закончился в фазе %s, хотя навесили только первую ступень", next.Phase)
	}
	if next.DealNo != 2 {
		t.Errorf("после первой раздачи идёт раздача №%d, ждали 2", next.DealNo)
	}
	if next.NavesLevelAt(1) != NoNaves+1 {
		t.Errorf("проигравший раздачу остался на уровне %d, ждали %d",
			next.NavesLevelAt(1), NoNaves+1)
	}
	if len(next.Results) != 1 {
		t.Errorf("итогов накоплено %d, а сыграна одна раздача", len(next.Results))
	}
}

func TestLiveMatchRefusesCommandsAfterItIsOver(t *testing.T) {
	engine := NewDefaultMatchEngine()
	jokerLevel := FullNavesScale().JokerLevel()
	finished := applyOrFail(t, engine, matchAboutToFinish(jokerLevel-1), PassCommand{Seat: 0}).State

	outcome := applyOrFail(t, engine, finished, PassCommand{Seat: 0})

	if outcome.Applied || outcome.Reason != NotYourTurn {
		t.Errorf("законченный матч ответил (принят=%v, причина %s), ждали отказ %s",
			outcome.Applied, outcome.Reason, NotYourTurn)
	}
}

// ⭐ Уровни переносятся, а слоты и руки — нет: карты из слотов возвращаются в игру
// сами собой, потому что колода собирается заново.
func TestCarriesTheLevelsAndClearsTheSlotsWhenANewDealIsDealt(t *testing.T) {
	engine := NewDefaultMatchEngine()
	state := playUntilDeal(t, engine, startMatchOrFail(t, engine, 3, anyMatchSeed), 2)

	if state.DealNo != 2 {
		t.Fatalf("матч не дошёл до второй раздачи: номер %d", state.DealNo)
	}
	for seat, player := range state.Deal.Players {
		if player.NavesLevel != state.NavesLevelAt(seat) {
			t.Errorf("место %d сдано с уровнем %d, а в счёте матча %d: уровень обязан "+
				"пережить перераздачу", seat, player.NavesLevel, state.NavesLevelAt(seat))
		}
		if len(player.HungCards) != 0 {
			t.Errorf("место %d начало новую раздачу с %d навешенными картами: слот живёт "+
				"одну раздачу", seat, len(player.HungCards))
		}
		if !player.InDeal {
			t.Errorf("место %d не участвует в новой раздаче", seat)
		}
	}
}

func TestPutsEveryCardBackIntoPlayWhenANewDealIsDealt(t *testing.T) {
	engine := NewDefaultMatchEngine()
	state := playUntilDeal(t, engine, startMatchOrFail(t, engine, 3, anyMatchSeed), 2)

	everywhere := slices.Clone(state.Deal.Deck)
	for _, player := range state.Deal.Players {
		everywhere = append(everywhere, player.Hand...)
		if player.HasFaceDownCard() {
			everywhere = append(everywhere, player.FaceDownCard)
		}
	}

	full, err := BuildOrderedDeck(3)
	if err != nil {
		t.Fatalf("колода не собралась: %v", err)
	}
	expectSameCards(t, everywhere, full, "новая сдача")
}

// ⭐ Расклад выводится из seed матча: другой матч — другая колода.
func TestDivergesWhenTheMatchSeedDiffers(t *testing.T) {
	engine := NewDefaultMatchEngine()

	first := startMatchOrFail(t, engine, 4, anyMatchSeed)
	second := startMatchOrFail(t, engine, 4, anyMatchSeed+1)

	if slices.Equal(codesOf(first.Deal.Deck), codesOf(second.Deal.Deck)) {
		t.Error("матчи с разными seed получили одинаковый остаток колоды")
	}
}

// ⭐ Проверка целостности: карта не должна ни исчезнуть, ни размножиться, и хоть одна
// команда обязана приниматься в любой фазе — иначе матч встаёт намертво.
func TestLongMatchAlwaysAcceptsSomeCommandAndNeverLosesACard(t *testing.T) {
	engine := NewDefaultMatchEngine()
	full, err := BuildOrderedDeck(4)
	if err != nil {
		t.Fatalf("колода не собралась: %v", err)
	}
	state := startMatchOrFail(t, engine, 4, anyMatchSeed)

	for move := 0; move < 3000 && !state.IsOver(); move++ {
		inPlay := cardsInPlay(state.Deal)
		expectNoDuplicates(t, inPlay, state.Deal.Phase)
		if len(inPlay) > len(full) {
			t.Fatalf("в игре %d карт, а в колоде их всего %d", len(inPlay), len(full))
		}
		for _, card := range inPlay {
			if !slices.Contains(codesOf(full), card.Code()) {
				t.Fatalf("в игре карта %s, которой нет в колоде", card.Code())
			}
		}
		next, ok := nextMove(t, engine, state)
		if !ok {
			t.Fatalf("ни одна команда не принимается в фазе %s (ход %d)",
				state.Deal.Phase, move)
		}
		state = next
	}
}

// ── Автоигрок ────────────────────────────────────────────────────────────────

func startMatchOrFail(t *testing.T, engine *MatchEngine, playerCount int, seed int64) MatchState {
	t.Helper()
	state, err := engine.StartMatch(playerCount, seed)
	if err != nil {
		t.Fatalf("матч не начался: %v", err)
	}
	return state
}

// playUntilDeal доигрывает матч до нужной раздачи простейшим автоигроком.
func playUntilDeal(t *testing.T, engine *MatchEngine, start MatchState, stopAtDealNo int) MatchState {
	t.Helper()
	state := start
	for move := 0; move < 20000; move++ {
		if state.IsOver() || state.DealNo >= stopAtDealNo {
			return state
		}
		next, ok := nextMove(t, engine, state)
		if !ok {
			t.Fatalf("ни одна команда не принимается в фазе %s", state.Deal.Phase)
		}
		state = next
	}
	t.Fatalf("матч не дошёл до раздачи №%d за 20000 ходов", stopAtDealNo)
	return state
}

// nextMove — первая команда, которую движок принял.
func nextMove(t *testing.T, engine *MatchEngine, state MatchState) (MatchState, bool) {
	t.Helper()
	for _, command := range botCommands(state.Deal) {
		outcome, err := engine.Apply(state, command)
		if err != nil {
			t.Fatalf("матч сорвался на команде %T: %v", command, err)
		}
		if outcome.Applied {
			return outcome.State, true
		}
	}
	return state, false
}

// botCommands — команды-кандидаты в порядке предпочтения: сыграть картой лучше,
// чем спасовать.
//
// ⭐ Бот намеренно НЕ подкидывает и предпочитает пас взятию: раунд из одной атаки либо
// отбивается и уходит в отбой, либо забирается. Иначе карты только и делают, что
// возвращаются в руки, и раздача не сходится — играть плохо тоже надо уметь предсказуемо.
func botCommands(deal DealState) []DealCommand {
	if deal.Phase == PhaseDice {
		chooser := deal.AttackRightSeat
		if deal.PendingHiddenTrump != nil {
			chooser = deal.PendingHiddenTrump.ChooserSeat
		}
		commands := make([]DealCommand, 0, len(Suits()))
		for _, suit := range Suits() {
			commands = append(commands, ChooseTrumpCommand{Seat: chooser, Suit: suit})
		}
		return commands
	}

	commands := make([]DealCommand, 0, 32)
	if window := deal.HangingWindow; window != nil {
		for _, seat := range window.CurrentStep() {
			for _, card := range deal.MustPlayerAt(seat).Hand {
				commands = append(commands, HangCardCommand{Seat: seat, Card: card})
			}
			commands = append(commands, HangSkipCommand{Seat: seat})
		}
	}

	defender := deal.Defender()
	for _, slot := range deal.Table {
		for _, card := range cheapestFirst(defender.Hand, deal) {
			commands = append(commands,
				DefendCommand{Seat: defender.SeatNo, Card: card, Target: slot.Attack})
		}
	}
	attacker := deal.AttackRightSeat
	if len(deal.Table) == 0 {
		for _, card := range cheapestFirst(deal.MustPlayerAt(attacker).Hand, deal) {
			commands = append(commands, AttackCommand{Seat: attacker, Card: card})
		}
	}
	for _, card := range defender.Hand {
		commands = append(commands, TransferCommand{Seat: defender.SeatNo, Card: card})
	}
	for _, slot := range deal.Table {
		commands = append(commands,
			RevealFaceDownToDefendCommand{Seat: defender.SeatNo, Target: slot.Attack})
	}
	return append(commands,
		RevealFaceDownCommand{Seat: attacker},
		PassCommand{Seat: attacker},
		TakeCommand{Seat: defender.SeatNo})
}

// cheapestFirst — карты от самой дешёвой к самой дорогой: сначала некозырные по рангу,
// потом козыри, джокеры последними.
//
// ⭐ Так бот и атакует младшим, и бьёт минимально достаточным. Без этого «бито» почти
// не случается, а без «бито» карты не уходят из игры и раздача не сходится.
func cheapestFirst(hand []Card, deal DealState) []Card {
	sorted := slices.Clone(hand)
	slices.SortStableFunc(sorted, func(left, right Card) int {
		return cardWeight(left, deal) - cardWeight(right, deal)
	})
	return sorted
}

func cardWeight(card Card, deal DealState) int {
	pip, isPip := card.(Pip)
	if !isPip {
		return 1000
	}
	if deal.HasTrump() && pip.Suit == deal.Trump.Suit {
		return int(pip.Rank) + 100
	}
	return int(pip.Rank)
}

// cardsInPlay — все карты, которые сейчас где-то лежат: колода, руки, скрытые карты,
// навесы и стол. Отбой сюда не входит — оттуда карты уже не возвращаются.
func cardsInPlay(deal DealState) []Card {
	cards := slices.Clone(deal.Deck)
	for _, player := range deal.Players {
		cards = append(cards, player.Hand...)
		cards = append(cards, player.HungCards...)
		if player.HasFaceDownCard() {
			cards = append(cards, player.FaceDownCard)
		}
	}
	for _, slot := range deal.Table {
		cards = append(cards, slot.Attack)
		if slot.Defence != nil {
			cards = append(cards, slot.Defence)
		}
	}
	return cards
}

func codesOf(cards []Card) []string {
	codes := make([]string, 0, len(cards))
	for _, card := range cards {
		codes = append(codes, card.Code())
	}
	return codes
}

func expectNoDuplicates(t *testing.T, cards []Card, phase DealPhase) {
	t.Helper()
	seen := make(map[string]bool, len(cards))
	for _, card := range cards {
		if seen[card.Code()] {
			t.Fatalf("карта %s удвоилась в фазе %s", card.Code(), phase)
		}
		seen[card.Code()] = true
	}
}

func expectSameCards(t *testing.T, got, want []Card, what string) {
	t.Helper()
	expectNoDuplicates(t, got, PhaseDealing)
	gotCodes := codesOf(got)
	slices.Sort(gotCodes)
	wantCodes := codesOf(want)
	slices.Sort(wantCodes)
	if !slices.Equal(gotCodes, wantCodes) {
		t.Errorf("%s: в игре %v, а в колоде %v", what, gotCodes, wantCodes)
	}
}
