package game

import "testing"

// Передумать можно, пока никто не возразил.
//
// ⭐ Положенная карта — это предложение, а не ход: пока её не зафиксировали, автор
// вправе забрать её обратно. Возражение выражается нажатием («Карте место!»),
// и после него карта остаётся на столе навсегда.
func TestRecallTakesYourOwnCardBackUntilItIsPinned(t *testing.T) {
	engine := NewDealEngineFor(DefaultRulesConfig())
	seven := NewPip(Seven, Hearts)

	// Место 0 положило семёрку, место 1 отбивается.
	table := func(mutate func(*TableSlot)) DealState {
		slot := NewSlot(seven, 0)
		if mutate != nil {
			mutate(&slot)
		}
		return DealState{
			Phase: PhaseDefend,
			Players: []PlayerState{
				{SeatNo: 0, InDeal: true, Hand: []Card{NewPip(Ace, Clubs)}, JokerHangerSeat: NobodySeat},
				{SeatNo: 1, InDeal: true, Hand: []Card{NewPip(King, Spades)}, JokerHangerSeat: NobodySeat},
			},
			Table:            []TableSlot{slot},
			DefenderSeat:     1,
			AttackRightSeat:  0,
			RoundStarterSeat: 0,
		}
	}

	t.Run("своя карта возвращается в руку", func(t *testing.T) {
		result := engine.Apply(table(nil).Clone(), RecallCardCommand{Seat: 0, Card: seven})
		if !result.Applied {
			t.Fatalf("отзыв отклонён: %s", result.Reason)
		}
		if len(result.State.Table) != 0 {
			t.Errorf("на столе осталось %d карт", len(result.State.Table))
		}
		if len(result.State.Players[0].Hand) != 2 {
			t.Errorf("в руке %d карт, ждали две", len(result.State.Players[0].Hand))
		}
		// ⭐ Стол опустел — раунд снова ждёт атаки, а не защиты.
		if result.State.Phase != PhaseAttack {
			t.Errorf("фаза %s, ждали ATTACK", result.State.Phase)
		}
	})

	t.Run("чужую забрать нельзя", func(t *testing.T) {
		result := engine.Apply(table(nil).Clone(), RecallCardCommand{Seat: 1, Card: seven})
		if result.Applied || result.Reason != CardNotYours {
			t.Errorf("чужая карта забрана: applied=%v reason=%s", result.Applied, result.Reason)
		}
	})

	t.Run("зафиксированную забрать нельзя", func(t *testing.T) {
		pinned := table(func(slot *TableSlot) { slot.AttackPinned = true })
		result := engine.Apply(pinned.Clone(), RecallCardCommand{Seat: 0, Card: seven})
		if result.Applied || result.Reason != CardIsPinned {
			t.Errorf("зафиксированная карта забрана: applied=%v reason=%s", result.Applied, result.Reason)
		}
	})

	t.Run("отбитую забрать поздно", func(t *testing.T) {
		beaten := table(func(slot *TableSlot) {
			slot.Defence = NewPip(King, Hearts)
			slot.DefenceBy = 1
		})
		result := engine.Apply(beaten.Clone(), RecallCardCommand{Seat: 0, Card: seven})
		if result.Applied || result.Reason != RecallTooLate {
			t.Errorf("отбитая атака отозвана: applied=%v reason=%s", result.Applied, result.Reason)
		}
	})

	t.Run("«Карте место!» фиксирует чужую, но не свою", func(t *testing.T) {
		pinned := engine.Apply(table(nil).Clone(), PinCardCommand{Seat: 1, Card: seven})
		if !pinned.Applied {
			t.Fatalf("фиксация отклонена: %s", pinned.Reason)
		}
		if !pinned.State.Table[0].AttackPinned {
			t.Error("карта не зафиксирована")
		}
		// После фиксации автор забрать её уже не может.
		after := engine.Apply(pinned.State.Clone(), RecallCardCommand{Seat: 0, Card: seven})
		if after.Applied {
			t.Error("зафиксированную карту всё-таки забрали")
		}

		own := engine.Apply(table(nil).Clone(), PinCardCommand{Seat: 0, Card: seven})
		if own.Applied || own.Reason != CardNotYours {
			t.Errorf("свою карту зафиксировали: applied=%v reason=%s", own.Applied, own.Reason)
		}
	})
}
