package game

import "fmt"

// DealState — снимок раздачи: всё, что нужно правилам, и ничего больше.
//
// ⭐ Значение неизменяемое: движок работает как apply(state, command) -> (newState, events).
// Мутация сделала бы невозможным сравнение «до и после», а на нём держатся и события,
// и откат отклонённого хода.
type DealState struct {
	// Phase — фаза автомата раздачи.
	Phase DealPhase
	// Trump — козырь и вытекающая защищённая масть.
	// nil только в фазах DEALING и DICE: нижней картой оказался джокер, масть ещё не названа.
	Trump *Trump
	// Deck — остаток колоды: индекс 0 — верх, последняя карта — потайной козырь.
	Deck []Card
	// Players — игроки по местам; индекс равен SeatNo.
	Players []PlayerState
	// Table — стол как список пар «атака — чем бита».
	Table []TableSlot
	// RoundStarterSeat — кто начал текущий раунд; от него считается порядок подкида и добора.
	RoundStarterSeat int
	// AttackRightSeat — у кого сейчас право положить атакующую карту.
	AttackRightSeat int
	// DefenderSeat — кто отбивается.
	DefenderSeat int
	// PassedSeats — кто уже спасовал в этом раунде: право назад не возвращается.
	PassedSeats []int
	// ExitOrder — места в порядке выхода из раздачи, первым вышедший первым.
	ExitOrder []int
	// AnyCardBeatenThisRound — хотя бы одна карта в раунде отбита: выключатель перевода.
	AnyCardBeatenThisRound bool
	// AnyPileDiscarded — в этой РАЗДАЧЕ уже уходили карты в отбой.
	// ⚠️ От этого зависит потолок атаки, а не от состояния стола.
	AnyPileDiscarded bool
	// HangingWindow — открытое окно навеса или nil.
	HangingWindow *HangingWindow
	// LastAttackCards — состав последней атаки раздачи.
	// ⭐ Нужен для степеней проигрыша: считаются восьмёрки в том, что было ВЫЛОЖЕНО,
	// а не в том, что попало игроку в руку.
	LastAttackCards []Card
	// PendingHiddenTrump — потайной козырь-джокер, ждущий выбора масти; обычно nil.
	PendingHiddenTrump *PendingHiddenTrump
	// RngSeed — под-seed раздачи: всё случайное выводится из него.
	RngSeed int64
	// DiceRolls — сколько бросков кости уже случилось, чтобы два спора подряд
	// не давали одинаковый результат.
	DiceRolls int
}

// HasTrump — козырь назван. В фазе выбора масти его ещё нет.
func (s DealState) HasTrump() bool { return s.Trump != nil }

// IsDeckEmpty — колода исчерпана.
func (s DealState) IsDeckEmpty() bool { return len(s.Deck) == 0 }

// PlayerAt — игрок на месте. Место вне стола — ошибка вызывающего.
func (s DealState) PlayerAt(seatNo int) (PlayerState, error) {
	if seatNo < 0 || seatNo >= len(s.Players) {
		return PlayerState{}, fmt.Errorf("за столом нет места %d", seatNo)
	}
	return s.Players[seatNo], nil
}

// MustPlayerAt — то же, но для мест, взятых из самого состояния.
func (s DealState) MustPlayerAt(seatNo int) PlayerState {
	player, err := s.PlayerAt(seatNo)
	if err != nil {
		panic(err)
	}
	return player
}

// Defender — тот, кто отбивается.
func (s DealState) Defender() PlayerState { return s.MustPlayerAt(s.DefenderSeat) }

// AttackCardCount — сколько атакующих карт уже лежит; с ними сверяется потолок атаки.
func (s DealState) AttackCardCount() int { return len(s.Table) }

// UnbeatenCount — сколько атакующих карт ещё не отбито; с ними сверяется рука защиты.
func (s DealState) UnbeatenCount() int {
	count := 0
	for _, slot := range s.Table {
		if !slot.IsBeaten() {
			count++
		}
	}
	return count
}

// HasRankOnTable — есть ли на столе карта такого же ранга: условие подкидывания.
//
// ⚠️ Считаются И атакующие карты, И те, которыми отбивались. Забыть вторые — значит
// запретить законный подкид.
func (s DealState) HasRankOnTable(card Card) bool {
	for _, slot := range s.Table {
		if slot.Attack.SameRankAs(card) {
			return true
		}
		if slot.Defence != nil && card.SameRankAs(slot.Defence) {
			return true
		}
	}
	return false
}

// TableCards — все карты со стола: то, что забирает взявший.
func (s DealState) TableCards() []Card {
	cards := make([]Card, 0, len(s.Table)*2)
	for _, slot := range s.Table {
		cards = append(cards, slot.Attack)
		if slot.Defence != nil {
			cards = append(cards, slot.Defence)
		}
	}
	return cards
}

// NextActiveSeatAfter — следующее по часовой стрелке место, где ещё есть игрок в раздаче.
//
// Именно к нему уезжает защита при переводе и переходит ход после «взял».
func (s DealState) NextActiveSeatAfter(seatNo int) (int, error) {
	size := len(s.Players)
	for step := 1; step <= size; step++ {
		candidate := (seatNo + step) % size
		if s.Players[candidate].InDeal {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("в раздаче не осталось игроков после места %d", seatNo)
}

// PlayersInDeal — сколько игроков ещё в раздаче.
func (s DealState) PlayersInDeal() int {
	count := 0
	for _, player := range s.Players {
		if player.InDeal {
			count++
		}
	}
	return count
}

// HasPassed — спасовал ли игрок в этом раунде.
func (s DealState) HasPassed(seatNo int) bool { return containsInt(s.PassedSeats, seatNo) }

// WithPlayer возвращает состояние с заменённым игроком.
//
// ⭐ Отдельный метод, потому что подменять срез руками — верный способ забыть копию
// и незаметно изменить «неизменяемое» состояние через общую ссылку.
func (s DealState) WithPlayer(player PlayerState) DealState {
	next := s
	players := make([]PlayerState, len(s.Players))
	copy(players, s.Players)
	players[player.SeatNo] = player
	next.Players = players
	return next
}

// WithTable возвращает состояние с новым столом.
func (s DealState) WithTable(table []TableSlot) DealState {
	next := s
	next.Table = append([]TableSlot(nil), table...)
	return next
}

// WithPhase возвращает состояние в другой фазе.
func (s DealState) WithPhase(phase DealPhase) DealState {
	next := s
	next.Phase = phase
	return next
}

// WithPassed отмечает место спасовавшим. Повтор не дублируется.
func (s DealState) WithPassed(seatNo int) DealState {
	if s.HasPassed(seatNo) {
		return s
	}
	next := s
	next.PassedSeats = append(append([]int(nil), s.PassedSeats...), seatNo)
	return next
}

// Clone — глубокая копия изменяемых частей. Нужна там, где состояние собирается
// по кускам: срезы в Go разделяются, и без копии правка «новой» раздачи меняла бы старую.
func (s DealState) Clone() DealState {
	next := s
	next.Deck = append([]Card(nil), s.Deck...)
	next.Players = append([]PlayerState(nil), s.Players...)
	next.Table = append([]TableSlot(nil), s.Table...)
	next.PassedSeats = append([]int(nil), s.PassedSeats...)
	next.ExitOrder = append([]int(nil), s.ExitOrder...)
	next.LastAttackCards = append([]Card(nil), s.LastAttackCards...)
	return next
}
