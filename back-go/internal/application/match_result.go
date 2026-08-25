package application

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// JokerNavesLevel — джокер навешен, игрок проиграл. Двух символов хватает: колонка
// объявлена varchar(2).
const JokerNavesLevel = "Jk"

// MatchResultStore — что нужно итогу матча от базы.
type MatchResultStore interface {
	AlreadyCounted(ctx context.Context, matchID string) (bool, error)
	OpenSeasonID(ctx context.Context) (string, bool, error)
	Finish(ctx context.Context, finished repository.FinishedMatch) error
}

// RatingReader — текущий рейтинг игрока.
type RatingReader interface {
	FindRating(ctx context.Context, userID string) (repository.UserRating, error)
}

// ⭐ Проверка сборкой: репозитории подходят сценарию итога.
var (
	_ MatchResultStore = repository.MatchResults{}
	_ RatingReader     = repository.Ratings{}
)

// RatingChange — что показать игроку после матча.
type RatingChange struct {
	UserID string
	SeatNo int
	Place  int
	// NavesLevel — ступень шкалы, пусто если навесов не было.
	NavesLevel *string
	// LossDegree — степень проигрыша, пусто у не проигравшего.
	LossDegree *string
	Before     string
	After      string
	Delta      string
}

// MatchResultService — запись итога матча и пересчёт рейтинга.
//
// ⚠️ Отменённый матч сюда не приходит вообще: он сохраняется для просмотра, но рейтинга
// не касается (§5.3). Иначе уход из проигранной партии стал бы способом не считаться.
type MatchResultService struct {
	results MatchResultStore
	ratings RatingReader
	now     func() time.Time
}

// NewMatchResultService собирает сценарий.
func NewMatchResultService(results MatchResultStore, ratings RatingReader,
	now func() time.Time) MatchResultService {
	if now == nil {
		now = time.Now
	}
	return MatchResultService{results: results, ratings: ratings, now: now}
}

// FinishMatch записывает места, уровни навесов, степени проигрыша и рейтинг.
//
// Возвращает изменения рейтинга по местам — их показывает экран итога. Пустой список
// означает, что матч уже был посчитан.
func (s MatchResultService) FinishMatch(ctx context.Context, matchID string, seats []SeatOwner,
	state game.MatchState, scale game.NavesScale) ([]RatingChange, error) {
	counted, err := s.results.AlreadyCounted(ctx, matchID)
	if err != nil {
		return nil, err
	}
	if counted {
		// ⭐ Повторный вызов не должен удваивать рейтинг: матч закрывается ровно один раз.
		return nil, nil
	}

	outcome, ok := state.LastResult()
	if !ok {
		return nil, errors.New("матч закончился без итога раздачи")
	}
	places := placesOf(outcome, len(seats))

	participants := make([]EloParticipant, 0, len(seats))
	before := make([]string, 0, len(seats))
	for index, seat := range seats {
		rating, err := s.ratingOf(ctx, seat.UserID)
		if err != nil {
			return nil, err
		}
		value, err := strconv.ParseFloat(rating.Rating, 64)
		if err != nil {
			return nil, fmt.Errorf("рейтинг игрока %s не разобран: %w", seat.UserID, err)
		}
		before = append(before, rating.Rating)
		participants = append(participants, EloParticipant{
			Rating: value, MatchesPlayed: rating.MatchesPlayed, Place: places[index]})
	}

	updated, err := Recalculate(participants)
	if err != nil {
		return nil, err
	}

	seasonID, hasSeason, err := s.results.OpenSeasonID(ctx)
	if err != nil {
		return nil, err
	}

	outcomes := make([]repository.SeatOutcome, 0, len(seats))
	changes := make([]RatingChange, 0, len(seats))
	for index, seat := range seats {
		result := outcome.MustForSeat(seat.SeatNo)
		after := formatRating(updated[index])
		level := encodeNavesLevel(scale, result.LevelAfter)
		degree := encodeLossDegree(result.LossDegree)

		outcomes = append(outcomes, repository.SeatOutcome{
			UserID: seat.UserID, SeatNo: seat.SeatNo, Place: places[index],
			NavesLevel: level, LossType: degree,
			RatingBefore: before[index], RatingAfter: after,
		})
		changes = append(changes, RatingChange{
			UserID: seat.UserID, SeatNo: seat.SeatNo, Place: places[index],
			NavesLevel: level, LossDegree: degree,
			Before: before[index], After: after,
			Delta: formatRating(updated[index] - participants[index].Rating),
		})
	}

	finished := repository.FinishedMatch{
		MatchID: matchID, Outcomes: outcomes, At: s.now(),
	}
	if hasSeason {
		finished.SeasonID = &seasonID
	}
	if loser, found := outcome.MainLoser(); found {
		for index, seat := range seats {
			if seat.SeatNo == loser.SeatNo {
				finished.LoserUserID = &seats[index].UserID
				break
			}
		}
	}

	if err := s.results.Finish(ctx, finished); err != nil {
		return nil, err
	}
	return changes, nil
}

// ratingOf — текущий рейтинг; у не игравшего его ещё нет, и это не ошибка.
//
// ⚠️ Строка user_rating заводится ЛЕНИВО, при первом засчитанном матче: у отменённого
// матча её быть не должно вовсе — по её отсутствию видно, что игрок не играл.
func (s MatchResultService) ratingOf(ctx context.Context, userID string) (repository.UserRating, error) {
	rating, err := s.ratings.FindRating(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.UserRating{UserID: userID, Rating: "1000.00", MatchesPlayed: 0}, nil
	}
	if err != nil {
		return repository.UserRating{}, err
	}
	return rating, nil
}

// placesOf — места по итогу матча.
//
// ⭐ Место определяет уровень навесов: чем ниже уровень, тем лучше сыграл — счёта в очках
// в игре нет (ADR-017). Проигравшие с джокером идут в конец и ранжируются между собой
// по степени: ROYAL тяжелее, чем FAIL (§0.3).
//
// ⚠️ Равные уровни делят соседние места по порядку мест за столом. Делёж мест Elo не
// предусматривает, а придумывать ради этого отдельное правило незачем: на итог влияет
// только то, кто выше кого.
func placesOf(outcome game.DealOutcome, seats int) []int {
	order := make([]int, 0, seats)
	for seat := 0; seat < seats; seat++ {
		order = append(order, seat)
	}

	sort.SliceStable(order, func(left, right int) bool {
		first, second := outcome.MustForSeat(order[left]), outcome.MustForSeat(order[right])
		if first.LevelAfter != second.LevelAfter {
			return first.LevelAfter < second.LevelAfter
		}
		// Тяжёлая степень — ХУДШЕЕ место, а не лучшее.
		if first.LossDegree != second.LossDegree {
			return second.LossDegree.IsHeavierThan(first.LossDegree)
		}
		return order[left] < order[right]
	})

	places := make([]int, seats)
	for index, seat := range order {
		places[seat] = index + 1
	}
	return places
}

// encodeNavesLevel — уровень навесов коротким кодом для базы и клиента.
//
// ⭐ Движок кодирует уровень индексом ступени, и внутри это правильно. Но в истории
// уровень должен читаться глазами и пережить смену шкалы: «4» осмысленно только вместе
// с той шкалой, при которой записано, а «10» — само по себе.
func encodeNavesLevel(scale game.NavesScale, level int) *string {
	if level == game.NoNaves {
		return nil
	}
	if scale.IsFinished(level) {
		joker := JokerNavesLevel
		return &joker
	}
	code := scale.Ranks[level].Code()
	return &code
}

func encodeLossDegree(degree game.LossDegree) *string {
	if degree == game.NoLossDegree {
		return nil
	}
	name := degree.String()
	return &name
}

// formatRating — два знака после запятой, как numeric(8,2) в базе.
//
// ⚠️ Округление «половина вверх ОТ НУЛЯ», а не банковское и не math.Round: так делает
// Java (`BigDecimal.valueOf(double).setScale(2, HALF_UP)`), и на ровно половинных
// значениях расхождение было бы в копейку — зато навсегда и в обе стороны от нуля.
//
// ⚠️ Через десятичную запись, а не через value*100: 2.675 в double чуть меньше 2.675,
// и умножение уводит такие значения вниз. Java идёт от короткой записи числа — здесь
// то же самое, точной дробью.
func formatRating(value float64) string {
	shortest := strconv.FormatFloat(value, 'f', -1, 64)
	exact, ok := new(big.Rat).SetString(shortest)
	if !ok {
		return strconv.FormatFloat(value, 'f', 2, 64)
	}

	scaled := new(big.Rat).Mul(exact, big.NewRat(100, 1))
	quotient, remainder := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	twice := new(big.Int).Abs(remainder)
	twice.Mul(twice, big.NewInt(2))
	if twice.Cmp(scaled.Denom()) >= 0 {
		if scaled.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}

	hundredths := quotient.Int64()
	sign := ""
	if hundredths < 0 {
		sign, hundredths = "-", -hundredths
	}
	return sign + strconv.FormatInt(hundredths/100, 10) + "." + twoDigits(int(hundredths%100))
}
