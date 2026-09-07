// Команда recompute-rating — пересчёт всего рейтинга по истории матчей.
//
// ⭐ Ради этого `rating_history` и хранится подробно (`07-rating-system.md`): формула
// рейтинга меняется, а история — нет. Команда проигрывает ВСЕ засчитанные матчи заново
// в хронологическом порядке тем же application.Recalculate, что и живая игра, и
// переписывает `rating_before`/`rating_after` везде, где они лежат.
//
// ⚠️ Ущерб восстанавливается из того, что уже записано в `match_players`: ступень навесов
// и степень проигрыша. Ничего досчитывать по логу событий не нужно — и это же значит, что
// оффлайн-партии пересчитываются наравне с онлайновыми.
//
// ⚠️ Порядок — по `finished_at`, а не по `started_at`: матч, начатый раньше, мог
// закончиться позже, а рейтинг двигает именно окончание.
//
// Запуск:
//
//	go run ./cmd/recompute-rating -dsn postgres://... [-dry]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/awesomeme01/bardak/back-go/internal/application"
	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// seat — участник матча в том виде, в каком он лежит в базе.
type seat struct {
	userID     string
	navesLevel *string
	lossType   *string
	damage     float64
}

// match — засчитанный матч: участники и когда он кончился.
type match struct {
	id       string
	seasonID *string
	finished time.Time
	seats    []seat
}

func main() {
	dsn := flag.String("dsn", os.Getenv("BARDAK_DSN"), "строка подключения к Postgres")
	dry := flag.Bool("dry", false, "посчитать и показать итог, но ничего не писать")
	flag.Parse()

	if *dsn == "" {
		log.Fatal("нужен -dsn")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		log.Fatalf("подключение к базе: %v", err)
	}
	defer pool.Close()

	matches, err := load(ctx, pool)
	if err != nil {
		log.Fatalf("чтение истории: %v", err)
	}
	log.Printf("матчей к пересчёту: %d", len(matches))

	if err := recompute(ctx, pool, matches, *dry); err != nil {
		log.Fatalf("пересчёт: %v", err)
	}
	if err := report(ctx, pool); err != nil {
		log.Fatalf("отчёт: %v", err)
	}
}

// load читает засчитанные матчи в хронологическом порядке.
//
// ⚠️ Признак «матч засчитан» — СТРОКА В rating_history, а не статус матча. Так же
// считает и сам движок (repository.MatchResults.AlreadyCounted), и это не придирка:
// на проде нашёлся матч со статусом ABORTED, у которого рейтинг уже был посчитан —
// его отменили после подведения итога. Фильтруй по статусу — и такой матч выпал бы
// из пересчёта, оставив свои строки истории с рейтингом по старой формуле, а
// matches_played разошёлся бы с числом строк.
func load(ctx context.Context, pool *pgxpool.Pool) ([]match, error) {
	const query = `select m.id, coalesce(m.finished_at, m.started_at),
	                      (select h.season_id from rating_history h
	                        where h.match_id = m.id limit 1),
	                      p.user_id, p.naves_level, p.loss_type
	               from matches m
	               join match_players p on p.match_id = m.id
	               where p.place is not null
	                 and exists (select 1 from rating_history h where h.match_id = m.id)
	               order by coalesce(m.finished_at, m.started_at), m.id, p.seat_no`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scale := game.FullNavesScale()
	byID := map[string]*match{}
	order := make([]*match, 0, 512)
	for rows.Next() {
		var (
			matchID  string
			finished time.Time
			seasonID *string
			current  seat
		)
		if err := rows.Scan(&matchID, &finished, &seasonID, &current.userID,
			&current.navesLevel, &current.lossType); err != nil {
			return nil, err
		}

		current.damage = damageOf(scale, current.navesLevel, current.lossType)
		found, ok := byID[matchID]
		if !ok {
			found = &match{id: matchID, seasonID: seasonID, finished: finished}
			byID[matchID] = found
			order = append(order, found)
		}
		found.seats = append(found.seats, current)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]match, 0, len(order))
	for _, item := range order {
		result = append(result, *item)
	}
	return result, nil
}

// damageOf восстанавливает ущерб из записанных ступени и степени.
func damageOf(scale game.NavesScale, level, loss *string) float64 {
	if level == nil {
		return application.DamageOf(scale, game.NoNaves, game.NoLossDegree)
	}
	if *level == application.JokerNavesLevel {
		return application.DamageOf(scale, scale.JokerLevel(), degreeOf(loss))
	}
	for index, rank := range scale.Ranks {
		if rank.Code() == *level {
			return application.DamageOf(scale, index, game.NoLossDegree)
		}
	}
	// Ступень не с нашей шкалы — считаем как джокер без степени: это заведомо хуже
	// любой карты и лучше любого проигрыша, то есть самое безопасное из предположений.
	return application.DamageOf(scale, scale.JokerLevel(), game.NoLossDegree)
}

func degreeOf(loss *string) game.LossDegree {
	if loss == nil {
		return game.NoLossDegree
	}
	for _, degree := range []game.LossDegree{game.LossRoyal, game.LossSuperMegaSuck,
		game.LossSuperMegaFail, game.LossSuperFail, game.LossFail} {
		if degree.String() == *loss {
			return degree
		}
	}
	return game.NoLossDegree
}

// recompute проигрывает историю заново и переписывает рейтинг везде, где он лежит.
//
// ⭐ ОДНОЙ транзакцией на весь пересчёт: половина истории по новой формуле и половина
// по старой — это состояние, из которого нет пути назад без бэкапа.
func recompute(ctx context.Context, pool *pgxpool.Pool, matches []match, dry bool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Рейтинг и число матчей копятся в памяти: база узнаёт итог один раз, в конце.
	type standing struct {
		rating float64
		played int
	}
	current := map[string]*standing{}
	standingOf := func(userID string) *standing {
		if found, ok := current[userID]; ok {
			return found
		}
		fresh := &standing{rating: 1000, played: 0}
		current[userID] = fresh
		return fresh
	}

	for _, item := range matches {
		participants := make([]application.EloParticipant, 0, len(item.seats))
		for _, player := range item.seats {
			position := standingOf(player.userID)
			participants = append(participants, application.EloParticipant{
				Rating: position.rating, MatchesPlayed: position.played, Damage: player.damage,
			})
		}

		updated, err := application.Recalculate(participants)
		if err != nil {
			return fmt.Errorf("матч %s: %w", item.id, err)
		}
		places := placesOf(participants)

		for index, player := range item.seats {
			position := standingOf(player.userID)
			before := format(position.rating)
			after := format(updated[index])

			if !dry {
				if err := write(ctx, tx, item, player.userID, places[index], before, after); err != nil {
					return err
				}
			}
			position.rating = updated[index]
			position.played++
		}
	}

	if dry {
		log.Print("сухой прогон: в базу ничего не записано")
		for userID, position := range current {
			log.Printf("  %s -> %s (%d матчей)", userID, format(position.rating), position.played)
		}
		return nil
	}

	// ⚠️ user_rating переписывается ЦЕЛИКОМ, а не досчитывается: matches_played обязано
	// совпасть с числом строк истории, иначе Elo выберет не тот шаг новичка.
	const upsert = `insert into user_rating (user_id, rating, matches_played, updated_at)
	                values ($1, $2::numeric, $3, now())
	                on conflict (user_id) do update set
	                    rating = excluded.rating,
	                    matches_played = excluded.matches_played,
	                    updated_at = excluded.updated_at`
	for userID, position := range current {
		if _, err := tx.Exec(ctx, upsert, userID, format(position.rating), position.played); err != nil {
			return fmt.Errorf("рейтинг игрока %s: %w", userID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("пересчитано матчей: %d, игроков: %d", len(matches), len(current))
	return nil
}

func placesOf(participants []application.EloParticipant) []int {
	order := make([]int, len(participants))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(left, right int) bool {
		return participants[order[left]].Damage < participants[order[right]].Damage
	})
	places := make([]int, len(participants))
	for position, index := range order {
		places[index] = position + 1
	}
	return places
}

func write(ctx context.Context, tx pgx.Tx, item match, userID string,
	place int, before, after string) error {
	const players = `update match_players
	                 set place = $3, rating_before = $4::numeric, rating_after = $5::numeric,
	                     rating_delta = $5::numeric - $4::numeric
	                 where match_id = $1 and user_id = $2`
	if _, err := tx.Exec(ctx, players, item.id, userID, place, before, after); err != nil {
		return fmt.Errorf("итог игрока %s: %w", userID, err)
	}

	const history = `update rating_history
	                 set rating_before = $3::numeric, rating_after = $4::numeric, place = $5
	                 where match_id = $1 and user_id = $2`
	if _, err := tx.Exec(ctx, history, item.id, userID, before, after, place); err != nil {
		return fmt.Errorf("история игрока %s: %w", userID, err)
	}
	return nil
}

func format(value float64) string { return strconv.FormatFloat(value, 'f', 2, 64) }

func report(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx,
		`select u.display_name, r.rating::text, r.matches_played
		 from user_rating r join users u on u.id = r.user_id
		 order by r.rating desc`)
	if err != nil {
		return err
	}
	defer rows.Close()

	log.Print("рейтинг после пересчёта:")
	for rows.Next() {
		var name, rating string
		var played int
		if err := rows.Scan(&name, &rating, &played); err != nil {
			return err
		}
		log.Printf("  %-28s %9s  (%d матчей)", name, rating, played)
	}
	return rows.Err()
}
