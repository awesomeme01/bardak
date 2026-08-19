package game

import "fmt"

// LossDegree — степень проигрыша (§0.3).
//
// ⭐ Реальные степени объявлены от самой тяжёлой к самой обычной: когда игроку подходит
// несколько условий сразу, всегда берётся САМАЯ ТЯЖЁЛАЯ. Это общее правило, а не частный
// случай, поэтому порядок проверок в degreeFor и есть алгоритм.
//
// ⚠️ NoLossDegree стоит нулевым значением, хотя в Java на его месте был null. В Go нулевое
// значение достаётся пустой структуре само; окажись нулём LossRoyal, любой несобранный
// итог объявлял бы игрока королевским проигравшим. Из-за этого сравнение степеней нельзя
// вести прямым сравнением чисел — только через IsHeavierThan.
//
// Названия в интерфейсе авторские; здесь — нейтральные константы.
type LossDegree uint8

const (
	// NoLossDegree — игрок игру не проиграл, степени у него нет.
	NoLossDegree LossDegree = iota
	// LossRoyal — джокер, проиграл раздачу, последняя атака — ровно четыре восьмёрки.
	LossRoyal
	// LossSuperMegaSuck — джокер, проиграл раздачу, в последней атаке одна–три восьмёрки.
	LossSuperMegaSuck
	// LossSuperMegaFail — джокер навешен картой, проиграл раздачу.
	LossSuperMegaFail
	// LossSuperFail — джокер получен не картой: в навесе был туз, проигрыш раздачи добавил +1.
	LossSuperFail
	// LossFail — джокер в навесе, но раздачу игрок не проиграл и первым не вышел.
	LossFail
)

var lossDegreeNames = [...]string{
	"NONE", "ROYAL", "SUPER_MEGA_SUCK", "SUPER_MEGA_FAIL", "SUPER_FAIL", "FAIL",
}

func (d LossDegree) String() string {
	if int(d) >= len(lossDegreeNames) {
		return "?"
	}
	return lossDegreeNames[d]
}

// weight — вес для сравнения: чем меньше, тем тяжелее. «Степени нет» тяжелее быть не может,
// поэтому её вес выведен за пределы шкалы.
func (d LossDegree) weight() int {
	if d == NoLossDegree {
		return len(lossDegreeNames) + 1
	}
	return int(d)
}

// IsHeavierThan — тяжелее другой степени. Сравнение по порядку объявления, а не по имени.
func (d LossDegree) IsHeavierThan(other LossDegree) bool {
	return d.weight() < other.weight()
}

// LevelChangeReason — почему уровень навесов сдвинулся (§0.1).
//
// ⭐ Сдвиги сочетаются в одной раздаче и друг друга не заменяют, поэтому итоговая разница
// уровней сама по себе ничего не объясняет: игрок с +1 и −1 выглядит как не сдвинувшийся
// вовсе. Причина уходит в историю матча, значит имя — часть контракта.
type LevelChangeReason string

const (
	// LostDeal — проиграл раздачу: остался с картами.
	LostDeal LevelChangeReason = "LOST_DEAL"
	// FirstOut — вышел из раздачи первым.
	FirstOut LevelChangeReason = "FIRST_OUT"
	// FinishedOpponent — добил соперника джокером; награда даётся за каждого и суммируется.
	FinishedOpponent LevelChangeReason = "FINISHED_OPPONENT"
	// ScaleLimit — шкала кончилась: ступени ниже «летит 6» и выше джокера не существует.
	ScaleLimit LevelChangeReason = "SCALE_LIMIT"
)

// LevelChange — один сдвиг уровня навесов с причиной.
type LevelChange struct {
	// Reason — почему сдвинули.
	Reason LevelChangeReason
	// Amount — на сколько ступеней; отрицательное значение — вниз по шкале.
	Amount int
}

// Unplaced — места нет: так выглядит итог, собранный вручную, а не движком.
const Unplaced = 0

// PlayerOutcome — итог раздачи для одного игрока.
//
// ⭐ Итог самодостаточен: раздача после подсчёта исчезает — карты собираются в колоду,
// и следующая сдача занимает её место. Всё, что нужно истории, обязано лежать здесь,
// иначе восстановить прошлое можно будет только переигрыванием всего матча.
type PlayerOutcome struct {
	// SeatNo — место за столом.
	SeatNo int
	// LevelBefore — уровень до подсчёта.
	LevelBefore int
	// LevelAfter — уровень после всех сдвигов и нижней границы.
	LevelAfter int
	// LossDegree — степень проигрыша; NoLossDegree, если игрок не проиграл игру.
	LossDegree LossDegree
	// Place — место в раздаче: вышедший первым — первый, оставшийся с картами — последний.
	// Unplaced, когда итог собран не движком.
	Place int
	// HungCards — что игроку навесили в этой раздаче.
	HungCards []Card
	// Changes — из чего сложился сдвиг уровня: слагаемые, а не сумма.
	Changes []LevelChange
}

// NewPlayerOutcome — итог без обстановки раздачи: так его собирают тесты и матч.
func NewPlayerOutcome(seatNo, levelBefore, levelAfter int, degree LossDegree) PlayerOutcome {
	return PlayerOutcome{
		SeatNo:      seatNo,
		LevelBefore: levelBefore,
		LevelAfter:  levelAfter,
		LossDegree:  degree,
		Place:       Unplaced,
		HungCards:   []Card{},
		Changes:     []LevelChange{},
	}
}

// IsLoser — игрок закончил игру с джокером.
func (o PlayerOutcome) IsLoser() bool { return o.LossDegree != NoLossDegree }

// Shift — на сколько ступеней сдвинулся уровень: отрицательное значение — удачная раздача.
func (o PlayerOutcome) Shift() int { return o.LevelAfter - o.LevelBefore }

// DealOutcome — итог раздачи целиком (§0.5).
//
// ⭐ Проигравших может быть несколько — это штатная ситуация, а не ошибка: несколько
// игроков заканчивают игру с джокером и различаются степенью. Главный проигравший — тот,
// у кого степень тяжелее.
type DealOutcome struct {
	// Players — итоги по всем местам, в порядке мест.
	Players []PlayerOutcome
	// DealLoserSeat — кто остался с картами: «дурак» раздачи.
	DealLoserSeat int
	// TrumpSuit — козырь этой раздачи; nil — раздача кончилась, не начавшись.
	TrumpSuit *Suit
	// LastAttackCards — состав последней атаки.
	//
	// ⭐ От него зависят степени ROYAL и SUPER_MEGA_SUCK (§0.3), и после подсчёта его
	// больше негде взять: раздачи уже нет.
	LastAttackCards []Card
}

// NewDealOutcome — итог без обстановки раздачи: так его собирают тесты.
func NewDealOutcome(players []PlayerOutcome, dealLoserSeat int) DealOutcome {
	out := make([]PlayerOutcome, len(players))
	copy(out, players)
	return DealOutcome{
		Players:         out,
		DealLoserSeat:   dealLoserSeat,
		LastAttackCards: []Card{},
	}
}

// ForSeat — итог одного места. Место вне стола — ошибка вызывающего.
func (o DealOutcome) ForSeat(seatNo int) (PlayerOutcome, error) {
	for _, outcome := range o.Players {
		if outcome.SeatNo == seatNo {
			return outcome, nil
		}
	}
	return PlayerOutcome{}, fmt.Errorf("за столом нет места %d", seatNo)
}

// MustForSeat — то же, но для мест, взятых из самого итога.
func (o DealOutcome) MustForSeat(seatNo int) PlayerOutcome {
	outcome, err := o.ForSeat(seatNo)
	if err != nil {
		panic(err)
	}
	return outcome
}

// Losers — все, кто закончил игру с джокером.
func (o DealOutcome) Losers() []PlayerOutcome {
	losers := make([]PlayerOutcome, 0, len(o.Players))
	for _, outcome := range o.Players {
		if outcome.IsLoser() {
			losers = append(losers, outcome)
		}
	}
	return losers
}

// IsMatchOver — матч окончен: кому-то навешен джокер, и он не вышел первым (§0.2).
func (o DealOutcome) IsMatchOver() bool { return len(o.Losers()) > 0 }

// MainLoser — главный проигравший, по старшинству степеней (§0.3).
// Второе значение false, если игру не проиграл никто.
func (o DealOutcome) MainLoser() (PlayerOutcome, bool) {
	var main PlayerOutcome
	found := false
	for _, outcome := range o.Players {
		if !outcome.IsLoser() {
			continue
		}
		// ⚠️ Строго «тяжелее»: при равных степенях выигрывает первый по порядку мест,
		// как findFirst у min() в Java. Иначе итог зависел бы от обхода.
		if !found || outcome.LossDegree.IsHeavierThan(main.LossDegree) {
			main = outcome
			found = true
		}
	}
	return main, found
}

// DealScoring — подсчёт итога раздачи (§0.1, §0.3, §0.4), самая коварная часть правил.
//
// ⭐ Считать поигроково и независимо нельзя: судьба навесившего зависит от судьбы того,
// кому он навесил. Поэтому проходов четыре, и порядок между ними — это и есть правило,
// а не деталь реализации:
//
//  1. автоматические сдвиги: +1 проигравшему раздачу, −1 вышедшему первым;
//  2. кто проиграл игру — джокер И не вышел первым;
//  3. −1 каждому, кто добил проигравшего джокером; награды суммируются;
//  4. нижняя граница и степени проигрыша.
//
// ⭐ Нижняя граница применяется В КОНЦЕ, а не после каждого сдвига: иначе игрок,
// получивший +1 и −2, упёрся бы в «летит 6» раньше времени и пришёл бы не туда.
type DealScoring struct {
	config RulesConfig
}

// NewDealScoring собирает счётчик итогов по правилам стола.
func NewDealScoring(config RulesConfig) DealScoring {
	return DealScoring{config: config}
}

// levelLedger — уровни и слагаемые сдвига в одном месте.
//
// ⭐ Слагаемые обязаны копиться вместе с уровнем: без них потом не объяснить счёт —
// игрок с +1 и −1 неотличим от несдвинувшегося.
type levelLedger struct {
	levels  map[int]int
	changes map[int][]LevelChange
}

func (l *levelLedger) shift(seat, amount int, reason LevelChangeReason) {
	l.levels[seat] += amount
	l.changes[seat] = append(l.changes[seat], LevelChange{Reason: reason, Amount: amount})
}

// Score — итог законченной раздачи.
//
// Состояние ожидается в фазе PhaseDealOver: карты остались у одного игрока.
func (s DealScoring) Score(state DealState) (DealOutcome, error) {
	dealLoser, err := dealLoserSeat(state)
	if err != nil {
		return DealOutcome{}, err
	}

	ledger := &levelLedger{
		levels:  make(map[int]int, len(state.Players)),
		changes: make(map[int][]LevelChange, len(state.Players)),
	}
	for _, player := range state.Players {
		ledger.levels[player.SeatNo] = player.NavesLevel
	}

	s.applyAutomaticShifts(state, ledger, dealLoser)
	gameLosers := s.gameLosers(state, ledger)
	s.applyFinisherRewards(state, ledger, gameLosers)

	var trumpSuit *Suit
	if state.HasTrump() {
		suit := state.Trump.Suit
		trumpSuit = &suit
	}
	return DealOutcome{
		Players:         s.outcomes(state, ledger, dealLoser, gameLosers),
		DealLoserSeat:   dealLoser,
		TrumpSuit:       trumpSuit,
		LastAttackCards: copyCards(state.LastAttackCards),
	}, nil
}

// dealLoserSeat — «дурак» раздачи: единственный, у кого остались карты (§0.2).
func dealLoserSeat(state DealState) (int, error) {
	for _, player := range state.Players {
		if player.InDeal {
			return player.SeatNo, nil
		}
	}
	return 0, fmt.Errorf("раздача без проигравшего невозможна")
}

// applyAutomaticShifts — проход 1. Проигравший раздачу получает +1, вышедший первым — −1.
// Сдвиги суммируются и друг друга не заменяют.
func (s DealScoring) applyAutomaticShifts(state DealState, ledger *levelLedger, dealLoser int) {
	ledger.shift(dealLoser, 1, LostDeal)
	if len(state.ExitOrder) > 0 {
		ledger.shift(state.ExitOrder[0], -1, FirstOut)
	}
}

// gameLosers — проход 2. Проиграл игру тот, у кого джокер И кто не вышел первым (§0.2).
//
// ⭐ Выход первым уже снял джокер на проходе 1 — отдельной проверки не требуется.
func (s DealScoring) gameLosers(state DealState, ledger *levelLedger) []int {
	losers := make([]int, 0, len(state.Players))
	for _, player := range state.Players {
		if s.config.NavesScale.IsFinished(ledger.levels[player.SeatNo]) {
			losers = append(losers, player.SeatNo)
		}
	}
	return losers
}

// applyFinisherRewards — проход 3. За каждого проигравшего −1 тому, кто навесил ему джокер.
//
// ⭐ Награда даётся за каждого добитого и суммируется: добил двоих — −2. Работает и тогда,
// когда у самого добившего в навесе джокер: он получает −1, джокер снимается, и
// проигравшим он уже не считается.
func (s DealScoring) applyFinisherRewards(state DealState, ledger *levelLedger, gameLosers []int) {
	for _, loser := range gameLosers {
		finisher := state.MustPlayerAt(loser).JokerHangerSeat
		if finisher != NobodySeat {
			ledger.shift(finisher, -1, FinishedOpponent)
		}
	}
}

// outcomes — проход 4. Нижняя граница и степени.
//
// ⚠️ Проигравшие определяются ЗАНОВО: награда на проходе 3 могла снять джокер с того,
// кто попал в список на проходе 2.
func (s DealScoring) outcomes(state DealState, ledger *levelLedger, dealLoser int,
	gameLosers []int) []PlayerOutcome {
	outcomes := make([]PlayerOutcome, 0, len(state.Players))
	for _, player := range state.Players {
		seat := player.SeatNo
		raw := ledger.levels[seat]
		level := s.clampToScale(raw)
		degree := NoLossDegree
		if s.config.NavesScale.IsFinished(level) && containsInt(gameLosers, seat) {
			degree = degreeFor(state, player, seat == dealLoser)
		}
		changes := append([]LevelChange(nil), ledger.changes[seat]...)
		if level != raw {
			// Упёрлись в край шкалы: без этой строки слагаемые не сходились бы с итогом.
			changes = append(changes, LevelChange{Reason: ScaleLimit, Amount: level - raw})
		}
		outcomes = append(outcomes, PlayerOutcome{
			SeatNo:      seat,
			LevelBefore: player.NavesLevel,
			LevelAfter:  level,
			LossDegree:  degree,
			Place:       placeOf(state, seat),
			HungCards:   copyCards(player.HungCards),
			Changes:     changes,
		})
	}
	return outcomes
}

// placeOf — место в раздаче: кто раньше вышел, тот выше. Оставшийся с картами — последний,
// и это единственное место, которое в правилах названо прямо (§0.2).
func placeOf(state DealState, seat int) int {
	for index, exited := range state.ExitOrder {
		if exited == seat {
			return index + 1
		}
	}
	return len(state.Players)
}

// clampToScale — границы шкалы.
//
// ⭐ Нижняя описана явно: «летит 6», ступени «5» нет (§0.1). Верхняя в правилах не названа,
// потому что джокер заканчивает матч, — но она есть: игрок, которому навесили джокер
// посреди раздачи, доигрывает её и может вдобавок проиграть раздачу. Ступени выше джокера
// не существует, лишний +1 пропадает.
func (s DealScoring) clampToScale(level int) int {
	if level < NoNaves {
		return NoNaves
	}
	if joker := s.config.NavesScale.JokerLevel(); level > joker {
		return joker
	}
	return level
}

// degreeFor — степень проигрыша (§0.3).
//
// ⚠️ Восьмёрки считаются по составу ПОСЛЕДНЕЙ АТАКИ, а не по тому, что попало игроку
// в руку: отбитая восьмёрка в руку не приходит, но степень всё равно даёт.
//
// ⭐ Подходить может несколько условий сразу — берётся самая тяжёлая, поэтому порядок
// проверок здесь и есть алгоритм.
func degreeFor(state DealState, player PlayerState, lostTheDeal bool) LossDegree {
	if !lostTheDeal {
		return LossFail
	}
	eights := 0
	for _, card := range state.LastAttackCards {
		if pip, ok := card.(Pip); ok && pip.Rank == Eight {
			eights++
		}
	}
	// ⚠️ Ровно четыре, а не «четыре и больше»: восьмёрок в колоде столько же, сколько мастей.
	if eights == len(Suits()) {
		return LossRoyal
	}
	if eights >= 1 {
		return LossSuperMegaSuck
	}
	if player.JokerHangerSeat != NobodySeat {
		return LossSuperMegaFail
	}
	return LossSuperFail
}
