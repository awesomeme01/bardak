package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Столы и места против НАСТОЯЩЕГО Postgres со схемой Java.
//
// ⚠️ Половина поведения здесь принадлежит базе: уникальные индексы (table_id, seat_no) и
// ux_table_players_user, частичные индексы «ровно один по умолчанию», регистр кода.
// На подделке ничего из этого не проверить — она согласится на что угодно.

// seatTableUser заводит игрока для теста столов.
//
// Имя своё, а не общее: у соседних тестов свои помощники, и одинаковые имена в одном
// пакете просто не соберутся.
func seatTableUser(t *testing.T, users Users, name string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := users.Insert(context.Background(), User{
		ID: id, Username: name + "-" + id[:8], DisplayName: name, PasswordHash: "hash",
	}); err != nil {
		t.Fatalf("игрок %s не завёлся: %v", name, err)
	}
	return id
}

// seatTable заводит стол с хозяином.
func seatTable(t *testing.T, tables Tables, host string, private bool) GameTable {
	t.Helper()
	cardSet, err := tables.DefaultCardSetID(context.Background())
	if err != nil {
		t.Fatalf("набор карт по умолчанию: %v", err)
	}
	theme, err := tables.DefaultThemeID(context.Background())
	if err != nil {
		t.Fatalf("тема по умолчанию: %v", err)
	}
	table, err := tables.Insert(context.Background(), GameTable{
		// ⚠️ Код в базе всегда в верхнем регистре: его алфавит из заглавных букв,
		// а поиск приводит запрошенное к upper. Строчный код здесь не нашёлся бы.
		ID: uuid.NewString(), Code: strings.ToUpper(uuid.NewString()[:6]), Name: "Стол",
		HostUserID: host, MaxPlayers: 2, CardSetID: cardSet, ThemeID: theme,
		RulesConfig: `{"trumpRule":"LAST"}`, IsPrivate: private,
	})
	if err != nil {
		t.Fatalf("стол не завёлся: %v", err)
	}
	return table
}

func TestTableRoundTrip(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	host := seatTableUser(t, users, "Хозяин")
	table := seatTable(t, tables, host, false)

	if table.Status != TableWaiting {
		t.Errorf("статус нового стола %q, ждали WAITING — его ставит база", table.Status)
	}
	if table.CreatedAt.IsZero() {
		t.Error("created_at ставит база, он не должен быть пустым")
	}
	if table.ClosedAt != nil {
		t.Error("новый стол не закрыт, closed_at обязан быть пустым")
	}
	if table.RulesConfig == "" {
		t.Error("правила стола потерялись при чтении jsonb")
	}

	found, err := tables.FindByID(ctx, table.ID)
	if err != nil {
		t.Fatalf("чтение по id: %v", err)
	}
	if found.Code != table.Code {
		t.Errorf("код разошёлся: %q против %q", found.Code, table.Code)
	}

	if _, err := tables.FindByID(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Errorf("несуществующий стол должен давать ErrNotFound, получили %v", err)
	}
}

// ⭐ Код диктуют голосом и пересылают в переписке, где клавиатура сама ставит заглавную
// букву. Точное сравнение означало бы «код верный, но стол не найден».
func TestFindTableByCodeIgnoresCase(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	table := seatTable(t, tables, seatTableUser(t, users, "Хозяин"), false)
	upperCode := table.Code

	for _, variant := range []string{upperCode, strings.ToLower(upperCode)} {
		found, err := tables.FindByCode(ctx, variant)
		if err != nil {
			t.Errorf("код %q не нашёлся: %v", variant, err)
			continue
		}
		if found.ID != table.ID {
			t.Errorf("код %q нашёл чужой стол", variant)
		}
	}
}

// ⚠️ Приватный стол не виден в лобби НИКОГДА, закрытый — тоже: попасть на приватный
// можно только по коду.
func TestOpenTablesHidePrivateAndClosed(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	public := seatTable(t, tables, seatTableUser(t, users, "Открытый"), false)
	private := seatTable(t, tables, seatTableUser(t, users, "Скрытый"), true)
	closed := seatTable(t, tables, seatTableUser(t, users, "Закрытый"), false)
	if err := tables.Close(ctx, closed.ID, time.Now()); err != nil {
		t.Fatalf("закрытие стола: %v", err)
	}

	open, err := tables.FindOpen(ctx)
	if err != nil {
		t.Fatalf("список открытых столов: %v", err)
	}
	visible := map[string]bool{}
	for _, table := range open {
		visible[table.ID] = true
	}

	if !visible[public.ID] {
		t.Error("открытый стол пропал из лобби")
	}
	if visible[private.ID] {
		t.Error("приватный стол виден в лобби — его нельзя показывать никогда")
	}
	if visible[closed.ID] {
		t.Error("закрытый стол виден в лобби")
	}
}

// ⭐ Гонка за последнее место: двое читают расклад, оба выбирают одно место. Правду
// держит уникальный индекс (table_id, seat_no), а не проверка в коде.
func TestSeatCannotBeTakenTwice(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	table := seatTable(t, tables, seatTableUser(t, users, "Хозяин"), false)
	first := seatTableUser(t, users, "Первый")
	second := seatTableUser(t, users, "Второй")

	if _, err := tables.InsertSeat(ctx, TablePlayer{TableID: table.ID, UserID: first, SeatNo: 0}); err != nil {
		t.Fatalf("посадка первого: %v", err)
	}
	_, err := tables.InsertSeat(ctx, TablePlayer{TableID: table.ID, UserID: second, SeatNo: 0})
	if !errors.Is(err, ErrSeatTaken) {
		t.Errorf("занятое место должно давать ErrSeatTaken, получили %v", err)
	}

	// Тот же игрок на то же место — это уже не «опоздал», а «он и так здесь».
	_, err = tables.InsertSeat(ctx, TablePlayer{TableID: table.ID, UserID: first, SeatNo: 1})
	if !errors.Is(err, ErrAlreadySeated) {
		t.Errorf("повтор того же игрока должен давать ErrAlreadySeated, получили %v", err)
	}
}

// ⭐ ux_table_players_user: строка в table_players существует ровно одна на игрока
// во всей базе. Именно этот индекс ловит гонку «создать стол пять раз подряд».
func TestPlayerCannotSitAtTwoTables(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	host := seatTableUser(t, users, "Хозяин")
	first := seatTable(t, tables, host, false)
	second := seatTable(t, tables, host, false)

	player := seatTableUser(t, users, "Игрок")
	if _, err := tables.InsertSeat(ctx, TablePlayer{TableID: first.ID, UserID: player, SeatNo: 0}); err != nil {
		t.Fatalf("посадка за первый стол: %v", err)
	}

	_, err := tables.InsertSeat(ctx, TablePlayer{TableID: second.ID, UserID: player, SeatNo: 0})
	if !errors.Is(err, ErrSeatedElsewhere) {
		t.Errorf("второй стол должен давать ErrSeatedElsewhere, получили %v", err)
	}
}

func TestSeatsAndLeave(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	table := seatTable(t, tables, seatTableUser(t, users, "Хозяин"), false)
	player := seatTableUser(t, users, "Игрок")

	seat, err := tables.InsertSeat(ctx, TablePlayer{TableID: table.ID, UserID: player, SeatNo: 1})
	if err != nil {
		t.Fatalf("посадка: %v", err)
	}
	if seat.State != SeatJoined || seat.IsReady() {
		t.Errorf("новое место должно быть JOINED и не готово, получили %q", seat.State)
	}
	if seat.JoinedAt.IsZero() {
		t.Error("joined_at ставит база")
	}

	seats, err := tables.Seats(ctx, table.ID)
	if err != nil || len(seats) != 1 {
		t.Fatalf("мест за столом: %v (%d)", err, len(seats))
	}

	found, err := tables.SeatOf(ctx, player)
	if err != nil {
		t.Fatalf("место игрока: %v", err)
	}
	if found.TableID != table.ID {
		t.Errorf("место игрока указывает на чужой стол %q", found.TableID)
	}

	removed, err := tables.DeleteSeat(ctx, table.ID, player)
	if err != nil || !removed {
		t.Fatalf("выход из-за стола: %v (%v)", err, removed)
	}
	// Повтор безвреден: цель уже достигнута.
	removed, err = tables.DeleteSeat(ctx, table.ID, player)
	if err != nil || removed {
		t.Fatalf("повторный выход должен быть тихим, получили %v (%v)", err, removed)
	}
	if _, err := tables.SeatOf(ctx, player); !errors.Is(err, ErrNotFound) {
		t.Errorf("после выхода места быть не должно, получили %v", err)
	}
}

// ⚠️ Закрытие стола НЕ трогает места — так же, как в Java: стол просто перестаёт
// считаться текущим.
func TestCloseKeepsSeats(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	host := seatTableUser(t, users, "Хозяин")
	table := seatTable(t, tables, host, false)
	if _, err := tables.InsertSeat(ctx, TablePlayer{TableID: table.ID, UserID: host, SeatNo: 0}); err != nil {
		t.Fatalf("посадка хозяина: %v", err)
	}

	closedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := tables.Close(ctx, table.ID, closedAt); err != nil {
		t.Fatalf("закрытие: %v", err)
	}

	closed, err := tables.FindByID(ctx, table.ID)
	if err != nil {
		t.Fatalf("чтение закрытого стола: %v", err)
	}
	if closed.Status != TableClosed {
		t.Errorf("статус после закрытия %q, ждали CLOSED", closed.Status)
	}
	if closed.ClosedAt == nil {
		t.Error("closed_at не проставился")
	}

	seats, err := tables.Seats(ctx, table.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 1 {
		t.Errorf("места после закрытия стола остаются, нашли %d", len(seats))
	}

	if err := tables.Close(ctx, uuid.NewString(), closedAt); !errors.Is(err, ErrNotFound) {
		t.Errorf("закрытие несуществующего стола должно давать ErrNotFound, получили %v", err)
	}
}

// Значения по умолчанию приходят из миграции V2 и держатся частичными уникальными
// индексами — их ровно по одному.
func TestDefaultCardSetAndTheme(t *testing.T) {
	pool := testDB(t)
	tables := NewTables(pool)
	ctx := context.Background()

	cardSet, err := tables.DefaultCardSetID(ctx)
	if err != nil || cardSet == "" {
		t.Fatalf("набор карт по умолчанию: %v (%q)", err, cardSet)
	}
	theme, err := tables.DefaultThemeID(ctx)
	if err != nil || theme == "" {
		t.Fatalf("тема по умолчанию: %v (%q)", err, theme)
	}
}

// ⚠️ Имя пропавшего игрока не приходит вовсе — прочерк подставляет сценарий, а не база.
func TestDisplayNamesOf(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	player := seatTableUser(t, users, "Игрок")
	missing := uuid.NewString()

	names, err := tables.DisplayNamesOf(ctx, []string{player, missing})
	if err != nil {
		t.Fatalf("имена игроков: %v", err)
	}
	if names[player] != "Игрок" {
		t.Errorf("имя игрока %q, ждали «Игрок»", names[player])
	}
	if _, found := names[missing]; found {
		t.Error("несуществующий игрок не должен попадать в ответ")
	}

	empty, err := tables.DisplayNamesOf(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("пустой список должен давать пустую карту без запроса: %v (%d)", err, len(empty))
	}
}

// Переход стола в матч и обратно.
//
// ⚠️ Половина смысла здесь принадлежит базе: переход обусловлен текущим статусом, и
// именно это не даёт двум одновременным MATCH_START завести за столом два матча.
func TestTableGoesIntoMatchAndBack(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	host := seatTableUser(t, users, "Хозяин")
	table := seatTable(t, tables, host, false)
	if _, err := tables.InsertSeat(ctx, TablePlayer{TableID: table.ID, UserID: host,
		SeatNo: 0, State: SeatReady}); err != nil {
		t.Fatalf("хозяин не сел: %v", err)
	}

	if err := tables.StartMatch(ctx, table.ID); err != nil {
		t.Fatalf("стол не перешёл в матч: %v", err)
	}
	inMatch, err := tables.FindByID(ctx, table.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inMatch.Status != TableInMatch {
		t.Fatalf("статус %q, ждали IN_MATCH", inMatch.Status)
	}
	if inMatch.Version <= table.Version {
		t.Errorf("версия не выросла: было %d, стало %d", table.Version, inMatch.Version)
	}

	// ⭐ Второй старт не проходит: стол уже не ждёт. Это и есть защита от двух матчей
	// за одним столом — проверка «матч уже идёт» и переход не атомарны, а этот update да.
	if err := tables.StartMatch(ctx, table.ID); !errors.Is(err, ErrWrongTableStatus) {
		t.Fatalf("второй старт прошёл: %v", err)
	}

	if err := tables.FinishMatch(ctx, table.ID); err != nil {
		t.Fatalf("стол не вернулся в лобби: %v", err)
	}
	back, err := tables.FindByID(ctx, table.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.Status != TableWaiting {
		t.Fatalf("статус %q, ждали WAITING", back.Status)
	}

	// ⭐ Готовность снята со всех: следующий матч начинается по общему согласию,
	// а не потому, что галочка осталась с прошлого раза.
	seats, err := tables.Seats(ctx, table.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 1 || seats[0].IsReady() {
		t.Fatalf("готовность не сброшена: %+v", seats)
	}
}

func TestSeatReadyIsSetAndCleared(t *testing.T) {
	pool := testDB(t)
	tables, users := NewTables(pool), NewUsers(pool)
	ctx := context.Background()

	host := seatTableUser(t, users, "Готовый")
	table := seatTable(t, tables, host, false)
	if _, err := tables.InsertSeat(ctx, TablePlayer{TableID: table.ID, UserID: host,
		SeatNo: 0, State: SeatJoined}); err != nil {
		t.Fatal(err)
	}

	ready, err := tables.SetSeatReady(ctx, table.ID, host, true)
	if err != nil {
		t.Fatalf("готовность не поставилась: %v", err)
	}
	if !ready.IsReady() {
		t.Fatalf("состояние места %q, ждали READY", ready.State)
	}

	back, err := tables.SetSeatReady(ctx, table.ID, host, false)
	if err != nil {
		t.Fatalf("готовность не снялась: %v", err)
	}
	if back.IsReady() {
		t.Fatalf("состояние места %q, ждали JOINED", back.State)
	}

	// Не за этим столом — отказ, а не молчаливое ничего: иначе клиент считал бы,
	// что готовность принята.
	if _, err := tables.SetSeatReady(ctx, table.ID, uuid.NewString(), true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("готовность за чужой стол прошла: %v", err)
	}
}
