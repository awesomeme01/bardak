package application

import (
	"errors"
	"math"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// Elo с попарным разложением и градуированным исходом (`07-rating-system.md`).
//
// ⭐ Классический Elo придуман для двоих с бинарным исходом, а за столом 2–5 игроков
// и результат — РАНЖИРОВАНИЕ. Матч разбирается на все пары.
//
// ⭐ Исход пары НЕ бинарный, и это главное отличие от первой версии. «Выиграл» и
// «проиграл» отвечают на вопрос «кто выше», но не на вопрос «насколько» — а в бардаке
// разница между «летит 6» и «королевским отсосом» больше, чем между соседними ступенями
// шкалы. Поэтому пара сравнивается по ШКАЛЕ УЩЕРБА, и близкая победа стоит дешевле
// разгромной. Отсюда же берётся и цена степени проигрыша: она сидит на той же шкале,
// поэтому королевский стоит игроку дороже фейла — отдельного правила для этого не нужно.
//
// ⭐ Деление на N−1 обязательно. Без него матч на пятерых двигал бы рейтинг вчетверо
// сильнее, чем матч на двоих, — при той же самой игре.
//
// Функция чистая: ни базы, ни сети. Формулу так можно проверить на бумаге.

const (
	// kNewcomer — новичок быстро находит свой уровень.
	kNewcomer = 24.0
	kRegular  = 12.0
	// kTop — наверху шкала плотная: резкие скачки там означали бы шум карт, а не силу.
	kTop = 8.0

	newcomerMatches = 20
	topRating       = 2200.0
	// minRating — в дураке велика роль карт, и серия невезения не должна загонять
	// рейтинг в минус.
	minRating = 100.0

	// gradeSteepness — крутизна градации: на сколько ступеней ущерба надо оторваться,
	// чтобы пара засчиталась как уверенная победа.
	//
	// ⚠️ Число подобрано ПО ИСТОРИИ РЕАЛЬНЫХ ПАРТИЙ, а не на глаз: на 344 оффлайн-матчах
	// кросс-проверкой мерялась предсказательная сила рейтинга, и градация побеждает
	// бинарный исход (log-loss 0.6937 против 0.6966). Разрыв в одну ступень даёт 0.58,
	// в четыре («шестёрка» против «десятки») — 0.79, до джокера — 0.95.
	gradeSteepness = 3.0
)

// Шкала ущерба — во сколько игроку обошёлся матч.
//
// ⭐ Ноль у того, кому не навесили ничего («летит 6»), дальше ступень за ступенью
// до джокера, а после джокера шкала РАСТЯГИВАЕТСЯ по степени проигрыша: между фейлом
// и королевским больше, чем между соседними картами. Иначе «навесили джокер» и
// «королевский отсос» стоили бы почти одинаково, хотя за столом это несравнимые вещи.
//
// ⚠️ Ступень берётся по РАНГУ карты, а не по индексу в шкале стола: шкалу навесов можно
// укоротить настройками, и тогда индекс «4» означал бы разные карты на разных столах —
// а рейтинг у игроков общий.
const (
	damageNoNaves = 0.0
	damageJoker   = 10.0
)

// damageByDegree — надбавка за степень проигрыша поверх джокера.
var damageByDegree = map[game.LossDegree]float64{
	game.NoLossDegree:      damageJoker,
	game.LossFail:          11.0,
	game.LossSuperFail:     12.5,
	game.LossSuperMegaFail: 14.0,
	game.LossSuperMegaSuck: 16.0,
	game.LossRoyal:         18.0,
}

// DamageOf — ущерб игрока по итогу матча.
//
// levelAfter — уровень навесов на конец матча, degree — степень проигрыша
// (game.NoLossDegree у того, кто игру не проиграл).
func DamageOf(scale game.NavesScale, levelAfter int, degree game.LossDegree) float64 {
	if levelAfter == game.NoNaves {
		return damageNoNaves
	}
	if scale.IsFinished(levelAfter) {
		if damage, ok := damageByDegree[degree]; ok {
			return damage
		}
		return damageJoker
	}
	// Ранг задаёт ступень: Six=0 даёт 1, Ace=8 даёт 9.
	return float64(scale.Ranks[levelAfter]) + 1
}

// ErrTooFewPlayers — рейтинг считается минимум для двоих.
//
// ⚠️ Явный отказ, а не тихий результат: на одном участнике деление на N−1 дало бы
// бесконечность и записало бы в базу мусор.
var ErrTooFewPlayers = errors.New("рейтинг считается минимум для двоих")

// EloParticipant — участник матча: текущий рейтинг, число сыгранных матчей и ущерб.
type EloParticipant struct {
	Rating        float64
	MatchesPlayed int
	// Damage — во сколько обошёлся матч; меньше — лучше сыграл. Считается DamageOf.
	Damage float64
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
			delta += kFor(player) * (gradedScore(player.Damage, opponent.Damage) - expected)
		}
		updated = append(updated, math.Max(minRating, player.Rating+delta/float64(len(players)-1)))
	}
	return updated, nil
}

// gradedScore — доля очка, которую игрок взял в паре.
//
// ⚠️ Ровно 0.5 при равном ущербе и симметрична: score(a,b) + score(b,a) = 1 тождественно.
// Без этого пара перестала бы быть игрой с нулевой суммой, и рейтинг всего стола
// потихоньку рос бы или падал сам по себе.
func gradedScore(mine, theirs float64) float64 {
	return 1 / (1 + math.Exp(-(theirs-mine)/gradeSteepness))
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
