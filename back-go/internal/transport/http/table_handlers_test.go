package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Ручки столов проверяются на ПОБАЙТНОЙ форме ответа, а не на «примерно те же поля».
//
// ⚠️ Java вырезает из JSON только null. Наивный Go с omitempty вырезал бы ещё нули,
// false и пустые списки (MD-003) — компилятор промолчит, а фронт получит отсутствующее
// поле там, где ждал ноль. Поэтому здесь сравниваются целые тела ответов.

// lobbyStub — подделка сценариев лобби: ручки не должны знать про базу.
type lobbyStub struct {
	tables  []application.TableSnapshot
	table   application.TableSnapshot
	invite  application.InviteSnapshot
	current application.CurrentSnapshot
	err     error

	created  application.CreateTableCommand
	closedID string
	askedFor string
}

func (s *lobbyStub) OpenTables(context.Context) ([]application.TableSnapshot, error) {
	return s.tables, s.err
}

func (s *lobbyStub) ByID(_ context.Context, tableID string) (application.TableSnapshot, error) {
	s.askedFor = tableID
	return s.table, s.err
}

func (s *lobbyStub) ByCode(_ context.Context, code string) (application.TableSnapshot, error) {
	s.askedFor = code
	return s.table, s.err
}

func (s *lobbyStub) Invite(_ context.Context, code string) (application.InviteSnapshot, error) {
	s.askedFor = code
	return s.invite, s.err
}

func (s *lobbyStub) Current(context.Context, string) (application.CurrentSnapshot, error) {
	return s.current, s.err
}

func (s *lobbyStub) Create(_ context.Context, cmd application.CreateTableCommand) (
	application.TableSnapshot, error) {
	s.created = cmd
	return s.table, s.err
}

func (s *lobbyStub) Close(_ context.Context, tableID, _ string) error {
	s.closedID = tableID
	return s.err
}

// callTables прогоняет запрос через маршруты столов от имени игрока.
func callTables(stub *lobbyStub, method, target, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	TableHandlers{Lobby: stub}.Routes(router)

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	// Владельца запроса обычно кладёт Authenticate; здесь он подставляется напрямую,
	// потому что проверяются ручки, а не разбор токена.
	request = request.WithContext(context.WithValue(request.Context(), principalKey{},
		Principal{UserID: "11111111-1111-1111-1111-111111111111", Username: "shabdan"}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func tableSnapshotForTest() application.TableSnapshot {
	return application.TableSnapshot{
		Table: repository.GameTable{
			ID:         "33333333-3333-3333-3333-333333333333",
			Code:       "ABC234",
			Name:       "Вечерний",
			HostUserID: "11111111-1111-1111-1111-111111111111",
			MaxPlayers: 4,
			Status:     repository.TableWaiting,
			CardSetID:  "11111111-1111-1111-1111-111111111111",
			ThemeID:    "22222222-2222-2222-2222-222222222222",
		},
		Seats: []application.SeatSnapshot{{
			SeatNo: 0, UserID: "11111111-1111-1111-1111-111111111111",
			DisplayName: "Шабдан", Ready: false,
		}},
	}
}

const tableViewJSON = `{"id":"33333333-3333-3333-3333-333333333333","code":"ABC234",` +
	`"name":"Вечерний","hostUserId":"11111111-1111-1111-1111-111111111111","maxPlayers":4,` +
	`"status":"WAITING","cardSetId":"11111111-1111-1111-1111-111111111111",` +
	`"themeId":"22222222-2222-2222-2222-222222222222","isPrivate":false,` +
	`"seats":[{"seatNo":0,"userId":"11111111-1111-1111-1111-111111111111",` +
	`"displayName":"Шабдан","ready":false,"online":true}]}`

// ⚠️ Место 0, ready=false и isPrivate=false обязаны присутствовать в теле: omitempty
// вырезал бы их все разом, и стол на экране остался бы без хозяина.
func TestTableViewKeepsZeroesAndFalses(t *testing.T) {
	stub := &lobbyStub{table: tableSnapshotForTest()}

	response := callTables(stub, http.MethodGet, "/api/tables/33333333-3333-3333-3333-333333333333", "")

	if response.Code != http.StatusOK {
		t.Fatalf("статус %d, ждали 200", response.Code)
	}
	if got := strings.TrimSpace(response.Body.String()); got != tableViewJSON {
		t.Errorf("тело разошлось с Java:\nполучили %s\nждали   %s", got, tableViewJSON)
	}
}

// ⚠️ Пустой список — [], а не null: nil-слайс сериализуется в null, чего Java не отдаёт.
func TestEmptyTableListIsEmptyArray(t *testing.T) {
	stub := &lobbyStub{}

	response := callTables(stub, http.MethodGet, "/api/tables", "")

	if got := strings.TrimSpace(response.Body.String()); got != "[]" {
		t.Errorf("пустое лобби должно отдавать [], получили %s", got)
	}
}

func TestCreateTableReturns200AndPassesFields(t *testing.T) {
	stub := &lobbyStub{table: tableSnapshotForTest()}

	response := callTables(stub, http.MethodPost, "/api/tables",
		`{"name":"Вечерний","maxPlayers":4,"isPrivate":true,"rulesConfig":{"hangingLimit":3}}`)

	// ⚠️ 200, а не 201, и без Location — как в Java.
	if response.Code != http.StatusOK {
		t.Fatalf("статус %d, ждали 200", response.Code)
	}
	if stub.created.HostUserID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("хозяином стола стал не владелец токена: %q", stub.created.HostUserID)
	}
	if !stub.created.IsPrivate {
		t.Error("ключ isPrivate не прочитан — в Java он называется именно так")
	}
	if stub.created.RulesConfig != `{"hangingLimit":3}` {
		t.Errorf("правила стола не дошли до сценария: %q", stub.created.RulesConfig)
	}
	if stub.created.CardSetID != "" || stub.created.ThemeID != "" {
		t.Error("невыбранные набор и тема должны остаться пустыми — их подставит сценарий")
	}
}

// Пустые правила — {}, а не null: колонка объявлена not null default '{}'.
func TestCreateTableWithoutRulesSendsEmptyObject(t *testing.T) {
	stub := &lobbyStub{table: tableSnapshotForTest()}

	callTables(stub, http.MethodPost, "/api/tables", `{"name":"Вечерний","maxPlayers":2}`)

	if stub.created.RulesConfig != "{}" {
		t.Errorf("правила по умолчанию %q, ждали {}", stub.created.RulesConfig)
	}
}

// ⚠️ Тексты в details — дословно как у Bean Validation в Java.
func TestCreateTableValidation(t *testing.T) {
	stub := &lobbyStub{}

	response := callTables(stub, http.MethodPost, "/api/tables", `{"name":"я","maxPlayers":7}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("статус %d, ждали 400", response.Code)
	}
	var body APIError
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "VALIDATION_FAILED" {
		t.Errorf("код ошибки %q, ждали VALIDATION_FAILED", body.Code)
	}
	if body.Details["name"] != "size must be between 2 and 64" {
		t.Errorf("разбор по полю name: %v", body.Details["name"])
	}
	if body.Details["maxPlayers"] != "must be less than or equal to 5" {
		t.Errorf("разбор по полю maxPlayers: %v", body.Details["maxPlayers"])
	}
}

// ⚠️ maxPlayers — примитив: отсутствие поля даёт 0 и ту же ошибку, что явный 0.
func TestCreateTableWithoutMaxPlayers(t *testing.T) {
	stub := &lobbyStub{}

	response := callTables(stub, http.MethodPost, "/api/tables", `{"name":"Вечерний"}`)

	var body APIError
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	if response.Code != http.StatusBadRequest ||
		body.Details["maxPlayers"] != "must be greater than or equal to 2" {
		t.Errorf("ждали 400 с разбором maxPlayers, получили %d %v", response.Code, body.Details)
	}
}

// ⚠️ Игрок, который нигде не сидит, получает РОВНО {"inMatch":false} — ни table,
// ни mySeatNo в теле нет.
func TestCurrentWithoutTable(t *testing.T) {
	stub := &lobbyStub{}

	response := callTables(stub, http.MethodGet, "/api/tables/current", "")

	if got := strings.TrimSpace(response.Body.String()); got != `{"inMatch":false}` {
		t.Errorf("получили %s, ждали {\"inMatch\":false}", got)
	}
}

// ⭐ Место 0 обязано быть в ответе: на нём сидит хозяин стола, и omitempty у обычного
// int вырезал бы именно его.
func TestCurrentKeepsSeatZero(t *testing.T) {
	snapshot := tableSnapshotForTest()
	seatNo := 0
	stub := &lobbyStub{current: application.CurrentSnapshot{
		Table: &snapshot, InMatch: false, MySeatNo: &seatNo,
	}}

	response := callTables(stub, http.MethodGet, "/api/tables/current", "")

	expected := `{"table":` + tableViewJSON + `,"inMatch":false,"mySeatNo":0}`
	if got := strings.TrimSpace(response.Body.String()); got != expected {
		t.Errorf("тело разошлось с Java:\nполучили %s\nждали   %s", got, expected)
	}
}

// ⚠️ В приглашении НЕТ ни имён игроков, ни их идентификаторов: код короткий и живёт
// в переписке, поэтому всё в этом ответе — публично.
func TestInviteHidesPlayers(t *testing.T) {
	stub := &lobbyStub{invite: application.InviteSnapshot{
		Code: "ABC234", Name: "Вечерний", MaxPlayers: 4, SeatsTaken: 0,
		IsPrivate: false, Joinable: true,
	}}

	response := callTables(stub, http.MethodGet, "/api/tables/invite/abc234", "")

	expected := `{"code":"ABC234","name":"Вечерний","maxPlayers":4,"seatsTaken":0,` +
		`"isPrivate":false,"joinable":true}`
	if got := strings.TrimSpace(response.Body.String()); got != expected {
		t.Errorf("тело разошлось с Java:\nполучили %s\nждали   %s", got, expected)
	}
	if stub.askedFor != "abc234" {
		t.Errorf("код дошёл до сценария как %q — регистр приводит запрос к базе, не ручка",
			stub.askedFor)
	}
}

// ⚠️ Невалидный UUID в пути — 400 BAD_REQUEST, а не 500: Java раньше валила такой
// запрос в «что-то пошло не так», это починено (ApiHardeningIT), и Go повторяет починку.
func TestTableByBrokenIDGivesBadRequest(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		stub := &lobbyStub{table: tableSnapshotForTest()}

		response := callTables(stub, method, "/api/tables/не-uuid", "")

		var body APIError
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		if response.Code != http.StatusBadRequest || body.Code != "BAD_REQUEST" {
			t.Fatalf("%s: ждали 400 BAD_REQUEST, получили %d %s", method, response.Code, body.Code)
		}
		if stub.askedFor != "" || stub.closedID != "" {
			t.Errorf("%s: до сценария такой запрос доходить не должен", method)
		}
	}
}

func TestTableNotFound(t *testing.T) {
	stub := &lobbyStub{err: application.ErrTableNotFound}

	response := callTables(stub, http.MethodGet, "/api/tables/by-code/abc234", "")

	var body APIError
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	if response.Code != http.StatusNotFound || body.Code != "TABLE_NOT_FOUND" {
		t.Errorf("ждали 404 TABLE_NOT_FOUND, получили %d %s", response.Code, body.Code)
	}
	if stub.askedFor != "abc234" {
		t.Errorf("код дошёл до сценария как %q — приводить регистр здесь не нужно", stub.askedFor)
	}
}

func TestCloseTableGives204(t *testing.T) {
	stub := &lobbyStub{}

	response := callTables(stub, http.MethodDelete,
		"/api/tables/33333333-3333-3333-3333-333333333333", "")

	if response.Code != http.StatusNoContent {
		t.Fatalf("статус %d, ждали 204", response.Code)
	}
	if response.Body.Len() != 0 {
		t.Errorf("тело должно быть пустым, получили %s", response.Body.String())
	}
	if stub.closedID != "33333333-3333-3333-3333-333333333333" {
		t.Errorf("закрыли не тот стол: %q", stub.closedID)
	}
}

func TestCloseTableByStranger(t *testing.T) {
	stub := &lobbyStub{err: application.ErrNotTableHost}

	response := callTables(stub, http.MethodDelete,
		"/api/tables/33333333-3333-3333-3333-333333333333", "")

	var body APIError
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	if response.Code != http.StatusForbidden || body.Code != "NOT_TABLE_HOST" {
		t.Errorf("ждали 403 NOT_TABLE_HOST, получили %d %s", response.Code, body.Code)
	}
}

// ⚠️ Один код — разные тексты по месту броска. Транспорт берёт текст из ошибки,
// а не из своей константы.
func TestMatchInProgressKeepsMessageOfItsPlace(t *testing.T) {
	cases := map[string]struct {
		err     error
		method  string
		target  string
		body    string
		message string
	}{
		"закрытие стола": {
			err:     application.MatchInProgressError{Message: "Нельзя закрыть стол посреди матча"},
			method:  http.MethodDelete,
			target:  "/api/tables/33333333-3333-3333-3333-333333333333",
			message: "Нельзя закрыть стол посреди матча",
		},
		"создание стола": {
			err:     application.MatchInProgressError{Message: "Сначала доиграй за текущим столом"},
			method:  http.MethodPost,
			target:  "/api/tables",
			body:    `{"name":"Вечерний","maxPlayers":2}`,
			message: "Сначала доиграй за текущим столом",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			response := callTables(&lobbyStub{err: tc.err}, tc.method, tc.target, tc.body)

			var body APIError
			_ = json.Unmarshal(response.Body.Bytes(), &body)
			if response.Code != http.StatusConflict || body.Code != "MATCH_IN_PROGRESS" {
				t.Fatalf("ждали 409 MATCH_IN_PROGRESS, получили %d %s", response.Code, body.Code)
			}
			if body.Message != tc.message {
				t.Errorf("текст %q, ждали %q", body.Message, tc.message)
			}
		})
	}
}

// ⚠️ Отказ «уже за столом» называет стол — иначе встать из-за него невозможно.
func TestAlreadyAtTableNamesTheTable(t *testing.T) {
	stub := &lobbyStub{err: application.AlreadyAtTableError{TableName: "Соседний"}}

	response := callTables(stub, http.MethodPost, "/api/tables", `{"name":"Вечерний","maxPlayers":2}`)

	var body APIError
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	if response.Code != http.StatusConflict || body.Code != "ALREADY_AT_TABLE" {
		t.Fatalf("ждали 409 ALREADY_AT_TABLE, получили %d %s", response.Code, body.Code)
	}
	if !strings.Contains(body.Message, "Соседний") {
		t.Errorf("в сообщении нет названия стола: %q", body.Message)
	}
}

// Ненастроенные значения по умолчанию — это 500 NO_DEFAULT, и текст копирует Java
// вместе с его рассогласованным родом.
func TestNoDefaultsGive500(t *testing.T) {
	for _, tc := range []struct {
		err     error
		message string
	}{
		{application.ErrNoDefaultCardSet, "Не настроен набор карт по умолчанию"},
		{application.ErrNoDefaultTheme, "Не настроен тема стола по умолчанию"},
	} {
		response := callTables(&lobbyStub{err: tc.err}, http.MethodPost, "/api/tables",
			`{"name":"Вечерний","maxPlayers":2}`)

		var body APIError
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		if response.Code != http.StatusInternalServerError || body.Code != "NO_DEFAULT" {
			t.Fatalf("ждали 500 NO_DEFAULT, получили %d %s", response.Code, body.Code)
		}
		if body.Message != tc.message {
			t.Errorf("текст %q, ждали %q", body.Message, tc.message)
		}
	}
}

func TestTableFullAndNotOpen(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{application.ErrTableFull, "TABLE_FULL"},
		{application.ErrTableNotOpen, "TABLE_NOT_OPEN"},
	} {
		response := callTables(&lobbyStub{err: tc.err}, http.MethodPost, "/api/tables",
			`{"name":"Вечерний","maxPlayers":2}`)

		var body APIError
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		if response.Code != http.StatusConflict || body.Code != tc.code {
			t.Errorf("ждали 409 %s, получили %d %s", tc.code, response.Code, body.Code)
		}
	}
}

// ⚠️ Незнакомая ошибка наружу не просачивается: только «что-то пошло не так».
func TestUnexpectedErrorIsHidden(t *testing.T) {
	stub := &lobbyStub{err: context.DeadlineExceeded}

	response := callTables(stub, http.MethodGet, "/api/tables", "")

	var body APIError
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	if response.Code != http.StatusInternalServerError || body.Code != "INTERNAL_ERROR" {
		t.Fatalf("ждали 500 INTERNAL_ERROR, получили %d %s", response.Code, body.Code)
	}
	if strings.Contains(body.Message, "deadline") {
		t.Error("текст исключения ушёл наружу — этого не бывает никогда")
	}
}

// ⭐ /invite/{code} — единственная ручка столов без токена; всё остальное закрыто.
func TestOnlyInviteIsPublic(t *testing.T) {
	public := map[string]bool{
		"/api/tables/invite/ABC234":                        true,
		"/api/tables":                                      false,
		"/api/tables/current":                              false,
		"/api/tables/by-code/ABC234":                       false,
		"/api/tables/33333333-3333-3333-3333-333333333333": false,
	}
	for path, open := range public {
		if got := PublicPath(http.MethodGet, path); got != open {
			t.Errorf("путь %s: открыт=%v, ждали %v", path, got, open)
		}
	}
}
