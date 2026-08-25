package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// DealHistoryStore — запись сыгранной раздачи.
type DealHistoryStore interface {
	Record(ctx context.Context, deal repository.PlayedDeal) error
}

// CardCodec — карта в код протокола.
//
// ⭐ Интерфейс здесь, реализация в транспорте: коды карт — часть контракта с клиентом,
// и сценарий не обязан их знать. Зато история пишется теми же кодами, что уезжают
// в сокет, — иначе реплей показывал бы не то, что видели игроки.
type CardCodec interface {
	Encode(card game.Card) string
	// SuitName — имя масти ровно как у перечисления Java: оно уезжает в базу и в историю.
	SuitName(suit game.Suit) string
}

// ⭐ Проверка сборкой: репозиторий подходит сценарию.
var _ DealHistoryStore = repository.DealHistory{}

// DealRecorder — записывает раздачи, закрытые последним ходом.
type DealRecorder struct {
	store DealHistoryStore
	cards CardCodec
	newID func() string
	now   func() time.Time
}

// NewDealRecorder собирает запись раздач.
func NewDealRecorder(store DealHistoryStore, cards CardCodec, newID func() string,
	now func() time.Time) DealRecorder {
	if newID == nil {
		newID = uuid.NewString
	}
	if now == nil {
		now = time.Now
	}
	return DealRecorder{store: store, cards: cards, newID: newID, now: now}
}

// RecordFinished записывает раздачи, появившиеся в итогах после этого хода.
//
// ⭐ Раздача заканчивается СРАЗУ вместе со следующей сдачей: движок собирает колоду
// заново, и сыгранной раздачи в состоянии больше нет. Поэтому история пишется здесь
// и сейчас, из итога, а не откладывается на конец матча.
func (r DealRecorder) RecordFinished(ctx context.Context, matchID string, state game.MatchState,
	scale game.NavesScale, playedBefore int) error {
	for index := playedBefore; index < len(state.Results); index++ {
		if err := r.record(ctx, matchID, index+1, state.Results[index], scale); err != nil {
			return err
		}
	}
	return nil
}

func (r DealRecorder) record(ctx context.Context, matchID string, dealNo int,
	outcome game.DealOutcome, scale game.NavesScale) error {
	seats := make([]repository.DealSeatResult, 0, len(outcome.Players))
	for _, player := range outcome.Players {
		place := player.Place
		seat := repository.DealSeatResult{
			SeatNo:           player.SeatNo,
			HungCards:        r.encodeAll(player.HungCards),
			NavesLevelBefore: encodeNavesLevel(scale, player.LevelBefore),
			NavesLevelAfter:  encodeNavesLevel(scale, player.LevelAfter),
			LevelChanges:     changesOf(player.Changes),
		}
		// ⚠️ Место пустое, когда итог собран не движком: «нуля» вместо «ничего» здесь
		// быть не должно — нулевое место значило бы победу.
		if place != game.Unplaced {
			seat.Place = &place
		}
		seats = append(seats, seat)
	}

	return r.store.Record(ctx, repository.PlayedDeal{
		ID:              r.newID(),
		MatchID:         matchID,
		DealNo:          dealNo,
		TrumpSuit:       r.trumpNameOf(outcome),
		LoserSeat:       outcome.DealLoserSeat,
		LastAttackCards: r.encodeAll(outcome.LastAttackCards),
		FinishedAt:      r.now(),
		Seats:           seats,
	})
}

func (r DealRecorder) encodeAll(cards []game.Card) []string {
	codes := make([]string, 0, len(cards))
	for _, card := range cards {
		codes = append(codes, r.cards.Encode(card))
	}
	return codes
}

// trumpNameOf — имя козырной масти. Пусто, если раздача кончилась, не начавшись.
//
// ⚠️ Имя берётся у кодека протокола, а не собирается здесь: два списка имён мастей
// разошлись бы при первой же правке, а разошлись бы они молча — в истории.
func (r DealRecorder) trumpNameOf(outcome game.DealOutcome) *string {
	if outcome.TrumpSuit == nil {
		return nil
	}
	name := r.cards.SuitName(*outcome.TrumpSuit)
	return &name
}

func changesOf(changes []game.LevelChange) []repository.DealLevelChange {
	out := make([]repository.DealLevelChange, 0, len(changes))
	for _, change := range changes {
		out = append(out, repository.DealLevelChange{
			Reason: string(change.Reason), Amount: change.Amount})
	}
	return out
}
