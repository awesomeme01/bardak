// Команда server — точка входа Go-бэкенда «Бардак».
//
// Здесь только жизненный цикл процесса: конфигурация, журнал, пул к базе, сигналы
// и корректное завершение. Сборка самого бэкенда живёт в internal/server — её же
// поднимает сквозной тест.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/awesomeme01/bardak/back-go/internal/config"
	"github.com/awesomeme01/bardak/back-go/internal/observability"
	"github.com/awesomeme01/bardak/back-go/internal/push"
	"github.com/awesomeme01/bardak/back-go/internal/server"
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

	// ⭐ Кривые ключи VAPID — поломка, и узнать о ней надо ПРИ СТАРТЕ, а не при первом
	// же ходе: иначе сервер поднимется здоровым, а уведомления окажутся мёртвыми ровно
	// тогда, когда понадобятся. Так же ведёт себя Java.
	if err := push.CheckKeys(push.Options{
		PublicKey: cfg.VAPIDPublic, PrivateKey: cfg.VAPIDPrivate,
	}); err != nil {
		return fmt.Errorf("уведомления: %w", err)
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

	handler, shutdownTables := server.Build(ctx, cfg, pool, log)
	defer shutdownTables()

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("сервер поднят", "port", cfg.Port, "autoMove", cfg.AutoMove,
			"version", server.Version())
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("завершение затянулось: %w", err)
	}
	log.Info("остановлен чисто")
	return nil
}
