package game

// DealEngine — автомат раздачи: apply(state, command) -> (newState, events).
//
// ⭐ Чистая функция: ни базы, ни сети, ни фреймворка. Одно и то же состояние с той же
// командой всегда даёт тот же результат, иначе невозможны ни реплей, ни воспроизведение
// бага. Отклонённая команда состояние НЕ меняет вообще.
type DealEngine struct {
	config      RulesConfig
	moveRules   MoveRules
	hanging     hangingProvider
	attackOrder AttackOrderPolicy
	dice        dicer
}

// dicer — разрешение спора костью. Интерфейс неэкспортируемый: движку нужно ровно одно
// действие, а не вся подсистема случайности.
//
// ⚠️ Ошибка возвращается на пустом споре: бросать кость не среди кого — ошибка
// вызывающего, а не повод молча вернуть нулевое место.
type dicer interface {
	WinnerAmong(seats []int, seed int64, rollNo int) (int, error)
}

// hangingProvider — правила навеса, нужные автомату.
type hangingProvider interface {
	SeatsHoldingFlyingCard(state DealState, victimSeat int) []int
	IsEveryClaimantHanging(state DealState, victimSeat int) bool
	IsRightEqualForAll(state DealState, victimSeat int) bool
	PriorityOrder(state DealState, victimSeat int) []int
	CanHang(state DealState, seatNo, victimSeat int, card Card) MoveVerdict
}

// NewDealEngine собирает автомат из правил стола и стратегий.
func NewDealEngine(config RulesConfig, attackOrder AttackOrderPolicy, dice dicer,
	hanging hangingProvider) DealEngine {
	return DealEngine{
		config:      config,
		moveRules:   NewMoveRules(config),
		hanging:     hanging,
		attackOrder: attackOrder,
		dice:        dice,
	}
}

// Apply применяет команду к раздаче.
func (e DealEngine) Apply(state DealState, command DealCommand) MoveResult {
	if state.Phase == PhaseDealOver {
		return RejectedResult(NotYourTurn)
	}
	// В споре за козырь ничего, кроме выбора масти, делать нельзя: карт на руках
	// ещё формально нет смысла играть, пока не известен козырь.
	if _, isChoose := command.(ChooseTrumpCommand); state.Phase == PhaseDice && !isChoose {
		return RejectedResult(TrumpNotChosenYet)
	}

	switch cmd := command.(type) {
	case AttackCommand:
		return e.applyAttack(state, cmd)
	case DefendCommand:
		return e.applyDefend(state, cmd)
	case TransferCommand:
		return e.applyTransfer(state, cmd)
	case PassCommand:
		return e.applyPass(state, cmd)
	case TakeCommand:
		return e.applyTake(state, cmd)
	case HangCardCommand:
		return e.applyHangCard(state, cmd)
	case HangSkipCommand:
		return e.applyHangSkip(state, cmd)
	case ChooseTrumpCommand:
		return e.applyChooseTrump(state, cmd)
	case RevealFaceDownCommand:
		return e.applyRevealFaceDown(state, cmd)
	case RevealFaceDownToDefendCommand:
		return e.applyRevealToDefend(state, cmd)
	default:
		return RejectedResult(NotYourTurn)
	}
}

// ── Козырь ───────────────────────────────────────────────────────────────────

// applyChooseTrump — победитель кости называет масть.
//
// ⭐ Любую из четырёх, В ТОМ ЧИСЛЕ ту, которой у него на руках нет: он выбирает, а не
// показывает. Сам джокер остаётся лежать нижней картой.
func (e DealEngine) applyChooseTrump(state DealState, cmd ChooseTrumpCommand) MoveResult {
	if state.Phase != PhaseDice {
		return RejectedResult(TrumpNotInDispute)
	}
	if state.PendingHiddenTrump != nil {
		return e.chooseTrumpForHidden(state, cmd)
	}
	if state.AttackRightSeat != cmd.Seat {
		return RejectedResult(NotYourTurn)
	}

	trump := NewTrump(cmd.Suit)
	starter := lowestTrumpSeat(state, trump)
	defender, err := state.NextActiveSeatAfter(starter)
	if err != nil {
		return RejectedResult(NotYourTurn)
	}

	next := state.Clone()
	next.Trump = &trump
	next.Phase = PhaseAttack
	next.RoundStarterSeat = starter
	next.AttackRightSeat = starter
	next.DefenderSeat = defender
	return AppliedResult(next, []DealEvent{NewTrumpChosen(cmd.Seat, cmd.Suit)})
}

// chooseTrumpForHidden — потайной козырь оказался джокером.
//
// ⭐ Порядок существенный: сначала кость и выбор масти, и только ПОТОМ карта уходит в руку
// добирающему. Порядок раунда при этом уже посчитан — первый ход по младшему козырю
// определяется только при сдаче.
func (e DealEngine) chooseTrumpForHidden(state DealState, cmd ChooseTrumpCommand) MoveResult {
	pending := state.PendingHiddenTrump
	if pending.ChooserSeat != cmd.Seat {
		return RejectedResult(NotYourTurn)
	}

	trump := NewTrump(cmd.Suit)
	next := state.Clone()
	next.Trump = &trump
	next.PendingHiddenTrump = nil
	next.Phase = PhaseAttack

	recipient := next.MustPlayerAt(pending.RecipientSeat)
	hand := append(copyCards(recipient.Hand), pending.Card)
	next = next.WithPlayer(recipient.WithHand(hand))

	return AppliedResult(next, []DealEvent{
		NewTrumpChosen(cmd.Seat, cmd.Suit),
		NewCardsDrawn(pending.RecipientSeat, []Card{pending.Card}),
	})
}

// lowestTrumpSeat — первый ход у обладателя младшего козыря, уже по выбранной масти.
func lowestTrumpSeat(state DealState, trump Trump) int {
	starter := 0
	var lowest *Rank
	for _, player := range state.Players {
		for _, card := range player.Hand {
			pip, ok := card.(Pip)
			if !ok || pip.Suit != trump.Suit {
				continue
			}
			if lowest == nil || lowest.IsHigherThan(pip.Rank) {
				rank := pip.Rank
				lowest = &rank
				starter = player.SeatNo
			}
		}
	}
	return starter
}

// ── Скрытая карта ────────────────────────────────────────────────────────────

// applyRevealFaceDown — вскрыть скрытую карту и пойти ею.
//
// ⚠️ Вскрытие НЕОБРАТИМО и не зависит от исхода хода: если открытая карта не вписалась
// в атаку по рангу, ход ею не проходит, но карта уже в руке. Поэтому команда НЕ
// отклоняется — она применяется, просто с разным результатом.
func (e DealEngine) applyRevealFaceDown(state DealState, cmd RevealFaceDownCommand) MoveResult {
	if verdict := e.moveRules.CanRevealFaceDown(state, cmd.Seat); !verdict.IsAllowed() {
		return RejectedResult(verdict.Reason())
	}
	card := state.MustPlayerAt(cmd.Seat).FaceDownCard
	revealed := revealInHand(state, cmd.Seat, card)
	events := []DealEvent{FaceDownRevealed{Seat: cmd.Seat, Card: card}}

	if attack := e.applyAttack(revealed, AttackCommand{Seat: cmd.Seat, Card: card}); attack.Applied {
		return AppliedResult(attack.State, append(events, attack.Events...))
	}
	return AppliedResult(revealed, events)
}

// applyRevealToDefend — то же для защиты: карта вскрывается, а дальше либо бьёт цель,
// либо остаётся в руке.
func (e DealEngine) applyRevealToDefend(state DealState, cmd RevealFaceDownToDefendCommand) MoveResult {
	if verdict := e.moveRules.CanRevealFaceDown(state, cmd.Seat); !verdict.IsAllowed() {
		return RejectedResult(verdict.Reason())
	}
	card := state.MustPlayerAt(cmd.Seat).FaceDownCard
	revealed := revealInHand(state, cmd.Seat, card)
	events := []DealEvent{FaceDownRevealed{Seat: cmd.Seat, Card: card}}

	defence := e.applyDefend(revealed, DefendCommand{Seat: cmd.Seat, Card: card, Target: cmd.Target})
	if defence.Applied {
		return AppliedResult(defence.State, append(events, defence.Events...))
	}
	return AppliedResult(revealed, events)
}

// revealInHand — скрытая карта переезжает в руку. Обратного пути нет ни в одном сценарии.
func revealInHand(state DealState, seatNo int, card Card) DealState {
	player := state.MustPlayerAt(seatNo)
	hand := append(copyCards(player.Hand), card)
	return state.WithPlayer(player.WithFaceDownRevealed().WithHand(hand))
}

// ── Атака, защита, перевод ───────────────────────────────────────────────────

func (e DealEngine) applyAttack(state DealState, cmd AttackCommand) MoveResult {
	if verdict := e.moveRules.CanAttack(state, cmd.Seat, cmd.Card); !verdict.IsAllowed() {
		return RejectedResult(verdict.Reason())
	}
	events := []DealEvent{}
	player, events := playCard(state, cmd.Seat, cmd.Card, events)
	events = append(events, NewCardAttacked(cmd.Seat, cmd.Card))

	next := state.WithPlayer(player)
	next.Table = append(copySlots(state.Table), NewSlot(cmd.Card))
	// ⚠️ Объявленное «беру» переживает подкид: пока подкидывающие не закончатся,
	// раунд остаётся в TAKING, а не возвращается в защиту.
	if state.Phase != PhaseTaking {
		next.Phase = PhaseDefend
	}
	return AppliedResult(next, events)
}

func (e DealEngine) applyDefend(state DealState, cmd DefendCommand) MoveResult {
	if verdict := e.moveRules.CanDefend(state, cmd.Seat, cmd.Card, cmd.Target); !verdict.IsAllowed() {
		return RejectedResult(verdict.Reason())
	}
	events := []DealEvent{}
	player, events := playCard(state, cmd.Seat, cmd.Card, events)
	events = append(events, NewCardDefended(cmd.Seat, cmd.Card, cmd.Target))

	table := make([]TableSlot, 0, len(state.Table))
	for _, slot := range state.Table {
		if slot.Attack == cmd.Target {
			beaten, err := slot.BeatenWith(cmd.Card)
			if err != nil {
				return RejectedResult(TargetAlreadyBeaten)
			}
			table = append(table, beaten)
			continue
		}
		table = append(table, slot)
	}

	defended := state.WithPlayer(player)
	defended.Table = table
	defended.AnyCardBeatenThisRound = true
	if unbeatenRemain(table) {
		defended.Phase = PhaseDefend
		return AppliedResult(defended, events)
	}
	defended.Phase = PhaseAttack
	return e.afterLastCardBeaten(defended, events)
}

// afterLastCardBeaten — отбита последняя карта на столе.
//
// ⚠️ Право подкидывать обязано достаться тому, кто им реально может воспользоваться.
// Пас при неотбитом столе право никуда не двигает: двигать его некуда, следующего
// подкидывающего нет. Оно так и остаётся за спасовавшим, и после отбоя последней карты
// этот игрок не может ни подкинуть, ни спасовать ещё раз — раздача вставала намертво
// у ВСЕХ за столом. Если подкидывать больше некому, раунд здесь же и закрывается: «бито».
func (e DealEngine) afterLastCardBeaten(state DealState, events []DealEvent) MoveResult {
	if next, ok := e.attackOrder.NextAttacker(state); ok {
		if next != state.AttackRightSeat {
			events = append(events, NewAttackRightMoved(next))
		}
		moved := state.Clone()
		moved.AttackRightSeat = next
		return AppliedResult(moved, events)
	}
	events = append(events, NewRoundBeaten(state.DefenderSeat, state.TableCards()))
	finished, events := e.finishRound(clearTable(state, true), false, events)
	return AppliedResult(finished, events)
}

// applyTransfer — перевод сдвигает защиту на следующего, а сам переводящий становится
// атакующим.
//
// ⭐ Роли считаются от НОВОГО защищающегося — отсюда и вытеснение прежнего атакующего
// из раунда при четырёх и более игроках.
func (e DealEngine) applyTransfer(state DealState, cmd TransferCommand) MoveResult {
	if verdict := e.moveRules.CanTransfer(state, cmd.Seat, cmd.Card); !verdict.IsAllowed() {
		return RejectedResult(verdict.Reason())
	}
	receiver, err := state.NextActiveSeatAfter(cmd.Seat)
	if err != nil {
		return RejectedResult(NotYourTurn)
	}

	events := []DealEvent{}
	player, events := playCard(state, cmd.Seat, cmd.Card, events)
	events = append(events, NewAttackTransferred(cmd.Seat, receiver, cmd.Card))

	next := state.WithPlayer(player)
	next.Table = append(copySlots(state.Table), NewSlot(cmd.Card))
	next.RoundStarterSeat = cmd.Seat
	next.AttackRightSeat = cmd.Seat
	next.DefenderSeat = receiver
	next.PassedSeats = []int{}
	next.Phase = PhaseDefend
	return AppliedResult(next, events)
}

// ── Пас и «беру» ─────────────────────────────────────────────────────────────

// applyPass — фиксация того, что игрок больше не подкидывает.
//
// ⭐ Раунд не завершается сам по себе, пока обладатель права не спасовал.
func (e DealEngine) applyPass(state DealState, cmd PassCommand) MoveResult {
	if state.AttackRightSeat != cmd.Seat || state.HasPassed(cmd.Seat) {
		return RejectedResult(NotYourTurn)
	}
	// ⚠️ Со скрытой картой и пустым столом пасовать нельзя: иначе игрок унёс бы её
	// в следующий раунд, хотя обязан вскрыть.
	if len(state.Table) == 0 && state.MustPlayerAt(cmd.Seat).CanPlayFaceDown(state.IsDeckEmpty()) {
		return RejectedResult(MustRevealFaceDown)
	}

	afterPass := state.WithPassed(cmd.Seat)
	events := []DealEvent{NewPassed(cmd.Seat)}

	if next, ok := e.attackOrder.NextAttacker(afterPass); ok {
		events = append(events, NewAttackRightMoved(next))
		moved := afterPass.Clone()
		moved.AttackRightSeat = next
		moved.Phase = nextPhaseAfterPass(afterPass)
		return AppliedResult(moved, events)
	}
	if afterPass.Phase == PhaseTaking {
		collected, events := e.collectTable(afterPass, events)
		return AppliedResult(collected, events)
	}
	if unbeatenRemain(afterPass.Table) {
		return AppliedResult(afterPass.WithPhase(PhaseDefend), events)
	}
	events = append(events, NewRoundBeaten(afterPass.DefenderSeat, afterPass.TableCards()))
	finished, events := e.finishRound(clearTable(afterPass, true), false, events)
	return AppliedResult(finished, events)
}

// applyTake — «беру» раунд НЕ закрывает: подкидывающие докидывают карты, пока не спасуют,
// и только тогда стол уезжает в руку.
//
// Потолком остаётся лимит раунда — рука взявшего больше ничего не ограничивает,
// он всё равно заберёт всё.
func (e DealEngine) applyTake(state DealState, cmd TakeCommand) MoveResult {
	if state.DefenderSeat != cmd.Seat || state.Phase == PhaseTaking {
		return RejectedResult(NotYourTurn)
	}
	if len(state.Table) == 0 {
		return RejectedResult(NothingToTake)
	}

	events := []DealEvent{NewTakeAnnounced(cmd.Seat)}
	taking := state.WithPhase(PhaseTaking)
	if _, ok := e.attackOrder.NextAttacker(taking); ok {
		return AppliedResult(taking, events)
	}
	collected, events := e.collectTable(taking, events)
	return AppliedResult(collected, events)
}

// collectTable — стол уезжает в руку взявшему, и раунд закрывается.
func (e DealEngine) collectTable(state DealState, events []DealEvent) (DealState, []DealEvent) {
	taken := state.TableCards()
	defender := state.Defender()
	next := state.WithPlayer(defender.WithHand(append(copyCards(defender.Hand), taken...)))
	events = append(events, NewCardsTaken(state.DefenderSeat, taken))
	return e.openHangingWindow(clearTable(next, false), events)
}

// ── Навесы ───────────────────────────────────────────────────────────────────

// openHangingWindow — окно открывается на взявшего, и только СЕЙЧАС, когда состав его
// руки уже окончателен.
//
// Если нужной карты нет ни у кого, окно не открывается вовсе: навес просто не происходит.
func (e DealEngine) openHangingWindow(state DealState, events []DealEvent) (DealState, []DealEvent) {
	if !e.config.NavesEnabled || e.hanging == nil {
		return e.finishRound(state, true, events)
	}
	victim := state.DefenderSeat
	holders := e.hanging.SeatsHoldingFlyingCard(state, victim)
	if len(holders) == 0 {
		return e.finishRound(state, true, events)
	}

	events = append(events, NewHangingWindowOpened(victim))
	window := OpenHangingWindow(victim, e.hangingSteps(state, victim, holders),
		e.hanging.IsEveryClaimantHanging(state, victim))
	next := state.Clone()
	next.Phase = PhaseHanging
	next.HangingWindow = &window
	return next, events
}

// hangingSteps — ступени права.
//
// При джокере и при уникальном отстающем ступень одна — право сразу у всех. В обычном
// случае их три: атаковавший, поддержавший и все остальные наравне.
func (e DealEngine) hangingSteps(state DealState, victim int, holders []int) [][]int {
	if e.hanging.IsRightEqualForAll(state, victim) {
		return [][]int{holders}
	}
	priority := e.hanging.PriorityOrder(state, victim)
	steps := make([][]int, 0, 3)
	for tier := 0; tier < 2 && tier < len(priority); tier++ {
		seat := priority[tier]
		if containsInt(holders, seat) {
			steps = append(steps, []int{seat})
		}
	}
	rest := make([]int, 0, len(holders))
	for _, seat := range holders {
		if indexOfInt(priority, seat) >= 2 {
			rest = append(rest, seat)
		}
	}
	if len(rest) > 0 {
		steps = append(steps, rest)
	}
	return steps
}

func (e DealEngine) applyHangCard(state DealState, cmd HangCardCommand) MoveResult {
	window := state.HangingWindow
	if state.Phase != PhaseHanging || window == nil || !window.IsSeatOnCurrentStep(cmd.Seat) {
		return RejectedResult(NotInHangingWindow)
	}
	if verdict := e.hanging.CanHang(state, cmd.Seat, window.VictimSeat, cmd.Card); !verdict.IsAllowed() {
		return RejectedResult(verdict.Reason())
	}
	claimed := window.WithClaim(HangClaim{SeatNo: cmd.Seat, Card: cmd.Card})
	next := state.Clone()
	next.HangingWindow = &claimed
	advanced, events := e.advanceWindow(next, []DealEvent{})
	return AppliedResult(advanced, events)
}

func (e DealEngine) applyHangSkip(state DealState, cmd HangSkipCommand) MoveResult {
	window := state.HangingWindow
	if state.Phase != PhaseHanging || window == nil || !window.IsSeatOnCurrentStep(cmd.Seat) {
		return RejectedResult(NotInHangingWindow)
	}
	declined := window.WithDecline(cmd.Seat)
	next := state.Clone()
	next.HangingWindow = &declined
	advanced, events := e.advanceWindow(next, []DealEvent{})
	return AppliedResult(advanced, events)
}

// advanceWindow — ступень исчерпана, разрешаем её.
//
// Заявок нет: право уходит дальше по очереди, а если очередь кончилась — окно
// закрывается без навеса.
func (e DealEngine) advanceWindow(state DealState, events []DealEvent) (DealState, []DealEvent) {
	window := state.HangingWindow
	if !window.IsStepComplete() {
		return state, events
	}
	if len(window.Claims) == 0 {
		if window.HasNextStep() {
			next := state.Clone()
			stepped := window.NextStep()
			next.HangingWindow = &stepped
			return next, events
		}
		return e.closeWindow(state, events)
	}
	applied, events := e.applyClaims(state, *window, events)
	return e.closeWindow(applied, events)
}

// applyClaims — уровень поднимается ровно на ОДНУ ступень за окно, сколько бы карт
// в слот ни ушло.
//
// ⚠️ При правиле отстающего навешивают все заявившиеся; в остальных случаях — один,
// и спор решается КОСТЬЮ, а не тем, кто успел нажать.
func (e DealEngine) applyClaims(state DealState, window HangingWindow,
	events []DealEvent) (DealState, []DealEvent) {
	winners, events := e.selectWinners(state, window, events)
	current := state
	for _, claim := range winners {
		current = current.WithPlayer(current.MustPlayerAt(claim.SeatNo).WithoutCard(claim.Card))
		current = current.WithPlayer(
			current.MustPlayerAt(window.VictimSeat).WithHungCard(claim.Card, claim.SeatNo))
		events = append(events, NewCardHung(claim.SeatNo, window.VictimSeat, claim.Card))
	}
	level := current.MustPlayerAt(window.VictimSeat).NavesLevel + 1
	events = append(events, NewNavesLevelChanged(window.VictimSeat, level))
	return current.WithPlayer(current.MustPlayerAt(window.VictimSeat).WithNavesLevel(level)), events
}

func (e DealEngine) selectWinners(state DealState, window HangingWindow,
	events []DealEvent) ([]HangClaim, []DealEvent) {
	if window.EveryClaimantHangs || len(window.Claims) == 1 {
		return window.Claims, events
	}
	participants := make([]int, 0, len(window.Claims))
	for _, claim := range window.Claims {
		participants = append(participants, claim.SeatNo)
	}
	winner, err := e.dice.WinnerAmong(participants, state.RngSeed, state.DiceRolls)
	if err != nil {
		// Спор без участников невозможен: сюда мы попадаем, только когда заявок больше
		// одной. Но молча выбрать «нулевого» нельзя — берём первого заявившегося.
		winner = participants[0]
	}
	events = append(events, NewDiceRolled(winner, participants))

	chosen := make([]HangClaim, 0, 1)
	for _, claim := range window.Claims {
		if claim.SeatNo == winner {
			chosen = append(chosen, claim)
		}
	}
	return chosen, events
}

func (e DealEngine) closeWindow(state DealState, events []DealEvent) (DealState, []DealEvent) {
	window := state.HangingWindow
	events = append(events, NewHangingWindowClosed(window.VictimSeat))
	next := state.Clone()
	next.HangingWindow = nil
	if len(window.Claims) > 1 {
		next.DiceRolls = state.DiceRolls + 1
	}
	return e.finishRound(next, true, events)
}

// ── Закрытие раунда ──────────────────────────────────────────────────────────

// finishRound — стол в отбой или в руку, добор в строгом порядке, выход игроков,
// проверка конца раздачи.
//
// ⚠️ Порядок шагов НЕ переставляется: добор обязан случиться до проверки выхода, иначе
// игрок вышел бы, не получив причитающихся карт.
func (e DealEngine) finishRound(state DealState, taken bool,
	events []DealEvent) (DealState, []DealEvent) {
	refilled, events := e.refill(state, events)
	afterExits, events := e.markExits(refilled, events)

	if afterExits.PlayersInDeal() <= 1 {
		loser := lastPlayerInDeal(afterExits)
		events = append(events, NewDealFinished(loser))
		return afterExits.WithPhase(PhaseDealOver), events
	}

	nextRound := e.startNextRound(afterExits, taken)
	if nextRound.PendingHiddenTrump != nil {
		return nextRound.WithPhase(PhaseDice), events
	}
	return nextRound, events
}

// clearTable — стол уезжает ровно в тот момент, когда раунд закрылся: в отбой или в руку.
//
// ⚠️ Раньше это делалось в конце закрытия, и между «взял» и очисткой успевало вклиниться
// окно навеса: карты одновременно лежали и в руке взявшего, и на столе.
//
// Заодно запоминается состав последней атаки: он нужен для степеней проигрыша,
// и после очистки его уже не восстановить.
func clearTable(state DealState, discarded bool) DealState {
	next := state.Clone()
	last := make([]Card, 0, len(state.Table))
	for _, slot := range state.Table {
		last = append(last, slot.Attack)
	}
	next.LastAttackCards = last
	next.Table = []TableSlot{}
	next.AnyPileDiscarded = state.AnyPileDiscarded || discarded
	return next
}

// refill — добор до размера раздачи в строгом порядке.
//
// ⚠️ Порядок не формальность: колода конечна, и тот, кто добирает раньше, успевает взять
// карты, которых не хватит остальным.
func (e DealEngine) refill(state DealState, events []DealEvent) (DealState, []DealEvent) {
	deck := copyCards(state.Deck)
	current := state
	var pending *PendingHiddenTrump
	var newTrump *Trump

	for _, seat := range refillOrder(state) {
		player := current.MustPlayerAt(seat)
		drawn := make([]Card, 0, e.config.DealSize)

		for player.HandSize()+len(drawn) < e.config.DealSize && len(deck) > 0 {
			lastCard := len(deck) == 1
			card := deck[0]
			deck = deck[1:]
			if !lastCard {
				drawn = append(drawn, card)
				continue
			}
			// ⭐ Нижняя карта колоды — потайной козырь. Событие ПУБЛИЧНОЕ, с картой:
			// она меняет козырь всему столу.
			events = append(events, NewHiddenTrumpRevealed(seat, card))
			if pip, ok := card.(Pip); ok {
				trump := NewTrump(pip.Suit)
				newTrump = &trump
				events = append(events, NewTrumpChanged(seat, pip.Suit))
				drawn = append(drawn, card)
			} else {
				chooser, err := e.dice.WinnerAmong(seatsInDeal(current), current.RngSeed, current.DiceRolls)
				if err != nil {
					// В раздаче всегда есть хотя бы один игрок; запасной путь — сам добирающий.
					chooser = seat
				}
				pending = &PendingHiddenTrump{Card: card, RecipientSeat: seat, ChooserSeat: chooser}
			}
		}

		if len(drawn) == 0 {
			continue
		}
		events = append(events, NewCardsDrawn(seat, drawn))
		current = current.WithPlayer(player.WithHand(append(copyCards(player.Hand), drawn...)))
	}

	next := current.Clone()
	next.Deck = deck
	if newTrump != nil {
		next.Trump = newTrump
	}
	if pending != nil {
		next.PendingHiddenTrump = pending
		next.DiceRolls = current.DiceRolls + 1
	}
	return next, events
}

func seatsInDeal(state DealState) []int {
	seats := make([]int, 0, len(state.Players))
	for _, player := range state.Players {
		if player.InDeal {
			seats = append(seats, player.SeatNo)
		}
	}
	return seats
}

// refillOrder — начавший раунд → второй сосед → остальные по часовой → защищавшийся последним.
func refillOrder(state DealState) []int {
	order := make([]int, 0, len(state.Players))
	add := func(seat int) {
		if seat == state.DefenderSeat || containsInt(order, seat) {
			return
		}
		if player, err := state.PlayerAt(seat); err == nil && player.InDeal {
			order = append(order, seat)
		}
	}

	add(state.RoundStarterSeat)
	if second, err := state.NextActiveSeatAfter(state.DefenderSeat); err == nil {
		add(second)
	}
	for step := 1; step <= len(state.Players); step++ {
		add((state.RoundStarterSeat + step) % len(state.Players))
	}
	if state.Defender().InDeal {
		order = append(order, state.DefenderSeat)
	}
	return order
}

// markExits — выход игроков: без карт при пустой колоде и только если нет скрытой карты.
//
// ⭐ Порядок проверки — тот же, что у добора, и это и есть правило «атакующий опережает
// защищающегося»: карты атакующего ложатся на стол раньше отбивающих, поэтому и выходит
// он первым. Последний оставшийся не выходит НИКОГДА — он и есть «дурак» раздачи,
// даже если его рука тоже опустела.
func (e DealEngine) markExits(state DealState, events []DealEvent) (DealState, []DealEvent) {
	exitOrder := append([]int(nil), state.ExitOrder...)
	current := state

	for _, seat := range refillOrder(state) {
		if current.PlayersInDeal() <= 1 {
			break
		}
		player := current.MustPlayerAt(seat)
		if !player.InDeal || !current.IsDeckEmpty() || player.HandSize() > 0 || player.HasFaceDownCard() {
			continue
		}
		exitOrder = append(exitOrder, seat)
		events = append(events, NewPlayerLeftDeal(seat))
		current = current.WithPlayer(player.LeftDeal())
	}

	next := current.Clone()
	next.ExitOrder = exitOrder
	return next, events
}

// startNextRound — после «бито» начинает защищавшийся, после «взял» — следующий за ним
// по кругу, а сам он раунд пропускает.
func (e DealEngine) startNextRound(state DealState, taken bool) DealState {
	starter := state.DefenderSeat
	if taken || !state.Defender().InDeal {
		if next, err := state.NextActiveSeatAfter(state.DefenderSeat); err == nil {
			starter = next
		}
	}
	defender := starter
	if next, err := state.NextActiveSeatAfter(starter); err == nil {
		defender = next
	}

	result := state.Clone()
	result.Phase = PhaseAttack
	result.RoundStarterSeat = starter
	result.AttackRightSeat = starter
	result.DefenderSeat = defender
	result.PassedSeats = []int{}
	result.AnyCardBeatenThisRound = false
	return result
}

func lastPlayerInDeal(state DealState) int {
	for _, player := range state.Players {
		if player.InDeal {
			return player.SeatNo
		}
	}
	return 0
}

// playCard — убирает сыгранную карту из руки или вскрывает скрытую.
//
// ⚠️ Открытие необратимо, поэтому событие о вскрытии рождается ЗДЕСЬ, а не в проверке
// легальности: проверка не меняет состояние, а вскрытие — меняет.
func playCard(state DealState, seatNo int, card Card, events []DealEvent) (PlayerState, []DealEvent) {
	player := state.MustPlayerAt(seatNo)
	if player.HoldsInHand(card) {
		return player.WithoutCard(card), events
	}
	events = append(events, FaceDownRevealed{Seat: seatNo, Card: card})
	return player.WithFaceDownRevealed(), events
}

// nextPhaseAfterPass — фаза после паса, когда право ушло дальше.
//
// ⭐ Объявленное «беру» переживает пас: пока подкидывающие не закончатся, раунд остаётся
// в TAKING.
func nextPhaseAfterPass(state DealState) DealPhase {
	if state.Phase == PhaseTaking {
		return PhaseTaking
	}
	if unbeatenRemain(state.Table) {
		return PhaseDefend
	}
	return PhaseAttack
}

func unbeatenRemain(table []TableSlot) bool {
	for _, slot := range table {
		if !slot.IsBeaten() {
			return true
		}
	}
	return false
}

func copySlots(slots []TableSlot) []TableSlot {
	out := make([]TableSlot, len(slots))
	copy(out, slots)
	return out
}

func indexOfInt(values []int, value int) int {
	for index, candidate := range values {
		if candidate == value {
			return index
		}
	}
	return -1
}
