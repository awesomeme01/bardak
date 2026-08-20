package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/awesomeme01/bardak/back-go/internal/repository"
)

// CardSetView — набор карт в списке каталога.
//
// ⚠️ id — строка, а не UUID: так объявлено в Java (MD-002). На проводе разницы нет,
// но «причёсывать» типы до отключения Java нельзя — иначе не сказать «отличий нет».
type CardSetView struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Version     string  `json:"version"`
	PreviewURL  *string `json:"previewUrl,omitempty"`
	IsDefault   bool    `json:"isDefault"`
}

// TableThemeView — тема стола в списке каталога.
//
// ⚠️ background_url у темы в базе есть, но наружу не отдаётся — ни в Java, ни здесь.
type TableThemeView struct {
	ID              string  `json:"id"`
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	FeltColor       *string `json:"feltColor,omitempty"`
	DefaultBackCode *string `json:"defaultBackCode,omitempty"`
	IsDefault       bool    `json:"isDefault"`
}

// CardSetManifestView — манифест набора: код карты → ссылка на картинку.
//
// ⭐ Манифест — единственное, что связывает код карты с картинкой (ADR-009). Ни движок,
// ни протокол игры про URL не знают, поэтому поменять дизайн колоды можно, не трогая
// правила и не выкатывая новый протокол.
type CardSetManifestView struct {
	ID      string        `json:"id"`
	Code    string        `json:"code"`
	Version string        `json:"version"`
	Cards   ManifestCards `json:"cards"`
}

// ManifestCard — одна пара «код карты → ссылка».
type ManifestCard struct {
	Code string
	URL  string
}

// ManifestCards — пары манифеста В ПОРЯДКЕ card_assets.ordinal ASC.
//
// ⚠️ Это срез, а НЕ карта, и именно поэтому: в Java здесь LinkedHashMap, порядок ключей
// значим и воспроизводим, а map[string]string в Go при сериализации сортируется по ключу.
// Порядок «10-hearts, 6-diamonds, …» вместо «6-diamonds, …, 10-hearts» ничего не сломает
// с виду, но перестанет совпадать с Java побайтно — а именно этим проверяется миграция.
type ManifestCards []ManifestCard

// MarshalJSON пишет объект JSON в том порядке, в каком пары лежат в срезе.
//
// Пустой набор даёт `{}`, как пустой LinkedHashMap в Java, а не `null`.
func (c ManifestCards) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, card := range c {
		if i > 0 {
			out.WriteByte(',')
		}
		key, err := json.Marshal(card.Code)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(card.URL)
		if err != nil {
			return nil, err
		}
		out.Write(key)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

// CatalogHandlers — каталог наборов карт и тем стола.
type CatalogHandlers struct {
	CardSets repository.CardSets
	Themes   repository.TableThemes
	Log      *slog.Logger
}

// Routes вешает пути.
//
// ⭐ Все три ручки открыты БЕЗ токена: экран входа показывает карты, а это ссылки на
// картинки, которые и так лежат в /assets. Требовать токен — запирать витрину, а не данные.
//
// ⚠️ Раз токена нет, ни один обработчик здесь не читает Principal — это инвариант,
// который легко нарушить случайно: в Java на открытой ручке он был бы просто null.
func (h CatalogHandlers) Routes(router chi.Router) {
	router.Get("/api/card-sets", h.cardSets)
	router.Get("/api/card-sets/{id}/manifest", h.manifest)
	router.Get("/api/table-themes", h.tableThemes)
}

func (h CatalogHandlers) cardSets(w http.ResponseWriter, r *http.Request) {
	sets, err := h.CardSets.ListOrderByName(r.Context())
	if err != nil {
		WriteError(w, r, h.Log, ErrInternal)
		return
	}

	// Пустой каталог — это [], а не null: Jackson отдаёт пустой список как есть.
	views := make([]CardSetView, 0, len(sets))
	for _, set := range sets {
		views = append(views, toCardSetView(set))
	}
	WriteJSON(w, http.StatusOK, views)
}

func (h CatalogHandlers) manifest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		// ⚠️ 400, а не 500: починено в Java (ApiHardeningIT) — клиент должен отличать
		// свою ошибку от поломки сервера.
		WriteError(w, r, h.Log, ErrBadRequest)
		return
	}

	set, err := h.CardSets.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			WriteError(w, r, h.Log, NewFault(http.StatusNotFound,
				"CARD_SET_NOT_FOUND", "Набор карт не найден"))
			return
		}
		WriteError(w, r, h.Log, ErrInternal)
		return
	}

	assets, err := h.CardSets.AssetsOf(r.Context(), id)
	if err != nil {
		WriteError(w, r, h.Log, ErrInternal)
		return
	}

	cards := make(ManifestCards, 0, len(assets))
	for _, asset := range assets {
		cards = append(cards, ManifestCard{Code: asset.CardCode, URL: asset.AssetURL})
	}
	WriteJSON(w, http.StatusOK, CardSetManifestView{
		ID: set.ID, Code: set.Code, Version: set.Version, Cards: cards,
	})
}

func (h CatalogHandlers) tableThemes(w http.ResponseWriter, r *http.Request) {
	themes, err := h.Themes.ListOrderByName(r.Context())
	if err != nil {
		WriteError(w, r, h.Log, ErrInternal)
		return
	}

	views := make([]TableThemeView, 0, len(themes))
	for _, theme := range themes {
		views = append(views, toTableThemeView(theme))
	}
	WriteJSON(w, http.StatusOK, views)
}

func toCardSetView(set repository.CardSet) CardSetView {
	return CardSetView{
		ID:          set.ID,
		Code:        set.Code,
		Name:        set.Name,
		Description: set.Description,
		Version:     set.Version,
		PreviewURL:  set.PreviewURL,
		IsDefault:   set.IsDefault,
	}
}

func toTableThemeView(theme repository.TableTheme) TableThemeView {
	return TableThemeView{
		ID:              theme.ID,
		Code:            theme.Code,
		Name:            theme.Name,
		FeltColor:       theme.FeltColor,
		DefaultBackCode: theme.DefaultBackCode,
		IsDefault:       theme.IsDefault,
	}
}
