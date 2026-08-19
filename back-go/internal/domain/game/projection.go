package game

import "fmt"

// MoveApplier — движок правил в том объёме, в каком он нужен проекции.
//
// ⭐ Проекция зависит от интерфейса, а не от конкретного движка, ровно по одной причине:
// список доступных действий она считает ПЕРЕБОРОМ ЧЕРЕЗ ДВИЖОК — команда попадает
// в список, только если движок принял её на копии состояния. Поэтому список не может
// разойтись с правилами: он и есть правила.
type MoveApplier interface {
	// Apply применяет команду к состоянию, не меняя переданное состояние.
	Apply(state DealState, command DealCommand) MoveResult
}

// SeatView — что видно про соседа по столу.
//
// ⭐ Руки здесь нет ФИЗИЧЕСКИ — только CardsCount. Не «есть, но скрыта»: если структура
// может содержать чужую карту, она спроектирована неправильно.
//
// ⭐ Слот навесов, наоборот, открыт всем легально (§2.3): «кому осталось два навеса
// до джокера» — ключевая информация за столом, заменяющая счёт.
type SeatView struct {
	// SeatNo — место за столом.
	SeatNo int
	// CardsCount — сколько карт в руке; скрытая в счёт не входит (§1.8).
	CardsCount int
	// HasHiddenCard — есть ли у него ещё не вскрытая скрытая карта; только факт.
	HasHiddenCard bool
	// HungCards — навешенное в этой раздаче; очищается перераздачей.
	HungCards []Card
	// NavesLevel — достигнутый уровень шкалы; переносится между раздачами.
	NavesLevel int
	// NextNavesRank — что можно навесить следующим; nil, если следующий шаг — джокер.
	NextNavesRank *Rank
	// NextIsJoker — следующая ступень джокер.
	NextIsJoker bool
	// Passed — спасовал в этом раунде.
	Passed bool
	// InDeal — ещё в раздаче.
	InDeal bool
	// ExitPlace — каким по счёту вышел из раздачи, начиная с первого; nil — ещё играет.
	// Порядок выхода не украшение: первый вышедший получает −1 по шкале (§0.1).
	ExitPlace *int
	// StepsToJoker — сколько навесов осталось до джокера. ⭐ Считает сервер: это и есть
	// счёт в игре (ADR-017), и выводить его на клиенте значило бы держать копию шкалы
	// в двух местах.
	StepsToJoker int
}

// NextRank — ранг, который сейчас летит этому месту; второе значение false, если
// следующая ступень джокер либо шкала уже пройдена.
func (s SeatView) NextRank() (Rank, bool) {
	if s.NextNavesRank == nil {
		return 0, false
	}
	return *s.NextNavesRank, true
}

// ExitedAt — каким по счёту место вышло из раздачи; false, если ещё играет.
func (s SeatView) ExitedAt() (int, bool) {
	if s.ExitPlace == nil {
		return 0, false
	}
	return *s.ExitPlace, true
}

// PlayerView — персональная проекция состояния, то что видит один игрок (fog of war).
//
// ⭐ Проекция — обязательный слой, а не фильтр в сериализаторе. Внутреннее состояние
// содержит все карты всех игроков и наружу не сериализуется никогда.
//
// ⭐ Скрытая карта не попадает сюда вообще ни к кому — ВКЛЮЧАЯ ВЛАДЕЛЬЦА (§1.8,
// ADR-026). Владелец знает только IHaveHiddenCard.
type PlayerView struct {
	// MySeat — место смотрящего.
	MySeat int
	// Phase — фаза раздачи.
	Phase DealPhase
	// TrumpSuit — козырная масть; nil, пока её разыгрывают костью (§1.2).
	TrumpSuit *Suit
	// TrumpCard — ⭐ сама козырная карта из-под колоды, ОТКРЫТА ВСЕМ (§1.9). Это не
	// нарушение тумана войны: она лежит на столе лицом вверх, и знать её положено
	// каждому. Не путать с потайным козырем — самой нижней картой, которая до вскрытия
	// не видна никому.
	TrumpCard Card
	// ProtectedSuit — защищённая масть; считается сервером, фронт её не выводит.
	ProtectedSuit *Suit
	// DeckLeft — сколько карт осталось в колоде; сами карты не отдаются.
	DeckLeft int
	// DiscardCount — сколько карт ушло в отбой. ⭐ Считает сервер: клиент знал бы состав
	// колоды и мог бы вычесть — то есть считать карты вместо игрока, а это другая игра.
	DiscardCount int
	// MyHand — своя рука целиком.
	MyHand []Card
	// IHaveHiddenCard — есть ли у меня не вскрытая скрытая карта.
	IHaveHiddenCard bool
	// Table — стол виден всем.
	Table []TableSlot
	// Seats — остальные места, включая своё.
	Seats []SeatView
	// RoundStarterSeat — кто начал раунд.
	RoundStarterSeat int
	// CanAttackSeat — у кого сейчас право положить карту.
	// Имя протокольное (canAttackSeat в STATE_SYNC); в состоянии это AttackRightSeat.
	CanAttackSeat int
	// DefenderSeat — кто отбивается.
	DefenderSeat int
	// HangingVictimSeat — кому сейчас навешивают; nil, если окна нет.
	HangingVictimSeat *int
	// AvailableActions — что именно я могу сделать прямо сейчас; считает сервер, чтобы
	// фронт не воспроизводил правила (ADR-003).
	AvailableActions []DealCommand
}

// Trump — козырная масть; false, пока козырь разыгрывают костью.
func (v PlayerView) Trump() (Suit, bool) {
	if v.TrumpSuit == nil {
		return 0, false
	}
	return *v.TrumpSuit, true
}

// HangingVictim — кому навешивают; false, если окна навеса нет.
func (v PlayerView) HangingVictim() (int, bool) {
	if v.HangingVictimSeat == nil {
		return 0, false
	}
	return *v.HangingVictimSeat, true
}

// Seat — проекция одного места за столом.
func (v PlayerView) Seat(seatNo int) (SeatView, error) {
	for _, seat := range v.Seats {
		if seat.SeatNo == seatNo {
			return seat, nil
		}
	}
	return SeatView{}, fmt.Errorf("в проекции нет места %d", seatNo)
}

// StateProjection — персональная проекция состояния (fog of war) и фильтр событий.
//
// ⭐ Проекция СТРОИТ PlayerView из своих карт и общих данных, а не фильтрует полное
// состояние. Разница принципиальная: отфильтровать можно забыть, а собрать из того,
// чего нет, — нельзя.
type StateProjection struct {
	config RulesConfig
	engine MoveApplier
}

// NewStateProjection собирает проекцию для правил стола и движка.
//
// ⚠️ Конструктора «по умолчанию» здесь нет намеренно: проекции достаточно интерфейса
// MoveApplier, и привязывать её к конкретному движку значило бы тащить в туман войны
// половину пакета. Обычный вызов — NewStateProjection(DefaultRulesConfig(), engine).
func NewStateProjection(config RulesConfig, engine MoveApplier) StateProjection {
	return StateProjection{config: config, engine: engine}
}

// Project — что видит игрок на месте viewerSeat.
func (p StateProjection) Project(state DealState, viewerSeat int) (PlayerView, error) {
	if p.engine == nil {
		return PlayerView{}, fmt.Errorf("проекция собрана без движка: список действий считать нечем")
	}
	viewer, err := state.PlayerAt(viewerSeat)
	if err != nil {
		return PlayerView{}, err
	}

	actions, err := p.availableActions(state, viewerSeat)
	if err != nil {
		return PlayerView{}, err
	}

	view := PlayerView{
		MySeat:           viewerSeat,
		Phase:            state.Phase,
		TrumpCard:        trumpCard(state),
		DeckLeft:         len(state.Deck),
		DiscardCount:     p.discardCount(state),
		MyHand:           copyCards(viewer.Hand),
		IHaveHiddenCard:  viewer.HasFaceDownCard(),
		Table:            append([]TableSlot(nil), state.Table...),
		Seats:            p.seats(state),
		RoundStarterSeat: state.RoundStarterSeat,
		CanAttackSeat:    state.AttackRightSeat,
		DefenderSeat:     state.DefenderSeat,
		AvailableActions: actions,
	}
	if state.HasTrump() {
		suit := state.Trump.Suit
		protected := state.Trump.ProtectedSuit()
		view.TrumpSuit = &suit
		view.ProtectedSuit = &protected
	}
	if state.HangingWindow != nil {
		victim := state.HangingWindow.VictimSeat
		view.HangingVictimSeat = &victim
	}
	return view, nil
}

// EventsFor — события, которые вправе увидеть этот игрок.
//
// ⭐ Вскрытие скрытой карты видит ТОЛЬКО ВЛАДЕЛЕЦ: карта переходит в его руку и дальше
// он играет ею как обычной, а чужая рука никому не показывается (§1.8). Остальные узнают
// лишь то, что видно в проекции: скрытой карты у него больше нет, а карт в руке стало
// на одну больше.
func (p StateProjection) EventsFor(events []DealEvent, viewerSeat int) []DealEvent {
	visible := make([]DealEvent, 0, len(events))
	for _, event := range events {
		if owner, private := event.PrivateToSeat(); private && owner != viewerSeat {
			continue
		}
		visible = append(visible, event)
	}
	return visible
}

// trumpCard — козырная карта, лежащая под колодой лицом вверх (§1.9).
//
// ⭐ Низ колоды — [ … ] [козырная карта] [потайной козырь], и карты уходят сверху.
// Значит козырная — предпоследняя, пока в колоде хотя бы две карты; когда осталась одна,
// козырную уже забрали в руку, а последняя — потайной козырь, и её не видит никто
// до вскрытия.
//
// Отдавать её всем можно и нужно: она открыта на столе. Скрывать её было бы проще,
// но за настоящим столом так не бывает — козырь знают все и с первой секунды.
func trumpCard(state DealState) Card {
	if len(state.Deck) < 2 {
		return nil
	}
	return state.Deck[len(state.Deck)-2]
}

// discardCount — сколько карт ушло в отбой.
//
// Прямого счётчика в раздаче нет — отбитые карты просто исчезают со стола. Считаем
// от обратного: всё, чего нет ни в колоде, ни на руках, ни на столе, ни в навесах,
// лежит в отбое.
//
// ⚠️ Счёт верен для состояний, собранных сдачей: он держится на том, что все карты
// колоды где-то есть. Снимок, собранный вручную (в тестах), даст ерунду — и это не
// поломка, а цена того, что отдельного счётчика не заводится.
func (p StateProjection) discardCount(state DealState) int {
	inPlay := len(state.Deck)
	for _, player := range state.Players {
		inPlay += player.HandSize() + len(player.HungCards)
		if player.HasFaceDownCard() {
			inPlay++
		}
	}
	for _, slot := range state.Table {
		if slot.IsBeaten() {
			inPlay += 2
		} else {
			inPlay++
		}
	}

	// ⚠️ Стол с невозможным числом игроков — не повод ронять проекцию: считать в такой
	// раздаче всё равно нечего, и «Бито 0» честнее выдуманного числа.
	full, err := BuildOrderedDeck(len(state.Players))
	if err != nil {
		return 0
	}
	if discarded := len(full) - inPlay; discarded > 0 {
		return discarded
	}
	return 0
}

func (p StateProjection) seats(state DealState) []SeatView {
	seats := make([]SeatView, 0, len(state.Players))
	for _, player := range state.Players {
		seat := SeatView{
			SeatNo:        player.SeatNo,
			CardsCount:    player.HandSize(),
			HasHiddenCard: player.HasFaceDownCard(),
			HungCards:     copyCards(player.HungCards),
			NavesLevel:    player.NavesLevel,
			NextIsJoker:   p.config.NavesScale.NextIsJoker(player.NavesLevel),
			Passed:        state.HasPassed(player.SeatNo),
			InDeal:        player.InDeal,
			StepsToJoker:  p.stepsToJoker(player.NavesLevel),
		}
		if rank, ok := p.config.NavesScale.NextRank(player.NavesLevel); ok {
			seat.NextNavesRank = &rank
		}
		if index := indexOfSeat(state.ExitOrder, player.SeatNo); index >= 0 {
			place := index + 1
			seat.ExitPlace = &place
		}
		seats = append(seats, seat)
	}
	return seats
}

// stepsToJoker — сколько навесов осталось до джокера.
//
// Джокер вешается, когда уровень доходит до JokerLevel; значит осталось ровно столько
// ступеней, сколько до него не хватает. Ноль означает, что джокер уже висит и игрок
// раздачу проиграл (§0.2).
func (p StateProjection) stepsToJoker(navesLevel int) int {
	if steps := p.config.NavesScale.JokerLevel() - navesLevel; steps > 0 {
		return steps
	}
	return 0
}

// availableActions — что игрок может сделать прямо сейчас.
//
// ⭐ Кандидаты собираются только из его собственных карт и того, что лежит на столе, —
// чужие карты в перебор не попадают даже мельком. Иначе отклонённый движком кандидат
// с чужой картой всё равно был бы построен, а до утечки осталось бы одно неверное
// условие фильтра.
func (p StateProjection) availableActions(state DealState, seat int) ([]DealCommand, error) {
	candidates, err := candidateCommands(state, seat)
	if err != nil {
		return nil, err
	}
	accepted := make([]DealCommand, 0, len(candidates))
	for _, candidate := range candidates {
		// ⭐ Проба идёт по копии: движок обязан быть чистым, но проекция не имеет права
		// на этом держаться — она вызывает его десятками раз подряд.
		if p.engine.Apply(state.Clone(), candidate).Applied {
			accepted = append(accepted, candidate)
		}
	}
	return accepted, nil
}

func candidateCommands(state DealState, seat int) ([]DealCommand, error) {
	viewer, err := state.PlayerAt(seat)
	if err != nil {
		return nil, err
	}

	candidates := make([]DealCommand, 0, 8+len(viewer.Hand)*(3+len(state.Table)))
	for _, suit := range Suits() {
		candidates = append(candidates, ChooseTrumpCommand{Seat: seat, Suit: suit})
	}
	for _, card := range viewer.Hand {
		candidates = append(candidates,
			AttackCommand{Seat: seat, Card: card},
			TransferCommand{Seat: seat, Card: card},
			HangCardCommand{Seat: seat, Card: card})
		for _, slot := range state.Table {
			candidates = append(candidates, DefendCommand{Seat: seat, Card: card, Target: slot.Attack})
		}
	}
	// ⭐ Вскрытие идёт без имени карты: игрок не видит свою скрытую карту, и подставить
	// её в кандидата было бы утечкой самому владельцу.
	candidates = append(candidates, RevealFaceDownCommand{Seat: seat})
	for _, slot := range state.Table {
		candidates = append(candidates, RevealFaceDownToDefendCommand{Seat: seat, Target: slot.Attack})
	}
	candidates = append(candidates,
		TakeCommand{Seat: seat},
		PassCommand{Seat: seat},
		HangSkipCommand{Seat: seat})
	return candidates, nil
}

func indexOfSeat(seats []int, seatNo int) int {
	for index, candidate := range seats {
		if candidate == seatNo {
			return index
		}
	}
	return -1
}
