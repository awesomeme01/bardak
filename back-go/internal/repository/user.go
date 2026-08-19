// Package repository — доступ к PostgreSQL.
//
// ⭐ Схемой владеет Flyway из Java-бэкенда, и Go в неё НЕ мигрирует (MD-004). В окне
// отката обе версии работают с одной базой; два мигратора на одну схему — это две
// служебные таблицы со своим представлением о накатанном, и первое же расхождение
// чинится руками в проде.
//
// ⭐ Между сущностями нет ни одной ассоциации ORM: связи выражены голыми UUID-колонками,
// внешние ключи живут только в базе. Поэтому каждый репозиторий работает со своей
// таблицей, без каскадов и ленивой загрузки.
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

// ErrNotFound — строки нет. Отдельная ошибка, чтобы вызывающий не разбирал текст.
var ErrNotFound = errors.New("не найдено")

// ErrConflict — нарушен уникальный индекс.
//
// ⚠️ Правду про занятость логина держит БАЗА, а не предварительная проверка: проверка
// и вставка не атомарны, и двое, регистрирующих один логин одновременно, оба её проходят.
var ErrConflict = errors.New("нарушено ограничение уникальности")

// User — строка таблицы users.
//
// Поля названы как колонки, а не «покрасивее»: сверять с базой придётся глазами,
// и лишний слой переименований этому только мешает.
type User struct {
	ID           string
	Username     string
	DisplayName  string
	Email        *string
	PasswordHash string
	AvatarURL    *string
	Avatar       *string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Users — репозиторий пользователей.
type Users struct{ pool *pgxpool.Pool }

// NewUsers собирает репозиторий поверх пула.
func NewUsers(pool *pgxpool.Pool) Users { return Users{pool: pool} }

const userColumns = `id, username, display_name, email, password_hash,
	avatar_url, avatar, status, created_at, updated_at`

// FindByUsernameIgnoreCase — поиск логина БЕЗ учёта регистра.
//
// ⚠️ Это не удобство, а совместимость: в базе стоит уникальный индекс по lower(username)
// (V10), и вход по «Shabdan» обязан находить «shabdan». Строгое сравнение однажды уже
// не пускало войти с верными данными.
func (r Users) FindByUsernameIgnoreCase(ctx context.Context, username string) (User, error) {
	return r.one(ctx, `select `+userColumns+` from users where lower(username) = lower($1)`, username)
}

// FindByID — пользователь по идентификатору.
func (r Users) FindByID(ctx context.Context, id string) (User, error) {
	return r.one(ctx, `select `+userColumns+` from users where id = $1`, id)
}

// ExistsByUsernameIgnoreCase — занят ли логин.
//
// ⚠️ Ответ устаревает сразу же: между проверкой и вставкой логин может занять другой.
// Опираться на него как на гарантию нельзя — гарантию даёт только уникальный индекс.
func (r Users) ExistsByUsernameIgnoreCase(ctx context.Context, username string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`select exists(select 1 from users where lower(username) = lower($1))`, username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("проверка логина: %w", err)
	}
	return exists, nil
}

// Insert заводит пользователя.
//
// Возвращает ErrConflict, если логин уже занят: вызывающий превращает это в понятный
// игроку отказ, а не в «что-то пошло не так».
func (r Users) Insert(ctx context.Context, user User) (User, error) {
	// created_at и updated_at ставит база — так же, как в Java (insertable = false).
	const query = `insert into users (id, username, display_name, email, password_hash, avatar, status)
	               values ($1, $2, $3, $4, $5, $6, $7)
	               returning ` + userColumns

	row := r.pool.QueryRow(ctx, query, user.ID, user.Username, user.DisplayName,
		user.Email, user.PasswordHash, user.Avatar, defaultStatus(user.Status))

	saved, err := scanUser(row)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrConflict
		}
		return User{}, fmt.Errorf("создание пользователя: %w", err)
	}
	return saved, nil
}

// UpdateProfile меняет имя за столом и мордочку. Логин не меняется: по нему входят.
func (r Users) UpdateProfile(ctx context.Context, id, displayName string, avatar *string) (User, error) {
	const query = `update users set display_name = $2, avatar = $3 where id = $1
	               returning ` + userColumns
	user, err := r.one(ctx, query, id, displayName, avatar)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// UpdatePasswordHash меняет пароль.
func (r Users) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	tag, err := r.pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, id, hash)
	if err != nil {
		return fmt.Errorf("смена пароля: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r Users) one(ctx context.Context, query string, args ...any) (User, error) {
	user, err := scanUser(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("чтение пользователя: %w", err)
	}
	return user, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanUser(row scannable) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email,
		&user.PasswordHash, &user.AvatarURL, &user.Avatar, &user.Status,
		&user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func defaultStatus(status string) string {
	if status == "" {
		return "ACTIVE"
	}
	return status
}

// isUniqueViolation — код 23505 из PostgreSQL.
//
// Разбор по КОДУ, а не по тексту: текст зависит от локали сервера и меняется между
// версиями, и проверка по нему тихо перестанет работать.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
