package http

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Проверка полей запроса.
//
// ⚠️ Границы взяты из аннотаций Java и должны совпадать до символа: расхождение
// означает, что один бэкенд принимает то, что другой отвергает, и это всплывёт
// не здесь, а у игрока при переключении.

// ValidationError — отказ по проверке полей.
//
// Отдаётся как 400 VALIDATION_FAILED с картой «поле → что не так», как в Java.
type ValidationError struct {
	Fields map[string]any
}

func (e ValidationError) Error() string { return "проверка полей не пройдена" }

// AsFault превращает в ответ клиенту.
//
// ⚠️ `details` с разбором по полям — часть контракта, а не отладочная роскошь: экран
// подсвечивает по нему конкретное поле. Без него игрок видит «проверьте заполнение»
// и гадает, какое именно.
func (e ValidationError) AsFault() Fault {
	return Fault{
		Status:  http.StatusBadRequest,
		Code:    "VALIDATION_FAILED",
		Message: "Проверьте заполнение полей",
		Details: e.Fields,
	}
}

type checker struct{ fields map[string]any }

func newChecker() *checker { return &checker{fields: map[string]any{}} }

// size — длина в СИМВОЛАХ, а не в байтах: имя «Шабдан» в UTF-8 занимает вдвое больше
// байт, и проверка по байтам отвергала бы законные имена.
//
// ⚠️ Тексты сообщений — ДОСЛОВНО как у Bean Validation в Java, вплоть до английского.
// Игроку они не видны (фронт показывает только message), но differential-проверка
// сверяет их посимвольно, и «улучшенная» формулировка светилась бы различием вечно.
// Улучшать надо в Java первой, как и всё остальное.
func (c *checker) size(name, value string, min, max int) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		c.fields[name] = "must not be blank"
		return
	}
	length := utf8.RuneCountInString(trimmed)
	if length < min || length > max {
		c.fields[name] = fmt.Sprintf("size must be between %d and %d", min, max)
	}
}

func (c *checker) result() error {
	if len(c.fields) == 0 {
		return nil
	}
	return ValidationError{Fields: c.fields}
}

// Validate проверяет тело регистрации.
func (r RegisterRequest) Validate() error {
	c := newChecker()
	c.size("username", r.Username, 3, 32)
	c.size("displayName", r.DisplayName, 2, 64)
	c.size("password", r.Password, 8, 128)
	c.size("inviteCode", r.InviteCode, 1, 128)
	if r.Email != nil && utf8.RuneCountInString(*r.Email) > 255 {
		c.fields["email"] = "size must be between 0 and 255"
	}
	return c.result()
}

// Validate проверяет тело входа.
func (r LoginRequest) Validate() error {
	c := newChecker()
	c.size("username", r.Username, 1, 32)
	c.size("password", r.Password, 1, 128)
	return c.result()
}

// Validate проверяет тело обновления пары.
func (r RefreshRequest) Validate() error {
	c := newChecker()
	c.size("refreshToken", r.RefreshToken, 1, 512)
	return c.result()
}

// Validate проверяет правку профиля.
func (r UpdateProfileRequest) Validate() error {
	c := newChecker()
	c.size("displayName", r.DisplayName, 2, 64)
	if r.Avatar != nil && utf8.RuneCountInString(*r.Avatar) > 8 {
		c.fields["avatar"] = "size must be between 0 and 8"
	}
	return c.result()
}

// Validate проверяет смену пароля.
func (r ChangePasswordRequest) Validate() error {
	c := newChecker()
	c.size("currentPassword", r.CurrentPassword, 1, 128)
	c.size("newPassword", r.NewPassword, 8, 128)
	return c.result()
}
