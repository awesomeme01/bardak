package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// История матчей: список сыгранного, разбивка по раздачам и лог событий.
//
// ⭐ Сырой лог (`match_events`) наружу не отдаётся никогда: в нём лежит скрытая
// информация — какая карта у кого. Репозиторий отдаёт его вместе с колонкой
// `private_to_seat`, а решение «показывать ли» принимает сценарий: правило видимости
// обязано жить в одном месте, иначе оно однажды разойдётся с тем, что видели за столом.

// HistoryMatch — строка `matches` в том объёме, в каком её показывает история.
//
// `rng_seed` и `rules_snapshot` сюда не читаются намеренно: клиенту они не отдаются,
// а seed до конца матча — это раскрытая колода.
type HistoryMatch struct {
	ID           string
	TableID      string
	Status       string
	PlayersCount int
	DealsPlayed  int
	StartedAt    time.Time
	FinishedAt   *time.Time
	AbortReason  *string
}

// HistoryParticipant — кто и на каком месте играл. Нужен для проверки видимости
// и для определения места спрашивающего в реплее.
type HistoryParticipant struct {
	UserID string
	SeatNo int
}

// HistoryPlayer — итог участника матча.
//
// ⚠️ У отменённого матча итог остаётся пустым: рейтинга он не касается. Поэтому почти
// всё здесь — указатели, и «нуля» вместо «ничего» быть не должно.
//
// Рейтинги едут строками, а не float64: в базе `numeric(8,2)`, и Java печатает их
// с тем же масштабом. Через float64 `1000.00` превратилось бы в `1000`.
type HistoryPlayer struct {
	MatchID      string
	UserID       string
	DisplayName  string
	SeatNo       int
	Place        *int
	NavesLevel   *string
	LossType     *string
	RatingBefore *string
	RatingAfter  *string
	RatingDelta  *string
}

// HistoryDeal — одна раздача матча.
type HistoryDeal struct {
	ID              string
	DealNo          int
	TrumpSuit       *string
	LoserSeat       int
	FinishedAt      *time.Time
	LastAttackCards []string
}

// HistoryDealSeat — итог места в раздаче.
type HistoryDealSeat struct {
	DealID           string
	SeatNo           int
	Place            *int
	HungCards        []string
	NavesLevelBefore *string
	NavesLevelAfter  *string
	LevelChanges     []HistoryLevelChange
}

// HistoryLevelChange — почему уровень навесов изменился и на сколько.
type HistoryLevelChange struct {
	Reason string
	Amount int
}

// HistoryEvent — событие матча из лога.
//
// ⭐ `PrivateToSeat` — записанная вместе с событием видимость: nil — публичное, иначе
// номер места, которому оно видно. Фильтр идёт по этой колонке, а не пересчётом правил.
type HistoryEvent struct {
	Seq           int
	DealNo        *int
	Type          string
	ActorSeat     *int
	Payload       json.RawMessage
	PrivateToSeat *int
}

// MatchHistory — доступ к истории матчей.
type MatchHistory struct{ pool *pgxpool.Pool }

// NewMatchHistory собирает репозиторий поверх пула.
func NewMatchHistory(pool *pgxpool.Pool) MatchHistory { return MatchHistory{pool: pool} }

const historyMatchColumns = `m.id, m.table_id, m.status, m.players_count, m.deals_played,
	m.started_at, m.finished_at, m.abort_reason`

// MatchesOf — матчи игрока, новые сверху.
//
// ⚠️ `status` сравнивается БЕЗ учёта регистра с именем состояния, а неизвестное значение
// просто даёт пустой список, а не ошибку — ровно как в Java. Пустая строка — без фильтра.
//
// Порядок — только по `started_at desc`, как в Java: добавить второй ключ сортировки
// значило бы разойтись с эталоном на матчах, начатых в одну микросекунду.
func (r MatchHistory) MatchesOf(ctx context.Context, userID, status string) ([]HistoryMatch, error) {
	const query = `select ` + historyMatchColumns + `
	               from matches m
	               join match_players p on p.match_id = m.id and p.user_id = $1
	               where $2::text = '' or lower(m.status) = lower($2::text)
	               order by m.started_at desc`

	rows, err := r.pool.Query(ctx, query, userID, status)
	if err != nil {
		return nil, fmt.Errorf("список матчей: %w", err)
	}
	defer rows.Close()

	matches := []HistoryMatch{}
	for rows.Next() {
		match, err := scanHistoryMatch(rows)
		if err != nil {
			return nil, fmt.Errorf("список матчей: %w", err)
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("список матчей: %w", err)
	}
	return matches, nil
}

// FindMatch — матч по идентификатору; ErrNotFound, если такого нет.
func (r MatchHistory) FindMatch(ctx context.Context, matchID string) (HistoryMatch, error) {
	const query = `select ` + historyMatchColumns + ` from matches m where m.id = $1`

	match, err := scanHistoryMatch(r.pool.QueryRow(ctx, query, matchID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HistoryMatch{}, ErrNotFound
		}
		return HistoryMatch{}, fmt.Errorf("чтение матча: %w", err)
	}
	return match, nil
}

// ParticipantsOf — кто играл в матче, по местам.
//
// Отдельный лёгкий запрос: решение «показывать ли матч» принимается до того, как
// понадобятся имена и рейтинги, и тянуть их ради проверки доступа незачем.
func (r MatchHistory) ParticipantsOf(ctx context.Context, matchID string) ([]HistoryParticipant, error) {
	const query = `select user_id, seat_no from match_players
	               where match_id = $1 order by seat_no`

	rows, err := r.pool.Query(ctx, query, matchID)
	if err != nil {
		return nil, fmt.Errorf("участники матча: %w", err)
	}
	defer rows.Close()

	players := []HistoryParticipant{}
	for rows.Next() {
		var player HistoryParticipant
		if err := rows.Scan(&player.UserID, &player.SeatNo); err != nil {
			return nil, fmt.Errorf("участники матча: %w", err)
		}
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("участники матча: %w", err)
	}
	return players, nil
}

// PlayersOf — итоги участников сразу нескольких матчей, по местам.
//
// ⭐ Один запрос на весь список, а не по запросу на матч: Java читает игроков в цикле,
// и на экране истории это N+1 обращений к базе на ровном месте. Ответ от этого не
// меняется — меняется только число походов в базу.
//
// ⚠️ Имя берётся LEFT JOIN'ом с прочерком по умолчанию: в Java пропавший пользователь
// даёт «—», а не пустое имя, и экран не должен отличать одно от другого.
func (r MatchHistory) PlayersOf(ctx context.Context, matchIDs []string) (map[string][]HistoryPlayer, error) {
	byMatch := map[string][]HistoryPlayer{}
	if len(matchIDs) == 0 {
		return byMatch, nil
	}

	const query = `select p.match_id, p.user_id, coalesce(u.display_name, '—'), p.seat_no,
	                      p.place, p.naves_level, p.loss_type,
	                      p.rating_before::text, p.rating_after::text, p.rating_delta::text
	               from match_players p
	               left join users u on u.id = p.user_id
	               where p.match_id = any($1::uuid[])
	               order by p.match_id, p.seat_no`

	rows, err := r.pool.Query(ctx, query, matchIDs)
	if err != nil {
		return nil, fmt.Errorf("итоги игроков: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var player HistoryPlayer
		err := rows.Scan(&player.MatchID, &player.UserID, &player.DisplayName, &player.SeatNo,
			&player.Place, &player.NavesLevel, &player.LossType,
			&player.RatingBefore, &player.RatingAfter, &player.RatingDelta)
		if err != nil {
			return nil, fmt.Errorf("итоги игроков: %w", err)
		}
		byMatch[player.MatchID] = append(byMatch[player.MatchID], player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("итоги игроков: %w", err)
	}
	return byMatch, nil
}

// DealsOf — раздачи матча по возрастанию номера.
func (r MatchHistory) DealsOf(ctx context.Context, matchID string) ([]HistoryDeal, error) {
	const query = `select id, deal_no, trump_suit, loser_seat, finished_at, last_attack_cards
	               from deals where match_id = $1 order by deal_no`

	rows, err := r.pool.Query(ctx, query, matchID)
	if err != nil {
		return nil, fmt.Errorf("раздачи матча: %w", err)
	}
	defer rows.Close()

	deals := []HistoryDeal{}
	for rows.Next() {
		var deal HistoryDeal
		// ⚠️ loser_seat в схеме nullable, но Java отдаёт его примитивным int: строка
		// без проигравшего даст там NPE и 500. Повторяем ошибкой, а не тихим нулём —
		// нулевое место означало бы «проиграл сидящий первым», и это была бы ложь.
		var loserSeat *int
		var lastAttack []string
		err := rows.Scan(&deal.ID, &deal.DealNo, &deal.TrumpSuit, &loserSeat,
			&deal.FinishedAt, &lastAttack)
		if err != nil {
			return nil, fmt.Errorf("раздачи матча: %w", err)
		}
		if loserSeat == nil {
			return nil, fmt.Errorf("раздача %s: в базе нет проигравшего места", deal.ID)
		}
		deal.LoserSeat = *loserSeat
		deal.LastAttackCards = orEmptyStrings(lastAttack)
		deals = append(deals, deal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("раздачи матча: %w", err)
	}
	return deals, nil
}

// historyLevelChangeRow — сырое изменение уровня из jsonb.
//
// ⚠️ Поля — указатели ради проверки: Java читает `change.get("reason").asText()` и на
// отсутствующем ключе падает в 500. Молча подставить пустую причину и ноль было бы хуже
// падения — экран показал бы правдоподобную неправду.
type historyLevelChangeRow struct {
	Reason *string `json:"reason"`
	Amount *int    `json:"amount"`
}

// DealSeatsOf — итоги мест по всем раздачам матча, сгруппированные по раздаче.
func (r MatchHistory) DealSeatsOf(ctx context.Context, matchID string) (map[string][]HistoryDealSeat, error) {
	const query = `select r.deal_id, r.seat_no, r.place, r.hung_cards,
	                      r.naves_level_before, r.naves_level_after, r.level_changes
	               from deal_results r
	               join deals d on d.id = r.deal_id
	               where d.match_id = $1
	               order by r.deal_id, r.seat_no`

	rows, err := r.pool.Query(ctx, query, matchID)
	if err != nil {
		return nil, fmt.Errorf("итоги раздач: %w", err)
	}
	defer rows.Close()

	byDeal := map[string][]HistoryDealSeat{}
	for rows.Next() {
		var seat HistoryDealSeat
		var hung []string
		var changes []historyLevelChangeRow
		err := rows.Scan(&seat.DealID, &seat.SeatNo, &seat.Place, &hung,
			&seat.NavesLevelBefore, &seat.NavesLevelAfter, &changes)
		if err != nil {
			return nil, fmt.Errorf("итоги раздач: %w", err)
		}
		seat.HungCards = orEmptyStrings(hung)
		seat.LevelChanges, err = toHistoryLevelChanges(seat.DealID, seat.SeatNo, changes)
		if err != nil {
			return nil, err
		}
		byDeal[seat.DealID] = append(byDeal[seat.DealID], seat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("итоги раздач: %w", err)
	}
	return byDeal, nil
}

// EventsOf — весь лог матча, включая приватные события, по возрастанию номера.
//
// ⚠️ Отдавать это наружу как есть НЕЛЬЗЯ: `PrivateToSeat` обязан быть применён выше.
// Фильтр не зашит в запрос намеренно — правило видимости живёт в сценарии, в одном месте
// и с догоном по сокету, а не размазано по SQL.
func (r MatchHistory) EventsOf(ctx context.Context, matchID string) ([]HistoryEvent, error) {
	// seq > 0 — как в Java (`matchLog.since(matchId, 0, seat)`): нумерация начинается
	// с единицы, и условие лишь повторяет её, ничего не отбрасывая.
	const query = `select seq, deal_no, type, actor_seat, payload, private_to_seat
	               from match_events where match_id = $1 and seq > 0 order by seq`

	rows, err := r.pool.Query(ctx, query, matchID)
	if err != nil {
		return nil, fmt.Errorf("лог матча: %w", err)
	}
	defer rows.Close()

	events := []HistoryEvent{}
	for rows.Next() {
		var event HistoryEvent
		var payload []byte
		err := rows.Scan(&event.Seq, &event.DealNo, &event.Type, &event.ActorSeat,
			&payload, &event.PrivateToSeat)
		if err != nil {
			return nil, fmt.Errorf("лог матча: %w", err)
		}
		if !json.Valid(payload) {
			// Java здесь бросает IllegalStateException и отвечает 500. Тихо отдать
			// сломанный кусок в тело ответа нельзя: он развалит разбор у клиента.
			return nil, fmt.Errorf("событие %d матча %s: в базе неразбираемый JSON", event.Seq, matchID)
		}
		event.Payload = json.RawMessage(payload)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("лог матча: %w", err)
	}
	return events, nil
}

func scanHistoryMatch(row scannable) (HistoryMatch, error) {
	var match HistoryMatch
	err := row.Scan(&match.ID, &match.TableID, &match.Status, &match.PlayersCount,
		&match.DealsPlayed, &match.StartedAt, &match.FinishedAt, &match.AbortReason)
	return match, err
}

func toHistoryLevelChanges(dealID string, seatNo int,
	rows []historyLevelChangeRow) ([]HistoryLevelChange, error) {
	changes := []HistoryLevelChange{}
	for _, row := range rows {
		if row.Reason == nil || row.Amount == nil {
			return nil, fmt.Errorf("раздача %s место %d: в level_changes нет reason или amount",
				dealID, seatNo)
		}
		changes = append(changes, HistoryLevelChange{Reason: *row.Reason, Amount: *row.Amount})
	}
	return changes, nil
}

// orEmptyStrings — пустой массив jsonb обязан остаться пустым СПИСКОМ, а не стать nil:
// nil сериализуется в null, а Java отдаёт здесь [] (MD-003).
func orEmptyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
