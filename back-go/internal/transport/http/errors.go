// Package http — REST-слой: обработчики, DTO и разбор ошибок.
//
// ⚠️ Формат ответа обязан совпадать с Java ДОСЛОВНО, включая отсутствие полей.
// Jackson вырезает null-поля глобально, поэтому «поле есть со значением null» и «поля нет»
// — разные ответы (MD-003). В Go это значит: nullable-поля — указатели с omitempty,
// а то, что Java отдаёт всегда, — обычные значения БЕЗ omitempty.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/awesomeme01/bardak/back-go/internal/observability"
)

// APIError — единый формат ошибки.
//
// ⚠️ В Java он помечен NON_EMPTY, а не non_null: пустая карта details и пустая строка
// тоже вырезаются. Поэтому обычная ошибка на проводе — ровно {code, message, traceId}.
type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	TraceID string         `json:"traceId,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// Fault — ошибка, которую видно клиенту: код, статус и человеческое сообщение.
type Fault struct {
	Status  int
	Code    string
	Message string
	// Details — разбор по полям для ошибки валидации. Пустая карта не сериализуется:
	// в Java ApiError помечен NON_EMPTY.
	Details map[string]any
}

func (f Fault) Error() string { return f.Code + ": " + f.Message }

// NewFault собирает ошибку для клиента.
func NewFault(status int, code, message string) Fault {
	return Fault{Status: status, Code: code, Message: message}
}

// Часто встречающиеся отказы. Собраны здесь, чтобы код и статус не разъезжались
// между обработчиками.
//
// ⚠️ Один и тот же код в Java живёт с РАЗНЫМИ статусами: INVALID_CREDENTIALS — 401
// при входе и 400 при смене пароля; NOT_FRIENDS — 403 в приглашении и 404 в поиске пары.
// Поэтому статус берётся по месту, а не из общей константы на код.
var (
	ErrUsernameTaken = NewFault(http.StatusConflict, "USERNAME_TAKEN", "Логин уже занят")
	ErrInvalidInvite = NewFault(http.StatusForbidden, "INVALID_INVITE_CODE", "Неверный код приглашения")
	ErrUserNotFound  = NewFault(http.StatusNotFound, "USER_NOT_FOUND", "Пользователь не найден")
	ErrBadRequest    = NewFault(http.StatusBadRequest, "BAD_REQUEST", "Запрос не разобран")
	ErrNotFound      = NewFault(http.StatusNotFound, "NOT_FOUND", "Такого адреса нет")
	ErrInternal      = NewFault(http.StatusInternalServerError, "INTERNAL_ERROR", "Что-то пошло не так")
)

// WriteJSON отдаёт тело с нужным статусом.
//
// ⚠️ Экранирование HTML ВЫКЛЮЧЕНО намеренно. Кодировщик Go по умолчанию превращает
// `<`, `>` и `&` в \u003c, \u003e и \u0026, а Jackson их не трогает. На сегодняшних
// данных это незаметно, но первое же имя за столом с амперсандом дало бы расхождение
// в побайтном сравнении — и искали бы его долго, потому что глазами оба ответа
// выглядят одинаково.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(body)
}

// WriteError отдаёт ошибку в общем формате.
//
// ⭐ traceId тот же, что в логе: разбор жалобы «у меня не работает» начинается с него,
// иначе приходится угадывать, какая из тысячи строк лога — та самая.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, fault Fault) {
	traceID := observability.TraceFrom(r.Context())
	if log != nil {
		if fault.Status >= 500 {
			log.Error("необработанная ошибка", "trace", traceID, "path", r.URL.Path, "code", fault.Code)
		} else {
			log.Info("запрос отклонён", "trace", traceID, "code", fault.Code, "message", fault.Message)
		}
	}
	WriteJSON(w, fault.Status, APIError{
		Code: fault.Code, Message: fault.Message, TraceID: traceID, Details: fault.Details,
	})
}

// DecodeJSON разбирает тело запроса.
//
// ⚠️ Битое тело — это 400, а не 500. В Java этого обработчика не было, и невалидный JSON
// проваливался в «что-то пошло не так»: клиент не мог отличить свою ошибку от поломки
// сервера. Исправлено в Java, повторено здесь.
func DecodeJSON(r *http.Request, into any) error {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		return ErrBadRequest
	}
	return nil
}
