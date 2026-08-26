package http

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimit — предел частоты запросов с одного адреса.
//
// ⭐ Ограничиваются ТОЛЬКО двери: вход, регистрация и выдача тикета к сокету. Игровые
// ручки не ограничиваются вовсе — за столом человек жмёт кнопки часто и законно, и предел
// там означал бы «сервер отказал в ходе», то есть испорченную партию.
//
// ⚠️ Тикет попал сюда не для симметрии. Клиент берёт его ПЕРЕД КАЖДЫМ подключением,
// включая переподключения, а отступ у него свой, клиентский: браузер с плохой связью
// (или страница, которую забыли закрыть) стучится в эту ручку сам по себе. Серверного
// предела до сих пор не было ни одного.
//
// ⚠️ Счётчик в памяти узла — как и присутствие (ADR-061). Со вторым узлом предел стал бы
// вдвое мягче; второго узла нет, и это осознанная плата, а не недосмотр.
type RateLimit struct {
	// Limit — сколько запросов разрешено за Window с одного адреса.
	Limit int
	// Window — окно, за которое считаются запросы.
	Window time.Duration
	// Now — часы; в тестах подменяются.
	Now func() time.Time

	Log *slog.Logger

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	count    int
	windowAt time.Time
}

// NewRateLimit собирает ограничитель.
func NewRateLimit(limit int, window time.Duration, now func() time.Time,
	log *slog.Logger) *RateLimit {
	if now == nil {
		now = time.Now
	}
	return &RateLimit{Limit: limit, Window: window, Now: now, Log: log,
		buckets: map[string]*bucket{}}
}

// guarded — пути, у которых есть предел частоты.
func guarded(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	switch path {
	case "/api/auth/login", "/api/auth/register", "/api/auth/ws-ticket":
		return true
	}
	return false
}

// Middleware отклоняет слишком частые запросы к дверям.
func (l *RateLimit) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l == nil || l.Limit <= 0 || !guarded(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		client := clientAddr(r)
		if l.allow(client) {
			next.ServeHTTP(w, r)
			return
		}

		if l.Log != nil {
			// Не Warn на каждый отказ: под перебором пароля это залило бы журнал
			// быстрее, чем кто-нибудь его прочитает.
			l.Log.Debug("предел частоты: запрос отклонён", "path", r.URL.Path, "client", client)
		}
		w.Header().Set("Retry-After", retryAfter(l.Window))
		WriteError(w, r, l.Log, NewFault(http.StatusTooManyRequests, "TOO_MANY_REQUESTS",
			"Слишком часто. Подожди немного и попробуй снова"))
	})
}

// allow — можно ли пропустить запрос с этого адреса.
func (l *RateLimit) allow(client string) bool {
	now := l.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	current, seen := l.buckets[client]
	if !seen || now.Sub(current.windowAt) >= l.Window {
		// ⚠️ Просроченные адреса выбрасываются здесь, а не по таймеру: иначе карта росла бы
		// по записи на каждый адрес, когда-либо постучавшийся в сервер.
		l.forgetStale(now)
		l.buckets[client] = &bucket{count: 1, windowAt: now}
		return true
	}
	if current.count >= l.Limit {
		return false
	}
	current.count++
	return true
}

func (l *RateLimit) forgetStale(now time.Time) {
	for client, item := range l.buckets {
		if now.Sub(item.windowAt) >= l.Window {
			delete(l.buckets, client)
		}
	}
}

// clientAddr — адрес запрашивающего.
//
// ⚠️ Берётся то, что положил RealIP: за Caddy настоящий адрес приходит заголовком,
// и без этого весь прод считался бы одним клиентом — то есть предел выключился бы
// ровно там, где он и нужен.
func clientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func retryAfter(window time.Duration) string {
	seconds := int(window.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
