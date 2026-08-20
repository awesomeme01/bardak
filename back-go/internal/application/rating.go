package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// ErrNotSeasonAdmin — сезон закрывает не каждый.
//
// ⭐ Ролей в системе нет и заводить их ради одной кнопки незачем: игра для узкого круга,
// сезон закрывается вручную и редко (ADR-037). Право живёт в настройках рейтинга —
// поэтому проверка здесь, а не в авторизации.
var ErrNotSeasonAdmin = errors.New("сезон закрывает не каждый")

// InitialRating — стартовый рейтинг новичка.
//
// ⚠️ Строка «1000», а не «1000.00», и это не опечатка: в Java это BigDecimal.valueOf(1000)
// с масштабом 0, и Jackson печатает `1000`. Тот же игрок после первого матча получит
// `1000.00` — уже из колонки numeric(8,2). Выровнять масштаб «для красоты» значило бы
// разойтись с эталоном в ответе, который сверяется побайтно.
const InitialRating = "1000"

// RatingStore — что рейтингу нужно от базы.
//
// ⭐ Интерфейс, а не конкретный репозиторий: сценарий проверяется на подставных данных
// за миллисекунды, а против настоящего Postgres проверяется SQL — в тестах репозитория.
type RatingStore interface {
	FindRating(ctx context.Context, userID string) (repository.UserRating, error)
	HistoryOf(ctx context.Context, userID string) ([]repository.RatingHistoryEntry, error)
	Leaderboard(ctx context.Context) ([]repository.LeaderRow, error)
	AllSeasons(ctx context.Context) ([]repository.Season, error)
	CloseAndOpenSeason(ctx context.Context, id, name string, now time.Time) (repository.Season, error)
}

// RatingUserStore — рейтингу нужно только имя игрока и факт его существования.
type RatingUserStore interface {
	FindByID(ctx context.Context, id string) (repository.User, error)
}

// RatingView — рейтинг игрока с графиком.
type RatingView struct {
	UserID        string
	DisplayName   string
	Rating        string
	MatchesPlayed int
	History       []repository.RatingHistoryEntry
}

// SeasonsView — сезоны и право спрашивающего ими управлять.
//
// ⭐ CanManage едет вместе со списком, а не отдельной ручкой и не флагом в профиле: без
// этого признака экран показывал бы кнопку «закрыть сезон» всем, а отказ прилетал бы уже
// после нажатия.
type SeasonsView struct {
	Seasons   []repository.Season
	CanManage bool
}

// RatingService — чтение рейтинга и управление сезонами.
//
// Пересчёт рейтинга по итогам матча живёт не здесь: сюда приходят только за готовым.
type RatingService struct {
	ratings      RatingStore
	users        RatingUserStore
	isSeasonAdmin func(string) bool
	now          func() time.Time
	newID        func() string
}

// NewRatingService собирает сценарии рейтинга.
//
// isSeasonAdmin приходит из настроек (config.IsSeasonAdmin): сценарий не должен знать,
// откуда взялся список ведущих сезона.
func NewRatingService(ratings RatingStore, users RatingUserStore,
	isSeasonAdmin func(string) bool, now func() time.Time) RatingService {
	if now == nil {
		now = time.Now
	}
	if isSeasonAdmin == nil {
		// Настроек нет — значит закрывать сезон некому. Отказ безопаснее разрешения.
		isSeasonAdmin = func(string) bool { return false }
	}
	return RatingService{
		ratings: ratings, users: users, isSeasonAdmin: isSeasonAdmin,
		now: now, newID: uuid.NewString,
	}
}

// Of — рейтинг игрока с графиком.
//
// ⚠️ Не игравший ни разу рейтинга в базе НЕ имеет, и это не ошибка: ему отдаётся стартовое
// значение и пустая история. Строка появится после первого матча. А вот несуществующий
// игрок — уже 404: разница между «ещё не играл» и «такого нет» видна только здесь.
func (s RatingService) Of(ctx context.Context, userID string) (RatingView, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return RatingView{}, ErrUserNotFound
		}
		return RatingView{}, err
	}

	view := RatingView{
		UserID:        userID,
		DisplayName:   user.DisplayName,
		Rating:        InitialRating,
		MatchesPlayed: 0,
	}

	rating, err := s.ratings.FindRating(ctx, userID)
	switch {
	case err == nil:
		view.Rating = rating.Rating
		view.MatchesPlayed = rating.MatchesPlayed
	case errors.Is(err, repository.ErrNotFound):
		// Так и задумано: стартовое значение уже проставлено выше.
	default:
		return RatingView{}, err
	}

	history, err := s.ratings.HistoryOf(ctx, userID)
	if err != nil {
		return RatingView{}, err
	}
	view.History = history
	return view, nil
}

// Leaderboard — таблица лидеров целиком.
func (s RatingService) Leaderboard(ctx context.Context) ([]repository.LeaderRow, error) {
	return s.ratings.Leaderboard(ctx)
}

// Seasons — список сезонов и право спрашивающего закрыть текущий.
//
// ⚠️ Право считается по ЛОГИНУ из токена и точным сравнением строк — в отличие от входа,
// где регистр не важен. Так же и в Java; «улучшить» это здесь значило бы дать доступ тому,
// кому в Java отказано.
func (s RatingService) Seasons(ctx context.Context, username string) (SeasonsView, error) {
	seasons, err := s.ratings.AllSeasons(ctx)
	if err != nil {
		return SeasonsView{}, err
	}
	return SeasonsView{Seasons: seasons, CanManage: s.isSeasonAdmin(username)}, nil
}

// CloseAndOpenSeason закрывает текущий сезон и открывает следующий.
func (s RatingService) CloseAndOpenSeason(ctx context.Context, username, name string) (repository.Season, error) {
	if !s.isSeasonAdmin(username) {
		return repository.Season{}, ErrNotSeasonAdmin
	}
	season, err := s.ratings.CloseAndOpenSeason(ctx, s.newID(), name, s.now())
	if err != nil {
		return repository.Season{}, fmt.Errorf("смена сезона: %w", err)
	}
	return season, nil
}

// StatsStore — что статистике нужно от базы.
type StatsStore interface {
	OutcomesOf(ctx context.Context, userID string) ([]repository.PlayerMatchOutcome, error)
	HistoryOf(ctx context.Context, userID string) ([]repository.RatingHistoryEntry, error)
}

// Streak — текущая серия: подряд выигранных или подряд проигранных матчей.
type Streak struct {
	// Kind — WIN, LOSS или NONE: до первого матча серии нет вовсе.
	Kind   string
	Length int
}

// DegreeCount — сколько раз игрок доигрывался до этой степени проигрыша.
type DegreeCount struct {
	Degree string
	Count  int
}

// PlayerStats — статистика игрока.
//
// ⚠️ Указатели у AvgPlace, BestRating и WorstRating не для красоты: у не игравшего их
// НЕТ, и в ответе этих ключей быть не должно. Degrees при этом остаётся пустым списком,
// а не отсутствует (MD-003).
type PlayerStats struct {
	Matches     int
	Wins        int
	Losses      int
	AvgPlace    *string
	DealsPlayed int
	Streak      Streak
	BestRating  *string
	WorstRating *string
	Degrees     []DegreeCount
}

// EmptyPlayerStats — статистика того, кто ещё не сыграл ни одного матча.
func EmptyPlayerStats() PlayerStats {
	return PlayerStats{
		Streak:  Streak{Kind: "NONE"},
		Degrees: []DegreeCount{},
	}
}

// StatsService — статистика игрока.
//
// ⭐ Считается по уже записанной истории матчей, а не копится отдельными счётчиками.
// Счётчик — это второе место, где живёт правда: он неизбежно разъезжается с историей,
// и потом не понять, какое из двух чисел настоящее.
//
// ⚠️ Считается на лету по ВСЕМ матчам игрока, без кэша и без пагинации. Для узкого круга
// это десятки строк; когда станет тысячами — сюда придёт витрина, а не досчитывание
// в живых запросах.
type StatsService struct{ stats StatsStore }

// NewStatsService собирает сценарий статистики.
func NewStatsService(stats StatsStore) StatsService { return StatsService{stats: stats} }

// Of — статистика игрока.
//
// ⚠️ «Нет такого игрока» здесь НЕ ошибка: неизвестный идентификатор даёт пустую
// статистику, а не 404. Так же и в Java — проверки существования тут нет вовсе.
func (s StatsService) Of(ctx context.Context, userID string) (PlayerStats, error) {
	outcomes, err := s.stats.OutcomesOf(ctx, userID)
	if err != nil {
		return PlayerStats{}, err
	}
	if len(outcomes) == 0 {
		return EmptyPlayerStats(), nil
	}

	stats := PlayerStats{Matches: len(outcomes)}
	places := 0
	degrees := map[string]int{}
	for _, outcome := range outcomes {
		places += outcome.Place
		if outcome.Place == 1 {
			stats.Wins++
		}
		if outcome.LossType != nil {
			stats.Losses++
			degrees[*outcome.LossType]++
		}
		// Раздачи считаются только у доигранных матчей: у отменённого их число ни о чём
		// не говорит — партия оборвалась.
		if outcome.Finished {
			stats.DealsPlayed += outcome.DealsPlayed
		}
	}
	average := averagePlace(places, len(outcomes))
	stats.AvgPlace = &average
	stats.Degrees = degreeList(degrees)

	history, err := s.stats.HistoryOf(ctx, userID)
	if err != nil {
		return PlayerStats{}, err
	}
	stats.Streak = streakOf(history)
	stats.BestRating, stats.WorstRating = ratingBounds(history)
	return stats, nil
}

// averagePlace — среднее место с двумя знаками, округление «половина вверх».
//
// ⭐ Среднее место честнее, чем «побед», когда за столом пятеро: победа одна, а разница
// между вторым и пятым — вся игра.
//
// ⚠️ Считается в целых числах, а не через float64. Java берёт BigDecimal.valueOf(double)
// и setScale(2, HALF_UP); наивное math.Round(x*100)/100 на границе вида 2.375 уводит вниз,
// потому что двоичное представление чуть меньше. Точная дробь sum/matches таких сюрпризов
// не знает: (2*sum*100 + matches) / (2*matches) — это и есть HALF_UP.
func averagePlace(places, matches int) string {
	hundredths := (2*places*100 + matches) / (2 * matches)
	return strconv.Itoa(hundredths/100) + "." + twoDigits(hundredths%100)
}

func twoDigits(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

// streakOf — текущая серия.
//
// ⭐ Серия считается ПО МЕСТАМ, а не по знаку дельты рейтинга: в матче на пятерых можно
// занять второе место и всё равно потерять очки. Побеждает тот, кто первый, — это и есть
// исход, который помнят за столом.
//
// ⚠️ Источник — история рейтинга, а не список матчей: серия идёт по матчам, попавшим
// в рейтинг. Порядок в истории — свежее сверху, на него всё и опирается.
func streakOf(history []repository.RatingHistoryEntry) Streak {
	if len(history) == 0 {
		return Streak{Kind: "NONE"}
	}
	won := history[0].Place == 1
	length := 0
	for _, entry := range history {
		if (entry.Place == 1) != won {
			break
		}
		length++
	}
	if won {
		return Streak{Kind: "WIN", Length: length}
	}
	return Streak{Kind: "LOSS", Length: length}
}

// ratingBounds — высшая и низшая точки рейтинга по истории.
//
// ⚠️ Значения сравниваются как ЧИСЛА, а хранятся и отдаются как строки: масштаб из базы
// («1012.50») обязан дожить до ответа, а лексикографическое сравнение поставило бы
// «9.00» выше «1012.50».
func ratingBounds(history []repository.RatingHistoryEntry) (*string, *string) {
	if len(history) == 0 {
		return nil, nil
	}
	best, worst := history[0].RatingAfter, history[0].RatingAfter
	bestValue, worstValue := decimalValue(best), decimalValue(worst)
	for _, entry := range history[1:] {
		value := decimalValue(entry.RatingAfter)
		if value > bestValue {
			best, bestValue = entry.RatingAfter, value
		}
		if value < worstValue {
			worst, worstValue = entry.RatingAfter, value
		}
	}
	return &best, &worst
}

// decimalValue — разбор numeric(8,2) для сравнения. Восьми значащих цифр float64 хватает
// с большим запасом, обратно в ответ это число не попадает.
func decimalValue(raw string) float64 {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return value
}

// degreeList — степени проигрыша с ненулевым счётом.
//
// ⚠️ Порядок — ОБЪЯВЛЕНИЯ степеней в правилах игры, от самой тяжёлой к обычной, а не
// по алфавиту и не по частоте: он несёт смысл, экран рисует его сверху вниз.
func degreeList(counts map[string]int) []DegreeCount {
	known := []game.LossDegree{
		game.LossRoyal, game.LossSuperMegaSuck, game.LossSuperMegaFail,
		game.LossSuperFail, game.LossFail,
	}
	list := make([]DegreeCount, 0, len(known))
	for _, degree := range known {
		name := degree.String()
		if count := counts[name]; count > 0 {
			list = append(list, DegreeCount{Degree: name, Count: count})
		}
	}
	return list
}
