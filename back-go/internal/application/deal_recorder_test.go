package application

import (
	"context"
	"testing"
	"time"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Запись сыгранных раздач.
//
// ⭐ Проверяется то, что иначе пришлось бы восстанавливать переигрыванием: ступени шкалы
// человеческими кодами, состав последней атаки и слагаемые сдвига уровня.

type fakeDealStore struct{ recorded []repository.PlayedDeal }

func (f *fakeDealStore) Record(_ context.Context, deal repository.PlayedDeal) error {
	f.recorded = append(f.recorded, deal)
	return nil
}

// cardCodec — кодек карт для теста; тот же формат, что уезжает клиенту.
type cardCodec struct{}

func (cardCodec) Encode(card game.Card) string { return card.Code() }

func (cardCodec) SuitName(suit game.Suit) string {
	return map[game.Suit]string{game.Diamonds: "DIAMONDS", game.Hearts: "HEARTS",
		game.Spades: "SPADES", game.Clubs: "CLUBS"}[suit]
}

func TestOnlyDealsFinishedByThisMoveAreRecorded(t *testing.T) {
	store := &fakeDealStore{}
	recorder := NewDealRecorder(store, cardCodec{}, func() string { return "deal-id" },
		func() time.Time { return time.Unix(0, 0) })
	scale := game.FullNavesScale()
	state := game.MatchState{Results: []game.DealOutcome{
		game.NewDealOutcome([]game.PlayerOutcome{game.NewPlayerOutcome(0, -1, 0, game.NoLossDegree)}, 0),
		game.NewDealOutcome([]game.PlayerOutcome{game.NewPlayerOutcome(0, 0, 1, game.NoLossDegree)}, 0),
	}}

	// Первая раздача записана ещё прошлым ходом: играется только то, что закрыл этот.
	if err := recorder.RecordFinished(context.Background(), "match-1", state, scale, 1); err != nil {
		t.Fatal(err)
	}

	if len(store.recorded) != 1 {
		t.Fatalf("записано раздач: %d, ждали одну", len(store.recorded))
	}
	if store.recorded[0].DealNo != 2 {
		t.Fatalf("номер раздачи %d, ждали 2", store.recorded[0].DealNo)
	}
}

// ⭐ Ступень пишется КОДОМ шкалы, а не индексом: «4» осмысленно только вместе со шкалой,
// при которой записано, а «10» — само по себе. И «навесов не было» — это отсутствие
// значения, а не нулевая ступень.
func TestDealRecordKeepsHumanReadableLevels(t *testing.T) {
	store := &fakeDealStore{}
	recorder := NewDealRecorder(store, cardCodec{}, nil, nil)
	scale := game.FullNavesScale()
	trump := game.Hearts
	outcome := game.NewDealOutcome([]game.PlayerOutcome{
		game.NewPlayerOutcome(0, game.NoNaves, 4, game.NoLossDegree),
		game.NewPlayerOutcome(1, 8, scale.JokerLevel(), game.LossSuperMegaFail),
	}, 1)
	outcome.TrumpSuit = &trump
	outcome.LastAttackCards = []game.Card{game.NewPip(game.Eight, game.Hearts)}

	err := recorder.RecordFinished(context.Background(), "match-1",
		game.MatchState{Results: []game.DealOutcome{outcome}}, scale, 0)
	if err != nil {
		t.Fatal(err)
	}

	deal := store.recorded[0]
	if deal.TrumpSuit == nil || *deal.TrumpSuit != "HEARTS" {
		t.Fatalf("козырь записан как %v", deal.TrumpSuit)
	}
	if len(deal.LastAttackCards) != 1 {
		t.Fatalf("состав последней атаки потерян: %v", deal.LastAttackCards)
	}
	if deal.Seats[0].NavesLevelBefore != nil {
		t.Fatalf("«навесов не было» записалось как ступень: %v", deal.Seats[0].NavesLevelBefore)
	}
	if deal.Seats[0].NavesLevelAfter == nil || *deal.Seats[0].NavesLevelAfter != "10" {
		t.Fatalf("ступень записана не кодом: %v", deal.Seats[0].NavesLevelAfter)
	}
	if deal.Seats[1].NavesLevelAfter == nil || *deal.Seats[1].NavesLevelAfter != JokerNavesLevel {
		t.Fatalf("джокер записан как %v", deal.Seats[1].NavesLevelAfter)
	}
}
