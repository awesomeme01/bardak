// Package migrate — владение схемой базы.
//
// ⭐ Схема принадлежит Go. До этого ею владел Flyway из Java (MD-004), и это было
// правильно ровно до тех пор, пока Java оставалась тем, что поднимает базу: два мигратора
// на одну схему — это две служебные таблицы, каждая со своим представлением о накатанном,
// и первое же расхождение чинится руками в проде.
//
// ⚠️ Своего мигратора здесь всего сотня строк, и это осознанно: задача — «примени файлы
// по порядку один раз», а бэкенд мы сокращаем, а не расширяем (ADR-061). Настоящая
// сложность не в применении файлов, а в ДВУХ случаях, каждый из которых описан ниже:
// пустая база и база, которую уже накатил Flyway.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed all:sql
var files embed.FS

// lockID — идентификатор advisory-блокировки на время миграции.
//
// ⚠️ Блокировка нужна даже при одном узле: два процесса рядом бывают всегда — старый
// ещё не остановился, новый уже поднялся. Без неё оба увидели бы пустую таблицу версий
// и накатили одно и то же дважды.
const lockID int64 = 8088_2026

// Migration — один файл схемы.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Apply приводит базу к текущей схеме и возвращает, сколько миграций накатано.
func Apply(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (int, error) {
	migrations, err := Load()
	if err != nil {
		return 0, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("соединение для миграций: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `select pg_advisory_lock($1)`, lockID); err != nil {
		return 0, fmt.Errorf("блокировка миграций: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `select pg_advisory_unlock($1)`, lockID) }()

	if _, err := conn.Exec(ctx, `create table if not exists schema_migrations (
		version int primary key,
		name text not null,
		applied_at timestamptz not null default now())`); err != nil {
		return 0, fmt.Errorf("таблица версий: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return 0, err
	}

	// ⭐ База, которую накатил Flyway, НЕ накатывается заново: файлы объявляются
	// применёнными без выполнения. Иначе первый же запуск Go на живой базе попытался бы
	// создать таблицы, которые там уже есть, и упал бы — ровно в момент переключения.
	if len(applied) == 0 {
		adopted, err := adoptFlywayIfPresent(ctx, conn, migrations, log)
		if err != nil {
			return 0, err
		}
		if adopted {
			return len(migrations), nil
		}
	}

	count := len(applied)
	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}
		if err := applyOne(ctx, conn, migration); err != nil {
			return count, err
		}
		count++
		if log != nil {
			log.Info("миграция накатана", "version", migration.Version, "name", migration.Name)
		}
	}
	return count, nil
}

// applyOne накатывает одну миграцию.
//
// ⚠️ Файл и отметка о нём — ОДНА транзакция. Иначе упавший посередине запуск оставил бы
// половину схемы без записи о ней, и следующий старт начал бы с того же файла заново.
func applyOne(ctx context.Context, conn *pgxpool.Conn, migration Migration) error {
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			return fmt.Errorf("миграция %04d_%s: %w", migration.Version, migration.Name, err)
		}
		_, err := tx.Exec(ctx, `insert into schema_migrations (version, name) values ($1, $2)`,
			migration.Version, migration.Name)
		return err
	})
}

// adoptFlywayIfPresent объявляет схему Flyway своей, ничего не выполняя.
func adoptFlywayIfPresent(ctx context.Context, conn *pgxpool.Conn, migrations []Migration,
	log *slog.Logger) (bool, error) {
	var flywayRows int
	err := conn.QueryRow(ctx, `select count(*) from flyway_schema_history where success`).Scan(&flywayRows)
	if err != nil {
		// Таблицы нет — база чистая, накатываем обычным порядком.
		return false, nil
	}
	if flywayRows == 0 {
		return false, nil
	}

	// ⚠️ Число файлов у нас и число успешных строк у Flyway обязаны совпасть. Не совпали —
	// значит схема в базе не та, из которой копировались файлы, и молча «усыновлять» её
	// нельзя: разойдётся не таблица версий, а сами таблицы.
	if flywayRows != len(migrations) {
		return false, fmt.Errorf(
			"в базе %d миграций Flyway, а файлов схемы %d: усыновлять такую базу вслепую нельзя",
			flywayRows, len(migrations))
	}

	return true, pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		for _, migration := range migrations {
			if _, err := tx.Exec(ctx,
				`insert into schema_migrations (version, name) values ($1, $2)
				 on conflict (version) do nothing`, migration.Version, migration.Name); err != nil {
				return err
			}
		}
		if log != nil {
			log.Info("схема Flyway принята как своя, миграции не выполнялись", "count", len(migrations))
		}
		return nil
	})
}

func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[int]bool, error) {
	rows, err := conn.Query(ctx, `select version from schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("чтение версий: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

// Load читает встроенные файлы схемы по возрастанию версии.
func Load() ([]Migration, error) {
	entries, err := files.ReadDir("sql")
	if err != nil {
		return nil, fmt.Errorf("каталог схемы: %w", err)
	}

	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, name, err := parseName(entry.Name())
		if err != nil {
			return nil, err
		}
		sql, err := files.ReadFile(path.Join("sql", entry.Name()))
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, Migration{Version: version, Name: name, SQL: string(sql)})
	}
	if len(migrations) == 0 {
		return nil, fmt.Errorf("файлы схемы не найдены")
	}

	// ⚠️ Сортировка по ЧИСЛУ, а не по имени: иначе 10 встанет раньше 2, и таблицы
	// начнут создаваться после того, как на них сошлются.
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

func parseName(fileName string) (int, string, error) {
	parts := strings.SplitN(strings.TrimSuffix(fileName, ".sql"), "_", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("имя файла схемы %q не в форме 0001_описание.sql", fileName)
	}
	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("имя файла схемы %q: номер не разобран", fileName)
	}
	return version, parts[1], nil
}
