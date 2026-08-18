// Package observability — логи, healthcheck и опознание запросов.
package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// NewLogger собирает журнал.
//
// ⭐ JSON в продакшене, человекочитаемый текст в разработке. Настройка одна и та же
// строка окружения, потому что выбор формата — это выбор читателя: машина или человек.
func NewLogger() *slog.Logger {
	level := slog.LevelInfo
	if raw, ok := os.LookupEnv("BARDAK_LOG_LEVEL"); ok {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}

	options := &slog.HandlerOptions{Level: level}
	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, options)
	if strings.EqualFold(os.Getenv("BARDAK_LOG_FORMAT"), "text") {
		handler = slog.NewTextHandler(os.Stdout, options)
	}
	return slog.New(handler)
}

// traceKey — ключ опознавательного номера запроса в контексте.
type traceKey struct{}

// WithTrace кладёт номер запроса в контекст.
//
// ⭐ Тот же номер уходит клиенту в поле traceId ответа об ошибке. Без этого разбор жалобы
// «у меня не работает» начинается с угадывания, какая из тысячи строк лога — та самая.
func WithTrace(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceKey{}, traceID)
}

// TraceFrom достаёт номер запроса; пусто, если его не клали.
func TraceFrom(ctx context.Context) string {
	if value, ok := ctx.Value(traceKey{}).(string); ok {
		return value
	}
	return ""
}
