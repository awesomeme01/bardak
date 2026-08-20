package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/awesomeme01/bardak/back-go/internal/application"
)

// AuthHandlers — обработчики входа, регистрации и профиля.
type AuthHandlers struct {
	Auth application.AuthService
	Log  *slog.Logger
}

// Routes вешает пути.
//
// ⭐ Пути и методы повторяют Java дословно: фронт не должен меняться вовсе.
func (h AuthHandlers) Routes(router chi.Router) {
	router.Post("/api/auth/register", h.register)
	router.Post("/api/auth/login", h.login)
	router.Post("/api/auth/refresh", h.refresh)
	router.Post("/api/auth/logout", h.logout)
}

func (h AuthHandlers) register(w http.ResponseWriter, r *http.Request) {
	var request RegisterRequest
	if err := DecodeJSON(r, &request); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := request.Validate(); err != nil {
		h.writeValidation(w, r, err)
		return
	}

	agent := userAgent(r)
	pair, err := h.Auth.Register(r.Context(), application.RegisterRequest{
		Username:    request.Username,
		DisplayName: request.DisplayName,
		Password:    request.Password,
		Email:       request.Email,
		InviteCode:  request.InviteCode,
		UserAgent:   agent,
	})
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}
	// ⚠️ 200, а не 201, и без заголовка Location — как в Java.
	WriteJSON(w, http.StatusOK, toPairView(pair))
}

func (h AuthHandlers) login(w http.ResponseWriter, r *http.Request) {
	var request LoginRequest
	if err := DecodeJSON(r, &request); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := request.Validate(); err != nil {
		h.writeValidation(w, r, err)
		return
	}

	pair, err := h.Auth.Login(r.Context(), request.Username, request.Password, userAgent(r))
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toPairView(pair))
}

func (h AuthHandlers) refresh(w http.ResponseWriter, r *http.Request) {
	var request RefreshRequest
	if err := DecodeJSON(r, &request); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := request.Validate(); err != nil {
		h.writeValidation(w, r, err)
		return
	}

	pair, err := h.Auth.Refresh(r.Context(), request.RefreshToken, userAgent(r))
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toPairView(pair))
}

// logout гасит токен «по возможности» и всегда отвечает 204.
//
// ⭐ Ручка открыта без токена намеренно — разбор в SecurityConfig Java: позвать её можно,
// лишь предъявив сам refresh-токен, а кто им владеет, уже может выпустить себе access.
func (h AuthHandlers) logout(w http.ResponseWriter, r *http.Request) {
	var request RefreshRequest
	if err := DecodeJSON(r, &request); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := h.Auth.Logout(r.Context(), request.RefreshToken); err != nil {
		WriteError(w, r, h.Log, ErrInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h AuthHandlers) writeValidation(w http.ResponseWriter, r *http.Request, err error) {
	var validation ValidationError
	if errors.As(err, &validation) {
		WriteError(w, r, h.Log, validation.AsFault())
		return
	}
	WriteError(w, r, h.Log, ErrBadRequest)
}

// writeAuthError переводит ошибку сценария в код и статус.
//
// ⚠️ Статусы взяты по месту, а не по коду: INVALID_CREDENTIALS — 401 при входе
// и 400 при смене пароля, и одна константа на код сломала бы контракт.
func (h AuthHandlers) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidInvite):
		WriteError(w, r, h.Log, ErrInvalidInvite)
	case errors.Is(err, application.ErrUsernameTaken):
		WriteError(w, r, h.Log, ErrUsernameTaken)
	case errors.Is(err, application.ErrInvalidCredentials):
		WriteError(w, r, h.Log, NewFault(http.StatusUnauthorized,
			"INVALID_CREDENTIALS", "Неверный логин или пароль"))
	case errors.Is(err, application.ErrRefreshInvalid):
		WriteError(w, r, h.Log, NewFault(http.StatusUnauthorized,
			"REFRESH_TOKEN_INVALID", "Сессия истекла, войди заново"))
	case errors.Is(err, application.ErrUserNotFound):
		WriteError(w, r, h.Log, ErrUserNotFound)
	default:
		WriteError(w, r, h.Log, ErrInternal)
	}
}

func toPairView(pair application.TokenPair) TokenPairView {
	return TokenPairView{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
		User:         ToUserView(pair.User),
	}
}

// userAgent — заголовок необязателен; пустой не сохраняем.
func userAgent(r *http.Request) *string {
	value := r.Header.Get("User-Agent")
	if value == "" {
		return nil
	}
	return &value
}
