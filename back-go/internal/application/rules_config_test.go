package application

import (
	"testing"

	"github.com/awesomeme01/bardak/back-go/internal/domain/game"
)

// Разбор `rules_config` стола.
//
// ⭐ Главное свойство здесь не «поля читаются», а «стол продолжает играть»: конфиг
// пустой, старый, кривой — матч всё равно должен начаться, но по умолчаниям, а не
// по половине настроек.

func TestRulesConfigFallsBackToDefaultsWhenEmpty(t *testing.T) {
	for _, raw := range []string{"", "{}", "null"} {
		got := ParseRulesConfig(raw, nil)
		if got.DealSize != game.DefaultRulesConfig().DealSize {
			t.Fatalf("конфиг %q: DealSize = %d, ждали умолчание %d",
				raw, got.DealSize, game.DefaultRulesConfig().DealSize)
		}
		if len(got.NavesScale.Ranks) != len(game.FullNavesScale().Ranks) {
			t.Fatalf("конфиг %q: шкала длиной %d, ждали полную", raw, len(got.NavesScale.Ranks))
		}
	}
}

func TestRulesConfigReadsEveryField(t *testing.T) {
	raw := `{"dealSize":5,"maxAttackFirstRound":4,"maxAttackPerRound":7,
	         "transfersEnabled":false,"jokersEnabled":false,
	         "naves":{"enabled":false,"scale":["9","10","J","Q","K","A"]}}`

	got := ParseRulesConfig(raw, nil)

	if got.DealSize != 5 || got.MaxAttackFirstRound != 4 || got.MaxAttackPerRound != 7 {
		t.Fatalf("числа разобраны неверно: %+v", got)
	}
	if got.TransfersEnabled || got.JokersEnabled || got.NavesEnabled {
		t.Fatalf("флаги разобраны неверно: %+v", got)
	}
	if len(got.NavesScale.Ranks) != 6 || got.NavesScale.Ranks[0] != game.Nine {
		t.Fatalf("шкала разобрана неверно: %v", got.NavesScale.Ranks)
	}
}

// ⚠️ Отсутствующее поле и поле, равное нулю, — разные вещи. Наивный разбор в значение
// прочитал бы отсутствующий `transfersEnabled` как false и молча выключил переводы.
func TestRulesConfigKeepsDefaultsForMissingFields(t *testing.T) {
	got := ParseRulesConfig(`{"dealSize":5}`, nil)

	defaults := game.DefaultRulesConfig()
	if !got.TransfersEnabled || !got.JokersEnabled || !got.NavesEnabled {
		t.Fatalf("отсутствующие флаги затёрты: %+v", got)
	}
	if got.MaxAttackPerRound != defaults.MaxAttackPerRound {
		t.Fatalf("отсутствующий лимит затёрт: %d", got.MaxAttackPerRound)
	}
	if got.DealSize != 5 {
		t.Fatalf("заданное поле не прочитано: %d", got.DealSize)
	}
}

// Кривой конфиг — не отказ в старте матча: за столом сидят живые люди.
func TestRulesConfigSurvivesBrokenInput(t *testing.T) {
	cases := map[string]string{
		"не JSON":             `{ это не json`,
		"чужие типы":          `{"dealSize":"шесть"}`,
		"ерунда в числах":     `{"dealSize":0}`,
		"неизвестная ступень": `{"naves":{"scale":["6","Ы"]}}`,
		"пустая шкала":        `{"naves":{"scale":[]}}`,
	}
	defaults := game.DefaultRulesConfig()

	for name, raw := range cases {
		got := ParseRulesConfig(raw, nil)
		if got.DealSize != defaults.DealSize {
			t.Fatalf("%s: DealSize = %d, ждали умолчание", name, got.DealSize)
		}
		if len(got.NavesScale.Ranks) != len(defaults.NavesScale.Ranks) {
			t.Fatalf("%s: шкала длиной %d, ждали полную", name, len(got.NavesScale.Ranks))
		}
	}
}

// ⚠️ Одно поле чужого типа не отменяет остальные — так читает Jackson (`asInt(default)`),
// и так же обязан читать Go. Разбор «всё или ничего» вернул бы стол к умолчаниям целиком,
// то есть поменял бы правила игры посреди вечера, ничего никому не сказав.
func TestRulesConfigKeepsGoodFieldsWhenOneIsBroken(t *testing.T) {
	got := ParseRulesConfig(`{"dealSize":"шесть","maxAttackPerRound":7,"transfersEnabled":false}`, nil)

	if got.DealSize != game.DefaultRulesConfig().DealSize {
		t.Fatalf("кривое поле прочитано: DealSize = %d", got.DealSize)
	}
	if got.MaxAttackPerRound != 7 {
		t.Fatalf("соседнее целое затёрто кривым полем: %d", got.MaxAttackPerRound)
	}
	if got.TransfersEnabled {
		t.Fatalf("соседний флаг затёрт кривым полем")
	}
}
