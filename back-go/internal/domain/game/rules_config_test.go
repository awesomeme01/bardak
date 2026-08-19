package game

import "testing"

// Перенос NavesScaleTest. Шкала — это и есть счёт в игре, ошибка здесь искажает результат матча.

func TestLowestRankFliesFirst(t *testing.T) {
	scale := FullNavesScale()

	rank, ok := scale.NextRank(NoNaves)
	if !ok {
		t.Fatal("до первого навеса должна лететь первая ступень")
	}
	if rank != Six {
		t.Errorf("первой летит %s, ждали шестёрку", rank.Code())
	}
}

func TestNextRankGoesUp(t *testing.T) {
	scale := FullNavesScale()

	rank, ok := scale.NextRank(0) // навешена шестёрка
	if !ok {
		t.Fatal("после шестёрки должна лететь семёрка")
	}
	if rank != Seven {
		t.Errorf("после шестёрки летит %s, ждали семёрку", rank.Code())
	}
}

// ⭐ Джокер — терминальная ступень сверх списка рангов: навешенный джокер означает проигрыш.
func TestJokerComesAfterTopRank(t *testing.T) {
	scale := FullNavesScale()
	topLevel := scale.JokerLevel() - 1 // навешен туз

	if !scale.NextIsJoker(topLevel) {
		t.Error("после туза обязан лететь джокер")
	}
	if _, ok := scale.NextRank(topLevel); ok {
		t.Error("после туза обычного ранга уже нет")
	}
	if !scale.IsFlyingCard(topLevel, MustJoker(1)) {
		t.Error("джокер обязан подходить под последнюю ступень")
	}
	if scale.IsFlyingCard(topLevel, NewPip(Ace, Spades)) {
		t.Error("туз уже навешен — второй раз он не летит")
	}
}

// Масть значения не имеет: летит ранг.
func TestFlyingRankIgnoresSuit(t *testing.T) {
	scale := FullNavesScale()

	for _, suit := range Suits() {
		if !scale.IsFlyingCard(NoNaves, NewPip(Six, suit)) {
			t.Errorf("шестёрка %s обязана лететь первой ступенью", suit.Symbol())
		}
		if scale.IsFlyingCard(NoNaves, NewPip(Seven, suit)) {
			t.Errorf("семёрка %s не должна лететь раньше шестёрки", suit.Symbol())
		}
	}
}

// ⚠️ Джокер навешен — игрок проиграл, навешивать ему больше нечего.
func TestNothingFliesAfterJoker(t *testing.T) {
	scale := FullNavesScale()
	finished := scale.JokerLevel()

	if !scale.IsFinished(finished) {
		t.Fatal("уровень джокера означает конец шкалы")
	}
	if scale.IsFlyingCard(finished, MustJoker(1)) {
		t.Error("на добитого джокер не летит")
	}
	for _, rank := range Ranks() {
		if scale.IsFlyingCard(finished, NewPip(rank, Hearts)) {
			t.Errorf("на добитого не летит и %s", rank.Code())
		}
	}
}

// Длина шкалы — параметр стола: укоротить её должно быть правкой настроек, а не кода.
func TestShortenedScaleIsHonoured(t *testing.T) {
	short, err := NewNavesScale([]Rank{Nine, Ten, Jack})
	if err != nil {
		t.Fatal(err)
	}

	if short.JokerLevel() != 3 {
		t.Errorf("джокер на уровне %d, ждали 3", short.JokerLevel())
	}
	if rank, ok := short.NextRank(NoNaves); !ok || rank != Nine {
		t.Error("укороченная шкала обязана начинаться с девятки")
	}
	if !short.NextIsJoker(2) {
		t.Error("после валета в укороченной шкале летит джокер")
	}
	if short.IsFlyingCard(NoNaves, NewPip(Six, Hearts)) {
		t.Error("шестёрки нет в укороченной шкале — она не летит")
	}

	if _, err := NewNavesScale(nil); err == nil {
		t.Error("пустая шкала бессмысленна и должна отвергаться")
	}
}

// Потолок атаки зависит от того, уходили ли карты в отбой в этой раздаче,
// а не от состояния стола.
func TestAttackLimitDependsOnDiscard(t *testing.T) {
	config := DefaultRulesConfig()

	if got := config.AttackLimit(false); got != 5 {
		t.Errorf("до первого отбоя потолок %d, ждали 5", got)
	}
	if got := config.AttackLimit(true); got != 6 {
		t.Errorf("после отбоя потолок %d, ждали 6", got)
	}
}

func TestRulesConfigValidation(t *testing.T) {
	if err := DefaultRulesConfig().Validate(); err != nil {
		t.Errorf("умолчания обязаны быть корректны: %v", err)
	}

	broken := DefaultRulesConfig()
	broken.DealSize = 0
	if err := broken.Validate(); err == nil {
		t.Error("нулевой размер раздачи недопустим")
	}
}
