package repository

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/awesomeme01/bardak/back-go/internal/testsupport"
)

// testDB — настоящий Postgres со схемой Java. Обвязка общая с остальными тестами:
// две копии подъёма контейнера разъехались бы, и половина прогона проверяла бы
// схему, которой нет.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testsupport.Postgres(t)
}
