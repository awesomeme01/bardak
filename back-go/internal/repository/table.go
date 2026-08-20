package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Отказы посадки. Их три, а не один, потому что нарушения разных индексов требуют
// разного ответа игроку:
//
//	table_players_table_id_seat_no_key — место увели, расклад надо пересчитать;
//	ux_table_players_user              — увели самого игрока за другой стол, пересчитывать нечего;
//	table_players_pkey                 — он уже сидит за ЭТИМ столом, посадка идемпотентна.
//
// ⭐ Разбор идёт по ИМЕНИ ограничения, а не по повторному чтению состояния, как в Java:
// повторное чтение отвечает на вопрос «что там сейчас», а нужен ответ «на чём именно
// упала вставка» — между этими двумя моментами состояние успевает поменяться ещё раз.
var (
	ErrSeatTaken       = errors.New("место за столом уже занято")
	ErrSeatedElsewhere = errors.New("игрок уже сидит за другим столом")
	ErrAlreadySeated   = errors.New("игрок уже сидит за этим столом")
)

// Статусы стола. Значения проверяет game_tables_status_check, поэтому строки здесь —
// не украшение, а часть контракта с базой.
const (
	TableWaiting = "WAITING"
	TableInMatch = "IN_MATCH"
	TableClosed  = "CLOSED"
)

// Состояния места. ⚠️ LEFT в базу не пишется никогда: место освобождают удалением строки,
// потому что занятость держит уникальный индекс (table_id, seat_no), а пометка в строке
// его не отпускает.
const (
	SeatJoined = "JOINED"
	SeatReady  = "READY"
)

// GameTable — строка таблицы game_tables.
//
// RulesConfig лежит текстом ровно как в jsonb: разбирать его здесь незачем — правила
// читает движок, а REST их наружу не отдаёт вовсе.
type GameTable struct {
	ID          string
	Code        string
	Name        string
	HostUserID  string
	MaxPlayers  int
	Status      string
	CardSetID   string
	ThemeID     string
	RulesConfig string
	IsPrivate   bool
	Version     int
	CreatedAt   time.Time
	ClosedAt    *time.Time
}

// IsOpenForJoin — за стол ещё можно сесть.
func (t GameTable) IsOpenForJoin() bool { return t.Status == TableWaiting }

// IsHost — этот игрок завёл стол.
func (t GameTable) IsHost(userID string) bool { return t.HostUserID == userID }

// TablePlayer — строка таблицы table_players: игрок на своём месте.
type TablePlayer struct {
	TableID  string
	UserID   string
	SeatNo   int
	State    string
	JoinedAt time.Time
}

// IsReady — игрок нажал «готов».
func (p TablePlayer) IsReady() bool { return p.State == SeatReady }

// Tables — репозиторий столов и мест за ними.
//
// ⭐ Обе таблицы в одном репозитории намеренно: место без стола не имеет смысла,
// а каждая операция посадки читает и то, и другое.
type Tables struct{ pool *pgxpool.Pool }

// NewTables собирает репозиторий поверх пула.
func NewTables(pool *pgxpool.Pool) Tables { return Tables{pool: pool} }

// ⚠️ rules_config приводится к тексту прямо в запросе: сканировать jsonb в строку иначе
// пришлось бы через кодек, а лишний слой преобразований здесь ничего не даёт.
const tableColumns = `id, code, name, host_user_id, max_players, status, card_set_id,
	theme_id, rules_config::text, is_private, version, created_at, closed_at`

const seatColumns = `table_id, user_id, seat_no, state, joined_at`

// FindOpen — столы для лобби.
//
// ⚠️ Приватные не попадают сюда НИКОГДА: на такой стол зовут кодом в переписке, и он не
// должен всплывать в общем списке — иначе приватность сводится к тому, что стол просто
// труднее заметить.
func (r Tables) FindOpen(ctx context.Context) ([]GameTable, error) {
	const query = `select ` + tableColumns + ` from game_tables
	               where status = 'WAITING' and is_private = false
	               order by created_at desc`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("список открытых столов: %w", err)
	}
	defer rows.Close()

	tables := make([]GameTable, 0)
	for rows.Next() {
		table, err := scanTable(rows)
		if err != nil {
			return nil, fmt.Errorf("чтение стола: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("список открытых столов: %w", err)
	}
	return tables, nil
}

// FindByID — стол по идентификатору.
func (r Tables) FindByID(ctx context.Context, id string) (GameTable, error) {
	return r.oneTable(ctx, `select `+tableColumns+` from game_tables where id = $1`, id)
}

// FindByCode — стол по коду приглашения, БЕЗ учёта регистра.
//
// ⚠️ Код диктуют голосом и пересылают в переписке, где клавиатура сама ставит заглавную
// букву. Точное сравнение означало бы «код верный, но стол не найден» — самая обидная
// из возможных ошибок.
func (r Tables) FindByCode(ctx context.Context, code string) (GameTable, error) {
	return r.oneTable(ctx, `select `+tableColumns+` from game_tables where code = upper($1)`, code)
}

// ExistsByCode — занят ли код.
//
// ⚠️ Ответ устаревает сразу же: правду про занятость держит уникальный индекс на code,
// а не эта проверка. Она нужна лишь чтобы не тратить попытку впустую.
func (r Tables) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`select exists(select 1 from game_tables where code = $1)`, code).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("проверка кода стола: %w", err)
	}
	return exists, nil
}

// Insert заводит стол. status, version и created_at ставит база — как в Java.
func (r Tables) Insert(ctx context.Context, table GameTable) (GameTable, error) {
	const query = `insert into game_tables
	               (id, code, name, host_user_id, max_players, status, card_set_id, theme_id,
	                rules_config, is_private)
	               values ($1, $2, $3, $4, $5, 'WAITING', $6, $7, $8::jsonb, $9)
	               returning ` + tableColumns

	saved, err := scanTable(r.pool.QueryRow(ctx, query, table.ID, table.Code, table.Name,
		table.HostUserID, table.MaxPlayers, table.CardSetID, table.ThemeID,
		defaultRules(table.RulesConfig), table.IsPrivate))
	if err != nil {
		if _, ok := uniqueViolationOf(err); ok {
			return GameTable{}, ErrConflict
		}
		return GameTable{}, fmt.Errorf("создание стола: %w", err)
	}
	return saved, nil
}

// Close закрывает стол.
//
// ⭐ Строка не удаляется никогда: по столу висят матчи, а история важнее чистоты списка.
// ⚠️ Места (table_players) при этом остаются — так же, как в Java.
func (r Tables) Close(ctx context.Context, id string, at time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`update game_tables set status = 'CLOSED', closed_at = $2, version = version + 1
		 where id = $1`, id, at)
	if err != nil {
		return fmt.Errorf("закрытие стола: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Seats — все места за столом по возрастанию номера.
func (r Tables) Seats(ctx context.Context, tableID string) ([]TablePlayer, error) {
	rows, err := r.pool.Query(ctx,
		`select `+seatColumns+` from table_players where table_id = $1 order by seat_no`, tableID)
	if err != nil {
		return nil, fmt.Errorf("места за столом: %w", err)
	}
	defer rows.Close()

	seats := make([]TablePlayer, 0)
	for rows.Next() {
		seat, err := scanSeat(rows)
		if err != nil {
			return nil, fmt.Errorf("чтение места: %w", err)
		}
		seats = append(seats, seat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("места за столом: %w", err)
	}
	return seats, nil
}

// SeatAt — место этого игрока за этим столом.
func (r Tables) SeatAt(ctx context.Context, tableID, userID string) (TablePlayer, error) {
	return r.oneSeat(ctx,
		`select `+seatColumns+` from table_players where table_id = $1 and user_id = $2`,
		tableID, userID)
}

// SeatOf — единственное место игрока во всей базе.
//
// ⭐ Именно единственное: ux_table_players_user (V9) — глобальный уникальный индекс по
// user_id, поэтому строк на игрока не может быть две. Возвращать список, как это делает
// Java, значит притворяться, будто их бывает больше.
func (r Tables) SeatOf(ctx context.Context, userID string) (TablePlayer, error) {
	return r.oneSeat(ctx, `select `+seatColumns+` from table_players where user_id = $1`, userID)
}

// InsertSeat сажает игрока на конкретное место.
//
// ⚠️ Правду про свободу места держит БАЗА. Проверка «место свободно» и вставка не
// атомарны: двое, нажавшие «сесть» одновременно, оба её проходят. Поэтому нарушение
// уникальности здесь — не поломка, а штатный ответ «опоздал», и вызывающий пересчитывает
// расклад.
func (r Tables) InsertSeat(ctx context.Context, seat TablePlayer) (TablePlayer, error) {
	const query = `insert into table_players (table_id, user_id, seat_no, state)
	               values ($1, $2, $3, $4)
	               returning ` + seatColumns

	saved, err := scanSeat(r.pool.QueryRow(ctx, query, seat.TableID, seat.UserID, seat.SeatNo,
		defaultSeatState(seat.State)))
	if err != nil {
		if constraint, ok := uniqueViolationOf(err); ok {
			return TablePlayer{}, seatConflict(constraint)
		}
		return TablePlayer{}, fmt.Errorf("посадка за стол: %w", err)
	}
	return saved, nil
}

// DeleteSeat освобождает место. Второй вызов безвреден: цель уже достигнута.
//
// ⭐ Строка именно удаляется, а не помечается: место обязано освободиться для других,
// а его занятость определяет уникальный индекс (table_id, seat_no).
func (r Tables) DeleteSeat(ctx context.Context, tableID, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`delete from table_players where table_id = $1 and user_id = $2`, tableID, userID)
	if err != nil {
		return false, fmt.Errorf("выход из-за стола: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DefaultCardSetID — набор карт по умолчанию.
//
// ⭐ Он ровно один: частичный уникальный индекс idx_card_sets_single_default не даёт
// завести второй, поэтому limit здесь — формальность, а не выбор наугад.
func (r Tables) DefaultCardSetID(ctx context.Context) (string, error) {
	return r.oneID(ctx, `select id from card_sets where is_default limit 1`)
}

// DefaultThemeID — тема стола по умолчанию (idx_table_themes_single_default).
func (r Tables) DefaultThemeID(ctx context.Context) (string, error) {
	return r.oneID(ctx, `select id from table_themes where is_default limit 1`)
}

// DisplayNamesOf — имена игроков за столом одним запросом.
//
// ⚠️ Java читает пользователя на КАЖДОЕ место по отдельности; за пятиместным столом это
// пять запросов вместо одного. Здесь один — поведение то же, а список лобби перестаёт
// упираться в число столов, помноженное на число мест.
//
// Отсутствующего в ответе нет вовсе: подставить прочерк — дело вызывающего.
func (r Tables) DisplayNamesOf(ctx context.Context, userIDs []string) (map[string]string, error) {
	names := make(map[string]string, len(userIDs))
	if len(userIDs) == 0 {
		return names, nil
	}
	rows, err := r.pool.Query(ctx,
		`select id, display_name from users where id = any($1::uuid[])`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("имена игроков: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("чтение имени игрока: %w", err)
		}
		names[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("имена игроков: %w", err)
	}
	return names, nil
}

func (r Tables) oneTable(ctx context.Context, query string, args ...any) (GameTable, error) {
	table, err := scanTable(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GameTable{}, ErrNotFound
		}
		return GameTable{}, fmt.Errorf("чтение стола: %w", err)
	}
	return table, nil
}

func (r Tables) oneSeat(ctx context.Context, query string, args ...any) (TablePlayer, error) {
	seat, err := scanSeat(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TablePlayer{}, ErrNotFound
		}
		return TablePlayer{}, fmt.Errorf("чтение места: %w", err)
	}
	return seat, nil
}

func (r Tables) oneID(ctx context.Context, query string) (string, error) {
	var id string
	if err := r.pool.QueryRow(ctx, query).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("чтение значения по умолчанию: %w", err)
	}
	return id, nil
}

func scanTable(row scannable) (GameTable, error) {
	var table GameTable
	err := row.Scan(&table.ID, &table.Code, &table.Name, &table.HostUserID, &table.MaxPlayers,
		&table.Status, &table.CardSetID, &table.ThemeID, &table.RulesConfig, &table.IsPrivate,
		&table.Version, &table.CreatedAt, &table.ClosedAt)
	return table, err
}

func scanSeat(row scannable) (TablePlayer, error) {
	var seat TablePlayer
	err := row.Scan(&seat.TableID, &seat.UserID, &seat.SeatNo, &seat.State, &seat.JoinedAt)
	return seat, err
}

// seatConflict переводит имя нарушенного индекса в понятный отказ.
//
// ⚠️ Имена индексов — из миграций V2 и V9. Неизвестное имя не выдаём за известное:
// общий ErrConflict честнее, чем уверенный, но неверный диагноз.
func seatConflict(constraint string) error {
	switch constraint {
	case "table_players_table_id_seat_no_key":
		return ErrSeatTaken
	case "ux_table_players_user":
		return ErrSeatedElsewhere
	case "table_players_pkey":
		return ErrAlreadySeated
	default:
		return ErrConflict
	}
}

// uniqueViolationOf — код 23505 и имя ограничения, на котором упали.
//
// Разбор по КОДУ, а не по тексту сообщения: текст зависит от локали сервера и меняется
// между версиями Postgres.
func uniqueViolationOf(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName, true
	}
	return "", false
}

func defaultRules(rules string) string {
	if rules == "" {
		return "{}"
	}
	return rules
}

func defaultSeatState(state string) string {
	if state == "" {
		return SeatJoined
	}
	return state
}
