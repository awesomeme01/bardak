package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Состояния пары. Отказ строку удаляет, поэтому третьего состояния нет.
const (
	// FriendshipPending — заявка ждёт ответа.
	FriendshipPending = "PENDING"
	// FriendshipAccepted — дружба состоялась.
	FriendshipAccepted = "ACCEPTED"
)

// Friendship — дружба или заявка на неё: ОДНА строка на пару.
//
// ⭐ Пара нормализована: в LowUserID всегда «меньший» из двух идентификаторов. Благодаря
// этому «А дружит с Б» и «Б дружит с А» — физически одна и та же строка, и рассинхрон,
// при котором человек у тебя в друзьях, а ты у него нет, невозможен.
//
// Кто кого позвал, хранится отдельно в RequestedBy: порядок в ключе — про сортировку,
// а не про отношения.
type Friendship struct {
	LowUserID   string
	HighUserID  string
	RequestedBy string
	Status      string
	CreatedAt   time.Time
	DecidedAt   *time.Time
}

// IsAccepted — дружба состоялась, а не висит заявкой.
func (f Friendship) IsAccepted() bool { return f.Status == FriendshipAccepted }

// Involves — участвует ли игрок в паре.
func (f Friendship) Involves(userID string) bool {
	id := normalizeUUID(userID)
	return f.LowUserID == id || f.HighUserID == id
}

// OtherThan — второй участник пары. Спрашивают всегда «а кто там со мной».
func (f Friendship) OtherThan(userID string) string {
	if f.LowUserID == normalizeUUID(userID) {
		return f.HighUserID
	}
	return f.LowUserID
}

// CanBeAcceptedBy — заявку принимает тот, кто её не отправлял.
func (f Friendship) CanBeAcceptedBy(userID string) bool {
	id := normalizeUUID(userID)
	return f.Status == FriendshipPending && f.Involves(id) && f.RequestedBy != id
}

// FriendProfile — второй участник пары так, как его показывают в списке друзей.
//
// Пароль и почта сюда намеренно не попадают: список друзей читается целиком и часто,
// и таскать по сети хеш пароля ради имени и мордочки незачем.
type FriendProfile struct {
	ID          string
	Username    string
	DisplayName string
	Avatar      *string
}

// FriendPair — строка пары вместе с профилем второго участника.
type FriendPair struct {
	Pair  Friendship
	Other FriendProfile
}

// ComparePairOrder — порядок пары, ТОТ ЖЕ, что у Postgres.
//
// ⚠️ Здесь нельзя сравнивать UUID как числа: Java UUID.compareTo сравнивает два ЗНАКОВЫХ
// long, а Postgres — побайтово. На идентификаторах со старшим единичным битом эти порядки
// противоположны, и строка, «правильная» по мнению Java, падала на проверке
// friendships_ordered (low_user_id < high_user_id).
//
// Сравнение по канонической записи совпадает с побайтовым: шестнадцатеричные цифры
// упорядочены так же, как байты, которые они изображают. Регистр приводится к нижнему —
// Postgres печатает uuid только строчными, а идентификатор мог прийти из пути запроса.
func ComparePairOrder(one, two string) int {
	return strings.Compare(normalizeUUID(one), normalizeUUID(two))
}

// OrderPair раскладывает двоих в порядок ключа: сначала «меньший», потом «больший».
func OrderPair(one, two string) (low, high string) {
	if ComparePairOrder(one, two) < 0 {
		return normalizeUUID(one), normalizeUUID(two)
	}
	return normalizeUUID(two), normalizeUUID(one)
}

func normalizeUUID(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// Friendships — репозиторий дружб и заявок.
type Friendships struct{ pool *pgxpool.Pool }

// NewFriendships собирает репозиторий поверх пула.
func NewFriendships(pool *pgxpool.Pool) Friendships { return Friendships{pool: pool} }

const friendshipColumns = `low_user_id, high_user_id, requested_by, status, created_at, decided_at`

// FindPair — пара двоих, в каком бы порядке их ни назвали.
//
// ⚠️ Порядок ключа считается тем же способом, что и при записи, — иначе половина пар
// (те, где идентификаторы «перевёрнуты») просто не находилась бы.
func (r Friendships) FindPair(ctx context.Context, one, two string) (Friendship, error) {
	low, high := OrderPair(one, two)
	row := r.pool.QueryRow(ctx,
		`select `+friendshipColumns+` from friendships where low_user_id = $1 and high_user_id = $2`,
		low, high)

	pair, err := scanFriendship(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Friendship{}, ErrNotFound
		}
		return Friendship{}, fmt.Errorf("чтение пары друзей: %w", err)
	}
	return pair, nil
}

// IsFriend — состоят ли двое в принятой дружбе.
//
// Спрашивают перед приглашением за стол и перед показом чужой истории матчей: висящая
// заявка правом не считается — согласия ещё не было.
func (r Friendships) IsFriend(ctx context.Context, one, two string) (bool, error) {
	low, high := OrderPair(one, two)
	var exists bool
	err := r.pool.QueryRow(ctx,
		`select exists(select 1 from friendships
		               where low_user_id = $1 and high_user_id = $2 and status = $3)`,
		low, high, FriendshipAccepted).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("проверка дружбы: %w", err)
	}
	return exists, nil
}

// FindAllInvolving — все пары игрока, и дружбы, и висящие заявки, сразу с профилями.
//
// ⭐ Один запрос вместо «выбрать пары, потом выбрать людей»: список читается целиком
// и часто, и второй заход в базу здесь ничего не добавляет.
//
// ⚠️ Соединение внутреннее: пара, у которой второй участник в users не найден, молча
// пропускается — ровно как в Java, где такой пары просто нет в карте профилей.
func (r Friendships) FindAllInvolving(ctx context.Context, userID string) ([]FriendPair, error) {
	const query = `select f.low_user_id, f.high_user_id, f.requested_by, f.status,
	                      f.created_at, f.decided_at,
	                      u.id, u.username, u.display_name, u.avatar
	               from friendships f
	               join users u
	                 on u.id = case when f.low_user_id = $1 then f.high_user_id else f.low_user_id end
	               where f.low_user_id = $1 or f.high_user_id = $1
	               order by f.created_at, u.username`

	rows, err := r.pool.Query(ctx, query, normalizeUUID(userID))
	if err != nil {
		return nil, fmt.Errorf("чтение списка друзей: %w", err)
	}
	defer rows.Close()

	// ⚠️ Пустой список — именно []FriendPair{}, а не nil: наверху он уходит в JSON,
	// а nil сериализовался бы в null, чего Java не отдаёт никогда (MD-003).
	pairs := []FriendPair{}
	for rows.Next() {
		var item FriendPair
		err := rows.Scan(&item.Pair.LowUserID, &item.Pair.HighUserID, &item.Pair.RequestedBy,
			&item.Pair.Status, &item.Pair.CreatedAt, &item.Pair.DecidedAt,
			&item.Other.ID, &item.Other.Username, &item.Other.DisplayName, &item.Other.Avatar)
		if err != nil {
			return nil, fmt.Errorf("разбор строки друга: %w", err)
		}
		pairs = append(pairs, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение списка друзей: %w", err)
	}
	return pairs, nil
}

// Insert заводит заявку от одного к другому.
//
// Порядок ключа выравнивается здесь, а не у вызывающего: перепутать его — значит упереться
// в проверку friendships_ordered, причём ровно в половине случаев.
func (r Friendships) Insert(ctx context.Context, from, to string) (Friendship, error) {
	low, high := OrderPair(from, to)
	// created_at ставит база — как в Java (insertable = false).
	const query = `insert into friendships (low_user_id, high_user_id, requested_by, status)
	               values ($1, $2, $3, $4)
	               returning ` + friendshipColumns

	pair, err := scanFriendship(r.pool.QueryRow(ctx, query, low, high, normalizeUUID(from), FriendshipPending))
	if err != nil {
		// Двое, нажавшие «добавить» одновременно, доходят сюда вдвоём: правду держит
		// первичный ключ, а не предварительная проверка «пары ещё нет».
		if isUniqueViolation(err) {
			return Friendship{}, ErrConflict
		}
		return Friendship{}, fmt.Errorf("создание заявки в друзья: %w", err)
	}
	return pair, nil
}

// Accept переводит пару в принятую.
func (r Friendships) Accept(ctx context.Context, one, two string, at time.Time) (Friendship, error) {
	low, high := OrderPair(one, two)
	const query = `update friendships set status = $3, decided_at = $4
	               where low_user_id = $1 and high_user_id = $2
	               returning ` + friendshipColumns

	pair, err := scanFriendship(r.pool.QueryRow(ctx, query, low, high, FriendshipAccepted, at))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Friendship{}, ErrNotFound
		}
		return Friendship{}, fmt.Errorf("принятие заявки: %w", err)
	}
	return pair, nil
}

// Delete убирает пару насовсем.
//
// ⭐ Убрать из друзей и отклонить заявку — одна и та же операция: и то и другое означает,
// что пары больше нет. Хранить отказы значило бы помнить, кому уже отказали, а это память
// не про игру.
func (r Friendships) Delete(ctx context.Context, one, two string) error {
	low, high := OrderPair(one, two)
	tag, err := r.pool.Exec(ctx,
		`delete from friendships where low_user_id = $1 and high_user_id = $2`, low, high)
	if err != nil {
		return fmt.Errorf("удаление пары друзей: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanFriendship(row scannable) (Friendship, error) {
	var pair Friendship
	err := row.Scan(&pair.LowUserID, &pair.HighUserID, &pair.RequestedBy, &pair.Status,
		&pair.CreatedAt, &pair.DecidedAt)
	return pair, err
}
