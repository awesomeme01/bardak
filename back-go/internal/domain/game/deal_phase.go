package game

import "fmt"

// DealPhase — фаза автомата раздачи.
//
// ⭐ Разрешённые переходы — единственный источник правды о том, что игрок может сделать.
// Всё, что не разрешено явно, запрещено: список «нельзя» устаревает молча, список
// «можно» — с ошибкой компиляции.
type DealPhase uint8

const (
	// PhaseDealing — под-seed, перемешивание, сдача, определение козыря.
	PhaseDealing DealPhase = iota

	// PhaseDice — козырь оказался джокером: бросок кости и выбор масти.
	PhaseDice

	// PhaseAttack — обладатель права кладёт карту либо пасует.
	PhaseAttack

	// PhaseDefend — защищающийся бьёт, переводит или берёт.
	PhaseDefend

	// PhaseTaking — защищающийся объявил «беру», но раунд ещё жив.
	//
	// ⭐ Подкидывающие докидывают ему карты, пока не спасуют. Отбиваться и переводить он
	// уже не может, а потолок подкида — только лимит раунда: рука взявшего больше ничего
	// не ограничивает.
	PhaseTaking

	// PhaseHanging — окно навеса на взявшего карты.
	//
	// ⭐ Открывается после того, как стол уехал в руку: раньше состав руки жертвы
	// ещё не окончателен.
	PhaseHanging

	// PhaseRefill — добор до размера раздачи в строгом порядке.
	PhaseRefill

	// PhaseDealOver — карты остались у одного игрока, раздача окончена.
	PhaseDealOver
)

var phaseNames = [...]string{
	"DEALING", "DICE", "ATTACK", "DEFEND", "TAKING", "HANGING", "REFILL", "DEAL_OVER",
}

// String — имя фазы. Совпадает с Java: оно уходит в снимок состояния и в протокол,
// поэтому это часть контракта, а не отладочный вывод.
func (p DealPhase) String() string {
	if int(p) >= len(phaseNames) {
		return fmt.Sprintf("DealPhase(%d)", uint8(p))
	}
	return phaseNames[p]
}

// ParseDealPhase разбирает имя фазы из снимка состояния.
func ParseDealPhase(name string) (DealPhase, error) {
	for index, known := range phaseNames {
		if known == name {
			return DealPhase(index), nil
		}
	}
	return 0, fmt.Errorf("неизвестная фаза раздачи: %q", name)
}

// TableSlot — пара «атакующая карта — чем отбита».
//
// ⭐ Защищающийся всегда указывает, какую именно карту он бьёт, поэтому стол — это
// список пар, а не две кучи. Без этого при нескольких неотбитых картах нельзя понять,
// что именно покрыто.
type TableSlot struct {
	Attack Card
	// AttackBy — кто положил атакующую карту.
	//
	// ⭐ Подкидывают разные игроки, и без автора «забрать своё» не отличить от «забрать
	// чужое». Раньше стол этого не помнил: до отзыва карты он был и не нужен.
	AttackBy int
	// Defence — чем отбита; nil, пока карта не бита.
	Defence Card
	// DefenceBy — кто отбил. Всегда защищающийся, но храним явно: роль за раунд
	// меняется переводом, а карта на столе остаётся.
	DefenceBy int
	// AttackPinned, DefencePinned — «Карте место!»: карту зафиксировал кто-то другой,
	// и владелец забрать её уже не может.
	AttackPinned  bool
	DefencePinned bool
}

// NewSlot кладёт карту атаки на стол.
func NewSlot(attack Card, by int) TableSlot { return TableSlot{Attack: attack, AttackBy: by} }

// IsBeaten — покрыта ли атака.
func (s TableSlot) IsBeaten() bool { return s.Defence != nil }

// BeatenWith возвращает слот, покрытый указанной картой.
//
// ⚠️ Повторное покрытие — ошибка вызывающего, а не «последнее выигрывает»: молча
// затереть карту защиты значит потерять ход из истории партии.
func (s TableSlot) BeatenWith(card Card, by int) (TableSlot, error) {
	if s.IsBeaten() {
		return s, fmt.Errorf("карта %s уже бита картой %s", s.Attack.Code(), s.Defence.Code())
	}
	if card == nil {
		return s, fmt.Errorf("карта защиты не указана")
	}
	next := s
	next.Defence = card
	next.DefenceBy = by
	return next, nil
}
