package game

import (
	"reflect"
	"testing"
)

// Сдача раздачи: руки, скрытые карты, козырь, потайной козырь и кость.
//
// Перенос DealerTest и той части HiddenTrumpTest, которая относится к сдаче: остальное
// в HiddenTrumpTest проверяет уже движок (вскрытие потайного козыря при доборе).

var dealerConfig = DefaultRulesConfig()

func newTestDealer() Dealer { return NewDealer(dealerConfig, SeededDice{}) }

// freshLevels — стол, на котором никому ещё ничего не навешивали.
func freshLevels(playerCount int) []int {
	levels := make([]int, playerCount)
	for seat := range levels {
		levels[seat] = NoNaves
	}
	return levels
}

func mustStartDeal(t *testing.T, dealer Dealer, levels []int, seed int64) DealState {
	t.Helper()
	deal, err := dealer.StartDeal(levels, seed)
	if err != nil {
		t.Fatalf("сдача с seed %d не удалась: %v", seed, err)
	}
	return deal
}

// firstDealMatching — первый seed, при котором нижняя карта нужного вида. Джокер внизу
// случается редко, поэтому его приходится искать перебором, как и в Java-тесте.
func firstDealMatching(t *testing.T, jokerAtTheBottom bool) DealState {
	t.Helper()
	dealer := newTestDealer()
	for seed := int64(1); seed < 200; seed++ {
		deal := mustStartDeal(t, dealer, freshLevels(4), seed)
		if deal.HasTrump() != jokerAtTheBottom {
			return deal
		}
	}
	t.Fatalf("не нашлось seed с нужной нижней картой (джокер внизу: %v)", jokerAtTheBottom)
	return DealState{}
}

func TestDealerGivesEveryPlayerAHandAndOneFaceDownCard(t *testing.T) {
	dealer := newTestDealer()
	for _, playerCount := range []int{2, 3, 4, 5} {
		deal := mustStartDeal(t, dealer, freshLevels(playerCount), 7)

		if len(deal.Players) != playerCount {
			t.Fatalf("за столом %d игроков вместо %d", len(deal.Players), playerCount)
		}
		for _, player := range deal.Players {
			if player.HandSize() != dealerConfig.DealSize {
				t.Errorf("место %d: в руке %d карт вместо %d — раздача начата с неверной рукой",
					player.SeatNo, player.HandSize(), dealerConfig.DealSize)
			}
			if !player.HasFaceDownCard() {
				t.Errorf("место %d: нет скрытой карты — играть последнюю карту будет нечем",
					player.SeatNo)
			}
			if !player.InDeal {
				t.Errorf("место %d: игрок вне раздачи сразу после сдачи", player.SeatNo)
			}
		}
	}
}

func TestDealerLeavesTheRestOfTheDeckForDrawing(t *testing.T) {
	const playerCount = 4
	deal := mustStartDeal(t, newTestDealer(), freshLevels(playerCount), 7)

	ordered, err := BuildOrderedDeck(playerCount)
	if err != nil {
		t.Fatalf("колода на %d игроков не собралась: %v", playerCount, err)
	}
	dealt := playerCount * (dealerConfig.DealSize + 1)
	if len(deal.Deck) != len(ordered)-dealt {
		t.Fatalf("в колоде осталось %d карт вместо %d — добирать будет не из чего",
			len(deal.Deck), len(ordered)-dealt)
	}
}

func TestDealerHandsOutEveryCardExactlyOnce(t *testing.T) {
	const playerCount = 3
	deal := mustStartDeal(t, newTestDealer(), freshLevels(playerCount), 11)

	seen := make(map[Card]int)
	for _, card := range deal.Deck {
		seen[card]++
	}
	for _, player := range deal.Players {
		for _, card := range player.Hand {
			seen[card]++
		}
		if player.HasFaceDownCard() {
			seen[player.FaceDownCard]++
		}
	}

	ordered, err := BuildOrderedDeck(playerCount)
	if err != nil {
		t.Fatalf("колода на %d игроков не собралась: %v", playerCount, err)
	}
	if len(seen) != len(ordered) {
		t.Fatalf("после сдачи в игре %d различных карт вместо %d", len(seen), len(ordered))
	}
	for _, card := range ordered {
		if seen[card] != 1 {
			t.Errorf("карта %s встречается %d раз — сдача теряет или дублирует карты",
				card.Code(), seen[card])
		}
	}
}

func TestDealerCarriesTheNavesLevelsIntoTheDeal(t *testing.T) {
	levels := []int{0, 4, NoNaves}

	deal := mustStartDeal(t, newTestDealer(), levels, 7)

	for seat, expected := range levels {
		player := deal.MustPlayerAt(seat)
		if player.NavesLevel != expected {
			t.Errorf("место %d: уровень навесов %d вместо %d — шкала должна переживать раздачу",
				seat, player.NavesLevel, expected)
		}
		if len(player.HungCards) != 0 {
			t.Errorf("место %d: навешенные карты не сбросились при новой раздаче", seat)
		}
	}
}

func TestDealerDealsTheSameCardsForTheSameSeed(t *testing.T) {
	dealer := newTestDealer()
	const seed = int64(20260810)

	first := mustStartDeal(t, dealer, freshLevels(4), seed)
	second := mustStartDeal(t, dealer, freshLevels(4), seed)

	if !reflect.DeepEqual(first.Players, second.Players) {
		t.Errorf("один seed дал разные руки — реплей и восстановление матча разойдутся")
	}
	if !reflect.DeepEqual(first.Deck, second.Deck) {
		t.Errorf("один seed дал разный остаток колоды")
	}
	if !reflect.DeepEqual(first.Trump, second.Trump) {
		t.Errorf("один seed дал разный козырь: %v и %v", first.Trump, second.Trump)
	}
}

func TestDealerGivesTheFirstMoveToTheLowestTrump(t *testing.T) {
	deal := firstDealMatching(t, false)

	starterLowest, starterHasTrump := lowestTrumpRank(deal.MustPlayerAt(deal.RoundStarterSeat), *deal.Trump)
	if !starterHasTrump {
		t.Fatalf("первым ходит место %d без единого козыря", deal.RoundStarterSeat)
	}
	for _, player := range deal.Players {
		candidate, hasTrump := lowestTrumpRank(player, *deal.Trump)
		if hasTrump && starterLowest.IsHigherThan(candidate) {
			t.Errorf("у места %d козырь %s младше, чем %s у начавшего место %d",
				player.SeatNo, candidate.Code(), starterLowest.Code(), deal.RoundStarterSeat)
		}
	}
	expectedDefender := (deal.RoundStarterSeat + 1) % len(deal.Players)
	if deal.DefenderSeat != expectedDefender {
		t.Errorf("отбивается место %d вместо соседа %d начавшего раунд",
			deal.DefenderSeat, expectedDefender)
	}
	if deal.AttackRightSeat != deal.RoundStarterSeat {
		t.Errorf("право атаки у места %d, а раунд начало место %d",
			deal.AttackRightSeat, deal.RoundStarterSeat)
	}
	if deal.Phase != PhaseAttack {
		t.Errorf("фаза %s вместо %s — козырь известен, играть можно сразу",
			deal.Phase, PhaseAttack)
	}
}

func TestDealerAlwaysLeavesATrumpInSomebodysHand(t *testing.T) {
	dealer := newTestDealer()
	for _, playerCount := range []int{2, 3, 4, 5} {
		for seed := int64(1); seed <= 60; seed++ {
			deal := mustStartDeal(t, dealer, freshLevels(playerCount), seed)
			if !deal.HasTrump() {
				continue
			}
			if !dealer.HasAnyTrumpInHands(deal) {
				t.Errorf("игроков %d, seed %d: козыря нет ни у кого, раздача не пересдалась — "+
					"первый ход определять не из чего", playerCount, seed)
			}
		}
	}
}

func TestDealerReshufflesDeterministically(t *testing.T) {
	dealer := newTestDealer()
	const seed = int64(3)

	first := mustStartDeal(t, dealer, freshLevels(2), seed)
	second := mustStartDeal(t, dealer, freshLevels(2), seed)

	if !reflect.DeepEqual(first.Players, second.Players) {
		t.Errorf("пересдача от одного seed дала разные руки — воспроизводимость потеряна")
	}
}

func TestReshuffleSeedStaysDerivedFromTheDealSeed(t *testing.T) {
	dealer := newTestDealer()

	if got := dealer.ReshuffleSeed(3, 0); got != 94 {
		t.Errorf("seed пересдачи %d вместо 94 — правило вывода изменилось", got)
	}
	if dealer.ReshuffleSeed(3, 0) == dealer.ReshuffleSeed(3, 1) {
		t.Errorf("две пересдачи подряд дают один seed — расклад повторится и цикл не кончится")
	}
	if dealer.ReshuffleSeed(3, 0) != dealer.ReshuffleSeed(3, 0) {
		t.Errorf("seed пересдачи не воспроизводится")
	}
}

func TestDealerOpensTheDicePhaseWhenTheTrumpCardIsAJoker(t *testing.T) {
	deal := firstDealMatching(t, true)

	if deal.Phase != PhaseDice {
		t.Errorf("фаза %s вместо %s — масть козыря ещё не названа", deal.Phase, PhaseDice)
	}
	if deal.HasTrump() {
		t.Errorf("козырь уже назван, хотя нижней картой лежит джокер")
	}
	if _, isJoker := deal.Deck[len(deal.Deck)-2].(Joker); !isJoker {
		t.Errorf("козырной картой оказался %s, а фаза DICE открывается только под джокер",
			deal.Deck[len(deal.Deck)-2].Code())
	}
	if deal.DiceRolls != 1 {
		t.Errorf("бросков кости учтено %d вместо 1 — следующий спор повторит этот бросок",
			deal.DiceRolls)
	}
	if deal.AttackRightSeat < 0 || deal.AttackRightSeat >= len(deal.Players) {
		t.Errorf("масть называет место %d, которого нет за столом", deal.AttackRightSeat)
	}
	// ⭐ Ожидание масти при сдаче — это НЕ потайной козырь: тот всплывает только в доборе.
	if deal.PendingHiddenTrump != nil {
		t.Errorf("при сдаче отложен потайной козырь, хотя колода ещё полна")
	}
}

func TestDealerKeepsTheHiddenTrumpUnderTheTrumpCard(t *testing.T) {
	deal := firstDealMatching(t, false)

	if len(deal.Deck) < 2 {
		t.Fatalf("в колоде %d карт — потайному козырю негде лежать", len(deal.Deck))
	}
	trumpCard, isPip := deal.Deck[len(deal.Deck)-2].(Pip)
	if !isPip {
		t.Fatalf("предпоследней картой лежит %s, а козырь определяет обычная карта",
			deal.Deck[len(deal.Deck)-2].Code())
	}
	if trumpCard.Suit != deal.Trump.Suit {
		t.Errorf("козырь %s не совпадает с мастью козырной карты %s",
			deal.Trump.Suit, trumpCard.Suit)
	}
	// Самая нижняя карта остаётся тайной: её никто не видел и в руку она не ушла.
	if deal.PendingHiddenTrump != nil {
		t.Errorf("потайной козырь отложен до добора — его вскрыли слишком рано")
	}
}

func TestDealerRefusesATableThatCannotBeDealt(t *testing.T) {
	dealer := newTestDealer()

	for _, playerCount := range []int{0, 1, 6} {
		if _, err := dealer.StartDeal(freshLevels(playerCount), 7); err == nil {
			t.Errorf("сдача на %d игроков прошла молча, хотя такого стола не бывает", playerCount)
		}
	}
}

func TestDealerRefusesADealSizeThatEatsTheTrumpCard(t *testing.T) {
	// ⚠️ Восьмерым картам на руку при пяти игроках колоды хватает, а вот на козырную
	// и потайную карты — уже нет. Молчаливая сдача здесь стоила бы паники в движке.
	config := DefaultRulesConfig()
	config.DealSize = 8
	dealer := NewDealer(config, SeededDice{})

	if _, err := dealer.StartDeal(freshLevels(5), 7); err == nil {
		t.Errorf("сдача по 8 карт впятером прошла, хотя под козырь карт не остаётся")
	}
}

func TestSeededDiceIsDeterministic(t *testing.T) {
	dice := SeededDice{}
	participants := []int{0, 1, 2, 3}

	first, err := dice.WinnerAmong(participants, 42, 1)
	if err != nil {
		t.Fatalf("бросок не удался: %v", err)
	}
	second, err := dice.WinnerAmong(participants, 42, 1)
	if err != nil {
		t.Fatalf("повторный бросок не удался: %v", err)
	}
	if first != second {
		t.Errorf("один и тот же бросок дал места %d и %d — спор нельзя будет воспроизвести",
			first, second)
	}
}

func TestSeededDicePicksOnlyAmongTheParticipants(t *testing.T) {
	dice := SeededDice{}
	participants := []int{2, 4}

	winners := make(map[int]int)
	for seed := int64(0); seed < 200; seed++ {
		winner, err := dice.WinnerAmong(participants, seed, 0)
		if err != nil {
			t.Fatalf("бросок с seed %d не удался: %v", seed, err)
		}
		if winner != 2 && winner != 4 {
			t.Fatalf("кость выбрала место %d, которого нет среди спорящих", winner)
		}
		winners[winner]++
	}
	for _, seat := range participants {
		if winners[seat] == 0 {
			t.Errorf("место %d не выиграло ни одного из 200 бросков — кость смещена", seat)
		}
	}
}

func TestSeededDiceSeparatesConsecutiveRolls(t *testing.T) {
	dice := SeededDice{}
	participants := []int{0, 1, 2}

	differed := 0
	for seed := int64(0); seed < 100; seed++ {
		first, _ := dice.WinnerAmong(participants, seed, 0)
		second, _ := dice.WinnerAmong(participants, seed, 1)
		if first != second {
			differed++
		}
	}
	// Два спора в одной раздаче обязаны разыгрываться независимо: совпадения допустимы
	// (их и в жизни примерно треть), а вот полное повторение означало бы, что номер
	// броска в seed не участвует.
	if differed == 0 {
		t.Errorf("второй бросок раздачи всегда повторяет первый — номер броска не влияет на кость")
	}
}

func TestSeededDiceRefusesADisputeWithoutParticipants(t *testing.T) {
	if _, err := (SeededDice{}).WinnerAmong(nil, 42, 0); err == nil {
		t.Errorf("бросок без спорящих вернул победителя вместо ошибки")
	}
}
