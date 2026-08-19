package game

import "fmt"

// NoNaves — уровень игрока, которому ещё ничего не навешивали.
const NoNaves = -1

// NavesScale — личная шкала навесов: 6 → 7 → … → A → Joker.
//
// ⭐ Счёта в очках в игре нет — шкала и есть счёт.
//
// Уровень игрока кодируется индексом:
//
//	-1            навесов ещё не было, летит первая ступень
//	0 … size-1    навешен Ranks[level]
//	size          навешен джокер — игрок проиграл
//
// Длина шкалы — параметр стола: укоротить её до 9…A должно быть правкой конфигурации,
// а не кода.
type NavesScale struct {
	// Ranks — ступени по возрастанию; джокер — терминальная ступень сверх списка.
	Ranks []Rank
}

// NewNavesScale собирает шкалу. Пустая шкала бессмысленна: навешивать было бы нечего.
func NewNavesScale(ranks []Rank) (NavesScale, error) {
	if len(ranks) == 0 {
		return NavesScale{}, fmt.Errorf("шкала навесов не может быть пустой")
	}
	out := make([]Rank, len(ranks))
	copy(out, ranks)
	return NavesScale{Ranks: out}, nil
}

// FullNavesScale — полная шкала из всех девяти рангов, как играют вживую.
func FullNavesScale() NavesScale {
	return NavesScale{Ranks: Ranks()}
}

// JokerLevel — уровень, после которого следующая ступень джокер.
func (s NavesScale) JokerLevel() int { return len(s.Ranks) }

// IsFinished — джокер уже навешен: игрок проиграл, навешивать ему больше нечего.
func (s NavesScale) IsFinished(level int) bool { return level >= s.JokerLevel() }

// NextIsJoker — следующая ступень джокер, а не обычный ранг.
func (s NavesScale) NextIsJoker(level int) bool { return level+1 == s.JokerLevel() }

// NextRank — ранг, который сейчас «летит» игроку.
//
// Второе значение false, если следующая ступень джокер либо шкала уже пройдена.
func (s NavesScale) NextRank(level int) (Rank, bool) {
	if level+1 >= s.JokerLevel() || level+1 < 0 {
		return 0, false
	}
	return s.Ranks[level+1], true
}

// IsFlyingCard — подходит ли карта под текущую ступень жертвы. Масть значения не имеет.
func (s NavesScale) IsFlyingCard(level int, card Card) bool {
	if s.IsFinished(level) {
		return false
	}
	if s.NextIsJoker(level) {
		_, isJoker := card.(Joker)
		return isJoker
	}
	rank, ok := s.NextRank(level)
	if !ok {
		return false
	}
	pip, isPip := card.(Pip)
	return isPip && pip.Rank == rank
}

// RulesConfig — правила стола.
//
// ⭐ Все игровые числа живут здесь, а не литералами в движке: «подкидывать не больше
// шести» — это правило стола, и менять его должна настройка, а не правка кода.
type RulesConfig struct {
	DealSize            int
	MaxAttackFirstRound int
	MaxAttackPerRound   int
	TransfersEnabled    bool
	JokersEnabled       bool
	NavesEnabled        bool
	NavesScale          NavesScale
}

// DefaultRulesConfig — стартовая точка стола, а не константы движка.
func DefaultRulesConfig() RulesConfig {
	return RulesConfig{
		DealSize:            6,
		MaxAttackFirstRound: 5,
		MaxAttackPerRound:   6,
		TransfersEnabled:    true,
		JokersEnabled:       true,
		NavesEnabled:        true,
		NavesScale:          FullNavesScale(),
	}
}

// WithoutNaves — каркас без навесов: серия раздач без продвижения по шкале.
// Годится только для отладки.
func (c RulesConfig) WithoutNaves() RulesConfig {
	next := c
	next.NavesEnabled = false
	return next
}

// AttackLimit — потолок атакующих карт в раунде.
//
// ⚠️ Зависит от того, уходили ли в этой РАЗДАЧЕ карты в отбой, а не от состояния стола:
// пять до первого отбоя, шесть после.
func (c RulesConfig) AttackLimit(anyPileDiscarded bool) int {
	if anyPileDiscarded {
		return c.MaxAttackPerRound
	}
	return c.MaxAttackFirstRound
}

// Validate — проверка на осмысленность; вызывается при разборе настроек стола.
func (c RulesConfig) Validate() error {
	for _, check := range []struct {
		value int
		name  string
	}{
		{c.DealSize, "dealSize"},
		{c.MaxAttackFirstRound, "maxAttackFirstRound"},
		{c.MaxAttackPerRound, "maxAttackPerRound"},
	} {
		if check.value <= 0 {
			return fmt.Errorf("%s должен быть положительным, получено: %d", check.name, check.value)
		}
	}
	if len(c.NavesScale.Ranks) == 0 {
		return fmt.Errorf("шкала навесов не может быть пустой")
	}
	return nil
}
