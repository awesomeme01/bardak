package http

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Предел частоты на дверях сервера.
//
// ⭐ Проверяется не «счётчик считает», а ГРАНИЦА: что ограничены двери, что игровые
// ручки не тронуты, и что предел отпускает через окно. Ограничитель, задевший ход
// за столом, испортил бы партию, а не защитил сервер.

type movingTime struct {
	mu  sync.Mutex
	now time.Time
}

func (m *movingTime) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

func (m *movingTime) advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}

func limitedHandler(limit int, window time.Duration) (http.Handler, *movingTime, *int) {
	clock := &movingTime{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)}
	passed := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		passed++
		w.WriteHeader(http.StatusOK)
	})
	return NewRateLimit(limit, window, clock.Now, nil).Middleware(next), clock, &passed
}

func call(t *testing.T, handler http.Handler, method, path, addr string) int {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = addr + ":54321"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestRateLimitStopsAFloodOfLogins(t *testing.T) {
	handler, _, passed := limitedHandler(3, time.Minute)

	for i := 0; i < 3; i++ {
		if code := call(t, handler, http.MethodPost, "/api/auth/login", "10.0.0.1"); code != http.StatusOK {
			t.Fatalf("попытка %d отклонена кодом %d, а предел ещё не достигнут", i+1, code)
		}
	}
	if code := call(t, handler, http.MethodPost, "/api/auth/login", "10.0.0.1"); code != http.StatusTooManyRequests {
		t.Fatalf("четвёртый вход при пределе в три получил %d", code)
	}
	if *passed != 3 {
		t.Fatalf("до обработчика дошло %d запросов вместо 3", *passed)
	}
}

func TestRateLimitReleasesAfterTheWindow(t *testing.T) {
	handler, clock, _ := limitedHandler(1, time.Minute)
	call(t, handler, http.MethodPost, "/api/auth/login", "10.0.0.1")
	if code := call(t, handler, http.MethodPost, "/api/auth/login", "10.0.0.1"); code != http.StatusTooManyRequests {
		t.Fatalf("предел не сработал: %d", code)
	}

	clock.advance(time.Minute + time.Second)

	// ⚠️ Предел, который не отпускает, — это отказ в обслуживании собственному игроку:
	// человек, забывший пароль, оказался бы заперт навсегда.
	if code := call(t, handler, http.MethodPost, "/api/auth/login", "10.0.0.1"); code != http.StatusOK {
		t.Fatalf("после окна вход всё ещё закрыт: %d", code)
	}
}

func TestRateLimitCountsEachAddressSeparately(t *testing.T) {
	handler, _, _ := limitedHandler(1, time.Minute)
	call(t, handler, http.MethodPost, "/api/auth/login", "10.0.0.1")

	// Общий счётчик означал бы, что один перебиратель пароля закрывает вход всем.
	if code := call(t, handler, http.MethodPost, "/api/auth/login", "10.0.0.2"); code != http.StatusOK {
		t.Fatalf("сосед по интернету получил чужой отказ: %d", code)
	}
}

func TestRateLimitLeavesTheGameAlone(t *testing.T) {
	handler, _, _ := limitedHandler(1, time.Minute)

	// ⚠️ За столом человек жмёт кнопки часто и законно. Предел здесь означал бы
	// «сервер отказал в ходе», то есть испорченную партию.
	for i := 0; i < 20; i++ {
		if code := call(t, handler, http.MethodPost, "/api/tables", "10.0.0.1"); code != http.StatusOK {
			t.Fatalf("игровая ручка ограничена на запросе %d: %d", i+1, code)
		}
	}
	for i := 0; i < 20; i++ {
		if code := call(t, handler, http.MethodGet, "/api/matches", "10.0.0.1"); code != http.StatusOK {
			t.Fatalf("чтение истории ограничено на запросе %d: %d", i+1, code)
		}
	}
}

func TestRateLimitGuardsTheSocketTicket(t *testing.T) {
	handler, _, _ := limitedHandler(2, time.Minute)
	call(t, handler, http.MethodPost, "/api/auth/ws-ticket", "10.0.0.1")
	call(t, handler, http.MethodPost, "/api/auth/ws-ticket", "10.0.0.1")

	// ⭐ Тикет берётся перед КАЖДЫМ подключением, включая переподключения: страница,
	// которую забыли закрыть, стучится сюда сама по себе, и отступ у неё клиентский.
	if code := call(t, handler, http.MethodPost, "/api/auth/ws-ticket", "10.0.0.1"); code != http.StatusTooManyRequests {
		t.Fatalf("тикет к сокету не ограничен: %d", code)
	}
}

func TestRateLimitSaysWhenToComeBack(t *testing.T) {
	handler, _, _ := limitedHandler(1, 30*time.Second)
	call(t, handler, http.MethodPost, "/api/auth/login", "10.0.0.1")

	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	request.RemoteAddr = "10.0.0.1:54321"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q: клиенту не сказано, когда возвращаться", got)
	}
}

func TestRateLimitForgetsAddressesItNoLongerHolds(t *testing.T) {
	clock := &movingTime{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)}
	limit := NewRateLimit(5, time.Minute, clock.Now, nil)
	handler := limit.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for i := 0; i < 50; i++ {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		request.RemoteAddr = "10.0.0." + string(rune('a'+i%26)) + ":1"
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}

	clock.advance(2 * time.Minute)
	call(t, handler, http.MethodPost, "/api/auth/login", "10.0.0.1")

	// ⚠️ Карта, которая только растёт, — это память, которая кончится позже и не там,
	// где виновник.
	limit.mu.Lock()
	defer limit.mu.Unlock()
	if len(limit.buckets) != 1 {
		t.Fatalf("после окна в памяти осталось %d адресов", len(limit.buckets))
	}
}
