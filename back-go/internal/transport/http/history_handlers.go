package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/application"
)

// История матчей: список, детали и реплей.
//
// ⚠️ Nullable-поля — указатели с omitempty, остальные — значения БЕЗ omitempty (MD-003).
// В MatchSummary у идущего матча пропадают finishedAt, abortReason, myPlace и
// myRatingDelta, а playersCount со значением 0 обязан остаться.

// MatchSummaryView — матч в списке.
type MatchSummaryView struct {
	ID           string     `json:"id"`
	TableID      string     `json:"tableId"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	PlayersCount int        `json:"playersCount"`
	DealsPlayed  int        `json:"dealsPlayed"`
	AbortReason  *string    `json:"abortReason,omitempty"`
	// RatingCounted — ⭐ отменённый матч виден в истории, но рейтинга не касается:
	// без этого признака нулевая дельта выглядела бы как ничья.
	RatingCounted bool              `json:"ratingCounted"`
	MyPlace       *int              `json:"myPlace,omitempty"`
	MyRatingDelta *json.Number      `json:"myRatingDelta,omitempty"`
	Players       []MatchPlayerView `json:"players"`
}

// MatchPlayerView — итог участника матча.
type MatchPlayerView struct {
	UserID      string  `json:"userId"`
	DisplayName string  `json:"displayName"`
	SeatNo      int     `json:"seatNo"`
	Place       *int    `json:"place,omitempty"`
	NavesLevel  *string `json:"navesLevel,omitempty"`
	LossType    *string `json:"lossType,omitempty"`
	// Рейтинги — числа без кавычек и с тем масштабом, что лежит в базе (numeric(8,2)):
	// json.Number пишет строку как есть, а float64 превратил бы 1000.00 в 1000.
	RatingBefore *json.Number `json:"ratingBefore,omitempty"`
	RatingAfter  *json.Number `json:"ratingAfter,omitempty"`
	RatingDelta  *json.Number `json:"ratingDelta,omitempty"`
}

// MatchDetailsView — матч с разбивкой по раздачам.
type MatchDetailsView struct {
	Match MatchSummaryView  `json:"match"`
	Deals []DealSummaryView `json:"deals"`
}

// DealSummaryView — одна раздача.
type DealSummaryView struct {
	DealNo    int     `json:"dealNo"`
	TrumpSuit *string `json:"trumpSuit,omitempty"`
	LoserSeat int     `json:"loserSeat"`
	// FinishedAt у незавершённой раздачи вырезается, как и в Java.
	FinishedAt      *time.Time     `json:"finishedAt,omitempty"`
	LastAttackCards []string       `json:"lastAttackCards"`
	Seats           []DealSeatView `json:"seats"`
}

// DealSeatView — итог места в раздаче.
type DealSeatView struct {
	SeatNo           int                   `json:"seatNo"`
	Place            *int                  `json:"place,omitempty"`
	HungCards        []string              `json:"hungCards"`
	NavesLevelBefore *string               `json:"navesLevelBefore,omitempty"`
	NavesLevelAfter  *string               `json:"navesLevelAfter,omitempty"`
	LevelChanges     []DealLevelChangeView `json:"levelChanges"`
}

// DealLevelChangeView — почему уровень навесов изменился и на сколько.
type DealLevelChangeView struct {
	Reason string `json:"reason"`
	Amount int    `json:"amount"`
}

// ReplayEventView — одно событие реплея, ровно в той форме, в какой оно уходило живьём.
type ReplayEventView struct {
	Seq       int    `json:"seq"`
	DealNo    *int   `json:"dealNo,omitempty"`
	Type      string `json:"type"`
	ActorSeat *int   `json:"actorSeat,omitempty"`
	// Payload отдаётся как есть, без обёртки: это тот же JSON, что ушёл в сокет.
	Payload json.RawMessage `json:"payload"`
}

// ReplayView — реплей матча глазами спрашивающего.
type ReplayView struct {
	MatchID string `json:"matchId"`
	Status  string `json:"status"`
	// MySeat — ⭐ -1 у того, кто в матче не играл; ему видны только публичные события.
	MySeat int               `json:"mySeat"`
	Events []ReplayEventView `json:"events"`
}

// HistoryHandlers — обработчики истории матчей.
type HistoryHandlers struct {
	History application.HistoryService
	Log     *slog.Logger
}

// Routes вешает пути.
func (h HistoryHandlers) Routes(router chi.Router) {
	router.Get("/api/matches", h.list)
	router.Get("/api/matches/{id}", h.details)
	router.Get("/api/matches/{id}/replay", h.replay)
}

// list — матчи игрока, новые сверху.
//
// ⭐ Смотреть можно свою историю и историю друга. Рейтинг и статистика при этом открыты
// — на них держится таблица лидеров, это агрегаты. История же — «с кем и когда я играл»,
// и посторонним её знать незачем.
func (h HistoryHandlers) list(w http.ResponseWriter, r *http.Request) {
	me, ok := PrincipalFrom(r.Context())
	if !ok {
		WriteError(w, r, h.Log, ErrInternal)
		return
	}

	whose := me.UserID
	if raw := strings.TrimSpace(r.URL.Query().Get("userId")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			// ⚠️ 400, а не 500: Java ловит MethodArgumentTypeMismatchException отдельным
			// обработчиком (ApiExceptionHandler.handleBadRequest). Раньше это был 500,
			// и клиент не мог отличить свою ошибку от поломки сервера.
			WriteError(w, r, h.Log, ErrBadRequest)
			return
		}
		whose = parsed.String()
	}

	// Неизвестное значение status даёт пустой список, а не ошибку — как в Java.
	summaries, err := h.History.Matches(r.Context(), me.UserID, whose, r.URL.Query().Get("status"))
	if err != nil {
		h.writeHistoryError(w, r, err, NewFault(http.StatusForbidden, "NOT_FRIENDS",
			"История матчей видна своим и друзьям"))
		return
	}

	views := make([]MatchSummaryView, 0, len(summaries))
	for _, summary := range summaries {
		views = append(views, toMatchSummaryView(summary))
	}
	WriteJSON(w, http.StatusOK, views)
}

func (h HistoryHandlers) details(w http.ResponseWriter, r *http.Request) {
	me, matchID, ok := h.matchRequest(w, r)
	if !ok {
		return
	}

	details, err := h.History.Details(r.Context(), me, matchID)
	if err != nil {
		h.writeHistoryError(w, r, err, matchForbidden())
		return
	}

	deals := make([]DealSummaryView, 0, len(details.Deals))
	for _, deal := range details.Deals {
		deals = append(deals, toDealSummaryView(deal))
	}
	WriteJSON(w, http.StatusOK, MatchDetailsView{
		Match: toMatchSummaryView(details.Match),
		Deals: deals,
	})
}

// replay — матч глазами спрашивающего и только после матча.
func (h HistoryHandlers) replay(w http.ResponseWriter, r *http.Request) {
	me, matchID, ok := h.matchRequest(w, r)
	if !ok {
		return
	}

	replay, err := h.History.Replay(r.Context(), me, matchID)
	if err != nil {
		h.writeHistoryError(w, r, err, matchForbidden())
		return
	}

	events := make([]ReplayEventView, 0, len(replay.Events))
	for _, event := range replay.Events {
		events = append(events, ReplayEventView{
			Seq:       event.Seq,
			DealNo:    event.DealNo,
			Type:      event.Type,
			ActorSeat: event.ActorSeat,
			Payload:   event.Payload,
		})
	}
	WriteJSON(w, http.StatusOK, ReplayView{
		MatchID: replay.MatchID,
		Status:  replay.Status,
		MySeat:  replay.MySeat,
		Events:  events,
	})
}

// matchRequest достаёт спрашивающего и идентификатор матча из пути.
func (h HistoryHandlers) matchRequest(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	me, ok := PrincipalFrom(r.Context())
	if !ok {
		WriteError(w, r, h.Log, ErrInternal)
		return "", "", false
	}
	parsed, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		// ⚠️ Как и с userId: непреобразуемый UUID в пути до контроллера не доходит
		// и отвечает 400 BAD_REQUEST.
		WriteError(w, r, h.Log, ErrBadRequest)
		return "", "", false
	}
	return me.UserID, parsed.String(), true
}

// matchForbidden — отказ по видимости КОНКРЕТНОГО матча.
//
// ⚠️ Сообщение другое, чем у списка: код один (NOT_FRIENDS), а поводы разные — «чужая
// история» и «чужой матч». Java различает их текстом, и различие видно игроку.
func matchForbidden() Fault {
	return NewFault(http.StatusForbidden, "NOT_FRIENDS", "Этот матч видят его игроки и их друзья")
}

// writeHistoryError переводит ошибку сценария в код и статус.
func (h HistoryHandlers) writeHistoryError(w http.ResponseWriter, r *http.Request,
	err error, forbidden Fault) {
	switch {
	case errors.Is(err, application.ErrHistoryForbidden):
		WriteError(w, r, h.Log, forbidden)
	case errors.Is(err, application.ErrHistoryMatchNotFound):
		WriteError(w, r, h.Log, NewFault(http.StatusNotFound, "MATCH_NOT_FOUND", "Такого матча нет"))
	case errors.Is(err, application.ErrHistoryMatchNotFinished):
		WriteError(w, r, h.Log, NewFault(http.StatusConflict, "MATCH_NOT_FINISHED",
			"Реплей доступен только после матча"))
	default:
		WriteError(w, r, h.Log, ErrInternal)
	}
}

func toMatchSummaryView(summary application.MatchSummary) MatchSummaryView {
	players := make([]MatchPlayerView, 0, len(summary.Players))
	for _, player := range summary.Players {
		players = append(players, MatchPlayerView{
			UserID:       player.UserID,
			DisplayName:  player.DisplayName,
			SeatNo:       player.SeatNo,
			Place:        player.Place,
			NavesLevel:   player.NavesLevel,
			LossType:     player.LossType,
			RatingBefore: historyNumber(player.RatingBefore),
			RatingAfter:  historyNumber(player.RatingAfter),
			RatingDelta:  historyNumber(player.RatingDelta),
		})
	}
	return MatchSummaryView{
		ID:            summary.Match.ID,
		TableID:       summary.Match.TableID,
		Status:        summary.Match.Status,
		StartedAt:     summary.Match.StartedAt.UTC(),
		FinishedAt:    historyInstant(summary.Match.FinishedAt),
		PlayersCount:  summary.Match.PlayersCount,
		DealsPlayed:   summary.Match.DealsPlayed,
		AbortReason:   summary.Match.AbortReason,
		RatingCounted: summary.RatingCounted,
		MyPlace:       summary.MyPlace,
		MyRatingDelta: historyNumber(summary.MyRatingDelta),
		Players:       players,
	}
}

func toDealSummaryView(breakdown application.DealBreakdown) DealSummaryView {
	seats := make([]DealSeatView, 0, len(breakdown.Seats))
	for _, seat := range breakdown.Seats {
		changes := make([]DealLevelChangeView, 0, len(seat.LevelChanges))
		for _, change := range seat.LevelChanges {
			changes = append(changes, DealLevelChangeView{Reason: change.Reason, Amount: change.Amount})
		}
		seats = append(seats, DealSeatView{
			SeatNo:           seat.SeatNo,
			Place:            seat.Place,
			HungCards:        seat.HungCards,
			NavesLevelBefore: seat.NavesLevelBefore,
			NavesLevelAfter:  seat.NavesLevelAfter,
			LevelChanges:     changes,
		})
	}
	return DealSummaryView{
		DealNo:          breakdown.Deal.DealNo,
		TrumpSuit:       breakdown.Deal.TrumpSuit,
		LoserSeat:       breakdown.Deal.LoserSeat,
		FinishedAt:      historyInstant(breakdown.Deal.FinishedAt),
		LastAttackCards: breakdown.Deal.LastAttackCards,
		Seats:           seats,
	}
}

// historyNumber — рейтинг из базы как есть, числом без кавычек.
//
// ⚠️ Масштаб приходит из numeric(8,2) и сохраняется: Java печатает BigDecimal с тем
// масштабом, что лежит в базе, и «улучшение» до 1000 сломало бы побайтовое сравнение.
func historyNumber(value *string) *json.Number {
	if value == nil {
		return nil
	}
	number := json.Number(*value)
	return &number
}

// historyInstant — время в UTC; nil остаётся nil и вырезается из ответа.
//
// ⚠️ Приведение к UTC обязательно: pgx отдаёт время в зоне соединения, а Java печатает
// Instant всегда с Z на конце.
func historyInstant(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}
