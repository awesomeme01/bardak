package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Сценарии рейтинга и статистики на подставном хранилище.
//
// ⭐ База здесь не нужна: проверяется арифметика и решения («не игравший — не ошибка»),
// а SQL проверяется отдельно, в тестах репозитория против настоящего Postgres.

type fakeRatingStore struct {
	rating     *repository.UserRating
	history    []repository.RatingHistoryEntry
	leaders    []repository.LeaderRow
	seasons    []repository.Season
	outcomes   []repository.PlayerMatchOutcome
	openedName string
}

func (f *fakeRatingStore) FindRating(_ context.Context, _ string) (repository.UserRating, error) {
	if f.rating == nil {
		return repository.UserRating{}, repository.ErrNotFound
	}
	return *f.rating, nil
}

func (f *fakeRatingStore) HistoryOf(_ context.Context, _ string) ([]repository.RatingHistoryEntry, error) {
	return f.history, nil
}

func (f *fakeRatingStore) Leaderboard(_ context.Context) ([]repository.LeaderRow, error) {
	return f.leaders, nil
}

func (f *fakeRatingStore) AllSeasons(_ context.Context) ([]repository.Season, error) {
	return f.seasons, nil
}

func (f *fakeRatingStore) CloseAndOpenSeason(_ context.Context, id, name string,
	now time.Time) (repository.Season, error) {
	f.openedName = name
	return repository.Season{ID: id, Name: name, StartedAt: now}, nil
}

func (f *fakeRatingStore) OutcomesOf(_ context.Context, _ string) ([]repository.PlayerMatchOutcome, error) {
	return f.outcomes, nil
}

type fakeRatingUsers struct{ user *repository.User }

func (f fakeRatingUsers) FindByID(_ context.Context, _ string) (repository.User, error) {
	if f.user == nil {
		return repository.User{}, repository.ErrNotFound
	}
	return *f.user, nil
}

func knownRatedPlayer() fakeRatingUsers {
	return fakeRatingUsers{user: &repository.User{
		ID: "11111111-1111-1111-1111-111111111111", DisplayName: "Шабдан",
	}}
}

// ⭐ Не игравший ни разу рейтинга в базе не имеет — ему показывается стартовое значение,
// а не отказ. Строка появится после первого матча.
func TestNewcomerGetsInitialRatingAndEmptyHistory(t *testing.T) {
	service := NewRatingService(&fakeRatingStore{}, knownRatedPlayer(), nil, nil)

	view, err := service.Of(context.Background(), "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("не игравший — не ошибка, а получили %v", err)
	}
	if view.Rating != InitialRating {
		t.Errorf("рейтинг %q, ждали стартовый %q", view.Rating, InitialRating)
	}
	if view.MatchesPlayed != 0 {
		t.Errorf("матчей %d, ждали 0", view.MatchesPlayed)
	}
	if len(view.History) != 0 {
		t.Errorf("история длиной %d, ждали пустую", len(view.History))
	}
	if view.DisplayName != "Шабдан" {
		t.Errorf("имя %q, ждали \"Шабдан\"", view.DisplayName)
	}
}

// ⚠️ А вот несуществующий игрок — уже 404: разница между «ещё не играл» и «такого нет»
// видна только здесь.
func TestRatingOfUnknownPlayerIsNotFound(t *testing.T) {
	service := NewRatingService(&fakeRatingStore{}, fakeRatingUsers{}, nil, nil)

	_, err := service.Of(context.Background(), "нет такого")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("ждали ErrUserNotFound, получили %v", err)
	}
}

func TestExistingRatingWinsOverStartingValue(t *testing.T) {
	store := &fakeRatingStore{rating: &repository.UserRating{Rating: "1012.50", MatchesPlayed: 4}}
	service := NewRatingService(store, knownRatedPlayer(), nil, nil)

	view, err := service.Of(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if view.Rating != "1012.50" || view.MatchesPlayed != 4 {
		t.Errorf("рейтинг %q при %d матчах, ждали \"1012.50\" при 4", view.Rating, view.MatchesPlayed)
	}
}

// ⭐ Право закрыть сезон живёт в настройках рейтинга и едет ВМЕСТЕ со списком: иначе экран
// показывал бы кнопку всем, а отказ прилетал бы уже после нажатия.
func TestSeasonsCarryTheRightToManage(t *testing.T) {
	store := &fakeRatingStore{seasons: []repository.Season{{ID: "s1", Name: "Первый"}}}
	service := NewRatingService(store, knownRatedPlayer(),
		func(username string) bool { return username == "shabdan" }, nil)

	mine, err := service.Seasons(context.Background(), "shabdan")
	if err != nil {
		t.Fatal(err)
	}
	if !mine.CanManage {
		t.Error("ведущий сезона обязан видеть право управлять")
	}
	if len(mine.Seasons) != 1 {
		t.Fatalf("сезонов %d, ждали 1", len(mine.Seasons))
	}

	// ⚠️ Регистр важен: список сверяется точным сравнением строк, в отличие от входа.
	other, err := service.Seasons(context.Background(), "SHABDAN")
	if err != nil {
		t.Fatal(err)
	}
	if other.CanManage {
		t.Error("логин в другом регистре права не даёт: сравнение точное")
	}
}

func TestCloseSeasonRefusedForOutsider(t *testing.T) {
	store := &fakeRatingStore{}
	service := NewRatingService(store, knownRatedPlayer(),
		func(username string) bool { return username == "shabdan" }, nil)

	_, err := service.CloseAndOpenSeason(context.Background(), "гость", "Второй")
	if !errors.Is(err, ErrNotSeasonAdmin) {
		t.Fatalf("ждали ErrNotSeasonAdmin, получили %v", err)
	}
	if store.openedName != "" {
		t.Error("отказ обязан случиться ДО записи: сезон не должен был открыться")
	}
}

func TestCloseSeasonOpensNextWithGivenName(t *testing.T) {
	store := &fakeRatingStore{}
	moment := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	service := NewRatingService(store, knownRatedPlayer(),
		func(string) bool { return true }, func() time.Time { return moment })

	season, err := service.CloseAndOpenSeason(context.Background(), "shabdan", "Второй сезон")
	if err != nil {
		t.Fatalf("смена сезона: %v", err)
	}
	if season.Name != "Второй сезон" || !season.StartedAt.Equal(moment) {
		t.Errorf("сезон открылся как %+v", season)
	}
	if !season.IsOpen() {
		t.Error("новый сезон обязан быть открытым")
	}
}

// ⚠️ Пустая статистика — это нули и пустой список степеней, а НЕ ошибка: неизвестный
// идентификатор в Java даёт ровно PlayerStats.empty().
func TestEmptyStatsForPlayerWithoutMatches(t *testing.T) {
	service := NewStatsService(&fakeRatingStore{})

	stats, err := service.Of(context.Background(), "кто угодно")
	if err != nil {
		t.Fatalf("пустая статистика — не ошибка, а получили %v", err)
	}
	if stats.Matches != 0 || stats.Wins != 0 || stats.Losses != 0 || stats.DealsPlayed != 0 {
		t.Errorf("ждали нули, получили %+v", stats)
	}
	if stats.AvgPlace != nil || stats.BestRating != nil || stats.WorstRating != nil {
		t.Error("среднее место и границы рейтинга у не игравшего отсутствуют, а не равны нулю")
	}
	if stats.Degrees == nil {
		t.Error("степени — ПУСТОЙ СПИСОК, а не отсутствие: nil уехал бы в ответ как null")
	}
	if stats.Streak.Kind != "NONE" || stats.Streak.Length != 0 {
		t.Errorf("серия %+v, ждали NONE длиной 0", stats.Streak)
	}
}

func ratingLossType(value string) *string { return &value }

// Среднее место, победы, поражения и раздачи — всё по истории матчей.
func TestStatsAreCountedFromMatchHistory(t *testing.T) {
	store := &fakeRatingStore{outcomes: []repository.PlayerMatchOutcome{
		{Place: 1, Finished: true, DealsPlayed: 5},
		{Place: 3, LossType: ratingLossType("ROYAL"), Finished: true, DealsPlayed: 2},
		{Place: 3, LossType: ratingLossType("FAIL"), Finished: true, DealsPlayed: 4},
	}}
	service := NewStatsService(store)

	stats, err := service.Of(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Matches != 3 || stats.Wins != 1 || stats.Losses != 2 {
		t.Errorf("матчей/побед/поражений %d/%d/%d, ждали 3/1/2",
			stats.Matches, stats.Wins, stats.Losses)
	}
	if stats.DealsPlayed != 11 {
		t.Errorf("раздач %d, ждали 11", stats.DealsPlayed)
	}
	if stats.AvgPlace == nil || *stats.AvgPlace != "2.33" {
		t.Errorf("среднее место %v, ждали \"2.33\" (7/3 с округлением до сотых)", stats.AvgPlace)
	}
}

// ⚠️ У отменённого матча число раздач ни о чём не говорит — партия оборвалась,
// и в сумму оно не идёт.
func TestDealsCountOnlyFinishedMatches(t *testing.T) {
	store := &fakeRatingStore{outcomes: []repository.PlayerMatchOutcome{
		{Place: 1, Finished: true, DealsPlayed: 5},
		{Place: 2, Finished: false, DealsPlayed: 9},
	}}

	stats, err := NewStatsService(store).Of(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if stats.DealsPlayed != 5 {
		t.Errorf("раздач %d, ждали 5 — незавершённый матч в сумму не идёт", stats.DealsPlayed)
	}
	if stats.Matches != 2 {
		t.Errorf("матчей %d, ждали 2: сам матч в счёт идёт, а его раздачи — нет", stats.Matches)
	}
}

// Округление «половина вверх» на точной дроби: 2.375 обязано дать 2.38, а не 2.37.
func TestAveragePlaceRoundsHalfUp(t *testing.T) {
	cases := []struct {
		places, matches int
		want            string
	}{
		{places: 1, matches: 1, want: "1.00"},
		{places: 7, matches: 3, want: "2.33"},
		{places: 19, matches: 8, want: "2.38"},
		{places: 5, matches: 2, want: "2.50"},
		{places: 401, matches: 200, want: "2.01"},
	}
	for _, c := range cases {
		if got := averagePlace(c.places, c.matches); got != c.want {
			t.Errorf("сумма мест %d на %d матчей дала %q, ждали %q",
				c.places, c.matches, got, c.want)
		}
	}
}

// ⭐ Серия считается ПО МЕСТАМ, а не по знаку дельты рейтинга: в матче на пятерых можно
// занять второе место и всё равно потерять очки.
func TestStreakGoesByPlacesNotByRatingDelta(t *testing.T) {
	// Свежие сверху: два первых места подряд, дальше второе.
	store := &fakeRatingStore{
		outcomes: []repository.PlayerMatchOutcome{{Place: 1, Finished: true}},
		history: []repository.RatingHistoryEntry{
			{Place: 1, RatingAfter: "1030.00"},
			{Place: 1, RatingAfter: "1015.00"},
			{Place: 2, RatingAfter: "1005.00"},
		},
	}

	stats, err := NewStatsService(store).Of(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Streak.Kind != "WIN" || stats.Streak.Length != 2 {
		t.Errorf("серия %+v, ждали WIN длиной 2", stats.Streak)
	}
}

func TestStreakOfLossesCountsFromFreshest(t *testing.T) {
	history := []repository.RatingHistoryEntry{
		{Place: 4, RatingAfter: "980.00"},
		{Place: 2, RatingAfter: "990.00"},
		{Place: 1, RatingAfter: "1000.00"},
	}
	streak := streakOf(history)
	if streak.Kind != "LOSS" || streak.Length != 2 {
		t.Errorf("серия %+v, ждали LOSS длиной 2 — считается сверху до первой победы", streak)
	}
}

// ⚠️ Границы рейтинга сравниваются как числа: лексикографически «980.00» оказалось бы
// выше «1012.50».
func TestRatingBoundsCompareAsNumbers(t *testing.T) {
	history := []repository.RatingHistoryEntry{
		{Place: 1, RatingAfter: "980.00"},
		{Place: 2, RatingAfter: "1012.50"},
		{Place: 3, RatingAfter: "1000.00"},
	}
	best, worst := ratingBounds(history)
	if best == nil || *best != "1012.50" {
		t.Errorf("высшая точка %v, ждали \"1012.50\"", best)
	}
	if worst == nil || *worst != "980.00" {
		t.Errorf("низшая точка %v, ждали \"980.00\"", worst)
	}
}

// ⚠️ Порядок степеней — объявления в правилах игры, от самой тяжёлой к обычной,
// а не по алфавиту и не по частоте.
func TestDegreesFollowDeclarationOrder(t *testing.T) {
	store := &fakeRatingStore{outcomes: []repository.PlayerMatchOutcome{
		{Place: 2, LossType: ratingLossType("FAIL"), Finished: true},
		{Place: 3, LossType: ratingLossType("ROYAL"), Finished: true},
		{Place: 4, LossType: ratingLossType("FAIL"), Finished: true},
		{Place: 5, LossType: ratingLossType("SUPER_MEGA_FAIL"), Finished: true},
	}}

	stats, err := NewStatsService(store).Of(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	want := []DegreeCount{
		{Degree: "ROYAL", Count: 1},
		{Degree: "SUPER_MEGA_FAIL", Count: 1},
		{Degree: "FAIL", Count: 2},
	}
	if len(stats.Degrees) != len(want) {
		t.Fatalf("степеней %d, ждали %d: пустые в список не попадают", len(stats.Degrees), len(want))
	}
	for i, expected := range want {
		if stats.Degrees[i] != expected {
			t.Errorf("степень №%d — %+v, ждали %+v", i, stats.Degrees[i], expected)
		}
	}
}

// Место игрока используется как место, а не как индекс: единица — победа.
func TestOnlyFirstPlaceCountsAsWin(t *testing.T) {
	store := &fakeRatingStore{outcomes: []repository.PlayerMatchOutcome{
		{Place: 2, Finished: true}, {Place: 1, Finished: true},
	}}
	stats, err := NewStatsService(store).Of(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Wins != 1 {
		t.Errorf("побед %d, ждали 1", stats.Wins)
	}
	if stats.Losses != 0 {
		t.Errorf("поражений %d, ждали 0: поражение — это степень проигрыша, а не место", stats.Losses)
	}
}
