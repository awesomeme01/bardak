package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Каталог против НАСТОЯЩЕГО Postgres со схемой и сидом Java.
//
// ⚠️ Главное здесь принадлежит базе, а не Go: порядок картинок (order by ordinal),
// частичный уникальный индекс «набор по умолчанию ровно один» и сид V2. На подделке
// ничего из этого не проверить.

const (
	seedCardSetID    = "11111111-1111-1111-1111-111111111111"
	seedTableThemeID = "22222222-2222-2222-2222-222222222222"
)

func TestCardSetsListContainsSeededSet(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	sets, err := NewCardSets(pool).ListOrderByName(ctx)
	if err != nil {
		t.Fatalf("список наборов: %v", err)
	}
	if len(sets) == 0 {
		t.Fatal("список наборов пуст, а сид V2 кладёт classic")
	}

	var classic *CardSet
	for i := range sets {
		if sets[i].Code == "classic" {
			classic = &sets[i]
		}
	}
	if classic == nil {
		t.Fatal("набор classic из сида не найден")
	}
	if classic.ID != seedCardSetID {
		t.Errorf("id набора classic %q, ждали %q", classic.ID, seedCardSetID)
	}
	if !classic.IsDefault {
		t.Error("classic обязан быть набором по умолчанию — на нём висит частичный уникальный индекс")
	}
	if classic.Version != "1.0.0" {
		t.Errorf("версия манифеста %q, ждали 1.0.0", classic.Version)
	}
	if classic.PreviewURL == nil {
		t.Error("preview_url у classic заполнен в сиде, а прочитался как null")
	}
}

// ⭐ Порядок картинок — часть контракта манифеста: клиент по нему раскладывает колоду.
// Сортирует база (order by ordinal asc), и тест проверяет именно её, а не пересортировку.
func TestCardAssetsComeInOrdinalOrder(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	assets, err := NewCardSets(pool).AssetsOf(ctx, seedCardSetID)
	if err != nil {
		t.Fatalf("картинки набора: %v", err)
	}
	// 52 карты перебором рангов и мастей плюс Joker и back.
	if len(assets) != 54 {
		t.Fatalf("картинок %d, ждали 54 (52 карты + Joker + back)", len(assets))
	}

	for i := 1; i < len(assets); i++ {
		if assets[i].Ordinal < assets[i-1].Ordinal {
			t.Fatalf("порядок сбился на позиции %d: ordinal %d идёт после %d",
				i, assets[i].Ordinal, assets[i-1].Ordinal)
		}
	}
	// Рубашка «back» — последняя (ordinal 201) и кодом карты движка не является.
	last := assets[len(assets)-1]
	if last.CardCode != "back" {
		t.Errorf("последней ждали рубашку back, получили %q", last.CardCode)
	}
	if last.Mime != "image/png" {
		t.Errorf("mime рубашки %q, ждали image/png", last.Mime)
	}
}

func TestCardSetLookupByID(t *testing.T) {
	pool := testDB(t)
	sets := NewCardSets(pool)
	ctx := context.Background()

	set, err := sets.FindByID(ctx, seedCardSetID)
	if err != nil {
		t.Fatalf("набор по id: %v", err)
	}
	if set.Code != "classic" {
		t.Errorf("код набора %q, ждали classic", set.Code)
	}

	// ⚠️ Несуществующий набор — это ErrNotFound, из которого транспорт делает
	// 404 CARD_SET_NOT_FOUND. Молчаливый пустой манифест был бы хуже: клиент показал
	// бы стол без карт и не понял, почему.
	if _, err := sets.FindByID(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Errorf("отсутствующий набор должен давать ErrNotFound, получили %v", err)
	}
	// У несуществующего набора и картинок нет — пустой срез, а не ошибка.
	assets, err := sets.AssetsOf(ctx, uuid.NewString())
	if err != nil {
		t.Fatalf("картинки несуществующего набора: %v", err)
	}
	if len(assets) != 0 {
		t.Errorf("ждали пусто, получили %d картинок", len(assets))
	}
}

func TestDefaultsAreExactlyOne(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	set, err := NewCardSets(pool).FindDefault(ctx)
	if err != nil {
		t.Fatalf("набор по умолчанию: %v", err)
	}
	if set.ID != seedCardSetID {
		t.Errorf("набор по умолчанию %q, ждали %q", set.ID, seedCardSetID)
	}

	theme, err := NewTableThemes(pool).FindDefault(ctx)
	if err != nil {
		t.Fatalf("тема по умолчанию: %v", err)
	}
	if theme.ID != seedTableThemeID {
		t.Errorf("тема по умолчанию %q, ждали %q", theme.ID, seedTableThemeID)
	}
}

func TestTableThemesList(t *testing.T) {
	pool := testDB(t)
	themes := NewTableThemes(pool)
	ctx := context.Background()

	list, err := themes.ListOrderByName(ctx)
	if err != nil {
		t.Fatalf("список тем: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("список тем пуст, а сид V2 кладёт green-felt")
	}

	theme, err := themes.FindByID(ctx, seedTableThemeID)
	if err != nil {
		t.Fatalf("тема по id: %v", err)
	}
	if theme.Code != "green-felt" {
		t.Errorf("код темы %q, ждали green-felt", theme.Code)
	}
	if theme.FeltColor == nil || *theme.FeltColor != "#1f6f43" {
		t.Error("цвет сукна из сида не прочитался")
	}
	if theme.DefaultBackCode == nil || *theme.DefaultBackCode != "back" {
		t.Error("рубашка по умолчанию из сида не прочиталась")
	}

	if _, err := themes.FindByID(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Errorf("отсутствующая тема должна давать ErrNotFound, получили %v", err)
	}
}
