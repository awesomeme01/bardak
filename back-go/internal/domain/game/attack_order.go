package game

// AttackOrderPolicy — кому переходит право подкидывать после паса.
//
// ⭐ Вынесено в стратегию, чтобы правило менялось без правки автомата: у бардака оно
// жёстче классического дурака и вполне может стать настройкой стола.
type AttackOrderPolicy interface {
	// NextAttacker — следующий обладатель права подкидывать.
	// Второе значение false, если подкидывать больше некому и раунд пора закрывать.
	NextAttacker(state DealState) (int, bool)
}

// BardakStrictNeighbours — боевое правило бардака: подкидывает начавший раунд, после его
// паса — второй сосед, то есть следующий по часовой за защищающимся.
//
// ⚠️ Право назад НЕ возвращается, остальные не подкидывают вообще.
type BardakStrictNeighbours struct{}

// NextAttacker — первый подходящий из очереди.
func (BardakStrictNeighbours) NextAttacker(state DealState) (int, bool) {
	for _, seat := range candidates(state) {
		if isEligibleAttacker(state, seat) {
			return seat, true
		}
	}
	return 0, false
}

// candidates — порядок очереди.
//
// ⭐ Второй сосед считается от ЗАЩИЩАЮЩЕГОСЯ, а не от начавшего раунд. Поэтому перевод,
// сдвигая защиту по кругу, меняет и состав подкидывающих.
func candidates(state DealState) []int {
	seats := []int{state.RoundStarterSeat}
	secondNeighbour, err := state.NextActiveSeatAfter(state.DefenderSeat)
	if err != nil {
		return seats
	}
	if !containsInt(seats, secondNeighbour) {
		seats = append(seats, secondNeighbour)
	}
	return seats
}

func isEligibleAttacker(state DealState, seat int) bool {
	if seat == state.DefenderSeat || state.HasPassed(seat) {
		return false
	}
	player, err := state.PlayerAt(seat)
	return err == nil && player.InDeal
}
