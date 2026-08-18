package game

import "testing"

// Перенос DeckFactoryTest.

const plainCards = 36

// В Java это @ParameterizedTest по 2..5 игрокам — здесь табличный тест.
func TestDeckHoldsPlainCardsPlusJokerPerPlayer(t *testing.T) {
	for players := MinPlayers; players <= MaxPlayers; players++ {
		deck, err := BuildOrderedDeck(players)
		if err != nil {
			t.Fatalf("%d игроков: %v", players, err)
		}

		if len(deck) != plainCards+players {
			t.Errorf("%d игроков: карт %d, ждали %d", players, len(deck), plainCards+players)
		}

		jokers, pips := 0, 0
		for _, card := range deck {
			switch card.(type) {
			case Joker:
				jokers++
			case Pip:
				pips++
			}
		}
		if jokers != players {
			t.Errorf("%d игроков: джокеров %d, ждали по одному на игрока", players, jokers)
		}
		if pips != plainCards {
			t.Errorf("%d игроков: обычных карт %d, ждали %d", players, pips, plainCards)
		}
	}
}

func TestDeckHoldsEveryRankAndSuitOnce(t *testing.T) {
	deck, err := BuildOrderedDeck(4)
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]int, len(deck))
	for _, card := range deck {
		seen[card.Code()]++
	}
	for code, count := range seen {
		if count != 1 {
			t.Errorf("карта %s встречается %d раз — в колоде дубликат", code, count)
		}
	}

	for _, suit := range Suits() {
		for _, rank := range Ranks() {
			if seen[NewPip(rank, suit).Code()] != 1 {
				t.Errorf("в колоде нет %s%s", rank.Code(), suit.Symbol())
			}
		}
	}
}

func TestJokersAreNumberedFromOne(t *testing.T) {
	deck, err := BuildOrderedDeck(5)
	if err != nil {
		t.Fatal(err)
	}

	numbers := map[int]bool{}
	for _, card := range deck {
		if joker, ok := card.(Joker); ok {
			numbers[joker.Number] = true
		}
	}
	for expected := 1; expected <= 5; expected++ {
		if !numbers[expected] {
			t.Errorf("нет джокера с номером %d", expected)
		}
	}
	if len(numbers) != 5 {
		t.Errorf("номеров джокеров %d, ждали 5", len(numbers))
	}
}

// ⭐ Один seed — одна колода. Без этого нельзя воспроизвести партию в тесте,
// а «плавающий» баг правил ловится раз в сто прогонов и не чинится.
func TestSameSeedGivesSameOrder(t *testing.T) {
	first, err := BuildShuffledDeck(4, SeededShuffler{Seed: 42})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildShuffledDeck(4, SeededShuffler{Seed: 42})
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != len(second) {
		t.Fatalf("длины разошлись: %d и %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Code() != second[i].Code() {
			t.Fatalf("позиция %d: %s против %s — один seed дал разный порядок",
				i, first[i].Code(), second[i].Code())
		}
	}
}

// Разные seed дают другой порядок, но тот же СОСТАВ: тасовка не теряет и не добавляет карт.
func TestDifferentSeedKeepsSameCards(t *testing.T) {
	first, err := BuildShuffledDeck(4, SeededShuffler{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildShuffledDeck(4, SeededShuffler{Seed: 2})
	if err != nil {
		t.Fatal(err)
	}

	countOf := func(deck []Card) map[string]int {
		out := map[string]int{}
		for _, card := range deck {
			out[card.Code()]++
		}
		return out
	}

	firstCounts, secondCounts := countOf(first), countOf(second)
	if len(firstCounts) != len(secondCounts) {
		t.Fatal("состав колод разошёлся")
	}
	for code, count := range firstCounts {
		if secondCounts[code] != count {
			t.Errorf("карта %s: %d против %d", code, count, secondCounts[code])
		}
	}

	same := true
	for i := range first {
		if first[i].Code() != second[i].Code() {
			same = false
			break
		}
	}
	if same {
		t.Error("разные seed дали идентичный порядок — тасовка не работает")
	}
}

func TestPlayerCountBounds(t *testing.T) {
	for _, bad := range []int{0, 1, 6, -1} {
		if _, err := BuildOrderedDeck(bad); err == nil {
			t.Errorf("%d игроков должно отвергаться", bad)
		}
	}
	for good := MinPlayers; good <= MaxPlayers; good++ {
		if _, err := BuildOrderedDeck(good); err != nil {
			t.Errorf("%d игроков — допустимый стол: %v", good, err)
		}
	}
}
