package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MatchSeatResult — итог игрока в законченном матче.
//
// ⚠️ Почти всё здесь указатели: у отменённого матча итога нет вовсе, и «ноль» вместо
// «ничего» превратил бы неигранный матч в первое место. Рейтинги едут строками — в базе
// `numeric(8,2)`, и через float64 `1000.00` стало бы `1000`.
type MatchSeatResult struct {
	UserID       string
	Place        int
	NavesLevel   *string
	LossType     *string
	RatingBefore string
	RatingAfter  string
}

// MatchPlayers — места в матче и их итоги.
//
// ⭐ Таблица заводится ПРИ СТАРТЕ, пустая: порядок мест известен сразу, а итог — нет.
// Без этого места матча пришлось бы потом брать из лобби, где к тому времени уже другие
// люди на других стульях (см. восстановление матча).
type MatchPlayers struct{ pool *pgxpool.Pool }

// NewMatchPlayers собирает репозиторий.
func NewMatchPlayers(pool *pgxpool.Pool) MatchPlayers { return MatchPlayers{pool: pool} }

// Seat сажает игроков матча по местам движка: индекс в списке и есть номер места.
//
// Повторный вызов безвреден: строки уже есть, значит матч уже посажен.
func (r MatchPlayers) Seat(ctx context.Context, matchID string, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for seatNo, userID := range userIDs {
		batch.Queue(`insert into match_players (match_id, user_id, seat_no)
		             values ($1, $2, $3) on conflict (match_id, user_id) do nothing`,
			matchID, userID, seatNo)
	}
	if err := r.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("посадка игроков матча: %w", err)
	}
	return nil
}
