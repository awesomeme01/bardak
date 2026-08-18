// Команда server — точка входа Go-бэкенда «Бардак».
//
// ⚠️ Пока это каркас этапа 2: конфигурация, журнал, пул к базе, healthcheck и корректное
// завершение. Игровая часть переносится этапами 3–6 и подключается сюда же.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/awesomeme01/bardak/back-go/internal/config"
	"github.com/awesomeme01/bardak/back-go/internal/observability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "запуск не удался:", err)
		os.Exit(1)
	}
}

func run() error {
	log := observability.NewLogger()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("конфигурация: %w", err)
	}

	// ⭐ Контекст рвётся по SIGTERM и SIGINT: под Docker приходит первый, из терминала —
	// второй, и обрабатывать надо оба, иначе «работает у меня» расходится с продакшеном.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("пул соединений: %w", err)
	}
	defer pool.Close()

	// Проверяем связь сразу: падать на старте честнее, чем отвечать «UP» без базы.
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	err = pool.Ping(pingCtx)
	cancelPing()
	if err != nil {
		return fmt.Errorf("база недоступна: %w", err)
	}

	router := chi.NewRouter()
	router.Use(middleware.RealIP)
	router.Use(traceMiddleware)
	router.Use(middleware.Recoverer)

	router.Method(http.MethodGet, "/api/health", observability.Health{
		DB:      poolAdapter{pool},
		Version: version(),
		Log:     log,
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("сервер поднят", "port", cfg.Port, "autoMove", cfg.AutoMove)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return fmt.Errorf("сервер упал: %w", err)
	case <-ctx.Done():
		log.Info("получен сигнал, завершаюсь")
	}

	// ⚠️ Отдельный контекст: тот, что уже отменён сигналом, немедленно прервал бы и само
	// завершение — соединения не успели бы закрыться, а игроки получили бы обрыв вместо
	// корректного прощания.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("завершение затянулось: %w", err)
	}
	log.Info("остановлен чисто")
	return nil
}

// traceMiddleware вешает на запрос опознавательный номер и кладёт его в контекст.
//
// ⚠️ Формат — 8 шестнадцатеричных символов, как у Java (`UUID.randomUUID().toString()
// .substring(0, 8)`). Номер уходит клиенту в поле traceId ответа об ошибке, попадает
// в скриншоты жалоб и в тесты; чужой формат сделал бы старые и новые логи несравнимыми.
func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(observability.WithTrace(r.Context(), newTraceID())))
	})
}

func newTraceID() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Случайность кончиться не может, но если бы могла — лучше пустой номер,
		// чем упавший запрос: traceId нужен для разбора, а не для работы.
		return ""
	}
	return hex.EncodeToString(buf[:])
}

// version — версия сборки; подставляется линковщиком, иначе «dev», как в Java.
var buildVersion string

func version() string {
	if buildVersion == "" {
		return "dev"
	}
	return buildVersion
}

// poolAdapter сводит pgxpool к узкому интерфейсу health-проверки: она не должна знать
// про пул больше, чем «дай одну строку».
type poolAdapter struct{ pool *pgxpool.Pool }

func (a poolAdapter) QueryRow(ctx context.Context, sql string, args ...any) observability.Row {
	return a.pool.QueryRow(ctx, sql, args...)
}
