package game

import "testing"

// Перенос DealScoringTest.
//
// Итог раздачи — §0.1, §0.3, §0.4. Считать поигроково нельзя: судьба навесившего зависит
// от судьбы того, кому он навесил, поэтому почти каждый тест здесь про взаимодействие
// двух мест, а не про одно.

var (
	aceStep   = FullNavesScale().JokerLevel() - 1
	jokerStep = FullNavesScale().JokerLevel()

	eightOfDiamonds = NewPip(Eight, Diamonds)
	eightOfHearts   = NewPip(Eight, Hearts)
	eightOfSpades   = NewPip(Eight, Spades)
	eightOfClubs    = NewPip(Eight, Clubs)
	nineOfClubs     = NewPip(Nine, Clubs)
)

func scoring() DealScoring { return NewDealScoring(DefaultRulesConfig()) }

// finishedDeal — раздача закончилась: карты остались у места 1, остальные вышли.
func finishedDeal() *dealFixture {
	return aDeal().
		withPhase(PhaseDealOver).
		withEmptyDeck().
		withOutOfDeal(0).
		withOutOfDeal(2)
}

// score считает итог и валит тест, если подсчёт вообще не состоялся.
func scoreDeal(t *testing.T, state DealState) DealOutcome {
	t.Helper()
	outcome, err := scoring().Score(state)
	if err != nil {
		t.Fatalf("подсчёт не состоялся: %v", err)
	}
	return outcome
}

func expectLevel(t *testing.T, outcome DealOutcome, seat, want int) {
	t.Helper()
	got := outcome.MustForSeat(seat).LevelAfter
	if got != want {
		t.Errorf("место %d: уровень после раздачи %d, ждали %d", seat, got, want)
	}
}

func expectDegree(t *testing.T, outcome DealOutcome, seat int, want LossDegree) {
	t.Helper()
	got := outcome.MustForSeat(seat).LossDegree
	if got != want {
		t.Errorf("место %d: степень проигрыша %s, ждали %s", seat, got, want)
	}
}

func expectChanges(t *testing.T, outcome DealOutcome, seat int, want ...LevelChange) {
	t.Helper()
	got := outcome.MustForSeat(seat).Changes
	if len(got) != len(want) {
		t.Fatalf("место %d: слагаемых сдвига %v, ждали %v", seat, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("место %d: слагаемое %d — %v, ждали %v", seat, index, got[index], want[index])
		}
	}
}

// ── Автоматические сдвиги ────────────────────────────────────────────────────

func TestDealLoserGoesOneStepUp(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withNavesLevel(1, 2).
		withExitOrder(0, 2).
		build())

	if outcome.DealLoserSeat != 1 {
		t.Errorf("проигравшим раздачу назван %d, а карты остались у места 1", outcome.DealLoserSeat)
	}
	expectLevel(t, outcome, 1, 3)
}

func TestFirstPlayerOutGoesOneStepDown(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withNavesLevel(0, 4).
		withNavesLevel(2, 4).
		withExitOrder(0, 2).
		build())

	expectLevel(t, outcome, 0, 3)
	// ⚠️ Награда только первому вышедшему: второй выходит ни с чем.
	expectLevel(t, outcome, 2, 4)
}

func TestFirstOutWithNothingHungStaysOnTheFloor(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withExitOrder(0, 2).
		build())

	expectLevel(t, outcome, 0, NoNaves)
	if shift := outcome.MustForSeat(0).Shift(); shift != 0 {
		t.Errorf("сдвиг с нижней ступени %d, а ниже «летит 6» ступеней нет", shift)
	}
}

func TestNobodyElseShifts(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withNavesLevel(2, 4).
		withExitOrder(0, 2).
		build())

	if shift := outcome.MustForSeat(2).Shift(); shift != 0 {
		t.Errorf("место 2 сдвинулось на %d, хотя ни раздачу не проиграло, ни первым не вышло", shift)
	}
}

// ── Джокер и конец матча ─────────────────────────────────────────────────────

func TestAceTurnsIntoJokerWhenTheDealIsLost(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withNavesLevel(1, aceStep).
		withExitOrder(0, 2).
		build())

	expectLevel(t, outcome, 1, jokerStep)
	// Джокер получен не картой, а через +1 за проигрыш раздачи.
	expectDegree(t, outcome, 1, LossSuperFail)
	if !outcome.IsMatchOver() {
		t.Error("матч не окончен, хотя игроку навесили джокер и первым он не вышел")
	}
}

func TestJokerIsStrippedFromThePlayerWhoWentOutFirst(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withJokerHungBy(0, 2).
		withExitOrder(0, 2).
		build())

	expectLevel(t, outcome, 0, aceStep)
	if outcome.MustForSeat(0).IsLoser() {
		t.Error("вышедший первым записан проигравшим, хотя −1 сняло с него джокер")
	}
	if outcome.IsMatchOver() {
		t.Error("матч объявлен оконченным, хотя джокер снят выходом первым")
	}
}

func TestJokerHolderWhoDidNotGoOutFirstLosesTheGame(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withJokerHungBy(2, 0).
		withExitOrder(0, 2).
		build())

	// Раздачу проиграло место 1, значит месту 2 достаётся самая лёгкая степень.
	expectDegree(t, outcome, 2, LossFail)
	if !outcome.IsMatchOver() {
		t.Error("матч не окончен, хотя джокер остался на месте 2")
	}
}

// ── Награда добившему ────────────────────────────────────────────────────────

func TestFinisherGetsOneStepDown(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withNavesLevel(0, 4).
		withJokerHungBy(2, 0).
		withExitOrder(1).
		build())

	expectLevel(t, outcome, 0, 3)
	if !outcome.MustForSeat(2).IsLoser() {
		t.Error("жертва не записана проигравшей, а без этого награда добившему беспочвенна")
	}
}

// ⭐ Corner case 3: у самого добившего в навесе джокер. Награда снимает его раньше,
// чем считаются проигравшие, и добивший из списка выпадает.
func TestFinisherWithAJokerOfTheirOwnIsSavedByTheReward(t *testing.T) {
	outcome := scoreDeal(t, aDeal().
		withPlayers(4).
		withPhase(PhaseDealOver).
		withEmptyDeck().
		withOutOfDeal(0).
		withOutOfDeal(2).
		withOutOfDeal(3).
		withJokerHungBy(0, 3).
		withJokerHungBy(2, 0).
		withExitOrder(3).
		build())

	expectLevel(t, outcome, 0, aceStep)
	if outcome.MustForSeat(0).IsLoser() {
		t.Error("добивший записан проигравшим, хотя награда за добитого сняла с него джокер")
	}
	if !outcome.MustForSeat(2).IsLoser() {
		t.Error("добитая жертва не записана проигравшей")
	}
}

func TestRewardsSumUpForEveryVictim(t *testing.T) {
	outcome := scoreDeal(t, aDeal().
		withPlayers(5).
		withPhase(PhaseDealOver).
		withEmptyDeck().
		withOutOfDeal(0).
		withOutOfDeal(2).
		withOutOfDeal(3).
		withOutOfDeal(4).
		withNavesLevel(0, 5).
		withJokerHungBy(2, 0).
		withJokerHungBy(3, 0).
		withExitOrder(4, 2, 3).
		build())

	losers := outcome.Losers()
	if len(losers) != 2 {
		t.Fatalf("проигравших %d, а джокер остался у двоих", len(losers))
	}
	seats := map[int]bool{losers[0].SeatNo: true, losers[1].SeatNo: true}
	if !seats[2] || !seats[3] {
		t.Errorf("проигравшими названы места %v, ждали 2 и 3", seats)
	}
	expectLevel(t, outcome, 0, 3)
	if shift := outcome.MustForSeat(0).Shift(); shift != -2 {
		t.Errorf("добивший двоих сдвинулся на %d, а награда даётся за каждого: ждали -2", shift)
	}
}

func TestEveryShiftOfTheDealIsSummed(t *testing.T) {
	outcome := scoreDeal(t, aDeal().
		withPlayers(4).
		withPhase(PhaseDealOver).
		withEmptyDeck().
		withOutOfDeal(0).
		withOutOfDeal(2).
		withOutOfDeal(3).
		withNavesLevel(0, 5).
		withJokerHungBy(2, 0).
		withJokerHungBy(3, 0).
		withExitOrder(0, 2, 3).
		build())

	// Вышел первым (−1) и добил двоих (−2): сдвиги складываются, а не заменяют друг друга.
	if shift := outcome.MustForSeat(0).Shift(); shift != -3 {
		t.Errorf("сдвиг %d, а вышедший первым и добивший двоих должен получить -3", shift)
	}
	expectLevel(t, outcome, 0, 2)
}

// ⭐ Нижняя граница применяется В КОНЦЕ: иначе +1 и −2 упёрлись бы в «летит 6»
// раньше времени и итог пришёл бы не туда.
func TestFloorIsAppliedOnlyAtTheEnd(t *testing.T) {
	outcome := scoreDeal(t, aDeal().
		withPlayers(4).
		withPhase(PhaseDealOver).
		withEmptyDeck().
		withOutOfDeal(0).
		withOutOfDeal(2).
		withOutOfDeal(3).
		withNavesLevel(1, 1).
		withJokerHungBy(2, 1).
		withJokerHungBy(3, 1).
		withExitOrder(0).
		build())

	if outcome.DealLoserSeat != 1 {
		t.Fatalf("проигравшим раздачу назван %d, ждали 1", outcome.DealLoserSeat)
	}
	expectLevel(t, outcome, 1, 0)
}

// ── Степени проигрыша ────────────────────────────────────────────────────────

func TestFourEightsInTheLastAttackMakeItRoyal(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withNavesLevel(1, aceStep).
		withExitOrder(0, 2).
		withLastAttack(eightOfDiamonds, eightOfHearts, eightOfSpades, eightOfClubs).
		build())

	expectDegree(t, outcome, 1, LossRoyal)
}

// ⚠️ Степень считается по СОСТАВУ ПОСЛЕДНЕЙ АТАКИ, а не по тому, что попало в руку:
// отбитые восьмёрки в руку не приходят, но степень всё равно дают.
func TestRoyalCountsBeatenEightsToo(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withNavesLevel(1, aceStep).
		withExitOrder(0, 2).
		withLastAttack(eightOfDiamonds, eightOfHearts, eightOfSpades, eightOfClubs).
		withHand(1, nineOfClubs).
		build())

	expectDegree(t, outcome, 1, LossRoyal)
}

func TestOneEightInTheLastAttackMakesItSuperMegaSuck(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withNavesLevel(1, aceStep).
		withExitOrder(0, 2).
		withLastAttack(eightOfDiamonds, nineOfClubs).
		build())

	expectDegree(t, outcome, 1, LossSuperMegaSuck)
}

func TestJokerByCardWithoutEightsMakesItSuperMegaFail(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withJokerHungBy(1, 0).
		withExitOrder(0, 2).
		withLastAttack(nineOfClubs).
		build())

	expectDegree(t, outcome, 1, LossSuperMegaFail)
}

// Джокер пришёл и картой, и через +1 за проигрыш раздачи — берётся тяжёлая степень.
func TestCardBeatsPlusOneWhenBothApply(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withJokerHungBy(1, 0).
		withExitOrder(0, 2).
		build())

	expectDegree(t, outcome, 1, LossSuperMegaFail)
}

func TestRoyalBeatsSuperFail(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withNavesLevel(1, aceStep).
		withExitOrder(0, 2).
		withLastAttack(eightOfDiamonds, eightOfHearts, eightOfSpades, eightOfClubs).
		build())

	expectDegree(t, outcome, 1, LossRoyal)
}

func TestRoyalBeatsSuperMegaFail(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withJokerHungBy(1, 0).
		withExitOrder(0, 2).
		withLastAttack(eightOfDiamonds, eightOfHearts, eightOfSpades, eightOfClubs).
		build())

	expectDegree(t, outcome, 1, LossRoyal)
}

func TestSuperMegaSuckBeatsSuperMegaFail(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withJokerHungBy(1, 0).
		withExitOrder(0, 2).
		withLastAttack(eightOfDiamonds, nineOfClubs).
		build())

	expectDegree(t, outcome, 1, LossSuperMegaSuck)
}

func TestMainLoserIsPickedByDegree(t *testing.T) {
	outcome := scoreDeal(t, aDeal().
		withPlayers(4).
		withPhase(PhaseDealOver).
		withEmptyDeck().
		withOutOfDeal(0).
		withOutOfDeal(2).
		withOutOfDeal(3).
		withNavesLevel(1, aceStep).
		withJokerHungBy(2, 0).
		withJokerHungBy(3, 0).
		withExitOrder(0).
		withLastAttack(eightOfDiamonds, eightOfHearts, eightOfSpades, eightOfClubs).
		build())

	if got := len(outcome.Losers()); got != 3 {
		t.Fatalf("проигравших %d, ждали 3: несколько проигравших — штатная ситуация", got)
	}
	main, found := outcome.MainLoser()
	if !found {
		t.Fatal("главный проигравший не найден, хотя проигравшие есть")
	}
	if main.SeatNo != 1 {
		t.Errorf("главным проигравшим названо место %d, ждали 1", main.SeatNo)
	}
	if main.LossDegree != LossRoyal {
		t.Errorf("степень главного проигравшего %s, ждали %s", main.LossDegree, LossRoyal)
	}
}

func TestLossDegreeOrderIsByDeclaration(t *testing.T) {
	if !LossRoyal.IsHeavierThan(LossFail) {
		t.Error("ROYAL не тяжелее FAIL, а степени сравниваются по порядку объявления")
	}
	if LossFail.IsHeavierThan(LossRoyal) {
		t.Error("FAIL объявлен тяжелее ROYAL — порядок степеней перевёрнут")
	}
	// ⚠️ Нулевое значение типа не должно выглядеть степенью, иначе пустой итог
	// оказался бы тяжелее любого настоящего проигрыша.
	if NoLossDegree.IsHeavierThan(LossFail) {
		t.Error("«степени нет» тяжелее FAIL — нулевое значение притворяется проигрышем")
	}
}

// ── Слагаемые сдвига и обстановка раздачи ────────────────────────────────────

// ⭐ Место 1 проиграло раздачу (+1) и добило место 2 джокером (−1): сумма нулевая,
// и по одному только уровню обе причины были бы неразличимы.
func TestShiftsAreExplainedSeparately(t *testing.T) {
	outcome := scoreDeal(t, aDeal().
		withPhase(PhaseDealOver).
		withEmptyDeck().
		withOutOfDeal(0).
		withNavesLevel(1, 3).
		withJokerHungBy(2, 1).
		withExitOrder(0).
		build())

	if shift := outcome.MustForSeat(1).Shift(); shift != 0 {
		t.Errorf("итоговый сдвиг %d, ждали 0: +1 за раздачу и −1 за добитого", shift)
	}
	expectChanges(t, outcome, 1,
		LevelChange{Reason: LostDeal, Amount: 1},
		LevelChange{Reason: FinishedOpponent, Amount: -1})
}

// «Летит 6» — нижняя ступень: −1 отсюда некуда, и это должно быть видно в истории.
func TestScaleLimitIsRecordedWhenTheFloorSwallowsAStep(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withExitOrder(0, 2).
		build())

	expectChanges(t, outcome, 0,
		LevelChange{Reason: FirstOut, Amount: -1},
		LevelChange{Reason: ScaleLimit, Amount: 1})
}

func TestPlacesFollowTheExitOrder(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withExitOrder(2, 0).
		build())

	for seat, want := range map[int]int{2: 1, 0: 2, 1: 3} {
		if got := outcome.MustForSeat(seat).Place; got != want {
			t.Errorf("место %d заняло %d-е место в раздаче, ждали %d-е", seat, got, want)
		}
	}
}

// ⭐ После подсчёта раздача исчезает, поэтому обстановку обязан нести итог.
func TestOutcomeCarriesTheLastAttackAndTheTrump(t *testing.T) {
	outcome := scoreDeal(t, finishedDeal().
		withExitOrder(0, 2).
		withLastAttack(eightOfDiamonds, nineOfClubs).
		build())

	if len(outcome.LastAttackCards) != 2 ||
		outcome.LastAttackCards[0] != Card(eightOfDiamonds) ||
		outcome.LastAttackCards[1] != Card(nineOfClubs) {
		t.Errorf("последняя атака в итоге %v, а без неё степени уже не пересчитать",
			outcome.LastAttackCards)
	}
	if outcome.TrumpSuit == nil {
		t.Error("козырь потерян: восстановить раздачу по истории будет нечем")
	}
}

// ⚠️ Итог обязан отвязаться от раздачи: иначе следующая сдача перепишет историю
// через общий срез.
func TestOutcomeDoesNotShareSlicesWithTheDeal(t *testing.T) {
	state := finishedDeal().
		withExitOrder(0, 2).
		withLastAttack(eightOfDiamonds, nineOfClubs).
		build()
	outcome := scoreDeal(t, state)

	state.LastAttackCards[0] = eightOfClubs
	if outcome.LastAttackCards[0] != Card(eightOfDiamonds) {
		t.Error("правка раздачи изменила уже посчитанный итог — срез не скопирован")
	}
}

func TestScoringWithoutALoserFails(t *testing.T) {
	state := finishedDeal().withOutOfDeal(1).withExitOrder(0, 2, 1).build()

	if _, err := scoring().Score(state); err == nil {
		t.Error("подсчёт прошёл без проигравшего, хотя раздача так закончиться не может")
	}
}
