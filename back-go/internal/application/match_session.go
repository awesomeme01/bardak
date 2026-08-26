package application

import (
	"fmt"
	"sync"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// MatchSession — живой матч за одним столом.
//
// ⚠️ Значение изменяемое и НЕ потокобезопасное по умолчанию: им владеет goroutine стола,
// и снаружи к нему обращаются только через её очередь. Мьютекс здесь всё же есть — но
// лишь для чтения состояния наблюдателями (снимок для нового подписчика), а не для игры.
type MatchSession struct {
	TableID string
	MatchID string

	// Seats — кто на каком месте; порядок зафиксирован на старте матча.
	//
	// ⚠️ Места берутся из МАТЧА, а не из лобби: лобби живёт своей жизнью, кто-то встал,
	// кто-то сел, а матч раздан по местам, зафиксированным при старте. Взять их из лобби
	// значит после перезапуска отдать игроку чужую руку.
	Seats []SeatOwner

	mu      sync.RWMutex
	state   game.MatchState
	lastSeq int

	engine  *game.MatchEngine
	config  game.RulesConfig
	applied *appliedCommands

	// movesInDeal и movesDealNo — предохранитель от заклинившей раздачи.
	//
	// ⭐ Счётчик живёт в СЕССИИ, а не в состоянии раздачи, и намеренно: состояние
	// уезжает в снимок, а снимок ещё читает Java. Лишнее поле там — расхождение
	// в окне отката ради страховки, которая переживать перезапуск не обязана:
	// зацикленная раздача — это то, что происходит здесь и сейчас.
	movesInDeal int
	movesDealNo int
}

// SeatOwner — кто сидит на месте.
type SeatOwner struct {
	SeatNo      int
	UserID      string
	DisplayName string
}

// NewMatchSession собирает сессию.
func NewMatchSession(tableID, matchID string, seats []SeatOwner, config game.RulesConfig,
	state game.MatchState) *MatchSession {
	return &MatchSession{
		TableID: tableID,
		MatchID: matchID,
		Seats:   seats,
		state:   state,
		engine:  game.NewMatchEngineFor(config),
		config:  config,
		applied: newAppliedCommands(),
	}
}

// State — текущее состояние матча.
func (s *MatchSession) State() game.MatchState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// LastSeq — номер последнего записанного события.
func (s *MatchSession) LastSeq() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSeq
}

// SetLastSeq запоминает номер последнего события.
func (s *MatchSession) SetLastSeq(seq int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSeq = seq
}

// NextSeq — номер, который получит следующее событие.
func (s *MatchSession) NextSeq() int { return s.LastSeq() + 1 }

// Config — правила стола.
func (s *MatchSession) Config() game.RulesConfig { return s.config }

// SeatOf — место игрока; false, если он за этим столом не играет.
func (s *MatchSession) SeatOf(userID string) (int, bool) {
	for _, seat := range s.Seats {
		if seat.UserID == userID {
			return seat.SeatNo, true
		}
	}
	return 0, false
}

// Naming — как назвать место: кто там сидит и под каким именем.
func (s *MatchSession) Naming(seatNo int) (string, string) {
	for _, seat := range s.Seats {
		if seat.SeatNo == seatNo {
			return seat.UserID, seat.DisplayName
		}
	}
	// ⚠️ Место без владельца — не повод падать: так же ведёт себя Java, подставляя
	// прочерк. Матч важнее аккуратности подписи.
	return "", "—"
}

// Players — идентификаторы игроков матча, в порядке мест.
func (s *MatchSession) Players() []string {
	out := make([]string, 0, len(s.Seats))
	for _, seat := range s.Seats {
		out = append(out, seat.UserID)
	}
	return out
}

// Apply применяет команду к матчу.
//
// ⚠️ Ошибка движка — это не отказ игроку, а поломка: состояние в таком случае НЕ меняется,
// и вызывающий обязан отличать «ход недопустим» от «что-то сломалось».
func (s *MatchSession) Apply(command game.DealCommand) (game.MatchOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	outcome, err := s.engine.Apply(s.state, command)
	if err != nil {
		return game.MatchOutcome{}, err
	}
	if outcome.Applied {
		s.state = outcome.State
		s.countMove()
	}
	return outcome, nil
}

// countMove ведёт счёт ходов текущей раздачи. Зовётся под уже взятым замком.
func (s *MatchSession) countMove() {
	if s.state.DealNo != s.movesDealNo {
		s.movesDealNo = s.state.DealNo
		s.movesInDeal = 0
	}
	s.movesInDeal++
}

// DealIsStuck — раздача выбрала все ходы, отведённые ей правилами стола.
//
// ⚠️ Срабатывание означает не «игроки заигрались», а ЦИКЛ законных ходов, не двигающий
// раздачу: ровно то, от чего стоит защита ADR-051. Один такой прогон уже случился
// (3018 ходов в первой раздаче) и не воспроизвёлся — поэтому здесь предохранитель,
// а не починка невоспроизводимого.
func (s *MatchSession) DealIsStuck() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// ⭐ Граница взята НЕ СТРОГОЙ намеренно. При тысяче ходов разница в один ход
	// не значит ничего, зато предохранитель проверяется настоящим ходом через
	// маршрутизатор (лимит 1 — и первый же ход его трогает), а не подпоркой в тесте,
	// подкручивающей счётчик в обход игры.
	return s.config.MaxMovesPerDeal > 0 && s.movesInDeal >= s.config.MaxMovesPerDeal
}

// MovesInDeal — сколько ходов прожила текущая раздача.
func (s *MatchSession) MovesInDeal() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.movesInDeal
}

// IsOver — матч закончен.
func (s *MatchSession) IsOver() bool {
	return s.State().Phase == game.MatchOver
}

// AlreadyApplied — команда с таким идентификатором уже применялась.
func (s *MatchSession) AlreadyApplied(id string) bool { return s.applied.has(id) }

// Remember отмечает команду применённой.
//
// ⚠️ Зовётся ТОЛЬКО после успешного применения. Запомнить отклонённую значит на повтор
// вернуть снимок вместо причины отказа, и игрок не узнает, почему ход не прошёл.
func (s *MatchSession) Remember(id string) { s.applied.remember(id) }

// appliedCommands — окно применённых команд.
type appliedCommands struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	order []string
}

const rememberedPerMatch = 200

func newAppliedCommands() *appliedCommands {
	return &appliedCommands{seen: make(map[string]struct{}, rememberedPerMatch)}
}

func (a *appliedCommands) has(id string) bool {
	if id == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.seen[id]
	return ok
}

func (a *appliedCommands) remember(id string) {
	if id == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.seen[id]; ok {
		return
	}
	a.seen[id] = struct{}{}
	a.order = append(a.order, id)
	if len(a.order) > rememberedPerMatch {
		delete(a.seen, a.order[0])
		a.order = a.order[1:]
	}
}

// ProjectFor — персональная проекция состояния для места.
func (s *MatchSession) ProjectFor(seatNo int) (game.PlayerView, error) {
	state := s.State()
	projection := game.NewStateProjection(s.config, game.NewDealEngineFor(s.config))
	view, err := projection.Project(state.Deal, seatNo)
	if err != nil {
		return game.PlayerView{}, fmt.Errorf("проекция для места %d: %w", seatNo, err)
	}
	return view, nil
}
