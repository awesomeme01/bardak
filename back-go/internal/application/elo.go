package application

import (
	"errors"
	"math"
)

// Elo с попарным разложением (`07-rating-system.md`).
//
// ⭐ Классический Elo придуман для двоих с бинарным исходом, а за столом 2–5 игроков
// и результат — РАНЖИРОВАНИЕ. Матч разбирается на все пары: у кого место лучше, тот
// «выиграл» у другого.
//
// ⭐ Деление на N−1 обязательно. Без него матч на пятерых двигал бы рейтинг вчетверо
// сильнее, чем матч на двоих, — при той же самой игре.
//
// Функция чистая: ни базы, ни сети. Формулу так можно проверить на бумаге.
const (
	// kNewcomer — новичок быстро находит свой уровень.
	kNewcomer = 40.0
	kRegular  = 20.0
	// kTop — наверху шкала плотная: резкие скачки там означали бы шум карт, а не силу.
	kTop = 10.0

	newcomerMatches = 20
	topRating       = 2200.0
	// minRating — в дураке велика роль карт, и серия невезения не должна загонять
	// рейтинг в минус.
	minRating = 100.0
)

// ErrTooFewPlayers — рейтинг считается минимум для двоих.
//
// ⚠️ Явный отказ, а не тихий результат: на одном участнике деление на N−1 дало бы
// бесконечность и записало бы в базу мусор.
var ErrTooFewPlayers = errors.New("рейтинг считается минимум для двоих")

// EloParticipant — участник матча: текущий рейтинг, число сыгранных матчей и место.
type EloParticipant struct {
	Rating        float64
	MatchesPlayed int
	// Place — итоговое место, с первого; получивший джокер — последний.
	Place int
}

// Recalculate — новые рейтинги участников в том же порядке.
func Recalculate(players []EloParticipant) ([]float64, error) {
	if len(players) < 2 {
		return nil, ErrTooFewPlayers
	}

	updated := make([]float64, 0, len(players))
	for index, player := range players {
		delta := 0.0
		for other, opponent := range players {
			if other == index {
				continue
			}
			expected := 1 / (1 + math.Pow(10, (opponent.Rating-player.Rating)/400))
			actual := 0.0
			// ⚠️ Равные места дают ноль ОБОИМ, а не половину: делёж мест Elo не
			// предусматривает, и придумывать для него правило здесь незачем.
			if player.Place < opponent.Place {
				actual = 1
			}
			delta += kFor(player) * (actual - expected)
		}
		updated = append(updated, math.Max(minRating, player.Rating+delta/float64(len(players)-1)))
	}
	return updated, nil
}

func kFor(player EloParticipant) float64 {
	if player.Rating > topRating {
		return kTop
	}
	if player.MatchesPlayed < newcomerMatches {
		return kNewcomer
	}
	return kRegular
}
