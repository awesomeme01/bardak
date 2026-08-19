package game

// MatchPhase — фаза матча (§4.1).
//
// ⭐ Фаз всего две: матч либо играется, либо закончен. «Между раздачами» отдельным
// состоянием не является — перераздача происходит в тот же момент, что и подсчёт итога,
// и промежуток, в который клиент мог бы прислать команду, просто не существует.
type MatchPhase uint8

const (
	// MatchInDeal — идёт раздача.
	MatchInDeal MatchPhase = iota

	// MatchOver — кому-то навешен джокер, и он не вышел первым (§0.2).
	MatchOver
)

var matchPhaseNames = [...]string{"IN_DEAL", "MATCH_OVER"}

// String — имя фазы. Совпадает с Java: оно уходит в снимок состояния и в протокол,
// поэтому это часть контракта, а не отладочный вывод.
func (p MatchPhase) String() string {
	if int(p) >= len(matchPhaseNames) {
		return "MatchPhase(?)"
	}
	return matchPhaseNames[p]
}

// MatchState — состояние матча, «бардака» целиком (§0).
//
// ⭐ Матч первичен, раздача вложена в него. Разделение не косметическое: уровни навесов
// живут здесь и переживают перераздачу, а руки, слоты и колода живут в DealState
// и обнуляются вместе с ней (ADR-018, §0.6).
//
// Значение неизменяемое, как и DealState: все «изменения» возвращают копию.
type MatchState struct {
	// Phase — идёт раздача или матч уже закончен.
	Phase MatchPhase
	// NavesLevels — уровни по местам; это и есть счёт матча (ADR-017).
	NavesLevels []int
	// DealNo — номер текущей раздачи, с единицы.
	DealNo int
	// MatchSeed — seed матча; под-seed каждой раздачи производится от него (§6).
	MatchSeed int64
	// Deal — текущая раздача; после конца матча — последняя сыгранная.
	Deal DealState
	// Results — итоги сыгранных раздач по порядку.
	Results []DealOutcome
}

// NewMatchState собирает снимок матча с защитными копиями срезов.
//
// ⚠️ Без копий «неизменяемое» состояние менялось бы через чужую ссылку: срез в Go
// разделяет память с тем, из чего он сделан.
func NewMatchState(phase MatchPhase, navesLevels []int, dealNo int, matchSeed int64,
	deal DealState, results []DealOutcome) MatchState {
	return MatchState{
		Phase:       phase,
		NavesLevels: copyLevels(navesLevels),
		DealNo:      dealNo,
		MatchSeed:   matchSeed,
		Deal:        deal.Clone(),
		Results:     copyOutcomes(results),
	}
}

// PlayerCount — сколько мест за столом. Число мест фиксировано на весь матч, поэтому
// длина шкал и есть размер стола.
func (m MatchState) PlayerCount() int { return len(m.NavesLevels) }

// NavesLevelAt — уровень игрока по шкале навесов.
//
// Место берётся из самого состояния матча, поэтому выход за стол — ошибка вызывающего,
// а не игровая ситуация.
func (m MatchState) NavesLevelAt(seatNo int) int { return m.NavesLevels[seatNo] }

// IsOver — матч закончен.
func (m MatchState) IsOver() bool { return m.Phase == MatchOver }

// LastResult — итог последней раздачи; в нём же и проигравшие матч, если матч закончился.
// Второе значение false, пока не сыграно ни одной раздачи.
func (m MatchState) LastResult() (DealOutcome, bool) {
	if len(m.Results) == 0 {
		return DealOutcome{}, false
	}
	return m.Results[len(m.Results)-1], true
}

// MainLoser — главный проигравший матча (§0.3). Второе значение false, пока матч идёт.
func (m MatchState) MainLoser() (PlayerOutcome, bool) {
	last, ok := m.LastResult()
	if !ok {
		return PlayerOutcome{}, false
	}
	return last.MainLoser()
}

// WithPhase возвращает матч в другой фазе.
func (m MatchState) WithPhase(phase MatchPhase) MatchState {
	next := m
	next.Phase = phase
	return next
}

// WithNavesLevels возвращает матч с новым счётом.
func (m MatchState) WithNavesLevels(levels []int) MatchState {
	next := m
	next.NavesLevels = copyLevels(levels)
	return next
}

// WithDealNo возвращает матч с другим номером раздачи.
func (m MatchState) WithDealNo(dealNo int) MatchState {
	next := m
	next.DealNo = dealNo
	return next
}

// WithDeal возвращает матч с другой текущей раздачей.
func (m MatchState) WithDeal(deal DealState) MatchState {
	next := m
	next.Deal = deal
	return next
}

// WithResult дописывает итог сыгранной раздачи.
//
// ⭐ Именно дописывает, а не заменяет: история раздач — единственное, что остаётся
// от раздачи после подсчёта, восстановить её потом неоткуда.
func (m MatchState) WithResult(outcome DealOutcome) MatchState {
	next := m
	results := make([]DealOutcome, 0, len(m.Results)+1)
	results = append(results, m.Results...)
	next.Results = append(results, outcome)
	return next
}

// Clone — глубокая копия изменяемых частей.
func (m MatchState) Clone() MatchState {
	next := m
	next.NavesLevels = copyLevels(m.NavesLevels)
	next.Results = copyOutcomes(m.Results)
	next.Deal = m.Deal.Clone()
	return next
}

// MatchOutcome — итог команды на уровне матча.
//
// ⭐ Как и в раздаче, отказ возвращается без состояния: отклонённая команда не меняет
// ничего (§4.2).
//
// ⚠️ Назван не MoveResult: тот занят итогом хода ВНУТРИ раздачи. Типы разные — здесь
// в состоянии лежат уровни и накопленные итоги, и подменять один другим нельзя.
type MatchOutcome struct {
	// Applied — команда принята.
	Applied bool
	// State — новое состояние матча; осмысленно только при Applied.
	State MatchState
	// Events — что произошло в раздаче; матч своих событий не добавляет.
	Events []DealEvent
	// Reason — причина отказа; пусто, если команда принята.
	Reason RejectionReason
}

// AppliedMatch — команда применена.
func AppliedMatch(state MatchState, events []DealEvent) MatchOutcome {
	return MatchOutcome{Applied: true, State: state, Events: events}
}

// RejectedMatch — команда отклонена с причиной.
func RejectedMatch(reason RejectionReason) MatchOutcome {
	return MatchOutcome{Applied: false, Reason: reason}
}

// DealApplier — движок раздачи: одна команда превращается в новое состояние либо в отказ.
type DealApplier interface {
	Apply(state DealState, command DealCommand) MoveResult
}

// DealScorer — подсчёт итога законченной раздачи.
type DealScorer interface {
	Score(state DealState) (DealOutcome, error)
}

// DealStarter — сдача раздачи и всё, что автомату матча нужно знать про расклад.
type DealStarter interface {
	// StartDeal — новая раздача от под-seed; уровни навесов приходят снаружи,
	// потому что живут весь матч.
	StartDeal(navesLevels []int, dealSeed int64) (DealState, error)

	// HasAnyTrumpInHands — козырь есть хоть у кого-то.
	HasAnyTrumpInHands(deal DealState) bool

	// ReshuffleSeed — seed пересдачи: другой расклад, но по-прежнему производный
	// от seed матча (§6).
	ReshuffleSeed(seed int64, attempt int) int64
}

// MatchEngine — автомат матча (§4.1): раздача → итог → перераздача, пока кому-то
// не навесят джокер.
//
// ⭐ Раздача заканчивается — матч не обязан. «Карты кончились» это штатное завершение
// раздачи, а не вырождение партии (§0.6): уровни переносятся, колода собирается заново,
// и игра продолжается.
//
// ⭐ Матч закончен только тогда, когда после всех сдвигов у кого-то остался джокер.
// Джокер сам по себе конца не означает: выход первым его снимает (corner case 1).
type MatchEngine struct {
	deals   DealApplier
	scoring DealScorer
	dealer  DealStarter
}

// NewMatchEngine собирает автомат матча над частями раздачи.
//
// ⭐ Части приходят интерфейсами, а не создаются внутри, как в Java: автомат матча
// проверяется на подставных частях, без сдачи настоящей колоды. Правило «джокер кончает
// матч» не должно зависеть от того, повезло ли боту доиграть до джокера.
func NewMatchEngine(deals DealApplier, scoring DealScorer, dealer DealStarter) *MatchEngine {
	return &MatchEngine{deals: deals, scoring: scoring, dealer: dealer}
}

// NewMatchEngineFor — боевая сборка матча для конкретных правил стола: те же части,
// что и в Java-конструкторе, — движок раздачи, подсчёт итога и сдающий.
func NewMatchEngineFor(config RulesConfig) *MatchEngine {
	return NewMatchEngine(NewDealEngineFor(config), NewDealScoring(config),
		NewDealer(config, SeededDice{}))
}

// NewDefaultMatchEngine — матч по боевым правилам бардака.
func NewDefaultMatchEngine() *MatchEngine {
	return NewMatchEngineFor(DefaultRulesConfig())
}

// StartMatch — новый матч: все на «летит 6», первая раздача сдана.
func (e *MatchEngine) StartMatch(playerCount int, matchSeed int64) (MatchState, error) {
	levels := make([]int, playerCount)
	for seat := range levels {
		levels[seat] = NoNaves
	}
	deal, err := e.dealer.StartDeal(levels, DealSeed(matchSeed, 1))
	if err != nil {
		return MatchState{}, err
	}
	return NewMatchState(MatchInDeal, levels, 1, matchSeed, deal, nil), nil
}

// Apply — команда игрока.
//
// Пока раздача не кончилась, матч просто передаёт её движку раздачи; на PhaseDealOver
// считается итог и начинается следующая раздача — либо матч заканчивается.
//
// Ошибка возвращается только на несостоятельности расклада (сдать раздачу не вышло,
// итог не считается): игровой отказ — это MatchOutcome с Applied=false, а не error.
func (e *MatchEngine) Apply(state MatchState, command DealCommand) (MatchOutcome, error) {
	if state.IsOver() {
		return RejectedMatch(NotYourTurn), nil
	}
	move := e.deals.Apply(state.Deal, command)
	if !move.Applied {
		return RejectedMatch(move.Reason), nil
	}
	if move.State.Phase == PhaseDealOver {
		return e.closeDeal(state, move)
	}
	deal, err := e.reshuffleIfNobodyHasTrump(state, move.State)
	if err != nil {
		return MatchOutcome{}, err
	}
	return AppliedMatch(state.WithDeal(deal), move.Events), nil
}

// reshuffleIfNobodyHasTrump — пересдача, когда козырь назвали, а козырей ни у кого нет.
//
// ⭐ Козырь могли назвать костью — и назвать масть, которой нет ни у кого (§1.2). Тогда
// первый ход определять не из чего, и раздача пересдаётся, как и при козыре с нижней
// карты (OQ-22).
//
// ⚠️ Проверка привязана к началу раздачи: фаза ATTACK и пустой стол. Позже по ходу
// раздачи козырей на руках может не остаться совершенно законно — пересдавать там нечего.
func (e *MatchEngine) reshuffleIfNobodyHasTrump(state MatchState, deal DealState) (DealState, error) {
	if !deal.HasTrump() || deal.Phase != PhaseAttack || len(deal.Table) != 0 ||
		e.dealer.HasAnyTrumpInHands(deal) {
		return deal, nil
	}
	seed := e.dealer.ReshuffleSeed(DealSeed(state.MatchSeed, state.DealNo), 0)
	return e.dealer.StartDeal(state.NavesLevels, seed)
}

// closeDeal — раздача сыграна: считаем итог, переносим уровни и либо заканчиваем матч,
// либо сдаём заново.
//
// ⭐ Карты из слотов, включая джокеры, возвращаются в игру сами собой: колода собирается
// заново по составу стола (§2.3). Переносится только уровень — навешенные карты живут
// одну раздачу.
func (e *MatchEngine) closeDeal(state MatchState, move MoveResult) (MatchOutcome, error) {
	outcome, err := e.scoring.Score(move.State)
	if err != nil {
		return MatchOutcome{}, err
	}
	levels := make([]int, 0, len(outcome.Players))
	for _, player := range outcome.Players {
		levels = append(levels, player.LevelAfter)
	}

	next := state.
		WithNavesLevels(levels).
		WithResult(outcome).
		WithDeal(move.State)
	if outcome.IsMatchOver() {
		return AppliedMatch(next.WithPhase(MatchOver), move.Events), nil
	}

	// ⚠️ Номер следующей раздачи растёт ДО вывода под-seed: одинаковый номер дал бы
	// одинаковый расклад, и матч зациклился бы на одной сдаче.
	dealNo := state.DealNo + 1
	deal, err := e.dealer.StartDeal(levels, DealSeed(state.MatchSeed, dealNo))
	if err != nil {
		return MatchOutcome{}, err
	}
	return AppliedMatch(next.WithDealNo(dealNo).WithDeal(deal), move.Events), nil
}

// DealSeed — под-seed раздачи.
//
// ⭐ Весь матч воспроизводим по паре «seed матча + последовательность команд», включая
// перераздачи (§6). Поэтому случайность раздачи не берётся из времени или глобального
// генератора, а выводится отсюда.
func DealSeed(matchSeed int64, dealNo int) int64 {
	return matchSeed*1_000_003 + int64(dealNo)
}

// copyLevels — защитная копия шкал по местам.
func copyLevels(levels []int) []int {
	if levels == nil {
		return []int{}
	}
	out := make([]int, len(levels))
	copy(out, levels)
	return out
}

// copyOutcomes — защитная копия истории раздач.
func copyOutcomes(results []DealOutcome) []DealOutcome {
	if results == nil {
		return []DealOutcome{}
	}
	out := make([]DealOutcome, len(results))
	copy(out, results)
	return out
}
