package http

import (
	"encoding/json"
	"testing"
)

// ⭐ Порядок ключей манифеста ЗНАЧИМ (card_assets.ordinal ASC): это единственное, что
// связывает код карты с картинкой. Тест сравнивает JSON целиком строкой, а не разбирает
// его обратно в карту — разбор как раз и потерял бы то, что проверяется.
func TestManifestKeepsCardOrder(t *testing.T) {
	manifest := CardSetManifestView{
		ID:      "11111111-1111-1111-1111-111111111111",
		Code:    "classic",
		Version: "1.0.0",
		Cards: ManifestCards{
			{Code: "6-diamonds", URL: "/assets/card-sets/classic/6-diamonds.png"},
			{Code: "10-hearts", URL: "/assets/card-sets/classic/10-hearts.png"},
			{Code: "Joker", URL: "/assets/card-sets/classic/Joker.png"},
			{Code: "back", URL: "/assets/card-sets/classic/back.png"},
		},
	}

	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("сериализация манифеста: %v", err)
	}

	// ⚠️ «10-hearts» идёт ВТОРЫМ, как пришло из базы. Карта map[string]string
	// отсортировала бы ключи по алфавиту и поставила его первым — молча.
	want := `{"id":"11111111-1111-1111-1111-111111111111","code":"classic","version":"1.0.0",` +
		`"cards":{"6-diamonds":"/assets/card-sets/classic/6-diamonds.png",` +
		`"10-hearts":"/assets/card-sets/classic/10-hearts.png",` +
		`"Joker":"/assets/card-sets/classic/Joker.png",` +
		`"back":"/assets/card-sets/classic/back.png"}}`
	if string(body) != want {
		t.Errorf("манифест разошёлся\nполучили: %s\nждали:    %s", body, want)
	}
}

// Пустой набор картинок — это `{}`, как пустой LinkedHashMap в Java, а не `null`.
func TestManifestWithoutAssetsIsEmptyObject(t *testing.T) {
	body, err := json.Marshal(CardSetManifestView{ID: "id", Code: "empty", Version: "1.0.0",
		Cards: ManifestCards{}})
	if err != nil {
		t.Fatalf("сериализация: %v", err)
	}
	want := `{"id":"id","code":"empty","version":"1.0.0","cards":{}}`
	if string(body) != want {
		t.Errorf("получили %s, ждали %s", body, want)
	}
}

// ⚠️ MD-003: null-поля Java вырезает целиком, а false и пустая строка остаются.
// Поэтому description и previewUrl — указатели с omitempty, а isDefault — значение без него.
func TestCardSetViewCutsOnlyNulls(t *testing.T) {
	body, err := json.Marshal(CardSetView{ID: "id", Code: "classic", Name: "Классический",
		Version: "1.0.0"})
	if err != nil {
		t.Fatalf("сериализация: %v", err)
	}
	want := `{"id":"id","code":"classic","name":"Классический","version":"1.0.0","isDefault":false}`
	if string(body) != want {
		t.Errorf("получили %s, ждали %s", body, want)
	}
}

func TestTableThemeViewCutsOnlyNulls(t *testing.T) {
	felt := "#1f6f43"
	body, err := json.Marshal(TableThemeView{ID: "id", Code: "green-felt", Name: "Зелёное сукно",
		FeltColor: &felt, IsDefault: true})
	if err != nil {
		t.Fatalf("сериализация: %v", err)
	}
	// defaultBackCode пуст — ключа нет вовсе; isDefault остаётся всегда.
	want := `{"id":"id","code":"green-felt","name":"Зелёное сукно","feltColor":"#1f6f43","isDefault":true}`
	if string(body) != want {
		t.Errorf("получили %s, ждали %s", body, want)
	}
}
