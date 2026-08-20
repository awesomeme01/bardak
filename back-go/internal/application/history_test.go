package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Сценарий истории без базы: здесь проверяются правила, а не SQL.
//
// ⭐ Главное — реплей. Чужая скрытая карта не должна всплыть задним числом: по логу
// стиль игры соперника читается не хуже, чем подглядыванием вживую.

const (
	historyMe        = "11111111-1111-1111-1111-111111111111"
	historyMate      = "22222222-2222-2222-2222-222222222222"
	historyStranger  = "33333333-3333-3333-3333-333333333333"
	historyMatchID   = "44444444-4444-4444-4444-444444444444"
	historyMissingID = "55555555-5555-5555-5555-555555555555"
)

func TestHistoryOfAStrangerIsRefused(t *testing.T) {
	service := NewHistoryService(newHistoryStoreStub(), historyFriendsStub{})

	_, err := service.Matches(context.Background(), historyStranger, historyMe, "")

	if !errors.Is(err, ErrHistoryForbidden) {
		t.Fatalf("посторонний обязан получить отказ, получили %v", err)
	}
}

// ⚠️ Итог в списке считается для ВЛАДЕЛЬЦА истории, а не для смотрящего: открыв историю
// друга, видишь его место в матче, а не своё. Так в Java.
func TestHistoryOfAFriendShowsHisOwnPlace(t *testing.T) {
	friends := historyFriendsStub{pairs: map[string]bool{historyMate + historyMe: true}}
	service := NewHistoryService(newHistoryStoreStub(), friends)

	summaries, err := service.Matches(context.Background(), historyMate, historyMe, "")
	if err != nil {
		t.Fatalf("друг обязан видеть историю: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("ждали один матч, получили %d", len(summaries))
	}
	if summaries[0].MyPlace == nil || *summaries[0].MyPlace != 1 {
		t.Errorf("место владельца истории %v, ждали 1", summaries[0].MyPlace)
	}
	if summaries[0].MyRatingDelta == nil || *summaries[0].MyRatingDelta != "12.50" {
		t.Errorf("дельта владельца %v, ждали 12.50", summaries[0].MyRatingDelta)
	}
	if !summaries[0].RatingCounted {
		t.Error("завершённый матч рейтинг считает")
	}
}

// ⚠️ Порядок игроков — по месту, непроставленные в конец, хотя из базы они приходят
// по местам за столом.
func TestHistoryPlayersAreSortedByPlace(t *testing.T) {
	store := newHistoryStoreStub()
	service := NewHistoryService(store, historyFriendsStub{})

	summaries, err := service.Matches(context.Background(), historyMe, historyMe, "")
	if err != nil {
		t.Fatalf("своя история: %v", err)
	}
	players := summaries[0].Players
	if len(players) != 3 {
		t.Fatalf("ждали трёх игроков, получили %d", len(players))
	}
	if players[0].UserID != historyMe || players[1].UserID != historyMate {
		t.Errorf("первым идёт победитель, порядок сбит: %v, %v", players[0].UserID, players[1].UserID)
	}
	if players[2].Place != nil {
		t.Error("игрок без места обязан оказаться в конце")
	}
}

// ⚠️ У несуществующего матча нет участников, поэтому посторонний получает отказ по
// видимости, а не 404: по разнице ответов не перебрать чужие идентификаторы матчей.
func TestHistoryUnknownMatchLooksForbiddenToAStranger(t *testing.T) {
	service := NewHistoryService(newHistoryStoreStub(), historyFriendsStub{})

	_, err := service.Details(context.Background(), historyStranger, historyMissingID)

	if !errors.Is(err, ErrHistoryForbidden) {
		t.Fatalf("ждали отказ по видимости, получили %v", err)
	}
}

func TestHistoryDetailsShowDealBreakdown(t *testing.T) {
	service := NewHistoryService(newHistoryStoreStub(), historyFriendsStub{})

	details, err := service.Details(context.Background(), historyMe, historyMatchID)
	if err != nil {
		t.Fatalf("детали своего матча: %v", err)
	}
	if len(details.Deals) != 1 {
		t.Fatalf("ждали одну раздачу, получили %d", len(details.Deals))
	}
	if len(details.Deals[0].Seats) != 2 {
		t.Errorf("ждали два места в раздаче, получили %d", len(details.Deals[0].Seats))
	}
	if details.Match.MyPlace == nil || *details.Match.MyPlace != 1 {
		t.Errorf("в деталях моё место %v, ждали 1", details.Match.MyPlace)
	}
}

// ⚠️ Реплей идущего матча — это чтение партии из другого окна.
func TestHistoryReplayOfARunningMatchIsRefused(t *testing.T) {
	store := newHistoryStoreStub()
	store.match.Status = "IN_PROGRESS"
	store.match.FinishedAt = nil
	service := NewHistoryService(store, historyFriendsStub{})

	_, err := service.Replay(context.Background(), historyMe, historyMatchID)

	if !errors.Is(err, ErrHistoryMatchNotFinished) {
		t.Fatalf("ждали отказ «матч ещё идёт», получили %v", err)
	}
}

// ⭐ Ради этого всё и затевалось: каждый видит свои приватные события и не видит чужих.
func TestHistoryReplayHidesTheOtherPlayersCard(t *testing.T) {
	service := NewHistoryService(newHistoryStoreStub(), historyFriendsStub{})
	ctx := context.Background()

	mine, err := service.Replay(ctx, historyMate, historyMatchID)
	if err != nil {
		t.Fatalf("реплей участника: %v", err)
	}
	if mine.MySeat != 1 {
		t.Errorf("место спрашивающего %d, ждали 1", mine.MySeat)
	}
	if !historyHasEventType(mine.Events, "FACE_DOWN_REVEALED") {
		t.Error("своё приватное событие игрок обязан видеть")
	}

	theirs, err := service.Replay(ctx, historyMe, historyMatchID)
	if err != nil {
		t.Fatalf("реплей соседа: %v", err)
	}
	if historyHasEventType(theirs.Events, "FACE_DOWN_REVEALED") {
		t.Fatal("чужая вскрытая карта утекла в реплей — это испорченная игра")
	}
	if len(theirs.Events) != 2 {
		t.Errorf("публичных событий %d, ждали два", len(theirs.Events))
	}
}

// ⭐ Не участник получает mySeat = -1 и только публичные события.
func TestHistoryReplayForAFriendOutsideTheMatch(t *testing.T) {
	friends := historyFriendsStub{pairs: map[string]bool{historyStranger + historyMe: true}}
	service := NewHistoryService(newHistoryStoreStub(), friends)

	replay, err := service.Replay(context.Background(), historyStranger, historyMatchID)
	if err != nil {
		t.Fatalf("друг участника обязан видеть матч: %v", err)
	}
	if replay.MySeat != -1 {
		t.Errorf("место постороннего %d, ждали -1", replay.MySeat)
	}
	for _, event := range replay.Events {
		if event.PrivateToSeat != nil {
			t.Fatalf("посторонний получил приватное событие %q", event.Type)
		}
	}
}

func TestHistoryMatchIsHiddenFromAStranger(t *testing.T) {
	service := NewHistoryService(newHistoryStoreStub(), historyFriendsStub{})

	if _, err := service.Replay(context.Background(), historyStranger, historyMatchID); !errors.Is(err, ErrHistoryForbidden) {
		t.Fatalf("посторонний обязан получить отказ, получили %v", err)
	}
}

func historyHasEventType(events []repository.HistoryEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

// historyFriendsStub — дружба по списку пар. Ключ — конкатенация в том порядке,
// в каком спрашивают: сценарий обязан спрашивать «я и он», а не наоборот.
type historyFriendsStub struct{ pairs map[string]bool }

func (s historyFriendsStub) IsFriend(_ context.Context, one, two string) (bool, error) {
	return s.pairs[one+two], nil
}

// historyStoreStub — история в памяти. Возвращает ровно то, что вернула бы база
// для матча из фикстуры MatchHistoryApiIT.
type historyStoreStub struct {
	match        repository.HistoryMatch
	participants []repository.HistoryParticipant
	players      []repository.HistoryPlayer
	deals        []repository.HistoryDeal
	dealSeats    []repository.HistoryDealSeat
	events       []repository.HistoryEvent
}

func newHistoryStoreStub() *historyStoreStub {
	firstPlace, secondPlace := 1, 2
	dealNo, privateToSeat := 1, 1
	return &historyStoreStub{
		match: repository.HistoryMatch{
			ID: historyMatchID, TableID: "66666666-6666-6666-6666-666666666666",
			Status: "FINISHED", PlayersCount: 3, DealsPlayed: 1,
		},
		participants: []repository.HistoryParticipant{
			{UserID: historyMe, SeatNo: 0},
			{UserID: historyMate, SeatNo: 1},
		},
		// Порядок как из базы — по местам за столом; третий без места проверяет,
		// что непроставленные уезжают в конец.
		players: []repository.HistoryPlayer{
			{MatchID: historyMatchID, UserID: historyMe, DisplayName: "Я", SeatNo: 0,
				Place: &firstPlace, RatingDelta: historyStubString("12.50")},
			{MatchID: historyMatchID, UserID: historyMate, DisplayName: "Друг", SeatNo: 1,
				Place: &secondPlace, RatingDelta: historyStubString("-12.50")},
			{MatchID: historyMatchID, UserID: historyStranger, DisplayName: "Ушедший", SeatNo: 2},
		},
		deals: []repository.HistoryDeal{
			{ID: "77777777-7777-7777-7777-777777777777", DealNo: 1,
				TrumpSuit: historyStubString("SPADES"), LoserSeat: 1,
				LastAttackCards: []string{"8-hearts"}},
		},
		dealSeats: []repository.HistoryDealSeat{
			{DealID: "77777777-7777-7777-7777-777777777777", SeatNo: 0, Place: &firstPlace,
				HungCards: []string{}, LevelChanges: []repository.HistoryLevelChange{}},
			{DealID: "77777777-7777-7777-7777-777777777777", SeatNo: 1, Place: &secondPlace,
				HungCards:    []string{"Joker-1"},
				LevelChanges: []repository.HistoryLevelChange{{Reason: "LOST_DEAL", Amount: 1}}},
		},
		events: []repository.HistoryEvent{
			{Seq: 1, DealNo: &dealNo, Type: "DEAL_STARTED", Payload: json.RawMessage(`{"dealNo":1}`)},
			{Seq: 2, DealNo: &dealNo, Type: "FACE_DOWN_REVEALED", ActorSeat: &privateToSeat,
				Payload: json.RawMessage(`{"card":"A-clubs"}`), PrivateToSeat: &privateToSeat},
			{Seq: 3, DealNo: &dealNo, Type: "DEAL_OVER", Payload: json.RawMessage(`{}`)},
		},
	}
}

func (s *historyStoreStub) MatchesOf(_ context.Context, userID, status string) ([]repository.HistoryMatch, error) {
	for _, participant := range s.participants {
		if participant.UserID != userID {
			continue
		}
		if status != "" && !strings.EqualFold(status, s.match.Status) {
			return []repository.HistoryMatch{}, nil
		}
		return []repository.HistoryMatch{s.match}, nil
	}
	return []repository.HistoryMatch{}, nil
}

func (s *historyStoreStub) FindMatch(_ context.Context, matchID string) (repository.HistoryMatch, error) {
	if matchID != s.match.ID {
		return repository.HistoryMatch{}, repository.ErrNotFound
	}
	return s.match, nil
}

func (s *historyStoreStub) ParticipantsOf(_ context.Context, matchID string) ([]repository.HistoryParticipant, error) {
	if matchID != s.match.ID {
		return nil, nil
	}
	return s.participants, nil
}

func (s *historyStoreStub) PlayersOf(_ context.Context, matchIDs []string) (map[string][]repository.HistoryPlayer, error) {
	byMatch := map[string][]repository.HistoryPlayer{}
	for _, id := range matchIDs {
		if id == s.match.ID {
			// Копия: сценарий сортирует список на месте, и общий срез между вызовами
			// прятал бы ошибку сортировки.
			byMatch[id] = append([]repository.HistoryPlayer{}, s.players...)
		}
	}
	return byMatch, nil
}

func (s *historyStoreStub) DealsOf(_ context.Context, matchID string) ([]repository.HistoryDeal, error) {
	if matchID != s.match.ID {
		return nil, nil
	}
	return s.deals, nil
}

func (s *historyStoreStub) DealSeatsOf(_ context.Context, matchID string) (map[string][]repository.HistoryDealSeat, error) {
	byDeal := map[string][]repository.HistoryDealSeat{}
	if matchID != s.match.ID {
		return byDeal, nil
	}
	for _, seat := range s.dealSeats {
		byDeal[seat.DealID] = append(byDeal[seat.DealID], seat)
	}
	return byDeal, nil
}

func (s *historyStoreStub) EventsOf(_ context.Context, matchID string) ([]repository.HistoryEvent, error) {
	if matchID != s.match.ID {
		return nil, nil
	}
	return s.events, nil
}

func historyStubString(value string) *string { return &value }
