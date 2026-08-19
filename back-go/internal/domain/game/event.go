package game

// DealEvent — что произошло в раздаче.
//
// ⭐ События — причина, снимок состояния — следствие. Клиент анимирует по событию,
// а состояние берёт из снимка, поэтому события уходят ПЕРЕД снимком.
type DealEvent interface {
	SeatNo() int

	// PrivateToSeat — кому событие видно, если не всем. Второе значение false — событие публичное.
	//
	// ⚠️ Единственный приватный случай — вскрытие скрытой карты: она уходит в руку
	// владельца и дальше играется как обычная, а чужую руку не видит никто. Остальные
	// узнают только то, что видно в проекции: скрытой карты у него больше нет.
	PrivateToSeat() (int, bool)

	sealedEvent()
}

// publicEvent — общая часть публичных событий.
type publicEvent struct{ Seat int }

func (e publicEvent) SeatNo() int              { return e.Seat }
func (publicEvent) PrivateToSeat() (int, bool) { return 0, false }
func (publicEvent) sealedEvent()               {}

// CardAttacked — карта положена в атаку.
type CardAttacked struct {
	publicEvent
	Card Card
}

// CardDefended — атака отбита указанной картой.
type CardDefended struct {
	publicEvent
	Card   Card
	Target Card
}

// AttackTransferred — атака переведена дальше по кругу.
type AttackTransferred struct {
	publicEvent
	ToSeatNo int
	Card     Card
}

// FaceDownRevealed — скрытая карта вскрыта.
//
// ⚠️ Единственное ПРИВАТНОЕ событие: карту видит только владелец.
type FaceDownRevealed struct {
	Seat int
	Card Card
}

func (e FaceDownRevealed) SeatNo() int                { return e.Seat }
func (e FaceDownRevealed) PrivateToSeat() (int, bool) { return e.Seat, true }
func (FaceDownRevealed) sealedEvent()                 {}

// Passed — игрок спасовал.
type Passed struct{ publicEvent }

// AttackRightMoved — право подкидывать ушло следующему, второму соседу.
type AttackRightMoved struct{ publicEvent }

// RoundBeaten — «бито»: все атаки отбиты, карты уходят в отбой.
type RoundBeaten struct {
	publicEvent
	Discarded []Card
}

// TakeAnnounced — «беру» объявлено.
//
// ⭐ Отдельное событие от CardsTaken: между ними подкидывающие ещё докидывают карты,
// и на клиенте это разные моменты.
type TakeAnnounced struct{ publicEvent }

// CardsTaken — «взял»: защищающийся забрал стол в руку.
type CardsTaken struct {
	publicEvent
	Cards []Card
}

// CardsDrawn — добор из колоды.
type CardsDrawn struct {
	publicEvent
	Cards []Card
}

// PlayerLeftDeal — игрок избавился от карт и вышел из раздачи.
//
// Порядок выхода важен для шкалы: первый вышедший отыгрывает ступень назад.
type PlayerLeftDeal struct{ publicEvent }

// HiddenTrumpRevealed — потайной козырь вскрыт.
//
// ⭐ Событие ПУБЛИЧНОЕ, с картой: видны и масть, и номинал — он меняет козырь всему
// столу, в отличие от скрытой карты игрока.
type HiddenTrumpRevealed struct {
	publicEvent
	Card Card
}

// TrumpChanged — козырь сменился.
type TrumpChanged struct {
	publicEvent
	Suit Suit
}

// TrumpChosen — победитель кости назвал масть.
type TrumpChosen struct {
	publicEvent
	Suit Suit
}

// HangingWindowOpened — открыто окно навеса на этого игрока.
type HangingWindowOpened struct{ publicEvent }

// CardHung — карта навешена жертве.
type CardHung struct {
	publicEvent
	VictimSeat int
	Card       Card
}

// NavesLevelChanged — уровень по шкале навесов изменился.
type NavesLevelChanged struct {
	publicEvent
	Level int
}

// DiceRolled — брошена кость, спор разрешён.
type DiceRolled struct {
	publicEvent
	Participants []int
}

// HangingWindowClosed — окно навеса закрыто.
type HangingWindowClosed struct{ publicEvent }

// DealFinished — раздача окончена.
type DealFinished struct{ publicEvent }

// событие-конструкторы: место задаётся полем встроенной структуры, и без помощников
// вызов выглядел бы как CardAttacked{publicEvent{seat}, card} — шумно и легко перепутать.

func NewCardAttacked(seat int, card Card) CardAttacked {
	return CardAttacked{publicEvent{seat}, card}
}

func NewCardDefended(seat int, card, target Card) CardDefended {
	return CardDefended{publicEvent{seat}, card, target}
}

func NewAttackTransferred(seat, toSeat int, card Card) AttackTransferred {
	return AttackTransferred{publicEvent{seat}, toSeat, card}
}

func NewPassed(seat int) Passed                     { return Passed{publicEvent{seat}} }
func NewAttackRightMoved(seat int) AttackRightMoved { return AttackRightMoved{publicEvent{seat}} }
func NewTakeAnnounced(seat int) TakeAnnounced       { return TakeAnnounced{publicEvent{seat}} }
func NewPlayerLeftDeal(seat int) PlayerLeftDeal     { return PlayerLeftDeal{publicEvent{seat}} }
func NewHangingWindowOpened(seat int) HangingWindowOpened {
	return HangingWindowOpened{publicEvent{seat}}
}
func NewHangingWindowClosed(seat int) HangingWindowClosed {
	return HangingWindowClosed{publicEvent{seat}}
}
func NewDealFinished(seat int) DealFinished { return DealFinished{publicEvent{seat}} }

func NewRoundBeaten(seat int, discarded []Card) RoundBeaten {
	return RoundBeaten{publicEvent{seat}, copyCards(discarded)}
}

func NewCardsTaken(seat int, cards []Card) CardsTaken {
	return CardsTaken{publicEvent{seat}, copyCards(cards)}
}

func NewCardsDrawn(seat int, cards []Card) CardsDrawn {
	return CardsDrawn{publicEvent{seat}, copyCards(cards)}
}

func NewHiddenTrumpRevealed(seat int, card Card) HiddenTrumpRevealed {
	return HiddenTrumpRevealed{publicEvent{seat}, card}
}

func NewTrumpChanged(seat int, suit Suit) TrumpChanged {
	return TrumpChanged{publicEvent{seat}, suit}
}

func NewTrumpChosen(seat int, suit Suit) TrumpChosen {
	return TrumpChosen{publicEvent{seat}, suit}
}

func NewCardHung(seat, victimSeat int, card Card) CardHung {
	return CardHung{publicEvent{seat}, victimSeat, card}
}

func NewNavesLevelChanged(seat, level int) NavesLevelChanged {
	return NavesLevelChanged{publicEvent{seat}, level}
}

func NewDiceRolled(seat int, participants []int) DiceRolled {
	return DiceRolled{publicEvent{seat}, append([]int(nil), participants...)}
}

// MoveResult — итог применения команды: либо новое состояние с событиями, либо отказ.
//
// ⭐ Отказ не меняет состояние вообще: сервер валидирует каждый ход, и отклонённый ход
// не должен оставлять следов.
type MoveResult struct {
	Applied bool
	State   DealState
	Events  []DealEvent
	Reason  RejectionReason
}

// AppliedResult — команда применена.
func AppliedResult(state DealState, events []DealEvent) MoveResult {
	return MoveResult{Applied: true, State: state, Events: events}
}

// RejectedResult — команда отклонена с причиной.
func RejectedResult(reason RejectionReason) MoveResult {
	return MoveResult{Applied: false, Reason: reason}
}
