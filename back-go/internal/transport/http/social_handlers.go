package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/application"
)

// Друзья и подписки на уведомления.
//
// Всё вне живой партии ходит по REST — друзья не исключение. По сокету уходит только само
// приглашение за стол, потому что его надо услышать сразу.

// friendScenarios — что обработчикам нужно от сценариев друзей.
//
// Интерфейс, а не конкретный сервис: так проверка кодов и форм ответа обходится без базы,
// а собирается всё равно из application.FriendService.
type friendScenarios interface {
	List(ctx context.Context, userID string) (application.FriendList, error)
	Request(ctx context.Context, userID, username string) (application.Friend, error)
	Accept(ctx context.Context, userID, friendID string) (application.Friend, error)
	Remove(ctx context.Context, userID, friendID string) error
	Invite(ctx context.Context, userID, friendID, tableID string) (bool, error)
}

// pushScenarios — что обработчикам нужно от подписок.
type pushScenarios interface {
	Enabled() bool
	PublicKey() string
	Subscribe(ctx context.Context, userID, endpoint, p256dh, auth string, userAgent *string) error
	Unsubscribe(ctx context.Context, userID, endpoint string) error
}

// SocialHandlers — друзья и подписки на уведомления.
type SocialHandlers struct {
	Friends friendScenarios
	Push    pushScenarios
	Log     *slog.Logger
}

// Routes вешает пути.
//
// ⭐ Пути и методы повторяют Java дословно: фронт не должен меняться вовсе.
func (h SocialHandlers) Routes(router chi.Router) {
	router.Get("/api/friends", h.list)
	router.Post("/api/friends/requests", h.request)
	router.Post("/api/friends/{friendId}/accept", h.accept)
	router.Delete("/api/friends/{friendId}", h.remove)
	router.Post("/api/friends/{friendId}/invite", h.invite)

	router.Get("/api/push/key", h.pushKey)
	router.Post("/api/push/subscriptions", h.subscribe)
	// ⚠️ DELETE С ТЕЛОМ — так в Java, и контракт менять нельзя: клиент шлёт endpoint
	// в теле, а не в пути и не в query.
	router.Delete("/api/push/subscriptions", h.unsubscribe)
}

// FriendView — друг или заявка в ответе.
//
// ⚠️ avatar — единственное поле, которого может не быть: остальные Java отдаёт всегда,
// включая false у online и mine (MD-003).
type FriendView struct {
	UserID      string  `json:"userId"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	Avatar      *string `json:"avatar,omitempty"`
	Online      bool    `json:"online"`
	Status      string  `json:"status"`
	Mine        bool    `json:"mine"`
}

// FriendListView — список друзей и заявок.
//
// ⚠️ Пустые списки отдаются как [], а не null: у Java они всегда массивы.
type FriendListView struct {
	Friends  []FriendView `json:"friends"`
	Incoming []FriendView `json:"incoming"`
	Outgoing []FriendView `json:"outgoing"`
}

// AddFriendRequest — тело заявки в друзья.
type AddFriendRequest struct {
	Username string `json:"username"`
}

// TableInviteRequest — тело приглашения за стол.
//
// ⚠️ tableId — строка, а не UUID: в Java он объявлен строкой и разбирается вручную.
type TableInviteRequest struct {
	TableID string `json:"tableId"`
}

// InviteResultView — дошло ли приглашение прямо сейчас.
type InviteResultView struct {
	Delivered bool `json:"delivered"`
}

// PushKeyView — ответ про уведомления.
//
// ⚠️ Две формы: {"enabled":true,"publicKey":"…"} и {"enabled":false} БЕЗ ключа.
// enabled=false — не поломка, а «уведомления не настроены».
type PushKeyView struct {
	Enabled   bool    `json:"enabled"`
	PublicKey *string `json:"publicKey,omitempty"`
}

// PushSubscribeRequest — тело подписки. Ключи браузер отдаёт в base64url; сервер их
// не разбирает, а только хранит.
type PushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

// PushUnsubscribeRequest — тело отписки.
type PushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

func (h SocialHandlers) list(w http.ResponseWriter, r *http.Request) {
	viewer, ok := h.viewer(w, r)
	if !ok {
		return
	}
	list, err := h.Friends.List(r.Context(), viewer)
	if err != nil {
		h.writeFriendError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, FriendListView{
		Friends:  toFriendViews(list.Friends),
		Incoming: toFriendViews(list.Incoming),
		Outgoing: toFriendViews(list.Outgoing),
	})
}

func (h SocialHandlers) request(w http.ResponseWriter, r *http.Request) {
	viewer, ok := h.viewer(w, r)
	if !ok {
		return
	}
	var body AddFriendRequest
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := body.Validate(); err != nil {
		h.writeSocialValidation(w, r, err)
		return
	}

	friend, err := h.Friends.Request(r.Context(), viewer, body.Username)
	if err != nil {
		h.writeFriendError(w, r, err)
		return
	}
	// ⚠️ 200, а не 201: заявка — не ресурс с адресом, и Location в Java не отдаётся.
	WriteJSON(w, http.StatusOK, toFriendView(friend))
}

func (h SocialHandlers) accept(w http.ResponseWriter, r *http.Request) {
	viewer, ok := h.viewer(w, r)
	if !ok {
		return
	}
	friendID, ok := h.pathUUID(w, r, "friendId")
	if !ok {
		return
	}

	friend, err := h.Friends.Accept(r.Context(), viewer, friendID)
	if err != nil {
		h.writeFriendError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toFriendView(friend))
}

func (h SocialHandlers) remove(w http.ResponseWriter, r *http.Request) {
	viewer, ok := h.viewer(w, r)
	if !ok {
		return
	}
	friendID, ok := h.pathUUID(w, r, "friendId")
	if !ok {
		return
	}

	if err := h.Friends.Remove(r.Context(), viewer, friendID); err != nil {
		h.writeFriendError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h SocialHandlers) invite(w http.ResponseWriter, r *http.Request) {
	viewer, ok := h.viewer(w, r)
	if !ok {
		return
	}
	friendID, ok := h.pathUUID(w, r, "friendId")
	if !ok {
		return
	}
	var body TableInviteRequest
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := body.Validate(); err != nil {
		h.writeSocialValidation(w, r, err)
		return
	}
	if _, err := uuid.Parse(strings.TrimSpace(body.TableID)); err != nil {
		// ⚠️ Это ПОВТОРЕНИЕ поведения Java, а не задумка: там tableId объявлен строкой
		// и разбирается через UUID.fromString уже в контроллере, поэтому мусор
		// проваливается в общий обработчик и отвечает 500, а не 400. Чинить надо в Java
		// первой — иначе два бэкенда ответят по-разному на один и тот же запрос.
		WriteError(w, r, h.Log, ErrInternal)
		return
	}

	delivered, err := h.Friends.Invite(r.Context(), viewer, friendID, strings.TrimSpace(body.TableID))
	if err != nil {
		h.writeFriendError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, InviteResultView{Delivered: delivered})
}

// pushKey отдаёт открытый ключ VAPID.
//
// ⚠️ Ключ публичный, но ручка закрыта токеном: под permitAll она в Java не попадает,
// и открывать её здесь значило бы разойтись с эталоном.
func (h SocialHandlers) pushKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.viewer(w, r); !ok {
		return
	}
	if !h.Push.Enabled() {
		WriteJSON(w, http.StatusOK, PushKeyView{Enabled: false})
		return
	}
	key := h.Push.PublicKey()
	WriteJSON(w, http.StatusOK, PushKeyView{Enabled: true, PublicKey: &key})
}

func (h SocialHandlers) subscribe(w http.ResponseWriter, r *http.Request) {
	viewer, ok := h.viewer(w, r)
	if !ok {
		return
	}
	var body PushSubscribeRequest
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := body.Validate(); err != nil {
		h.writeSocialValidation(w, r, err)
		return
	}

	err := h.Push.Subscribe(r.Context(), viewer, body.Endpoint, body.P256dh, body.Auth, userAgent(r))
	if err != nil {
		WriteError(w, r, h.Log, ErrInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h SocialHandlers) unsubscribe(w http.ResponseWriter, r *http.Request) {
	viewer, ok := h.viewer(w, r)
	if !ok {
		return
	}
	var body PushUnsubscribeRequest
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := body.Validate(); err != nil {
		h.writeSocialValidation(w, r, err)
		return
	}

	if err := h.Push.Unsubscribe(r.Context(), viewer, body.Endpoint); err != nil {
		WriteError(w, r, h.Log, ErrInternal)
		return
	}
	// Неизвестный endpoint — тоже 204: отписка идемпотентна.
	w.WriteHeader(http.StatusNoContent)
}

// viewer — кто спрашивает. Токен проверен посредником, но обработчик обязан пережить
// и его отсутствие: 401 честнее, чем паника на пустом контексте.
func (h SocialHandlers) viewer(w http.ResponseWriter, r *http.Request) (string, bool) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok || principal.UserID == "" {
		unauthorized(w)
		return "", false
	}
	return principal.UserID, true
}

// pathUUID достаёт идентификатор из пути.
//
// ⚠️ Мусор вместо UUID — 400 BAD_REQUEST: в Java такой путь ловит обработчик
// MethodArgumentTypeMismatchException, и это его код. Заодно идентификатор приводится
// к канонической записи — по ней считается порядок пары в базе.
func (h SocialHandlers) pathUUID(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, name)))
	if err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return "", false
	}
	return parsed.String(), true
}

func (h SocialHandlers) writeSocialValidation(w http.ResponseWriter, r *http.Request, err error) {
	var validation ValidationError
	if errors.As(err, &validation) {
		WriteError(w, r, h.Log, validation.AsFault())
		return
	}
	WriteError(w, r, h.Log, ErrBadRequest)
}

// writeFriendError переводит отказ сценария в код и статус.
//
// ⚠️ NOT_FRIENDS живёт с ДВУМЯ статусами: 404, когда пары нет вовсе, и 403, когда зовут
// за стол не друга. Одна константа на код сломала бы контракт, поэтому статус берётся
// по месту.
func (h SocialHandlers) writeFriendError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrUserNotFound):
		WriteError(w, r, h.Log, ErrUserNotFound)
	case errors.Is(err, application.ErrCannotFriendSelf):
		WriteError(w, r, h.Log, NewFault(http.StatusConflict,
			"CANNOT_FRIEND_SELF", "С самим собой дружить не получится"))
	case errors.Is(err, application.ErrAlreadyFriends):
		WriteError(w, r, h.Log, NewFault(http.StatusConflict, "ALREADY_FRIENDS", "Вы уже друзья"))
	case errors.Is(err, application.ErrRequestAlreadySent):
		WriteError(w, r, h.Log, NewFault(http.StatusConflict,
			"REQUEST_ALREADY_SENT", "Заявка уже отправлена — ждём ответа"))
	case errors.Is(err, application.ErrNotYourRequest):
		WriteError(w, r, h.Log, NewFault(http.StatusConflict,
			"NOT_YOUR_REQUEST", "Эту заявку принимаешь не ты"))
	case errors.Is(err, application.ErrPairNotFound):
		WriteError(w, r, h.Log, NewFault(http.StatusNotFound, "NOT_FRIENDS", "Такой пары нет"))
	case errors.Is(err, application.ErrNotFriends):
		WriteError(w, r, h.Log, NewFault(http.StatusForbidden,
			"NOT_FRIENDS", "Звать за стол можно только друзей"))
	case errors.Is(err, application.ErrInviteTableNotFound):
		WriteError(w, r, h.Log, NewFault(http.StatusNotFound, "TABLE_NOT_FOUND", "Стол не найден"))
	default:
		WriteError(w, r, h.Log, ErrInternal)
	}
}

func toFriendViews(friends []application.Friend) []FriendView {
	// ⚠️ Именно [], а не nil: nil ушёл бы в JSON как null, а Java отдаёт пустой массив.
	views := []FriendView{}
	for _, friend := range friends {
		views = append(views, toFriendView(friend))
	}
	return views
}

func toFriendView(friend application.Friend) FriendView {
	return FriendView{
		UserID:      friend.UserID,
		Username:    friend.Username,
		DisplayName: friend.DisplayName,
		Avatar:      friend.Avatar,
		Online:      friend.Online,
		Status:      friend.Status,
		Mine:        friend.Mine,
	}
}

// socialChecker — проверка полей друзей и push.
//
// Свой, а не общий: у endpoint в Java стоит только @NotBlank без @Size, и общая проверка
// «от и до» отвергала бы длинные, но законные адреса подписки.
type socialChecker struct{ fields map[string]any }

func newSocialChecker() *socialChecker { return &socialChecker{fields: map[string]any{}} }

// notBlank — @NotBlank: пусто или одни пробелы.
//
// ⚠️ Текст сообщения ДОСЛОВНО как у Bean Validation, вплоть до английского: differential-
// проверка сверяет details посимвольно, и «улучшенная» формулировка светилась бы
// различием вечно.
func (c *socialChecker) notBlank(name, value string) bool {
	if strings.TrimSpace(value) == "" {
		c.fields[name] = "must not be blank"
		return false
	}
	return true
}

// maxSize — @Size(max = n).
//
// Длина в СИМВОЛАХ: логин «Шабданбек» в UTF-8 занимает вдвое больше байт, и проверка
// по байтам отвергала бы законные логины.
//
// ⚠️ Считается по СЫРОЙ строке, без обрезки: @Size в Java смотрит на длину как есть,
// обрезает только @NotBlank. Логин из 33 символов с пробелом на конце Java отвергает.
func (c *socialChecker) maxSize(name, value string, max int) {
	if utf8.RuneCountInString(value) > max {
		c.fields[name] = "size must be between 0 and " + strconv.Itoa(max)
	}
}

func (c *socialChecker) result() error {
	if len(c.fields) == 0 {
		return nil
	}
	return ValidationError{Fields: c.fields}
}

// Validate проверяет заявку в друзья.
func (r AddFriendRequest) Validate() error {
	c := newSocialChecker()
	if c.notBlank("username", r.Username) {
		c.maxSize("username", r.Username, 32)
	}
	return c.result()
}

// Validate проверяет приглашение за стол.
func (r TableInviteRequest) Validate() error {
	c := newSocialChecker()
	c.notBlank("tableId", r.TableID)
	return c.result()
}

// Validate проверяет подписку на уведомления.
func (r PushSubscribeRequest) Validate() error {
	c := newSocialChecker()
	c.notBlank("endpoint", r.Endpoint)
	c.notBlank("p256dh", r.P256dh)
	c.notBlank("auth", r.Auth)
	return c.result()
}

// Validate проверяет отписку.
func (r PushUnsubscribeRequest) Validate() error {
	c := newSocialChecker()
	c.notBlank("endpoint", r.Endpoint)
	return c.result()
}
