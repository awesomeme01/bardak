// Package application — сценарии использования и границы транзакций.
//
// ⭐ Слой знает про домен и репозитории, но НЕ знает про HTTP и WebSocket: одно и то же
// действие приходит и по REST, и по сокету, и повторять его в двух транспортах — верный
// способ развести поведение.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/auth"
	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// Ошибки сценариев. Транспорт превращает их в коды и статусы, а сам сценарий про HTTP
// ничего не знает.
var (
	ErrInvalidInvite      = errors.New("неверный код приглашения")
	ErrUsernameTaken      = errors.New("логин уже занят")
	ErrInvalidCredentials = errors.New("неверные учётные данные")
	ErrRefreshInvalid     = errors.New("refresh-токен непригоден")
	ErrUserNotFound       = errors.New("пользователь не найден")
)

// TokenPair — то, что получает клиент после входа.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	User         repository.User
}

// AuthService — регистрация, вход, обновление пары и выход.
type AuthService struct {
	users      repository.Users
	refresh    repository.RefreshTokens
	tokens     auth.TokenService
	inviteOK   func(string) bool
	refreshTTL time.Duration
	accessTTL  time.Duration
	now        func() time.Time
	log        *slog.Logger
}

// NewAuthService собирает сценарии входа.
func NewAuthService(users repository.Users, refresh repository.RefreshTokens,
	tokens auth.TokenService, inviteOK func(string) bool,
	accessTTL, refreshTTL time.Duration, now func() time.Time, log *slog.Logger) AuthService {
	if now == nil {
		now = time.Now
	}
	return AuthService{
		users: users, refresh: refresh, tokens: tokens, inviteOK: inviteOK,
		accessTTL: accessTTL, refreshTTL: refreshTTL, now: now, log: log,
	}
}

// RegisterRequest — что нужно для заведения учётки.
type RegisterRequest struct {
	Username    string
	DisplayName string
	Password    string
	Email       *string
	InviteCode  string
	UserAgent   *string
}

// Register заводит игрока.
//
// ⚠️ Порядок проверок важен и повторяет Java: СНАЧАЛА код приглашения, потом занятость
// логина. Иначе по ответу «логин занят» можно было бы перебирать логины, не зная кода.
func (s AuthService) Register(ctx context.Context, request RegisterRequest) (TokenPair, error) {
	if !s.inviteOK(request.InviteCode) {
		return TokenPair{}, ErrInvalidInvite
	}

	taken, err := s.users.ExistsByUsernameIgnoreCase(ctx, request.Username)
	if err != nil {
		return TokenPair{}, err
	}
	if taken {
		return TokenPair{}, ErrUsernameTaken
	}

	hash, err := auth.HashPassword(request.Password)
	if err != nil {
		return TokenPair{}, err
	}

	user, err := s.users.Insert(ctx, repository.User{
		ID:           uuid.NewString(),
		Username:     request.Username,
		DisplayName:  request.DisplayName,
		Email:        request.Email,
		PasswordHash: hash,
	})
	if err != nil {
		// ⚠️ Проверка выше и вставка не атомарны: двое, регистрирующих один логин
		// одновременно, обе её проходят, и второй упирается в уникальный индекс.
		// Правду держит база, а не проверка.
		if errors.Is(err, repository.ErrConflict) {
			return TokenPair{}, ErrUsernameTaken
		}
		return TokenPair{}, err
	}
	return s.issuePair(ctx, user, request.UserAgent)
}

// Login — вход по логину и паролю.
//
// ⚠️ Логин ищется БЕЗ учёта регистра, а ответ на «нет такого игрока» и «неверный пароль»
// один и тот же: иначе по разнице ответов перебирают существующие логины.
func (s AuthService) Login(ctx context.Context, username, password string,
	userAgent *string) (TokenPair, error) {
	user, err := s.users.FindByUsernameIgnoreCase(ctx, username)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return TokenPair{}, ErrInvalidCredentials
		}
		return TokenPair{}, err
	}
	if !auth.CheckPassword(user.PasswordHash, password) {
		return TokenPair{}, ErrInvalidCredentials
	}
	return s.issuePair(ctx, user, userAgent)
}

// Refresh — обновление пары с ротацией.
//
// ⭐ Предъявленный УЖЕ ОТОЗВАННЫЙ токен — признак кражи: отзывается вся серия игрока.
// Логика простая: честный клиент отозванный токен не предъявит, он его заменил. Значит
// им завладел кто-то ещё, и обе стороны надо выкинуть.
func (s AuthService) Refresh(ctx context.Context, token string, userAgent *string) (TokenPair, error) {
	stored, err := s.refresh.FindByHash(ctx, auth.HashRefreshToken(token))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return TokenPair{}, ErrRefreshInvalid
		}
		return TokenPair{}, err
	}

	now := s.now()
	if stored.RevokedAt != nil {
		if s.log != nil {
			s.log.Warn("предъявлен отозванный refresh-токен — гашу всю серию",
				"user", stored.UserID)
		}
		// ⚠️ Отзыв серии не должен откатиться вместе с отказом: гасим ДО возврата ошибки.
		if err := s.refresh.RevokeAllOf(ctx, stored.UserID, now); err != nil && s.log != nil {
			s.log.Error("не удалось погасить серию", "user", stored.UserID, "err", err)
		}
		return TokenPair{}, ErrRefreshInvalid
	}
	if !stored.ExpiresAt.After(now) {
		return TokenPair{}, ErrRefreshInvalid
	}

	user, err := s.users.FindByID(ctx, stored.UserID)
	if err != nil {
		return TokenPair{}, ErrRefreshInvalid
	}
	if err := s.refresh.Revoke(ctx, stored.ID, now); err != nil {
		return TokenPair{}, err
	}
	return s.issuePair(ctx, user, userAgent)
}

// Logout гасит предъявленный токен.
//
// ⭐ «По возможности»: токен не найден или уже негоден — ничего не делаем и отвечаем
// успехом. Выход, который отказывает, оставляет игрока в подвешенном состоянии.
func (s AuthService) Logout(ctx context.Context, token string) error {
	stored, err := s.refresh.FindByHash(ctx, auth.HashRefreshToken(token))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	return s.refresh.Revoke(ctx, stored.ID, s.now())
}

// ChangePassword меняет пароль и гасит все refresh-токены игрока.
//
// ⚠️ Старый пароль обязателен ДАЖЕ при живом токене: смена пароля — это защита от того,
// кто уже сидит за твоим устройством, и знание токена таким доказательством не является.
func (s AuthService) ChangePassword(ctx context.Context, userID, current, next string) error {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	if !auth.CheckPassword(user.PasswordHash, current) {
		return ErrInvalidCredentials
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePasswordHash(ctx, userID, hash); err != nil {
		return err
	}
	return s.refresh.RevokeAllOf(ctx, userID, s.now())
}

func (s AuthService) issuePair(ctx context.Context, user repository.User,
	userAgent *string) (TokenPair, error) {
	access, ttl, err := s.tokens.Issue(user.ID, user.Username, user.DisplayName)
	if err != nil {
		return TokenPair{}, err
	}

	refreshToken, err := auth.NewRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}
	err = s.refresh.Insert(ctx, repository.RefreshToken{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		TokenHash: auth.HashRefreshToken(refreshToken),
		ExpiresAt: s.now().Add(s.refreshTTL),
		UserAgent: trimUserAgent(userAgent),
	})
	if err != nil {
		return TokenPair{}, fmt.Errorf("выдача пары: %w", err)
	}

	return TokenPair{
		AccessToken:  access,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(ttl.Seconds()),
		User:         user,
	}, nil
}

// trimUserAgent — колонка ограничена 255 символами, а заголовок приходит из сети
// и бывает любой длины.
func trimUserAgent(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > 255 {
		trimmed = trimmed[:255]
	}
	return &trimmed
}
