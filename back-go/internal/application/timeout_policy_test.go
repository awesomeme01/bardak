package application

import (
	"testing"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// Автодействие по таймауту (§5.1).
//
// ⭐ Проверяется не «что-то произошло», а что произошло САМОЕ БЕЗОБИДНОЕ: сервер не
// выбирает за человека карту. Разъедься это правило — и партия начнёт играть себя сама.

// dealForTimeout — раздача с умолчаниями: трое, козырь черви, атакует место 0,
// отбивается место 1. Тест меняет только то, что проверяет.
func dealForTimeout() game.DealState {
	trump := game.NewTrump(game.Hearts)
	return game.DealState{
		Phase: game.PhaseAttack,
		Trump: &trump,
		Deck:  []game.Card{game.NewPip(game.Six, game.Clubs)},
		Players: []game.PlayerState{
			game.NewPlayerState(0, nil, nil),
			game.NewPlayerState(1, nil, nil),
			game.NewPlayerState(2, nil, nil),
		},
		Table:            []game.TableSlot{},
		RoundStarterSeat: 0,
		AttackRightSeat:  0,
		DefenderSeat:     1,
		PassedSeats:      []int{},
		ExitOrder:        []int{},
		LastAttackCards:  []game.Card{},
		RngSeed:          42,
	}
}

func TestTimeoutPassesForTheSilentAttacker(t *testing.T) {
	deal := dealForTimeout()

	seat, ok := SeatOnTheClock(deal)
	if !ok || seat != deal.AttackRightSeat {
		t.Fatalf("на часах место %d (%v), ждали обладателя права атаки", seat, ok)
	}

	command, ok := AutoActionFor(deal)
	if !ok {
		t.Fatalf("сервер не сделал ничего за молчащего")
	}
	if pass, isPass := command.(game.PassCommand); !isPass || pass.Seat != deal.AttackRightSeat {
		t.Fatalf("за атакующего сходили %T, ждали пас", command)
	}
}

func TestTimeoutTakesForTheSilentDefender(t *testing.T) {
	deal := dealForTimeout()
	deal.Phase = game.PhaseDefend
	deal.Table = []game.TableSlot{{Attack: game.NewPip(game.Seven, game.Diamonds)}}

	command, ok := AutoActionFor(deal)

	if !ok {
		t.Fatalf("сервер не сделал ничего за защищающегося")
	}
	if take, isTake := command.(game.TakeCommand); !isTake || take.Seat != deal.DefenderSeat {
		t.Fatalf("за защищающегося сходили %T, ждали «беру»", command)
	}
}

// ⚠️ Пустой стол «взять» не даёт: правило пустого стола, и остаётся пас. Наивное
// «защищающийся всегда берёт» отдало бы движку невозможную команду.
func TestTimeoutPassesInsteadOfTakingWhenTableIsEmpty(t *testing.T) {
	deal := dealForTimeout()
	deal.Phase = game.PhaseDefend

	command, ok := AutoActionFor(deal)

	if !ok {
		t.Fatalf("сервер не сделал ничего за защищающегося")
	}
	if pass, isPass := command.(game.PassCommand); !isPass || pass.Seat != deal.DefenderSeat {
		t.Fatalf("на пустом столе сходили %T, ждали пас", command)
	}
}

func TestTimeoutSkipsTheHangingWhenNobodyDecides(t *testing.T) {
	deal := dealForTimeout()
	deal.Phase = game.PhaseHanging
	window := game.OpenHangingWindow(1, [][]int{{0, 2}}, false)
	deal.HangingWindow = &window

	seat, onClock := SeatOnTheClock(deal)
	if !onClock || seat != 0 {
		t.Fatalf("на часах место %d (%v), ждали первого нерешившего", seat, onClock)
	}

	command, ok := AutoActionFor(deal)
	if !ok {
		t.Fatalf("сервер не закрыл окно навеса")
	}
	if skip, isSkip := command.(game.HangSkipCommand); !isSkip || skip.Seat != 0 {
		t.Fatalf("в окне навеса сходили %T, ждали пропуск", command)
	}
}

// Уже решившее место на часах не стоит: ждут следующего, а не того же самого.
func TestTimeoutWaitsForTheNextUndecidedSeatInTheHangingWindow(t *testing.T) {
	deal := dealForTimeout()
	deal.Phase = game.PhaseHanging
	window := game.OpenHangingWindow(1, [][]int{{0, 2}}, false).WithDecline(0)
	deal.HangingWindow = &window

	seat, ok := SeatOnTheClock(deal)

	if !ok || seat != 2 {
		t.Fatalf("на часах место %d (%v), ждали место 2", seat, ok)
	}
}

// ⭐ Кость брошена за молчащего, но масть — уже решение: берётся самая многочисленная
// масть в его руке, то есть то, что человек и выбрал бы, глядя в свои карты.
func TestTimeoutChoosesTheRichestSuitForTheSilentChooser(t *testing.T) {
	deal := dealForTimeout()
	deal.Phase = game.PhaseDice
	deal.Trump = nil
	deal.PendingHiddenTrump = &game.PendingHiddenTrump{
		Card: game.MustJoker(1), RecipientSeat: 2, ChooserSeat: 2}
	deal = deal.WithPlayer(deal.MustPlayerAt(2).WithHand([]game.Card{
		game.NewPip(game.Six, game.Clubs),
		game.NewPip(game.Seven, game.Clubs),
		game.NewPip(game.Ace, game.Diamonds),
	}))

	command, ok := AutoActionFor(deal)

	if !ok {
		t.Fatalf("сервер не выбрал масть за молчащего")
	}
	choose, isChoose := command.(game.ChooseTrumpCommand)
	if !isChoose {
		t.Fatalf("за победителя кости сходили %T, ждали выбор масти", command)
	}
	if choose.Seat != 2 || choose.Suit != game.Clubs {
		t.Fatalf("выбрана масть %v на месте %d, ждали трефы у места 2", choose.Suit, choose.Seat)
	}
}

// ⚠️ Равенство мастей решается МЛАДШЕЙ по порядку объявления — бубнами, а не трефами.
// В Java это спрятано в `max` по компаратору с ОБРАТНЫМ порядком по ordinal: выглядит
// как «старшая масть», а выбирает первую. Расхождение здесь всплыло бы только на ничьей.
func TestTimeoutBreaksASuitTieByTheLowerSuit(t *testing.T) {
	deal := dealForTimeout()
	deal.Phase = game.PhaseDice
	deal.Trump = nil
	deal.PendingHiddenTrump = &game.PendingHiddenTrump{
		Card: game.MustJoker(1), RecipientSeat: 0, ChooserSeat: 0}
	deal = deal.WithPlayer(deal.MustPlayerAt(0).WithHand([]game.Card{
		game.NewPip(game.Six, game.Diamonds),
		game.NewPip(game.Seven, game.Clubs),
	}))

	command, _ := AutoActionFor(deal)

	choose, isChoose := command.(game.ChooseTrumpCommand)
	if !isChoose || choose.Suit != game.Diamonds {
		t.Fatalf("на равенстве выбрана %v, ждали младшую масть — бубны", command)
	}
}

// Раздача кончилась — ждать некого, и часы заводить не на кого.
func TestTimeoutWaitsForNobodyWhenTheDealIsOver(t *testing.T) {
	deal := dealForTimeout()
	deal.Phase = game.PhaseDealOver

	if _, ok := SeatOnTheClock(deal); ok {
		t.Fatalf("в законченной раздаче кто-то оказался на часах")
	}
	if _, ok := AutoActionFor(deal); ok {
		t.Fatalf("в законченной раздаче сервер попытался сходить")
	}
}
