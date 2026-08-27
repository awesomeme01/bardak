package application

import (
	"context"
	"errors"
	"testing"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Старт матча и подъём из снимка на подставных хранилищах.
//
// ⭐ База здесь не нужна: проверяются решения — «кого не пускать», «откуда берутся места»,
// «что происходит, когда матч уже идёт». SQL проверяется отдельно, против Postgres.

type fakeMatchLog struct {
	started  []repository.MatchRecord
	active   *repository.MatchRecord
	snapshot string
	seq      int
}

func (f *fakeMatchLog) StartMatch(_ context.Context, id, tableID string, playersCount int,
	seed int64, rulesSnapshot string) (repository.MatchRecord, error) {
	record := repository.MatchRecord{ID: id, TableID: tableID, Status: repository.MatchInProgress,
		PlayersCount: playersCount, RngSeed: seed, RulesSnapshot: rulesSnapshot}
	f.started = append(f.started, record)
	return record, nil
}

func (f *fakeMatchLog) ActiveMatchFor(_ context.Context, _ string) (repository.MatchRecord, error) {
	if f.active == nil {
		return repository.MatchRecord{}, repository.ErrNotFound
	}
	return *f.active, nil
}

func (f *fakeMatchLog) LatestSnapshot(_ context.Context, _ string) (int, string, error) {
	if f.snapshot == "" {
		return 0, "", repository.ErrNotFound
	}
	return f.seq, f.snapshot, nil
}

func (f *fakeMatchLog) SaveSnapshot(_ context.Context, _ string, seq int, state string) error {
	f.seq, f.snapshot = seq, state
	return nil
}

type fakeMatchPlayers struct {
	seated map[string][]string
}

func (f *fakeMatchPlayers) Seat(_ context.Context, matchID string, userIDs []string) error {
	if f.seated == nil {
		f.seated = map[string][]string{}
	}
	f.seated[matchID] = append([]string(nil), userIDs...)
	return nil
}

func (f *fakeMatchPlayers) ParticipantsOf(_ context.Context,
	matchID string) ([]repository.HistoryParticipant, error) {
	participants := []repository.HistoryParticipant{}
	for seatNo, userID := range f.seated[matchID] {
		participants = append(participants, repository.HistoryParticipant{
			UserID: userID, SeatNo: seatNo})
	}
	return participants, nil
}

// jsonStateCodec — настоящий кодек в тестах не нужен: проверяется не формат снимка,
// а то, что состояние доезжает до сессии целиком. Формат проверен своими тестами.
type stubStateCodec struct {
	state game.MatchState
	fail  bool
}

func (c stubStateCodec) EncodeState(game.MatchState) (string, error) { return "снимок", nil }

func (c stubStateCodec) DecodeState(string) (game.MatchState, error) {
	if c.fail {
		return game.MatchState{}, errors.New("снимок не разобран")
	}
	return c.state, nil
}

func matchServiceFixture(t *testing.T) (*MatchService, *fakeTables, *fakeMatchLog, *fakeMatchPlayers) {
	t.Helper()
	store := newFakeTables()
	store.tables["table-1"] = repository.GameTable{ID: "table-1", Status: repository.TableWaiting,
		MaxPlayers: 4, RulesConfig: "{}"}
	store.seats = []repository.TablePlayer{
		{TableID: "table-1", UserID: "user-a", SeatNo: 0, State: repository.SeatReady},
		{TableID: "table-1", UserID: "user-b", SeatNo: 1, State: repository.SeatReady},
	}
	store.names = map[string]string{"user-a": "Аида", "user-b": "Борис"}

	log := &fakeMatchLog{}
	players := &fakeMatchPlayers{}
	lobby := NewLobbyService(store, nil, nil)
	service := NewMatchService(lobby, store, log, players, players, stubStateCodec{},
		func() string { return "match-1" }, func() int64 { return 42 }, nil)
	return service, store, log, players
}

func TestMatchStartsWhenEverybodyIsReady(t *testing.T) {
	service, store, log, players := matchServiceFixture(t)

	session, err := service.Start(context.Background(), "table-1")
	if err != nil {
		t.Fatalf("матч не начался: %v", err)
	}

	if session.MatchID != "match-1" || len(session.Seats) != 2 {
		t.Fatalf("сессия собрана неверно: %+v", session)
	}
	if session.Seats[0].DisplayName != "Аида" {
		t.Fatalf("имя игрока не подставлено: %+v", session.Seats[0])
	}
	if len(log.started) != 1 || log.started[0].RngSeed != 42 {
		t.Fatalf("матч не записан в журнал: %+v", log.started)
	}
	// ⚠️ Места матча пишутся при старте: потом их взять будет неоткуда.
	if got := players.seated["match-1"]; len(got) != 2 || got[0] != "user-a" {
		t.Fatalf("места матча не записаны: %v", got)
	}
	if store.tables["table-1"].Status != repository.TableInMatch {
		t.Fatalf("стол не переведён в матч: %s", store.tables["table-1"].Status)
	}
	if _, ok := service.Find(context.Background(), "table-1"); !ok {
		t.Fatalf("матч не найден сразу после старта")
	}
}

// В одиночку матч не играется, и клиент пытается это регулярно: кнопка есть,
// соперников ещё нет.
func TestMatchRefusesToStartWhenTableIsNotReady(t *testing.T) {
	service, store, _, _ := matchServiceFixture(t)
	store.seats = store.seats[:1]

	_, err := service.Start(context.Background(), "table-1")

	if !errors.Is(err, ErrTableNotReady) {
		t.Fatalf("ждали отказ TABLE_NOT_READY, получили: %v", err)
	}
	if store.tables["table-1"].Status != repository.TableWaiting {
		t.Fatalf("стол увели в матч на отказе: %s", store.tables["table-1"].Status)
	}
}

func TestMatchRefusesToStartTwiceAtTheSameTable(t *testing.T) {
	service, _, log, _ := matchServiceFixture(t)
	if _, err := service.Start(context.Background(), "table-1"); err != nil {
		t.Fatalf("первый старт не прошёл: %v", err)
	}

	_, err := service.Start(context.Background(), "table-1")

	if !errors.Is(err, ErrMatchAlreadyStarted) {
		t.Fatalf("ждали отказ «матч уже идёт», получили: %v", err)
	}
	if len(log.started) != 1 {
		t.Fatalf("второй матч всё-таки записан: %+v", log.started)
	}
}

// Готовность всех — не то же самое, что «двое сели»: неготовый обязан задержать старт.
func TestMatchRefusesToStartWhenSomebodyIsNotReady(t *testing.T) {
	service, store, _, _ := matchServiceFixture(t)
	store.seats[1].State = repository.SeatJoined

	_, err := service.Start(context.Background(), "table-1")

	if !errors.Is(err, ErrTableNotReady) {
		t.Fatalf("ждали отказ TABLE_NOT_READY, получили: %v", err)
	}
}

// ⭐ Рестарт процесса не убивает партию: матч поднимается из снимка, а не из лога.
func TestMatchComesBackFromSnapshotWhenMemoryIsEmpty(t *testing.T) {
	service, _, log, players := matchServiceFixture(t)
	session, err := service.Start(context.Background(), "table-1")
	if err != nil {
		t.Fatalf("матч не начался: %v", err)
	}
	saved := session.State()
	service.Finish("table-1") // сервер «перезапустили»: память пуста, база помнит

	log.active = &repository.MatchRecord{ID: "match-1", TableID: "table-1",
		Status: repository.MatchInProgress, PlayersCount: 2}
	log.snapshot, log.seq = "снимок", 17
	service.codec = stubStateCodec{state: saved}

	restored, ok := service.Find(context.Background(), "table-1")

	if !ok {
		t.Fatalf("матч не поднялся из снимка")
	}
	if restored.MatchID != "match-1" || restored.LastSeq() != 17 {
		t.Fatalf("матч поднят не тем: matchId=%s lastSeq=%d", restored.MatchID, restored.LastSeq())
	}
	// ⚠️ Места берутся из МАТЧА: в лобби к этому времени могут сидеть уже другие.
	if len(players.seated["match-1"]) != len(restored.Seats) {
		t.Fatalf("места восстановлены не из матча: %+v", restored.Seats)
	}
	if restored.Seats[0].UserID != "user-a" || restored.Seats[1].UserID != "user-b" {
		t.Fatalf("порядок мест матча разъехался: %+v", restored.Seats)
	}
}

// ⚠️ Рестарт сервера в окно «матч начат, ходов ноль» раньше оставлял матч
// НЕВОССТАНОВИМЫМ: обычные снимки пишутся по ходам, а стартового не было. Restore
// без снимка бессилен, и стол застревал IN_MATCH навсегда — даже MATCH_LEAVE отвечал
// NO_MATCH. Поэтому снимок обязан появляться вместе с матчем, а не с первым ходом.
func TestMatchStartedWithoutASingleMoveSurvivesRestart(t *testing.T) {
	service, _, log, _ := matchServiceFixture(t)
	session, err := service.Start(context.Background(), "table-1")
	if err != nil {
		t.Fatalf("матч не начался: %v", err)
	}
	if log.snapshot == "" {
		t.Fatalf("стартовый снимок не записан")
	}

	saved := session.State()
	service.Finish("table-1") // сервер «перезапустили» ДО первого хода

	log.active = &repository.MatchRecord{ID: "match-1", TableID: "table-1",
		Status: repository.MatchInProgress, PlayersCount: 2}
	service.codec = stubStateCodec{state: saved}

	restored, ok := service.Find(context.Background(), "table-1")
	if !ok {
		t.Fatalf("матч без единого хода не поднялся после рестарта")
	}
	if restored.LastSeq() != 0 {
		t.Fatalf("стартовый снимок должен нести seq 0, поднято с %d", restored.LastSeq())
	}
}

// ⚠️ Порядок мест берётся из матча, а не из лобби. Иначе игрок, севший на освободившееся
// место, после рестарта получил бы чужую руку — и расклад выглядел бы правдоподобно.
func TestMatchKeepsItsOwnSeatOrderWhenTheLobbyChanged(t *testing.T) {
	service, store, log, _ := matchServiceFixture(t)
	if _, err := service.Start(context.Background(), "table-1"); err != nil {
		t.Fatalf("матч не начался: %v", err)
	}
	service.Finish("table-1")

	// За столом всё переставили местами, пока сервер лежал.
	store.seats = []repository.TablePlayer{
		{TableID: "table-1", UserID: "user-b", SeatNo: 0, State: repository.SeatReady},
		{TableID: "table-1", UserID: "user-c", SeatNo: 1, State: repository.SeatReady},
	}
	log.active = &repository.MatchRecord{ID: "match-1", TableID: "table-1",
		Status: repository.MatchInProgress, PlayersCount: 2}
	log.snapshot, log.seq = "снимок", 3

	restored, ok := service.Find(context.Background(), "table-1")

	if !ok {
		t.Fatalf("матч не поднялся из снимка")
	}
	if restored.Seats[0].UserID != "user-a" || restored.Seats[1].UserID != "user-b" {
		t.Fatalf("места взяты из лобби, а не из матча: %+v", restored.Seats)
	}
}

// Готовый, но ни разу не игравший стол не должен отдавать выдуманную сессию.
func TestMatchStaysEmptyWhenTableNeverPlayed(t *testing.T) {
	service, _, _, _ := matchServiceFixture(t)

	if _, ok := service.Find(context.Background(), "table-1"); ok {
		t.Fatalf("нашёлся матч там, где его не было")
	}
}

// Матч числится идущим, а снимка нет — поднимать нечего, и выдумывать состояние нельзя.
func TestMatchStaysEmptyWhenSnapshotIsMissing(t *testing.T) {
	service, _, log, _ := matchServiceFixture(t)
	log.active = &repository.MatchRecord{ID: "match-1", TableID: "table-1",
		Status: repository.MatchInProgress, PlayersCount: 2}

	if _, ok := service.Find(context.Background(), "table-1"); ok {
		t.Fatalf("матч поднялся без снимка")
	}
}
