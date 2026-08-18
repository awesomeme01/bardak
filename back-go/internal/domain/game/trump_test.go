package game

import "testing"

// Перенос TrumpTest. Здесь бардак расходится с обычным дураком, и каждая проверка
// закрывает одно из расхождений.

func TestBeatsInsideOneSuitByRank(t *testing.T) {
	trump := NewTrump(Hearts)

	if !trump.Beats(NewPip(Nine, Diamonds), NewPip(Seven, Diamonds)) {
		t.Error("девятка бубён обязана бить семёрку бубён")
	}
	if trump.Beats(NewPip(Seven, Diamonds), NewPip(Nine, Diamonds)) {
		t.Error("семёрка не бьёт девятку той же масти")
	}
	if trump.Beats(NewPip(Seven, Diamonds), NewPip(Seven, Diamonds)) {
		t.Error("равный ранг не бьёт: сравнение строгое")
	}
}

func TestDoesNotBeatAnotherSuitWithoutTrump(t *testing.T) {
	trump := NewTrump(Hearts)

	// Ни та, ни другая не козырь и не защищённая масть — взять нечем.
	if trump.Beats(NewPip(Ace, Diamonds), NewPip(Six, Clubs)) {
		t.Error("туз бубён не бьёт шестёрку треф: масти разные, козыря нет")
	}
}

func TestLowestTrumpBeatsPlainCard(t *testing.T) {
	trump := NewTrump(Hearts)

	if !trump.Beats(NewPip(Six, Hearts), NewPip(Ace, Diamonds)) {
		t.Error("младший козырь обязан бить старшую некозырную")
	}
	if trump.Beats(NewPip(Ace, Diamonds), NewPip(Six, Hearts)) {
		t.Error("некозырная не бьёт козырь, каким бы старшим ни была")
	}
}

// ⭐ Главное отличие бардака: козырь НЕ берёт защищённую масть.
func TestProtectedSuitResistsTrump(t *testing.T) {
	trump := NewTrump(Hearts) // защищённая масть — пики

	if trump.ProtectedSuit() != Spades {
		t.Fatalf("при козыре черви защищённая масть должна быть пиками, получили %s",
			trump.ProtectedSuit())
	}
	if trump.Beats(NewPip(Ace, Hearts), NewPip(Six, Spades)) {
		t.Error("козырный туз не должен брать шестёрку пик — пики защищены")
	}
	// Отбиться от защищённой масти можно только старшей той же масти или джокером.
	if !trump.Beats(NewPip(Seven, Spades), NewPip(Six, Spades)) {
		t.Error("старшая пика обязана бить младшую пику")
	}
	if !trump.Beats(MustJoker(1), NewPip(Six, Spades)) {
		t.Error("джокер кроет и защищённую масть")
	}
}

// Если козырь сам пики — защищённой становится трефа, и пики теряют защиту.
func TestProtectionMovesToClubsWhenTrumpIsSpades(t *testing.T) {
	trump := NewTrump(Spades)

	if trump.ProtectedSuit() != Clubs {
		t.Fatalf("при козырных пиках защищённая масть — трефы, получили %s", trump.ProtectedSuit())
	}
	if trump.Beats(NewPip(Ace, Spades), NewPip(Six, Clubs)) {
		t.Error("козырные пики не берут трефу: теперь защищена она")
	}
	// А сами пики защиту потеряли — их берёт козырь, то есть они же.
	if !trump.Beats(NewPip(Seven, Spades), NewPip(Six, Diamonds)) {
		t.Error("козырные пики обязаны бить некозырную бубну")
	}
}

// Защищённая масть есть всегда, ровно одна, и никогда не совпадает с козырем.
func TestAlwaysExactlyOneProtectedSuit(t *testing.T) {
	for _, suit := range Suits() {
		trump := NewTrump(suit)
		protected := trump.ProtectedSuit()

		if protected == suit {
			t.Errorf("козырь %s: защищённая масть совпала с козырем — защищать было бы нечего", suit)
		}
		if protected != Spades && protected != Clubs {
			t.Errorf("козырь %s: защищённой оказалась %s, ждали пики или трефы", suit, protected)
		}
	}
}

func TestJokerDefenceBeatsEverything(t *testing.T) {
	trump := NewTrump(Diamonds)
	joker := MustJoker(2)

	for _, suit := range Suits() {
		for _, rank := range Ranks() {
			if !trump.Beats(joker, NewPip(rank, suit)) {
				t.Fatalf("джокер обязан крыть %s%s", rank.Code(), suit.Symbol())
			}
		}
	}
	if !trump.Beats(joker, MustJoker(1)) {
		t.Error("джокер кроет джокера")
	}
}

func TestJokerAttackIsUnbeatableByPlainCards(t *testing.T) {
	trump := NewTrump(Diamonds)
	joker := MustJoker(3)

	for _, suit := range Suits() {
		for _, rank := range Ranks() {
			if trump.Beats(NewPip(rank, suit), joker) {
				t.Fatalf("%s%s не должна брать джокер", rank.Code(), suit.Symbol())
			}
		}
	}
}
