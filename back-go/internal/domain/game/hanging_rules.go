package game

// HangingRules — правила навеса, центральной механики бардака (§2.3).
//
// Навесить = положить карту из своей руки в чужой слот. Выгода двойная: избавляешься
// от карты и продвигаешь соперника к джокеру. Поэтому навес — возможность, а не обязанность.
//
// ⭐ Право навесить устроено тремя разными способами, и какой из них включится, решает
// не роль игрока, а положение жертвы:
//
//  1. джокер — право сразу у всех, приоритета нет вообще;
//  2. уникальный отстающий — право у всех, и навешивают все сразу;
//  3. обычный случай — очередь: атаковавший → поддержавший → остальные наравне.
//
// Как и MoveRules, это чистые проверки над снимком раздачи: очередь ведёт автомат,
// здесь только то, что зависит от карт и от положения игроков.
type HangingRules struct {
	Config RulesConfig
}

// NewHangingRules собирает правила навеса для правил стола.
func NewHangingRules(config RulesConfig) HangingRules { return HangingRules{Config: config} }

// IsFlyingCard — подходит ли карта под текущую ступень жертвы. Масть не важна.
func (r HangingRules) IsFlyingCard(victim PlayerState, card Card) bool {
	return r.Config.NavesScale.IsFlyingCard(victim.NavesLevel, card)
}

// NextIsJoker — следующая ступень джокер: право навесить сразу у всех (§2.3).
func (r HangingRules) NextIsJoker(victim PlayerState) bool {
	return r.Config.NavesScale.NextIsJoker(victim.NavesLevel)
}

// IsUniqueLaggard — жертва отстающая: у неё самый низкий уровень среди оставшихся
// в раздаче, и такая она ОДНА.
//
// ⭐ Разделённый минимум правило не включает — навес тогда обычный (ADR-028).
//
// ⚠️ Вышедшие в сравнении не участвуют, даже если их уровень ниже всех: добивать
// того, кто уже сбросил карты, незачем.
func (r HangingRules) IsUniqueLaggard(state DealState, victimSeat int) bool {
	victimLevel := state.MustPlayerAt(victimSeat).NavesLevel
	for _, player := range state.Players {
		if !player.InDeal || player.SeatNo == victimSeat {
			continue
		}
		if player.NavesLevel <= victimLevel {
			return false
		}
	}
	return true
}

// IsRightEqualForAll — право равно у всех: навешивают либо джокер, либо уникальному
// отстающему. В обоих случаях приоритет не действует.
func (r HangingRules) IsRightEqualForAll(state DealState, victimSeat int) bool {
	return r.NextIsJoker(state.MustPlayerAt(victimSeat)) || r.IsUniqueLaggard(state, victimSeat)
}

// IsEveryClaimantHanging — правило отстающего: его добивают всем столом, карты в слот
// уходят от каждого желающего, а уровень поднимается ровно на одну ступень.
//
// ⚠️ Джокер так не работает — он один и решает исход, поэтому при джокере правило
// выключено, даже если жертва вдобавок отстающая.
func (r HangingRules) IsEveryClaimantHanging(state DealState, victimSeat int) bool {
	return !r.NextIsJoker(state.MustPlayerAt(victimSeat)) && r.IsUniqueLaggard(state, victimSeat)
}

// PriorityOrder — очередь права в обычном случае (§2.3): атаковавший (начавший раунд),
// затем поддержавший (сосед жертвы), затем все остальные наравне.
//
// Жертва и вышедшие из раздачи в очередь не попадают, повторов в ней нет.
func (r HangingRules) PriorityOrder(state DealState, victimSeat int) []int {
	order := make([]int, 0, len(state.Players))
	order = r.addCandidate(state, order, victimSeat, state.RoundStarterSeat)
	// ⚠️ Ошибка здесь означает, что в раздаче не осталось активных мест после жертвы:
	// добавлять тогда всё равно некого, а падать посреди проверки правил незачем.
	if supporter, err := state.NextActiveSeatAfter(victimSeat); err == nil {
		order = r.addCandidate(state, order, victimSeat, supporter)
	}
	size := len(state.Players)
	for step := 1; step <= size; step++ {
		order = r.addCandidate(state, order, victimSeat, (state.RoundStarterSeat+step)%size)
	}
	return order
}

// SeatsHoldingFlyingCard — кто вообще способен навесить: держит нужную карту и это
// не сама жертва. Порядок — приоритетный.
func (r HangingRules) SeatsHoldingFlyingCard(state DealState, victimSeat int) []int {
	victim := state.MustPlayerAt(victimSeat)
	holders := make([]int, 0, len(state.Players))
	for _, seat := range r.PriorityOrder(state, victimSeat) {
		for _, card := range state.MustPlayerAt(seat).Hand {
			if r.IsFlyingCard(victim, card) {
				holders = append(holders, seat)
				break
			}
		}
	}
	return holders
}

// Steps — ступени права для окна навеса.
//
// ⭐ При джокере и при уникальном отстающем ступень одна: право сразу у всех.
// В обычном случае их до трёх — атаковавший, поддержавший и все остальные наравне;
// ступень пропускается, если у приоритетного игрока нужной карты нет.
//
// ⚠️ Ступень из нескольких мест разрешается КОСТЬЮ, а не «кто первый успел»: последнее —
// состязание пинга, а не игры (ADR-029). Поэтому остальные и склеены в ОДНУ ступень,
// а не разложены по одному.
func (r HangingRules) Steps(state DealState, victimSeat int, holders []int) [][]int {
	if r.IsRightEqualForAll(state, victimSeat) {
		return [][]int{append([]int(nil), holders...)}
	}
	priority := r.PriorityOrder(state, victimSeat)
	steps := make([][]int, 0, 3)
	for tier := 0; tier < 2 && tier < len(priority); tier++ {
		seat := priority[tier]
		if containsInt(holders, seat) {
			steps = append(steps, []int{seat})
		}
	}
	// Всё, что дальше поддержавшего, — одна ступень «остальные наравне».
	var beyondSupporter []int
	if len(priority) > 2 {
		beyondSupporter = priority[2:]
	}
	rest := make([]int, 0, len(holders))
	for _, seat := range holders {
		if containsInt(beyondSupporter, seat) {
			rest = append(rest, seat)
		}
	}
	if len(rest) > 0 {
		steps = append(steps, rest)
	}
	return steps
}

// CanHang — можно ли навесить эту карту.
//
// ⭐ Проверка не смотрит на очередь: очередь ведёт автомат через HangingWindow, а здесь
// только то, что зависит от карт и от положения игроков.
func (r HangingRules) CanHang(state DealState, seatNo, victimSeat int, card Card) MoveVerdict {
	if !r.Config.NavesEnabled {
		return Rejected(NavesDisabled)
	}
	if seatNo == victimSeat {
		return Rejected(CannotHangOnSelf)
	}
	hanger, err := state.PlayerAt(seatNo)
	if err != nil {
		// ⚠️ В Java место вне стола — исключение. Здесь это отказ: правила не должны
		// ронять раздачу из-за кривой команды клиента.
		return Rejected(NotInHangingWindow)
	}
	if !hanger.HoldsInHand(card) {
		return Rejected(CardNotInHand)
	}
	victim, err := state.PlayerAt(victimSeat)
	if err != nil {
		return Rejected(NotInHangingWindow)
	}
	if !r.IsFlyingCard(victim, card) {
		return Rejected(CardNotOnNavesScale)
	}
	return Allowed()
}

// addCandidate — место идёт в очередь, если это не жертва, оно ещё в раздаче
// и его там ещё нет.
func (r HangingRules) addCandidate(state DealState, order []int, victimSeat, seat int) []int {
	if seat == victimSeat || containsInt(order, seat) {
		return order
	}
	player, err := state.PlayerAt(seat)
	if err != nil || !player.InDeal {
		return order
	}
	return append(order, seat)
}
