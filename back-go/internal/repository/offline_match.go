package repository

import (
	"context"
	"fmt"
	"time"
)

// OfflineMatch — партия, сыгранная за настоящим столом и записанная вручную.
//
// ⭐ От законченного онлайн-матча отличается только происхождением: стола нет, раздач
// нет, лога событий нет. Места, ступени навесов, степени проигрыша и рейтинг — те же
// самые поля в тех же таблицах, и считает их тот же код.
type OfflineMatch struct {
	MatchID string
	// CreatedBy — кто записал партию; обязан быть одним из её участников.
	CreatedBy string
	// PlayedAt — когда играли. Может быть сильно раньше момента записи.
	PlayedAt time.Time
	// SeasonID — сезон, к которому отнести партию; пусто — вне сезонов.
	SeasonID *string
	// LoserUserID — главный проигравший; пусто, если до джокера никто не дошёл.
	LoserUserID *string
	Outcomes    []SeatOutcome
}

// CreateOffline записывает оффлайн-партию целиком: матч, итоги игроков и рейтинг.
//
// ⭐ Одной транзакцией и теми же помощниками, что и у онлайн-матча (writeSeatOutcome,
// writeRating): «матч записался, а рейтинг нет» здесь сломало бы историю ровно так же.
//
// ⚠️ rng_seed и rules_snapshot заполняются пустыми значениями, а не остаются null:
// колонки объявлены not null у онлайн-матча, и ослаблять их ради партии, у которой
// колоды не было вовсе, значило бы разрешить матч без правил и там, где они нужны.
func (r MatchResults) CreateOffline(ctx context.Context, match OfflineMatch) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("оффлайн-партия: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insert = `insert into matches (id, table_id, is_offline, created_by, status,
	                    players_count, deals_played, rng_seed, rules_snapshot,
	                    started_at, finished_at, loser_user_id)
	                values ($1, null, true, $2, $3, $4, 0, 0, '{}'::jsonb, $5, $5, $6)`
	_, err = tx.Exec(ctx, insert, match.MatchID, match.CreatedBy, MatchFinished,
		len(match.Outcomes), match.PlayedAt, match.LoserUserID)
	if err != nil {
		return fmt.Errorf("запись оффлайн-партии: %w", err)
	}

	finished := FinishedMatch{
		MatchID:     match.MatchID,
		LoserUserID: match.LoserUserID,
		SeasonID:    match.SeasonID,
		Outcomes:    match.Outcomes,
		At:          match.PlayedAt,
	}
	for _, outcome := range match.Outcomes {
		if err := writeSeatOutcome(ctx, tx, finished, outcome); err != nil {
			return err
		}
		if err := writeRating(ctx, tx, finished, outcome); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("оффлайн-партия: %w", err)
	}
	return nil
}
