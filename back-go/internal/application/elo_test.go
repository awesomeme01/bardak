package application

import (
	"errors"
	"math"
	"testing"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// Elo с попарным разложением и градуированным исходом.
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

// Ожидаемая цена пары при равных рейтингах: K * (градация − 0.5).
func pairPrice(k, mine, theirs float64) float64 {
	return k * (1/(1+math.Exp(-(theirs-mine)/gradeSteepness)) - 0.5)
}

func TestEloMovesTheRatingTowardsTheWinnerWhenTwoEqualsPlay(t *testing.T) {
	// «Летит 6» против навешенного джокера: ущерб 0 и 10.
	updated := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Damage: 0},
		{Rating: 1000, MatchesPlayed: 50, Damage: 10},
	})

	want := pairPrice(kRegular, 0, 10)
	if math.Abs(updated[0]-(1000+want)) > eloTolerance ||
		math.Abs(updated[1]-(1000-want)) > eloTolerance {
		t.Fatalf("ждали ровно ±%.6f, получили %v", want, updated)
	}
}

// ⭐ Главное свойство второй версии: близкая победа стоит дешевле разгромной.
func TestEloPaysMoreForABiggerGap(t *testing.T) {
	narrow := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Damage: 5}, // десятка
		{Rating: 1000, MatchesPlayed: 50, Damage: 6}, // валет
	})
	wide := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Damage: 1},  // шестёрка
		{Rating: 1000, MatchesPlayed: 50, Damage: 18}, // королевский отсос
	})

	if !(wide[0]-1000 > narrow[0]-1000) {
		t.Fatalf("разгром (%.4f) обязан стоить дороже близкой победы (%.4f)",
			wide[0]-1000, narrow[0]-1000)
	}
	// Соседняя ступень — заметно меньше половины цены разгрома, иначе градация
	// не отличалась бы от бинарного исхода.
	if narrow[0]-1000 > (wide[0]-1000)/2 {
		t.Fatalf("соседняя ступень стоит %.4f — слишком близко к разгрому %.4f",
			narrow[0]-1000, wide[0]-1000)
	}
}

// ⭐ Ровно то, ради чего затевалась вторая версия: за тяжёлую степень проигрыша
// игрок теряет больше, чем за лёгкую, при одном и том же составе стола.
func TestEloPunishesTheHeavierLossDegreeHarder(t *testing.T) {
	field := func(loserDamage float64) []EloParticipant {
		return []EloParticipant{
			{Rating: 1000, MatchesPlayed: 50, Damage: 1},
			{Rating: 1000, MatchesPlayed: 50, Damage: 3},
			{Rating: 1000, MatchesPlayed: 50, Damage: 5},
			{Rating: 1000, MatchesPlayed: 50, Damage: loserDamage},
		}
	}

	full := DamageOf(game.FullNavesScale(), game.FullNavesScale().JokerLevel(), game.NoLossDegree)
	fail := recalcOrFail(t, field(damageByDegree[game.LossFail]))
	royal := recalcOrFail(t, field(damageByDegree[game.LossRoyal]))
	plain := recalcOrFail(t, field(full))

	if !(royal[3] < fail[3] && fail[3] < plain[3]) {
		t.Fatalf("королевский (%.4f) обязан стоить дороже фейла (%.4f), "+
			"а фейл — дороже простого джокера (%.4f)", royal[3], fail[3], plain[3])
	}
}

// ⚠️ Пара обязана быть игрой с нулевой суммой при одинаковом K: иначе рейтинг стола
// потихоньку рос бы или падал сам по себе, без всякой игры.
func TestEloKeepsThePairZeroSum(t *testing.T) {
	for _, damage := range []float64{0, 1, 4.5, 10, 18} {
		updated := recalcOrFail(t, []EloParticipant{
			{Rating: 1180, MatchesPlayed: 50, Damage: 2},
			{Rating: 970, MatchesPlayed: 50, Damage: damage},
		})
		sum := (updated[0] - 1180) + (updated[1] - 970)
		if math.Abs(sum) > eloTolerance {
			t.Fatalf("при ущербе %v сумма изменений %.12f, ждали ноль", damage, sum)
		}
	}
}

// Равный ущерб — ничья: рейтинг не двигается ни у кого.
func TestEloLeavesTheRatingAloneOnAnEqualOutcome(t *testing.T) {
	updated := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Damage: 7},
		{Rating: 1000, MatchesPlayed: 50, Damage: 7},
		{Rating: 1000, MatchesPlayed: 50, Damage: 7},
	})

	for index, value := range updated {
		if math.Abs(value-1000) > eloTolerance {
			t.Fatalf("при равном ущербе игрок %d получил %v, ждали 1000", index, value)
		}
	}
}

// Новичок обязан находить свой уровень быстрее ветерана, иначе свежий аккаунт годами
// полз бы к своему настоящему рейтингу.
func TestEloGivesTheNewcomerABiggerStep(t *testing.T) {
	updated := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 0, Damage: 1},
		{Rating: 1000, MatchesPlayed: 50, Damage: 11},
	})

	newcomer := pairPrice(kNewcomer, 1, 11)
	veteran := pairPrice(kRegular, 11, 1)
	if math.Abs(updated[0]-(1000+newcomer)) > eloTolerance {
		t.Fatalf("новичок получил %v, ждали %v", updated[0], 1000+newcomer)
	}
	if math.Abs(updated[1]-(1000+veteran)) > eloTolerance {
		t.Fatalf("ветеран получил %v, ждали %v", updated[1], 1000+veteran)
	}
}

// Порог 2200 включает плотную шкалу: наверху рейтинг двигается медленнее.
func TestEloBarelyMovesTheRatingAtTheTop(t *testing.T) {
	updated := recalcOrFail(t, []EloParticipant{
		{Rating: 2300, MatchesPlayed: 100, Damage: 1},
		{Rating: 2300, MatchesPlayed: 100, Damage: 11},
	})

	want := pairPrice(kTop, 1, 11)
	if math.Abs(updated[0]-(2300+want)) > eloTolerance {
		t.Fatalf("наверху победитель получил %v, ждали %v", updated[0], 2300+want)
	}
}

// ⭐ Деление на N−1: матч на пятерых не должен двигать рейтинг вчетверо сильнее,
// чем матч на двоих при той же игре.
func TestEloNormalisesByTheNumberOfOpponents(t *testing.T) {
	pair := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Damage: 1},
		{Rating: 1000, MatchesPlayed: 50, Damage: 11},
	})
	table := recalcOrFail(t, []EloParticipant{
		{Rating: 1000, MatchesPlayed: 50, Damage: 1},
		{Rating: 1000, MatchesPlayed: 50, Damage: 11},
		{Rating: 1000, MatchesPlayed: 50, Damage: 11},
		{Rating: 1000, MatchesPlayed: 50, Damage: 11},
		{Rating: 1000, MatchesPlayed: 50, Damage: 11},
	})

	if math.Abs(pair[0]-table[0]) > eloTolerance {
		t.Fatalf("победа над одним (%v) и над четырьмя такими же (%v) обязаны "+
			"стоить одинаково", pair[0], table[0])
	}
}

// ⚠️ Серия невезения не должна загонять рейтинг в минус.
func TestEloNeverFallsBelowTheFloor(t *testing.T) {
	updated := recalcOrFail(t, []EloParticipant{
		{Rating: 2000, MatchesPlayed: 50, Damage: 0},
		{Rating: minRating, MatchesPlayed: 50, Damage: 18},
	})

	if updated[1] < minRating-eloTolerance {
		t.Fatalf("рейтинг упал до %v, ниже порога %v", updated[1], minRating)
	}
}

func TestEloRefusesToCountASinglePlayer(t *testing.T) {
	_, err := Recalculate([]EloParticipant{{Rating: 1000, MatchesPlayed: 3, Damage: 1}})
	if !errors.Is(err, ErrTooFewPlayers) {
		t.Fatalf("на одном участнике ждали ErrTooFewPlayers, получили %v", err)
	}
}

// Шкала ущерба: «летит 6» — ноль, дальше ступень за ступенью, джокер — десятка,
// а степени проигрыша растягивают шкалу дальше.
func TestDamageFollowsTheNavesScale(t *testing.T) {
	scale := game.FullNavesScale()

	if got := DamageOf(scale, game.NoNaves, game.NoLossDegree); got != 0 {
		t.Fatalf("«летит 6» дало ущерб %v, ждали 0", got)
	}
	for level, want := range []float64{1, 2, 3, 4, 5, 6, 7, 8, 9} {
		if got := DamageOf(scale, level, game.NoLossDegree); got != want {
			t.Fatalf("ступень %d дала ущерб %v, ждали %v", level, got, want)
		}
	}

	joker := scale.JokerLevel()
	previous := DamageOf(scale, joker, game.NoLossDegree)
	if previous != damageJoker {
		t.Fatalf("джокер без степени дал %v, ждали %v", previous, damageJoker)
	}
	// От самой лёгкой степени к самой тяжёлой ущерб обязан только расти.
	for _, degree := range []game.LossDegree{game.LossFail, game.LossSuperFail,
		game.LossSuperMegaFail, game.LossSuperMegaSuck, game.LossRoyal} {
		got := DamageOf(scale, joker, degree)
		if got <= previous {
			t.Fatalf("степень %s дала ущерб %v, а предыдущая — %v: шкала обязана расти",
				degree, got, previous)
		}
		previous = got
	}
}

// ⚠️ Ступень берётся по рангу карты, а не по индексу в шкале стола: на укороченной
// шкале тот же туз обязан стоить столько же, сколько на полной.
func TestDamageIsTheSameOnAShortenedScale(t *testing.T) {
	short, err := game.NewNavesScale([]game.Rank{game.Ten, game.Jack, game.Queen,
		game.King, game.Ace})
	if err != nil {
		t.Fatalf("укороченная шкала не собралась: %v", err)
	}

	full := game.FullNavesScale()
	fullAce := DamageOf(full, len(full.Ranks)-1, game.NoLossDegree)
	shortAce := DamageOf(short, len(short.Ranks)-1, game.NoLossDegree)
	if fullAce != shortAce {
		t.Fatalf("туз на полной шкале стоит %v, на укороченной %v", fullAce, shortAce)
	}
}
