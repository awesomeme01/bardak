package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RefreshToken — строка таблицы refresh_tokens.
//
// ⚠️ В базе лежит только ХЕШ токена, самого токена нет нигде. Утечка дампа не даёт
// возможности войти — это и есть смысл хранить хеш, а не значение.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	UserAgent *string
	CreatedAt time.Time
}

// IsUsable — токен ещё годен: не отозван и не просрочен.
func (t RefreshToken) IsUsable(now time.Time) bool {
	return t.RevokedAt == nil && t.ExpiresAt.After(now)
}

// RefreshTokens — репозиторий refresh-токенов.
type RefreshTokens struct{ pool *pgxpool.Pool }

// NewRefreshTokens собирает репозиторий.
func NewRefreshTokens(pool *pgxpool.Pool) RefreshTokens { return RefreshTokens{pool: pool} }

const refreshColumns = `id, user_id, token_hash, expires_at, revoked_at, user_agent, created_at`

// Insert сохраняет выданный токен.
func (r RefreshTokens) Insert(ctx context.Context, token RefreshToken) error {
	const query = `insert into refresh_tokens (id, user_id, token_hash, expires_at, user_agent)
	               values ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, query, token.ID, token.UserID, token.TokenHash,
		token.ExpiresAt, token.UserAgent)
	if err != nil {
		return fmt.Errorf("сохранение refresh-токена: %w", err)
	}
	return nil
}

// FindByHash — поиск по хешу.
func (r RefreshTokens) FindByHash(ctx context.Context, hash string) (RefreshToken, error) {
	row := r.pool.QueryRow(ctx, `select `+refreshColumns+` from refresh_tokens where token_hash = $1`, hash)

	var token RefreshToken
	err := row.Scan(&token.ID, &token.UserID, &token.TokenHash, &token.ExpiresAt,
		&token.RevokedAt, &token.UserAgent, &token.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RefreshToken{}, ErrNotFound
		}
		return RefreshToken{}, fmt.Errorf("чтение refresh-токена: %w", err)
	}
	return token, nil
}

// Revoke гасит один токен. Повторный вызов безвреден: отзыв идемпотентен.
func (r RefreshTokens) Revoke(ctx context.Context, id string, at time.Time) error {
	_, err := r.pool.Exec(ctx,
		`update refresh_tokens set revoked_at = $2 where id = $1 and revoked_at is null`, id, at)
	if err != nil {
		return fmt.Errorf("отзыв refresh-токена: %w", err)
	}
	return nil
}

// RevokeAllOf гасит все токены игрока.
//
// ⭐ Зовётся при смене пароля и при подозрении на кражу: предъявленный отозванный токен
// означает, что им завладел кто-то ещё, и вся серия перестаёт работать разом.
//
// ⚠️ Access-токены при этом НЕ инвалидируются — отзывного списка для них нет вовсе,
// и текущая вкладка живёт до конца своих пятнадцати минут. Так же и в Java.
func (r RefreshTokens) RevokeAllOf(ctx context.Context, userID string, at time.Time) error {
	_, err := r.pool.Exec(ctx,
		`update refresh_tokens set revoked_at = $2 where user_id = $1 and revoked_at is null`,
		userID, at)
	if err != nil {
		return fmt.Errorf("отзыв серии токенов: %w", err)
	}
	return nil
}
