package application

import (
	"context"
	"testing"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Итог матча: места по уровням навесов, коды ступеней и рейтинг.
//
// ⭐ База здесь не нужна: проверяется, КАК считается итог. Что он ложится в базу одной
// транзакцией — проверяется отдельно, против настоящего Postgres.

type fakeResultStore struct {
	counted  bool
	seasonID string
	finished *repository.FinishedMatch
}

func (f *fakeResultStore) AlreadyCounted(context.Context, string) (bool, error) {
	return f.counted, nil
}

func (f *fakeResultStore) OpenSeasonID(context.Context) (string, bool, error) {
	if f.seasonID == "" {
		return "", false, nil
	}
	return f.seasonID, true, nil
}

func (f *fakeResultStore) Finish(_ context.Context, finished repository.FinishedMatch) error {
	f.finished = &finished
	return nil
}

type fakeRatings map[string]repository.UserRating

func (f fakeRatings) FindRating(_ context.Context, userID string) (repository.UserRating, error) {
	if rating, ok := f[userID]; ok {
		return rating, nil
	}
	return repository.UserRating{}, repository.ErrNotFound
}

func seatsOf(userIDs ...string) []SeatOwner {
	seats := make([]SeatOwner, 0, len(userIDs))
	for seatNo, userID := range userIDs {
		seats = append(seats, SeatOwner{SeatNo: seatNo, UserID: userID, DisplayName: userID})
	}
	return seats
}

// stateEndingWith — состояние матча, законченного этим итогом раздачи.
func stateEndingWith(outcome game.DealOutcome) game.MatchState {
	return game.MatchState{Results: []game.DealOutcome{outcome}}
}

func TestMatchResultRanksSeatsByNavesLevel(t *testing.T) {
	store := &fakeResultStore{}
	service := NewMatchResultService(store, fakeRatings{}, func() time.Time { return time.Unix(0, 0) })
	scale := game.FullNavesScale()
	// Место 0 летит выше всех, место 2 закончило с джокером — значит, оно и последнее.
	outcome := game.NewDealOutcome([]game.PlayerOutcome{
		game.NewPlayerOutcome(0, 3, 4, game.NoLossDegree),
		game.NewPlayerOutcome(1, 0, 1, game.NoLossDegree),
		game.NewPlayerOutcome(2, 8, scale.JokerLevel(), game.LossSuperMegaFail),
	}, 2)

	changes, err := service.FinishMatch(context.Background(), "match-1",
		seatsOf("user-a", "user-b", "user-c"), stateEndingWith(outcome), scale)
	if err != nil {
		t.Fatalf("итог не посчитался: %v", err)
	}

	if changes[1].Place != 1 || changes[0].Place != 2 || changes[2].Place != 3 {
		t.Fatalf("места разошлись с уровнями навесов: %+v", changes)
	}
	// ⭐ Ступень пишется кодом шкалы, а не индексом: «4» осмысленно только вместе со
	// шкалой, при которой записано, а «10» — само по себе.
	if changes[0].NavesLevel == nil || *changes[0].NavesLevel != "10" {
		t.Fatalf("ступень записана не кодом: %v", changes[0].NavesLevel)
	}
	if changes[2].NavesLevel == nil || *changes[2].NavesLevel != JokerNavesLevel {
		t.Fatalf("джокер записан как %v, ждали Jk", changes[2].NavesLevel)
	}
	if changes[2].LossDegree == nil || *changes[2].LossDegree != "SUPER_MEGA_FAIL" {
		t.Fatalf("степень проигрыша записана неверно: %v", changes[2].LossDegree)
	}
	if store.finished.LoserUserID == nil || *store.finished.LoserUserID != "user-c" {
		t.Fatalf("главный проигравший записан неверно: %v", store.finished.LoserUserID)
	}
}

// ⚠️ При равном уровне джокера ранжирует ТЯЖЕСТЬ степени: ROYAL — худшее место.
// Перевернуть это сравнение легко, и тогда самый позорный проигрыш получал бы лучшее
// место и прирост рейтинга.
func TestMatchResultPutsTheHeavierLossLast(t *testing.T) {
	store := &fakeResultStore{}
	service := NewMatchResultService(store, fakeRatings{}, nil)
	scale := game.FullNavesScale()
	joker := scale.JokerLevel()
	outcome := game.NewDealOutcome([]game.PlayerOutcome{
		game.NewPlayerOutcome(0, 8, joker, game.LossRoyal),
		game.NewPlayerOutcome(1, 8, joker, game.LossSuperMegaFail),
		game.NewPlayerOutcome(2, 2, 3, game.NoLossDegree),
	}, 0)

	changes, err := service.FinishMatch(context.Background(), "match-1",
		seatsOf("user-a", "user-b", "user-c"), stateEndingWith(outcome), scale)
	if err != nil {
		t.Fatal(err)
	}

	if changes[2].Place != 1 || changes[1].Place != 2 || changes[0].Place != 3 {
		t.Fatalf("места при двух джокерах: %d/%d/%d, ждали 3, 2, 1",
			changes[0].Place, changes[1].Place, changes[2].Place)
	}
}

// ⭐ Матч закрывается ровно один раз: повторный вызов не считает рейтинг заново.
func TestMatchResultSkipsAnAlreadyCountedMatch(t *testing.T) {
	store := &fakeResultStore{counted: true}
	service := NewMatchResultService(store, fakeRatings{}, nil)

	changes, err := service.FinishMatch(context.Background(), "match-1", seatsOf("user-a", "user-b"),
		stateEndingWith(game.NewDealOutcome([]game.PlayerOutcome{
			game.NewPlayerOutcome(0, 0, 1, game.NoLossDegree),
			game.NewPlayerOutcome(1, 0, 2, game.NoLossDegree),
		}, 1)), game.FullNavesScale())

	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 || store.finished != nil {
		t.Fatalf("посчитанный матч посчитан второй раз: %+v", store.finished)
	}
}

// У не игравшего строки рейтинга ещё нет — это не ошибка, а первый матч.
func TestMatchResultStartsTheNewcomerFromTheBaseRating(t *testing.T) {
	store := &fakeResultStore{}
	service := NewMatchResultService(store, fakeRatings{}, nil)

	changes, err := service.FinishMatch(context.Background(), "match-1", seatsOf("user-a", "user-b"),
		stateEndingWith(game.NewDealOutcome([]game.PlayerOutcome{
			game.NewPlayerOutcome(0, 0, 1, game.NoLossDegree),
			game.NewPlayerOutcome(1, 0, 2, game.NoLossDegree),
		}, 1)), game.FullNavesScale())
	if err != nil {
		t.Fatal(err)
	}

	if changes[0].Before != "1000.00" {
		t.Fatalf("новичок начал с %q, ждали 1000.00", changes[0].Before)
	}
	// K новичка — 40, при равных рейтингах победитель получает +20.
	if changes[0].After != "1020.00" || changes[0].Delta != "20.00" {
		t.Fatalf("рейтинг новичка после победы: %q (%q)", changes[0].After, changes[0].Delta)
	}
	if changes[1].Delta != "-20.00" {
		t.Fatalf("дельта проигравшего %q, ждали -20.00", changes[1].Delta)
	}
}

// ⚠️ Округление «половина вверх от нуля», как в Java: банковское или math.Round дали бы
// копеечное расхождение в истории — навсегда и в обе стороны от нуля.
func TestRatingIsRoundedHalfUpAwayFromZero(t *testing.T) {
	cases := map[float64]string{
		1000:      "1000.00",
		1000.005:  "1000.01",
		-0.005:    "-0.01",
		2.675:     "2.68",
		1009.9949: "1009.99",
		-12.345:   "-12.35",
	}

	for value, want := range cases {
		if got := formatRating(value); got != want {
			t.Errorf("formatRating(%v) = %q, ждали %q", value, got, want)
		}
	}
}
