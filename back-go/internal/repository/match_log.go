package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Статусы матча. Строки те же, что в check-ограничении базы.
const (
	MatchInProgress = "IN_PROGRESS"
	MatchPaused     = "PAUSED"
	MatchFinished   = "FINISHED"
	MatchAborted    = "ABORTED"
)

// MatchRecord — строка таблицы matches.
type MatchRecord struct {
	ID            string
	TableID       string
	Status        string
	PlayersCount  int
	DealsPlayed   int
	RngSeed       int64
	RulesSnapshot string
	StartedAt     time.Time
	FinishedAt    *time.Time
	LoserUserID   *string
	AbortReason   *string
}

// MatchEvent — строка журнала событий.
//
// ⚠️ Payload содержит ПОЛНУЮ информацию, включая скрытую: это внутренний лог, и наружу
// он идёт только через проекцию под место смотрящего. Отдать его сырым — значит раскрыть
// чужие руки.
type MatchEvent struct {
	Seq       int
	DealNo    *int
	Type      string
	ActorSeat *int
	Payload   string
	CreatedAt time.Time
}

// MatchLog — журнал матча, снимки и сам матч.
type MatchLog struct{ pool *pgxpool.Pool }

// NewMatchLog собирает журнал.
func NewMatchLog(pool *pgxpool.Pool) MatchLog { return MatchLog{pool: pool} }

// StartMatch заводит матч.
func (r MatchLog) StartMatch(ctx context.Context, id, tableID string, playersCount int,
	seed int64, rulesSnapshot string) (MatchRecord, error) {
	const query = `insert into matches (id, table_id, status, players_count, rng_seed, rules_snapshot)
	               values ($1, $2, $3, $4, $5, $6)
	               returning id, table_id, status, players_count, deals_played, rng_seed,
	                         rules_snapshot::text, started_at, finished_at, loser_user_id, abort_reason`
	row := r.pool.QueryRow(ctx, query, id, tableID, MatchInProgress, playersCount, seed, rulesSnapshot)
	return scanMatch(row)
}

// ActiveMatchFor — идущий матч за столом.
//
// ⭐ По нему матч поднимается после перезапуска сервера: в памяти его нет, а в базе он
// числится идущим — значит, сервер перезапускали, и игроки об этом знать не должны.
func (r MatchLog) ActiveMatchFor(ctx context.Context, tableID string) (MatchRecord, error) {
	const query = `select id, table_id, status, players_count, deals_played, rng_seed,
	                      rules_snapshot::text, started_at, finished_at, loser_user_id, abort_reason
	               from matches
	               where table_id = $1 and status in ('IN_PROGRESS', 'PAUSED')
	               order by started_at desc limit 1`
	return r.one(ctx, query, tableID)
}

// MatchByID — матч по идентификатору.
func (r MatchLog) MatchByID(ctx context.Context, id string) (MatchRecord, error) {
	const query = `select id, table_id, status, players_count, deals_played, rng_seed,
	                      rules_snapshot::text, started_at, finished_at, loser_user_id, abort_reason
	               from matches where id = $1`
	return r.one(ctx, query, id)
}

// Append дописывает события одним пакетом и возвращает последний номер.
//
// ⭐ Сначала лог, потом рассылка. Иначе после падения между ними клиенты видели бы ход,
// которого в истории нет, — и реплей разошёлся бы с тем, что люди помнят.
//
// ⚠️ Уникальность (match_id, seq) держит база: дыр и дублей при гонках быть не может,
// потому что только вставка, а seq сквозной по матчу.
func (r MatchLog) Append(ctx context.Context, matchID string, firstSeq int, dealNo int,
	events []MatchEvent) (int, error) {
	if len(events) == 0 {
		return firstSeq - 1, nil
	}

	batch := &pgx.Batch{}
	seq := firstSeq
	for _, event := range events {
		batch.Queue(`insert into match_events (match_id, seq, deal_no, type, actor_seat, payload)
		             values ($1, $2, $3, $4, $5, $6::jsonb)`,
			matchID, seq, dealNo, event.Type, event.ActorSeat, event.Payload)
		seq++
	}

	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range events {
		if _, err := results.Exec(); err != nil {
			return 0, fmt.Errorf("запись событий матча: %w", err)
		}
	}
	return seq - 1, nil
}

// AppendRejected записывает отклонённую попытку.
//
// ⭐ Отказ — часть истории стола, хотя состояние не меняет: по нему видно, что игрок
// пытался сделать, и разбор спорной партии без этого неполон.
func (r MatchLog) AppendRejected(ctx context.Context, matchID string, seq, dealNo, actorSeat int,
	commandType, reason string) error {
	const query = `insert into match_events (match_id, seq, deal_no, type, actor_seat, payload)
	               values ($1, $2, $3, 'ATTEMPT_REJECTED', $4, $5::jsonb)`
	payload := fmt.Sprintf(`{"command":%q,"reason":%q}`, commandType, reason)
	if _, err := r.pool.Exec(ctx, query, matchID, seq, dealNo, actorSeat, payload); err != nil {
		return fmt.Errorf("запись отклонённого хода: %w", err)
	}
	return nil
}

// Since — события матча начиная с номера.
func (r MatchLog) Since(ctx context.Context, matchID string, afterSeq int) ([]MatchEvent, error) {
	const query = `select seq, deal_no, type, actor_seat, payload::text, created_at
	               from match_events where match_id = $1 and seq > $2 order by seq`
	rows, err := r.pool.Query(ctx, query, matchID, afterSeq)
	if err != nil {
		return nil, fmt.Errorf("чтение событий матча: %w", err)
	}
	defer rows.Close()

	events := make([]MatchEvent, 0, 64)
	for rows.Next() {
		var event MatchEvent
		if err := rows.Scan(&event.Seq, &event.DealNo, &event.Type, &event.ActorSeat,
			&event.Payload, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("разбор события матча: %w", err)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// SaveSnapshot сохраняет состояние после события с этим номером.
//
// ⚠️ Повтор номера — не ошибка: снимок переписывается. Иначе после переподключения
// сохранение падало бы на первичном ключе там, где состояние просто не изменилось.
func (r MatchLog) SaveSnapshot(ctx context.Context, matchID string, seq int, state string) error {
	const query = `insert into match_snapshots (match_id, seq, state) values ($1, $2, $3::jsonb)
	               on conflict (match_id, seq) do update set state = excluded.state`
	if _, err := r.pool.Exec(ctx, query, matchID, seq, state); err != nil {
		return fmt.Errorf("сохранение снимка: %w", err)
	}
	return nil
}

// LatestSnapshot — самый свежий снимок матча.
func (r MatchLog) LatestSnapshot(ctx context.Context, matchID string) (int, string, error) {
	const query = `select seq, state::text from match_snapshots
	               where match_id = $1 order by seq desc limit 1`
	var seq int
	var state string
	err := r.pool.QueryRow(ctx, query, matchID).Scan(&seq, &state)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", ErrNotFound
		}
		return 0, "", fmt.Errorf("чтение снимка: %w", err)
	}
	return seq, state, nil
}

// DealsPlayed обновляет счётчик сыгранных раздач.
func (r MatchLog) DealsPlayed(ctx context.Context, matchID string, played int) error {
	_, err := r.pool.Exec(ctx, `update matches set deals_played = $2 where id = $1`, matchID, played)
	if err != nil {
		return fmt.Errorf("счётчик раздач: %w", err)
	}
	return nil
}

// Finish закрывает матч.
func (r MatchLog) Finish(ctx context.Context, matchID, status string, loserUserID *string,
	abortReason *string, at time.Time) error {
	const query = `update matches set status = $2, finished_at = $3,
	                      loser_user_id = $4, abort_reason = $5 where id = $1`
	if _, err := r.pool.Exec(ctx, query, matchID, status, at, loserUserID, abortReason); err != nil {
		return fmt.Errorf("закрытие матча: %w", err)
	}
	return nil
}

func (r MatchLog) one(ctx context.Context, query string, args ...any) (MatchRecord, error) {
	record, err := scanMatch(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MatchRecord{}, ErrNotFound
		}
		return MatchRecord{}, fmt.Errorf("чтение матча: %w", err)
	}
	return record, nil
}

func scanMatch(row scannable) (MatchRecord, error) {
	var record MatchRecord
	err := row.Scan(&record.ID, &record.TableID, &record.Status, &record.PlayersCount,
		&record.DealsPlayed, &record.RngSeed, &record.RulesSnapshot, &record.StartedAt,
		&record.FinishedAt, &record.LoserUserID, &record.AbortReason)
	return record, err
}
