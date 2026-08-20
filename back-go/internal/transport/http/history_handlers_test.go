package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// История матчей через HTTP: форма тела, коды состояния и — главное — что чужие карты
// не утекают в реплей.

const (
	histMe       = "11111111-1111-1111-1111-111111111111"
	histMate     = "22222222-2222-2222-2222-222222222222"
	histStranger = "33333333-3333-3333-3333-333333333333"
	histMatch    = "44444444-4444-4444-4444-444444444444"
)

// ⚠️ MD-003: Java вырезает только null. finishedAt на месте, abortReason и place
// проигравшего без итога вырезаны, а ratingCounted и dealsPlayed остаются — даже нулями.
func TestHistoryListBodyMatchesJava(t *testing.T) {
	response := histServe(t, histNewStore(), histMe, http.MethodGet, "/api/matches")

	if response.Code != http.StatusOK {
		t.Fatalf("код %d, ждали 200: %s", response.Code, response.Body)
	}
	want := `[{"id":"44444444-4444-4444-4444-444444444444",` +
		`"tableId":"66666666-6666-6666-6666-666666666666","status":"FINISHED",` +
		`"startedAt":"2026-08-19T10:15:30Z","finishedAt":"2026-08-19T10:45:00Z",` +
		`"playersCount":2,"dealsPlayed":1,"ratingCounted":true,"myPlace":1,` +
		`"myRatingDelta":12.50,"players":[` +
		`{"userId":"11111111-1111-1111-1111-111111111111","displayName":"Я","seatNo":0,` +
		`"place":1,"navesLevel":"6","ratingBefore":1000.00,"ratingAfter":1012.50,` +
		`"ratingDelta":12.50},` +
		`{"userId":"22222222-2222-2222-2222-222222222222","displayName":"Друг","seatNo":1,` +
		`"place":2,"navesLevel":"8","lossType":"SUPER_MEGA_FAIL","ratingBefore":1000.00,` +
		`"ratingAfter":987.50,"ratingDelta":-12.50}]}]`
	if got := strings.TrimSpace(response.Body.String()); got != want {
		t.Errorf("тело разошлось\nполучили: %s\nждали:    %s", got, want)
	}
}

// ⚠️ Пустая история — это [], а не null: nil-срез ушёл бы в ответ как null, чего Java
// не отдаёт никогда.
func TestHistoryOfANewcomerIsEmptyList(t *testing.T) {
	store := histNewStore()
	store.matches = []repository.HistoryMatch{}

	response := histServe(t, store, histMe, http.MethodGet, "/api/matches")

	if got := strings.TrimSpace(response.Body.String()); got != "[]" {
		t.Errorf("получили %s, ждали []", got)
	}
}

// ⭐ История видна своим и друзьям. Рейтинг и статистика при этом открыты — они агрегаты,
// а «с кем и когда я играл» посторонним знать незачем.
func TestHistoryOfAStrangerIsForbidden(t *testing.T) {
	response := histServe(t, histNewStore(), histStranger, http.MethodGet,
		"/api/matches?userId="+histMe)

	if response.Code != http.StatusForbidden {
		t.Fatalf("код %d, ждали 403: %s", response.Code, response.Body)
	}
	code, message := histErrorOf(t, response)
	if code != "NOT_FRIENDS" {
		t.Errorf("код ошибки %q, ждали NOT_FRIENDS", code)
	}
	if message != "История матчей видна своим и друзьям" {
		t.Errorf("сообщение %q разошлось с Java", message)
	}
}

// ⚠️ Тот же код NOT_FRIENDS, но повод другой — и текст в Java другой.
func TestHistoryMatchOfAStrangerIsForbidden(t *testing.T) {
	response := histServe(t, histNewStore(), histStranger, http.MethodGet, "/api/matches/"+histMatch)

	if response.Code != http.StatusForbidden {
		t.Fatalf("код %d, ждали 403: %s", response.Code, response.Body)
	}
	_, message := histErrorOf(t, response)
	if message != "Этот матч видят его игроки и их друзья" {
		t.Errorf("сообщение %q разошлось с Java", message)
	}
}

func TestHistoryDetailsCarryDeals(t *testing.T) {
	response := histServe(t, histNewStore(), histMe, http.MethodGet, "/api/matches/"+histMatch)

	if response.Code != http.StatusOK {
		t.Fatalf("код %d, ждали 200: %s", response.Code, response.Body)
	}
	var body struct {
		Match json.RawMessage `json:"match"`
		Deals []struct {
			DealNo          int      `json:"dealNo"`
			TrumpSuit       string   `json:"trumpSuit"`
			LoserSeat       int      `json:"loserSeat"`
			LastAttackCards []string `json:"lastAttackCards"`
			Seats           []struct {
				SeatNo       int `json:"seatNo"`
				HungCards    []string
				LevelChanges []struct {
					Reason string `json:"reason"`
					Amount int    `json:"amount"`
				} `json:"levelChanges"`
			} `json:"seats"`
		} `json:"deals"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("разбор тела: %v", err)
	}
	if len(body.Deals) != 1 || body.Deals[0].TrumpSuit != "SPADES" {
		t.Fatalf("раздачи разошлись: %+v", body.Deals)
	}
	if len(body.Deals[0].Seats) != 2 {
		t.Fatalf("ждали два места, получили %d", len(body.Deals[0].Seats))
	}
	if got := body.Deals[0].Seats[1].LevelChanges; len(got) != 1 || got[0].Reason != "LOST_DEAL" {
		t.Errorf("изменения уровня %+v, ждали одно с причиной LOST_DEAL", got)
	}
}

// ⚠️ Реплей идущего матча — это чтение партии из другого окна.
func TestHistoryReplayOfARunningMatchIsConflict(t *testing.T) {
	store := histNewStore()
	store.matches[0].Status = "IN_PROGRESS"

	response := histServe(t, store, histMe, http.MethodGet, "/api/matches/"+histMatch+"/replay")

	if response.Code != http.StatusConflict {
		t.Fatalf("код %d, ждали 409: %s", response.Code, response.Body)
	}
	if code, _ := histErrorOf(t, response); code != "MATCH_NOT_FINISHED" {
		t.Errorf("код ошибки %q, ждали MATCH_NOT_FINISHED", code)
	}
}

// ⭐ Главная проверка: реплей показывает матч глазами спрашивающего. Чужая вскрытая
// карта не должна всплыть задним числом — иначе история сдаёт то, что правила прятали
// весь матч.
func TestHistoryReplayNeverLeaksTheOtherHand(t *testing.T) {
	store := histNewStore()

	mine := histServe(t, store, histMate, http.MethodGet, "/api/matches/"+histMatch+"/replay")
	theirs := histServe(t, store, histMe, http.MethodGet, "/api/matches/"+histMatch+"/replay")

	if !strings.Contains(mine.Body.String(), "FACE_DOWN_REVEALED") {
		t.Error("своё приватное событие игрок обязан видеть")
	}
	if strings.Contains(theirs.Body.String(), "FACE_DOWN_REVEALED") {
		t.Fatalf("чужая карта утекла в реплей: %s", theirs.Body)
	}
	if strings.Contains(theirs.Body.String(), "A-clubs") {
		t.Fatalf("payload с чужой картой утёк в реплей: %s", theirs.Body)
	}
}

// ⭐ Друг участника матч видит, но играет он в нём или нет — разница видна: mySeat = -1,
// и приватных событий у такого нет ни одного.
func TestHistoryReplayForAnOutsiderHasSeatMinusOne(t *testing.T) {
	store := histNewStore()
	store.friends[histStranger+histMe] = true

	response := histServe(t, store, histStranger, http.MethodGet,
		"/api/matches/"+histMatch+"/replay")

	if response.Code != http.StatusOK {
		t.Fatalf("код %d, ждали 200: %s", response.Code, response.Body)
	}
	var body struct {
		MatchID string `json:"matchId"`
		Status  string `json:"status"`
		MySeat  int    `json:"mySeat"`
		Events  []struct {
			Seq     int             `json:"seq"`
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("разбор тела: %v", err)
	}
	if body.MySeat != -1 {
		t.Errorf("mySeat = %d, ждали -1", body.MySeat)
	}
	if len(body.Events) != 2 {
		t.Errorf("публичных событий %d, ждали два", len(body.Events))
	}
	if string(body.Events[0].Payload) != `{"dealNo":1}` {
		t.Errorf("payload отдаётся как есть, а пришло %s", body.Events[0].Payload)
	}
}

// ⚠️ Невалидный UUID — это 400, а не 500 и не 404: до контроллера такой запрос
// не доходит, его ловит handleBadRequest в Java. Клиент обязан отличать свою ошибку
// от поломки сервера.
func TestHistoryBrokenIdentifierIsBadRequest(t *testing.T) {
	store := histNewStore()

	for _, path := range []string{"/api/matches?userId=не-uuid", "/api/matches/не-uuid"} {
		response := histServe(t, store, histMe, http.MethodGet, path)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s дал %d, ждали 400 — как в Java", path, response.Code)
		}
		if code, _ := histErrorOf(t, response); code != "BAD_REQUEST" {
			t.Errorf("%s дал код %q, ждали BAD_REQUEST", path, code)
		}
	}
}

// histServe прогоняет запрос через настоящий маршрутизатор с подставленным владельцем.
func histServe(t *testing.T, store *histStore, me, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	HistoryHandlers{History: application.NewHistoryService(store, store)}.Routes(router)

	request := httptest.NewRequest(method, target, nil)
	// Токен проверяет middleware; здесь важен только его результат.
	request = request.WithContext(context.WithValue(request.Context(), principalKey{},
		Principal{UserID: me, Username: "player"}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func histErrorOf(t *testing.T, response *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var body APIError
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("разбор ошибки: %v", err)
	}
	return body.Code, body.Message
}

// histStore — история в памяти: и хранилище, и проверка дружбы разом.
type histStore struct {
	matches      []repository.HistoryMatch
	participants []repository.HistoryParticipant
	players      []repository.HistoryPlayer
	deals        []repository.HistoryDeal
	dealSeats    []repository.HistoryDealSeat
	events       []repository.HistoryEvent
	friends      map[string]bool
}

func histNewStore() *histStore {
	started := time.Date(2026, 8, 19, 10, 15, 30, 0, time.UTC)
	finished := time.Date(2026, 8, 19, 10, 45, 0, 0, time.UTC)
	firstPlace, secondPlace := 1, 2
	dealNo, seatOne := 1, 1
	loss := "SUPER_MEGA_FAIL"

	return &histStore{
		matches: []repository.HistoryMatch{{
			ID: histMatch, TableID: "66666666-6666-6666-6666-666666666666",
			Status: "FINISHED", PlayersCount: 2, DealsPlayed: 1,
			StartedAt: started, FinishedAt: &finished,
		}},
		participants: []repository.HistoryParticipant{
			{UserID: histMe, SeatNo: 0}, {UserID: histMate, SeatNo: 1},
		},
		players: []repository.HistoryPlayer{
			{MatchID: histMatch, UserID: histMe, DisplayName: "Я", SeatNo: 0, Place: &firstPlace,
				NavesLevel: histText("6"), RatingBefore: histText("1000.00"),
				RatingAfter: histText("1012.50"), RatingDelta: histText("12.50")},
			{MatchID: histMatch, UserID: histMate, DisplayName: "Друг", SeatNo: 1,
				Place: &secondPlace, NavesLevel: histText("8"), LossType: &loss,
				RatingBefore: histText("1000.00"), RatingAfter: histText("987.50"),
				RatingDelta: histText("-12.50")},
		},
		deals: []repository.HistoryDeal{{
			ID: "77777777-7777-7777-7777-777777777777", DealNo: 1,
			TrumpSuit: histText("SPADES"), LoserSeat: 1, FinishedAt: &finished,
			LastAttackCards: []string{"8-hearts"},
		}},
		dealSeats: []repository.HistoryDealSeat{
			{DealID: "77777777-7777-7777-7777-777777777777", SeatNo: 0, Place: &firstPlace,
				HungCards: []string{}, LevelChanges: []repository.HistoryLevelChange{}},
			{DealID: "77777777-7777-7777-7777-777777777777", SeatNo: 1, Place: &secondPlace,
				HungCards:    []string{"Joker-1"},
				LevelChanges: []repository.HistoryLevelChange{{Reason: "LOST_DEAL", Amount: 1}}},
		},
		// Второе событие видит только место 1: вскрытая закрытая карта — чужая тайна.
		events: []repository.HistoryEvent{
			{Seq: 1, DealNo: &dealNo, Type: "DEAL_STARTED", Payload: json.RawMessage(`{"dealNo":1}`)},
			{Seq: 2, DealNo: &dealNo, Type: "FACE_DOWN_REVEALED", ActorSeat: &seatOne,
				Payload: json.RawMessage(`{"card":"A-clubs"}`), PrivateToSeat: &seatOne},
			{Seq: 3, DealNo: &dealNo, Type: "DEAL_OVER", Payload: json.RawMessage(`{}`)},
		},
		friends: map[string]bool{},
	}
}

func (s *histStore) IsFriend(_ context.Context, one, two string) (bool, error) {
	return s.friends[one+two], nil
}

func (s *histStore) MatchesOf(_ context.Context, userID, status string) ([]repository.HistoryMatch, error) {
	found := []repository.HistoryMatch{}
	for _, participant := range s.participants {
		if participant.UserID != userID {
			continue
		}
		for _, match := range s.matches {
			if status == "" || strings.EqualFold(status, match.Status) {
				found = append(found, match)
			}
		}
	}
	return found, nil
}

func (s *histStore) FindMatch(_ context.Context, matchID string) (repository.HistoryMatch, error) {
	for _, match := range s.matches {
		if match.ID == matchID {
			return match, nil
		}
	}
	return repository.HistoryMatch{}, repository.ErrNotFound
}

func (s *histStore) ParticipantsOf(_ context.Context, matchID string) ([]repository.HistoryParticipant, error) {
	if matchID != histMatch {
		return nil, nil
	}
	return s.participants, nil
}

func (s *histStore) PlayersOf(_ context.Context, matchIDs []string) (map[string][]repository.HistoryPlayer, error) {
	byMatch := map[string][]repository.HistoryPlayer{}
	for _, id := range matchIDs {
		if id == histMatch {
			byMatch[id] = append([]repository.HistoryPlayer{}, s.players...)
		}
	}
	return byMatch, nil
}

func (s *histStore) DealsOf(_ context.Context, matchID string) ([]repository.HistoryDeal, error) {
	if matchID != histMatch {
		return nil, nil
	}
	return s.deals, nil
}

func (s *histStore) DealSeatsOf(_ context.Context, matchID string) (map[string][]repository.HistoryDealSeat, error) {
	byDeal := map[string][]repository.HistoryDealSeat{}
	if matchID != histMatch {
		return byDeal, nil
	}
	for _, seat := range s.dealSeats {
		byDeal[seat.DealID] = append(byDeal[seat.DealID], seat)
	}
	return byDeal, nil
}

func (s *histStore) EventsOf(_ context.Context, matchID string) ([]repository.HistoryEvent, error) {
	if matchID != histMatch {
		return nil, nil
	}
	return s.events, nil
}

func histText(value string) *string { return &value }
