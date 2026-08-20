package http

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// ⚠️ Кодировщик Go по умолчанию экранирует <, > и & в <, >, &,
// а Jackson их не трогает. Глазами оба ответа выглядят одинаково, а побайтное
// сравнение расходится — и искать такое долго.
func TestJSONDoesNotEscapeHTML(t *testing.T) {
	recorder := httptest.NewRecorder()

	WriteJSON(recorder, 200, map[string]string{
		"displayName": "Тим & Кот <Шаба>",
	})

	body := recorder.Body.String()
	// Ищем именно ESCAPE-последовательности, а не сами символы: символы как раз
	// и должны остаться на месте.
	for _, escaped := range []string{`\u0026`, `\u003c`, `\u003e`} {
		if strings.Contains(body, escaped) {
			t.Errorf("найдено экранирование %s, а Jackson его не делает: %s", escaped, body)
		}
	}
	if !strings.Contains(body, "Тим & Кот <Шаба>") {
		t.Errorf("имя не дошло как есть: %s", body)
	}
}

// Кириллица не должна превращаться в \uXXXX: Jackson отдаёт её как есть в UTF-8.
func TestCyrillicSurvivesAsIs(t *testing.T) {
	recorder := httptest.NewRecorder()

	WriteJSON(recorder, 200, APIError{Code: "USERNAME_TAKEN", Message: "Логин уже занят"})

	if !strings.Contains(recorder.Body.String(), "Логин уже занят") {
		t.Errorf("кириллица уехала в escape-последовательности: %s", recorder.Body.String())
	}
}
