package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/awesomeme01/bardak/back-go/internal/application"
)

// ProfileHandlers — свой профиль: посмотреть, поправить, сменить пароль.
type ProfileHandlers struct {
	Profile application.ProfileService
	// Auth нужен ровно ради смены пароля: она гасит refresh-токены, а ими владеет
	// сценарий входа, а не профиля.
	Auth application.AuthService
	Log  *slog.Logger
}

// Routes вешает пути.
//
// ⭐ Пути и методы повторяют Java дословно: фронт не должен меняться вовсе.
// Все три ручки — за токеном, игрок правит ТОЛЬКО свой профиль: идентификатор берётся
// из токена, а не из тела или пути, поэтому чужой профиль через них не достать.
func (h ProfileHandlers) Routes(router chi.Router) {
	router.Get("/api/profile", h.profile)
	router.Patch("/api/profile", h.update)
	router.Post("/api/profile/password", h.changePassword)
}

func (h ProfileHandlers) profile(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	user, err := h.Profile.Profile(r.Context(), principal.UserID)
	if err != nil {
		h.writeProfileError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, ToUserView(user))
}

func (h ProfileHandlers) update(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	var request UpdateProfileRequest
	if err := DecodeJSON(r, &request); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := request.Validate(); err != nil {
		h.writeValidation(w, r, err)
		return
	}

	user, err := h.Profile.UpdateProfile(r.Context(), principal.UserID,
		request.DisplayName, request.Avatar)
	if err != nil {
		h.writeProfileError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, ToUserView(user))
}

// changePassword меняет пароль и гасит все входы игрока.
//
// ⚠️ Старый пароль спрашивается ДАЖЕ при живом токене: иначе оставленная открытой вкладка
// позволяет запереть владельца снаружи. Текущий access-токен доживает до своего exp —
// это осознанное окно, и оно такое же в Java.
func (h ProfileHandlers) changePassword(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	var request ChangePasswordRequest
	if err := DecodeJSON(r, &request); err != nil {
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}
	if err := request.Validate(); err != nil {
		h.writeValidation(w, r, err)
		return
	}

	err := h.Auth.ChangePassword(r.Context(), principal.UserID,
		request.CurrentPassword, request.NewPassword)
	if err != nil {
		h.writeProfileError(w, r, err)
		return
	}
	// 204 и пустое тело — как в Java.
	w.WriteHeader(http.StatusNoContent)
}

func (h ProfileHandlers) writeValidation(w http.ResponseWriter, r *http.Request, err error) {
	var validation ValidationError
	if errors.As(err, &validation) {
		WriteError(w, r, h.Log, validation.AsFault())
		return
	}
	WriteError(w, r, h.Log, ErrBadRequest)
}

// writeProfileError переводит ошибку сценария в код и статус.
//
// ⚠️ INVALID_CREDENTIALS здесь — 400, а НЕ 401, в отличие от входа: при смене пароля
// клиент уже представился, отказ относится к полю формы, а не к сессии. 401 заставил бы
// фронт выкинуть игрока на экран входа из-за опечатки в старом пароле.
func (h ProfileHandlers) writeProfileError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidCredentials):
		WriteError(w, r, h.Log, NewFault(http.StatusBadRequest,
			"INVALID_CREDENTIALS", "Текущий пароль не подошёл"))
	case errors.Is(err, application.ErrUserNotFound):
		WriteError(w, r, h.Log, ErrUserNotFound)
	default:
		WriteError(w, r, h.Log, ErrInternal)
	}
}
