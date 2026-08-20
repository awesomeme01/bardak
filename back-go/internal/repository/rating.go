package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Рейтинг, сезоны и сырьё для статистики.
//
// ⭐ Десятичные колонки (`numeric(8,2)`) читаются как ТЕКСТ (`rating::text`), а не как
// float. Java отдаёт их через BigDecimal, и Jackson печатает ровно тот масштаб, что лежит
// в базе: `1012.50`, а не `1012.5`. Через float64 хвостовой ноль теряется молча, и ответы
// двух бэкендов разъезжаются в символе, которого никто не ищет.
//
// ⚠️ Сравнивать такие строки как строки нельзя — только разобрав в число; этим занимается
// слой сценариев, а не база.

// UserRating — строка таблицы user_rating.
type UserRating struct {
	UserID string
	// Rating — десятичное значение в том виде, в каком лежит в базе («1000.00»).
	Rating        string
	MatchesPlayed int
}

// LeaderRow — строка таблицы лидеров: рейтинг вместе с именем игрока.
type LeaderRow struct {
	UserID        string
	DisplayName   string
	Rating        string
	MatchesPlayed int
}

// RatingHistoryEntry — точка истории рейтинга: что было до матча и что стало после.
type RatingHistoryEntry struct {
	ID           int64
	MatchID      string
	RatingBefore string
	RatingAfter  string
	Place        int
	PlayersCount int
	CreatedAt    time.Time
}

// Season — сезон рейтинга. ClosedAt пуст у открытого сезона.
type Season struct {
	ID        string
	Name      string
	StartedAt time.Time
	ClosedAt  *time.Time
}

// IsOpen — сезон ещё идёт.
func (s Season) IsOpen() bool { return s.ClosedAt == nil }

// PlayerMatchOutcome — итог одного матча для одного игрока.
//
// ⚠️ Берутся только строки с непустым `place`: у отменённого матча итог так и остаётся
// пустым, и в статистику он не идёт (§5.3). Фильтр стоит в SQL, а не в цикле по всем
// матчам игрока, — это единственное место, где отмена отличается от завершения.
type PlayerMatchOutcome struct {
	Place int
	// LossType — степень проигрыша; пусто, если игрок не проиграл.
	LossType *string
	// Finished — матч дошёл до конца (status = FINISHED).
	Finished bool
	// DealsPlayed — раздач в матче; считается только у завершённых.
	DealsPlayed int
}

// Ratings — репозиторий рейтинга, сезонов и статистики.
//
// ⚠️ Один тип на три таблицы (`user_rating`, `rating_history`, `seasons`) плюс чтение
// итогов матчей: в Java это четыре Spring Data-интерфейса, но здесь они обслуживают ровно
// два сценария — рейтинг и статистику. Дробить на четыре структуры значило бы завести
// четыре конструктора ради одного и того же пула.
type Ratings struct{ pool *pgxpool.Pool }

// NewRatings собирает репозиторий поверх пула.
func NewRatings(pool *pgxpool.Pool) Ratings { return Ratings{pool: pool} }

// FindRating — текущий рейтинг игрока.
//
// ⚠️ ErrNotFound здесь НЕ ошибка сценария: у не игравшего ни разу строки нет вовсе,
// и вызывающий подставляет стартовое значение. Отдавать 404 по этому поводу — врать,
// что игрока не существует.
func (r Ratings) FindRating(ctx context.Context, userID string) (UserRating, error) {
	const query = `select user_id, rating::text, matches_played from user_rating where user_id = $1`

	var row UserRating
	err := r.pool.QueryRow(ctx, query, userID).Scan(&row.UserID, &row.Rating, &row.MatchesPlayed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserRating{}, ErrNotFound
		}
		return UserRating{}, fmt.Errorf("чтение рейтинга: %w", err)
	}
	return row, nil
}

// Leaderboard — вся таблица рейтинга по убыванию.
//
// ⚠️ Без пагинации и без лимита — как в Java. Индекса по rating нет, сортируется всё
// целиком; для узкого круга это десятки строк, но при росте сюда придёт лимит.
//
// ⭐ Имя игрока берётся ОДНИМ запросом через left join, а не отдельным чтением на строку:
// в Java здесь честный N+1 (`users.findById` в цикле). Ответ тот же — «—» для игрока,
// которого не нашли, — а запросов вместо N+1 ровно один.
func (r Ratings) Leaderboard(ctx context.Context) ([]LeaderRow, error) {
	const query = `select r.user_id, coalesce(u.display_name, '—'), r.rating::text, r.matches_played
	               from user_rating r
	               left join users u on u.id = r.user_id
	               order by r.rating desc`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("чтение таблицы лидеров: %w", err)
	}
	defer rows.Close()

	leaders := make([]LeaderRow, 0)
	for rows.Next() {
		var row LeaderRow
		if err := rows.Scan(&row.UserID, &row.DisplayName, &row.Rating, &row.MatchesPlayed); err != nil {
			return nil, fmt.Errorf("разбор строки лидера: %w", err)
		}
		leaders = append(leaders, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение таблицы лидеров: %w", err)
	}
	return leaders, nil
}

// HistoryOf — история рейтинга игрока, свежее сверху.
//
// ⚠️ Порядок — часть контракта дважды: по нему рисуется график И считается текущая серия
// побед. Java сортирует только по created_at; здесь добавлен `id desc` вторым ключом,
// потому что при совпадении времени порядок иначе выбирает база, и серия у двух бэкендов
// вышла бы разной на ровном месте. Пока времена различаются, выборки совпадают.
func (r Ratings) HistoryOf(ctx context.Context, userID string) ([]RatingHistoryEntry, error) {
	const query = `select id, match_id, rating_before::text, rating_after::text,
	                      place, players_count, created_at
	               from rating_history
	               where user_id = $1
	               order by created_at desc, id desc`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("чтение истории рейтинга: %w", err)
	}
	defer rows.Close()

	history := make([]RatingHistoryEntry, 0)
	for rows.Next() {
		var entry RatingHistoryEntry
		err := rows.Scan(&entry.ID, &entry.MatchID, &entry.RatingBefore, &entry.RatingAfter,
			&entry.Place, &entry.PlayersCount, &entry.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("разбор точки истории: %w", err)
		}
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение истории рейтинга: %w", err)
	}
	return history, nil
}

// AllSeasons — все сезоны, свежий сверху.
func (r Ratings) AllSeasons(ctx context.Context) ([]Season, error) {
	const query = `select id, name, started_at, closed_at from seasons order by started_at desc`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("чтение сезонов: %w", err)
	}
	defer rows.Close()

	seasons := make([]Season, 0)
	for rows.Next() {
		var season Season
		if err := rows.Scan(&season.ID, &season.Name, &season.StartedAt, &season.ClosedAt); err != nil {
			return nil, fmt.Errorf("разбор сезона: %w", err)
		}
		seasons = append(seasons, season)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение сезонов: %w", err)
	}
	return seasons, nil
}

// CloseAndOpenSeason закрывает открытый сезон и открывает следующий.
//
// ⭐ Одним действием и одной транзакцией: открытый сезон должен быть всегда, иначе матчи,
// сыгранные между закрытием и открытием, осели бы вне сезонов — и заметили бы это сильно
// позже, когда чинить уже нечего.
//
// ⚠️ Двумя ОТДЕЛЬНЫМИ операторами, а не одним `with ... insert`: у data-modifying CTE
// и вставки один и тот же command id, освобождение частичного уникального индекса вставке
// ещё не видно, и она упала бы на дубликате.
func (r Ratings) CloseAndOpenSeason(ctx context.Context, id, name string, now time.Time) (Season, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Season{}, fmt.Errorf("смена сезона: %w", err)
	}
	// Откат безвреден после успешного Commit — так закрывается путь «вернулись с ошибкой,
	// а транзакция осталась висеть».
	defer func() { _ = tx.Rollback(ctx) }()

	// ⚠️ Открытый сезон выбирается САМЫЙ ПОЗДНИЙ. Java берёт «первый попавшийся»
	// (findFirstByClosedAtIsNull без сортировки) в расчёте на то, что он ровно один,
	// но частичный индекс этого не гарантирует (см. комментарий к тесту): NULL-ы
	// в уникальном индексе различны, и второй открытый сезон завести ничто не мешает.
	const closeQuery = `update seasons set closed_at = $1
	                    where id = (select id from seasons where closed_at is null
	                                order by started_at desc, id desc limit 1)`
	if _, err := tx.Exec(ctx, closeQuery, now); err != nil {
		return Season{}, fmt.Errorf("закрытие сезона: %w", err)
	}

	// started_at пишет код, а не база: дату начала показывают сразу после открытия.
	const openQuery = `insert into seasons (id, name, started_at) values ($1, $2, $3)
	                   returning id, name, started_at, closed_at`
	var season Season
	err = tx.QueryRow(ctx, openQuery, id, name, now).
		Scan(&season.ID, &season.Name, &season.StartedAt, &season.ClosedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return Season{}, ErrConflict
		}
		return Season{}, fmt.Errorf("открытие сезона: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Season{}, fmt.Errorf("смена сезона: %w", err)
	}
	return season, nil
}

// OutcomesOf — итоги всех сыгранных матчей игрока.
//
// ⭐ Читается история матчей, а не отдельные счётчики: счётчик — это второе место, где
// живёт правда, он неизбежно разъезжается с историей, и потом не понять, какое из двух
// чисел настоящее.
//
// ⚠️ join к matches внутренний, хотя Java терпит отсутствие матча (`match != null`):
// `match_players.match_id` — внешний ключ с каскадом, строки без матча не бывает.
func (r Ratings) OutcomesOf(ctx context.Context, userID string) ([]PlayerMatchOutcome, error) {
	const query = `select p.place, p.loss_type, m.status = 'FINISHED', m.deals_played
	               from match_players p
	               join matches m on m.id = p.match_id
	               where p.user_id = $1 and p.place is not null`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("чтение итогов матчей: %w", err)
	}
	defer rows.Close()

	outcomes := make([]PlayerMatchOutcome, 0)
	for rows.Next() {
		var outcome PlayerMatchOutcome
		err := rows.Scan(&outcome.Place, &outcome.LossType, &outcome.Finished, &outcome.DealsPlayed)
		if err != nil {
			return nil, fmt.Errorf("разбор итога матча: %w", err)
		}
		outcomes = append(outcomes, outcome)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение итогов матчей: %w", err)
	}
	return outcomes, nil
}
