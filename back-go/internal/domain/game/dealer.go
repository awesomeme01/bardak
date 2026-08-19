package game

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

// DiceResolver — бросок кости, общая подсистема на все четыре повода: козырь-джокер,
// потайной козырь, спор за джокер и спор за обычный навес.
//
// ⭐ Вынесено в интерфейс не ради подмены правила, а ради тестов: движок обязан
// оставаться детерминированным, поэтому случайность не берётся из воздуха, а выводится
// из seed раздачи и номера броска.
type DiceResolver interface {
	// WinnerAmong — кто выиграл бросок среди участников спора.
	//
	// seats — участники в стабильном порядке; seed — seed раздачи; rollNo — номер броска
	// внутри раздачи: два спора подряд не должны давать одинаковый результат.
	WinnerAmong(seats []int, seed int64, rollNo int) (int, error)
}

// SeededDice — шестигранная кость от seed раздачи.
//
// Переброс при ничьей отдельно моделировать не нужно: результат здесь уже единственный,
// а сам факт равных бросков — деталь показа в UI.
//
// ⚠️ Совпадения с `java.util.Random` НЕТ и не требуется (MD-005): ни восстановление
// матча, ни реплей не выводят случайность из seed заново. Требуется только детерминизм
// внутри Go: одна тройка (seats, seed, rollNo) — один победитель.
type SeededDice struct{}

// WinnerAmong — победитель броска. Пустой спор — ошибка вызывающего, а не случайный ноль.
func (SeededDice) WinnerAmong(seats []int, seed int64, rollNo int) (int, error) {
	if len(seats) == 0 {
		return 0, errors.New("бросок кости без участников")
	}
	// ⭐ Номер броска подмешивается в seed так же, как в Java: без него два спора подряд
	// в одной раздаче получали бы одного и того же победителя.
	mixed := seed*31 + int64(rollNo)
	source := rand.NewPCG(uint64(mixed), uint64(mixed)>>32^0x9e3779b97f4a7c15)
	return seats[rand.New(source).IntN(len(seats))], nil
}

// maxReshuffles — сколько раз подряд можно пересдать, прежде чем сдаться. Практический
// предохранитель: вероятность десяти пересдач подряд исчезающе мала, а бесконечный цикл
// в движке — нет.
const maxReshuffles = 10

// Dealer — сдача раздачи: перемешивание от под-seed, руки, скрытые карты, козырь.
//
// ⭐ Колода собирается заново каждый раз — это и есть «карты из слотов, включая джокеры,
// возвращаются в колоду». Возвращать их поштучно не нужно и было бы источником ошибок:
// слот живёт одну раздачу, а колода строится по составу стола.
//
// Уровни навесов, наоборот, приходят снаружи: они живут весь матч.
//
// ⭐ Низ колоды устроен так (снизу вверх):
//
//	[потайной козырь] [козырная карта] [ … остальные … ]
//
// В терминах среза это `deck[len-1]` — потайной козырь и `deck[len-2]` — козырная карта.
// Козырь определяет козырная карта, а самая нижняя — потайной козырь: он придёт
// последнему добирающему и сменит козырь. Если до него не дошли, он так и останется
// тайной.
type Dealer struct {
	config RulesConfig
	dice   DiceResolver
}

// NewDealer собирает сдающего.
//
// ⚠️ Кость передаётся параметром, а не берётся глобально: иначе воспроизвести раздачу
// в тесте было бы нечем. Отсутствие кости трактуется как «обычная, от seed» — паниковать
// посреди раздачи из-за nil хуже, чем взять единственную существующую реализацию.
func NewDealer(config RulesConfig, dice DiceResolver) Dealer {
	if dice == nil {
		dice = SeededDice{}
	}
	return Dealer{config: config, dice: dice}
}

// StartDeal — новая раздача.
//
// ⭐ Если козырной масти не оказалось ни у кого, раздача ПЕРЕСДАЁТСЯ: первый ход
// определяется младшим козырем, и без козырей на руках определять его не из чего.
// Пересдача идёт от производного seed и остаётся воспроизводимой.
//
// navesLevels — уровни игроков по местам, переносятся между раздачами; его длина и задаёт
// число игроков. dealSeed — под-seed раздачи, производный от seed матча.
func (d Dealer) StartDeal(navesLevels []int, dealSeed int64) (DealState, error) {
	seed := dealSeed
	for attempt := 0; attempt < maxReshuffles; attempt++ {
		deal, err := d.dealOnce(navesLevels, seed)
		if err != nil {
			return DealState{}, err
		}
		// Нижняя карта — джокер: козыря ещё нет, пересдавать не из чего и незачем.
		if !deal.HasTrump() || d.HasAnyTrumpInHands(deal) {
			return deal, nil
		}
		seed = d.ReshuffleSeed(seed, attempt)
	}
	return d.dealOnce(navesLevels, seed)
}

// HasAnyTrumpInHands — козырь есть хоть у кого-то, иначе первый ход определять не из чего.
func (d Dealer) HasAnyTrumpInHands(deal DealState) bool {
	if !deal.HasTrump() {
		return false
	}
	for _, player := range deal.Players {
		if _, found := lowestTrumpRank(player, *deal.Trump); found {
			return true
		}
	}
	return false
}

// ReshuffleSeed — seed пересдачи: другой расклад, но по-прежнему производный от seed матча.
func (d Dealer) ReshuffleSeed(seed int64, attempt int) int64 {
	return seed*31 + int64(attempt) + 1
}

func (d Dealer) dealOnce(navesLevels []int, dealSeed int64) (DealState, error) {
	playerCount := len(navesLevels)
	deck, err := BuildShuffledDeck(playerCount, SeededShuffler{Seed: dealSeed})
	if err != nil {
		return DealState{}, err
	}
	players, rest, err := d.dealHands(deck, navesLevels)
	if err != nil {
		return DealState{}, err
	}
	// ⚠️ Ниже козырной карты обязана лежать ещё одна — потайной козырь. Стол, на котором
	// после сдачи осталась одна карта, играть нечем: козырь было бы неоткуда взять.
	if len(rest) < 2 {
		return DealState{}, fmt.Errorf(
			"после сдачи в колоде осталось %d карт, а нужны козырная и потайной козырь", len(rest))
	}

	deal := d.emptyDeal(rest, players, dealSeed)
	trumpCard := rest[len(rest)-2]

	if pip, isPip := trumpCard.(Pip); isPip {
		trump := NewTrump(pip.Suit)
		starter := firstMoveSeat(players, trump)
		deal.Trump = &trump
		deal.Phase = PhaseAttack
		deal.RoundStarterSeat = starter
		deal.AttackRightSeat = starter
		deal.DefenderSeat = (starter + 1) % playerCount
		return deal, nil
	}

	// Козырной картой оказался джокер: масть называет победитель кости, а до тех пор
	// раздача стоит в фазе DICE и козыря не имеет.
	winner, err := d.dice.WinnerAmong(allSeats(playerCount), dealSeed, 0)
	if err != nil {
		return DealState{}, err
	}
	deal.Phase = PhaseDice
	deal.AttackRightSeat = winner
	deal.DiceRolls = 1
	return deal, nil
}

// dealHands — раздача карт: по DealSize каждому, плюс одна скрытая карта сверх руки.
// Карты снимаются с верха колоды, поэтому порядок сдачи детерминирован seed'ом.
//
// ⭐ Сдаётся по кругу, а не пачкой на игрока: при равном seed это тот же набор карт,
// но порядок обхода — часть правил, и менять его нельзя даже «эквивалентно».
func (d Dealer) dealHands(deck []Card, navesLevels []int) ([]PlayerState, []Card, error) {
	playerCount := len(navesLevels)
	needed := playerCount * (d.config.DealSize + 1)
	if needed > len(deck) {
		return nil, nil, fmt.Errorf("на %d игроков нужно %d карт, в колоде %d",
			playerCount, needed, len(deck))
	}

	hands := make([][]Card, playerCount)
	for seat := range hands {
		hands[seat] = make([]Card, 0, d.config.DealSize)
	}
	top := 0
	for card := 0; card < d.config.DealSize; card++ {
		for seat := 0; seat < playerCount; seat++ {
			hands[seat] = append(hands[seat], deck[top])
			top++
		}
	}

	players := make([]PlayerState, playerCount)
	for seat := 0; seat < playerCount; seat++ {
		faceDown := deck[top]
		top++
		players[seat] = NewPlayerState(seat, hands[seat], faceDown).
			WithNavesLevel(navesLevels[seat])
	}
	return players, copyCards(deck[top:]), nil
}

// firstMoveSeat — кому ходить первым: обладателю МЛАДШЕГО козыря. Правило одно и то же
// в каждой раздаче матча — проигравший прошлую преимуществ не получает.
//
// Козырей нет ни у кого — до этого места дело не доходит: такая раздача пересдаётся.
func firstMoveSeat(players []PlayerState, trump Trump) int {
	var lowest Rank
	found := false
	starter := 0
	for _, player := range players {
		candidate, hasTrump := lowestTrumpRank(player, trump)
		if hasTrump && (!found || lowest.IsHigherThan(candidate)) {
			lowest = candidate
			found = true
			starter = player.SeatNo
		}
	}
	return starter
}

// lowestTrumpRank — младший козырь в руке; второе значение false, если козырей нет.
//
// ⚠️ Скрытая карта в счёт не идёт: её не видит даже владелец, и определять по ней
// первый ход значило бы подглядывать.
func lowestTrumpRank(player PlayerState, trump Trump) (Rank, bool) {
	var lowest Rank
	found := false
	for _, card := range player.Hand {
		pip, isPip := card.(Pip)
		if !isPip || pip.Suit != trump.Suit {
			continue
		}
		if !found || lowest.IsHigherThan(pip.Rank) {
			lowest = pip.Rank
			found = true
		}
	}
	return lowest, found
}

// emptyDeal — снимок сразу после сдачи: козырь ещё не проставлен, стол пуст.
//
// ⚠️ Место защищающегося здесь условное: при обычной козырной карте его тут же
// пересчитают от начавшего раунд, а в фазе DICE его пересчитает движок после выбора масти.
func (d Dealer) emptyDeal(deck []Card, players []PlayerState, dealSeed int64) DealState {
	defender := 0
	if len(players) > 1 {
		defender = 1
	}
	return DealState{
		Phase:            PhaseDealing,
		Deck:             copyCards(deck),
		Players:          append([]PlayerState(nil), players...),
		Table:            []TableSlot{},
		RoundStarterSeat: 0,
		AttackRightSeat:  0,
		DefenderSeat:     defender,
		PassedSeats:      []int{},
		ExitOrder:        []int{},
		LastAttackCards:  []Card{},
		RngSeed:          dealSeed,
	}
}

// seats — участники броска при сдаче: за столом ещё все, никто не вышел.
func allSeats(playerCount int) []int {
	all := make([]int, playerCount)
	for seat := range all {
		all[seat] = seat
	}
	return all
}
