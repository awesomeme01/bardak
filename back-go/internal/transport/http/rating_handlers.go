package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Рейтинг, сезоны и статистика.
//
// ⚠️ Порядок полей в структурах повторяет порядок объявления в Java-записях: Jackson
// пишет поля в порядке объявления, и golden-фикстуры сверяются побайтно. Перестановка
// полей «по алфавиту» сломала бы сверку, ничего не изменив по смыслу.

// RatingInstant — момент времени в том же виде, в каком его печатает Java.
//
// ⚠️ Не RFC3339Nano из коробки: Go обрезает ВСЕ хвостовые нули дробной части
// («…:30.12345Z»), а DateTimeFormatter.ISO_INSTANT, которым пользуется Jackson, печатает
// дробь группами по три цифры («…:30.123450Z») и опускает её целиком, когда она нулевая.
// Расхождение молчаливое: типы те же, тест на «время как время» его не увидит.
type RatingInstant time.Time

// MarshalJSON печатает время правилами ISO_INSTANT.
func (t RatingInstant) MarshalJSON() ([]byte, error) {
	moment := time.Time(t).UTC()
	text := moment.Format("2006-01-02T15:04:05")

	switch nanos := moment.Nanosecond(); {
	case nanos == 0:
		// Дробной части нет вовсе — так же поступает и Java.
	case nanos%1_000_000 == 0:
		text += "." + pad(nanos/1_000_000, 3)
	case nanos%1_000 == 0:
		text += "." + pad(nanos/1_000, 6)
	default:
		text += "." + pad(nanos, 9)
	}
	return json.Marshal(text + "Z")
}

func pad(value, width int) string {
	digits := strconv.Itoa(value)
	for len(digits) < width {
		digits = "0" + digits
	}
	return digits
}

// RatingView — рейтинг игрока с графиком.
type RatingView struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	// Rating — десятичное число БЕЗ кавычек: Jackson печатает BigDecimal числом,
	// сохраняя масштаб из базы.
	Rating        json.Number   `json:"rating"`
	MatchesPlayed int           `json:"matchesPlayed"`
	History       []RatingPoint `json:"history"`
}

// RatingPoint — точка графика: рейтинг до и после матча.
type RatingPoint struct {
	MatchID      string        `json:"matchId"`
	RatingBefore json.Number   `json:"ratingBefore"`
	RatingAfter  json.Number   `json:"ratingAfter"`
	Place        int           `json:"place"`
	PlayersCount int           `json:"playersCount"`
	PlayedAt     RatingInstant `json:"playedAt"`
}

// LeaderRow — строка таблицы лидеров.
type LeaderRow struct {
	UserID        string      `json:"userId"`
	DisplayName   string      `json:"displayName"`
	Rating        json.Number `json:"rating"`
	MatchesPlayed int         `json:"matchesPlayed"`
}

// SeasonView — сезон рейтинга.
type SeasonView struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	StartedAt RatingInstant `json:"startedAt"`
	// ClosedAt — у открытого сезона ключа в ответе НЕТ, а не null.
	ClosedAt *RatingInstant `json:"closedAt,omitempty"`
	Open     bool           `json:"open"`
}

// SeasonsView — сезоны и право спрашивающего ими управлять.
type SeasonsView struct {
	Seasons []SeasonView `json:"seasons"`
	// CanManage — БЕЗ omitempty: «нельзя» это false, а не отсутствие поля. Иначе экран
	// не отличит «права нет» от «сервер не ответил» (MD-003).
	CanManage bool `json:"canManage"`
}

// CreateSeasonRequest — тело открытия следующего сезона.
type CreateSeasonRequest struct {
	Name string `json:"name"`
}

// Validate проверяет имя сезона.
//
// ⚠️ Проверка обязательна: без неё name = null доходил до вставки и давал 500 вместо
// внятного «заполни имя».
func (r CreateSeasonRequest) Validate() error {
	c := newChecker()
	c.size("name", r.Name, 1, 64)
	return c.result()
}

// StreakView — текущая серия побед или поражений.
type StreakView struct {
	Kind   string `json:"kind"`
	Length int    `json:"length"`
}

// DegreeCountView — сколько раз игрок доигрывался до этой степени проигрыша.
type DegreeCountView struct {
	Degree string `json:"degree"`
	Count  int    `json:"count"`
}

// PlayerStatsView — статистика игрока.
//
// ⚠️ Здесь сходятся оба правила MD-003 сразу: avgPlace, bestRating и worstRating
// у не игравшего ВЫРЕЗАЮТСЯ (в Java они null), а degrees остаётся пустым списком.
// Пустая статистика на проводе — ровно
// {"matches":0,"wins":0,"losses":0,"dealsPlayed":0,"streak":{"kind":"NONE","length":0},"degrees":[]}.
type PlayerStatsView struct {
	Matches     int               `json:"matches"`
	Wins        int               `json:"wins"`
	Losses      int               `json:"losses"`
	AvgPlace    *json.Number      `json:"avgPlace,omitempty"`
	DealsPlayed int               `json:"dealsPlayed"`
	Streak      StreakView        `json:"streak"`
	BestRating  *json.Number      `json:"bestRating,omitempty"`
	WorstRating *json.Number      `json:"worstRating,omitempty"`
	Degrees     []DegreeCountView `json:"degrees"`
}

// RatingHandlers — обработчики рейтинга, сезонов и статистики.
//
// ⚠️ Один тип на два префикса (/api/rating и /api/stats): в Java это два контроллера,
// но зависимость у них общая — рейтинг и статистика читают одни и те же таблицы.
type RatingHandlers struct {
	Rating application.RatingService
	Stats  application.StatsService
	Log    *slog.Logger
}

// Routes вешает пути.
//
// ⭐ Пути и методы повторяют Java дословно: фронт не должен меняться вовсе.
func (h RatingHandlers) Routes(router chi.Router) {
	router.Get("/api/rating/me", h.mine)
	router.Get("/api/rating/users/{id}", h.of)
	router.Get("/api/rating/top", h.top)
	router.Get("/api/rating/seasons", h.seasons)
	router.Post("/api/rating/seasons", h.newSeason)
	router.Get("/api/stats/me", h.statsMine)
	router.Get("/api/stats/users/{id}", h.statsOf)
}

func (h RatingHandlers) mine(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	h.writeRating(w, r, principal.UserID)
}

// of — чужой рейтинг.
//
// ⚠️ Вопреки ожиданию «публичного профиля» ручка закрыта токеном: так в SecurityConfig
// Java, где под токен попадает всё, что не перечислено явно.
func (h RatingHandlers) of(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r)
	if !ok {
		return
	}
	h.writeRating(w, r, id)
}

func (h RatingHandlers) writeRating(w http.ResponseWriter, r *http.Request, userID string) {
	view, err := h.Rating.Of(r.Context(), userID)
	if err != nil {
		h.writeRatingError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toRatingView(view))
}

func (h RatingHandlers) top(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Rating.Leaderboard(r.Context())
	if err != nil {
		h.writeRatingError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toLeaderRows(rows))
}

func (h RatingHandlers) seasons(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	// ⚠️ Право берётся по ЛОГИНУ, а не по идентификатору: список ведущих сезона записан
	// логинами и сверяется точным сравнением строк.
	view, err := h.Rating.Seasons(r.Context(), principal.Username)
	if err != nil {
		h.writeRatingError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, SeasonsView{
		Seasons: toSeasonViews(view.Seasons), CanManage: view.CanManage,
	})
}

// newSeason закрывает текущий сезон и открывает следующий.
//
// ⚠️ 200, а не 201: так отвечает Java, и заголовка Location тоже нет.
func (h RatingHandlers) newSeason(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}

	var request CreateSeasonRequest
	if err := DecodeJSON(r, &request); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	// Проверка полей идёт ПЕРЕД проверкой права — как в Java, где @Valid отрабатывает
	// до тела контроллера. Иначе не-ведущий с пустым именем получал бы разные ответы
	// от двух бэкендов.
	if err := request.Validate(); err != nil {
		h.writeRatingValidation(w, r, err)
		return
	}

	season, err := h.Rating.CloseAndOpenSeason(r.Context(), principal.Username, request.Name)
	if err != nil {
		h.writeRatingError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toSeasonView(season))
}

func (h RatingHandlers) statsMine(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	h.writeStats(w, r, principal.UserID)
}

// statsOf — чужая статистика.
//
// ⭐ Открыта любому вошедшему: за столом и так всё друг про друга знают, а прятать
// сыгранные партии от соперника — прятать то, что он видел своими глазами.
func (h RatingHandlers) statsOf(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(w, r)
	if !ok {
		return
	}
	h.writeStats(w, r, id)
}

func (h RatingHandlers) writeStats(w http.ResponseWriter, r *http.Request, userID string) {
	// ⚠️ «Нет такого игрока» здесь не ошибка: неизвестный идентификатор даёт пустую
	// статистику, а не 404.
	stats, err := h.Stats.Of(r.Context(), userID)
	if err != nil {
		WriteError(w, r, h.Log, ErrInternal)
		return
	}
	WriteJSON(w, http.StatusOK, toPlayerStatsView(stats))
}

// principal — владелец запроса.
//
// Токен уже проверен посредником, поэтому его отсутствие здесь — поломка сервера,
// а не ошибка клиента: в Java на этом месте был бы NPE и те же 500.
func (h RatingHandlers) principal(w http.ResponseWriter, r *http.Request) (Principal, bool) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		WriteError(w, r, h.Log, ErrInternal)
		return Principal{}, false
	}
	return principal, true
}

// pathUUID — идентификатор из пути.
//
// ⚠️ Битый UUID — это 400 BAD_REQUEST, а не 404: в Java он не доходит до контроллера,
// его отвергает разбор параметра. Длина проверяется отдельно, потому что Go-разбор
// принимает ещё и формы «без дефисов» и «urn:uuid:…», которых Java не знает.
func (h RatingHandlers) pathUUID(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := chi.URLParam(r, "id")
	parsed, err := uuid.Parse(raw)
	if err != nil || len(raw) != 36 {
		WriteError(w, r, h.Log, ErrBadRequest)
		return "", false
	}
	// Канонический нижний регистр: Java печатает UUID именно так, и ответ обязан
	// совпасть с запросом не по написанию, а по значению.
	return parsed.String(), true
}

func (h RatingHandlers) writeRatingValidation(w http.ResponseWriter, r *http.Request, err error) {
	var validation ValidationError
	if errors.As(err, &validation) {
		WriteError(w, r, h.Log, validation.AsFault())
		return
	}
	WriteError(w, r, h.Log, ErrBadRequest)
}

// writeRatingError переводит ошибку сценария в код и статус.
func (h RatingHandlers) writeRatingError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrUserNotFound):
		// ⚠️ Сообщение — как в Java: «Такого игрока нет», а не общее «Пользователь не найден».
		WriteError(w, r, h.Log, NewFault(http.StatusNotFound, "USER_NOT_FOUND", "Такого игрока нет"))
	case errors.Is(err, application.ErrNotSeasonAdmin):
		WriteError(w, r, h.Log, NewFault(http.StatusForbidden, "NOT_SEASON_ADMIN",
			"Сезон закрывает не каждый"))
	default:
		WriteError(w, r, h.Log, ErrInternal)
	}
}

func toRatingView(view application.RatingView) RatingView {
	// Пустая история — это [], а не null: nil-слайс сериализуется в null, чего Java
	// не отдаёт никогда (MD-003).
	points := make([]RatingPoint, 0, len(view.History))
	for _, entry := range view.History {
		points = append(points, RatingPoint{
			MatchID:      entry.MatchID,
			RatingBefore: json.Number(entry.RatingBefore),
			RatingAfter:  json.Number(entry.RatingAfter),
			Place:        entry.Place,
			PlayersCount: entry.PlayersCount,
			// В Java это createdAt, но наружу поле зовётся playedAt: график рисуется
			// по времени матча, а не по времени записи строки.
			PlayedAt: RatingInstant(entry.CreatedAt),
		})
	}
	return RatingView{
		UserID:        view.UserID,
		DisplayName:   view.DisplayName,
		Rating:        json.Number(view.Rating),
		MatchesPlayed: view.MatchesPlayed,
		History:       points,
	}
}

func toLeaderRows(rows []repository.LeaderRow) []LeaderRow {
	leaders := make([]LeaderRow, 0, len(rows))
	for _, row := range rows {
		leaders = append(leaders, LeaderRow{
			UserID:        row.UserID,
			DisplayName:   row.DisplayName,
			Rating:        json.Number(row.Rating),
			MatchesPlayed: row.MatchesPlayed,
		})
	}
	return leaders
}

func toSeasonViews(seasons []repository.Season) []SeasonView {
	views := make([]SeasonView, 0, len(seasons))
	for _, season := range seasons {
		views = append(views, toSeasonView(season))
	}
	return views
}

func toSeasonView(season repository.Season) SeasonView {
	view := SeasonView{
		ID:        season.ID,
		Name:      season.Name,
		StartedAt: RatingInstant(season.StartedAt),
		Open:      season.IsOpen(),
	}
	if season.ClosedAt != nil {
		closed := RatingInstant(*season.ClosedAt)
		view.ClosedAt = &closed
	}
	return view
}

func toPlayerStatsView(stats application.PlayerStats) PlayerStatsView {
	degrees := make([]DegreeCountView, 0, len(stats.Degrees))
	for _, degree := range stats.Degrees {
		degrees = append(degrees, DegreeCountView{Degree: degree.Degree, Count: degree.Count})
	}
	return PlayerStatsView{
		Matches:     stats.Matches,
		Wins:        stats.Wins,
		Losses:      stats.Losses,
		AvgPlace:    toRatingNumber(stats.AvgPlace),
		DealsPlayed: stats.DealsPlayed,
		Streak:      StreakView{Kind: stats.Streak.Kind, Length: stats.Streak.Length},
		BestRating:  toRatingNumber(stats.BestRating),
		WorstRating: toRatingNumber(stats.WorstRating),
		Degrees:     degrees,
	}
}

// toRatingNumber — пустое значение остаётся пустым: ключа в ответе не будет.
func toRatingNumber(raw *string) *json.Number {
	if raw == nil {
		return nil
	}
	number := json.Number(*raw)
	return &number
}
