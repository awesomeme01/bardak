// Package game — правила бардака: модель, состояния и переходы.
//
// ⭐ Пакет НЕ знает про HTTP, WebSocket, PostgreSQL и любой фреймворк. Это не стилевое
// пожелание: правила игры — единственное, что нельзя починить в проде «на живую», и
// проверяться они должны без сети и без базы, за миллисекунды.
//
// Перенос с Java (`kz.bardak.game.rules`). Где Go вынуждает делать иначе, чем Java,
// это отмечено в комментарии — расхождение в механике не должно превратиться
// в расхождение в поведении.
package game

import "fmt"

// Suit — масть обычной карты.
//
// Порядок объявления значения не имеет: масти между собой не сравниваются, старшинство
// существует только внутри одной масти.
type Suit uint8

const (
	Diamonds Suit = iota
	Hearts
	Spades
	Clubs
)

var suitSymbols = [...]string{"♦", "♥", "♠", "♣"}

// Symbol — знак масти для кода карты и логов.
func (s Suit) Symbol() string {
	if int(s) >= len(suitSymbols) {
		return "?"
	}
	return suitSymbols[s]
}

func (s Suit) String() string { return s.Symbol() }

// Suits — все масти в порядке объявления. Нужен колоде, чтобы собирать её перебором,
// а не перечислением 36 карт руками.
func Suits() []Suit { return []Suit{Diamonds, Hearts, Spades, Clubs} }

// Rank — ранг обычной карты. Старшинство задано порядком: 6 < 7 < … < A.
//
// ⚠️ Джокер рангом НЕ является: у него собственный ранг вне этой шкалы.
type Rank uint8

const (
	Six Rank = iota
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
)

var rankCodes = [...]string{"6", "7", "8", "9", "10", "J", "Q", "K", "A"}

// Code — короткий код ранга.
func (r Rank) Code() string {
	if int(r) >= len(rankCodes) {
		return "?"
	}
	return rankCodes[r]
}

func (r Rank) String() string { return r.Code() }

// IsHigherThan — строго старше другого ранга.
//
// Сравнение осмысленно только внутри одной масти: межмастевое старшинство определяет козырь.
func (r Rank) IsHigherThan(other Rank) bool { return r > other }

// Ranks — все ранги от младшего к старшему.
func Ranks() []Rank {
	return []Rank{Six, Seven, Eight, Nine, Ten, Jack, Queen, King, Ace}
}

// Card — карта колоды: либо обычная (Pip), либо джокер (Joker).
//
// ⭐ Иерархия закрыта неэкспортируемым методом sealed(): снаружи пакета свой вид карты
// не объявить. В Java это делал `sealed interface`, здесь — соглашение, которое держит
// компилятор. Без него правила однажды получили бы карту, которую не умеют разбирать.
type Card interface {
	// SameRankAs — совпадение по рангу: то, что разрешает подкинуть карту к лежащим
	// на столе и перевести атаку.
	//
	// ⭐ Ранг джокера — «джокер»: джокер совпадает только с джокером, обычная карта —
	// только с обычной того же ранга. Масть в сравнении не участвует.
	SameRankAs(other Card) bool

	// Code — человекочитаемый код: «10♥», «Joker-3». Для логов и тестов, не для протокола.
	Code() string

	sealed()
}

// Pip — обычная карта. Таких в колоде 36 при любом числе игроков.
type Pip struct {
	Rank Rank
	Suit Suit
}

// NewPip собирает обычную карту.
func NewPip(rank Rank, suit Suit) Pip { return Pip{Rank: rank, Suit: suit} }

// SameRankAs — обычная карта совпадает только с обычной того же ранга.
func (p Pip) SameRankAs(other Card) bool {
	pip, ok := other.(Pip)
	return ok && pip.Rank == p.Rank
}

// Code — «10♥».
func (p Pip) Code() string { return p.Rank.Code() + p.Suit.Symbol() }

func (p Pip) String() string { return p.Code() }

func (Pip) sealed() {}

// Joker — джокер. В колоде их ровно по одному на игрока.
//
// Number различает экземпляры для лога и картинки, но на правила не влияет:
// старшинства между джокерами нет.
type Joker struct {
	Number int
}

// NewJoker собирает джокер. Номера начинаются с единицы.
func NewJoker(number int) (Joker, error) {
	if number < 1 {
		return Joker{}, fmt.Errorf("номер джокера начинается с 1, получен: %d", number)
	}
	return Joker{Number: number}, nil
}

// MustJoker — для тестов и сборки колоды, где номер заведомо корректен.
func MustJoker(number int) Joker {
	joker, err := NewJoker(number)
	if err != nil {
		panic(err)
	}
	return joker
}

// SameRankAs — джокер совпадает с любым джокером и ни с чем больше.
func (j Joker) SameRankAs(other Card) bool {
	_, ok := other.(Joker)
	return ok
}

// Code — «Joker-3».
func (j Joker) Code() string { return fmt.Sprintf("Joker-%d", j.Number) }

func (j Joker) String() string { return j.Code() }

func (Joker) sealed() {}
