package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PushSubscription — подписка УСТРОЙСТВА на уведомления.
//
// ⭐ Подписка принадлежит устройству, а не человеку: телефон и ноутбук — две строки,
// и «твой ход» уходит на оба. Ключи хранятся как есть: ими шифруется полезная нагрузка
// (RFC 8291), и без них уведомление отправить нечем. Сервер их не разбирает.
type PushSubscription struct {
	ID         string
	UserID     string
	Endpoint   string
	P256dh     string
	Auth       string
	UserAgent  *string
	CreatedAt  time.Time
	LastSentAt *time.Time
}

// PushSubscriptions — репозиторий подписок.
type PushSubscriptions struct{ pool *pgxpool.Pool }

// NewPushSubscriptions собирает репозиторий поверх пула.
func NewPushSubscriptions(pool *pgxpool.Pool) PushSubscriptions {
	return PushSubscriptions{pool: pool}
}

const pushColumns = `id, user_id, endpoint, p256dh, auth, user_agent, created_at, last_sent_at`

// Save запоминает подписку.
//
// ⭐ Ключ строки — endpoint, а не пара «пользователь + устройство»: адрес выдаёт браузер
// и он же меняет его при обновлении подписки. Один endpoint — одна строка, поэтому
// повторная подписка обновляет существующую, а не плодит дубли, из-за которых игрок
// получал бы один и тот же звонок дважды.
//
// ⭐ Устройство могло перейти к другому игроку — тогда у строки просто меняется владелец:
// звонить на этот телефон прежнему нельзя. id и created_at при этом сохраняются: это то же
// самое устройство, сменились лишь хозяин и ключи.
//
// ⚠️ В Java здесь два запроса (найти по endpoint, потом сохранить) — не от хорошей жизни:
// удалить и вставить в одной транзакции Hibernate не давал. В Go это ровно один
// INSERT … ON CONFLICT, и гонка двух подписок с одного устройства разрешается базой.
func (r PushSubscriptions) Save(ctx context.Context, sub PushSubscription) (PushSubscription, error) {
	const query = `insert into push_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent)
	               values ($1, $2, $3, $4, $5, $6)
	               on conflict (endpoint) do update
	                   set user_id = excluded.user_id, p256dh = excluded.p256dh,
	                       auth = excluded.auth, user_agent = excluded.user_agent
	               returning ` + pushColumns

	row := r.pool.QueryRow(ctx, query, sub.ID, sub.UserID, sub.Endpoint, sub.P256dh,
		sub.Auth, sub.UserAgent)
	saved, err := scanPushSubscription(row)
	if err != nil {
		return PushSubscription{}, fmt.Errorf("сохранение подписки: %w", err)
	}
	return saved, nil
}

// DeleteByEndpointAndUserID отписывает устройство ЕГО ВЛАДЕЛЬЦА.
//
// ⚠️ Владелец в условии обязателен. Пока отписка шла по одному endpoint, любой вошедший,
// знающий чужой адрес подписки, отписывал чужое устройство: оно переставало получать
// уведомления о своём ходе, а хозяин не мог понять, почему.
//
// Отсутствие строки ошибкой не считается — отписка идемпотентна, повторный вызов с того же
// устройства обычное дело. Поэтому возвращается число удалённых, а не ErrNotFound.
func (r PushSubscriptions) DeleteByEndpointAndUserID(ctx context.Context, endpoint, userID string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`delete from push_subscriptions where endpoint = $1 and user_id = $2`, endpoint, userID)
	if err != nil {
		return 0, fmt.Errorf("отписка устройства: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteByEndpoint убирает подписку без оглядки на владельца.
//
// ⚠️ Не для ручки отписки: там владелец обязателен. Это для отправителя — push-сервис
// отвечает 404 или 410, когда подписка мертва, и держать её дальше незачем.
func (r PushSubscriptions) DeleteByEndpoint(ctx context.Context, endpoint string) error {
	_, err := r.pool.Exec(ctx, `delete from push_subscriptions where endpoint = $1`, endpoint)
	if err != nil {
		return fmt.Errorf("удаление мёртвой подписки: %w", err)
	}
	return nil
}

// FindByEndpoint — подписка по адресу. Адрес уникален, поэтому строка не больше одной.
func (r PushSubscriptions) FindByEndpoint(ctx context.Context, endpoint string) (PushSubscription, error) {
	row := r.pool.QueryRow(ctx,
		`select `+pushColumns+` from push_subscriptions where endpoint = $1`, endpoint)
	sub, err := scanPushSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PushSubscription{}, ErrNotFound
		}
		return PushSubscription{}, fmt.Errorf("чтение подписки: %w", err)
	}
	return sub, nil
}

// FindByUserID — все устройства игрока: уведомление уходит на каждое.
func (r PushSubscriptions) FindByUserID(ctx context.Context, userID string) ([]PushSubscription, error) {
	rows, err := r.pool.Query(ctx,
		`select `+pushColumns+` from push_subscriptions where user_id = $1 order by created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("чтение подписок игрока: %w", err)
	}
	defer rows.Close()

	subs := []PushSubscription{}
	for rows.Next() {
		sub, err := scanPushSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("разбор подписки: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение подписок игрока: %w", err)
	}
	return subs, nil
}

// MarkSent отмечает время последней отправки — по нему считается окно тишины.
func (r PushSubscriptions) MarkSent(ctx context.Context, id string, at time.Time) error {
	_, err := r.pool.Exec(ctx, `update push_subscriptions set last_sent_at = $2 where id = $1`, id, at)
	if err != nil {
		return fmt.Errorf("отметка отправки: %w", err)
	}
	return nil
}

func scanPushSubscription(row scannable) (PushSubscription, error) {
	var sub PushSubscription
	err := row.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth,
		&sub.UserAgent, &sub.CreatedAt, &sub.LastSentAt)
	return sub, err
}
