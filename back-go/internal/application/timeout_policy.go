package application

import "github.com/awesomeme01/bardak/back-go/internal/domain/game"

// Что сервер делает за игрока, который не успел (§5.1).
//
// ⭐ Всегда выбирается САМОЕ БЕЗОБИДНОЕ действие: сервер никогда не решает за человека,
// какой картой ходить. Пропустил ход — пас или взял, но не «сервер сыграл твоим козырем».
//
// Единственное исключение — бросок кости: там выбора нет по сути, и пассивность не должна
// лишать права (ADR-030).

// SeatOnTheClock — чей сейчас ход. Второе значение false — ждать некого, часы не нужны.
func SeatOnTheClock(deal game.DealState) (int, bool) {
	switch deal.Phase {
	case game.PhaseAttack, game.PhaseTaking:
		return deal.AttackRightSeat, true
	case game.PhaseDefend:
		return deal.DefenderSeat, true
	case game.PhaseDice:
		// ⭐ Масть называет победитель кости, а не обладатель права атаки: кость уже
		// брошена, и ждут именно его.
		if deal.PendingHiddenTrump != nil {
			return deal.PendingHiddenTrump.ChooserSeat, true
		}
		return deal.AttackRightSeat, true
	case game.PhaseHanging:
		if deal.HangingWindow == nil {
			return 0, false
		}
		for _, seat := range deal.HangingWindow.CurrentStep() {
			if !deal.HangingWindow.IsSeatOnCurrentStep(seat) {
				continue
			}
			return seat, true
		}
		return 0, false
	default:
		// В сдаче, доборе и законченной раздаче ход не принадлежит никому: там работает
		// движок, а не игрок.
		return 0, false
	}
}

// AutoActionFor — команда, которую сервер выполнит за молчащего.
//
// Второе значение false — делать нечего: часы либо не идут, либо фаза не игрока.
func AutoActionFor(deal game.DealState) (game.DealCommand, bool) {
	seat, ok := SeatOnTheClock(deal)
	if !ok {
		return nil, false
	}
	switch deal.Phase {
	case game.PhaseAttack, game.PhaseTaking:
		return game.PassCommand{Seat: seat}, true
	case game.PhaseDefend:
		// ⚠️ Защищающийся по таймауту берёт. Но если брать нечего — стол пуст, — то и
		// «взял» невозможно по правилу пустого стола, и остаётся пас.
		if len(deal.Table) == 0 {
			return game.PassCommand{Seat: seat}, true
		}
		return game.TakeCommand{Seat: seat}, true
	case game.PhaseHanging:
		return game.HangSkipCommand{Seat: seat}, true
	case game.PhaseDice:
		return game.ChooseTrumpCommand{Seat: seat, Suit: richestSuit(deal, seat)}, true
	default:
		return nil, false
	}
}

// richestSuit — масть, которой у игрока больше всего.
//
// Спецификация не описывает, что делать, если победитель кости молчит: сама кость
// брошена за него (ADR-030), а выбор масти — уже решение. Берём самую многочисленную
// масть на руках: то, что человек и выбрал бы, глядя в свои карты (§1.2).
//
// ⚠️ Равенство решается МЛАДШЕЙ мастью по порядку объявления (бубны раньше треф).
// Это не выбор вкуса, а буквальное поведение Java: там `max` идёт по компаратору
// «сначала количество, потом ordinal В ОБРАТНОМ порядке», а максимум по обратному
// порядку — это наименьший ordinal. Читается как «старшая масть», работает наоборот.
// Свой порядок дал бы другую масть при том же раскладе, и разошлись бы только на ничьей —
// то есть редко и необъяснимо.
func richestSuit(deal game.DealState, seatNo int) game.Suit {
	player, err := deal.PlayerAt(seatNo)
	if err != nil {
		return game.Suits()[0]
	}

	counts := map[game.Suit]int{}
	for _, card := range player.Hand {
		if pip, ok := card.(game.Pip); ok {
			counts[pip.Suit]++
		}
	}

	best, found := game.Suits()[0], false
	for _, suit := range game.Suits() {
		if counts[suit] == 0 {
			continue
		}
		// Строго больше: при равенстве побеждает та, что встретилась раньше, а идём
		// мы по порядку объявления мастей.
		if !found || counts[suit] > counts[best] {
			best, found = suit, true
		}
	}
	return best
}
