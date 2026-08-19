package game

// Сборка снимка раздачи для тестов.
//
// ⭐ Нужна затем, чтобы в самом тесте оставалось только то, что он проверяет. Остальное
// берётся из осмысленных умолчаний: трое игроков, козырь черви, колода не пуста, стол
// пуст, атакует место 0, отбивается место 1.
//
// Перенос DealStateFixture из Java.

type dealFixture struct {
	state DealState
}

// aDeal — раздача с умолчаниями.
func aDeal() *dealFixture {
	trump := NewTrump(Hearts)
	return &dealFixture{state: DealState{
		Phase: PhaseAttack,
		Trump: &trump,
		Deck:  []Card{NewPip(Six, Clubs)},
		Players: []PlayerState{
			NewPlayerState(0, nil, nil),
			NewPlayerState(1, nil, nil),
			NewPlayerState(2, nil, nil),
		},
		Table:            []TableSlot{},
		RoundStarterSeat: 0,
		AttackRightSeat:  0,
		DefenderSeat:     1,
		PassedSeats:      []int{},
		ExitOrder:        []int{},
		LastAttackCards:  []Card{},
		RngSeed:          42,
	}}
}

func (f *dealFixture) withPlayers(count int) *dealFixture {
	players := make([]PlayerState, count)
	for seat := 0; seat < count; seat++ {
		players[seat] = NewPlayerState(seat, nil, nil)
	}
	f.state.Players = players
	return f
}

// withHand — карты в руке игрока; остальные поля игрока не трогаются.
func (f *dealFixture) withHand(seatNo int, cards ...Card) *dealFixture {
	f.state = f.state.WithPlayer(f.state.MustPlayerAt(seatNo).WithHand(cards))
	return f
}

func (f *dealFixture) withFaceDown(seatNo int, card Card) *dealFixture {
	player := f.state.MustPlayerAt(seatNo)
	player.FaceDownCard = card
	f.state = f.state.WithPlayer(player)
	return f
}

func (f *dealFixture) withOutOfDeal(seatNo int) *dealFixture {
	f.state = f.state.WithPlayer(f.state.MustPlayerAt(seatNo).LeftDeal())
	return f
}

func (f *dealFixture) withNavesLevel(seatNo, level int) *dealFixture {
	f.state = f.state.WithPlayer(f.state.MustPlayerAt(seatNo).WithNavesLevel(level))
	return f
}

// withJokerHungBy — джокер навешен жертве конкретным игроком: тот получит −1,
// если жертва проиграет.
func (f *dealFixture) withJokerHungBy(victimSeat, hangerSeat int) *dealFixture {
	victim := f.state.MustPlayerAt(victimSeat).
		WithNavesLevel(FullNavesScale().JokerLevel()).
		WithHungCard(MustJoker(hangerSeat+1), hangerSeat)
	f.state = f.state.WithPlayer(victim)
	return f
}

func (f *dealFixture) withPhase(phase DealPhase) *dealFixture {
	f.state.Phase = phase
	return f
}

func (f *dealFixture) withTrump(suit Suit) *dealFixture {
	trump := NewTrump(suit)
	f.state.Trump = &trump
	return f
}

func (f *dealFixture) withEmptyDeck() *dealFixture {
	f.state.Deck = []Card{}
	return f
}

func (f *dealFixture) withDeck(cards ...Card) *dealFixture {
	f.state.Deck = cards
	return f
}

// withAttack — неотбитые атакующие карты на столе.
func (f *dealFixture) withAttack(cards ...Card) *dealFixture {
	slots := make([]TableSlot, 0, len(cards))
	for _, card := range cards {
		slots = append(slots, NewSlot(card))
	}
	f.state.Table = append(f.state.Table, slots...)
	return f
}

// withBeaten — пара «атака отбита защитой».
func (f *dealFixture) withBeaten(attack, defence Card) *dealFixture {
	f.state.Table = append(f.state.Table, TableSlot{Attack: attack, Defence: defence})
	f.state.AnyCardBeatenThisRound = true
	return f
}

func (f *dealFixture) withAttackRight(seatNo int) *dealFixture {
	f.state.AttackRightSeat = seatNo
	return f
}

func (f *dealFixture) withDefender(seatNo int) *dealFixture {
	f.state.DefenderSeat = seatNo
	return f
}

func (f *dealFixture) withPassed(seats ...int) *dealFixture {
	f.state.PassedSeats = seats
	return f
}

func (f *dealFixture) withPileDiscarded() *dealFixture {
	f.state.AnyPileDiscarded = true
	return f
}

func (f *dealFixture) withExitOrder(seats ...int) *dealFixture {
	f.state.ExitOrder = seats
	return f
}

func (f *dealFixture) withLastAttack(cards ...Card) *dealFixture {
	f.state.LastAttackCards = cards
	return f
}

func (f *dealFixture) build() DealState { return f.state.Clone() }

func (f *dealFixture) withRoundStarter(seatNo int) *dealFixture {
	f.state.RoundStarterSeat = seatNo
	return f
}
