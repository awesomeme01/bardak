package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Сценарии лобби на подделке хранилища.
//
// ⭐ Подделка повторяет ровно те запреты, что держит база: место не достаётся двоим
// (unique table_id, seat_no) и игрок не сидит за двумя столами (ux_table_players_user).
// Без них проверять посадку бессмысленно — вся её логика построена именно на этих отказах.
// Сами индексы проверяются в repository/table_test.go на настоящем Postgres.

type fakeTables struct {
	tables map[string]repository.GameTable
	seats  []repository.TablePlayer
	names  map[string]string

	cardSet string
	theme   string

	// beforeSeat вызывается перед вставкой места — так подделывается гонка:
	// соседний запрос успевает занять выбранное место.
	beforeSeat func(store *fakeTables)
	inserted   int
}

func newFakeTables() *fakeTables {
	return &fakeTables{
		tables:  map[string]repository.GameTable{},
		names:   map[string]string{},
		cardSet: "card-set-default",
		theme:   "theme-default",
	}
}

func (f *fakeTables) FindOpen(context.Context) ([]repository.GameTable, error) {
	open := make([]repository.GameTable, 0)
	for _, table := range f.tables {
		if table.Status == repository.TableWaiting && !table.IsPrivate {
			open = append(open, table)
		}
	}
	return open, nil
}

func (f *fakeTables) FindByID(_ context.Context, id string) (repository.GameTable, error) {
	table, found := f.tables[id]
	if !found {
		return repository.GameTable{}, repository.ErrNotFound
	}
	return table, nil
}

func (f *fakeTables) FindByCode(_ context.Context, code string) (repository.GameTable, error) {
	for _, table := range f.tables {
		if equalFoldASCII(table.Code, code) {
			return table, nil
		}
	}
	return repository.GameTable{}, repository.ErrNotFound
}

func (f *fakeTables) ExistsByCode(_ context.Context, code string) (bool, error) {
	for _, table := range f.tables {
		if table.Code == code {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeTables) Insert(_ context.Context, table repository.GameTable) (repository.GameTable, error) {
	table.Status = repository.TableWaiting
	table.CreatedAt = time.Unix(0, 0)
	f.tables[table.ID] = table
	return table, nil
}

func (f *fakeTables) Close(_ context.Context, id string, at time.Time) error {
	table, found := f.tables[id]
	if !found {
		return repository.ErrNotFound
	}
	table.Status = repository.TableClosed
	table.ClosedAt = &at
	f.tables[id] = table
	return nil
}

func (f *fakeTables) Seats(_ context.Context, tableID string) ([]repository.TablePlayer, error) {
	seats := make([]repository.TablePlayer, 0)
	for _, seat := range f.seats {
		if seat.TableID == tableID {
			seats = append(seats, seat)
		}
	}
	return seats, nil
}

func (f *fakeTables) SeatAt(_ context.Context, tableID, userID string) (repository.TablePlayer, error) {
	for _, seat := range f.seats {
		if seat.TableID == tableID && seat.UserID == userID {
			return seat, nil
		}
	}
	return repository.TablePlayer{}, repository.ErrNotFound
}

func (f *fakeTables) SeatOf(_ context.Context, userID string) (repository.TablePlayer, error) {
	for _, seat := range f.seats {
		if seat.UserID == userID {
			return seat, nil
		}
	}
	return repository.TablePlayer{}, repository.ErrNotFound
}

func (f *fakeTables) InsertSeat(_ context.Context, seat repository.TablePlayer) (repository.TablePlayer, error) {
	if f.beforeSeat != nil {
		hook := f.beforeSeat
		f.beforeSeat = nil
		hook(f)
	}
	for _, existing := range f.seats {
		switch {
		case existing.TableID == seat.TableID && existing.UserID == seat.UserID:
			return repository.TablePlayer{}, repository.ErrAlreadySeated
		case existing.TableID == seat.TableID && existing.SeatNo == seat.SeatNo:
			return repository.TablePlayer{}, repository.ErrSeatTaken
		case existing.UserID == seat.UserID:
			return repository.TablePlayer{}, repository.ErrSeatedElsewhere
		}
	}
	if seat.State == "" {
		seat.State = repository.SeatJoined
	}
	seat.JoinedAt = time.Unix(0, 0)
	f.seats = append(f.seats, seat)
	f.inserted++
	return seat, nil
}

func (f *fakeTables) DeleteSeat(_ context.Context, tableID, userID string) (bool, error) {
	for i, seat := range f.seats {
		if seat.TableID == tableID && seat.UserID == userID {
			f.seats = append(f.seats[:i], f.seats[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeTables) DefaultCardSetID(context.Context) (string, error) {
	if f.cardSet == "" {
		return "", repository.ErrNotFound
	}
	return f.cardSet, nil
}

func (f *fakeTables) DefaultThemeID(context.Context) (string, error) {
	if f.theme == "" {
		return "", repository.ErrNotFound
	}
	return f.theme, nil
}

func (f *fakeTables) DisplayNamesOf(_ context.Context, userIDs []string) (map[string]string, error) {
	names := map[string]string{}
	for _, id := range userIDs {
		if name, found := f.names[id]; found {
			names[id] = name
		}
	}
	return names, nil
}

// seatDirectly сажает игрока мимо сценария — так готовится исходное состояние.
func (f *fakeTables) seatDirectly(tableID, userID string, seatNo int) {
	f.seats = append(f.seats, repository.TablePlayer{
		TableID: tableID, UserID: userID, SeatNo: seatNo, State: repository.SeatJoined,
	})
}

func (f *fakeTables) addTable(id, code string, status string, maxPlayers int) repository.GameTable {
	table := repository.GameTable{
		ID: id, Code: code, Name: "Стол " + id, HostUserID: "host", MaxPlayers: maxPlayers,
		Status: status, CardSetID: "card-set-default", ThemeID: "theme-default",
		RulesConfig: "{}",
	}
	f.tables[id] = table
	return table
}

func equalFoldASCII(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := 0; i < len(left); i++ {
		a, b := left[i], right[i]
		if a >= 'a' && a <= 'z' {
			a -= 32
		}
		if b >= 'a' && b <= 'z' {
			b -= 32
		}
		if a != b {
			return false
		}
	}
	return true
}

func newTestLobby(store *fakeTables) LobbyService {
	return NewLobbyService(store, func() time.Time { return time.Unix(1, 0) }, nil)
}

func TestCreateSeatsHostAtFirstSeat(t *testing.T) {
	store := newFakeTables()
	store.names["host"] = "Хозяин"
	lobby := newTestLobby(store)

	snapshot, err := lobby.Create(context.Background(), CreateTableCommand{
		HostUserID: "host", Name: "Вечерний", MaxPlayers: 4,
	})
	if err != nil {
		t.Fatalf("создание стола: %v", err)
	}

	if snapshot.Table.Status != repository.TableWaiting {
		t.Errorf("новый стол должен ждать игроков, статус %q", snapshot.Table.Status)
	}
	if len(snapshot.Table.Code) != codeLength {
		t.Errorf("код стола длиной %d, ждали %d", len(snapshot.Table.Code), codeLength)
	}
	// ⭐ Хозяин сразу за столом и на нулевом месте: без него стол родился бы пустым.
	if len(snapshot.Seats) != 1 || snapshot.Seats[0].SeatNo != 0 {
		t.Fatalf("хозяин не сел на место 0: %+v", snapshot.Seats)
	}
	if snapshot.Seats[0].DisplayName != "Хозяин" {
		t.Errorf("имя игрока %q, ждали «Хозяин»", snapshot.Seats[0].DisplayName)
	}
	if snapshot.Table.CardSetID != "card-set-default" || snapshot.Table.ThemeID != "theme-default" {
		t.Error("не подставились набор карт и тема по умолчанию")
	}
}

// ⚠️ Пустое имя игрока — не повод не показать стол: подставляется прочерк.
func TestSnapshotFallsBackToDashForUnknownPlayer(t *testing.T) {
	store := newFakeTables()
	lobby := newTestLobby(store)

	snapshot, err := lobby.Create(context.Background(), CreateTableCommand{
		HostUserID: "host", Name: "Вечерний", MaxPlayers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Seats[0].DisplayName != unknownDisplayName {
		t.Errorf("имя пропавшего игрока %q, ждали прочерк", snapshot.Seats[0].DisplayName)
	}
}

// ⚠️ Посреди матча новый стол не создаётся вовсе — иначе движок ждал бы ушедшего.
func TestCreateRefusedDuringMatch(t *testing.T) {
	store := newFakeTables()
	store.addTable("old", "AAAAAA", repository.TableInMatch, 2)
	store.seatDirectly("old", "host", 0)
	lobby := newTestLobby(store)

	_, err := lobby.Create(context.Background(), CreateTableCommand{
		HostUserID: "host", Name: "Новый", MaxPlayers: 2,
	})
	if !errors.Is(err, ErrMatchInProgress) {
		t.Fatalf("ждали отказ «идёт матч», получили %v", err)
	}
	var inMatch MatchInProgressError
	if !errors.As(err, &inMatch) || inMatch.Message != "Сначала доиграй за текущим столом" {
		t.Errorf("текст отказа зависит от места и разошёлся с Java: %v", err)
	}
	if len(store.tables) != 1 {
		t.Error("стол не должен был появиться")
	}
}

// Прошлое место освобождается, а опустевший стол закрывается: пустой стол в списке —
// это приглашение, за которым никого нет.
func TestCreateReleasesPreviousSeatAndClosesEmptyTable(t *testing.T) {
	store := newFakeTables()
	store.addTable("old", "AAAAAA", repository.TableWaiting, 2)
	store.seatDirectly("old", "host", 0)
	lobby := newTestLobby(store)

	snapshot, err := lobby.Create(context.Background(), CreateTableCommand{
		HostUserID: "host", Name: "Новый", MaxPlayers: 2,
	})
	if err != nil {
		t.Fatalf("создание стола: %v", err)
	}
	if store.tables["old"].Status != repository.TableClosed {
		t.Errorf("опустевший стол не закрылся: %q", store.tables["old"].Status)
	}
	if store.tables["old"].ClosedAt == nil {
		t.Error("closed_at не проставился")
	}
	seat, err := store.SeatOf(context.Background(), "host")
	if err != nil || seat.TableID != snapshot.Table.ID {
		t.Errorf("игрок остался за прошлым столом: %+v (%v)", seat, err)
	}
}

// ⭐ Порядок шагов: значения по умолчанию разбираются ПЕРВЫМИ. Иначе ненастроенный набор
// карт сначала поднял бы игрока из-за прошлого стола и закрыл его, а потом ответил 500 —
// игрок остался бы и без старого стола, и без нового.
func TestCreateKeepsSeatWhenDefaultMissing(t *testing.T) {
	store := newFakeTables()
	store.cardSet = ""
	store.addTable("old", "AAAAAA", repository.TableWaiting, 2)
	store.seatDirectly("old", "host", 0)
	lobby := newTestLobby(store)

	_, err := lobby.Create(context.Background(), CreateTableCommand{
		HostUserID: "host", Name: "Новый", MaxPlayers: 2,
	})
	if !errors.Is(err, ErrNoDefaultCardSet) {
		t.Fatalf("ждали отказ «нет набора по умолчанию», получили %v", err)
	}
	if store.tables["old"].Status != repository.TableWaiting {
		t.Error("прошлый стол закрылся, хотя новый так и не появился")
	}
	if _, err := store.SeatAt(context.Background(), "old", "host"); err != nil {
		t.Error("игрок потерял место за прошлым столом из-за несостоявшегося создания")
	}
}

// ⭐ Гонка за место: пока считали расклад, нулевое место увели. Пересчитываем и садимся
// на следующее — это и есть смысл повторных попыток.
func TestJoinRetriesWhenSeatTakenUnderfoot(t *testing.T) {
	store := newFakeTables()
	table := store.addTable("t", "AAAAAA", repository.TableWaiting, 3)
	store.beforeSeat = func(s *fakeTables) { s.seatDirectly(table.ID, "чужой", 0) }
	lobby := newTestLobby(store)

	seat, err := lobby.Join(context.Background(), table.ID, "игрок")
	if err != nil {
		t.Fatalf("посадка: %v", err)
	}
	if seat.SeatNo != 1 {
		t.Errorf("после гонки игрок должен занять место 1, занял %d", seat.SeatNo)
	}
}

func TestJoinFullTable(t *testing.T) {
	store := newFakeTables()
	table := store.addTable("t", "AAAAAA", repository.TableWaiting, 2)
	store.seatDirectly(table.ID, "первый", 0)
	store.seatDirectly(table.ID, "второй", 1)
	lobby := newTestLobby(store)

	if _, err := lobby.Join(context.Background(), table.ID, "третий"); !errors.Is(err, ErrTableFull) {
		t.Errorf("за полным столом ждали ErrTableFull, получили %v", err)
	}
}

// Повторная посадка идемпотентна: то же место, а не отказ.
func TestJoinTwiceReturnsSameSeat(t *testing.T) {
	store := newFakeTables()
	table := store.addTable("t", "AAAAAA", repository.TableWaiting, 3)
	lobby := newTestLobby(store)
	ctx := context.Background()

	first, err := lobby.Join(ctx, table.ID, "игрок")
	if err != nil {
		t.Fatal(err)
	}
	second, err := lobby.Join(ctx, table.ID, "игрок")
	if err != nil {
		t.Fatalf("повторная посадка: %v", err)
	}
	if first.SeatNo != second.SeatNo || store.inserted != 1 {
		t.Errorf("повторная посадка завела новое место: %d против %d (вставок %d)",
			first.SeatNo, second.SeatNo, store.inserted)
	}
}

// ⚠️ Отказ называет стол: без названия игрок видит «ты уже за столом» и не понимает,
// за каким именно, — а встать можно только зная стол.
func TestJoinRefusesPlayerSeatedElsewhere(t *testing.T) {
	store := newFakeTables()
	other := store.addTable("other", "BBBBBB", repository.TableWaiting, 2)
	other.Name = "Соседний"
	store.tables["other"] = other
	store.seatDirectly("other", "игрок", 0)
	table := store.addTable("t", "AAAAAA", repository.TableWaiting, 2)
	lobby := newTestLobby(store)

	_, err := lobby.Join(context.Background(), table.ID, "игрок")
	if !errors.Is(err, ErrAlreadyAtTable) {
		t.Fatalf("ждали отказ «уже за столом», получили %v", err)
	}
	var seated AlreadyAtTableError
	if !errors.As(err, &seated) || seated.TableName != "Соседний" {
		t.Errorf("в отказе нет названия стола: %v", err)
	}
}

func TestJoinClosedTable(t *testing.T) {
	store := newFakeTables()
	table := store.addTable("t", "AAAAAA", repository.TableInMatch, 2)
	lobby := newTestLobby(store)

	if _, err := lobby.Join(context.Background(), table.ID, "игрок"); !errors.Is(err, ErrTableNotOpen) {
		t.Errorf("за идущий матч садиться нельзя, ждали ErrTableNotOpen, получили %v", err)
	}
	if _, err := lobby.Join(context.Background(), "нет-такого", "игрок"); !errors.Is(err, ErrTableNotFound) {
		t.Errorf("ждали ErrTableNotFound, получили %v", err)
	}
}

// ⚠️ Расхождение с Java, осознанное: после закрытия стола строка в table_players
// остаётся, и в Java она запирает игрока навсегда — любая следующая посадка падает на
// ux_table_players_user и выглядит как «нет свободных мест» про пустой новый стол.
func TestClosedTableDoesNotTrapPlayer(t *testing.T) {
	store := newFakeTables()
	store.addTable("old", "AAAAAA", repository.TableClosed, 2)
	store.seatDirectly("old", "host", 0)
	lobby := newTestLobby(store)
	ctx := context.Background()

	current, err := lobby.Current(ctx, "host")
	if err != nil {
		t.Fatalf("текущий стол: %v", err)
	}
	if current.Table != nil {
		t.Error("закрытый стол не может быть текущим")
	}

	if _, err := lobby.Create(ctx, CreateTableCommand{
		HostUserID: "host", Name: "Новый", MaxPlayers: 2,
	}); err != nil {
		t.Fatalf("после закрытия стола игрок обязан снова заводить столы: %v", err)
	}
}

// ⭐ Место 0 — законное значение, и его обязано быть видно: хозяин сидит именно на нём.
func TestCurrentReturnsSeatZero(t *testing.T) {
	store := newFakeTables()
	store.addTable("t", "AAAAAA", repository.TableInMatch, 2)
	store.seatDirectly("t", "host", 0)
	lobby := newTestLobby(store)

	current, err := lobby.Current(context.Background(), "host")
	if err != nil {
		t.Fatal(err)
	}
	if current.Table == nil || current.MySeatNo == nil {
		t.Fatalf("текущий стол не найден: %+v", current)
	}
	if *current.MySeatNo != 0 {
		t.Errorf("место %d, ждали 0", *current.MySeatNo)
	}
	if !current.InMatch {
		t.Error("за столом идёт матч — возвращаться нужно немедленно, флаг обязан быть поднят")
	}
}

func TestCurrentOfPlayerWithoutTable(t *testing.T) {
	lobby := newTestLobby(newFakeTables())

	current, err := lobby.Current(context.Background(), "никто")
	if err != nil {
		t.Fatal(err)
	}
	if current.Table != nil || current.MySeatNo != nil || current.InMatch {
		t.Errorf("игрок нигде не сидит — ответ должен быть пустым: %+v", current)
	}
}

func TestCloseOnlyByHost(t *testing.T) {
	store := newFakeTables()
	store.addTable("t", "AAAAAA", repository.TableWaiting, 2)
	lobby := newTestLobby(store)
	ctx := context.Background()

	if err := lobby.Close(ctx, "t", "чужой"); !errors.Is(err, ErrNotTableHost) {
		t.Errorf("стол закрывает только хозяин, получили %v", err)
	}
	if err := lobby.Close(ctx, "t", "host"); err != nil {
		t.Fatalf("хозяин не смог закрыть стол: %v", err)
	}
	if store.tables["t"].Status != repository.TableClosed {
		t.Error("стол не закрылся")
	}
	// ⚠️ Места остаются: стол просто перестаёт считаться текущим.
	if len(store.seats) != 0 && store.seats[0].TableID != "t" {
		t.Error("закрытие не должно трогать места")
	}
}

func TestCloseRefusedDuringMatch(t *testing.T) {
	store := newFakeTables()
	store.addTable("t", "AAAAAA", repository.TableInMatch, 2)
	lobby := newTestLobby(store)

	err := lobby.Close(context.Background(), "t", "host")
	var inMatch MatchInProgressError
	if !errors.As(err, &inMatch) || inMatch.Message != "Нельзя закрыть стол посреди матча" {
		t.Errorf("ждали отказ с текстом закрытия, получили %v", err)
	}
}

// ⚠️ В приглашении нет ни имён, ни идентификаторов — только стол и число занятых мест.
func TestInviteShowsCountsAndJoinable(t *testing.T) {
	store := newFakeTables()
	table := store.addTable("t", "AAAAAA", repository.TableWaiting, 2)
	store.seatDirectly(table.ID, "первый", 0)
	lobby := newTestLobby(store)
	ctx := context.Background()

	// Регистр кода не важен: его диктуют голосом.
	invite, err := lobby.Invite(ctx, "aaaaaa")
	if err != nil {
		t.Fatalf("приглашение: %v", err)
	}
	if invite.SeatsTaken != 1 || !invite.Joinable {
		t.Errorf("за стол ещё можно сесть: %+v", invite)
	}

	store.seatDirectly(table.ID, "второй", 1)
	invite, err = lobby.Invite(ctx, "AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Joinable {
		t.Error("полный стол не может быть joinable")
	}

	if _, err := lobby.Invite(ctx, "ZZZZZZ"); !errors.Is(err, ErrTableNotFound) {
		t.Errorf("неизвестный код должен давать ErrTableNotFound, получили %v", err)
	}
}

// ⚠️ Приватный стол не попадает в лобби НИКОГДА.
func TestOpenTablesSkipPrivate(t *testing.T) {
	store := newFakeTables()
	store.addTable("open", "AAAAAA", repository.TableWaiting, 2)
	private := store.addTable("secret", "BBBBBB", repository.TableWaiting, 2)
	private.IsPrivate = true
	store.tables["secret"] = private
	lobby := newTestLobby(store)

	tables, err := lobby.OpenTables(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if table.Table.ID == "secret" {
			t.Fatal("приватный стол попал в лобби")
		}
	}
	if len(tables) != 1 {
		t.Errorf("в лобби %d столов, ждали 1", len(tables))
	}
}
