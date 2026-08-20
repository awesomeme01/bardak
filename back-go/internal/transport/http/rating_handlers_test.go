package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Форма ответа сверяется ПОБАЙТНО, а не по полям.
//
// ⭐ Именно здесь ломается наивный порт: отсутствие ключа и ключ со значением null —
// разные ответы, а omitempty вырезал бы ещё и нули (MD-003). Проверка «поле равно нулю»
// такой ошибки не увидит, проверка строкой — увидит.

// ⚠️ Пустая статистика: avgPlace, bestRating и worstRating ВЫРЕЗАНЫ, degrees остаётся
// пустым списком. Ровно так отдаёт PlayerStats.empty() в Java.
func TestEmptyPlayerStatsMatchesJavaByteForByte(t *testing.T) {
	body, err := json.Marshal(toPlayerStatsView(application.EmptyPlayerStats()))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"matches":0,"wins":0,"losses":0,"dealsPlayed":0,` +
		`"streak":{"kind":"NONE","length":0},"degrees":[]}`
	if string(body) != want {
		t.Errorf("пустая статистика:\n получили %s\n ждали   %s", body, want)
	}
}

// Десятичные едут ЧИСЛОМ без кавычек и с тем масштабом, что пришёл из базы.
func TestPlayerStatsKeepsDecimalScaleAndFieldOrder(t *testing.T) {
	average, best, worst := "2.33", "1012.50", "980.00"
	stats := application.PlayerStats{
		Matches: 3, Wins: 1, Losses: 2, AvgPlace: &average, DealsPlayed: 11,
		Streak:      application.Streak{Kind: "WIN", Length: 1},
		BestRating:  &best,
		WorstRating: &worst,
		Degrees:     []application.DegreeCount{{Degree: "ROYAL", Count: 1}},
	}

	body, err := json.Marshal(toPlayerStatsView(stats))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"matches":3,"wins":1,"losses":2,"avgPlace":2.33,"dealsPlayed":11,` +
		`"streak":{"kind":"WIN","length":1},"bestRating":1012.50,"worstRating":980.00,` +
		`"degrees":[{"degree":"ROYAL","count":1}]}`
	if string(body) != want {
		t.Errorf("статистика:\n получили %s\n ждали   %s", body, want)
	}
}

// Пустая история — это [], а не null: nil-слайс уехал бы как null, чего Java не отдаёт.
func TestRatingViewWithoutHistoryKeepsEmptyList(t *testing.T) {
	view := application.RatingView{
		UserID: "11111111-1111-1111-1111-111111111111", DisplayName: "Шабдан",
		Rating: application.InitialRating,
	}

	body, err := json.Marshal(toRatingView(view))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"userId":"11111111-1111-1111-1111-111111111111","displayName":"Шабдан",` +
		`"rating":1000,"matchesPlayed":0,"history":[]}`
	if string(body) != want {
		t.Errorf("рейтинг новичка:\n получили %s\n ждали   %s", body, want)
	}
}

// ⚠️ У открытого сезона ключа closedAt в ответе НЕТ, а у закрытого он есть.
func TestSeasonHidesClosedAtWhileOpen(t *testing.T) {
	started := time.Date(2026, 8, 20, 9, 30, 0, 0, time.UTC)
	closed := started.Add(24 * time.Hour)

	open, err := json.Marshal(toSeasonView(repository.Season{
		ID: "33333333-3333-3333-3333-333333333333", Name: "Первый сезон", StartedAt: started,
	}))
	if err != nil {
		t.Fatal(err)
	}
	const wantOpen = `{"id":"33333333-3333-3333-3333-333333333333","name":"Первый сезон",` +
		`"startedAt":"2026-08-20T09:30:00Z","open":true}`
	if string(open) != wantOpen {
		t.Errorf("открытый сезон:\n получили %s\n ждали   %s", open, wantOpen)
	}

	done, err := json.Marshal(toSeasonView(repository.Season{
		ID: "44444444-4444-4444-4444-444444444444", Name: "Второй", StartedAt: started,
		ClosedAt: &closed,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(done), `"closedAt":"2026-08-21T09:30:00Z"`) {
		t.Errorf("у закрытого сезона обязана быть дата закрытия: %s", done)
	}
	if !strings.Contains(string(done), `"open":false`) {
		t.Errorf("закрытый сезон обязан отдавать open:false, а не пропускать поле: %s", done)
	}
}

// canManage = false обязано БЫТЬ в ответе: «права нет» — это false, а не отсутствие поля.
func TestSeasonsAlwaysCarryCanManage(t *testing.T) {
	body, err := json.Marshal(SeasonsView{Seasons: []SeasonView{}, CanManage: false})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"seasons":[],"canManage":false}`
	if string(body) != want {
		t.Errorf("сезоны:\n получили %s\n ждали   %s", body, want)
	}
}

// ⚠️ Дробная часть печатается группами по три цифры, как ISO_INSTANT в Java, а не
// с обрезкой всех хвостовых нулей, как RFC3339Nano в Go.
func TestInstantPrintsLikeJava(t *testing.T) {
	base := time.Date(2026, 8, 19, 10, 15, 30, 0, time.UTC)
	cases := []struct {
		nanos int
		want  string
	}{
		{nanos: 0, want: `"2026-08-19T10:15:30Z"`},
		{nanos: 123_000_000, want: `"2026-08-19T10:15:30.123Z"`},
		{nanos: 123_456_000, want: `"2026-08-19T10:15:30.123456Z"`},
		{nanos: 123_450_000, want: `"2026-08-19T10:15:30.123450Z"`},
		{nanos: 100_000_000, want: `"2026-08-19T10:15:30.100Z"`},
		{nanos: 123_456_789, want: `"2026-08-19T10:15:30.123456789Z"`},
	}
	for _, c := range cases {
		moment := RatingInstant(base.Add(time.Duration(c.nanos)))
		body, err := json.Marshal(moment)
		if err != nil {
			t.Fatalf("%d нс: %v", c.nanos, err)
		}
		if string(body) != c.want {
			t.Errorf("%d нс дали %s, ждали %s", c.nanos, body, c.want)
		}
	}
}

// Время приводится к UTC: Java отдаёт Instant, у которого пояса нет вовсе.
func TestInstantIsPrintedInUTC(t *testing.T) {
	zone := time.FixedZone("ALMT", 5*60*60)
	moment := RatingInstant(time.Date(2026, 8, 19, 15, 0, 0, 0, zone))

	body, err := json.Marshal(moment)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `"2026-08-19T10:00:00Z"` {
		t.Errorf("время в другом поясе дало %s, ждали \"2026-08-19T10:00:00Z\"", body)
	}
}

func TestSeasonNameIsChecked(t *testing.T) {
	if err := (CreateSeasonRequest{Name: "Второй сезон"}).Validate(); err != nil {
		t.Errorf("нормальное имя отвергнуто: %v", err)
	}
	// ⚠️ Без проверки пустое имя доходило до вставки и давало 500 вместо внятного отказа.
	if err := (CreateSeasonRequest{Name: "   "}).Validate(); err == nil {
		t.Error("имя из пробелов обязано быть отвергнуто")
	}
	if err := (CreateSeasonRequest{Name: strings.Repeat("я", 65)}).Validate(); err == nil {
		t.Error("имя длиннее 64 символов обязано быть отвергнуто: колонка varchar(64)")
	}
	// Длина в СИМВОЛАХ, а не в байтах: 64 кириллических символа занимают 128 байт.
	if err := (CreateSeasonRequest{Name: strings.Repeat("я", 64)}).Validate(); err != nil {
		t.Errorf("64 кириллических символа обязаны пройти, а получили %v", err)
	}
}

// --- обработчики целиком ---

type stubRatingStore struct {
	history []repository.RatingHistoryEntry
	seasons []repository.Season
}

func (s stubRatingStore) FindRating(context.Context, string) (repository.UserRating, error) {
	return repository.UserRating{}, repository.ErrNotFound
}

func (s stubRatingStore) HistoryOf(context.Context, string) ([]repository.RatingHistoryEntry, error) {
	return s.history, nil
}

func (s stubRatingStore) Leaderboard(context.Context) ([]repository.LeaderRow, error) {
	return nil, nil
}

func (s stubRatingStore) AllSeasons(context.Context) ([]repository.Season, error) {
	return s.seasons, nil
}

func (s stubRatingStore) CloseAndOpenSeason(_ context.Context, id, name string,
	now time.Time) (repository.Season, error) {
	return repository.Season{ID: id, Name: name, StartedAt: now}, nil
}

func (s stubRatingStore) OutcomesOf(context.Context, string) ([]repository.PlayerMatchOutcome, error) {
	return nil, nil
}

type stubRatingUsers struct{ known bool }

func (s stubRatingUsers) FindByID(_ context.Context, id string) (repository.User, error) {
	if !s.known {
		return repository.User{}, repository.ErrNotFound
	}
	return repository.User{ID: id, DisplayName: "Шабдан"}, nil
}

// ratingRouter собирает маршруты вместе с подставным владельцем запроса: посредник
// авторизации в этих тестах не участвует, проверяются сами обработчики.
func ratingRouter(handlers RatingHandlers, principal Principal) chi.Router {
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), principalKey{}, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	handlers.Routes(router)
	return router
}

func ratingHandlersWith(store stubRatingStore, users stubRatingUsers,
	isAdmin func(string) bool) RatingHandlers {
	return RatingHandlers{
		Rating: application.NewRatingService(store, users, isAdmin, func() time.Time {
			return time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
		}),
		Stats: application.NewStatsService(store),
	}
}

// ⚠️ Битый UUID в пути — это 400, а не 404: в Java он не доходит до контроллера.
func TestBrokenUUIDInRatingPathIsBadRequest(t *testing.T) {
	router := ratingRouter(ratingHandlersWith(stubRatingStore{}, stubRatingUsers{known: true}, nil),
		Principal{UserID: "me", Username: "shabdan"})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/rating/users/не-uuid", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("статус %d, ждали 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Errorf("тело %s, ждали код BAD_REQUEST", recorder.Body.String())
	}
}

func TestRatingOfUnknownPlayerIs404(t *testing.T) {
	router := ratingRouter(ratingHandlersWith(stubRatingStore{}, stubRatingUsers{known: false}, nil),
		Principal{UserID: "me", Username: "shabdan"})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/rating/users/11111111-1111-1111-1111-111111111111", nil))

	if recorder.Code != http.StatusNotFound {
		t.Errorf("статус %d, ждали 404", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"USER_NOT_FOUND"`) {
		t.Errorf("тело %s, ждали код USER_NOT_FOUND", recorder.Body.String())
	}
}

// ⚠️ А вот статистика неизвестного игрока — 200 с пустыми полями, а не 404.
func TestStatsOfUnknownPlayerIsEmptyNotAnError(t *testing.T) {
	router := ratingRouter(ratingHandlersWith(stubRatingStore{}, stubRatingUsers{known: false}, nil),
		Principal{UserID: "me", Username: "shabdan"})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/stats/users/11111111-1111-1111-1111-111111111111", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("статус %d, ждали 200", recorder.Code)
	}
	const want = `{"matches":0,"wins":0,"losses":0,"dealsPlayed":0,` +
		`"streak":{"kind":"NONE","length":0},"degrees":[]}`
	if strings.TrimSpace(recorder.Body.String()) != want {
		t.Errorf("тело %s, ждали %s", recorder.Body.String(), want)
	}
}

func TestSeasonsShowManageRightOfTheAsker(t *testing.T) {
	store := stubRatingStore{seasons: []repository.Season{{
		ID: "33333333-3333-3333-3333-333333333333", Name: "Первый сезон",
		StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}}}
	admin := func(username string) bool { return username == "shabdan" }

	for _, c := range []struct {
		username string
		want     string
	}{
		{username: "shabdan", want: `"canManage":true`},
		{username: "гость", want: `"canManage":false`},
	} {
		router := ratingRouter(ratingHandlersWith(store, stubRatingUsers{known: true}, admin),
			Principal{UserID: "me", Username: c.username})

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/rating/seasons", nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: статус %d, ждали 200", c.username, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), c.want) {
			t.Errorf("%s: тело %s, ждали %s", c.username, recorder.Body.String(), c.want)
		}
	}
}

func TestCloseSeasonRefusedForOutsiderWith403(t *testing.T) {
	router := ratingRouter(
		ratingHandlersWith(stubRatingStore{}, stubRatingUsers{known: true},
			func(username string) bool { return username == "shabdan" }),
		Principal{UserID: "me", Username: "гость"})

	request := httptest.NewRequest(http.MethodPost, "/api/rating/seasons",
		strings.NewReader(`{"name":"Второй сезон"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("статус %d, ждали 403", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"NOT_SEASON_ADMIN"`) {
		t.Errorf("тело %s, ждали код NOT_SEASON_ADMIN", recorder.Body.String())
	}
}

// ⚠️ Проверка полей идёт ПЕРЕД проверкой права: так же и в Java, где @Valid отрабатывает
// до тела контроллера.
func TestEmptySeasonNameIsCheckedBeforeTheRight(t *testing.T) {
	router := ratingRouter(
		ratingHandlersWith(stubRatingStore{}, stubRatingUsers{known: true},
			func(string) bool { return false }),
		Principal{UserID: "me", Username: "гость"})

	request := httptest.NewRequest(http.MethodPost, "/api/rating/seasons",
		strings.NewReader(`{"name":""}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("статус %d, ждали 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"VALIDATION_FAILED"`) {
		t.Errorf("тело %s, ждали код VALIDATION_FAILED", recorder.Body.String())
	}
}

func TestNewSeasonAnswersWithTheOpenedSeason(t *testing.T) {
	router := ratingRouter(
		ratingHandlersWith(stubRatingStore{}, stubRatingUsers{known: true},
			func(string) bool { return true }),
		Principal{UserID: "me", Username: "shabdan"})

	request := httptest.NewRequest(http.MethodPost, "/api/rating/seasons",
		strings.NewReader(`{"name":"Второй сезон"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	// ⚠️ 200, а не 201, и без заголовка Location — как в Java.
	if recorder.Code != http.StatusOK {
		t.Fatalf("статус %d, ждали 200", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"name":"Второй сезон"`) || !strings.Contains(body, `"open":true`) {
		t.Errorf("тело %s: ждали открытый сезон с заданным именем", body)
	}
	if strings.Contains(body, "closedAt") {
		t.Errorf("у только что открытого сезона даты закрытия быть не может: %s", body)
	}
}

// Рейтинг новичка: 200 со стартовым значением и пустой историей, а не отказ.
func TestMyRatingWithoutMatchesIsStartingValue(t *testing.T) {
	router := ratingRouter(ratingHandlersWith(stubRatingStore{}, stubRatingUsers{known: true}, nil),
		Principal{UserID: "11111111-1111-1111-1111-111111111111", Username: "shabdan"})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/rating/me", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("статус %d, ждали 200", recorder.Code)
	}
	const want = `{"userId":"11111111-1111-1111-1111-111111111111","displayName":"Шабдан",` +
		`"rating":1000,"matchesPlayed":0,"history":[]}`
	if strings.TrimSpace(recorder.Body.String()) != want {
		t.Errorf("тело %s, ждали %s", recorder.Body.String(), want)
	}
}

// Таблица лидеров пуста — это [], а не null.
func TestEmptyLeaderboardIsEmptyList(t *testing.T) {
	router := ratingRouter(ratingHandlersWith(stubRatingStore{}, stubRatingUsers{known: true}, nil),
		Principal{UserID: "me", Username: "shabdan"})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/rating/top", nil))

	if strings.TrimSpace(recorder.Body.String()) != `[]` {
		t.Errorf("тело %s, ждали []", recorder.Body.String())
	}
}
