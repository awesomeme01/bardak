package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/awesomeme01/bardak/back-go/internal/auth"
)

type principalKey struct{}

// Principal — кто выполняет запрос.
type Principal struct {
	UserID   string
	Username string
}

// PrincipalFrom достаёт владельца запроса из контекста.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	value, ok := ctx.Value(principalKey{}).(Principal)
	return value, ok
}

// PublicPath — открыт ли путь без токена.
//
// ⚠️ Список повторяет SecurityConfig Java ДОСЛОВНО. Лишний открытый путь — это дыра,
// недостающий — сломанный экран, и оба всплывут не здесь.
func PublicPath(method, path string) bool {
	if method == http.MethodPost {
		switch path {
		case "/api/auth/register", "/api/auth/login", "/api/auth/refresh", "/api/auth/logout":
			return true
		}
	}
	if method != http.MethodGet {
		return false
	}
	switch {
	case path == "/api/health":
		return true
	case strings.HasPrefix(path, "/actuator/"), strings.HasPrefix(path, "/assets/"):
		return true
	// ⭐ Каталог наборов открыт: экран входа показывает карты, а это ссылки на картинки,
	// которые и так лежат в /assets. Требовать токен — запирать витрину, а не данные.
	case path == "/api/card-sets", strings.HasPrefix(path, "/api/card-sets/"):
		return true
	case path == "/api/table-themes":
		return true
	// ⭐ Приглашение по ссылке — единственная ручка столов без токена: по ссылке приходит
	// и тот, у кого учётки ещё нет.
	case strings.HasPrefix(path, "/api/tables/invite/"):
		return true
	}
	return false
}

// Authenticate — проверка токена перед маршрутизацией.
//
// ⚠️ Порядок важен и повторяет Java: сначала авторизация, потом маршрутизация. Поэтому
// неизвестный путь БЕЗ токена отвечает 401, а не 404 — сервер не сообщает, существует
// ли адрес, тому, кто не представился.
func Authenticate(tokens auth.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") || PublicPath(r.Method, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			header := r.Header.Get("Authorization")
			token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
			if !strings.HasPrefix(header, "Bearer ") || token == "" {
				unauthorized(w)
				return
			}
			claims, err := tokens.Parse(token)
			if err != nil {
				unauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), principalKey{},
				Principal{UserID: claims.UserID, Username: claims.Username})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// unauthorized — отказ по токену.
//
// ⚠️ Тело ПУСТОЕ и без формата ApiError: так отвечает Spring Security, у которого
// точка входа не переопределена. Клиент отличает такой отказ только по статусу,
// поля code там нет — и Go обязан вести себя так же.
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
}
