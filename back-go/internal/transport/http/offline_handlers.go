package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/application"
)

// Запись партии, сыгранной за настоящим столом.
//
// ⭐ Отдельный префикс `/api/offline`, а не ещё один метод в истории: история — чтение,
// а это единственная ручка во всём приложении, которая создаёт законченный матч сразу,
// минуя стол и сокет.

// OfflinePlayerRequest — один игрок в записываемой партии.
type OfflinePlayerRequest struct {
	UserID string `json:"userId"`
	// Outcome — исход по шкале: «NONE», ступень («6»…«A»), «Jk» или степень проигрыша.
	Outcome string `json:"outcome"`
}

// OfflineMatchRequest — тело записи партии.
type OfflineMatchRequest struct {
	// PlayedAt — когда играли; пусто — сейчас.
	PlayedAt *time.Time             `json:"playedAt,omitempty"`
	Players  []OfflinePlayerRequest `json:"players"`
}

// Validate проверяет форму запроса.
//
// ⚠️ Только форму: состав, дружба и шкала — дело сценария. Здесь ловится то, из-за чего
// запрос вообще не имеет смысла разбирать дальше.
func (r OfflineMatchRequest) Validate() error {
	c := newChecker()
	if len(r.Players) == 0 {
		c.fail("players", "укажи, кто играл")
	}
	for _, player := range r.Players {
		if _, err := uuid.Parse(player.UserID); err != nil || len(player.UserID) != 36 {
			c.fail("players", "битый идентификатор игрока")
			break
		}
	}
	for _, player := range r.Players {
		if player.Outcome == "" {
			c.fail("players", "у каждого игрока должен быть исход")
			break
		}
	}
	return c.result()
}

// OfflineOutcomeView — исход шкалы для формы записи.
type OfflineOutcomeView struct {
	Code string `json:"code"`
}

// OfflineMatchView — что вернулось после записи партии.
type OfflineMatchView struct {
	MatchID string             `json:"matchId"`
	Changes []RatingChangeView `json:"ratingChanges"`
}

// RatingChangeView — изменение рейтинга одного участника.
type RatingChangeView struct {
	UserID string `json:"userId"`
	Place  int    `json:"place"`
	// NavesLevel — ступень; пусто, если навесов не было.
	NavesLevel *string `json:"navesLevel,omitempty"`
	// LossDegree — степень проигрыша; пусто у не проигравшего.
	LossDegree *string     `json:"lossDegree,omitempty"`
	Before     json.Number `json:"ratingBefore"`
	After      json.Number `json:"ratingAfter"`
	Delta      json.Number `json:"ratingDelta"`
}

// OfflineHandlers — запись оффлайн-партий и шкала исходов для формы.
type OfflineHandlers struct {
	Offline application.OfflineMatchService
	Log     *slog.Logger
}

// Routes вешает пути.
func (h OfflineHandlers) Routes(router chi.Router) {
	router.Get("/api/offline/outcomes", h.outcomes)
	router.Post("/api/offline/matches", h.register)
}

// outcomes — шкала исходов от лучшего к худшему.
//
// ⭐ Отдаётся сервером, а не выписывается во фронте: шкала — это правила игры, и её копия
// в разметке разошлась бы с рейтингом на первой же правке.
func (h OfflineHandlers) outcomes(w http.ResponseWriter, r *http.Request) {
	codes := application.OfflineOutcomeCodes()
	views := make([]OfflineOutcomeView, 0, len(codes))
	for _, code := range codes {
		views = append(views, OfflineOutcomeView{Code: code})
	}
	WriteJSON(w, http.StatusOK, views)
}

func (h OfflineHandlers) register(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		WriteError(w, r, h.Log, ErrInternal)
		return
	}

	var request OfflineMatchRequest
	if err := DecodeJSON(r, &request); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := request.Validate(); err != nil {
		var validation ValidationError
		if errors.As(err, &validation) {
			WriteError(w, r, h.Log, validation.AsFault())
			return
		}
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}

	players := make([]application.OfflineOutcome, 0, len(request.Players))
	for _, player := range request.Players {
		players = append(players, application.OfflineOutcome{
			UserID: player.UserID, Code: player.Outcome,
		})
	}

	matchID, changes, err := h.Offline.Register(r.Context(), principal.UserID,
		application.OfflineMatchRequest{PlayedAt: request.PlayedAt, Players: players})
	if err != nil {
		h.writeOfflineError(w, r, err)
		return
	}

	views := make([]RatingChangeView, 0, len(changes))
	for _, change := range changes {
		views = append(views, RatingChangeView{
			UserID: change.UserID, Place: change.Place,
			NavesLevel: change.NavesLevel, LossDegree: change.LossDegree,
			Before: json.Number(change.Before), After: json.Number(change.After),
			Delta: json.Number(change.Delta),
		})
	}
	WriteJSON(w, http.StatusCreated, OfflineMatchView{MatchID: matchID, Changes: views})
}

// writeOfflineError переводит отказ сценария в код и статус.
func (h OfflineHandlers) writeOfflineError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrOfflineTooFewPlayers):
		WriteError(w, r, h.Log, NewFault(http.StatusBadRequest, "OFFLINE_TOO_FEW_PLAYERS",
			"В партии должно быть хотя бы двое"))
	case errors.Is(err, application.ErrOfflineTooManyPlayers):
		WriteError(w, r, h.Log, NewFault(http.StatusBadRequest, "OFFLINE_TOO_MANY_PLAYERS",
			"За столом не бывает больше восьми"))
	case errors.Is(err, application.ErrOfflineDuplicatePlayer):
		WriteError(w, r, h.Log, NewFault(http.StatusBadRequest, "OFFLINE_DUPLICATE_PLAYER",
			"Игрок указан дважды"))
	case errors.Is(err, application.ErrOfflineUnknownOutcome):
		WriteError(w, r, h.Log, NewFault(http.StatusBadRequest, "OFFLINE_UNKNOWN_OUTCOME",
			"Такого исхода нет на шкале"))
	case errors.Is(err, application.ErrOfflineFuture):
		WriteError(w, r, h.Log, NewFault(http.StatusBadRequest, "OFFLINE_FUTURE",
			"Партия не может быть сыграна в будущем"))
	case errors.Is(err, application.ErrOfflineNotAParticipant):
		WriteError(w, r, h.Log, NewFault(http.StatusForbidden, "OFFLINE_NOT_A_PARTICIPANT",
			"Записать можно только партию, в которой играл сам"))
	case errors.Is(err, application.ErrOfflineNotFriends):
		WriteError(w, r, h.Log, NewFault(http.StatusForbidden, "NOT_FRIENDS",
			"В состав идут только друзья"))
	default:
		WriteError(w, r, h.Log, ErrInternal)
	}
}
