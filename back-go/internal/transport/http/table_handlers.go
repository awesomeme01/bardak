package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/application"
)

// LobbyUseCases — что нужно ручкам столов от сценариев.
//
// ⭐ Узкий интерфейс на стороне потребителя: ручки проверяются подделкой, без базы и
// контейнера. Ему удовлетворяет application.LobbyService как есть.
type LobbyUseCases interface {
	OpenTables(ctx context.Context) ([]application.TableSnapshot, error)
	ByID(ctx context.Context, tableID string) (application.TableSnapshot, error)
	ByCode(ctx context.Context, code string) (application.TableSnapshot, error)
	Invite(ctx context.Context, code string) (application.InviteSnapshot, error)
	Current(ctx context.Context, userID string) (application.CurrentSnapshot, error)
	Create(ctx context.Context, cmd application.CreateTableCommand) (application.TableSnapshot, error)
	Close(ctx context.Context, tableID, userID string) error
}

// TableHandlers — обработчики столов.
type TableHandlers struct {
	Lobby LobbyUseCases
	Log   *slog.Logger
}

// Routes вешает пути.
//
// ⭐ Пути и методы повторяют Java дословно: фронт не должен меняться вовсе.
// ⚠️ /invite/{code} — единственная ручка столов без токена (список открытых путей —
// в PublicPath). Всё остальное здесь требует Bearer.
func (h TableHandlers) Routes(router chi.Router) {
	router.Get("/api/tables", h.list)
	router.Post("/api/tables", h.create)
	router.Get("/api/tables/current", h.current)
	router.Get("/api/tables/invite/{code}", h.invite)
	router.Get("/api/tables/by-code/{code}", h.byCode)
	router.Get("/api/tables/{id}", h.byID)
	router.Delete("/api/tables/{id}", h.close)
}

// SeatView — место за столом.
type SeatView struct {
	SeatNo      int    `json:"seatNo"`
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Ready       bool   `json:"ready"`
	// ⚠️ Захардкожено true, как в Java: настоящего присутствия здесь пока нет, оно
	// придёт вместе с presence по сокету. Поле уже в контракте, и убрать его нельзя.
	Online bool `json:"online"`
}

// TableView — стол со списком мест.
//
// ⚠️ rulesConfig наружу НЕ отдаётся, хотя в запросе создания он есть: прочитать правила
// стола через REST нельзя — так же, как в Java.
type TableView struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	HostUserID string `json:"hostUserId"`
	MaxPlayers int    `json:"maxPlayers"`
	Status     string `json:"status"`
	CardSetID  string `json:"cardSetId"`
	ThemeID    string `json:"themeId"`
	IsPrivate  bool   `json:"isPrivate"`
	// ⚠️ Пустой список — []SeatView{} и БЕЗ omitempty: nil дал бы null, а Java отдаёт [].
	Seats []SeatView `json:"seats"`
}

// CurrentTableView — где игрок сидит сейчас.
//
// ⚠️ Table и MySeatNo — УКАЗАТЕЛИ (MD-003). Для MySeatNo это принципиально: место 0 —
// законное значение, и обычный int с omitempty вырезал бы его у хозяина стола, который
// как раз и сидит на нулевом. У того, кто нигде не сидит, ответ — ровно {"inMatch":false}.
type CurrentTableView struct {
	Table    *TableView `json:"table,omitempty"`
	InMatch  bool       `json:"inMatch"`
	MySeatNo *int       `json:"mySeatNo,omitempty"`
}

// TableInviteView — стол глазами того, кто пришёл по ссылке и ещё не вошёл в игру.
//
// ⚠️ Имён и идентификаторов игроков здесь НЕТ намеренно: код короткий и живёт в
// переписке, поэтому всё в этом ответе — публично.
type TableInviteView struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	MaxPlayers int    `json:"maxPlayers"`
	SeatsTaken int    `json:"seatsTaken"`
	IsPrivate  bool   `json:"isPrivate"`
	Joinable   bool   `json:"joinable"`
}

// CreateTableRequest — тело создания стола.
//
// ⚠️ Ключ приватности в JSON — именно isPrivate: в Java так назван компонент record,
// и Jackson читает его буквально; ключ private не подхватится ни там, ни здесь.
// cardSetId и themeId необязательны: пусто — берутся значения по умолчанию.
type CreateTableRequest struct {
	Name        string         `json:"name"`
	MaxPlayers  int            `json:"maxPlayers"`
	CardSetID   *string        `json:"cardSetId"`
	ThemeID     *string        `json:"themeId"`
	RulesConfig map[string]any `json:"rulesConfig"`
	IsPrivate   bool           `json:"isPrivate"`
}

// Validate проверяет тело создания стола.
//
// ⚠️ Тексты — дословно как у Bean Validation в Java: differential-проверка сверяет
// details посимвольно, и «улучшенная» формулировка светилась бы различием вечно.
func (r CreateTableRequest) Validate() error {
	c := newChecker()
	c.size("name", r.Name, 2, 64)
	// maxPlayers — примитив int: отсутствие поля даёт 0 и ту же ошибку, что и явный 0.
	switch {
	case r.MaxPlayers < 2:
		c.fields["maxPlayers"] = "must be greater than or equal to 2"
	case r.MaxPlayers > 5:
		c.fields["maxPlayers"] = "must be less than or equal to 5"
	}
	return c.result()
}

func (h TableHandlers) list(w http.ResponseWriter, r *http.Request) {
	tables, err := h.Lobby.OpenTables(r.Context())
	if err != nil {
		h.writeLobbyError(w, r, err)
		return
	}
	views := make([]TableView, 0, len(tables))
	for _, table := range tables {
		views = append(views, toTableView(table))
	}
	WriteJSON(w, http.StatusOK, views)
}

func (h TableHandlers) create(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	var request CreateTableRequest
	if err := DecodeJSON(r, &request); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := request.Validate(); err != nil {
		h.writeTableValidation(w, r, err)
		return
	}

	rules, err := rulesConfigJSON(request.RulesConfig)
	if err != nil {
		WriteError(w, r, h.Log, NewFault(http.StatusBadRequest,
			"INVALID_RULES_CONFIG", "Не удалось разобрать правила стола"))
		return
	}

	table, err := h.Lobby.Create(r.Context(), application.CreateTableCommand{
		HostUserID:  principal.UserID,
		Name:        request.Name,
		MaxPlayers:  request.MaxPlayers,
		CardSetID:   valueOr(request.CardSetID),
		ThemeID:     valueOr(request.ThemeID),
		RulesConfig: rules,
		IsPrivate:   request.IsPrivate,
	})
	if err != nil {
		h.writeLobbyError(w, r, err)
		return
	}
	// ⚠️ 200, а не 201, и без заголовка Location — как в Java.
	WriteJSON(w, http.StatusOK, toTableView(table))
}

func (h TableHandlers) current(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	current, err := h.Lobby.Current(r.Context(), principal.UserID)
	if err != nil {
		h.writeLobbyError(w, r, err)
		return
	}

	// ⚠️ Игрок нигде не сидит — это 200 и {"inMatch":false}, а не 404: «нет стола» здесь
	// нормальный ответ, а не ошибка.
	view := CurrentTableView{InMatch: current.InMatch, MySeatNo: current.MySeatNo}
	if current.Table != nil {
		table := toTableView(*current.Table)
		view.Table = &table
	}
	WriteJSON(w, http.StatusOK, view)
}

func (h TableHandlers) byID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		// ⚠️ 400, а не 500. Раньше Java валила такой запрос в «что-то пошло не так»,
		// и клиент не мог отличить свою ошибку от поломки сервера. Починено в Java
		// (ApiHardeningIT), здесь повторяется исправленное поведение.
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}

	table, err := h.Lobby.ByID(r.Context(), id)
	if err != nil {
		h.writeLobbyError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toTableView(table))
}

func (h TableHandlers) byCode(w http.ResponseWriter, r *http.Request) {
	// ⚠️ Полный вид стола со списком игроков остаётся ЗА токеном: без токена доступен
	// только /invite/{code}. Регистр кода не важен — этим занимается запрос к базе.
	table, err := h.Lobby.ByCode(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		h.writeLobbyError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toTableView(table))
}

// invite — заглянуть за стол по коду ДО входа в игру.
//
// ⭐ Единственная ручка столов без токена, и это её смысл: по ссылке-приглашению приходит
// и тот, у кого учётки ещё нет. Он должен увидеть, куда его зовут, ДО регистрации, —
// иначе ссылка открывает безымянную форму входа, и человек уходит.
func (h TableHandlers) invite(w http.ResponseWriter, r *http.Request) {
	invite, err := h.Lobby.Invite(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		h.writeLobbyError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, TableInviteView{
		Code:       invite.Code,
		Name:       invite.Name,
		MaxPlayers: invite.MaxPlayers,
		SeatsTaken: invite.SeatsTaken,
		IsPrivate:  invite.IsPrivate,
		Joinable:   invite.Joinable,
	})
}

func (h TableHandlers) close(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")
	// ⚠️ Та же починка, что и в byID: невалидный UUID — это 400 BAD_REQUEST.
	if _, err := uuid.Parse(id); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}

	if err := h.Lobby.Close(r.Context(), id, principal.UserID); err != nil {
		h.writeLobbyError(w, r, err)
		return
	}
	// 204 и пустое тело.
	w.WriteHeader(http.StatusNoContent)
}

func (h TableHandlers) writeTableValidation(w http.ResponseWriter, r *http.Request, err error) {
	var validation ValidationError
	if errors.As(err, &validation) {
		WriteError(w, r, h.Log, validation.AsFault())
		return
	}
	WriteError(w, r, h.Log, ErrBadRequest)
}

// writeLobbyError переводит отказ сценария в код и статус.
//
// ⚠️ Текст у MATCH_IN_PROGRESS и ALREADY_AT_TABLE берётся ИЗ ОШИБКИ, а не из константы:
// в Java на один код приходится три разные строки, и какая из них верна, знает место
// броска, а не транспорт.
func (h TableHandlers) writeLobbyError(w http.ResponseWriter, r *http.Request, err error) {
	var inMatch application.MatchInProgressError
	if errors.As(err, &inMatch) {
		WriteError(w, r, h.Log, NewFault(http.StatusConflict, "MATCH_IN_PROGRESS", inMatch.Error()))
		return
	}
	var seated application.AlreadyAtTableError
	if errors.As(err, &seated) {
		WriteError(w, r, h.Log, NewFault(http.StatusConflict, "ALREADY_AT_TABLE", seated.Error()))
		return
	}

	switch {
	case errors.Is(err, application.ErrTableNotFound):
		WriteError(w, r, h.Log, NewFault(http.StatusNotFound, "TABLE_NOT_FOUND", "Стол не найден"))
	case errors.Is(err, application.ErrNotTableHost):
		WriteError(w, r, h.Log, NewFault(http.StatusForbidden, "NOT_TABLE_HOST",
			"Стол закрывает только хозяин"))
	case errors.Is(err, application.ErrTableNotOpen):
		WriteError(w, r, h.Log, NewFault(http.StatusConflict, "TABLE_NOT_OPEN",
			"За этот стол уже нельзя сесть"))
	case errors.Is(err, application.ErrTableFull):
		WriteError(w, r, h.Log, NewFault(http.StatusConflict, "TABLE_FULL",
			"За столом нет свободных мест"))
	case errors.Is(err, application.ErrNoDefaultCardSet):
		WriteError(w, r, h.Log, NewFault(http.StatusInternalServerError, "NO_DEFAULT",
			"Не настроен набор карт по умолчанию"))
	case errors.Is(err, application.ErrNoDefaultTheme):
		// ⚠️ Рассогласованный род — не опечатка, а копия Java: там сообщение собирается
		// как «Не настроен %s по умолчанию» с подстановкой «тема стола».
		WriteError(w, r, h.Log, NewFault(http.StatusInternalServerError, "NO_DEFAULT",
			"Не настроен тема стола по умолчанию"))
	default:
		WriteError(w, r, h.Log, ErrInternal)
	}
}

func toTableView(snapshot application.TableSnapshot) TableView {
	seats := make([]SeatView, 0, len(snapshot.Seats))
	for _, seat := range snapshot.Seats {
		seats = append(seats, SeatView{
			SeatNo:      seat.SeatNo,
			UserID:      seat.UserID,
			DisplayName: seat.DisplayName,
			Ready:       seat.Ready,
			Online:      true,
		})
	}
	table := snapshot.Table
	return TableView{
		ID:         table.ID,
		Code:       table.Code,
		Name:       table.Name,
		HostUserID: table.HostUserID,
		MaxPlayers: table.MaxPlayers,
		Status:     table.Status,
		CardSetID:  table.CardSetID,
		ThemeID:    table.ThemeID,
		IsPrivate:  table.IsPrivate,
		Seats:      seats,
	}
}

// rulesConfigJSON сериализует правила стола для jsonb-колонки.
//
// Пусто — это {}, а не null: колонка объявлена not null default '{}'.
func rulesConfigJSON(rules map[string]any) (string, error) {
	if len(rules) == 0 {
		return "{}", nil
	}
	encoded, err := json.Marshal(rules)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func valueOr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
