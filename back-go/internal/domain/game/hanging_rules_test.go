package game

import "testing"

// Перенос HangingTest в части правил навеса.
//
// Навес — центральная механика: ошибка здесь не роняет сервер, а тихо отдаёт право
// не тому игроку или пропускает карту не с той ступени шкалы, и партия расходится
// с тем, во что играют за столом.

func hangRules() HangingRules { return NewHangingRules(DefaultRulesConfig()) }

// aceLevel — уровень «навешен туз»: следующая ступень уже джокер.
func aceLevel() int { return FullNavesScale().JokerLevel() - 1 }

// atRoundStarter — кто начал раунд. От него считается очередь права на навес.
func atRoundStarter(state DealState, seatNo int) DealState {
	state.RoundStarterSeat = seatNo
	return state
}

func expectSeatList(t *testing.T, got, want []int, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: получили места %v, ждали %v", what, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s: получили места %v, ждали %v", what, got, want)
		}
	}
}

func expectRightSteps(t *testing.T, got, want [][]int, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: получили ступени %v, ждали %v", what, got, want)
	}
	for index := range want {
		expectSeatList(t, got[index], want[index], what)
	}
}

// ── Шкала: какая карта летит ─────────────────────────────────────────────────

// Масть при навесе не значит ничего: летит ранг следующей ступени.
func TestFlyingCardIsNextRankOfAnySuit(t *testing.T) {
	victim := NewPlayerState(1, nil, nil) // уровня ещё нет, летит шестёрка

	for _, suit := range Suits() {
		if !hangRules().IsFlyingCard(victim, NewPip(Six, suit)) {
			t.Errorf("шестёрка %s не летит на первую ступень, хотя масть при навесе не важна", suit)
		}
	}
}

func TestFlyingCardRejectsOtherRanks(t *testing.T) {
	victim := NewPlayerState(1, nil, nil)

	if hangRules().IsFlyingCard(victim, NewPip(Ace, Clubs)) {
		t.Error("туз залетел на ступень шестёрки: шкала перестала быть счётом")
	}
}

// ⭐ На тузе следующая ступень — джокер, и обычная карта туда уже не летит.
func TestJokerLevelAcceptsOnlyJoker(t *testing.T) {
	victim := NewPlayerState(1, nil, nil).WithNavesLevel(aceLevel())
	rules := hangRules()

	if !rules.NextIsJoker(victim) {
		t.Fatal("после туза следующая ступень обязана быть джокером")
	}
	if !rules.IsFlyingCard(victim, MustJoker(1)) {
		t.Error("джокер не летит на джокерную ступень, а именно он там и решает исход")
	}
	if rules.IsFlyingCard(victim, NewPip(Ace, Clubs)) {
		t.Error("туз залетел вместо джокера: жертву добили бы обычной картой")
	}
}

// Шкала пройдена — навешивать больше нечего: игрок уже проиграл.
func TestFinishedScaleTakesNoMoreCards(t *testing.T) {
	victim := NewPlayerState(1, nil, nil).WithNavesLevel(FullNavesScale().JokerLevel())

	if hangRules().IsFlyingCard(victim, MustJoker(1)) {
		t.Error("на добитого игрока всё ещё что-то летит")
	}
}

// ── Отстающий ────────────────────────────────────────────────────────────────

func TestUniqueLaggardWhenLowestLevelIsAlone(t *testing.T) {
	state := aDeal().withNavesLevel(0, 3).withNavesLevel(1, 0).withNavesLevel(2, 5).build()

	if !hangRules().IsUniqueLaggard(state, 1) {
		t.Error("единственный отстающий не распознан: правило добивания не включится")
	}
}

// ⭐ Разделённый минимум правило не включает — навес тогда обычный (ADR-028).
func TestSharedLowestLevelIsNotLaggard(t *testing.T) {
	state := aDeal().withNavesLevel(0, 3).withNavesLevel(1, 0).withNavesLevel(2, 0).build()

	if hangRules().IsUniqueLaggard(state, 1) {
		t.Error("при разделённом минимуме включилось добивание, хотя навес должен быть обычным")
	}
}

// ⚠️ Вышедшие в сравнении не участвуют, даже если их уровень ниже всех.
func TestLaggardIgnoresPlayersOutOfDeal(t *testing.T) {
	state := aDeal().
		withNavesLevel(0, 4).
		withNavesLevel(1, 2).
		withNavesLevel(2, NoNaves).
		withOutOfDeal(2).
		build()

	if !hangRules().IsUniqueLaggard(state, 1) {
		t.Error("вышедший из раздачи помешал признать отстающего, хотя его карт уже нет в игре")
	}
}

// ── Кому принадлежит право ───────────────────────────────────────────────────

// Джокер: право сразу у всех, приоритета нет вообще.
func TestJokerGivesTheRightToEveryoneAtOnce(t *testing.T) {
	state := aDeal().withNavesLevel(1, aceLevel()).build()

	if !hangRules().IsRightEqualForAll(state, 1) {
		t.Error("право на джокер осталось у очереди, хотя джокер разыгрывают все сразу")
	}
}

func TestUniqueLaggardGivesTheRightToEveryoneAtOnce(t *testing.T) {
	state := aDeal().withNavesLevel(0, 3).withNavesLevel(2, 5).build()

	if !hangRules().IsRightEqualForAll(state, 1) {
		t.Error("отстающего добивают всем столом, очередь тут не действует")
	}
}

func TestOrdinaryCaseKeepsThePriorityQueue(t *testing.T) {
	state := aDeal().withNavesLevel(0, 3).withNavesLevel(2, NoNaves).build()

	if hangRules().IsRightEqualForAll(state, 1) {
		t.Error("в обычном случае право раздали всем сразу, минуя атаковавшего")
	}
}

// ⭐ Отстающего добивают всем столом: навешивают все заявившиеся.
func TestEveryClaimantHangsForTheLaggard(t *testing.T) {
	state := aDeal().withNavesLevel(0, 3).withNavesLevel(2, 5).build()

	if !hangRules().IsEveryClaimantHanging(state, 1) {
		t.Error("отстающему навесил бы только один, хотя уровень поднимается всё равно на одну ступень")
	}
}

// ⚠️ Джокер один и решает исход — правило отстающего на него не распространяется,
// даже когда жертва вдобавок отстающая.
func TestJokerCancelsTheLaggardRule(t *testing.T) {
	finished := FullNavesScale().JokerLevel()
	state := aDeal().
		withNavesLevel(1, aceLevel()).
		withNavesLevel(0, finished).
		withNavesLevel(2, finished).
		build()
	rules := hangRules()

	if !rules.IsUniqueLaggard(state, 1) {
		t.Fatal("подготовка теста: жертва обязана быть отстающей")
	}
	if rules.IsEveryClaimantHanging(state, 1) {
		t.Error("джокеров навесили бы сразу два: добить игрока может только один")
	}
}

// ── Очередь права ────────────────────────────────────────────────────────────

// Очередь: атаковавший → поддержавший (сосед жертвы) → все остальные.
func TestPriorityOrderStartsWithAttackerThenSupporter(t *testing.T) {
	state := atRoundStarter(aDeal().withPlayers(4).build(), 0)

	expectSeatList(t, hangRules().PriorityOrder(state, 1), []int{0, 2, 3}, "очередь права на навес")
}

// Начавший раунд сам оказался жертвой: очередь начинается с его соседа, без дублей.
func TestPriorityOrderSkipsTheVictimEvenWhenSheStartedTheRound(t *testing.T) {
	state := atRoundStarter(aDeal().withPlayers(4).build(), 0)

	expectSeatList(t, hangRules().PriorityOrder(state, 0), []int{1, 2, 3}, "жертва начала раунд")
}

func TestPriorityOrderSkipsPlayersOutOfDeal(t *testing.T) {
	state := atRoundStarter(aDeal().withPlayers(4).withOutOfDeal(2).build(), 0)

	expectSeatList(t, hangRules().PriorityOrder(state, 1), []int{0, 3}, "вышедший в очереди права")
}

// Атаковавшим может оказаться не место 0: очередь считается от начавшего раунд.
func TestPriorityOrderIsCountedFromTheRoundStarter(t *testing.T) {
	state := atRoundStarter(aDeal().withPlayers(4).build(), 2)

	expectSeatList(t, hangRules().PriorityOrder(state, 3), []int{2, 0, 1}, "очередь от начавшего раунд")
}

// ── Кто способен навесить ────────────────────────────────────────────────────

func TestSeatsHoldingFlyingCardKeepPriorityOrder(t *testing.T) {
	state := atRoundStarter(aDeal().withPlayers(4).
		withHand(0, NewPip(Ace, Clubs)).
		withHand(2, NewPip(Six, Hearts)).
		withHand(3, NewPip(Six, Clubs)).
		build(), 0)

	expectSeatList(t, hangRules().SeatsHoldingFlyingCard(state, 1), []int{2, 3},
		"держатели летящей карты")
}

// ⭐ Нужной карты нет ни у кого — окно навеса вообще не открывается.
func TestNobodyHoldsTheFlyingCard(t *testing.T) {
	state := aDeal().
		withHand(0, NewPip(Ace, Clubs)).
		withHand(1, NewPip(King, Clubs)).
		withHand(2, NewPip(Queen, Clubs)).
		build()

	if holders := hangRules().SeatsHoldingFlyingCard(state, 1); len(holders) != 0 {
		t.Errorf("окно открылось бы на пустом месте: держатели %v", holders)
	}
}

// Жертва держит летящую карту, но самой себе навесить нельзя.
func TestVictimIsNeverAHolder(t *testing.T) {
	state := aDeal().withHand(1, NewPip(Six, Hearts)).build()

	if holders := hangRules().SeatsHoldingFlyingCard(state, 1); len(holders) != 0 {
		t.Errorf("жертва попала в держатели и навесила бы сама себе: %v", holders)
	}
}

// ── Ступени права ────────────────────────────────────────────────────────────

func TestStepsSplitTheQueueIntoTiers(t *testing.T) {
	state := atRoundStarter(aDeal().withPlayers(4).build(), 0)
	rules := hangRules()

	steps := rules.Steps(state, 1, []int{0, 2, 3})

	expectRightSteps(t, steps, [][]int{{0}, {2}, {3}}, "ступени права в обычном случае")
}

// Ступень пропускается, если у приоритетного игрока нужной карты нет.
func TestStepsSkipTiersWithoutTheCard(t *testing.T) {
	state := atRoundStarter(aDeal().withPlayers(4).build(), 0)

	steps := hangRules().Steps(state, 1, []int{2, 3})

	expectRightSteps(t, steps, [][]int{{2}, {3}}, "атаковавший без карты")
}

// ⚠️ Все остальные стоят на ОДНОЙ ступени: спор между ними решает кость,
// а не «кто первый успел».
func TestStepsGroupTheRestIntoOneStep(t *testing.T) {
	state := atRoundStarter(aDeal().withPlayers(5).build(), 0)

	steps := hangRules().Steps(state, 1, []int{3, 4})

	expectRightSteps(t, steps, [][]int{{3, 4}}, "остальные наравне")
}

// Право равно у всех — ступень одна, и на ней сразу все держатели.
func TestStepsAreSingleWhenRightIsEqualForAll(t *testing.T) {
	state := atRoundStarter(aDeal().withPlayers(4).
		withNavesLevel(0, 3).
		withNavesLevel(2, 5).
		withNavesLevel(3, 5).
		build(), 0)

	steps := hangRules().Steps(state, 1, []int{0, 2, 3})

	expectRightSteps(t, steps, [][]int{{0, 2, 3}}, "право у всех сразу")
}

// ⭐ Ступени — копия: правка возвращённого среза не должна менять окно навеса.
func TestStepsDoNotShareTheHoldersSlice(t *testing.T) {
	state := aDeal().withNavesLevel(0, 3).withNavesLevel(2, 5).build()
	holders := []int{0, 2}

	steps := hangRules().Steps(state, 1, holders)
	steps[0][0] = 42

	if holders[0] != 0 {
		t.Error("ступени делят срез с держателями: окно навеса меняется со стороны")
	}
}

// ── Проверка самого навеса ───────────────────────────────────────────────────

func TestCanHangAllowsTheFlyingCardFromHand(t *testing.T) {
	six := NewPip(Six, Clubs)
	state := aDeal().withHand(0, six).build()

	expectAllowed(t, hangRules().CanHang(state, 0, 1, six), "навес летящей картой")
}

// ⚠️ Самому себе навесить нельзя — и проверка эта стоит РАНЬШЕ проверки руки:
// иначе жертва по ответу движка узнала бы, что за карту ей приписали.
func TestCanHangRefusesHangingOnYourself(t *testing.T) {
	six := NewPip(Six, Hearts)
	state := aDeal().withHand(1, six).build()
	rules := hangRules()

	expectRejected(t, rules.CanHang(state, 1, 1, six), CannotHangOnSelf, "навес самому себе")
	expectRejected(t, rules.CanHang(state, 1, 1, NewPip(Six, Clubs)), CannotHangOnSelf,
		"навес самому себе картой не из руки")
}

func TestCanHangRefusesCardNotInHand(t *testing.T) {
	state := aDeal().withHand(0, NewPip(Ace, Clubs)).build()

	expectRejected(t, hangRules().CanHang(state, 0, 1, NewPip(Six, Clubs)),
		CardNotInHand, "навес картой не из руки")
}

// Карта обязана подходить под текущую ступень шкалы жертвы.
func TestCanHangRefusesCardOffTheScale(t *testing.T) {
	ace := NewPip(Ace, Clubs)
	state := aDeal().withHand(0, ace).build()

	expectRejected(t, hangRules().CanHang(state, 0, 1, ace),
		CardNotOnNavesScale, "карта мимо ступени шкалы")
}

// Стол без навесов: навесить нельзя ничем и никому.
func TestCanHangRefusesWhenNavesAreDisabled(t *testing.T) {
	six := NewPip(Six, Clubs)
	state := aDeal().withHand(0, six).build()
	rules := NewHangingRules(DefaultRulesConfig().WithoutNaves())

	expectRejected(t, rules.CanHang(state, 0, 1, six), NavesDisabled, "навес на столе без навесов")
}

// Кривая команда с несуществующим местом — отказ, а не падение раздачи.
func TestCanHangRefusesUnknownSeat(t *testing.T) {
	state := aDeal().build()

	expectRejected(t, hangRules().CanHang(state, 9, 1, NewPip(Six, Clubs)),
		NotInHangingWindow, "навес с несуществующего места")
}
