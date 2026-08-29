package game

import "testing"

// Репродукция живого матча 14c4c247: козырь черви, на столе K-clubs, у защищающегося
// Joker-3. Джокер обязан бить всё (§1.1.1) и обязан быть предложен в availableActions.
func TestJokerDefendsKingOfClubsRepro(t *testing.T) {
	trump := Trump{Suit: Hearts}
	deal := DealState{
		Phase: PhaseDefend,
		Trump: &trump,
		Table: []TableSlot{{Attack: NewPip(King, Clubs)}},
		Players: []PlayerState{
			{SeatNo: 0, InDeal: true, Hand: []Card{NewPip(Ten, Hearts)}},
			{SeatNo: 1, InDeal: true, Hand: []Card{NewPip(Seven, Spades)}},
			{SeatNo: 2, InDeal: true, Hand: []Card{MustJoker(3), NewPip(Eight, Clubs)}},
		},
		RoundStarterSeat: 0, AttackRightSeat: 0, DefenderSeat: 2,
	}

	if !trump.Beats(MustJoker(3), NewPip(King, Clubs)) {
		t.Fatal("Beats: джокер не бьёт короля треф")
	}

	engine := NewDealEngineFor(DefaultRulesConfig())
	result := engine.Apply(deal.Clone(), DefendCommand{Seat: 2, Card: MustJoker(3),
		Target: NewPip(King, Clubs)})
	if !result.Applied {
		t.Fatalf("движок отклонил защиту джокером: %v", result.Reason)
	}

	projection := NewStateProjection(DefaultRulesConfig(), engine)
	actions, err := projection.availableActions(deal, 2)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, action := range actions {
		if defend, ok := action.(DefendCommand); ok {
			if _, isJoker := defend.Card.(Joker); isJoker {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("в availableActions нет защиты джокером; действия: %+v", actions)
	}
}
