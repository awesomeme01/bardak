package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// Итог матча и рейтинг — одной транзакцией, на настоящем Postgres.
//
// ⭐ Смысл теста: история и текущий рейтинг обязаны сойтись. Разъедься они — чинить
// потом можно только руками и на глаз.

func TestMatchResultWritesPlacesRatingAndHistoryTogether(t *testing.T) {
	pool := testDB(t)
	players, history, matchID, users, ctx := matchPlayersFixture(t)
	results := NewMatchResults(pool)
	if err := players.Seat(ctx, matchID, users); err != nil {
		t.Fatal(err)
	}
	joker, royal := "Jk", "ROYAL"

	err := results.Finish(ctx, FinishedMatch{
		MatchID:     matchID,
		LoserUserID: &users[1],
		Outcomes: []SeatOutcome{
			{UserID: users[0], SeatNo: 0, Place: 1, RatingBefore: "1000.00", RatingAfter: "1010.00"},
			{UserID: users[1], SeatNo: 1, Place: 2, NavesLevel: &joker, LossType: &royal,
				RatingBefore: "1000.00", RatingAfter: "990.00"},
		},
		At: time.Now(),
	})
	if err != nil {
		t.Fatalf("итог не записался: %v", err)
	}

	match, err := NewMatchLog(pool).MatchByID(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if match.Status != MatchFinished || match.LoserUserID == nil || *match.LoserUserID != users[1] {
		t.Fatalf("матч закрыт неверно: %+v", match)
	}

	rows, err := history.PlayersOf(ctx, []string{matchID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows[matchID]) != 2 {
		t.Fatalf("итоги завели лишние строки: %+v", rows[matchID])
	}
	for _, row := range rows[matchID] {
		if row.Place == nil {
			t.Fatalf("итог без места: %+v", row)
		}
		if *row.Place == 2 {
			if row.LossType == nil || *row.LossType != royal {
				t.Fatalf("степень проигрыша не записана: %+v", row)
			}
			// ⚠️ Дельту считает база из тех же чисел, что записаны: посчитанная отдельно
			// в коде, она однажды разошлась бы с историей на копейку.
			if row.RatingDelta == nil || *row.RatingDelta != "-10.00" {
				t.Fatalf("дельта рейтинга разошлась: %v", row.RatingDelta)
			}
		}
	}

	ratings := NewRatings(pool)
	winner, err := ratings.FindRating(ctx, users[0])
	if err != nil {
		t.Fatalf("рейтинг победителя не завёлся: %v", err)
	}
	if winner.Rating != "1010.00" || winner.MatchesPlayed != 1 {
		t.Fatalf("рейтинг победителя %+v, ждали 1010.00 и один матч", winner)
	}
	entries, err := ratings.HistoryOf(ctx, users[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].RatingAfter != "990.00" || entries[0].Place != 2 {
		t.Fatalf("история рейтинга проигравшего: %+v", entries)
	}
}

// ⭐ Матч закрывается ровно один раз: повтор не должен удваивать ни рейтинг, ни счётчик
// матчей. Реконнект и повторная доставка события — обычное дело, а раздутый рейтинг
// потом чинится только руками.
func TestMatchResultIsCountedOnlyOnce(t *testing.T) {
	pool := testDB(t)
	players, _, matchID, users, ctx := matchPlayersFixture(t)
	results := NewMatchResults(pool)
	if err := players.Seat(ctx, matchID, users); err != nil {
		t.Fatal(err)
	}
	finished := FinishedMatch{
		MatchID: matchID,
		Outcomes: []SeatOutcome{
			{UserID: users[0], SeatNo: 0, Place: 1, RatingBefore: "1000.00", RatingAfter: "1010.00"},
			{UserID: users[1], SeatNo: 1, Place: 2, RatingBefore: "1000.00", RatingAfter: "990.00"},
		},
		At: time.Now(),
	}
	if err := results.Finish(ctx, finished); err != nil {
		t.Fatal(err)
	}

	counted, err := results.AlreadyCounted(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if !counted {
		t.Fatalf("посчитанный матч не считается посчитанным: защита от повтора не сработает")
	}

	// Повтор той же записи не должен добавить матч в счётчик.
	if err := results.Finish(ctx, finished); err != nil {
		t.Fatal(err)
	}
	ratings := NewRatings(pool)
	winner, err := ratings.FindRating(ctx, users[0])
	if err != nil {
		t.Fatal(err)
	}
	if winner.Rating != "1010.00" || winner.MatchesPlayed != 1 {
		t.Fatalf("повтор удвоил рейтинг или счётчик матчей: %+v", winner)
	}
	entries, err := ratings.HistoryOf(ctx, users[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("история рейтинга удвоилась: %+v", entries)
	}
}

// Матч без сезона пишется тоже: сезонов может не быть вовсе.
func TestMatchResultLandsInTheOpenSeasonWhenThereIsOne(t *testing.T) {
	pool := testDB(t)
	players, _, matchID, users, ctx := matchPlayersFixture(t)
	results := NewMatchResults(pool)
	if err := players.Seat(ctx, matchID, users); err != nil {
		t.Fatal(err)
	}

	seasonID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`insert into seasons (id, name, started_at) values ($1, 'Сезон теста', now())
		 on conflict do nothing`, seasonID); err != nil {
		t.Skipf("открытый сезон уже есть, проверять нечего: %v", err)
	}

	open, ok, err := results.OpenSeasonID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("открытый сезон не найден")
	}

	err = results.Finish(ctx, FinishedMatch{
		MatchID:  matchID,
		SeasonID: &open,
		Outcomes: []SeatOutcome{
			{UserID: users[0], SeatNo: 0, Place: 1, RatingBefore: "1000.00", RatingAfter: "1010.00"},
			{UserID: users[1], SeatNo: 1, Place: 2, RatingBefore: "1000.00", RatingAfter: "990.00"},
		},
		At: time.Now(),
	})
	if err != nil {
		t.Fatalf("итог с сезоном не записался: %v", err)
	}

	var stored *string
	if err := pool.QueryRow(ctx,
		`select season_id::text from rating_history where match_id = $1 limit 1`,
		matchID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == nil || *stored != open {
		t.Fatalf("сезон в истории рейтинга: %v, ждали %s", stored, open)
	}
}
