package application

import (
	"errors"
	"math"
	"testing"
)

// Elo с попарным разложением.
//
// ⭐ Числа здесь зафиксированы ТОЧНО, а не «примерно»: это цена одного матча в продукте,
// и любая правка констант обязана уронить тест, а не тихо поменять шкалу всей игры.

const eloTolerance = 1e-9

func recalcOrFail(t *testing.T, players []EloParticipant) []float64 {
	t.Helper()
	updated, err := Recalculate(players)
	if err != nil {
		t.Fatalf("рейтинг не посчитался: %v", err)
	}
	return updated
}

func TestEloMovesTheRatingTowardsTheWinnerWhenTwoEqualsPlay(t *testing.T) {
	updated := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Place: 1},
		{Rating: 1000, MatchesPlayed: 50, Place: 2},
	})

	if math.Abs(updated[0]-1010) > eloTolerance || math.Abs(updated[1]-990) > eloTolerance {
		t.Fatalf("при равных рейтингах и K=20 ждали ровно ±10, получили %v", updated)
	}
}

// Новичок обязан находить свой уровень быстрее ветерана, иначе свежий аккаунт годами
// полз бы к своему настоящему рейтингу.
func TestEloGivesTheNewcomerABiggerStep(t *testing.T) {
	updated := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 0, Place: 1},
		{Rating: 1000, MatchesPlayed: 50, Place: 2},
	})

	if math.Abs(updated[0]-1020) > eloTolerance {
		t.Fatalf("новичок получил %v, ждали +20 (K=40)", updated[0])
	}
	if math.Abs(updated[1]-990) > eloTolerance {
		t.Fatalf("ветеран получил %v, ждали −10 (K=20)", updated[1])
	}
}

// Порог 2200 включает плотную шкалу: наверху рейтинг двигается вдвое медленнее.
func TestEloBarelyMovesTheRatingAtTheTop(t *testing.T) {
	updated := recalcOrFail(t, []EloParticipant{
		{Rating: 2300, MatchesPlayed: 100, Place: 1},
		{Rating: 2300, MatchesPlayed: 100, Place: 2},
	})

	if math.Abs(updated[0]-2305) > eloTolerance {
		t.Fatalf("наверху победитель получил %v, ждали +5", updated[0])
	}
}

// ⚠️ Знак и направление разности в expected: перепутать их значит начать вознаграждать
// фарм слабых соперников.
func TestEloRewardsBeatingAStrongerPlayerMore(t *testing.T) {
	underdog := recalcOrFail(t, []EloParticipant{
		{Rating: 800, MatchesPlayed: 50, Place: 1},
		{Rating: 1400, MatchesPlayed: 50, Place: 2},
	})
	favourite := recalcOrFail(t, []EloParticipant{
		{Rating: 1400, MatchesPlayed: 50, Place: 1},
		{Rating: 800, MatchesPlayed: 50, Place: 2},
	})

	if underdog[0]-800 <= favourite[0]-1400 {
		t.Fatalf("победа над сильным дала %v, над слабым %v — должно быть больше",
			underdog[0]-800, favourite[0]-1400)
	}
}

// ⭐ Нормировка на (N−1): матч на пятерых двигает рейтинг ровно так же, как дуэль.
// Без неё за столом на пятерых победитель получал бы вчетверо больше за ту же игру.
func TestEloKeepsTheSwingTheSameWhenFivePlay(t *testing.T) {
	duel := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Place: 1},
		{Rating: 1000, MatchesPlayed: 50, Place: 2},
	})

	table := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Place: 1},
		{Rating: 1000, MatchesPlayed: 50, Place: 2},
		{Rating: 1000, MatchesPlayed: 50, Place: 3},
		{Rating: 1000, MatchesPlayed: 50, Place: 4},
		{Rating: 1000, MatchesPlayed: 50, Place: 5},
	})

	if math.Abs((duel[0]-1000)-(table[0]-1000)) > eloTolerance {
		t.Fatalf("в дуэли %v, за столом на пятерых %v — рейтинги перестали быть сопоставимыми",
			duel[0]-1000, table[0]-1000)
	}
}

// Elo — игра с нулевой суммой: очки перераспределяются, а не создаются.
//
// ⚠️ Инвариант держится только при равных рейтингах и одинаковом K. С новичком против
// ветерана сумма уже не сохраняется, и этот случай тест намеренно не берёт.
func TestEloKeepsTheSumWhenEveryoneStartsEqual(t *testing.T) {
	players := []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Place: 1},
		{Rating: 1000, MatchesPlayed: 50, Place: 2},
		{Rating: 1000, MatchesPlayed: 50, Place: 3},
		{Rating: 1000, MatchesPlayed: 50, Place: 4},
	}

	updated := recalcOrFail(t, players)

	sum := 0.0
	for _, rating := range updated {
		sum += rating
	}
	if math.Abs(sum-4000) > 1e-4 {
		t.Fatalf("сумма рейтингов %v вместо 4000: рейтинг инфлирует по всей базе", sum)
	}
}

// Нижняя граница: серия невезения не должна загонять рейтинг в минус.
func TestEloNeverFallsBelowTheFloor(t *testing.T) {
	updated := recalcOrFail(t, []EloParticipant{
		{Rating: 100, MatchesPlayed: 50, Place: 2},
		{Rating: 2000, MatchesPlayed: 50, Place: 1},
	})

	if updated[0] < minRating {
		t.Fatalf("рейтинг ушёл ниже пола: %v", updated[0])
	}
}

// Одиночный матч — громкий отказ до записи в базу, а не Infinity в рейтинге.
func TestEloRefusesToCountASoloMatch(t *testing.T) {
	if _, err := Recalculate([]EloParticipant{{Rating: 1000, Place: 1}}); !errors.Is(err, ErrTooFewPlayers) {
		t.Fatalf("матч на одного посчитался: %v", err)
	}
}
