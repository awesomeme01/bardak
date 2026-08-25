package application

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// ErrMatchAlreadyStarted — за столом уже идёт матч.
var ErrMatchAlreadyStarted = errors.New("матч уже идёт")

// MatchLogStore — что нужно матчу от журнала.
type MatchLogStore interface {
	StartMatch(ctx context.Context, id, tableID string, playersCount int, seed int64,
		rulesSnapshot string) (repository.MatchRecord, error)
	ActiveMatchFor(ctx context.Context, tableID string) (repository.MatchRecord, error)
	LatestSnapshot(ctx context.Context, matchID string) (int, string, error)
}

// MatchPlayerStore — места матча в базе.
type MatchPlayerStore interface {
	Seat(ctx context.Context, matchID string, userIDs []string) error
}

// MatchSeatStore — чтение мест уже начатого матча.
type MatchSeatStore interface {
	ParticipantsOf(ctx context.Context, matchID string) ([]repository.HistoryParticipant, error)
}

// StateCodec — снимок состояния матча ↔ JSON.
//
// ⭐ Интерфейс здесь, а реализация в транспорте: формат снимка — часть протокола
// совместимости с Java, а сценарий матча про JSON знать не обязан.
type StateCodec interface {
	EncodeState(state game.MatchState) (string, error)
	DecodeState(raw string) (game.MatchState, error)
}

// ⭐ Проверка на этапе компиляции: репозитории удовлетворяют интерфейсам матча.
// Разъехавшаяся сигнатура всплывает сборкой, а не на первом живом матче.
var (
	_ MatchLogStore    = repository.MatchLog{}
	_ MatchPlayerStore = repository.MatchPlayers{}
	_ MatchSeatStore   = repository.MatchHistory{}
)

// MatchService — запуск и хранение идущих матчей.
//
// ⚠️ Матчи живут В ПАМЯТИ узла, в базе — журнал и снимки. Со вторым узлом это сломалось
// бы, но второго узла нет по решению (ADR-061), и это осознанная плата.
type MatchService struct {
	lobby   LobbyService
	tables  TableStore
	log     MatchLogStore
	players MatchPlayerStore
	seats   MatchSeatStore
	codec   StateCodec
	newID   func() string
	newSeed func() int64
	logger  *slog.Logger

	mu       sync.Mutex
	sessions map[string]*MatchSession
}

// NewMatchService собирает сценарий матчей.
//
// newID и newSeed подменяемы: без этого матч не воспроизвести в тесте, а «плавающее»
// правило не поймать.
func NewMatchService(lobby LobbyService, tables TableStore, log MatchLogStore,
	players MatchPlayerStore, seats MatchSeatStore, codec StateCodec,
	newID func() string, newSeed func() int64, logger *slog.Logger) *MatchService {
	if newID == nil {
		newID = uuid.NewString
	}
	if newSeed == nil {
		newSeed = secureSeed
	}
	return &MatchService{
		lobby: lobby, tables: tables, log: log, players: players, seats: seats,
		codec: codec, newID: newID, newSeed: newSeed, logger: logger,
		sessions: map[string]*MatchSession{},
	}
}

// Find — идущий матч за столом, из памяти или из снимка.
//
// Второе значение false — матча нет: стол ни разу не играл либо матч уже закончен.
func (s *MatchService) Find(ctx context.Context, tableID string) (*MatchSession, bool) {
	s.mu.Lock()
	session, ok := s.sessions[tableID]
	s.mu.Unlock()
	if ok {
		return session, true
	}
	return s.restore(ctx, tableID)
}

// Start начинает матч за столом.
//
// ⭐ Порядок мест в движке — это порядок мест за столом. Он определяет очерёдность хода
// и фиксируется на весь матч, поэтому берётся ровно один раз, здесь.
func (s *MatchService) Start(ctx context.Context, tableID string) (*MatchSession, error) {
	if _, running := s.Find(ctx, tableID); running {
		return nil, ErrMatchAlreadyStarted
	}
	ready, err := s.lobby.IsReadyToStart(ctx, tableID)
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, ErrTableNotReady
	}

	table, err := s.tables.FindByID(ctx, tableID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrTableNotFound
	}
	if err != nil {
		return nil, err
	}
	seats, err := s.tables.Seats(ctx, tableID)
	if err != nil {
		return nil, err
	}

	config := ParseRulesConfig(table.RulesConfig, s.logger)
	// ⭐ Seed матча из криптографического источника, а дальше всё случайное выводится
	// из него: матч воспроизводится по паре «seed + последовательность команд».
	seed := s.newSeed()
	state, err := game.NewMatchEngineFor(config).StartMatch(len(seats), seed)
	if err != nil {
		return nil, fmt.Errorf("раздача не собралась: %w", err)
	}

	// ⭐ rules_snapshot обязателен: правила стола могут поменяться, а матч должен остаться
	// интерпретируемым ровно по тем, по которым игрался.
	rules := table.RulesConfig
	if rules == "" {
		rules = "{}"
	}
	record, err := s.log.StartMatch(ctx, s.newID(), tableID, len(seats), seed, rules)
	if err != nil {
		return nil, fmt.Errorf("матч не записан: %w", err)
	}

	userIDs := make([]string, 0, len(seats))
	for _, seat := range seats {
		userIDs = append(userIDs, seat.UserID)
	}
	// ⚠️ Места матча пишутся СРАЗУ, пустыми: итог будет потом, а порядок мест известен
	// только сейчас. Возьми их потом из лобби — и после рестарта игрок получит чужую руку,
	// причём молча: расклад будет выглядеть совершенно правдоподобно.
	if err := s.players.Seat(ctx, record.ID, userIDs); err != nil {
		return nil, err
	}
	if err := s.lobby.StartMatch(ctx, tableID); err != nil {
		return nil, err
	}

	session := NewMatchSession(tableID, record.ID, s.owners(ctx, tableID, userIDs), config, state)
	s.remember(tableID, session)
	if s.logger != nil {
		s.logger.Info("матч начат", "tableId", tableID, "matchId", record.ID,
			"players", len(userIDs))
	}
	return session, nil
}

// Finish убирает матч из памяти. Стол при этом не трогается: возвращает его в лобби тот,
// кто знает, чем матч кончился.
func (s *MatchService) Finish(tableID string) {
	s.mu.Lock()
	delete(s.sessions, tableID)
	s.mu.Unlock()
}

// restore поднимает матч из последнего снимка.
//
// ⭐ В памяти матча нет, а в базе он числится идущим — значит, сервер перезапускали.
// Игроки за столом об этом знать не должны: они переподключатся и продолжат с того же
// места (ADR-004). События заново НЕ проигрываются — состояние берётся из снимка целиком.
func (s *MatchService) restore(ctx context.Context, tableID string) (*MatchSession, bool) {
	record, err := s.log.ActiveMatchFor(ctx, tableID)
	if err != nil {
		return nil, false
	}
	seq, raw, err := s.log.LatestSnapshot(ctx, record.ID)
	if err != nil {
		return nil, false
	}
	state, err := s.codec.DecodeState(raw)
	if err != nil {
		s.warn("снимок матча не разобран", "matchId", record.ID, "err", err)
		return nil, false
	}
	table, err := s.tables.FindByID(ctx, tableID)
	if err != nil {
		return nil, false
	}

	userIDs, err := s.seatUsersOf(ctx, record.ID, tableID)
	if err != nil {
		return nil, false
	}

	session := NewMatchSession(tableID, record.ID, s.owners(ctx, tableID, userIDs),
		ParseRulesConfig(table.RulesConfig, s.logger), state)
	session.SetLastSeq(seq)

	// ⚠️ Проверка под замком: пока читался снимок, матч мог поднять сосед по горутине.
	// Две сессии одного стола — это две правды об одной раздаче.
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[tableID]; ok {
		return existing, true
	}
	s.sessions[tableID] = session
	if s.logger != nil {
		s.logger.Info("матч поднят из снимка", "tableId", tableID, "matchId", record.ID, "seq", seq)
	}
	return session, true
}

// seatUsersOf — места матча по порядку.
//
// ⚠️ Из МАТЧА, а не из лобби: лобби живёт своей жизнью — кто-то встал, кто-то сел,
// порядок мест изменился. В снимке места это индексы, и чужой порядок раздал бы игрокам
// чужие руки.
func (s *MatchService) seatUsersOf(ctx context.Context, matchID, tableID string) ([]string, error) {
	participants, err := s.seats.ParticipantsOf(ctx, matchID)
	if err != nil {
		return nil, err
	}
	if len(participants) > 0 {
		userIDs := make([]string, 0, len(participants))
		for _, participant := range participants {
			userIDs = append(userIDs, participant.UserID)
		}
		return userIDs, nil
	}

	// Матчи, начатые до появления match_players, восстанавливаются по-старому.
	s.warn("у матча нет записанных мест, беру порядок из лобби", "matchId", matchID)
	seats, err := s.tables.Seats(ctx, tableID)
	if err != nil {
		return nil, err
	}
	userIDs := make([]string, 0, len(seats))
	for _, seat := range seats {
		userIDs = append(userIDs, seat.UserID)
	}
	return userIDs, nil
}

// owners — места матча вместе с именами.
//
// Имена нужны рассылке, а не движку: не нашлись — матч всё равно идёт, в событиях будет
// прочерк, как и в Java.
func (s *MatchService) owners(ctx context.Context, tableID string, userIDs []string) []SeatOwner {
	names, err := s.tables.DisplayNamesOf(ctx, userIDs)
	if err != nil {
		s.warn("не прочитал имена игроков стола", "tableId", tableID, "err", err)
		names = map[string]string{}
	}
	owners := make([]SeatOwner, 0, len(userIDs))
	for seatNo, userID := range userIDs {
		name, ok := names[userID]
		if !ok {
			name = unknownDisplayName
		}
		owners = append(owners, SeatOwner{SeatNo: seatNo, UserID: userID, DisplayName: name})
	}
	return owners
}

func (s *MatchService) remember(tableID string, session *MatchSession) {
	s.mu.Lock()
	s.sessions[tableID] = session
	s.mu.Unlock()
}

func (s *MatchService) warn(message string, args ...any) {
	if s.logger != nil {
		s.logger.Warn(message, args...)
	}
}

// secureSeed — seed матча из криптографического источника.
//
// ⚠️ Не время и не счётчик: seed определяет всю раздачу, и предсказуемый источник
// означал бы предсказуемые карты у того, кто знает, когда матч начался.
func secureSeed() int64 {
	var buffer [8]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		panic(fmt.Sprintf("нет источника случайности: %v", err))
	}
	return int64(binary.BigEndian.Uint64(buffer[:]))
}
