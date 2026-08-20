package application

import (
	"context"
	"errors"
	"strings"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// ProfileService — свой профиль: посмотреть и поправить.
//
// ⭐ Смена пароля живёт не здесь, а в AuthService.ChangePassword: она гасит все
// refresh-токены игрока, то есть принадлежит сессиям, а не профилю. Разводить её на два
// сценария значило бы однажды поменять пароль, забыв выкинуть чужие входы.
type ProfileService struct {
	users repository.Users
}

// NewProfileService собирает сценарии профиля.
func NewProfileService(users repository.Users) ProfileService {
	return ProfileService{users: users}
}

// Profile — профиль игрока, чей идентификатор пришёл в токене.
//
// Пользователя может уже не быть (учётку заблокировали, пока токен жив), поэтому
// ErrNotFound базы превращается в ErrUserNotFound сценария — транспорт отдаст 404.
func (s ProfileService) Profile(ctx context.Context, userID string) (repository.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.User{}, ErrUserNotFound
		}
		return repository.User{}, err
	}
	return user, nil
}

// UpdateProfile меняет имя за столом и мордочку.
//
// ⭐ Логин не меняется намеренно: по нему входят, и его знают соседи по столу как адрес
// приглашения. Имя же — то, что видно в игре, и его хочется поправить.
//
// В Java это два обращения к базе (найти, потом сохранить), здесь одно: update ... returning
// сам сообщает, что строки нет, и итог для клиента тот же — 404 USER_NOT_FOUND.
func (s ProfileService) UpdateProfile(ctx context.Context, userID, displayName string,
	avatar *string) (repository.User, error) {
	name, emoji := trimmedProfile(displayName, avatar)

	user, err := s.users.UpdateProfile(ctx, userID, name, emoji)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.User{}, ErrUserNotFound
		}
		return repository.User{}, err
	}
	return user, nil
}

// trimmedProfile повторяет обработку Java: имя обрезается по краям, а пустая или
// пробельная мордочка становится отсутствующей.
//
// ⚠️ Разница «пустая строка» и «нет значения» тут не косметическая: Jackson вырезает
// null-поля целиком, и профиль с avatar = "" отдал бы клиенту `"avatar":""` там, где
// Java не отдаёт ключа вовсе (MD-003).
func trimmedProfile(displayName string, avatar *string) (string, *string) {
	name := strings.TrimSpace(displayName)
	if avatar == nil {
		return name, nil
	}
	emoji := strings.TrimSpace(*avatar)
	if emoji == "" {
		return name, nil
	}
	return name, &emoji
}
