package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Общий контейнер Postgres на весь пакет тестов.
//
// ⚠️ Именно ОДИН, а не по контейнеру на тест. В Java этот же выбор был сделан не сразу:
// пятнадцать интеграционных классов поднимали по своему контейнеру, исчерпывали память
// Docker, и прогон падал плавающе. Лечение — общий контейнер; заодно время упало почти вдвое.
//
// ⭐ Схему накатываем ТЕМИ ЖЕ миграциями Flyway, что и Java: своих миграций у Go нет
// (MD-004), а проверять репозитории надо против настоящей схемы, а не против её пересказа.
var (
	testPoolOnce sync.Once
	testPool     *pgxpool.Pool
	testPoolErr  error
)

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	testPoolOnce.Do(startTestDB)
	if testPoolErr != nil {
		t.Skipf("Postgres для тестов недоступен: %v", testPoolErr)
	}
	return testPool
}

func startTestDB() {
	ctx := context.Background()

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

// applyMigrations накатывает SQL-файлы Flyway по возрастанию версии.
//
// ⚠️ Читаются файлы Java-бэкенда, а не копия: копия разъехалась бы с оригиналом молча,
// и тесты Go проверяли бы схему, которой нет.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir, err := migrationsDir()
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(dir, "V*.sql"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("миграции не найдены в %s", dir)
	}
	sortByVersion(files)

	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(file), err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(file), err)
		}
	}

	// Healthcheck Go читает историю Flyway (MD-004), поэтому таблица должна существовать
	// и в тестовой базе — иначе проверка «жив ли сервер» упала бы только в бою.
	_, err = pool.Exec(ctx, `create table if not exists flyway_schema_history (
		installed_rank int primary key, version varchar(50), description varchar(200),
		type varchar(20), script varchar(1000), checksum int, installed_by varchar(100),
		installed_on timestamp default now(), execution_time int, success boolean)`)
	return err
}

func migrationsDir() (string, error) {
	// Тест запускается из своего каталога; поднимаемся до корня репозитория.
	here, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := here; dir != "/" && dir != ""; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "back-bardak", "src", "main", "resources", "db", "migration")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("каталог миграций не найден от %s вверх", here)
}

// sortByVersion — V2 идёт раньше V10, поэтому сравниваем ЧИСЛА, а не строки.
func sortByVersion(files []string) {
	version := func(path string) int {
		base := filepath.Base(path)
		number := 0
		for i := 1; i < len(base) && base[i] >= '0' && base[i] <= '9'; i++ {
			number = number*10 + int(base[i]-'0')
		}
		return number
	}
	for i := 1; i < len(files); i++ {
		for j := i; j > 0 && version(files[j]) < version(files[j-1]); j-- {
			files[j], files[j-1] = files[j-1], files[j]
		}
	}
}
