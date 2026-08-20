package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Рейтинг, сезоны и статистика против НАСТОЯЩЕГО Postgres со схемой Java.
//
// ⚠️ На подделке это проверять бессмысленно: половина поведения принадлежит базе —
// масштаб numeric(8,2), порядок сортировки, каскады и частичный уникальный индекс
// на открытом сезоне.

// ratingFixtureUser заводит игрока: на него ссылаются все остальные таблицы.
func ratingFixtureUser(t *testing.T, pool *pgxpool.Pool, displayName string) string {
	t.Helper()
	ctx := context.Background()
	id := uuid.NewString()
	_, err := pool.Exec(ctx,
		`insert into users (id, username, display_name, password_hash, status)
		 values ($1, $2, $3, 'hash', 'ACTIVE')`,
		id, "rating-"+id[:8], displayName)
	if err != nil {
		t.Fatalf("игрок не завёлся: %v", err)
	}
	return id
}

// ratingFixtureTable — стол нужен только затем, что на него ссылается матч.
func ratingFixtureTable(t *testing.T, pool *pgxpool.Pool, hostID string) string {
	t.Helper()
	ctx := context.Background()
	id := uuid.NewString()
	_, err := pool.Exec(ctx,
		`insert into game_tables (id, code, name, host_user_id, max_players, status, card_set_id, theme_id)
		 values ($1, $2, 'Стол', $3, 4, 'WAITING',
		         (select id from card_sets order by created_at limit 1),
		         (select id from table_themes order by created_at limit 1))`,
		id, id[:8], hostID)
	if err != nil {
		t.Fatalf("стол не завёлся: %v", err)
	}
	return id
}

// ratingFixtureMatch — матч с игроком за столом. place = nil означает отменённый матч:
// итог у него так и остаётся пустым.
func ratingFixtureMatch(t *testing.T, pool *pgxpool.Pool, tableID, userID, status string,
	dealsPlayed int, place *int, lossType *string) string {
	t.Helper()
	ctx := context.Background()
	matchID := uuid.NewString()
	_, err := pool.Exec(ctx,
		`insert into matches (id, table_id, status, players_count, deals_played, rng_seed, rules_snapshot)
		 values ($1, $2, $3, 4, $4, 42, '{}'::jsonb)`,
		matchID, tableID, status, dealsPlayed)
	if err != nil {
		t.Fatalf("матч не завёлся: %v", err)
	}
	_, err = pool.Exec(ctx,
		`insert into match_players (match_id, user_id, seat_no, place, loss_type)
		 values ($1, $2, 0, $3, $4)`,
		matchID, userID, place, lossType)
	if err != nil {
		t.Fatalf("участник матча не завёлся: %v", err)
	}
	return matchID
}

func ratingFixtureHistory(t *testing.T, pool *pgxpool.Pool, userID, matchID,
	before, after string, place int, createdAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`insert into rating_history (user_id, match_id, rating_before, rating_after,
		                             deviation_after, place, players_count, created_at)
		 values ($1, $2, $3, $4, 350, $5, 4, $6)`,
		userID, matchID, before, after, place, createdAt)
	if err != nil {
		t.Fatalf("точка истории не завелась: %v", err)
	}
}

// ⭐ Не игравший ни разу строки рейтинга не имеет — и это НЕ ошибка сценария.
// Отдавать по этому поводу «нет такого игрока» значило бы врать: игрок есть, матчей нет.
func TestRatingIsMissingUntilFirstMatch(t *testing.T) {
	pool := testDB(t)
	ratings := NewRatings(pool)

	userID := ratingFixtureUser(t, pool, "Новичок")

	_, err := ratings.FindRating(context.Background(), userID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ждали ErrNotFound для не игравшего, получили %v", err)
	}
}

// ⚠️ Масштаб numeric(8,2) обязан дожить до ответа: Java отдаёт `1012.50`, и через float64
// хвостовой ноль потерялся бы молча.
func TestRatingKeepsScaleFromDatabase(t *testing.T) {
	pool := testDB(t)
	ratings := NewRatings(pool)
	ctx := context.Background()

	userID := ratingFixtureUser(t, pool, "Игрок")
	_, err := pool.Exec(ctx,
		`insert into user_rating (user_id, rating, matches_played) values ($1, 1012.5, 7)`, userID)
	if err != nil {
		t.Fatal(err)
	}

	rating, err := ratings.FindRating(ctx, userID)
	if err != nil {
		t.Fatalf("чтение рейтинга: %v", err)
	}
	if rating.Rating != "1012.50" {
		t.Errorf("рейтинг %q, ждали \"1012.50\" — масштаб колонки обязан сохраниться", rating.Rating)
	}
	if rating.MatchesPlayed != 7 {
		t.Errorf("матчей %d, ждали 7", rating.MatchesPlayed)
	}
}

// Порядок истории — часть контракта: по нему рисуется график и считается серия побед.
func TestHistoryOfIsFreshestFirst(t *testing.T) {
	pool := testDB(t)
	ratings := NewRatings(pool)
	ctx := context.Background()

	userID := ratingFixtureUser(t, pool, "Историк")
	tableID := ratingFixtureTable(t, pool, userID)
	place := 1
	older := ratingFixtureMatch(t, pool, tableID, userID, "FINISHED", 3, &place, nil)
	newer := ratingFixtureMatch(t, pool, tableID, userID, "FINISHED", 4, &place, nil)

	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	ratingFixtureHistory(t, pool, userID, older, "1000.00", "1010.00", 1, base)
	ratingFixtureHistory(t, pool, userID, newer, "1010.00", "1025.50", 2, base.Add(time.Hour))

	history, err := ratings.HistoryOf(ctx, userID)
	if err != nil {
		t.Fatalf("чтение истории: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("точек истории %d, ждали 2", len(history))
	}
	if history[0].MatchID != newer {
		t.Errorf("сверху оказался матч %s, ждали свежий %s", history[0].MatchID, newer)
	}
	if history[0].RatingAfter != "1025.50" || history[0].Place != 2 || history[0].PlayersCount != 4 {
		t.Errorf("точка истории прочиталась как %+v", history[0])
	}
}

// Таблица лидеров читается целиком и сортируется по убыванию рейтинга.
func TestLeaderboardSortsByRatingDesc(t *testing.T) {
	pool := testDB(t)
	ratings := NewRatings(pool)
	ctx := context.Background()

	weak := ratingFixtureUser(t, pool, "Слабый")
	strong := ratingFixtureUser(t, pool, "Сильный")
	_, err := pool.Exec(ctx,
		`insert into user_rating (user_id, rating, matches_played) values ($1, 900.00, 3), ($2, 1500.00, 9)`,
		weak, strong)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := ratings.Leaderboard(ctx)
	if err != nil {
		t.Fatalf("чтение таблицы лидеров: %v", err)
	}

	strongAt, weakAt := -1, -1
	for i, row := range rows {
		switch row.UserID {
		case strong:
			strongAt = i
			if row.DisplayName != "Сильный" {
				t.Errorf("имя в таблице лидеров %q, ждали \"Сильный\"", row.DisplayName)
			}
			if row.Rating != "1500.00" {
				t.Errorf("рейтинг лидера %q, ждали \"1500.00\"", row.Rating)
			}
		case weak:
			weakAt = i
		}
	}
	if strongAt < 0 || weakAt < 0 {
		t.Fatalf("оба игрока обязаны быть в таблице: сильный %d, слабый %d", strongAt, weakAt)
	}
	if strongAt > weakAt {
		t.Errorf("сильный оказался ниже слабого (%d против %d) — сортировка не по убыванию", strongAt, weakAt)
	}
}

// ⭐ Закрытие и открытие — одно действие: открытый сезон должен быть всегда, иначе матчи
// между двумя вызовами осели бы вне сезонов.
func TestCloseAndOpenSeasonKeepsExactlyOneOpen(t *testing.T) {
	pool := testDB(t)
	ratings := NewRatings(pool)
	ctx := context.Background()

	var openBefore int
	if err := pool.QueryRow(ctx,
		`select count(*) from seasons where closed_at is null`).Scan(&openBefore); err != nil {
		t.Fatal(err)
	}
	if openBefore == 0 {
		t.Fatal("до смены сезона обязан быть открытый: его сеет миграция V5")
	}

	// Дата заведомо позже сеяного сезона: список сезонов идёт по убыванию начала,
	// и «свежий сверху» иначе зависел бы от того, в какую минуту запущен тест.
	now := time.Date(2099, 1, 1, 12, 0, 0, 0, time.UTC)
	season, err := ratings.CloseAndOpenSeason(ctx, uuid.NewString(), "Второй сезон", now)
	if err != nil {
		t.Fatalf("смена сезона: %v", err)
	}
	if !season.IsOpen() {
		t.Error("новый сезон обязан быть открытым")
	}
	if season.Name != "Второй сезон" || !season.StartedAt.Equal(now) {
		t.Errorf("новый сезон прочитался как %+v", season)
	}

	var openAfter int
	if err := pool.QueryRow(ctx,
		`select count(*) from seasons where closed_at is null`).Scan(&openAfter); err != nil {
		t.Fatal(err)
	}
	// ⚠️ Ровно один открытый держит КОД, а не база: частичный уникальный индекс
	// idx_seasons_single_open построен по closed_at, а NULL-ы в уникальном индексе
	// различны — второй открытый сезон он не запрещает.
	if openAfter != openBefore {
		t.Errorf("открытых сезонов стало %d, было %d — закрыт не ровно один", openAfter, openBefore)
	}

	seasons, err := ratings.AllSeasons(ctx)
	if err != nil {
		t.Fatalf("чтение сезонов: %v", err)
	}
	if len(seasons) == 0 || seasons[0].ID != season.ID {
		t.Error("свежий сезон обязан быть первым: список идёт по убыванию даты начала")
	}
}

// ⚠️ Отменённый матч в статистику не идёт: у него так и остаётся пустой place.
func TestOutcomesOfSkipCancelledMatches(t *testing.T) {
	pool := testDB(t)
	ratings := NewRatings(pool)
	ctx := context.Background()

	userID := ratingFixtureUser(t, pool, "Статистик")
	tableID := ratingFixtureTable(t, pool, userID)

	first := 1
	royal := "ROYAL"
	ratingFixtureMatch(t, pool, tableID, userID, "FINISHED", 5, &first, nil)
	third := 3
	ratingFixtureMatch(t, pool, tableID, userID, "FINISHED", 2, &third, &royal)
	// Отменённый: итога нет вовсе.
	ratingFixtureMatch(t, pool, tableID, userID, "ABORTED", 4, nil, nil)

	outcomes, err := ratings.OutcomesOf(ctx, userID)
	if err != nil {
		t.Fatalf("чтение итогов: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("итогов %d, ждали 2 — отменённый матч в счёт не идёт", len(outcomes))
	}
	deals := 0
	degrees := 0
	for _, outcome := range outcomes {
		if !outcome.Finished {
			t.Error("оба доигранных матча обязаны быть отмечены как завершённые")
		}
		deals += outcome.DealsPlayed
		if outcome.LossType != nil {
			degrees++
			if *outcome.LossType != royal {
				t.Errorf("степень проигрыша %q, ждали %q", *outcome.LossType, royal)
			}
		}
	}
	if deals != 7 {
		t.Errorf("раздач %d, ждали 7 (5 + 2, отменённый не в счёт)", deals)
	}
	if degrees != 1 {
		t.Errorf("проигрышей со степенью %d, ждали 1", degrees)
	}
}

// Матч без строки в match_players для этого игрока к нему и не относится.
func TestOutcomesOfSeesOnlyOwnMatches(t *testing.T) {
	pool := testDB(t)
	ratings := NewRatings(pool)
	ctx := context.Background()

	mine := ratingFixtureUser(t, pool, "Свой")
	stranger := ratingFixtureUser(t, pool, "Чужой")
	tableID := ratingFixtureTable(t, pool, mine)

	place := 2
	ratingFixtureMatch(t, pool, tableID, stranger, "FINISHED", 3, &place, nil)

	outcomes, err := ratings.OutcomesOf(ctx, mine)
	if err != nil {
		t.Fatalf("чтение итогов: %v", err)
	}
	if len(outcomes) != 0 {
		t.Errorf("итогов %d, ждали 0 — чужой матч к игроку не относится", len(outcomes))
	}
}
