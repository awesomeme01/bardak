package game

// Trump — козырь раздачи и вытекающая из него защищённая масть.
//
// Вместе, потому что порознь они бессмысленны: защищённая масть определяется козырем
// и меняется вместе с ним.
//
// ⭐ Базовое допущение дурака «козырь бьёт всё» здесь НЕВЕРНО. Защищённую масть козырь
// не берёт, поэтому старшинство — отдельная функция Beats, а не выражение из двух условий.
type Trump struct {
	Suit Suit
}

// Защищённая масть по умолчанию — пики; если козырь сам пики, роль переходит к трефам.
const (
	defaultProtectedSuit  = Spades
	fallbackProtectedSuit = Clubs
)

// NewTrump собирает козырь.
func NewTrump(suit Suit) Trump { return Trump{Suit: suit} }

// ProtectedSuit — масть, которую козырь не берёт.
//
// Защищённая масть есть всегда, и она всегда одна.
func (t Trump) ProtectedSuit() Suit {
	if t.Suit == defaultProtectedSuit {
		return fallbackProtectedSuit
	}
	return defaultProtectedSuit
}

// Beats — бьёт ли карта защиты карту атаки.
//
// ⚠️ Правило применяется ИСКЛЮЧИТЕЛЬНО в момент защиты: на атаку, подкидывание и перевод
// защищённая масть не влияет. Смешать это — значит запретить подкидывать пики.
func (t Trump) Beats(defence, attack Card) bool {
	// Джокер кроет что угодно, включая другой джокер.
	if _, ok := defence.(Joker); ok {
		return true
	}
	// Джокер в атаке не берётся ничем, кроме джокера, — а тот отработал выше.
	if _, ok := attack.(Joker); ok {
		return false
	}

	defencePip, okDefence := defence.(Pip)
	attackPip, okAttack := attack.(Pip)
	if !okDefence || !okAttack {
		return false
	}

	// Внутри одной масти решает ранг — козырность роли не играет.
	if defencePip.Suit == attackPip.Suit {
		return defencePip.Rank.IsHigherThan(attackPip.Rank)
	}

	// ⭐ Вот здесь дурак и бардак расходятся: защищённую масть чужой мастью не взять,
	// даже козырем. Отбиться от неё можно только старшей картой той же масти или джокером.
	if attackPip.Suit == t.ProtectedSuit() {
		return false
	}

	return defencePip.Suit == t.Suit
}
