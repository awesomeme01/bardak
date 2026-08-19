package game

// NobodySeat — джокер никем не навешен: либо его нет, либо он получен через +1.
const NobodySeat = -1

// PlayerState — состояние игрока внутри раздачи.
//
// ⭐ У руки ДВА разных счёта, и их легко перепутать: HandSize — для добора, скрытая карта
// в него не входит; DefendableCards — для лимита атаки, и там она считается, но только
// при пустой колоде.
//
// ⭐ Навесы распадаются на две части с разным временем жизни: NavesLevel живёт весь матч
// и переживает перераздачу, HungCards очищается вместе с раздачей — карты возвращаются
// в колоду. Путать их нельзя.
//
// Значение неизменяемое: все «изменения» возвращают новую копию, как в Java-оригинале.
// Движок работает как apply(state, command) -> (newState, events), и мутация здесь
// сделала бы невозможным сравнение «до и после».
type PlayerState struct {
	// SeatNo — место за столом, фиксировано на весь матч.
	SeatNo int
	// Hand — открытые для владельца карты.
	Hand []Card
	// FaceDownCard — скрытая карта; nil, если уже вскрыта.
	// Её не видит никто, включая владельца.
	FaceDownCard Card
	// InDeal — игрок ещё в раздаче: не вышел и не выбыл.
	InDeal bool
	// NavesLevel — уровень по шкале навесов.
	NavesLevel int
	// HungCards — карты, навешенные ему в этой раздаче; видны всем.
	HungCards []Card
	// JokerHangerSeat — кто навесил ему джокер, NobodySeat — никто.
	// Нужен в конце раздачи: добивший получает −1 за каждого добитого.
	JokerHangerSeat int
}

// NewPlayerState — игрок в начале раздачи.
func NewPlayerState(seatNo int, hand []Card, faceDown Card) PlayerState {
	return PlayerState{
		SeatNo:          seatNo,
		Hand:            copyCards(hand),
		FaceDownCard:    faceDown,
		InDeal:          true,
		NavesLevel:      NoNaves,
		HungCards:       []Card{},
		JokerHangerSeat: NobodySeat,
	}
}

// HandSize — счёт для добора: скрытая карта в него не входит.
func (p PlayerState) HandSize() int { return len(p.Hand) }

// HasFaceDownCard — есть ли ещё не вскрытая скрытая карта.
func (p PlayerState) HasFaceDownCard() bool { return p.FaceDownCard != nil }

// DefendableCards — счёт для лимита атаки: сколько карт игрок физически способен положить
// в защиту.
//
// ⭐ Скрытая карта входит сюда, ТОЛЬКО когда колода пуста. Пока в колоде есть карты,
// атака не вправе вынудить её вскрыть.
func (p PlayerState) DefendableCards(deckEmpty bool) int {
	if deckEmpty && p.HasFaceDownCard() {
		return p.HandSize() + 1
	}
	return p.HandSize()
}

// HoldsInHand — держит ли игрок эту карту открытой.
func (p PlayerState) HoldsInHand(card Card) bool {
	return indexOfCard(p.Hand, card) >= 0
}

// CanPlayFaceDown — скрытая карта играется, только когда колода пуста и обычных карт
// не осталось. Открытие необратимо, но это уже переход состояния, а не проверка.
func (p PlayerState) CanPlayFaceDown(deckEmpty bool) bool {
	return deckEmpty && p.HasFaceDownCard() && len(p.Hand) == 0
}

// WithHand — рука заменена целиком.
func (p PlayerState) WithHand(hand []Card) PlayerState {
	next := p
	next.Hand = copyCards(hand)
	return next
}

// WithoutCard — карта сыграна из руки. Удаляется ОДИН экземпляр, как в Java.
func (p PlayerState) WithoutCard(card Card) PlayerState {
	index := indexOfCard(p.Hand, card)
	if index < 0 {
		return p
	}
	remaining := make([]Card, 0, len(p.Hand)-1)
	remaining = append(remaining, p.Hand[:index]...)
	remaining = append(remaining, p.Hand[index+1:]...)
	next := p
	next.Hand = remaining
	return next
}

// WithFaceDownRevealed — скрытая карта вскрыта; переход односторонний.
func (p PlayerState) WithFaceDownRevealed() PlayerState {
	next := p
	next.FaceDownCard = nil
	return next
}

// LeftDeal — игрок вышел из раздачи.
func (p PlayerState) LeftDeal() PlayerState {
	next := p
	next.InDeal = false
	return next
}

// WithNavesLevel — новый уровень по шкале навесов.
func (p PlayerState) WithNavesLevel(level int) PlayerState {
	next := p
	next.NavesLevel = level
	return next
}

// WithHungCard — карта уходит из чужой руки в этот слот и выбывает из игры до конца раздачи.
//
// Для джокера запоминается навесивший: в конце раздачи он получит −1, если жертва проиграет.
func (p PlayerState) WithHungCard(card Card, hangerSeat int) PlayerState {
	next := p
	slot := make([]Card, 0, len(p.HungCards)+1)
	slot = append(slot, p.HungCards...)
	slot = append(slot, card)
	next.HungCards = slot
	if _, isJoker := card.(Joker); isJoker {
		next.JokerHangerSeat = hangerSeat
	}
	return next
}

// copyCards — защитная копия: срез в Go разделяется между значениями, и без копии
// «неизменяемое» состояние менялось бы через чужую ссылку.
func copyCards(cards []Card) []Card {
	if cards == nil {
		return []Card{}
	}
	out := make([]Card, len(cards))
	copy(out, cards)
	return out
}

// indexOfCard ищет карту по значению. Карты — сравнимые структуры, поэтому равенство
// работает как в Java (record equals), без ловушек указателей.
func indexOfCard(cards []Card, card Card) int {
	for index, candidate := range cards {
		if candidate == card {
			return index
		}
	}
	return -1
}
