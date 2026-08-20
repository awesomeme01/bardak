package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Каталог: наборы карт, их картинки и темы стола.
//
// ⭐ Движок о наборах не знает вообще (ADR-009): он оперирует кодами карт, а картинку
// по коду находит клиент через манифест. Поэтому здесь только чтение — правила игры
// сюда не заглядывают ни разу.

// CardSet — строка таблицы card_sets.
//
// ⚠️ Колонки is_public, owner_user_id и created_at Java НЕ мэппит, и наружу они не
// уходят. Здесь их тоже нет: лишнее поле в структуре — это приглашение однажды отдать
// его клиенту и разойтись с Java.
type CardSet struct {
	ID          string
	Code        string
	Name        string
	Description *string
	Version     string
	PreviewURL  *string
	IsDefault   bool
}

// CardAsset — одна картинка набора: код карты → ссылка.
//
// ⚠️ Алфавит card_code не совпадает с кодами движка: джокер здесь один — «Joker»,
// а движок кодирует их номером (Joker-1, Joker-2). Схлопывание делает клиент.
// Ещё есть «back» — рубашка, которая кодом карты не является вовсе.
type CardAsset struct {
	CardSetID string
	CardCode  string
	AssetURL  string
	Mime      string
	Ordinal   int16
}

// TableTheme — строка таблицы table_themes.
//
// ⚠️ background_url и preview_url наружу не отдаются (в Java у них нет геттера и места
// в DTO), поэтому их здесь нет.
type TableTheme struct {
	ID              string
	Code            string
	Name            string
	FeltColor       *string
	DefaultBackCode *string
	IsDefault       bool
}

// CardSets — репозиторий наборов карт и их картинок.
//
// Две таблицы в одном репозитории намеренно: card_assets без своего набора смысла
// не имеет (внешний ключ с ON DELETE CASCADE), и отдельный тип только добавил бы
// координатору ещё одну зависимость на ровном месте.
type CardSets struct{ pool *pgxpool.Pool }

// NewCardSets собирает репозиторий поверх пула.
func NewCardSets(pool *pgxpool.Pool) CardSets { return CardSets{pool: pool} }

const cardSetColumns = `id, code, name, description, version, preview_url, is_default`

// ListOrderByName — весь каталог, сортировка по имени.
//
// ⚠️ Сортирует БАЗА, а не Go: в Java это findAllByOrderByNameAsc, то есть тот же
// order by с той же коллацией. Пересортировка на стороне приложения дала бы другой
// порядок для кириллицы, и списки двух бэкендов разошлись бы.
func (r CardSets) ListOrderByName(ctx context.Context) ([]CardSet, error) {
	rows, err := r.pool.Query(ctx, `select `+cardSetColumns+` from card_sets order by name asc`)
	if err != nil {
		return nil, fmt.Errorf("список наборов карт: %w", err)
	}
	defer rows.Close()

	sets := make([]CardSet, 0)
	for rows.Next() {
		set, err := scanCardSet(rows)
		if err != nil {
			return nil, fmt.Errorf("чтение набора карт: %w", err)
		}
		sets = append(sets, set)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("список наборов карт: %w", err)
	}
	return sets, nil
}

// FindByID — набор по идентификатору. Нет строки — ErrNotFound.
func (r CardSets) FindByID(ctx context.Context, id string) (CardSet, error) {
	return r.oneCardSet(ctx, `select `+cardSetColumns+` from card_sets where id = $1`, id)
}

// FindDefault — набор по умолчанию.
//
// ⭐ Он ровно один: в базе стоит частичный уникальный индекс (is_default) where is_default.
// Поэтому здесь именно «одна строка или ErrNotFound», а не «первая попавшаяся».
func (r CardSets) FindDefault(ctx context.Context) (CardSet, error) {
	return r.oneCardSet(ctx, `select `+cardSetColumns+` from card_sets where is_default limit 1`)
}

// AssetsOf — картинки набора В ПОРЯДКЕ ordinal ASC.
//
// ⚠️ Порядок ЗНАЧИМ и является частью контракта манифеста: клиент раскладывает по нему
// колоду. Сортировку делает база; в Go порядок обязан дожить до JSON, поэтому наружу
// уходит срез, а не карта — у карты порядка нет вовсе.
func (r CardSets) AssetsOf(ctx context.Context, cardSetID string) ([]CardAsset, error) {
	const query = `select card_set_id, card_code, asset_url, mime, ordinal
	               from card_assets where card_set_id = $1 order by ordinal asc`
	rows, err := r.pool.Query(ctx, query, cardSetID)
	if err != nil {
		return nil, fmt.Errorf("картинки набора: %w", err)
	}
	defer rows.Close()

	assets := make([]CardAsset, 0)
	for rows.Next() {
		var asset CardAsset
		if err := rows.Scan(&asset.CardSetID, &asset.CardCode, &asset.AssetURL,
			&asset.Mime, &asset.Ordinal); err != nil {
			return nil, fmt.Errorf("чтение картинки набора: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("картинки набора: %w", err)
	}
	return assets, nil
}

func (r CardSets) oneCardSet(ctx context.Context, query string, args ...any) (CardSet, error) {
	set, err := scanCardSet(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CardSet{}, ErrNotFound
		}
		return CardSet{}, fmt.Errorf("чтение набора карт: %w", err)
	}
	return set, nil
}

func scanCardSet(row scannable) (CardSet, error) {
	var set CardSet
	err := row.Scan(&set.ID, &set.Code, &set.Name, &set.Description, &set.Version,
		&set.PreviewURL, &set.IsDefault)
	return set, err
}

// TableThemes — репозиторий тем стола.
type TableThemes struct{ pool *pgxpool.Pool }

// NewTableThemes собирает репозиторий поверх пула.
func NewTableThemes(pool *pgxpool.Pool) TableThemes { return TableThemes{pool: pool} }

const tableThemeColumns = `id, code, name, felt_color, default_back_code, is_default`

// ListOrderByName — все темы, сортировка по имени (сортирует база, как в Java).
func (r TableThemes) ListOrderByName(ctx context.Context) ([]TableTheme, error) {
	rows, err := r.pool.Query(ctx, `select `+tableThemeColumns+` from table_themes order by name asc`)
	if err != nil {
		return nil, fmt.Errorf("список тем стола: %w", err)
	}
	defer rows.Close()

	themes := make([]TableTheme, 0)
	for rows.Next() {
		theme, err := scanTableTheme(rows)
		if err != nil {
			return nil, fmt.Errorf("чтение темы стола: %w", err)
		}
		themes = append(themes, theme)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("список тем стола: %w", err)
	}
	return themes, nil
}

// FindByID — тема по идентификатору. Нет строки — ErrNotFound.
func (r TableThemes) FindByID(ctx context.Context, id string) (TableTheme, error) {
	return r.oneTableTheme(ctx, `select `+tableThemeColumns+` from table_themes where id = $1`, id)
}

// FindDefault — тема по умолчанию; она тоже ровно одна (частичный уникальный индекс).
func (r TableThemes) FindDefault(ctx context.Context) (TableTheme, error) {
	return r.oneTableTheme(ctx, `select `+tableThemeColumns+` from table_themes where is_default limit 1`)
}

func (r TableThemes) oneTableTheme(ctx context.Context, query string, args ...any) (TableTheme, error) {
	theme, err := scanTableTheme(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TableTheme{}, ErrNotFound
		}
		return TableTheme{}, fmt.Errorf("чтение темы стола: %w", err)
	}
	return theme, nil
}

func scanTableTheme(row scannable) (TableTheme, error) {
	var theme TableTheme
	err := row.Scan(&theme.ID, &theme.Code, &theme.Name, &theme.FeltColor,
		&theme.DefaultBackCode, &theme.IsDefault)
	return theme, err
}
