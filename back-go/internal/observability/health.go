package observability

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Pinger — то немногое, что health знает о базе.
type Pinger interface {
	QueryRow(ctx context.Context, sql string, args ...any) Row
}

// Row — одна строка ответа; ровно столько, сколько нужно проверке.
type Row interface {
	Scan(dest ...any) error
}

// Health отвечает на GET /api/health.
//
// ⚠️ Форма ответа повторяет Java ДОСЛОВНО: {status, version, db:{status, version,
// migrations}, ts}. По этой ручке смотрят «жив ли сервер» и люди, и скрипты
// (tools/smoke/run.sh ждёт именно её), поэтому менять форму нельзя даже к лучшему.
//
// ⭐ Число миграций читается из flyway_schema_history, а не из таблицы goose. В окне
// отката схемой владеет Java: она её накатывала, она же откатит, если Go придётся снять.
// Go читает чужую служебную таблицу сознательно — чтобы обе версии отвечали одинаково.
type Health struct {
	DB      Pinger
	Version string
	Log     *slog.Logger
}

func (h Health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	version := h.Version
	if version == "" {
		version = "dev"
	}

	body := map[string]any{
		"status":  "UP",
		"version": version,
		"db":      h.checkDatabase(r.Context()),
		"ts":      time.Now().UnixMilli(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil && h.Log != nil {
		h.Log.Warn("не удалось отдать health", "err", err)
	}
}

func (h Health) checkDatabase(ctx context.Context) map[string]any {
	if h.DB == nil {
		return map[string]any{"status": "DOWN", "error": "NoDataSource"}
	}

	var pgVersion string
	if err := h.DB.QueryRow(ctx, "select version()").Scan(&pgVersion); err != nil {
		if h.Log != nil {
			h.Log.Warn("проверка БД не прошла", "err", err)
		}
		return map[string]any{"status": "DOWN", "error": "QueryFailed"}
	}

	var migrations int
	if err := h.DB.QueryRow(ctx,
		"select count(*) from flyway_schema_history where success = true").Scan(&migrations); err != nil {
		if h.Log != nil {
			h.Log.Warn("не прочитать историю миграций", "err", err)
		}
		return map[string]any{"status": "DOWN", "error": "QueryFailed"}
	}

	// Java режет строку версии по первой запятой — оставляем ту же короткую форму.
	short := pgVersion
	if comma := strings.Index(pgVersion, ","); comma > 0 {
		short = pgVersion[:comma]
	}

	return map[string]any{
		"status":     "UP",
		"version":    short,
		"migrations": migrations,
	}
}
