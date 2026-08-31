package game

import "testing"

// Уникальный отстающий: одним действием можно отдать несколько копий одного ранга,
// но уровень поднимается ровно на ОДНУ ступень (§2.3).
//
// ⚠️ Живая партия: у игрока четыре восьмёрки, жертве летит восьмёрка и она у неё самая
// низкая за столом. Отдать хотелось все четыре — по правилу это законно, а движок
// принимал только одну: заявка помечала место «решившим», и вторая карта уже не шла.
func TestLaggardTakesSeveralCopiesButRisesOneStep(t *testing.T) {
	engine := NewDealEngineFor(DefaultRulesConfig())
	eights := []Card{NewPip(Eight, Diamonds), NewPip(Eight, Hearts), NewPip(Eight, Spades)}

	// ⭐ Жертва (место 1) на уровне 1 — ей летит восьмёрка; у места 0 уровень выше,
	// поэтому минимум уникален и право у всех.
	build := func(everyClaimantHangs bool) DealState {
		window := OpenHangingWindow(1, [][]int{{0}}, everyClaimantHangs)
		return DealState{
			Phase: PhaseHanging,
			Players: []PlayerState{
				{SeatNo: 0, InDeal: true, Hand: eights, NavesLevel: 4, JokerHangerSeat: NobodySeat},
				{SeatNo: 1, InDeal: true, Hand: []Card{NewPip(Ace, Clubs)}, NavesLevel: 1,
					JokerHangerSeat: NobodySeat},
			},
			HangingWindow:    &window,
			DefenderSeat:     1,
			AttackRightSeat:  0,
			RoundStarterSeat: 0,
		}
	}

	result := engine.Apply(build(true).Clone(),
		HangCardCommand{Seat: 0, Card: eights[0], Also: eights[1:]})
	if !result.Applied {
		t.Fatalf("три копии отклонены: %s", result.Reason)
	}
	victim := result.State.Players[1]
	if len(victim.HungCards) != 3 {
		t.Errorf("в слот ушло %d карт, ждали три", len(victim.HungCards))
	}
	// ⭐ Копии ступень не двигают: было 1, стало 2, а не 4.
	if victim.NavesLevel != 2 {
		t.Errorf("уровень жертвы %d, ждали 2 — копии поднимать не должны", victim.NavesLevel)
	}

	// ⚠️ Без правила отстающего несколько карт разом отдать нельзя: обычный навес —
	// одна карта, и спор решается костью, а не тем, кто больше выложил.
	ordinary := engine.Apply(build(false).Clone(),
		HangCardCommand{Seat: 0, Card: eights[0], Also: eights[1:]})
	if ordinary.Applied {
		t.Error("несколько карт приняты при обычном навесе")
	} else if ordinary.Reason != ExtraHangsNotAllowed {
		t.Errorf("причина отказа %s, ждали EXTRA_HANGS_NOT_ALLOWED", ordinary.Reason)
	}

	// Одна и та же карта дважды — рука не резиновая.
	twice := engine.Apply(build(true).Clone(),
		HangCardCommand{Seat: 0, Card: eights[0], Also: []Card{eights[0]}})
	if twice.Applied {
		t.Error("одна карта отдана дважды")
	}
}
