package migrate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/awesomeme01/bardak/back-go/internal/migrate"
)

// Владение схемой.
//
// ⭐ Проверяются ДВА случая, и второй важнее первого: чистая база (накатить всё)
// и база, которую уже накатил Flyway (не накатывать НИЧЕГО, но признать своей).
// Второй случай — это и есть переключение на живой базе, и ошибиться в нём можно
// ровно один раз.

// testDB — подключение к базе для миграционных тестов.
//
// ⚠️ Своя база, а не общая из testsupport: та уже накатана, а здесь проверяется
// именно ПРИМЕНЕНИЕ к пустой. Без адреса тест пропускается.
func testDB(t *testing.T, dbName string) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("BARDAK_TEST_DB_ADMIN_URL")
	if base == "" {
		t.Skip("BARDAK_TEST_DB_ADMIN_URL не задан: тестам миграций нужна своя база")
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatalf("подключение к postgres: %v", err)
	}
	defer admin.Close()

	if _, err := admin.Exec(ctx, `drop database if exists `+dbName+` with (force)`); err != nil {
		t.Fatalf("удаление базы: %v", err)
	}
	if _, err := admin.Exec(ctx, `create database `+dbName); err != nil {
		t.Fatalf("создание базы: %v", err)
	}

	pool, err := pgxpool.New(ctx, replaceDB(base, dbName))
	if err != nil {
		t.Fatalf("подключение к %s: %v", dbName, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func replaceDB(url, dbName string) string {
	slash := len(url) - 1
	for ; slash >= 0 && url[slash] != '/'; slash-- {
	}
	return url[:slash+1] + dbName + "?sslmode=disable"
}

func tableCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(),
		`select count(*) from information_schema.tables where table_schema = 'public'`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestCleanDatabaseGetsTheWholeSchema(t *testing.T) {
	pool := testDB(t, "bardak_migrate_clean")

	applied, err := migrate.Apply(context.Background(), pool, nil)
	if err != nil {
		t.Fatalf("миграции не накатились: %v", err)
	}

	if applied != 11 {
		t.Fatalf("накатано %d миграций, ждали 11", applied)
	}
	// 18 таблиц схемы плюс своя таблица версий.
	if got := tableCount(t, pool); got != 19 {
		t.Fatalf("в схеме %d таблиц, ждали 19", got)
	}
}

func TestSecondRunChangesNothing(t *testing.T) {
	pool := testDB(t, "bardak_migrate_twice")
	ctx := context.Background()
	if _, err := migrate.Apply(ctx, pool, nil); err != nil {
		t.Fatal(err)
	}
	before := tableCount(t, pool)

	// ⚠️ Повторный старт сервера — обычное дело, и он не должен ни падать, ни трогать схему.
	applied, err := migrate.Apply(ctx, pool, nil)
	if err != nil {
		t.Fatalf("повторный прогон упал: %v", err)
	}
	if applied != 11 || tableCount(t, pool) != before {
		t.Fatalf("повторный прогон изменил схему: миграций %d, таблиц было %d, стало %d",
			applied, before, tableCount(t, pool))
	}
}

// ⭐ Главный тест этого файла: база, накатанная Flyway, принимается как своя, и НИ ОДИН
// файл при этом не выполняется. Выполнись хоть один — старт упал бы на «таблица уже есть»
// ровно в момент переключения, на живых людях.
func TestFlywayDatabaseIsAdoptedWithoutRunningAnything(t *testing.T) {
	pool := testDB(t, "bardak_migrate_adopt")
	ctx := context.Background()
	applyFlywayLike(t, pool)
	before := tableCount(t, pool)

	applied, err := migrate.Apply(ctx, pool, nil)
	if err != nil {
		t.Fatalf("живая база не принята: %v", err)
	}

	if applied != 11 {
		t.Fatalf("принято %d миграций, ждали 11", applied)
	}
	// Таблицы те же плюс schema_migrations: ни одна миграция не выполнялась заново.
	if got := tableCount(t, pool); got != before+1 {
		t.Fatalf("таблиц было %d, стало %d: значит что-то выполнилось", before, got)
	}
	var versions int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 11 {
		t.Fatalf("в таблице версий %d строк, ждали 11", versions)
	}
}

// ⚠️ Чужая база с другим числом миграций не усыновляется вслепую: разойдётся не таблица
// версий, а сами таблицы, и узнаем мы об этом по отсутствующей колонке в бою.
func TestUnfamiliarFlywayDatabaseIsRefused(t *testing.T) {
	pool := testDB(t, "bardak_migrate_stranger")
	ctx := context.Background()
	applyFlywayLike(t, pool)
	if _, err := pool.Exec(ctx, `insert into flyway_schema_history
		(installed_rank, version, description, type, script, checksum, installed_by, execution_time, success)
		values (99, '99', 'чужая', 'SQL', 'V99__чужая.sql', 0, 'кто-то', 1, true)`); err != nil {
		t.Fatal(err)
	}

	if _, err := migrate.Apply(ctx, pool, nil); err == nil {
		t.Fatal("база с чужой миграцией принята молча")
	}
}

// applyFlywayLike накатывает схему так, как это сделала бы Java: те же файлы плюс
// её служебная таблица.
func applyFlywayLike(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	migrations, err := migrate.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `create table flyway_schema_history (
		installed_rank int primary key, version varchar(50), description varchar(200),
		type varchar(20), script varchar(1000), checksum int, installed_by varchar(100),
		installed_on timestamp default now(), execution_time int, success boolean)`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if _, err := pool.Exec(ctx, migration.SQL); err != nil {
			t.Fatalf("миграция %d: %v", migration.Version, err)
		}
		if _, err := pool.Exec(ctx, `insert into flyway_schema_history
			(installed_rank, version, description, type, script, checksum, installed_by, execution_time, success)
			values ($1, $2, $3, 'SQL', $4, 0, 'flyway', 1, true)`,
			migration.Version, itoa(migration.Version), migration.Name,
			"V"+itoa(migration.Version)+"__"+migration.Name+".sql"); err != nil {
			t.Fatal(err)
		}
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for ; value > 0; value /= 10 {
		digits = string(rune('0'+value%10)) + digits
	}
	return digits
}

// ⚠️ Пока Java жива, файлы схемы обязаны совпадать с её миграциями ПОБАЙТНО. Копия,
// разъехавшаяся с оригиналом, — это два разных представления об одной базе, и заметно
// это станет по отсутствующей колонке в бою, а не здесь.
func TestSchemaFilesMatchTheJavaOriginals(t *testing.T) {
	javaDir := filepath.Join("..", "..", "..", "back-bardak", "src", "main", "resources", "db", "migration")
	if _, err := os.Stat(javaDir); err != nil {
		t.Skip("эталон Java уже удалён — сверять не с чем")
	}

	migrations, err := migrate.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		original := filepath.Join(javaDir, "V"+itoa(migration.Version)+"__"+migration.Name+".sql")
		expected, err := os.ReadFile(original)
		if err != nil {
			t.Fatalf("не прочитать оригинал %s: %v", original, err)
		}
		if string(expected) != migration.SQL {
			t.Fatalf("миграция %d разошлась с оригиналом Java", migration.Version)
		}
	}
}
