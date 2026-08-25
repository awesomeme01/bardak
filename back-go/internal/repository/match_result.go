package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeatOutcome — итог одного игрока в законченном матче.
//
// Рейтинги едут строками: в базе `numeric(8,2)`, и через float64 «1000.00» стало бы
// «1000». Считает их сценарий, а форматирует — он же.
type SeatOutcome struct {
	UserID string
	SeatNo int
	Place  int
	// NavesLevel — ступень шкалы («10», «Jk»); пусто, если навесов не было.
	NavesLevel *string
	// LossType — степень проигрыша; пусто у не проигравшего.
	LossType     *string
	RatingBefore string
	RatingAfter  string
}

// FinishedMatch — всё, что записывается по итогу матча.
type FinishedMatch struct {
	MatchID string
	// LoserUserID — главный проигравший; пусто, если его нет.
	LoserUserID *string
	// SeasonID — открытый сезон на момент матча; пусто — матч вне сезонов.
	SeasonID *string
	Outcomes []SeatOutcome
	At       time.Time
}

// MatchResults — запись итога матча вместе с рейтингом.
//
// ⭐ ОДНОЙ ТРАНЗАКЦИЕЙ, и это главное свойство этого типа: «матч записался, а рейтинг
// нет» даёт неисправимое расхождение между историей и текущим рейтингом — починить его
// потом можно только руками и на глаз. Поэтому три таблицы обслуживает один тип, а не
// три репозитория со своими транзакциями.
type MatchResults struct{ pool *pgxpool.Pool }

// NewMatchResults собирает репозиторий итогов.
func NewMatchResults(pool *pgxpool.Pool) MatchResults { return MatchResults{pool: pool} }

// AlreadyCounted — рейтинг за этот матч уже посчитан.
//
// ⭐ Ключ идемпотентности — именно строка в rating_history, а не статус матча: реконнект,
// повтор после рестарта или вторая доставка события не должны удваивать рейтинг.
func (r MatchResults) AlreadyCounted(ctx context.Context, matchID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`select exists (select 1 from rating_history where match_id = $1)`, matchID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("проверка итога матча: %w", err)
	}
	return exists, nil
}

// OpenSeasonID — идущий сезон. Второе значение false — сезонов нет, и матч вне их.
func (r MatchResults) OpenSeasonID(ctx context.Context) (string, bool, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`select id from seasons where closed_at is null limit 1`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("открытый сезон: %w", err)
	}
	return id, true, nil
}

// Finish записывает места, степени, рейтинг и закрывает матч — одной транзакцией.
func (r MatchResults) Finish(ctx context.Context, finished FinishedMatch) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("итог матча: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, outcome := range finished.Outcomes {
		if err := writeSeatOutcome(ctx, tx, finished, outcome); err != nil {
			return err
		}
		if err := writeRating(ctx, tx, finished, outcome); err != nil {
			return err
		}
	}

	// ⚠️ Статус матча меняется В ТОЙ ЖЕ транзакции: закрытый матч без рейтинга выглядел
	// бы сыгранным и в статистику попал бы уже неправильным.
	_, err = tx.Exec(ctx, `update matches set status = $2, finished_at = $3, loser_user_id = $4
	                       where id = $1`,
		finished.MatchID, MatchFinished, finished.At, finished.LoserUserID)
	if err != nil {
		return fmt.Errorf("закрытие матча: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("итог матча: %w", err)
	}
	return nil
}

// writeSeatOutcome — место, ступень и степень проигрыша игрока.
//
// ⚠️ Строка обычно уже есть со старта матча, но upsert не для красоты: матч,
// восстановленный из снимка, мог быть начат ещё до появления match_players.
func writeSeatOutcome(ctx context.Context, tx pgx.Tx, finished FinishedMatch,
	outcome SeatOutcome) error {
	const query = `insert into match_players (match_id, user_id, seat_no, naves_level, loss_type,
	                   place, rating_before, rating_after, rating_delta)
	               values ($1, $2, $3, $4, $5, $6, $7::numeric, $8::numeric,
	                       $8::numeric - $7::numeric)
	               on conflict (match_id, user_id) do update set
	                   naves_level = excluded.naves_level,
	                   loss_type = excluded.loss_type,
	                   place = excluded.place,
	                   rating_before = excluded.rating_before,
	                   rating_after = excluded.rating_after,
	                   rating_delta = excluded.rating_delta`

	_, err := tx.Exec(ctx, query, finished.MatchID, outcome.UserID, outcome.SeatNo,
		outcome.NavesLevel, outcome.LossType, outcome.Place,
		outcome.RatingBefore, outcome.RatingAfter)
	if err != nil {
		return fmt.Errorf("итог игрока матча: %w", err)
	}
	return nil
}

// writeRating — точка истории рейтинга и новый рейтинг игрока.
//
// ⭐ Порядок обратный ожидаемому: СНАЧАЛА история, потом рейтинг, и рейтинг двигается
// только если строка истории действительно появилась. Так повтор — реконнект, ретрай,
// повторная доставка после рестарта — не удваивает ни рейтинг, ни счётчик матчей.
// Проверка «уже посчитан» есть и в сценарии, но она не атомарна, а эта — атомарна.
func writeRating(ctx context.Context, tx pgx.Tx, finished FinishedMatch,
	outcome SeatOutcome) error {
	// ⚠️ deviation_after not null: MVP считает простой Elo и отклонение не двигает,
	// но колонка заведена под переезд на Glicko-2 и пустой быть не может.
	const history = `insert into rating_history (user_id, match_id, rating_before, rating_after,
	                     deviation_after, place, players_count, season_id)
	                 values ($1, $2, $3::numeric, $4::numeric,
	                         coalesce((select deviation from user_rating where user_id = $1), 350),
	                         $5, $6, $7)
	                 on conflict (user_id, match_id) do nothing`
	tag, err := tx.Exec(ctx, history, outcome.UserID, finished.MatchID, outcome.RatingBefore,
		outcome.RatingAfter, outcome.Place, len(finished.Outcomes), finished.SeasonID)
	if err != nil {
		return fmt.Errorf("история рейтинга: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Этот матч игроку уже посчитан. Место и степень выше обновились — они
		// идемпотентны, — а рейтинг трогать нельзя.
		return nil
	}

	// ⭐ Счётчик матчей растёт вместе с рейтингом: по нему Elo выбирает шаг новичка,
	// и разъехавшись с числом строк в истории, он стал бы выбирать не тот.
	const rating = `insert into user_rating (user_id, rating, matches_played, updated_at)
	                values ($1, $2::numeric, 1, $3)
	                on conflict (user_id) do update set
	                    rating = excluded.rating,
	                    matches_played = user_rating.matches_played + 1,
	                    updated_at = excluded.updated_at`
	if _, err := tx.Exec(ctx, rating, outcome.UserID, outcome.RatingAfter, finished.At); err != nil {
		return fmt.Errorf("рейтинг игрока: %w", err)
	}
	return nil
}
