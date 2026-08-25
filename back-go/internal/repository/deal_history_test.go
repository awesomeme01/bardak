package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// Запись сыгранной раздачи на настоящем Postgres.

func TestPlayedDealIsWrittenWithSeatResults(t *testing.T) {
	pool := testDB(t)
	_, history, matchID, _, ctx := matchPlayersFixture(t)
	deals := NewDealHistory(pool)
	hearts, level := "HEARTS", "7"

	err := deals.Record(ctx, PlayedDeal{
		ID: uuid.NewString(), MatchID: matchID, DealNo: 1, TrumpSuit: &hearts, LoserSeat: 1,
		LastAttackCards: []string{"8-hearts", "8-spades"}, FinishedAt: time.Now(),
		Seats: []DealSeatResult{
			{SeatNo: 0, Place: intPtr(1), HungCards: []string{}, NavesLevelAfter: nil},
			{SeatNo: 1, Place: intPtr(2), HungCards: []string{"7-clubs"}, NavesLevelAfter: &level,
				LevelChanges: []DealLevelChange{{Reason: "LOST_DEAL", Amount: 1}}},
		},
	})
	if err != nil {
		t.Fatalf("раздача не записалась: %v", err)
	}

	stored, err := history.DealsOf(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].LoserSeat != 1 || len(stored[0].LastAttackCards) != 2 {
		t.Fatalf("раздача прочиталась не той: %+v", stored)
	}
	if stored[0].TrumpSuit == nil || *stored[0].TrumpSuit != hearts {
		t.Fatalf("козырь записан как %v, ждали HEARTS", stored[0].TrumpSuit)
	}

	seats, err := history.DealSeatsOf(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	rows := seats[stored[0].ID]
	if len(rows) != 2 {
		t.Fatalf("итоги мест: %+v", rows)
	}
	for _, row := range rows {
		if row.SeatNo != 1 {
			continue
		}
		if len(row.HungCards) != 1 || row.HungCards[0] != "7-clubs" {
			t.Fatalf("навешенное не записалось: %+v", row.HungCards)
		}
		// ⭐ Сдвиг уровня пишется слагаемыми, а не суммой: «почему» из суммы уже
		// не восстановить, а экран истории показывает именно его.
		if len(row.LevelChanges) != 1 {
			t.Fatalf("сдвиги уровня не записались: %+v", row.LevelChanges)
		}
	}
}

// Повтор той же раздачи не двоит историю: движок мог доиграть её второй раз после
// восстановления из снимка.
func TestPlayedDealIsWrittenOnlyOnce(t *testing.T) {
	pool := testDB(t)
	_, history, matchID, _, ctx := matchPlayersFixture(t)
	deals := NewDealHistory(pool)
	deal := PlayedDeal{
		ID: uuid.NewString(), MatchID: matchID, DealNo: 1, LoserSeat: 0,
		FinishedAt: time.Now(),
		Seats:      []DealSeatResult{{SeatNo: 0, HungCards: []string{}}},
	}

	if err := deals.Record(ctx, deal); err != nil {
		t.Fatal(err)
	}
	deal.ID = uuid.NewString() // повтор приходит с новым идентификатором
	if err := deals.Record(ctx, deal); err != nil {
		t.Fatalf("повтор записи упал: %v", err)
	}

	stored, err := history.DealsOf(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("раздача записана дважды: %+v", stored)
	}
}

func intPtr(value int) *int { return &value }
