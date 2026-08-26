// Package testsupport — общая обвязка тестов: настоящий Postgres с настоящей схемой.
//
// ⚠️ Пакет нужен ТОЛЬКО тестам, но не может лежать в _test.go: его просят и репозитории,
// и сквозной прогон сервера, а тестовые файлы одного пакета другому недоступны.
package testsupport

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/awesomeme01/bardak/back-go/internal/migrate"
)

// Общий контейнер Postgres на весь прогон.
//
// ⚠️ Именно ОДИН, а не по контейнеру на тест. В Java этот же выбор был сделан не сразу:
// пятнадцать интеграционных классов поднимали по своему контейнеру, исчерпывали память
// Docker, и прогон падал плавающе. Лечение — общий контейнер; заодно время упало почти вдвое.
//
// ⭐ Схему накатывает ТОТ ЖЕ мигратор, что и в бою (MD-006). Проверять надо против
// настоящей схемы и настоящим способом: свой обход файлов в тестах означал бы, что
// мигратор, поднимающий прод, не проверен вообще.
var (
	testPoolOnce sync.Once
	testPool     *pgxpool.Pool
	testPoolErr  error
)

// Postgres — пул к тестовой базе. Докера нет — тест пропускается, а не падает.
//
// ⭐ `BARDAK_TEST_DB_URL` подставляет ГОТОВУЮ базу вместо контейнера. Заведено не для
// удобства: Docker на машине умеет ломаться целиком (демон отвечает 500, VM недоступна),
// и без этой лазейки в такой день не проверить вообще ничего, что касается базы —
// а пропуск в сводке `go test` выглядит как `ok`.
func Postgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	testPoolOnce.Do(startTestDB)
	if testPoolErr != nil {
		t.Skipf("Postgres для тестов недоступен: %v", testPoolErr)
	}
	return testPool
}

func startTestDB() {
	ctx := context.Background()

	if url := os.Getenv("BARDAK_TEST_DB_URL"); url != "" {
		pool, err := pgxpool.New(ctx, url)
		if err != nil {
			testPoolErr = fmt.Errorf("BARDAK_TEST_DB_URL: %w", err)
			return
		}
		if err := applyMigrations(ctx, pool); err != nil {
			testPoolErr = fmt.Errorf("миграции: %w", err)
			return
		}
		testPool = pool
		return
	}

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("bardak"),
		postgres.WithUsername("bardak"),
		postgres.WithPassword("bardak"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(90*time.Second)),
	)
	if err != nil {
		testPoolErr = fmt.Errorf("контейнер не поднялся: %w", err)
		return
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		testPoolErr = fmt.Errorf("строка подключения: %w", err)
		return
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		testPoolErr = fmt.Errorf("пул: %w", err)
		return
	}
	if err := applyMigrations(ctx, pool); err != nil {
		testPoolErr = fmt.Errorf("миграции: %w", err)
		return
	}
	testPool = pool
}

// applyMigrations приводит тестовую базу к текущей схеме.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := migrate.Apply(ctx, pool, nil)
	return err
}
