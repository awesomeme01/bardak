package game

import "testing"

// Перенос CardRankMatchingTest. Совпадение по рангу — то, что разрешает подкинуть
// и перевести; ошибка здесь ломает сразу две механики.

func TestSameRankIgnoresSuit(t *testing.T) {
	seven := NewPip(Seven, Diamonds)

	if !seven.SameRankAs(NewPip(Seven, Clubs)) {
		t.Error("семёрки разных мастей обязаны совпадать по рангу")
	}
	if seven.SameRankAs(NewPip(Eight, Diamonds)) {
		t.Error("разные ранги одной масти совпадать не должны")
	}
}

func TestJokerMatchesOnlyJoker(t *testing.T) {
	joker := MustJoker(1)

	if !joker.SameRankAs(MustJoker(2)) {
		t.Error("джокер обязан совпадать с джокером — на этом держится крытие джокера")
	}
	if joker.SameRankAs(NewPip(Ace, Spades)) {
		t.Error("джокер не совпадает с тузом: ранг джокера — «джокер»")
	}
}

// Ни один обычный ранг не совпадает с джокером. В Java это @ParameterizedTest
// по всем рангам — здесь табличный тест, один к одному по смыслу.
func TestNoPlainRankMatchesJoker(t *testing.T) {
	for _, rank := range Ranks() {
		plain := NewPip(rank, Hearts)
		if plain.SameRankAs(MustJoker(1)) {
			t.Errorf("ранг %s не должен совпадать с джокером", rank.Code())
		}
	}
}

// ⚠️ Симметрия не подразумевается сама собой: сравнение реализовано в двух разных типах,
// и разъехаться они могут независимо. Тогда «подкинуть можно, а перевести нельзя»
// зависело бы от того, с какой стороны сравнили.
func TestMatchingIsSymmetric(t *testing.T) {
	jack := NewPip(Jack, Spades)
	joker := MustJoker(4)

	if jack.SameRankAs(joker) != joker.SameRankAs(jack) {
		t.Error("сравнение обязано быть симметричным в обе стороны")
	}

	seven := NewPip(Seven, Hearts)
	other := NewPip(Seven, Clubs)
	if seven.SameRankAs(other) != other.SameRankAs(seven) {
		t.Error("симметрия нарушена и для обычных карт")
	}
}

func TestJokerNumberBelowOneIsRefused(t *testing.T) {
	if _, err := NewJoker(0); err == nil {
		t.Error("номер джокера начинается с единицы")
	}
	if _, err := NewJoker(-1); err == nil {
		t.Error("отрицательный номер джокера недопустим")
	}
	if _, err := NewJoker(1); err != nil {
		t.Errorf("единица — корректный номер: %v", err)
	}
}

func TestCardCodes(t *testing.T) {
	cases := []struct {
		card Card
		want string
	}{
		{NewPip(Ten, Hearts), "10♥"},
		{NewPip(Ace, Spades), "A♠"},
		{NewPip(Six, Diamonds), "6♦"},
		{NewPip(King, Clubs), "K♣"},
		{MustJoker(3), "Joker-3"},
	}

	for _, c := range cases {
		if got := c.card.Code(); got != c.want {
			t.Errorf("код карты: получили %q, ждали %q", got, c.want)
		}
	}
}

// Старшинство рангов задано порядком объявления и осмысленно только внутри масти.
func TestRankOrdering(t *testing.T) {
	if !Ace.IsHigherThan(King) {
		t.Error("туз старше короля")
	}
	if Six.IsHigherThan(Seven) {
		t.Error("шестёрка не старше семёрки")
	}
	if Seven.IsHigherThan(Seven) {
		t.Error("сравнение строгое: ранг не старше самого себя")
	}

	ranks := Ranks()
	for i := 1; i < len(ranks); i++ {
		if !ranks[i].IsHigherThan(ranks[i-1]) {
			t.Fatalf("шкала рангов сломана на %s → %s", ranks[i-1].Code(), ranks[i].Code())
		}
	}
	if len(ranks) != 9 {
		t.Errorf("рангов должно быть 9 (6..A), получили %d", len(ranks))
	}
}
