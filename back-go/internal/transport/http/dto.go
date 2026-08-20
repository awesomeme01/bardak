package http

import "github.com/awesomeme01/bardak/back-go/internal/repository"

// DTO наружу. Формы повторяют Java дословно.
//
// ⚠️ Nullable-поля — УКАЗАТЕЛИ с omitempty, остальные — значения БЕЗ omitempty.
// Java вырезает только null; omitempty в Go вырезал бы ещё и нули с пустыми строками,
// и `{"matchesPlayed":0}` превратилось бы в `{}` (MD-003).

// UserView — профиль игрока.
type UserView struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl,omitempty"`
	Avatar      *string `json:"avatar,omitempty"`
}

// ToUserView переводит строку базы в ответ.
func ToUserView(user repository.User) UserView {
	return UserView{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		AvatarURL:   user.AvatarURL,
		Avatar:      user.Avatar,
	}
}

// TokenPairView — ответ на вход, регистрацию и обновление пары.
type TokenPairView struct {
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	ExpiresIn    int64    `json:"expiresIn"`
	User         UserView `json:"user"`
}

// RegisterRequest — тело регистрации.
type RegisterRequest struct {
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	Password    string  `json:"password"`
	Email       *string `json:"email"`
	InviteCode  string  `json:"inviteCode"`
}

// LoginRequest — тело входа.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RefreshRequest — тело обновления пары и выхода.
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// UpdateProfileRequest — что можно поменять в профиле.
//
// Логин не меняется: по нему входят.
type UpdateProfileRequest struct {
	DisplayName string  `json:"displayName"`
	Avatar      *string `json:"avatar"`
}

// ChangePasswordRequest — смена пароля.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}
