package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DealSeatResult — итог одного места в сыгранной раздаче.
type DealSeatResult struct {
	SeatNo int
	// Place — место в раздаче; пусто у не расставленных итогов.
	Place *int
	// HungCards — что игроку навесили, кодами карт.
	HungCards []string
	// NavesLevelBefore и NavesLevelAfter — ступени шкалы; пусто, если навесов не было.
	NavesLevelBefore *string
	NavesLevelAfter  *string
	// LevelChanges — из чего сложился сдвиг: слагаемые, а не сумма.
	LevelChanges []DealLevelChange
}

// DealLevelChange — одно слагаемое сдвига уровня.
type DealLevelChange struct {
	Reason string `json:"reason"`
	Amount int    `json:"amount"`
}

// PlayedDeal — сыгранная раздача целиком.
type PlayedDeal struct {
	ID              string
	MatchID         string
	DealNo          int
	TrumpSuit       *string
	LoserSeat       int
	LastAttackCards []string
	FinishedAt      time.Time
	Seats           []DealSeatResult
}

// DealHistory — запись сыгранных раздач.
//
// ⭐ Пишется ИТОГ, а не ход раздачи: ходы уже лежат в журнале событий. Здесь то, что
// иначе пришлось бы восстанавливать переигрыванием, — козырь, состав последней атаки,
// места, навешенное и из чего сложился сдвиг уровня.
type DealHistory struct{ pool *pgxpool.Pool }

// NewDealHistory собирает репозиторий.
func NewDealHistory(pool *pgxpool.Pool) DealHistory { return DealHistory{pool: pool} }

// Record записывает раздачу вместе с итогами мест.
//
// ⚠️ Раздача с этим номером уже записанная — не ошибка, а повтор: движок мог доиграть
// её второй раз после восстановления. Тогда запись пропускается целиком, чтобы история
// не двоилась.
func (r DealHistory) Record(ctx context.Context, deal PlayedDeal) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("запись раздачи: %w", err)
	}
	defer tx.Rollback(ctx)

	const insertDeal = `insert into deals (id, match_id, deal_no, trump_suit, loser_seat,
	                        last_attack_cards, finished_at)
	                    values ($1, $2, $3, $4, $5, $6::jsonb, $7)
	                    on conflict (match_id, deal_no) do nothing`
	tag, err := tx.Exec(ctx, insertDeal, deal.ID, deal.MatchID, deal.DealNo, deal.TrumpSuit,
		deal.LoserSeat, jsonArrayOf(deal.LastAttackCards), deal.FinishedAt)
	if err != nil {
		return fmt.Errorf("запись раздачи: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	for _, seat := range deal.Seats {
		const insertResult = `insert into deal_results (deal_id, seat_no, place, hung_cards,
		                          naves_level_before, naves_level_after, level_changes)
		                      values ($1, $2, $3, $4::jsonb, $5, $6, $7::jsonb)`
		changes, err := jsonObjectsOf(seat.LevelChanges)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, insertResult, deal.ID, seat.SeatNo, seat.Place,
			jsonArrayOf(seat.HungCards), seat.NavesLevelBefore, seat.NavesLevelAfter, changes)
		if err != nil {
			return fmt.Errorf("итог места в раздаче: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("запись раздачи: %w", err)
	}
	return nil
}

// jsonArrayOf — коды карт как массив JSON. Пусто — «[]», а не null: колонка объявлена
// not null default '[]'.
func jsonArrayOf(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func jsonObjectsOf(changes []DealLevelChange) (string, error) {
	if len(changes) == 0 {
		return "[]", nil
	}
	encoded, err := json.Marshal(changes)
	if err != nil {
		return "", fmt.Errorf("сдвиги уровня не сериализуются: %w", err)
	}
	return string(encoded), nil
}
