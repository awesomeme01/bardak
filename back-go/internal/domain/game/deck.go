package game

import (
	"fmt"
	"math/rand/v2"
)

// Границы стола. Переедут в конфигурацию правил, когда она появится.
const (
	MinPlayers = 2
	MaxPlayers = 5
)

// Shuffler тасует колоду.
//
// ⭐ Внедряется параметром, а не берётся глобально. Тест обязан быть воспроизводимым:
// «плавающий» баг правил, который ловится раз в сто прогонов, без этого не поймать вовсе.
type Shuffler interface {
	Shuffle(n int, swap func(i, j int))
}

// SeededShuffler — тасовка от seed. Один seed — одна колода.
//
// ⚠️ Совпадения с `java.util.Random` НЕТ и не требуется (MD-005): восстановление матча
// идёт из полного снимка, а реплей — из записанных событий, поэтому колода из seed
// заново нигде не выводится. Требуется только детерминизм внутри Go.
type SeededShuffler struct{ Seed int64 }

// Shuffle перемешивает n элементов, вызывая swap.
func (s SeededShuffler) Shuffle(n int, swap func(i, j int)) {
	source := rand.NewPCG(uint64(s.Seed), uint64(s.Seed)>>32^0x9e3779b97f4a7c15)
	rand.New(source).Shuffle(n, swap)
}

// BuildOrderedDeck — колода в каноническом порядке: 36 обычных карт по мастям и рангам,
// затем по одному джокеру на игрока.
//
// ⭐ Состав не зависит от числа игроков сверх этого: 36 + N при любом столе. Короткая
// раздача на пятерых — штатный сценарий, а не вырождение.
//
// Играть полагается перемешанной; этот порядок нужен тестам и проверке состава.
func BuildOrderedDeck(playerCount int) ([]Card, error) {
	if err := validatePlayerCount(playerCount); err != nil {
		return nil, err
	}

	cards := make([]Card, 0, 36+playerCount)
	for _, suit := range Suits() {
		for _, rank := range Ranks() {
			cards = append(cards, NewPip(rank, suit))
		}
	}
	for number := 1; number <= playerCount; number++ {
		cards = append(cards, MustJoker(number))
	}
	return cards, nil
}

// BuildShuffledDeck — колода, перемешанная переданным тасовщиком.
func BuildShuffledDeck(playerCount int, shuffler Shuffler) ([]Card, error) {
	cards, err := BuildOrderedDeck(playerCount)
	if err != nil {
		return nil, err
	}
	if shuffler != nil {
		shuffler.Shuffle(len(cards), func(i, j int) {
			cards[i], cards[j] = cards[j], cards[i]
		})
	}
	return cards, nil
}

func validatePlayerCount(playerCount int) error {
	if playerCount < MinPlayers || playerCount > MaxPlayers {
		return fmt.Errorf("игроков за столом должно быть от %d до %d, получено: %d",
			MinPlayers, MaxPlayers, playerCount)
	}
	return nil
}
